package app

import (
	"errors"
	"sync"
	"testing"

	"awesomeProject/internal/store"

	"github.com/diamondburned/arikawa/v3/api"
	"github.com/diamondburned/arikawa/v3/discord"
	"github.com/diamondburned/arikawa/v3/gateway"
)

// syncPoster runs posted closures immediately, as if already on the UI goroutine.
type syncPoster struct{}

func (syncPoster) Post(fn func()) { fn() }

// fakeSender records sends and returns a preset error.
type fakeSender struct {
	mu           sync.Mutex
	sent         int
	err          error
	done         chan struct{}
	history      []discord.Message
	historyErr   error
	historyN     int
	historyDone  chan struct{}
	roles        []discord.Role
	rolesErr     error
	rolesN       int
	rolesDone    chan struct{}
	guilds       []discord.Guild
	guildsErr    error
	guildsN      int
	guildsDone   chan struct{}
	dms          []discord.Channel
	dmsErr       error
	channels     []discord.Channel
	channelsErr  error
	channelsN    int
	channelsDone chan struct{}
}

func (f *fakeSender) SendMessageComplex(discord.ChannelID, api.SendMessageData) (*discord.Message, error) {
	f.mu.Lock()
	f.sent++
	f.mu.Unlock()
	if f.done != nil {
		close(f.done)
	}
	return &discord.Message{}, f.err
}

func (f *fakeSender) Messages(discord.ChannelID, uint) ([]discord.Message, error) {
	f.mu.Lock()
	f.historyN++
	f.mu.Unlock()
	if f.historyDone != nil {
		close(f.historyDone)
		f.historyDone = nil
	}
	return append([]discord.Message(nil), f.history...), f.historyErr
}

func (f *fakeSender) Roles(discord.GuildID) ([]discord.Role, error) {
	f.mu.Lock()
	f.rolesN++
	f.mu.Unlock()
	if f.rolesDone != nil {
		close(f.rolesDone)
		f.rolesDone = nil
	}
	return append([]discord.Role(nil), f.roles...), f.rolesErr
}

func (f *fakeSender) Guilds(uint) ([]discord.Guild, error) {
	f.mu.Lock()
	f.guildsN++
	f.mu.Unlock()
	if f.guildsDone != nil {
		close(f.guildsDone)
		f.guildsDone = nil
	}
	return append([]discord.Guild(nil), f.guilds...), f.guildsErr
}

func (f *fakeSender) PrivateChannels() ([]discord.Channel, error) {
	return append([]discord.Channel(nil), f.dms...), f.dmsErr
}

func (f *fakeSender) Channels(discord.GuildID) ([]discord.Channel, error) {
	f.mu.Lock()
	f.channelsN++
	f.mu.Unlock()
	if f.channelsDone != nil {
		close(f.channelsDone)
		f.channelsDone = nil
	}
	return append([]discord.Channel(nil), f.channels...), f.channelsErr
}

func newTestApp(send sender) *App {
	a := &App{
		store: store.New(0),
		ui:    syncPoster{},
		send:  send,
	}
	if history, ok := send.(historyLoader); ok {
		a.history = history
	}
	if roles, ok := send.(roleLoader); ok {
		a.roles = roles
	}
	if dirs, ok := send.(directoryLoader); ok {
		a.dirs = dirs
	}
	if chans, ok := send.(channelLoader); ok {
		a.chans = chans
	}
	return a
}

func TestSendAppendsOptimisticPending(t *testing.T) {
	fs := &fakeSender{done: make(chan struct{})}
	a := newTestApp(fs)
	a.SetActive(1, 42)

	a.Send("hello")

	msgs := a.store.Messages(42)
	if len(msgs) != 1 {
		t.Fatalf("want 1 optimistic message, got %d", len(msgs))
	}
	if !msgs[0].Pending || msgs[0].Content != "hello" {
		t.Errorf("optimistic message = %+v, want pending 'hello'", msgs[0])
	}
	<-fs.done // deliver goroutine ran
}

