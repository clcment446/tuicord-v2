package store_test

import (
	"fmt"

	"awesomeProject/internal/store"
)

// A store keeps guilds, their channels, and a bounded message history.
func ExampleStore() {
	s := store.New(0)
	s.UpsertGuild(store.Guild{ID: 1, Name: "gophers"})
	s.UpsertChannel(store.Channel{ID: 10, GuildID: 1, Name: "general"})
	s.AppendMessage(store.Message{ChannelID: 10, Author: "alice", Content: "hi"})

	for _, g := range s.Guilds() {
		fmt.Println("guild:", g.Name)
		for _, c := range s.Channels(g.ID) {
			fmt.Println("  #" + c.Name)
			for _, m := range s.Messages(c.ID) {
				fmt.Printf("    %s: %s\n", m.Author, m.Content)
			}
		}
	}
	// Output:
	// guild: gophers
	//   #general
	//     alice: hi
}
