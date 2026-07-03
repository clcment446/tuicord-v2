package media

import (
	"testing"
)

func TestClassifyURL(t *testing.T) {
	tests := []struct {
		name string
		url  string
		want Class
	}{
		// ── Stickers ─────────────────────────────────────────────────────────
		{
			name: "media.discordapp.net sticker",
			url:  "https://media.discordapp.net/stickers/1234567890/sticker.png",
			want: ClassSticker,
		},
		{
			name: "cdn.discordapp.com sticker",
			url:  "https://cdn.discordapp.com/stickers/987654321.gif",
			want: ClassSticker,
		},
		{
			name: "cdn sticker with query params",
			url:  "https://cdn.discordapp.com/stickers/111?size=512&passthrough=false",
			want: ClassSticker,
		},
		{
			name: "media sticker with format query",
			url:  "https://media.discordapp.net/stickers/999?format=webp&size=160",
			want: ClassSticker,
		},

		// ── Emojis ───────────────────────────────────────────────────────────
		{
			name: "cdn emoji plain",
			url:  "https://cdn.discordapp.com/emojis/123456789.png",
			want: ClassEmoji,
		},
		{
			name: "cdn emoji animated",
			url:  "https://cdn.discordapp.com/emojis/123456789.gif",
			want: ClassEmoji,
		},
		{
			name: "cdn emoji with name query (fake-nitro pattern)",
			url:  "https://cdn.discordapp.com/emojis/912345678?size=48&name=kekw",
			want: ClassEmoji,
		},

		// ── GIFs ─────────────────────────────────────────────────────────────
		{
			name: "plain .gif extension",
			url:  "https://example.com/animation.gif",
			want: ClassGIF,
		},
		{
			name: ".gif extension with query string",
			url:  "https://example.com/image.gif?v=3",
			want: ClassGIF,
		},
		{
			name: ".GIF uppercase extension",
			url:  "https://example.com/IMAGE.GIF",
			want: ClassGIF,
		},
		{
			name: "tenor.com host",
			url:  "https://tenor.com/view/cat-gif-12345678",
			want: ClassGIF,
		},
		{
			name: "media.tenor.com host",
			url:  "https://media.tenor.com/AbCdEf12GhI/cat.gif",
			want: ClassGIF,
		},
		{
			name: "c.tenor.com host",
			url:  "https://c.tenor.com/AbCdEf12GhI/cat.gif",
			want: ClassGIF,
		},
		{
			name: "giphy.com host",
			url:  "https://giphy.com/gifs/cat-funny-AbCdEf12",
			want: ClassGIF,
		},
		{
			name: "media.giphy.com host",
			url:  "https://media.giphy.com/media/AbCdEf12/giphy.gif",
			want: ClassGIF,
		},
		{
			name: "i.giphy.com host",
			url:  "https://i.giphy.com/media/AbCdEf12/giphy.gif",
			want: ClassGIF,
		},

		// ── Images ───────────────────────────────────────────────────────────
		{
			name: ".png extension",
			url:  "https://example.com/image.png",
			want: ClassImage,
		},
		{
			name: ".jpg extension",
			url:  "https://example.com/photo.jpg",
			want: ClassImage,
		},
		{
			name: ".jpeg extension",
			url:  "https://example.com/photo.jpeg",
			want: ClassImage,
		},
		{
			name: ".webp extension",
			url:  "https://example.com/picture.webp",
			want: ClassImage,
		},
		{
			name: ".PNG uppercase",
			url:  "https://example.com/logo.PNG",
			want: ClassImage,
		},
		{
			name: "image with query string",
			url:  "https://cdn.discordapp.com/attachments/1/2/image.png?ex=abc&is=def",
			want: ClassImage,
		},
		// cdn.discordapp.com path that is NOT sticker or emoji → Image by ext
		{
			name: "cdn attachment png (not sticker/emoji path)",
			url:  "https://cdn.discordapp.com/attachments/123/456/photo.jpg",
			want: ClassImage,
		},

		// ── Videos ───────────────────────────────────────────────────────────
		{
			name: ".mp4 extension",
			url:  "https://example.com/video.mp4",
			want: ClassVideo,
		},
		{
			name: ".webm extension",
			url:  "https://example.com/clip.webm",
			want: ClassVideo,
		},
		{
			name: ".mov extension",
			url:  "https://example.com/recording.mov",
			want: ClassVideo,
		},
		{
			name: ".MP4 uppercase",
			url:  "https://example.com/CLIP.MP4",
			want: ClassVideo,
		},

		// ── Plain ─────────────────────────────────────────────────────────────
		{
			name: "plain HTTPS URL no extension",
			url:  "https://discord.com/channels/123/456/789",
			want: ClassPlain,
		},
		{
			name: "plain HTTP URL",
			url:  "http://example.com/",
			want: ClassPlain,
		},
		{
			name: "empty string",
			url:  "",
			want: ClassPlain,
		},
		{
			name: "unparseable URL",
			url:  "://not-a-url",
			want: ClassPlain,
		},
		{
			name: "text message",
			url:  "hello world",
			want: ClassPlain,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange: tc.url set above.
			// Act.
			got := ClassifyURL(tc.url)
			// Assert.
			if got != tc.want {
				t.Errorf("ClassifyURL(%q) = %d, want %d", tc.url, got, tc.want)
			}
		})
	}
}

// TestClassifyURL_StickerBeatsExtension verifies that the Discord sticker CDN
// path takes precedence over the file extension (a sticker URL ending in .gif
// must still be ClassSticker, not ClassGIF).
func TestClassifyURL_StickerBeatsExtension(t *testing.T) {
	// Arrange.
	url := "https://cdn.discordapp.com/stickers/1234567.gif"
	// Act.
	got := ClassifyURL(url)
	// Assert.
	if got != ClassSticker {
		t.Errorf("ClassifyURL(%q) = %d, want ClassSticker (%d)", url, got, ClassSticker)
	}
}

// TestClassifyURL_EmojiBeatsExtension verifies that an emoji CDN URL with a
// .gif extension is still classified as ClassEmoji, not ClassGIF.
func TestClassifyURL_EmojiBeatsExtension(t *testing.T) {
	// Arrange.
	url := "https://cdn.discordapp.com/emojis/999888777.gif"
	// Act.
	got := ClassifyURL(url)
	// Assert.
	if got != ClassEmoji {
		t.Errorf("ClassifyURL(%q) = %d, want ClassEmoji (%d)", url, got, ClassEmoji)
	}
}
