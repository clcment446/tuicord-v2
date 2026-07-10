package app

import (
	"errors"
	"sync"
	"testing"
	"time"

	"awesomeProject/internal/store"

	"github.com/diamondburned/arikawa/v3/api"
	"github.com/diamondburned/arikawa/v3/discord"
)

// fakeThreadClient records thread REST calls. Each method sends on sig after it
// runs so tests can wait for the background goroutine deterministically.
type fakeThreadClient struct {
	mu        sync.Mutex
	active    *api.ActiveThreads
	activeN   int
	activeErr error
	archived  *api.ArchivedThreads
	archivedN int
	joinN     int
	leaveN    int
	startMsgN int
	err       error
	sig       chan struct{}
}

func (f *fakeThreadClient) signal() {
	if f.sig != nil {
		f.sig <- struct{}{}
	}
}

func (f *fakeThreadClient) ActiveThreads(discord.GuildID) (*api.ActiveThreads, error) {
	f.mu.Lock()
	f.activeN++
	f.mu.Unlock()
	if f.activeErr != nil {
		return nil, f.activeErr
	}
	if f.active == nil {
		return &api.ActiveThreads{}, nil
	}
	return f.active, nil
}

func (f *fakeThreadClient) PublicArchivedThreads(discord.ChannelID, discord.Timestamp, uint) (*api.ArchivedThreads, error) {
	f.mu.Lock()
	f.archivedN++
	f.mu.Unlock()
	if f.archived == nil {
		return &api.ArchivedThreads{}, nil
	}
	return f.archived, nil
}

func (f *fakeThreadClient) StartThreadWithMessage(discord.ChannelID, discord.MessageID, api.StartThreadData) (*discord.Channel, error) {
	f.mu.Lock()
	f.startMsgN++
	f.mu.Unlock()
	defer f.signal()
	return &discord.Channel{}, f.err
}

func (f *fakeThreadClient) StartThreadWithoutMessage(discord.ChannelID, api.StartThreadData) (*discord.Channel, error) {
	return &discord.Channel{}, f.err
}

func (f *fakeThreadClient) JoinThread(discord.ChannelID) error {
	f.mu.Lock()
	f.joinN++
	f.mu.Unlock()
	defer f.signal()
	return f.err
}

func (f *fakeThreadClient) LeaveThread(discord.ChannelID) error {
	f.mu.Lock()
	f.leaveN++
	f.mu.Unlock()
	defer f.signal()
	return f.err
}

func newThreadTestApp(tc threadClient) (*App, chan struct{}) {
	changed := make(chan struct{}, 8)
	a := &App{store: store.New(0), ui: syncPoster{}, threads: tc}
	a.OnChange(func() { changed <- struct{}{} })
	return a, changed
}

func waitSig(t *testing.T, ch chan struct{}) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for background work")
	}
}

func TestLoadActiveThreadsPopulatesStore(t *testing.T) {
	tc := &fakeThreadClient{active: &api.ActiveThreads{
		Threads: []discord.Channel{
			{ID: 10, Type: discord.GuildPublicThread, ParentID: 100, Name: "a"},
			{ID: 11, Type: discord.GuildPublicThread, ParentID: 100, Name: "b"},
		},
		Members: []discord.ThreadMember{{ID: 10}},
	}}
	a, changed := newThreadTestApp(tc)
	a.LoadActiveThreads(1)
	waitSig(t, changed)

	got := a.Store().Threads(100)
	if len(got) != 2 {
		t.Fatalf("threads = %d, want 2", len(got))
	}
	c, _ := a.Store().Channel(10)
	if !c.Thread.Joined {
		t.Error("thread 10 should be marked joined from Members")
	}
	c11, _ := a.Store().Channel(11)
	if c11.Thread.Joined {
		t.Error("thread 11 should not be joined")
	}
}

func TestLoadActiveThreadsGatedOncePerGuild(t *testing.T) {
	tc := &fakeThreadClient{}
	a, changed := newThreadTestApp(tc)
	a.LoadActiveThreads(1)
	waitSig(t, changed)
	a.LoadActiveThreads(1) // gated: no second call
	if tc.activeN != 1 {
		t.Errorf("ActiveThreads called %d times, want 1 (gated)", tc.activeN)
	}
}

func TestLoadActiveThreadsRetriesAfterError(t *testing.T) {
	tc := &fakeThreadClient{activeErr: errors.New("boom")}
	errs := make(chan struct{}, 4)
	a := &App{store: store.New(0), ui: syncPoster{}, threads: tc}
	a.OnError(func(error) { errs <- struct{}{} })
	changed := make(chan struct{}, 4)
	a.OnChange(func() { changed <- struct{}{} })

	a.LoadActiveThreads(1)
	waitSig(t, errs) // failed load
	tc.mu.Lock()
	tc.activeErr = nil
	tc.mu.Unlock()
	a.LoadActiveThreads(1) // gating cleared on failure, so this retries
	waitSig(t, changed)
	if tc.activeN != 2 {
		t.Errorf("ActiveThreads called %d times, want 2 after error", tc.activeN)
	}
}

func TestJoinLeaveThread(t *testing.T) {
	tc := &fakeThreadClient{sig: make(chan struct{}, 4)}
	a, _ := newThreadTestApp(tc)
	a.JoinThread(10)
	waitSig(t, tc.sig)
	a.LeaveThread(10)
	waitSig(t, tc.sig)
	if tc.joinN != 1 || tc.leaveN != 1 {
		t.Errorf("join=%d leave=%d, want 1/1", tc.joinN, tc.leaveN)
	}
}

func TestCreateThreadFromMessageValidates(t *testing.T) {
	tc := &fakeThreadClient{sig: make(chan struct{}, 4)}
	a, _ := newThreadTestApp(tc)
	a.CreateThreadFromMessage(100, 5, "   ") // blank name: no call
	a.CreateThreadFromMessage(0, 5, "name")  // no channel: no call
	a.CreateThreadFromMessage(100, 5, "topic")
	waitSig(t, tc.sig)
	if tc.startMsgN != 1 {
		t.Errorf("StartThreadWithMessage called %d times, want 1", tc.startMsgN)
	}
}

func TestPublishCrossposts(t *testing.T) {
	fs := &fakeSender{done: make(chan struct{})}
	a := &App{store: store.New(0), ui: syncPoster{}, send: fs}
	a.Publish(100, 5)
	waitSig(t, fs.done)
	if fs.crossposted != 1 {
		t.Errorf("crossposted = %d, want 1", fs.crossposted)
	}
}
