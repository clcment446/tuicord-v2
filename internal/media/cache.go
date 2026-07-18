package media

import (
	"container/list"
	"crypto/sha256"
	"fmt"
	"image"
	"io"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

const (
	defaultLRUSize = 64
	// DefaultDecodedCacheMaxBytes independently bounds decoded pixel memory;
	// the entry count alone is not meaningful for large source images.
	DefaultDecodedCacheMaxBytes int64 = 64 << 20
)

// Cache is a bounded decoded-image LRU backed by a TTL and byte-bounded raw
// disk cache. The zero value is not usable; construct one with NewCache.
type Cache struct {
	Dir string

	mu       sync.Mutex
	lru      *list.List
	items    map[string]*list.Element
	maxSize  int
	maxBytes int64
	lruBytes int64

	diskMu       sync.Mutex
	diskMaxBytes int64
	diskTTL      time.Duration
	now          func() time.Time
}

type lruEntry struct {
	key   string
	img   image.Image
	bytes int64
}

// NewCache returns a cache with safe default disk limits.
func NewCache(maxLRU int, dir string) (*Cache, error) {
	if maxLRU <= 0 {
		maxLRU = defaultLRUSize
	}
	if dir == "" {
		base, err := os.UserCacheDir()
		if err != nil {
			return nil, fmt.Errorf("media: resolve cache dir: %w", err)
		}
		dir = filepath.Join(base, "tuicord", "media")
	}
	c := &Cache{
		Dir:          dir,
		lru:          list.New(),
		items:        make(map[string]*list.Element),
		maxSize:      maxLRU,
		maxBytes:     DefaultDecodedCacheMaxBytes,
		diskMaxBytes: DefaultDiskCacheMaxBytes,
		diskTTL:      DefaultDiskCacheTTL,
		now:          time.Now,
	}
	if _, err := os.Stat(dir); err == nil {
		_ = c.pruneDiskLocked()
	}
	return c, nil
}

// NewMemoryCache constructs only the decoded LRU. It performs no cache-dir
// resolution, stat, pruning, deletion, or other disk IO.
func NewMemoryCache(maxLRU int, maxBytes int64) *Cache {
	if maxLRU <= 0 {
		maxLRU = defaultLRUSize
	}
	if maxBytes <= 0 {
		maxBytes = DefaultDecodedCacheMaxBytes
	}
	return &Cache{
		lru:      list.New(),
		items:    make(map[string]*list.Element),
		maxSize:  maxLRU,
		maxBytes: maxBytes,
		now:      time.Now,
	}
}

// ConfigureLRU sets decoded entry and byte limits and immediately evicts least
// recently used entries until both limits are met.
func (c *Cache) ConfigureLRU(maxEntries int, maxBytes int64) {
	if c == nil {
		return
	}
	if maxEntries <= 0 {
		maxEntries = defaultLRUSize
	}
	if maxBytes <= 0 {
		maxBytes = DefaultDecodedCacheMaxBytes
	}
	c.mu.Lock()
	c.maxSize, c.maxBytes = maxEntries, maxBytes
	c.evictLRULocked()
	c.mu.Unlock()
}

// ConfigureDisk overrides the persistent cache byte and TTL limits. Nonpositive
// values select the bounded defaults rather than disabling enforcement.
func (c *Cache) ConfigureDisk(maxBytes int64, ttl time.Duration) {
	if c == nil {
		return
	}
	if maxBytes <= 0 {
		maxBytes = DefaultDiskCacheMaxBytes
	}
	if ttl <= 0 {
		ttl = DefaultDiskCacheTTL
	}
	c.diskMu.Lock()
	c.diskMaxBytes, c.diskTTL = maxBytes, ttl
	if _, err := os.Stat(c.Dir); err == nil {
		_ = c.pruneDiskLocked()
	}
	c.diskMu.Unlock()
}

func (c *Cache) GetLRU(url string) image.Image {
	c.mu.Lock()
	defer c.mu.Unlock()
	elem, ok := c.items[url]
	if !ok {
		return nil
	}
	c.lru.MoveToFront(elem)
	return elem.Value.(*lruEntry).img
}

func (c *Cache) PutLRU(url string, img image.Image) {
	if c == nil || img == nil {
		return
	}
	bytes := decodedImageBytes(img)
	c.mu.Lock()
	defer c.mu.Unlock()
	if bytes <= 0 || bytes > c.maxBytes {
		// Replacing an existing value with an uncacheable image must not leave the
		// old URL mapped to unrelated pixels.
		if elem, ok := c.items[url]; ok {
			c.removeLRULocked(elem)
		}
		return
	}
	if elem, ok := c.items[url]; ok {
		entry := elem.Value.(*lruEntry)
		c.lruBytes -= entry.bytes
		entry.img, entry.bytes = img, bytes
		c.lruBytes += bytes
		c.lru.MoveToFront(elem)
		c.evictLRULocked()
		return
	}
	c.items[url] = c.lru.PushFront(&lruEntry{key: url, img: img, bytes: bytes})
	c.lruBytes += bytes
	c.evictLRULocked()
}

func (c *Cache) evictLRULocked() {
	for c.lru.Len() > c.maxSize || c.lruBytes > c.maxBytes {
		elem := c.lru.Back()
		if elem == nil {
			break
		}
		c.removeLRULocked(elem)
	}
}

func (c *Cache) removeLRULocked(elem *list.Element) {
	entry := elem.Value.(*lruEntry)
	delete(c.items, entry.key)
	c.lruBytes -= entry.bytes
	c.lru.Remove(elem)
}

// decodedImageBytes estimates retained decoded storage. Known standard image
// implementations use their actual pixel slices; generic implementations use a
// conservative four-byte RGBA surface estimate.
func decodedImageBytes(img image.Image) int64 {
	if img == nil {
		return 0
	}
	switch img := img.(type) {
	case *image.RGBA:
		return int64(len(img.Pix))
	case *image.NRGBA:
		return int64(len(img.Pix))
	case *image.RGBA64:
		return int64(len(img.Pix))
	case *image.NRGBA64:
		return int64(len(img.Pix))
	case *image.Alpha:
		return int64(len(img.Pix))
	case *image.Alpha16:
		return int64(len(img.Pix))
	case *image.Gray:
		return int64(len(img.Pix))
	case *image.Gray16:
		return int64(len(img.Pix))
	case *image.CMYK:
		return int64(len(img.Pix))
	case *image.Paletted:
		return int64(len(img.Pix)) + int64(len(img.Palette))*8
	case *image.YCbCr:
		return int64(len(img.Y)) + int64(len(img.Cb)) + int64(len(img.Cr))
	}
	b := img.Bounds()
	pixels, ok := checkedMul(int64(b.Dx()), int64(b.Dy()))
	if !ok {
		return 1<<63 - 1
	}
	bytes, ok := checkedMul(pixels, 4)
	if !ok {
		return 1<<63 - 1
	}
	return bytes
}

func (c *Cache) LRULen() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.lru.Len()
}