func TestDeliverMarksFailedOnError(t *testing.T) {
	fs := &fakeSender{err: errors.New("boom")}
	a := newTestApp(fs)
	a.SetActive(1, 42)
	a.store.AppendMessage(store.Message{ChannelID: 42, Nonce: "n1", Pending: true, Content: "hi"})

	// Call deliver directly (synchronous) to avoid racing with Send's goroutine;
	// syncPoster then applies MarkFailed on this goroutine.
	a.deliver(42, "hi", "n1")

	msgs := a.store.Messages(42)
	if len(msgs) != 1 || !msgs[0].Failed || msgs[0].Pending {
		t.Errorf("message after failed deliver = %+v, want failed and not pending", msgs[0])
	}
}

func TestDeliverReportsError(t *testing.T) {
	sendErr := errors.New("boom")
	fs := &fakeSender{err: sendErr}
	a := newTestApp(fs)
	a.SetActive(1, 42)
	a.store.AppendMessage(store.Message{ChannelID: 42, Nonce: "n1", Pending: true, Content: "hi"})

	var got error
	a.OnError(func(err error) { got = err })
	a.deliver(42, "hi", "n1")

	if !errors.Is(got, sendErr) {
		t.Fatalf("reported error = %v, want %v", got, sendErr)
	}
}

func TestSendIgnoredWithoutActiveChannel(t *testing.T) {
	fs := &fakeSender{}
	a := newTestApp(fs)

	a.Send("hello") // no active channel

	fs.mu.Lock()
	defer fs.mu.Unlock()
	if fs.sent != 0 {
		t.Errorf("sent %d messages without an active channel, want 0", fs.sent)
	}
}

func TestReconcileReplacesPendingOnGatewayEcho(t *testing.T) {
	a := newTestApp(&fakeSender{})
	a.SetActive(1, 42)
	a.store.AppendMessage(store.Message{ChannelID: 42, Nonce: "n1", Pending: true, Content: "hi"})

	echo := store.Message{ID: 99, ChannelID: 42, Nonce: "n1", Content: "hi"}
	if !a.store.ReplaceMessage(echo.Nonce, echo) {
		a.store.AppendMessage(echo)
	}

	msgs := a.store.Messages(42)
	if len(msgs) != 1 {
		t.Fatalf("want 1 message after reconcile, got %d", len(msgs))
	}
	if msgs[0].Pending || msgs[0].ID != 99 {
		t.Errorf("reconciled message = %+v, want id 99 not pending", msgs[0])
	}
}

func TestReadyEventLoadsDiscordGuildData(t *testing.T) {
	a := newTestApp(&fakeSender{})
	readyCalled := false
	a.OnReady(func() { readyCalled = true })

	a.handleReady(&gateway.ReadyEvent{Guilds: []gateway.GuildCreateEvent{{
		Guild: discord.Guild{ID: 1, Name: "gophers", Roles: []discord.Role{
			{ID: 200, Name: "admin", Position: 10, Hoist: true, Mentionable: true},
			{ID: 201, Name: "member", Position: 1},
		}},
		Channels: []discord.Channel{
			{ID: 10, Type: discord.GuildText, Name: "general", Position: 1},
			{ID: 11, Type: discord.GuildVoice, Name: "voice", Position: 2},
		},
		Members: []discord.Member{{
			User:    discord.User{ID: 100, Username: "alice"},
			Nick:    "ali",
			RoleIDs: []discord.RoleID{200, 201},
		}},
	}}})

	guilds := a.store.Guilds()
	if len(guilds) != 1 || guilds[0].ID != 1 || guilds[0].Name != "gophers" {
		t.Fatalf("loaded guilds = %+v, want gophers", guilds)
	}
	if name, ok := a.store.GuildName(1); !ok || name != "gophers" {
		t.Fatalf("GuildName = %q,%v, want gophers,true", name, ok)
	}
	channels := a.store.Channels(1)
	if len(channels) != 2 || channels[0].ID != 10 || channels[0].GuildID != 1 || channels[1].Kind != store.ChannelVoice {
		t.Fatalf("loaded channels = %+v, want Discord channels in guild 1", channels)
	}
	if name, ok := a.store.ChannelName(10); !ok || name != "general" {
		t.Fatalf("ChannelName = %q,%v, want general,true", name, ok)
	}
	if name, ok := a.store.MemberName(1, 100); !ok || name != "ali" {
		t.Fatalf("loaded member = %q,%v, want ali,true", name, ok)
	}
	member, ok := a.store.Member(1, 100)
	if !ok || len(member.RoleIDs) != 2 || member.RoleIDs[0] != 200 || member.RoleIDs[1] != 201 {
		t.Fatalf("loaded member roles = %+v,%v, want 200,201", member, ok)
	}
	role, ok := a.store.Role(1, 200)
	if !ok || role.Name != "admin" || !role.Hoist || !role.Mentionable {
		t.Fatalf("loaded role = %+v,%v, want admin hoisted mentionable", role, ok)
	}
	if !readyCalled {
		t.Fatal("OnReady was not called after loading Discord READY data")
	}
}

