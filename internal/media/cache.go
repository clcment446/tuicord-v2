package media

import (
	"container/list"
	"crypto/sha256"
	"fmt"
	"image"
	"os"
	"path/filepath"
	"sync"
)

const defaultLRUSize = 64

// Cache is a two-level media cache: a bounded in-memory LRU of decoded
// image.Image values (keyed by URL) backed by a disk store of raw encoded
// bytes (stored at Dir/<sha256(url)>).
//
// The zero value is not usable; construct one with NewCache.
type Cache struct {
	// Dir is the root directory for on-disk raw-byte storage.
	// Set via NewCache; override before the first Put to redirect the path.
	Dir string

	mu      sync.Mutex
	lru     *list.List
	items   map[string]*list.Element
	maxSize int
}

type lruEntry struct {
	key string
	img image.Image
}

// NewCache returns a Cache with the given LRU capacity and disk directory.
// If maxLRU is ≤ 0, it defaults to 64. If dir is empty, the platform cache
// directory (os.UserCacheDir()/tuicord/media) is used.
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
	return &Cache{
		Dir:     dir,
		lru:     list.New(),
		items:   make(map[string]*list.Element),
		maxSize: maxLRU,
	}, nil
}

// GetLRU returns the decoded image stored in the LRU for the given URL, or
// nil when the key is absent. The accessed entry is promoted to the front.
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

// PutLRU inserts img into the in-memory LRU under url, evicting the least
// recently used entry when the capacity is exceeded.
func (c *Cache) PutLRU(url string, img image.Image) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if elem, ok := c.items[url]; ok {
		c.lru.MoveToFront(elem)
		elem.Value.(*lruEntry).img = img
		return
	}
	entry := &lruEntry{key: url, img: img}
	c.items[url] = c.lru.PushFront(entry)
	c.evictLocked()
}

// evictLocked removes least-recently-used entries until the LRU is within its
// size limit. The caller must hold c.mu.
func (c *Cache) evictLocked() {
	for c.lru.Len() > c.maxSize {
		elem := c.lru.Back()
		if elem == nil {
			return
		}
		entry := elem.Value.(*lruEntry)
		delete(c.items, entry.key)
		c.lru.Remove(elem)
	}
}

// LRULen returns the number of entries currently held in the LRU.
func (c *Cache) LRULen() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.lru.Len()
}

// GetDisk looks up the raw bytes for url in the disk cache. It returns
// (nil, nil) when the entry is absent (not an error).
func (c *Cache) GetDisk(url string) ([]byte, error) {
	p := c.diskPath(url)
	data, err := os.ReadFile(p)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("media: disk cache read: %w", err)
	}
	return data, nil
}

// PutDisk writes raw bytes to the disk cache for url, creating the directory
// if necessary. Write errors are returned but never fatal to the caller; a
// disk-write failure simply means the entry will not be cached on disk.
func (c *Cache) PutDisk(url string, raw []byte) error {
	if err := os.MkdirAll(c.Dir, 0o700); err != nil {
		return fmt.Errorf("media: create cache dir: %w", err)
	}
	p := c.diskPath(url)
	if err := os.WriteFile(p, raw, 0o600); err != nil {
		return fmt.Errorf("media: disk cache write: %w", err)
	}
	return nil
}

// diskPath returns the absolute path for the disk cache entry of url.
// The filename is the lowercase hex encoding of SHA-256(url), which is
// collision-resistant and safe for all file systems.
func (c *Cache) diskPath(url string) string {
	sum := sha256.Sum256([]byte(url))
	return filepath.Join(c.Dir, fmt.Sprintf("%x", sum))
}
