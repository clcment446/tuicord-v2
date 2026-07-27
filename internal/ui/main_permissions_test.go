package ui

import (
	"testing"

	"awesomeProject/internal/app"
	"awesomeProject/internal/discord"
	"awesomeProject/internal/store"
	"awesomeProject/internal/tui/tui"
	"github.com/diamondburned/arikawa/v3/session"
)

func TestChannelReadOnlyAllowsForumPostWithSendMessagesInThreads(t *testing.T) {
	const (
		guildID = store.GuildID(1)
		forumID = store.ChannelID(10)
		postID  = store.ChannelID(11)
	)

	st := store.New(0)
	st.UpsertGuild(store.Guild{ID: guildID, Name: "Guild"})
	st.UpsertRole(guildID, store.Role{
		ID:          store.RoleID(guildID),
		Name:        "@everyone",
		Permissions: store.PermViewChannel | store.PermSendMessagesInThreads,
	})
	st.UpsertMember(guildID, store.Member{ID: 0, Name: "member"})
	st.UpsertChannel(store.Channel{ID: forumID, GuildID: guildID, Kind: store.ChannelForum})
	st.UpsertThread(store.Channel{
		ID:       postID,
		GuildID:  guildID,
		ParentID: forumID,
		Kind:     store.ChannelThread,
		Thread:   &store.ThreadMeta{},
	})

	logic := app.New(discord.WrapSession(session.New("")), st, tui.New())
	mv := &MainView{app: logic}

	if mv.channelReadOnly(postID) {
		t.Fatal("forum post with SEND_MESSAGES_IN_THREADS is read-only")
	}
}
