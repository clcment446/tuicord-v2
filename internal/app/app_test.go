package app

import (
	"errors"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"awesomeProject/internal/store"

	"github.com/diamondburned/arikawa/v3/api"
	"github.com/diamondburned/arikawa/v3/discord"
	"github.com/diamondburned/arikawa/v3/gateway"
	"github.com/diamondburned/arikawa/v3/utils/sendpart"
)

// syncPoster runs posted closures immediately, as if already on the UI goroutine.
type syncPoster struct{}

func (syncPoster) Post(fn func())  { fn() }
func (syncPoster) WriteRaw([]byte) {}
func (syncPoster) Invalidate()     {}
func (syncPoster) ForceRepaint()   {}

// channelPoster lets race tests execute every posted mutation deterministically
// on the test goroutine, matching the production UI-thread store model.
type channelPoster struct {
	posts chan func()
}

func newChannelPoster() *channelPoster  { return &channelPoster{posts: make(chan func(), 16)} }
func (p *channelPoster) Post(fn func()) { p.posts <- fn }
func (*channelPoster) WriteRaw([]byte)  {}
func (*channelPoster) Invalidate()      {}
func (*channelPoster) ForceRepaint()    {}
func (p *channelPoster) runNext(t *testing.T) {
	t.Helper()
	select {
	case fn := <-p.posts:
		fn()
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for posted UI mutation")
	}
}

// fakeSender records sends and returns a preset error.
type fakeSender struct {
	mu                   sync.Mutex
	sent                 int
	lastSend             api.SendMessageData
	edited               int
	editContent          string
	deleted              int
	pinned               int
	unpinned             int
	crossposted          int
	reacted              int
	reaction             discord.APIEmoji
	err                  error
	done                 chan struct{}
	history              []discord.Message
	historyBefore        []discord.Message
	historyErr           error
	historyN             int
	historyDone          chan struct{}
	historyRelease       chan struct{}
	historyBeforeDone    chan struct{}
	historyBeforeRelease chan struct{}
	roles                []discord.Role
	rolesErr             error
	rolesN               int
	guilds               []discord.Guild
	guildsErr            error
	guildsN              int
	dms                  []discord.Channel
	dmsErr               error
	channels             []discord.Channel
	channelsErr          error
	channelsN            int
	channelDetail        *discord.Channel
}

func (f *fakeSender) SendMessageComplex(_ discord.ChannelID, data api.SendMessageData) (*discord.Message, error) {
	f.mu.Lock()
	f.sent++
	f.lastSend = data
	f.mu.Unlock()
	if f.done != nil {
		close(f.done)
	}
	return &discord.Message{}, f.err
}

func (f *fakeSender) EditText(_ discord.ChannelID, _ discord.MessageID, content string) (*discord.Message, error) {
	f.mu.Lock()
	f.edited++
	f.editContent = content
	f.mu.Unlock()
	if f.done != nil {
		close(f.done)
	}
	return &discord.Message{}, f.err
}

func (f *fakeSender) DeleteMessage(discord.ChannelID, discord.MessageID, api.AuditLogReason) error {
	f.mu.Lock()
	f.deleted++
	f.mu.Unlock()
	if f.done != nil {
		close(f.done)
	}
	return f.err
}

func (f *fakeSender) PinMessage(discord.ChannelID, discord.MessageID, api.AuditLogReason) error {
	f.mu.Lock()
	f.pinned++
	f.mu.Unlock()
	if f.done != nil {
		close(f.done)
	}
	return f.err
}

func (f *fakeSender) UnpinMessage(discord.ChannelID, discord.MessageID, api.AuditLogReason) error {
	f.mu.Lock()
	f.unpinned++
	f.mu.Unlock()
	if f.done != nil {
		close(f.done)
	}
	return f.err
}

func (f *fakeSender) CrosspostMessage(discord.ChannelID, discord.MessageID) (*discord.Message, error) {
	f.mu.Lock()
	f.crossposted++
	f.mu.Unlock()
	if f.done != nil {
		close(f.done)
	}
	return &discord.Message{}, f.err
}

