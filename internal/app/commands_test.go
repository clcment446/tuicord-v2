package app

import (
	"sync"
	"testing"
	"time"

	"awesomeProject/internal/store"

	"github.com/diamondburned/arikawa/v3/discord"
)

type fakeCommandCatalogLoader struct {
	mu       sync.Mutex
	commands []ApplicationCommand
	calls    int
	err      error
}

func (f *fakeCommandCatalogLoader) LoadCommandCatalog(CommandContext) ([]ApplicationCommand, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	return append([]ApplicationCommand(nil), f.commands...), f.err
}

func TestLoadCommandsCachesByContext(t *testing.T) {
	loader := &fakeCommandCatalogLoader{commands: []ApplicationCommand{{Command: discord.Command{ID: 1, Name: "weather"}}}}
	a := &App{
		store:          store.New(0),
		ui:             syncPoster{},
		commandCatalog: loader,
		commandCache:   map[CommandContext]commandCacheEntry{},
		now:            time.Now,
	}
	ctx := CommandContext{GuildID: 7, ChannelID: 42}

	first, err := a.LoadCommands(ctx)
	if err != nil || len(first) != 1 || first[0].Name != "weather" {
		t.Fatalf("first load = %#v, %v", first, err)
	}
	second, err := a.LoadCommands(ctx)
	if err != nil || len(second) != 1 {
		t.Fatalf("cached load = %#v, %v", second, err)
	}
	if loader.calls != 1 {
		t.Fatalf("loader calls = %d, want 1", loader.calls)
	}
}

func TestLoadCommandsSeparatesGuildAndDMContexts(t *testing.T) {
	loader := &fakeCommandCatalogLoader{commands: []ApplicationCommand{{Command: discord.Command{ID: 1, Name: "weather"}}}}
	a := &App{
		store:          store.New(0),
		ui:             syncPoster{},
		commandCatalog: loader,
		commandCache:   map[CommandContext]commandCacheEntry{},
		now:            time.Now,
	}
	if _, err := a.LoadCommands(CommandContext{GuildID: 7, ChannelID: 42}); err != nil {
		t.Fatal(err)
	}
	if _, err := a.LoadCommands(CommandContext{ChannelID: 42}); err != nil {
		t.Fatal(err)
	}
	if loader.calls != 2 {
		t.Fatalf("loader calls = %d, want distinct guild and DM cache entries", loader.calls)
	}
}

func TestLoadCommandsReturnsCopies(t *testing.T) {
	loader := &fakeCommandCatalogLoader{commands: []ApplicationCommand{{Command: discord.Command{ID: 1, Name: "weather"}}}}
	a := &App{
		store:          store.New(0),
		ui:             syncPoster{},
		commandCatalog: loader,
		commandCache:   map[CommandContext]commandCacheEntry{},
		now:            time.Now,
	}
	ctx := CommandContext{GuildID: 7, ChannelID: 42}
	commands, err := a.LoadCommands(ctx)
	if err != nil {
		t.Fatal(err)
	}
	commands[0].Name = "mutated"
	cached, err := a.LoadCommands(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if cached[0].Name != "weather" {
		t.Fatalf("cache returned caller mutation: %#v", cached)
	}
}

func TestLoadCommandsRefreshesAfterTTL(t *testing.T) {
	loader := &fakeCommandCatalogLoader{commands: []ApplicationCommand{{Command: discord.Command{ID: 1, Name: "weather"}}}}
	now := time.Date(2026, time.July, 16, 12, 0, 0, 0, time.UTC)
	a := &App{
		store:          store.New(0),
		ui:             syncPoster{},
		commandCatalog: loader,
		commandCache:   map[CommandContext]commandCacheEntry{},
		now:            func() time.Time { return now },
	}
	ctx := CommandContext{GuildID: 7, ChannelID: 42}
	if _, err := a.LoadCommands(ctx); err != nil {
		t.Fatal(err)
	}
	now = now.Add(commandCatalogTTL)
	if _, err := a.LoadCommands(ctx); err != nil {
		t.Fatal(err)
	}
	if loader.calls != 2 {
		t.Fatalf("loader calls = %d, want refresh after TTL", loader.calls)
	}
}

func TestInvalidateCommandCacheForcesRefresh(t *testing.T) {
	loader := &fakeCommandCatalogLoader{commands: []ApplicationCommand{{Command: discord.Command{ID: 1, Name: "weather"}}}}
	a := &App{
		store:          store.New(0),
		ui:             syncPoster{},
		commandCatalog: loader,
		commandCache:   map[CommandContext]commandCacheEntry{},
		now:            time.Now,
	}
	ctx := CommandContext{GuildID: 7, ChannelID: 42}
	if _, err := a.LoadCommands(ctx); err != nil {
		t.Fatal(err)
	}
	a.InvalidateCommandCache()
	if _, err := a.LoadCommands(ctx); err != nil {
		t.Fatal(err)
	}
	if loader.calls != 2 {
		t.Fatalf("loader calls = %d, want refresh after invalidation", loader.calls)
	}
}