func TestReadyEventLoadsDMUserNames(t *testing.T) {
	a := newTestApp(&fakeSender{})

	a.handleReady(&gateway.ReadyEvent{PrivateChannels: []discord.Channel{
		{
			ID:   91,
			Type: discord.DirectMessage,
			DMRecipients: []discord.User{{
				ID:          100,
				Username:    "alice",
				DisplayName: "Alice A.",
			}},
		},
		{
			ID:   92,
			Type: discord.GroupDM,
			DMRecipients: []discord.User{
				{ID: 101, Username: "bob"},
				{ID: 102, Username: "carol"},
			},
		},
	}})

	if name, ok := a.store.ChannelName(91); !ok || name != "Alice A." {
		t.Fatalf("DM ChannelName = %q,%v, want Alice A.,true", name, ok)
	}
	if name, ok := a.store.ChannelName(92); !ok || name != "bob, carol" {
		t.Fatalf("group DM ChannelName = %q,%v, want bob, carol,true", name, ok)
	}
	if name, ok := a.store.GuildName(DirectMessagesGuildID); !ok || name != "Direct Messages" {
		t.Fatalf("DM guild = %q,%v, want Direct Messages,true", name, ok)
	}
	dms := a.store.Channels(DirectMessagesGuildID)
	if len(dms) != 2 || dms[0].Kind != store.ChannelDM || dms[1].Kind != store.ChannelDM {
		t.Fatalf("DM channels = %+v, want two ChannelDM entries under synthetic DM guild", dms)
	}
}

func TestMessageCreateEventLoadsDiscordMessage(t *testing.T) {
	a := newTestApp(&fakeSender{})
	a.SetActive(1, 42)
	changed := false
	a.OnChange(func() { changed = true })

	a.handleMessageCreate(&gateway.MessageCreateEvent{Message: discord.Message{
		ID:        99,
		GuildID:   1,
		ChannelID: 43,
		Author:    discord.User{ID: 100, Username: "alice"},
		Content:   "hello from api",
	}, Member: &discord.Member{
		Nick:    "ali",
		RoleIDs: []discord.RoleID{200},
	}})

	msgs := a.store.Messages(43)
	if len(msgs) != 1 || msgs[0].ID != 99 || msgs[0].Author != "alice" || msgs[0].Content != "hello from api" {
		t.Fatalf("loaded messages = %+v, want Discord message", msgs)
	}
	if got := a.store.Unread(43); got != 1 {
		t.Fatalf("unread = %d, want 1 for inactive channel", got)
	}
	if !changed {
		t.Fatal("OnChange was not called after loading Discord message")
	}
	member, ok := a.store.Member(1, 100)
	if !ok || member.Name != "ali" || len(member.RoleIDs) != 1 || member.RoleIDs[0] != 200 {
		t.Fatalf("message member = %+v,%v, want ali with role 200", member, ok)
	}
}