func (f *fakeSender) React(_ discord.ChannelID, _ discord.MessageID, emoji discord.APIEmoji) error {
	f.mu.Lock()
	f.reacted++
	f.reaction = emoji
	f.mu.Unlock()
	if f.done != nil {
		close(f.done)
	}
	return f.err
}

func (f *fakeSender) Messages(discord.ChannelID, uint) ([]discord.Message, error) {
	f.mu.Lock()
	f.historyN++
	f.mu.Unlock()
	if f.historyDone != nil {
		close(f.historyDone)
	}
	if f.historyRelease != nil {
		<-f.historyRelease
	}
	return append([]discord.Message(nil), f.history...), f.historyErr
}

func (f *fakeSender) MessagesBefore(discord.ChannelID, discord.MessageID, uint) ([]discord.Message, error) {
	f.mu.Lock()
	f.historyN++
	f.mu.Unlock()
	if f.historyBeforeDone != nil {
		close(f.historyBeforeDone)
	}
	if f.historyBeforeRelease != nil {
		<-f.historyBeforeRelease
	}
	return append([]discord.Message(nil), f.historyBefore...), f.historyErr
}

func (f *fakeSender) Roles(discord.GuildID) ([]discord.Role, error) {
	f.mu.Lock()
	f.rolesN++
	f.mu.Unlock()
	return append([]discord.Role(nil), f.roles...), f.rolesErr
}

func (f *fakeSender) Guilds(uint) ([]discord.Guild, error) {
	f.mu.Lock()
	f.guildsN++
	f.mu.Unlock()
	return append([]discord.Guild(nil), f.guilds...), f.guildsErr
}

func (f *fakeSender) PrivateChannels() ([]discord.Channel, error) {
	return append([]discord.Channel(nil), f.dms...), f.dmsErr
}

func (f *fakeSender) Channels(discord.GuildID) ([]discord.Channel, error) {
	f.mu.Lock()
	f.channelsN++
	f.mu.Unlock()
	return append([]discord.Channel(nil), f.channels...), f.channelsErr
}

