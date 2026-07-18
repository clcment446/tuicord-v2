package app

import (
	"sync"
	"testing"

	"awesomeProject/internal/store"

	"github.com/diamondburned/arikawa/v3/discord"
	"github.com/diamondburned/arikawa/v3/gateway"
)

type channelLoadStep struct {
	ready   chan struct{}
	release chan struct{}
	result  []discord.Channel
}

type sequencedChannelLoader struct {
	mu    sync.Mutex
	steps []*channelLoadStep
	calls int
}

func (l *sequencedChannelLoader) Channels(discord.GuildID) ([]discord.Channel, error) {
	l.mu.Lock()
	step := l.steps[l.calls]
	l.calls++
	l.mu.Unlock()
	close(step.ready)
	<-step.release
	return append([]discord.Channel(nil), step.result...), nil
}

func (l *sequencedChannelLoader) callCount() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.calls
}

type channelDetailStep struct {
	ready   chan struct{}
	release chan struct{}
	result  discord.Channel
}

type sequencedChannelDetails struct {
	mu    sync.Mutex
	steps []*channelDetailStep
	calls int
}

func (l *sequencedChannelDetails) Channel(discord.ChannelID) (*discord.Channel, error) {
	l.mu.Lock()
	step := l.steps[l.calls]
	l.calls++
	l.mu.Unlock()
	close(step.ready)
	<-step.release
	result := step.result
	return &result, nil
}

func TestChannelLoaderStaleCompletionCannotPoisonRecreatedGuildGate(t *testing.T) {
	first := &channelLoadStep{
		ready:   make(chan struct{}),
		release: make(chan struct{}),
		result:  []discord.Channel{{ID: 10, Type: discord.GuildText, Name: "old"}},
	}
	second := &channelLoadStep{
		ready:   make(chan struct{}),
		release: make(chan struct{}),
		result:  []discord.Channel{{ID: 20, Type: discord.GuildText, Name: "new"}},
	}
	loader := &sequencedChannelLoader{steps: []*channelLoadStep{first, second}}
	ui := newChannelPoster()
	a := &App{store: store.New(0), ui: ui, chans: loader}
	a.store.UpsertGuild(store.Guild{ID: 1, Name: "first lifetime"})

	a.LoadChannels(1)
	waitSig(t, first.ready)

	// Delete and recreate the same guild ID while the old REST request is
	// blocked. Permanent deletion must reset the gate for the new lifetime.
	a.handleGuildDelete(&gateway.GuildDeleteEvent{ID: 1})
	ui.runNext(t)
	a.handleGuildCreate(&gateway.GuildCreateEvent{Guild: discord.Guild{ID: 1, Name: "second lifetime"}})
	ui.runNext(t)

	a.LoadChannels(1)
	waitSig(t, second.ready)
	if got := loader.callCount(); got != 2 {
		t.Fatalf("channel loader calls = %d, want new lifetime request", got)
	}

	// The first completion must neither recreate channel 10 nor clear/complete
	// the second request's pending gate.
	close(first.release)
	ui.runNext(t)
	if _, ok := a.store.Channel(10); ok {
		t.Fatal("stale channel response recreated an object from the old guild lifetime")
	}
	a.resourceMu.Lock()
	_, stillPending := a.channelsGate.pending[store.GuildID(1)]
	a.resourceMu.Unlock()
	if !stillPending {
		t.Fatal("stale completion cleared the new lifetime's pending gate")
	}

	close(second.release)
	ui.runNext(t)
	channel, ok := a.store.Channel(20)
	if !ok || channel.Name != "new" || channel.GuildID != 1 {
		t.Fatalf("new lifetime channel = %+v, %t", channel, ok)
	}
}

func TestForumMetadataStaleCompletionCannotOverwriteRecreatedChannelOrPendingGate(t *testing.T) {
	first := &channelDetailStep{
		ready:   make(chan struct{}),
		release: make(chan struct{}),
		result:  discord.Channel{ID: 100, GuildID: 1, Type: discord.GuildForum, Name: "old metadata"},
	}
	second := &channelDetailStep{
		ready:   make(chan struct{}),
		release: make(chan struct{}),
		result:  discord.Channel{ID: 100, GuildID: 1, Type: discord.GuildForum, Name: "new metadata"},
	}
	details := &sequencedChannelDetails{steps: []*channelDetailStep{first, second}}
	ui := newChannelPoster()
	a := &App{store: store.New(0), ui: ui, channelDetail: details}
	a.store.UpsertGuild(store.Guild{ID: 1})
	a.store.UpsertChannel(store.Channel{ID: 100, GuildID: 1, Kind: store.ChannelForum, Name: "initial"})

	a.LoadForumMetadata(100)
	waitSig(t, first.ready)
	a.handleChannelDelete(&gateway.ChannelDeleteEvent{Channel: discord.Channel{ID: 100, GuildID: 1}})
	ui.runNext(t)
	a.handleChannelUpsert(discord.Channel{ID: 100, GuildID: 1, Type: discord.GuildForum, Name: "recreated"})
	ui.runNext(t)
	a.LoadForumMetadata(100)
	waitSig(t, second.ready)

	close(first.release)
	ui.runNext(t)
	channel, _ := a.store.Channel(100)
	if channel.Name != "recreated" {
		t.Fatalf("stale forum metadata overwrote recreated channel: %+v", channel)
	}
	a.resourceMu.Lock()
	_, stillPending := a.forumMetaPending[store.ChannelID(100)]
	a.resourceMu.Unlock()
	if !stillPending {
		t.Fatal("stale forum completion cleared the new lifetime's pending gate")
	}

	close(second.release)
	ui.runNext(t)
	channel, _ = a.store.Channel(100)
	if channel.Name != "new metadata" {
		t.Fatalf("new forum metadata was not installed: %+v", channel)
	}
}

func TestHistoryDiscardedWhenBoundedTombstonesOverflowInFlight(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	fake := &fakeSender{
		history: []discord.Message{
			{ID: 3, ChannelID: 7},
			{ID: 2, ChannelID: 7},
			{ID: 1, ChannelID: 7},
		},
		historyDone:    started,
		historyRelease: release,
	}
	ui := newChannelPoster()
	a := &App{store: store.New(2), ui: ui, history: fake}
	a.store.UpsertChannel(store.Channel{ID: 7, GuildID: 1, Kind: store.ChannelText})

	a.LoadHistory(7, 3)
	waitSig(t, started)
	for id := store.MessageID(1); id <= 3; id++ {
		a.store.RemoveMessage(7, id)
	}
	if got := a.store.MessageTombstoneCount(7); got != 2 {
		t.Fatalf("tombstones = %d, want history bound 2", got)
	}
	close(release)
	ui.runNext(t)
	if got := a.store.Messages(7); len(got) != 0 {
		t.Fatalf("overflowed in-flight history was installed: %+v", got)
	}
}
