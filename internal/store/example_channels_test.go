package store_test

import (
	"fmt"
	"time"

	"awesomeProject/internal/store"
)

// Threads are channels parented to a channel. Store.Threads lists the active
// ones under a parent, most-recent-activity first; archiving moves a thread out
// of that list without deleting it.
func ExampleStore_Threads() {
	s := store.New(0)
	s.UpsertChannel(store.Channel{ID: 100, GuildID: 1, Name: "general", Kind: store.ChannelText})

	base := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	s.UpsertThread(store.Channel{ID: 10, GuildID: 1, Name: "old-chat", ParentID: 100,
		Thread: &store.ThreadMeta{LastActive: base}})
	s.UpsertThread(store.Channel{ID: 11, GuildID: 1, Name: "hot-take", ParentID: 100,
		Thread: &store.ThreadMeta{LastActive: base.Add(time.Hour)}})

	for _, t := range s.Threads(100) {
		fmt.Println(t.Name)
	}

	s.SetArchived(11, true)
	fmt.Println("active after archive:", len(s.Threads(100)))
	// Output:
	// hot-take
	// old-chat
	// active after archive: 1
}

// ChannelPermissions layers a channel's overwrites on top of the guild
// baseline: a SEND_MESSAGES deny on @everyone makes the channel read-only.
func ExampleStore_ChannelPermissions() {
	s := store.New(0)
	s.UpsertGuild(store.Guild{ID: 1})
	s.UpsertRole(1, store.Role{ID: 1, Permissions: store.PermViewChannel | store.PermSendMessages})
	s.UpsertMember(1, store.Member{ID: 42, Name: "reader"})
	s.UpsertChannel(store.Channel{ID: 5, GuildID: 1, Name: "rules", Kind: store.ChannelText,
		Overwrites: []store.PermissionOverwrite{
			{ID: 1, Role: true, Deny: store.PermSendMessages},
		}})

	fmt.Println("can send:", s.ChannelCan(1, 42, 5, store.PermSendMessages))
	fmt.Println("can view:", s.ChannelCan(1, 42, 5, store.PermViewChannel))
	// Output:
	// can send: false
	// can view: true
}