func TestLoadHistoryStoresDiscordMessagesOldestFirst(t *testing.T) {
	fs := &fakeSender{history: []discord.Message{
		{
			ID:        103,
			ChannelID: 42,
			Author:    discord.User{ID: 1, Username: "alice"},
			Content:   "newest",
		},
		{
			ID:        102,
			ChannelID: 42,
			Author:    discord.User{ID: 2, DisplayName: "Bobby", Username: "bob"},
			Content:   "middle",
		},
		{
			ID:        101,
			ChannelID: 42,
			Author:    discord.User{ID: 3, Username: "carol"},
			Content:   "oldest",
		},
	}}
	a := newTestApp(fs)
	a.store.UpsertGuild(store.Guild{ID: 1, Name: "gophers"})
	a.store.UpsertChannel(store.Channel{ID: 42, GuildID: 1, Name: "general"})
	changed := false
	a.OnChange(func() { changed = true })

	a.loadHistory(42, 50)

	if name, ok := a.store.GuildName(1); !ok || name != "gophers" {
		t.Fatalf("GuildName = %q,%v, want gophers,true", name, ok)
	}
	if name, ok := a.store.ChannelName(42); !ok || name != "general" {
		t.Fatalf("ChannelName = %q,%v, want general,true", name, ok)
	}
	msgs := a.store.Messages(42)
	if len(msgs) != 3 {
		t.Fatalf("history length = %d, want 3", len(msgs))
	}
	if msgs[0].Content != "oldest" || msgs[1].Content != "middle" || msgs[2].Content != "newest" {
		t.Fatalf("history = %+v, want oldest/middle/newest", msgs)
	}
	if msgs[1].Author != "Bobby" {
		t.Fatalf("history author = %q, want display name Bobby", msgs[1].Author)
	}
	if !changed {
		t.Fatal("OnChange was not called after loading history")
	}
}

func TestLoadHistoryStoresDMHistory(t *testing.T) {
	fs := &fakeSender{history: []discord.Message{
		{
			ID:        202,
			ChannelID: 91,
			Author:    discord.User{ID: 100, Username: "alice"},
			Content:   "new dm",
		},
		{
			ID:        201,
			ChannelID: 91,
			Author:    discord.User{ID: 200, Username: "you"},
			Content:   "old dm",
		},
	}}
	a := newTestApp(fs)
	a.store.UpsertGuild(store.Guild{ID: DirectMessagesGuildID, Name: "Direct Messages"})
	a.store.UpsertChannel(store.Channel{ID: 91, GuildID: DirectMessagesGuildID, Name: "alice", Kind: store.ChannelDM})

	a.loadHistory(91, 50)

	if name, ok := a.store.GuildName(DirectMessagesGuildID); !ok || name != "Direct Messages" {
		t.Fatalf("DM guild = %q,%v, want Direct Messages,true", name, ok)
	}
	if name, ok := a.store.ChannelName(91); !ok || name != "alice" {
		t.Fatalf("DM ChannelName = %q,%v, want alice,true", name, ok)
	}
	msgs := a.store.Messages(91)
	if len(msgs) != 2 || msgs[0].Content != "old dm" || msgs[1].Content != "new dm" {
		t.Fatalf("DM history = %+v, want oldest/newest", msgs)
	}
}

func TestLoadHistoryReportsDiscordError(t *testing.T) {
	historyErr := errors.New("history failed")
	a := newTestApp(&fakeSender{historyErr: historyErr})
	var got error
	a.OnError(func(err error) { got = err })

	a.loadHistory(42, 50)

	if !errors.Is(got, historyErr) {
		t.Fatalf("reported error = %v, want %v", got, historyErr)
	}
}

func TestLoadHistoryUsesSessionCache(t *testing.T) {
	fs := &fakeSender{
		history:     []discord.Message{{ID: 1, ChannelID: 42, Content: "hi"}},
		historyDone: make(chan struct{}),
	}
	a := newTestApp(fs)

	a.LoadHistory(42, 50)
	<-fs.historyDone
	a.LoadHistory(42, 50)

	fs.mu.Lock()
	defer fs.mu.Unlock()
	if fs.historyN != 1 {
		t.Fatalf("history API calls = %d, want 1", fs.historyN)
	}
}