func (f *fakeSender) Channel(discord.ChannelID) (*discord.Channel, error) {
	return f.channelDetail, nil
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
	if details, ok := send.(channelDetailsLoader); ok {
		a.channelDetail = details
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

func TestSendIgnoresWhitespaceOnlyContent(t *testing.T) {
	fs := &fakeSender{}
	a := newTestApp(fs)
	a.SetActive(1, 42)

	for _, content := range []string{" ", "\t", "\n \t"} {
		a.Send(content)
	}

	if messages := a.store.Messages(42); len(messages) != 0 {
		t.Fatalf("whitespace sends created optimistic messages: %+v", messages)
	}
	fs.mu.Lock()
	defer fs.mu.Unlock()
	if fs.sent != 0 {
		t.Fatalf("whitespace sends made %d REST calls, want 0", fs.sent)
	}
}

func TestReplyIgnoresWhitespaceOnlyContent(t *testing.T) {
	fs := &fakeSender{}
	a := newTestApp(fs)
	target := store.Message{ID: 7, ChannelID: 42}

	a.Reply(" \n\t", target, true)

	if messages := a.store.Messages(42); len(messages) != 0 {
		t.Fatalf("whitespace reply created optimistic messages: %+v", messages)
	}
	fs.mu.Lock()
	defer fs.mu.Unlock()
	if fs.sent != 0 {
		t.Fatalf("whitespace reply made %d REST calls, want 0", fs.sent)
	}
}

func TestSendFilesSendsMultipartOptimisticallyAndCleansUp(t *testing.T) {
	fs := &fakeSender{done: make(chan struct{})}
	a := newTestApp(fs)
	a.SetActive(1, 42)

	cleaned := make(chan struct{})
	attachments := []store.Attachment{{Filename: "report.txt", ContentType: "text/plain", Size: 6}}
	a.SendFiles("", []sendpart.File{{Name: "report.txt", Reader: strings.NewReader("report")}}, attachments, func() { close(cleaned) })

	msgs := a.store.Messages(42)
	if len(msgs) != 1 || !msgs[0].Pending || msgs[0].Content != "" {
		t.Fatalf("optimistic file-only message = %+v", msgs)
	}
	if got := msgs[0].Attachments; len(got) != 1 || got[0].Filename != "report.txt" || got[0].Size != 6 {
		t.Fatalf("optimistic attachments = %+v", got)
	}

	<-fs.done
	<-cleaned
	fs.mu.Lock()
	defer fs.mu.Unlock()
	if fs.lastSend.Content != "" || len(fs.lastSend.Files) != 1 || fs.lastSend.Files[0].Name != "report.txt" {
		t.Fatalf("send data = %+v", fs.lastSend)
	}
}

func TestSendFilesCleansUpWhenNoSendCanStart(t *testing.T) {
	a := newTestApp(&fakeSender{})
	cleaned := false
	a.SendFiles("", []sendpart.File{{Name: "report.txt", Reader: strings.NewReader("report")}}, nil, func() { cleaned = true })
	if !cleaned {
		t.Fatal("cleanup was not called when there was no active channel")
	}
}

func TestSendFilesCleansUpAfterFailedDelivery(t *testing.T) {
	fs := &fakeSender{err: errors.New("upload failed"), done: make(chan struct{})}
	a := newTestApp(fs)
	a.SetActive(1, 42)
	cleaned := make(chan struct{})

	a.SendFiles("note", []sendpart.File{{Name: "report.txt", Reader: strings.NewReader("report")}}, nil, func() { close(cleaned) })

	<-fs.done
	<-cleaned
	msgs := a.store.Messages(42)
	if len(msgs) != 1 || !msgs[0].Failed || msgs[0].Pending {
		t.Fatalf("failed upload echo = %+v", msgs)
	}
}

func TestDeliverMarksFailedOnError(t *testing.T) {
	fs := &fakeSender{err: errors.New("boom")}
	a := newTestApp(fs)
	a.SetActive(1, 42)
	a.store.AppendMessage(store.Message{ChannelID: 42, Nonce: "n1", Pending: true, Content: "hi"})

	// Call deliver directly (synchronous) to avoid racing with Send's goroutine;
	// syncPoster then applies MarkFailed on this goroutine.
	a.deliver(42, api.SendMessageData{Content: "hi", Nonce: "n1"}, "n1")

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
	a.deliver(42, api.SendMessageData{Content: "hi", Nonce: "n1"}, "n1")

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

func TestSendStickerUsesNativeStickerIDs(t *testing.T) {
	fs := &fakeSender{done: make(chan struct{})}
	a := newTestApp(fs)
	a.SetActive(1, 42)
	a.SendSticker(99)
	<-fs.done
	fs.mu.Lock()
	defer fs.mu.Unlock()
	if len(fs.lastSend.StickerIDs) != 1 || uint64(fs.lastSend.StickerIDs[0]) != 99 {
		t.Fatalf("sticker_ids = %v, want [99]", fs.lastSend.StickerIDs)
	}
	if fs.lastSend.Content != "" {
		t.Fatalf("content = %q, want empty native sticker message", fs.lastSend.Content)
	}
}

func TestReplyNoMentionBuildsReferenceAndAllowedMentions(t *testing.T) {
	fs := &fakeSender{done: make(chan struct{})}
	a := newTestApp(fs)

	a.Reply("ack", store.Message{ID: 9, ChannelID: 42, Author: "alice"}, false)
	<-fs.done

	fs.mu.Lock()
	data := fs.lastSend
	fs.mu.Unlock()
	if data.Reference == nil || data.Reference.MessageID != 9 {
		t.Fatalf("reply reference = %+v, want message 9", data.Reference)
	}
	if data.AllowedMentions == nil || data.AllowedMentions.RepliedUser == nil || *data.AllowedMentions.RepliedUser {
		t.Fatalf("allowed mentions = %+v, want replied_user=false", data.AllowedMentions)
	}
}

func TestEditMessageCallsREST(t *testing.T) {
	fs := &fakeSender{done: make(chan struct{})}
	a := newTestApp(fs)

	a.EditMessage(42, 9, "edited")
	<-fs.done

	fs.mu.Lock()
	defer fs.mu.Unlock()
	if fs.edited != 1 || fs.editContent != "edited" {
		t.Fatalf("edit calls = %d content %q, want 1 edited", fs.edited, fs.editContent)
	}
}

func TestDeleteMessageWaitsForGatewayRemoval(t *testing.T) {
	fs := &fakeSender{done: make(chan struct{})}
	a := newTestApp(fs)
	a.store.AppendMessage(store.Message{ID: 9, ChannelID: 42, Content: "bye"})

	a.DeleteMessage(42, 9)
	<-fs.done
	if got := len(a.store.Messages(42)); got != 1 {
		t.Fatalf("messages after REST delete = %d, want still cached before gateway echo", got)
	}
	a.handleMessageDelete(&gateway.MessageDeleteEvent{ID: 9, ChannelID: 42})
	if got := len(a.store.Messages(42)); got != 0 {
		t.Fatalf("messages after gateway delete = %d, want 0", got)
	}
}

func TestAddReactionCallsREST(t *testing.T) {
	fs := &fakeSender{done: make(chan struct{})}
	a := newTestApp(fs)

	a.AddReaction(42, 9, "👍")
	<-fs.done

	fs.mu.Lock()
	defer fs.mu.Unlock()
	if fs.reacted != 1 || fs.reaction != "👍" {
		t.Fatalf("reaction calls = %d emoji = %q, want 1 thumbs-up", fs.reacted, fs.reaction)
	}
}

func TestSetPinnedPatchesCacheAfterSuccess(t *testing.T) {
	fs := &fakeSender{done: make(chan struct{})}
	a := newTestApp(fs)
	a.store.AppendMessage(store.Message{ID: 9, ChannelID: 42, Content: "pin me"})
	applied := make(chan struct{})
	a.OnChange(func() { close(applied) })

	a.SetPinned(42, 9, true)
	<-applied

	msgs := a.store.Messages(42)
	if len(msgs) != 1 || !msgs[0].Pinned {
		t.Fatalf("messages after pin = %+v, want pinned message", msgs)
	}
	fs.mu.Lock()
	defer fs.mu.Unlock()
	if fs.pinned != 1 {
		t.Fatalf("pin calls = %d, want 1", fs.pinned)
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
	if !ok || member.Username != "alice" || member.Nick != "ali" || len(member.RoleIDs) != 2 || member.RoleIDs[0] != 200 || member.RoleIDs[1] != 201 {
		t.Fatalf("loaded member identity/roles = %+v,%v, want alice, ali, 200,201", member, ok)
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

func TestReadyEventPreservesHydratedDMUserNames(t *testing.T) {
	a := newTestApp(&fakeSender{})
	a.store.UpsertGuild(store.Guild{ID: DirectMessagesGuildID, Name: "Direct Messages"})
	a.store.UpsertChannel(store.Channel{
		ID:         91,
		GuildID:    DirectMessagesGuildID,
		Kind:       store.ChannelDM,
		Name:       "alice",
		Recipients: []store.Member{{ID: 100, Name: "alice"}},
	})

	a.handleReady(&gateway.ReadyEvent{PrivateChannels: []discord.Channel{{
		ID:   91,
		Type: discord.DirectMessage,
	}}})

	if name, ok := a.store.ChannelName(91); !ok || name != "alice" {
		t.Fatalf("preserved DM ChannelName = %q,%v, want alice,true", name, ok)
	}
	channel, ok := a.store.Channel(91)
	if !ok || len(channel.Recipients) != 1 || channel.Recipients[0].ID != 100 {
		t.Fatalf("preserved DM recipients = %+v,%v, want alice (100)", channel.Recipients, ok)
	}
}

func TestReadyEventPreservesNamedGroupDMRecipients(t *testing.T) {
	a := newTestApp(&fakeSender{})
	a.store.UpsertGuild(store.Guild{ID: DirectMessagesGuildID, Name: "Direct Messages"})
	a.store.UpsertChannel(store.Channel{
		ID:      92,
		GuildID: DirectMessagesGuildID,
		Kind:    store.ChannelDM,
		Name:    "The Crew",
		Recipients: []store.Member{
			{ID: 101, Name: "bob"},
			{ID: 102, Name: "carol"},
		},
	})

	// A named group DM that arrives without recipients must keep the members it
	// was already hydrated with, otherwise its @ mention menu goes empty.
	a.handleReady(&gateway.ReadyEvent{PrivateChannels: []discord.Channel{{
		ID:   92,
		Type: discord.GroupDM,
		Name: "The Crew",
	}}})

	channel, ok := a.store.Channel(92)
	if !ok || len(channel.Recipients) != 2 {
		t.Fatalf("preserved group DM recipients = %+v,%v, want bob and carol", channel.Recipients, ok)
	}
	if channel.Recipients[0].ID != 101 || channel.Recipients[1].ID != 102 {
		t.Fatalf("preserved group DM recipients = %+v, want bob (101), carol (102)", channel.Recipients)
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

func TestMessageCreateTracksOnlyPingsForPriority(t *testing.T) {
	a := newTestApp(&fakeSender{})
	a.selfID = 7
	a.store.UpsertGuild(store.Guild{ID: 1, Name: "guild"})
	a.store.UpsertChannel(store.Channel{ID: 10, GuildID: 1, Name: "general", Kind: store.ChannelText})
	a.store.UpsertChannel(store.Channel{ID: 11, GuildID: DirectMessagesGuildID, Name: "alice", Kind: store.ChannelDM})

	// Ordinary unread messages do not get priority.
	a.handleMessageCreate(&gateway.MessageCreateEvent{Message: discord.Message{ID: 1, GuildID: 1, ChannelID: 10, Author: discord.User{ID: 8}, Content: "hello"}})
	if got := a.store.Pings(10); got != 0 {
		t.Fatalf("ordinary message pings = %d, want 0", got)
	}

	// A direct mention and a DM both receive priority.
	a.handleMessageCreate(&gateway.MessageCreateEvent{Message: discord.Message{ID: 2, GuildID: 1, ChannelID: 10, Author: discord.User{ID: 8}, Mentions: []discord.GuildUser{{User: discord.User{ID: 7}}}}})
	a.handleMessageCreate(&gateway.MessageCreateEvent{Message: discord.Message{ID: 3, ChannelID: 11, Author: discord.User{ID: 8}, Content: "dm"}})
	if got := a.store.Pings(10); got != 1 {
		t.Errorf("mention pings = %d, want 1", got)
	}
	if got := a.store.Pings(11); got != 1 {
		t.Errorf("DM pings = %d, want 1", got)
	}
	if got := a.store.GuildPings(1); got != 1 {
		t.Errorf("guild pings = %d, want 1", got)
	}

	// Gateway echoes without a nonce are still authored by us and must not
	// create unread or ping state for an inactive channel.
	a.handleMessageCreate(&gateway.MessageCreateEvent{Message: discord.Message{ID: 4, GuildID: 1, ChannelID: 10, Author: discord.User{ID: 7}, Mentions: []discord.GuildUser{{User: discord.User{ID: 7}}}}})
	if got := a.store.Pings(10); got != 1 {
		t.Errorf("self message pings = %d, want 1", got)
	}

	// Selecting a channel clears both its ordinary unread and ping badge.
	a.SetActive(1, 10)
	if got := a.store.Pings(10); got != 0 {
		t.Errorf("active channel pings = %d, want 0", got)
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

func TestLoadHistoryPreservesConfirmedGatewayMessagesArrivingInFlight(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	fs := &fakeSender{
		history: []discord.Message{
			{ID: 102, ChannelID: 42, Content: "REST newest"},
			{ID: 101, ChannelID: 42, Content: "REST oldest"},
		},
		historyDone: started, historyRelease: release,
	}
	a := newTestApp(fs)
	loaded := make(chan struct{})
	var loadedOnce sync.Once
	a.OnChange(func() {
		messages := a.store.Messages(42)
		if len(messages) == 3 {
			loadedOnce.Do(func() { close(loaded) })
		}
	})

	a.LoadHistory(42, 50)
	<-started
	a.handleMessageCreate(&gateway.MessageCreateEvent{Message: discord.Message{
		ID: 103, ChannelID: 42, Content: "live gateway message",
	}})
	close(release)
	<-loaded

	got := messageStoreIDs(a.store.Messages(42))
	want := []store.MessageID{101, 102, 103}
	if len(got) != len(want) {
		t.Fatalf("history IDs = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("history IDs = %v, want %v", got, want)
		}
	}
}

func TestLoadHistoryPreservesInFlightEditAndDelete(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	fs := &fakeSender{
		history: []discord.Message{
			{ID: 102, ChannelID: 42, Content: "stale deleted copy"},
			{ID: 101, ChannelID: 42, Content: "stale pre-edit copy"},
		},
		historyDone: started, historyRelease: release,
	}
	ui := newChannelPoster()
	a := newTestApp(fs)
	a.ui = ui
	a.store.UpsertChannel(store.Channel{ID: 42, GuildID: 1, Name: "general"})
	a.store.SetMessages(42, []store.Message{
		{ID: 101, Content: "before edit"},
		{ID: 102, Content: "before delete"},
	})

	a.LoadHistory(42, 50)
	<-started
	a.handleMessageUpdate(&gateway.MessageUpdateEvent{Message: discord.Message{
		ID: 101, ChannelID: 42, Content: "edited while loading",
	}})
	ui.runNext(t)
	a.handleMessageDelete(&gateway.MessageDeleteEvent{ID: 102, ChannelID: 42})
	ui.runNext(t)
	close(release)
	ui.runNext(t)

	got := a.store.Messages(42)
	if len(got) != 1 || got[0].ID != 101 || got[0].Content != "edited while loading" {
		t.Fatalf("history = %+v, want only preserved edited message 101", got)
	}
}

func TestLoadHistoryCompletionDoesNotRecreateDeletedChannelRing(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	fs := &fakeSender{
		history:        []discord.Message{{ID: 101, ChannelID: 42, Content: "stale"}},
		historyDone:    started,
		historyRelease: release,
	}
	ui := newChannelPoster()
	a := newTestApp(fs)
	a.ui = ui
	a.store.UpsertChannel(store.Channel{ID: 42, GuildID: 1, Name: "general"})

	a.LoadHistory(42, 50)
	<-started
	a.handleChannelDelete(&gateway.ChannelDeleteEvent{Channel: discord.Channel{ID: 42, GuildID: 1}})
	ui.runNext(t)
	close(release)
	ui.runNext(t)

	if _, ok := a.store.Channel(42); ok {
		t.Fatal("deleted channel was recreated by history completion")
	}
	if got := a.store.Messages(42); got != nil {
		t.Fatalf("deleted channel history = %+v, want nil", got)
	}
}

func messageStoreIDs(messages []store.Message) []store.MessageID {
	ids := make([]store.MessageID, len(messages))
	for i, message := range messages {
		ids[i] = message.ID
	}
	return ids
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
		history: []discord.Message{{ID: 1, ChannelID: 42, Content: "hi"}},
	}
	a := newTestApp(fs)
	loaded := make(chan struct{})
	a.OnChange(func() { close(loaded) })

	a.LoadHistory(42, 50)
	<-loaded
	a.LoadHistory(42, 50)

	fs.mu.Lock()
	defer fs.mu.Unlock()
	if fs.historyN != 1 {
		t.Fatalf("history API calls = %d, want 1", fs.historyN)
	}
}

func TestLoadOlderHistoryPrependsMessagesBeforeOldest(t *testing.T) {
	fs := &fakeSender{historyBefore: []discord.Message{
		{ID: 8, ChannelID: 42, Author: discord.User{Username: "old"}, Content: "older"},
		{ID: 7, ChannelID: 42, Author: discord.User{Username: "oldest"}, Content: "oldest"},
	}}
	a := newTestApp(fs)
	a.store.SetMessages(42, []store.Message{
		{ID: 9, ChannelID: 42, Author: "newer", Content: "newer"},
		{ID: 10, ChannelID: 42, Author: "newest", Content: "newest"},
	})

	a.loadOlderHistory(42, 9, 50)

	msgs := a.store.Messages(42)
	if len(msgs) != 4 || msgs[0].ID != 7 || msgs[1].ID != 8 || msgs[2].ID != 9 || msgs[3].ID != 10 {
		t.Fatalf("history = %+v, want IDs 7,8,9,10", msgs)
	}
}

func TestLoadOlderHistoryAtCapacityPreservesLiveArrivalAndAdvancesWindow(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	fs := &fakeSender{
		historyBefore: []discord.Message{
			{ID: 4, ChannelID: 42}, {ID: 3, ChannelID: 42},
			{ID: 2, ChannelID: 42}, {ID: 1, ChannelID: 42},
		},
		historyBeforeDone: started, historyBeforeRelease: release,
	}
	ui := newChannelPoster()
	a := newTestApp(fs)
	a.ui = ui
	a.store = store.New(4)
	a.store.UpsertChannel(store.Channel{ID: 42, GuildID: 1, Name: "general"})
	a.store.SetMessages(42, []store.Message{{ID: 5}, {ID: 6}, {ID: 7}, {ID: 8}})

	a.LoadOlderHistory(42)
	<-started
	a.handleMessageCreate(&gateway.MessageCreateEvent{Message: discord.Message{
		ID: 9, ChannelID: 42, Content: "live while loading older history",
	}})
	ui.runNext(t)
	close(release)
	ui.runNext(t)

	got := messageStoreIDs(a.store.Messages(42))
	want := []store.MessageID{1, 7, 8, 9}
	if !slices.Equal(got, want) {
		t.Fatalf("history IDs = %v, want %v (older progress plus live tail)", got, want)
	}
}

func TestLoadRolesStoresRolesAndUsesSessionCache(t *testing.T) {
	fs := &fakeSender{
		roles: []discord.Role{
			{ID: 200, Name: "admin", Position: 10},
			{ID: 201, Name: "member", Position: 1},
		},
	}
	a := newTestApp(fs)
	loaded := make(chan struct{})
	a.OnChange(func() { close(loaded) })

	a.LoadRoles(1)
	<-loaded
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
	}
	a := newTestApp(fs)
	ready := false
	readyDone := make(chan struct{})
	a.OnReady(func() {
		ready = true
		close(readyDone)
	})

	a.LoadGuilds(100)
	<-readyDone
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
		channels: []discord.Channel{{ID: 10, Type: discord.GuildText, Name: "general"}},
	}
	a := newTestApp(fs)
	loaded := make(chan struct{})
	a.OnChange(func() { close(loaded) })

	a.LoadChannels(1)
	<-loaded
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

func TestLoadForumMetadataLoadsAvailableTagsFromChannelDetail(t *testing.T) {
	fs := &fakeSender{channelDetail: &discord.Channel{
		ID: 42, Type: discord.GuildForum, AvailableTags: []discord.Tag{{ID: 9, Name: "bug"}},
	}}
	a := newTestApp(fs)
	a.Store().UpsertChannel(store.Channel{ID: 42, GuildID: 7, Kind: store.ChannelForum})
	loaded := make(chan struct{})
	a.OnChange(func() { close(loaded) })
	a.LoadForumMetadata(42)
	select {
	case <-loaded:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for forum metadata")
	}
	forum, ok := a.Store().Channel(42)
	if !ok || forum.Forum == nil || len(forum.Forum.Tags) != 1 || forum.Forum.Tags[0].Name != "bug" {
		t.Fatalf("forum metadata = %+v, want one bug tag", forum)
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
