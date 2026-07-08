package media_test

import (
	"fmt"

	"awesomeProject/internal/media"
)

// ClassifyURL is the single entry point for URL type detection. It handles
// Discord CDN patterns, GIF hosts, and common image/video extensions with no
// network I/O. These examples cover the most common cases a Discord client
// will encounter.
func ExampleClassifyURL() {
	urls := []string{
		"https://cdn.discordapp.com/stickers/1234567890.png",
		"https://cdn.discordapp.com/emojis/987654321.png?size=48&name=kekw",
		"https://media.tenor.com/AbCdEf12GhI/cat.gif",
		"https://example.com/photo.jpg",
		"https://example.com/clip.mp4",
		"https://discord.com/channels/123/456/789",
	}
	names := map[media.Class]string{
		media.ClassPlain:   "Plain",
		media.ClassImage:   "Image",
		media.ClassGIF:     "GIF",
		media.ClassVideo:   "Video",
		media.ClassSticker: "Sticker",
		media.ClassEmoji:   "Emoji",
	}
	for _, u := range urls {
		c := media.ClassifyURL(u)
		fmt.Println(names[c])
	}
	// Output:
	// Sticker
	// Emoji
	// GIF
	// Image
	// Video
	// Plain
}

// Fake-nitro sticker links — bare CDN URLs without Discord's embed wrapper —
// are classified as ClassSticker so the renderer can display them correctly.
func ExampleClassifyURL_fakeNitro() {
	// A bare sticker CDN link shared by a user without Nitro.
	stickerURL := "https://media.discordapp.net/stickers/1122334455?format=webp&size=160"
	// A bare emoji CDN link (often has &name= appended by Discord clients).
	emojiURL := "https://cdn.discordapp.com/emojis/9988776655?size=48&name=pepega"

	fmt.Println(media.ClassifyURL(stickerURL))
	fmt.Println(media.ClassifyURL(emojiURL))
	// Output:
	// 4
	// 5
}

// DefaultConfig documents the factory defaults for the media subsystem.
func ExampleDefaultConfig() {
	cfg := media.DefaultConfig()
	fmt.Println("enabled:", cfg.Enabled)
	fmt.Println("max_height_cells:", cfg.MaxHeightCells)
	fmt.Println("animate:", cfg.Animate)
	fmt.Println("emoji_images:", cfg.EmojiImages)
	fmt.Println("video_player:", cfg.VideoPlayer)
	// Output:
	// enabled: true
	// max_height_cells: 12
	// animate: true
	// emoji_images: true
	// video_player: xdg-open
}