func TestLoadRolesStoresRolesAndUsesSessionCache(t *testing.T) {
	fs := &fakeSender{
		roles: []discord.Role{
			{ID: 200, Name: "admin", Position: 10},
			{ID: 201, Name: "member", Position: 1},
		},
		rolesDone: make(chan struct{}),
	}
	a := newTestApp(fs)

	a.LoadRoles(1)
	<-fs.rolesDone
	a.LoadRoles(1)

	fs.mu.Lock()
	roleCalls := fs.rolesN
	fs.mu.Unlock()
	if roleCalls != 1 {
		t.Fatalf("roles API calls = %d, want 1", roleCalls)
	}
	roles := a.store.Roles(1)
	if len(roles) != 2 || roles[0].ID != 200 || roles[1].ID != 201 {
		t.Fatalf("roles = %+v, want position-sorted 200,201", roles)
	}
}

func TestReadyRolePayloadPreventsRoleRefetch(t *testing.T) {
	fs := &fakeSender{}
	a := newTestApp(fs)
	a.handleReady(&gateway.ReadyEvent{Guilds: []gateway.GuildCreateEvent{{
		Guild: discord.Guild{ID: 1, Name: "gophers", Roles: []discord.Role{{ID: 200, Name: "admin"}}},
	}}})

	a.LoadRoles(1)

	fs.mu.Lock()
	defer fs.mu.Unlock()
	if fs.rolesN != 0 {
		t.Fatalf("roles API calls = %d, want 0 after READY roles loaded", fs.rolesN)
	}
}

func TestLoadGuildsLoadsDirectoryAndUsesSessionCache(t *testing.T) {
	fs := &fakeSender{
		guilds: []discord.Guild{{ID: 1, Name: "gophers"}},
		dms: []discord.Channel{{
			ID:           91,
			Type:         discord.DirectMessage,
			DMRecipients: []discord.User{{ID: 100, Username: "alice"}},
		}},
		guildsDone: make(chan struct{}),
	}
	a := newTestApp(fs)
	ready := false
	a.OnReady(func() { ready = true })

	a.LoadGuilds(100)
	<-fs.guildsDone
	a.LoadGuilds(100)

	fs.mu.Lock()
	guildCalls := fs.guildsN
	fs.mu.Unlock()
	if guildCalls != 1 {
		t.Fatalf("guild API calls = %d, want 1", guildCalls)
	}
	if name, ok := a.store.GuildName(1); !ok || name != "gophers" {
		t.Fatalf("GuildName = %q,%v, want gophers,true", name, ok)
	}
	if name, ok := a.store.ChannelName(91); !ok || name != "alice" {
		t.Fatalf("DM ChannelName = %q,%v, want alice,true", name, ok)
	}
	if !ready {
		t.Fatal("OnReady was not called after REST directory load")
	}
}

func TestLoadChannelsLoadsGuildChannelsAndUsesSessionCache(t *testing.T) {
	fs := &fakeSender{
		channels:     []discord.Channel{{ID: 10, Type: discord.GuildText, Name: "general"}},
		channelsDone: make(chan struct{}),
	}
	a := newTestApp(fs)

	a.LoadChannels(1)
	<-fs.channelsDone
	a.LoadChannels(1)

	fs.mu.Lock()
	channelCalls := fs.channelsN
	fs.mu.Unlock()
	if channelCalls != 1 {
		t.Fatalf("channel API calls = %d, want 1", channelCalls)
	}
	channels := a.store.Channels(1)
	if len(channels) != 1 || channels[0].ID != 10 || channels[0].Name != "general" {
		t.Fatalf("channels = %+v, want #general", channels)
	}
}

func TestReadyChannelPayloadPreventsChannelRefetch(t *testing.T) {
	fs := &fakeSender{}
	a := newTestApp(fs)
	a.handleReady(&gateway.ReadyEvent{Guilds: []gateway.GuildCreateEvent{{
		Guild:    discord.Guild{ID: 1, Name: "gophers"},
		Channels: []discord.Channel{{ID: 10, Type: discord.GuildText, Name: "general"}},
	}}})

	a.LoadChannels(1)

	fs.mu.Lock()
	defer fs.mu.Unlock()
	if fs.channelsN != 0 {
		t.Fatalf("channel API calls = %d, want 0 after READY channels loaded", fs.channelsN)
	}
}
