package ui

import (
	"strings"
	"testing"
	"time"

	"awesomeProject/internal/store"
)

func TestMediaPrefetchURLsPrioritizesActiveGuildAndSkipsLottie(t *testing.T) {
	st := store.New(0)
	st.UpsertGuild(store.Guild{ID: 1, Name: "Home"})
	st.UpsertGuild(store.Guild{ID: 2, Name: "Other"})
	st.SetGuildEmojis(1, []store.GuildEmoji{{ID: 10, Name: "home"}})
	st.SetGuildStickers(1, []store.GuildSticker{{ID: 11, Name: "home-sticker", Format: store.StickerPNG}})
	st.SetGuildEmojis(2, []store.GuildEmoji{{ID: 20, Name: "other"}})
	st.SetGuildStickers(2, []store.GuildSticker{{ID: 21, Name: "lottie", Format: store.StickerLottie}})

	urls := mediaPrefetchURLs(st, 1)
	if len(urls) != 3 {
		t.Fatalf("urls = %v, want 3 cacheable media URLs", urls)
	}
	for _, url := range urls[:2] {
		if !strings.Contains(url, "10") && !strings.Contains(url, "11") {
			t.Fatalf("active guild URL %q was not prioritized: %v", url, urls)
		}
	}
	if !strings.Contains(urls[2], "20") {
		t.Fatalf("other guild URL = %q, want emoji 20", urls[2])
	}
}

func TestPrefetchEligibleOnlyAfterIdleDelay(t *testing.T) {
	last := time.Date(2026, 7, 16, 17, 0, 0, 0, time.UTC)
	if prefetchEligible(last, last.Add(idlePrefetchDelay-time.Nanosecond)) {
		t.Fatal("prefetch started before the idle delay")
	}
	if !prefetchEligible(last, last.Add(idlePrefetchDelay)) {
		t.Fatal("prefetch did not start after the idle delay")
	}
}
