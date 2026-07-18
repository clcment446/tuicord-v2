package ui

import (
	"context"
	"sync"

	"awesomeProject/internal/media"
	"awesomeProject/internal/picker"
	"awesomeProject/internal/store"
)

// idleMediaPrefetcher warms the shared media cache one item at a time while
// the user is idle. It is cancelled as soon as input resumes.
type idleMediaPrefetcher struct {
	fetcher *media.Fetcher

	mu     sync.Mutex
	cancel context.CancelFunc
	gen    uint64
	closed bool
}

func newIdleMediaPrefetcher(fetcher *media.Fetcher) *idleMediaPrefetcher {
	return &idleMediaPrefetcher{fetcher: fetcher}
}

func (p *idleMediaPrefetcher) Start(urls []string) {
	if p == nil || p.fetcher == nil || len(urls) == 0 {
		return
	}
	p.Stop()
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	p.gen++
	gen := p.gen
	p.cancel = cancel
	p.mu.Unlock()
	go func() {
		for _, url := range urls {
			if ctx.Err() != nil {
				return
			}
			_, _ = p.fetcher.Fetch(ctx, url)
		}
		p.mu.Lock()
		if p.gen == gen {
			p.cancel = nil
		}
		p.mu.Unlock()
	}()
}

func (p *idleMediaPrefetcher) Stop() {
	if p == nil {
		return
	}
	p.mu.Lock()
	p.gen++
	cancel := p.cancel
	p.cancel = nil
	p.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

func (p *idleMediaPrefetcher) Close() {
	if p == nil {
		return
	}
	p.mu.Lock()
	p.closed = true
	p.gen++
	cancel := p.cancel
	p.cancel = nil
	p.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

func mediaPrefetchURLs(st *store.Store, active store.GuildID) []string {
	if st == nil {
		return nil
	}
	guilds := st.Guilds()
	ordered := make([]store.Guild, 0, len(guilds))
	for _, guild := range guilds {
		if guild.ID == active {
			ordered = append(ordered, guild)
		}
	}
	for _, guild := range guilds {
		if guild.ID != active {
			ordered = append(ordered, guild)
		}
	}
	seen := make(map[string]bool)
	urls := make([]string, 0)
	appendURL := func(url string) {
		if url != "" && !seen[url] {
			seen[url] = true
			urls = append(urls, url)
		}
	}
	for _, guild := range ordered {
		for _, emoji := range st.GuildEmojis(guild.ID) {
			appendURL(picker.EmojiCDNURL(emoji.ID, emoji.Name, emoji.Animated))
		}
		for _, sticker := range st.GuildStickers(guild.ID) {
			if sticker.Format != store.StickerLottie {
				appendURL(picker.StickerCDNURL(sticker.ID))
			}
		}
	}
	return urls
}
