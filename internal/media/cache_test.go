package media

import (
	"image"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// blankImg returns a trivially small image for cache tests.
func blankImg(w, h int) image.Image {
	return image.NewRGBA(image.Rect(0, 0, w, h))
}

// newTempCache returns a Cache backed by a temporary directory that is
// cleaned up at the end of the test.
func newTempCache(t *testing.T, maxLRU int) *Cache {
	t.Helper()
	dir := t.TempDir()
	c, err := NewCache(maxLRU, dir)
	if err != nil {
		t.Fatalf("NewCache: %v", err)
	}
	return c
}

// ── LRU behaviour ────────────────────────────────────────────────────────────

func TestCache_LRU_MissOnEmpty(t *testing.T) {
	// Arrange.
	c := newTempCache(t, 4)
	// Act.
	got := c.GetLRU("https://example.com/a.png")
	// Assert.
	if got != nil {
		t.Errorf("GetLRU on empty cache: got %v, want nil", got)
	}
}

func TestCache_LRU_HitAfterPut(t *testing.T) {
	// Arrange.
	c := newTempCache(t, 4)
	img := blankImg(1, 1)
	url := "https://example.com/a.png"
	// Act.
	c.PutLRU(url, img)
	got := c.GetLRU(url)
	// Assert.
	if got == nil {
		t.Fatal("GetLRU after PutLRU: got nil, want the stored image")
	}
}

func TestCache_LRU_EvictsOldestEntry(t *testing.T) {
	// Arrange: capacity of 3 entries.
	c := newTempCache(t, 3)
	urls := []string{
		"https://example.com/1.png",
		"https://example.com/2.png",
		"https://example.com/3.png",
	}
	for _, u := range urls {
		c.PutLRU(u, blankImg(1, 1))
	}
	// Act: insert a 4th entry — should evict the least recently used (url[0]).
	c.PutLRU("https://example.com/4.png", blankImg(1, 1))
	// Assert: url[0] was evicted.
	if got := c.GetLRU(urls[0]); got != nil {
		t.Errorf("expected url[0] to be evicted, but GetLRU returned non-nil")
	}
	// The other three entries must still be present.
	for _, u := range []string{urls[1], urls[2], "https://example.com/4.png"} {
		if got := c.GetLRU(u); got == nil {
			t.Errorf("expected %q to still be cached, but GetLRU returned nil", u)
		}
	}
}

func TestCache_LRU_GetPromotesToFront(t *testing.T) {
	// Arrange: capacity of 3; insert 3 entries, then access the oldest.
	c := newTempCache(t, 3)
	old := "https://example.com/old.png"
	c.PutLRU(old, blankImg(1, 1))
	c.PutLRU("https://example.com/b.png", blankImg(1, 1))
	c.PutLRU("https://example.com/c.png", blankImg(1, 1))
	// Act: access 'old' to promote it, then insert a new entry.
	c.GetLRU(old) // promotes old to front
	c.PutLRU("https://example.com/d.png", blankImg(1, 1))
	// Assert: 'old' was promoted so it should NOT be evicted.
	if got := c.GetLRU(old); got == nil {
		t.Errorf("expected recently-accessed entry to survive eviction, but got nil")
	}
}

func TestCacheLRUEvictsByDecodedBytes(t *testing.T) {
	c := newTempCache(t, 64)
	c.ConfigureLRU(64, 600)
	c.PutLRU("old", blankImg(10, 10)) // 400 RGBA bytes
	c.PutLRU("new", blankImg(10, 10))
	if got := c.GetLRU("old"); got != nil {
		t.Fatal("old entry survived decoded-byte eviction")
	}
	if got := c.GetLRU("new"); got == nil {
		t.Fatal("new entry was unexpectedly evicted")
	}
	if got := c.LRUBytes(); got != 400 {
		t.Fatalf("LRUBytes = %d, want 400", got)
	}
}

func TestCacheLRUReplacementUpdatesMemoryAccounting(t *testing.T) {
	c := newTempCache(t, 64)
	c.ConfigureLRU(64, 1<<20)
	c.PutLRU("same", blankImg(2, 2))
	if got := c.LRUBytes(); got != 16 {
		t.Fatalf("initial LRUBytes = %d, want 16", got)
	}
	c.PutLRU("same", blankImg(3, 4))
	if got := c.LRUBytes(); got != 48 {
		t.Fatalf("replacement LRUBytes = %d, want 48", got)
	}
}

func TestCacheLRUSkipsSingleImageOverByteBudget(t *testing.T) {
	c := NewMemoryCache(64, 100)
	c.PutLRU("large", blankImg(10, 10))
	if c.LRULen() != 0 || c.LRUBytes() != 0 {
		t.Fatalf("oversized image retained: entries=%d bytes=%d", c.LRULen(), c.LRUBytes())
	}
}

func TestCache_LRU_CapacityBound(t *testing.T) {
	// Arrange: capacity of 5.
	c := newTempCache(t, 5)
	for i := range 10 {
		c.PutLRU(string(rune('a'+i)), blankImg(1, 1))
	}
	// Assert: LRU must never exceed the configured size.
	if n := c.LRULen(); n > 5 {
		t.Errorf("LRULen = %d after 10 puts with capacity 5", n)
	}
}

// ── Disk cache ────────────────────────────────────────────────────────────────

func TestCache_Disk_MissOnEmpty(t *testing.T) {
	// Arrange.
	c := newTempCache(t, 4)
	// Act.
	data, err := c.GetDisk("https://example.com/missing.png")
	// Assert.
	if err != nil {
		t.Fatalf("GetDisk on empty dir: unexpected error: %v", err)
	}
	if data != nil {
		t.Errorf("GetDisk on empty dir: got %d bytes, want nil", len(data))
	}
}

func TestCache_Disk_HitAfterPut(t *testing.T) {
	// Arrange.
	c := newTempCache(t, 4)
	url := "https://example.com/img.png"
	raw := []byte("fake png bytes")
	// Act.
	if err := c.PutDisk(url, raw); err != nil {
		t.Fatalf("PutDisk: %v", err)
	}
	got, err := c.GetDisk(url)
	// Assert.
	if err != nil {
		t.Fatalf("GetDisk: %v", err)
	}
	if string(got) != string(raw) {
		t.Errorf("GetDisk: got %q, want %q", got, raw)
	}
}

func TestCache_Disk_PathIsURLInsensitive(t *testing.T) {
	// Arrange: two different URLs must map to different disk paths.
	c := newTempCache(t, 4)
	urlA := "https://example.com/a.png"
	urlB := "https://example.com/b.png"
	if err := c.PutDisk(urlA, []byte("a")); err != nil {
		t.Fatalf("PutDisk a: %v", err)
	}
	if err := c.PutDisk(urlB, []byte("b")); err != nil {
		t.Fatalf("PutDisk b: %v", err)
	}
	// Act.
	gotA, _ := c.GetDisk(urlA)
	gotB, _ := c.GetDisk(urlB)
	// Assert.
	if string(gotA) != "a" || string(gotB) != "b" {
		t.Errorf("disk paths collided: gotA=%q gotB=%q", gotA, gotB)
	}
}

func TestCacheDiskCallerLimitRejectsBeforeRead(t *testing.T) {
	c := newTempCache(t, 4)
	c.ConfigureDisk(100, time.Hour)
	if err := c.PutDisk("large", []byte("123456")); err != nil {
		t.Fatal(err)
	}
	if got, err := c.GetDiskLimit("large", 5); err != nil || got != nil {
		t.Fatalf("GetDiskLimit = %q, %v; want miss", got, err)
	}
	if _, err := os.Stat(c.diskPath("large")); !os.IsNotExist(err) {
		t.Fatalf("oversized cache file was not removed: %v", err)
	}
}

func TestCacheDiskEvictsOldestToByteBudget(t *testing.T) {
	c := newTempCache(t, 4)
	c.ConfigureDisk(5, time.Hour)
	if err := c.PutDisk("old", []byte("1234")); err != nil {
		t.Fatal(err)
	}
	oldTime := time.Now().Add(-time.Minute)
	if err := os.Chtimes(c.diskPath("old"), oldTime, oldTime); err != nil {
		t.Fatal(err)
	}
	if err := c.PutDisk("new", []byte("5678")); err != nil {
		t.Fatal(err)
	}
	if got, _ := c.GetDisk("old"); got != nil {
		t.Fatal("oldest entry survived byte-budget eviction")
	}
	if got, _ := c.GetDisk("new"); string(got) != "5678" {
		t.Fatalf("new entry = %q", got)
	}
}

func TestCacheDiskExpiresByTTL(t *testing.T) {
	c := newTempCache(t, 4)
	c.ConfigureDisk(1<<20, time.Hour)
	if err := c.PutDisk("expired", []byte("data")); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	old := now.Add(-2 * time.Hour)
	if err := os.Chtimes(c.diskPath("expired"), old, old); err != nil {
		t.Fatal(err)
	}
	c.now = func() time.Time { return now }
	if got, err := c.GetDisk("expired"); err != nil || got != nil {
		t.Fatalf("expired GetDisk = %q, %v", got, err)
	}
}

func TestCache_Disk_CreatesDirectory(t *testing.T) {
	// Arrange: point cache at a non-existent sub-directory.
	dir := filepath.Join(t.TempDir(), "sub", "media")
	c, err := NewCache(4, dir)
	if err != nil {
		t.Fatalf("NewCache: %v", err)
	}
	url := "https://example.com/x.png"
	// Act.
	if err := c.PutDisk(url, []byte("data")); err != nil {
		t.Fatalf("PutDisk: %v", err)
	}
	// Assert: directory must now exist.
	if _, err := os.Stat(dir); err != nil {
		t.Errorf("cache dir not created: %v", err)
	}
}
