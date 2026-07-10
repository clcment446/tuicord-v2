package app

import (
	"testing"

	"awesomeProject/internal/store"

	"github.com/diamondburned/arikawa/v3/discord"
)

func TestConvertGuildEmojis(t *testing.T) {
	in := []discord.Emoji{
		{ID: 0, Name: "🙂"},                      // unicode: skipped (no ID)
		{ID: 10, Name: "blob", Animated: false}, // custom static
		{ID: 11, Name: "spin", Animated: true},  // custom animated
	}
	got := convertGuildEmojis(in)
	if len(got) != 2 {
		t.Fatalf("converted %d emoji, want 2 (unicode dropped): %+v", len(got), got)
	}
	if got[0].ID != 10 || got[0].Name != "blob" || got[0].Animated {
		t.Errorf("static emoji = %+v", got[0])
	}
	if got[1].ID != 11 || !got[1].Animated {
		t.Errorf("animated emoji = %+v", got[1])
	}
	if convertGuildEmojis(nil) != nil {
		t.Error("nil input should convert to nil")
	}
}

func TestConvertGuildStickers(t *testing.T) {
	in := []discord.Sticker{
		{ID: 0, Name: "invalid"},
		{ID: 99, Name: "wave", FormatType: discord.StickerFormatPNG},
	}
	got := convertGuildStickers(in)
	if len(got) != 1 || got[0].ID != 99 || got[0].Name != "wave" {
		t.Fatalf("converted stickers = %+v", got)
	}
	if got[0].Format != store.StickerPNG {
		t.Errorf("format = %v, want PNG", got[0].Format)
	}
	if convertGuildStickers(nil) != nil {
		t.Error("nil input should convert to nil")
	}
}
