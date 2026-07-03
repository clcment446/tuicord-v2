package media

import (
	"net/url"
	"path"
	"strings"
)

// Class describes the media type of a URL as determined by ClassifyURL.
type Class int

const (
	// ClassPlain is returned when the URL matches no known media pattern.
	ClassPlain Class = iota
	// ClassImage is returned for static image URLs (.png, .jpg, .jpeg, .webp).
	ClassImage
	// ClassGIF is returned for animated GIF URLs or known GIF-hosting domains
	// (tenor, giphy).
	ClassGIF
	// ClassVideo is returned for video URLs (.mp4, .webm, .mov).
	ClassVideo
	// ClassSticker is returned for Discord sticker CDN URLs on
	// media.discordapp.net/stickers/... or cdn.discordapp.com/stickers/...
	// These include the "fake-nitro sticker" pattern where users share bare
	// CDN links.
	ClassSticker
	// ClassEmoji is returned for Discord emoji CDN URLs on
	// cdn.discordapp.com/emojis/... (often with an &name= query parameter).
	// These include the "fake-nitro emoji" pattern.
	ClassEmoji
)

// ClassifyURL examines rawURL and returns the most specific Class without
// making any network requests. Detection is case-insensitive for extensions
// and hostnames. Query strings are ignored for path-based rules.
//
// Priority order:
//  1. Discord sticker CDN path  → ClassSticker
//  2. Discord emoji CDN path    → ClassEmoji
//  3. Known GIF hosts (tenor, giphy) → ClassGIF
//  4. .gif extension            → ClassGIF
//  5. .png / .jpg / .jpeg / .webp → ClassImage
//  6. .mp4 / .webm / .mov      → ClassVideo
//  7. anything else             → ClassPlain
func ClassifyURL(rawURL string) Class {
	u, err := url.Parse(rawURL)
	if err != nil {
		return ClassPlain
	}

	host := strings.ToLower(u.Hostname())
	p := u.Path

	// 1. Discord sticker
	if isDiscordMedia(host) && strings.HasPrefix(p, "/stickers/") {
		return ClassSticker
	}

	// 2. Discord emoji
	if host == "cdn.discordapp.com" && strings.HasPrefix(p, "/emojis/") {
		return ClassEmoji
	}

	// 3. Known GIF hosting domains
	if isGIFHost(host) {
		return ClassGIF
	}

	// 4-6. Extension-based detection (case-insensitive)
	switch strings.ToLower(path.Ext(p)) {
	case ".gif":
		return ClassGIF
	case ".png", ".jpg", ".jpeg", ".webp":
		return ClassImage
	case ".mp4", ".webm", ".mov":
		return ClassVideo
	}

	return ClassPlain
}

// isDiscordMedia reports whether host is one of the Discord CDN origins that
// serve stickers.
func isDiscordMedia(host string) bool {
	return host == "media.discordapp.net" || host == "cdn.discordapp.com"
}

// isGIFHost reports whether host is a known GIF-hosting service.
func isGIFHost(host string) bool {
	switch {
	case host == "tenor.com",
		host == "media.tenor.com",
		host == "c.tenor.com",
		strings.HasSuffix(host, ".tenor.com"),
		host == "giphy.com",
		host == "media.giphy.com",
		host == "i.giphy.com",
		strings.HasSuffix(host, ".giphy.com"):
		return true
	}
	return false
}
