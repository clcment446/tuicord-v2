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

const defaultLRUSize = 64

// Cache is a bounded decoded-image LRU backed by a TTL and byte-bounded raw
// disk cache. The zero value is not usable; construct one with NewCache.
type Cache struct {
	Dir string

	mu      sync.Mutex
	lru     *list.List
	items   map[string]*list.Element
	maxSize int

	diskMu       sync.Mutex
	diskMaxBytes int64
	diskTTL      time.Duration
	now          func() time.Time
}

type lruEntry struct {
	key string
	img image.Image
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
		diskMaxBytes: DefaultDiskCacheMaxBytes,
		diskTTL:      DefaultDiskCacheTTL,
		now:          time.Now,
	}
	if _, err := os.Stat(dir); err == nil {
		_ = c.pruneDiskLocked()
	}
	return c, nil
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
	c.mu.Lock()
	defer c.mu.Unlock()
	if elem, ok := c.items[url]; ok {
		c.lru.MoveToFront(elem)
		elem.Value.(*lruEntry).img = img
		return
	}
	c.items[url] = c.lru.PushFront(&lruEntry{key: url, img: img})
	for c.lru.Len() > c.maxSize {
		elem := c.lru.Back()
		if elem == nil {
			break
		}
		delete(c.items, elem.Value.(*lruEntry).key)
		c.lru.Remove(elem)
	}
}

func (c *Cache) LRULen() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.lru.Len()
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