func (c *Cache) LRUBytes() int64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.lruBytes
}

// GetDisk returns a non-expired cache entry. It checks file size before reading
// and performs a max+1 read, so even externally replaced cache files are bound.
func (c *Cache) GetDisk(url string) ([]byte, error) {
	return c.GetDiskLimit(url, 0)
}

// GetDiskLimit applies a caller-specific byte cap in addition to the cache's
// global budget, allowing Fetcher response limits to be enforced before read.
func (c *Cache) GetDiskLimit(url string, maxBytes int64) ([]byte, error) {
	c.diskMu.Lock()
	defer c.diskMu.Unlock()
	if maxBytes <= 0 || maxBytes > c.diskMaxBytes {
		maxBytes = c.diskMaxBytes
	}
	p := c.diskPath(url)
	info, err := os.Stat(p)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("media: disk cache stat: %w", err)
	}
	if c.expired(info) {
		_ = os.Remove(p)
		return nil, nil
	}
	if info.Size() > maxBytes {
		_ = os.Remove(p)
		return nil, nil
	}
	f, err := os.Open(p)
	if err != nil {
		return nil, fmt.Errorf("media: disk cache open: %w", err)
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, maxBytes+1))
	if err != nil {
		return nil, fmt.Errorf("media: disk cache read: %w", err)
	}
	if int64(len(data)) > maxBytes {
		_ = os.Remove(p)
		return nil, nil
	}
	return data, nil
}

// PutDisk atomically writes raw bytes then removes expired and oldest entries
// until the directory is within its configured byte budget.
func (c *Cache) PutDisk(url string, raw []byte) error {
	c.diskMu.Lock()
	defer c.diskMu.Unlock()
	if int64(len(raw)) > c.diskMaxBytes {
		return fmt.Errorf("media: disk cache entry is %d bytes, limit is %d", len(raw), c.diskMaxBytes)
	}
	if err := os.MkdirAll(c.Dir, 0o700); err != nil {
		return fmt.Errorf("media: create cache dir: %w", err)
	}
	tmp, err := os.CreateTemp(c.Dir, ".media-*")
	if err != nil {
		return fmt.Errorf("media: create disk cache temp: %w", err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.Write(raw); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("media: disk cache write: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("media: disk cache close: %w", err)
	}
	if err := os.Rename(tmpPath, c.diskPath(url)); err != nil {
		return fmt.Errorf("media: disk cache replace: %w", err)
	}
	return c.pruneDiskLocked()
}

type diskEntry struct {
	path string
	size int64
	mod  time.Time
}

func (c *Cache) pruneDiskLocked() error {
	entries, err := os.ReadDir(c.Dir)
	if err != nil {
		return err
	}
	var files []diskEntry
	var total int64
	for _, entry := range entries {
		if entry.IsDir() || len(entry.Name()) != sha256.Size*2 {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		path := filepath.Join(c.Dir, entry.Name())
		if c.expired(info) {
			_ = os.Remove(path)
			continue
		}
		files = append(files, diskEntry{path: path, size: info.Size(), mod: info.ModTime()})
		total += info.Size()
	}
	sort.Slice(files, func(i, j int) bool { return files[i].mod.Before(files[j].mod) })
	for _, file := range files {
		if total <= c.diskMaxBytes {
			break
		}
		if err := os.Remove(file.path); err == nil {
			total -= file.size
		}
	}
	return nil
}

func (c *Cache) expired(info os.FileInfo) bool {
	return c.diskTTL > 0 && c.now().Sub(info.ModTime()) > c.diskTTL
}

func (c *Cache) diskPath(url string) string {
	sum := sha256.Sum256([]byte(url))
	return filepath.Join(c.Dir, fmt.Sprintf("%x", sum))
}
