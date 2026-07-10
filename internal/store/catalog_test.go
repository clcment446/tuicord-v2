package store

import "testing"

func TestGuildEmojiCatalog(t *testing.T) {
	s := New(0)
	if got := s.GuildEmojis(1); got != nil {
		t.Fatalf("empty catalog = %v, want nil", got)
	}
	s.SetGuildEmojis(1, []GuildEmoji{
		{ID: 10, Name: "blob", Animated: false},
		{ID: 11, Name: "spin", Animated: true},
	})
	got := s.GuildEmojis(1)
	if len(got) != 2 || got[0].Name != "blob" || !got[1].Animated {
		t.Fatalf("catalog = %+v", got)
	}
	// Returned slice is a copy.
	got[0].Name = "mutated"
	if s.GuildEmojis(1)[0].Name != "blob" {
		t.Fatal("GuildEmojis aliased internal storage")
	}
	// Empty slice clears.
	s.SetGuildEmojis(1, nil)
	if s.GuildEmojis(1) != nil {
		t.Fatal("expected catalog cleared")
	}
}

func TestGuildStickerCatalog(t *testing.T) {
	s := New(0)
	if got := s.GuildStickers(2); got != nil {
		t.Fatalf("empty sticker catalog = %v, want nil", got)
	}
	s.SetGuildStickers(2, []GuildSticker{{ID: 99, Name: "wave", Format: StickerPNG}})
	got := s.GuildStickers(2)
	if len(got) != 1 || got[0].ID != 99 {
		t.Fatalf("sticker catalog = %+v", got)
	}
	got[0].Name = "mutated"
	if s.GuildStickers(2)[0].Name != "wave" {
		t.Fatal("GuildStickers aliased internal storage")
	}
	s.SetGuildStickers(2, nil)
	if s.GuildStickers(2) != nil {
		t.Fatal("expected sticker catalog cleared")
	}
}

func TestNitroFlag(t *testing.T) {
	s := New(0)
	if s.HasNitro() {
		t.Fatal("new store should default to no nitro")
	}
	s.SetNitro(true)
	if !s.HasNitro() {
		t.Fatal("SetNitro(true) not reflected")
	}
	s.SetNitro(false)
	if s.HasNitro() {
		t.Fatal("SetNitro(false) not reflected")
	}
}
