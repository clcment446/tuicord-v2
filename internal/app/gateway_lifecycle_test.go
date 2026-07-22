package app

import (
	"testing"

	"awesomeProject/internal/store"

	"github.com/diamondburned/arikawa/v3/discord"
	"github.com/diamondburned/arikawa/v3/gateway"
)

type queuedPoster struct {
	posts []func()
}

func (p *queuedPoster) Post(fn func()) { p.posts = append(p.posts, fn) }
func (*queuedPoster) WriteRaw([]byte)  {}
func (*queuedPoster) Invalidate()      {}
func (*queuedPoster) ForceRepaint()    {}
func (p *queuedPoster) run() {
	for len(p.posts) > 0 {
		fn := p.posts[0]
		p.posts = p.posts[1:]
		fn()
	}
}

func TestGatewaySelfIDReadsFollowPostedReadyMutation(t *testing.T) {
	ui := &queuedPoster{}
	a := &App{store: store.New(0), ui: ui}
	a.store.AppendMessage(store.Message{ID: 1, ChannelID: 10})
	a.store.UpsertChannel(store.Channel{
		ID: 20, GuildID: 1, Kind: store.ChannelThread, Thread: &store.ThreadMeta{},
	})

	// Both gateway handlers are received before READY's posted closure runs.
	// Their self checks must happen later, in UI queue order.
	a.handleReady(&gateway.ReadyEvent{User: discord.User{ID: 42}})
	a.handleReactionAdd(&gateway.MessageReactionAddEvent{
		UserID: 42, ChannelID: 10, MessageID: 1, Emoji: discord.Emoji{Name: "ok"},
	})
	a.handleThreadMembersUpdate(&gateway.ThreadMembersUpdateEvent{
		ID: 20, AddedMembers: []discord.ThreadMember{{UserID: 42}},
	})
	ui.run()

	message := a.store.Messages(10)[0]
	if len(message.Reactions) != 1 || !message.Reactions[0].Me {
		t.Fatalf("reaction after queued READY = %+v, want Me=true", message.Reactions)
	}
	thread, _ := a.store.Channel(20)
	if thread.Thread == nil || !thread.Thread.Joined {
		t.Fatalf("thread after queued READY = %+v, want Joined=true", thread.Thread)
	}
}

func TestChannelLifecycleUpdatesStoreAndRepairsActiveSelection(t *testing.T) {
	a := newTestApp(&fakeSender{})
	a.store.UpsertGuild(store.Guild{ID: 1, Name: "guild"})
	changes := 0
	a.OnChange(func() { changes++ })

	a.handleChannelUpsert(discord.Channel{ID: 10, GuildID: 1, Type: discord.GuildText, Name: "general", Position: 2})
	channel, ok := a.store.Channel(10)
	if !ok || channel.Name != "general" || channel.GuildID != 1 {
		t.Fatalf("created channel = %+v, %t", channel, ok)
	}

	a.handleChannelUpsert(discord.Channel{ID: 10, GuildID: 1, Type: discord.GuildText, Name: "renamed", Position: 1})
	channel, _ = a.store.Channel(10)
	if channel.Name != "renamed" || channel.Position != 1 {
		t.Fatalf("updated channel = %+v", channel)
	}

	a.store.AppendMessage(store.Message{ID: 1, ChannelID: 10})
	a.SetActive(1, 10)
	a.handleChannelDelete(&gateway.ChannelDeleteEvent{Channel: discord.Channel{ID: 10, GuildID: 1}})
	if _, ok := a.store.Channel(10); ok || a.store.Messages(10) != nil {
		t.Fatal("deleted channel or its history remains cached")
	}
	if a.ActiveGuild() != 1 || a.ActiveChannel() != 0 {
		t.Fatalf("active selection = %d/%d, want 1/0", a.ActiveGuild(), a.ActiveChannel())
	}
	if changes != 3 {
		t.Fatalf("OnChange calls = %d, want 3", changes)
	}
}

func TestGuildLifecycleUnavailableThenDelete(t *testing.T) {
	a := newTestApp(&fakeSender{})
	a.store.UpsertGuild(store.Guild{ID: 1, Name: "old"})
	a.store.UpsertChannel(store.Channel{ID: 10, GuildID: 1})
	a.store.AppendMessage(store.Message{ID: 1, ChannelID: 10})
	a.SetActive(1, 10)
	guildChanges, changes := 0, 0
	a.OnGuildChange(func() { guildChanges++ })
	a.OnChange(func() { changes++ })

	a.handleGuildUpdate(&gateway.GuildUpdateEvent{Guild: discord.Guild{ID: 1, Name: "renamed"}})
	guild, ok := a.store.Guild(1)
	if !ok || guild.Name != "renamed" || guild.Unavailable {
		t.Fatalf("updated guild = %+v, %t", guild, ok)
	}

	a.handleGuildDelete(&gateway.GuildDeleteEvent{ID: 1, Unavailable: true})
	guild, ok = a.store.Guild(1)
	if !ok || !guild.Unavailable || len(a.store.Channels(1)) != 1 || a.ActiveChannel() != 10 {
		t.Fatalf("temporary outage discarded state: guild=%+v channels=%v active=%d", guild, a.store.Channels(1), a.ActiveChannel())
	}

	a.handleGuildDelete(&gateway.GuildDeleteEvent{ID: 1})
	if _, ok := a.store.Guild(1); ok || len(a.store.Channels(1)) != 0 || a.store.Messages(10) != nil {
		t.Fatal("permanent guild delete did not cascade")
	}
	if a.ActiveGuild() != 0 || a.ActiveChannel() != 0 {
		t.Fatalf("active selection = %d/%d, want 0/0", a.ActiveGuild(), a.ActiveChannel())
	}
	if guildChanges != 3 || changes != 3 {
		t.Fatalf("callbacks guild=%d change=%d, want 3/3", guildChanges, changes)
	}
}

func TestGuildCreateRestoresUnavailableGuild(t *testing.T) {
	a := newTestApp(&fakeSender{})
	a.store.UpsertGuild(store.Guild{ID: 1, Name: "guild", Unavailable: true})
	guildChanges, genericChanges := 0, 0
	a.OnGuildChange(func() { guildChanges++ })
	a.OnChange(func() { genericChanges++ })

	a.handleGuildCreate(&gateway.GuildCreateEvent{
		Guild:    discord.Guild{ID: 1, Name: "guild"},
		Channels: []discord.Channel{{ID: 10, Type: discord.GuildText, Name: "general"}},
	})

	guild, ok := a.store.Guild(1)
	if !ok || guild.Unavailable || len(a.store.Channels(1)) != 1 {
		t.Fatalf("restored guild = %+v, channels=%v", guild, a.store.Channels(1))
	}
	if guildChanges != 1 || genericChanges != 0 {
		t.Fatalf("guild/generic callbacks = %d/%d, want 1/0", guildChanges, genericChanges)
	}
}
