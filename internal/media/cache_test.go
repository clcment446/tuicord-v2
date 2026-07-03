package media

import (
	"image"
	"os"
	"path/filepath"
	"testing"
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
