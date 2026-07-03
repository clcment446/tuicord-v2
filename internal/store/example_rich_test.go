package store_test

import (
	"fmt"

	"awesomeProject/internal/store"
)

// UpdateMessage patches a message that is already in the ring — the primary
// use-case is Discord's asynchronous embed delivery.
func ExampleStore_UpdateMessage() {
	s := store.New(0)
	s.AppendMessage(store.Message{ID: 1, ChannelID: 10, Content: "https://tenor.com/abc"})

	// Discord sends MESSAGE_UPDATE with embed data after unfurling the URL.
	s.UpdateMessage(10, 1, func(m *store.Message) {
		m.Embeds = []store.Embed{{Kind: store.EmbedGIFV, VideoURL: "https://media.tenor.com/abc.mp4"}}
	})

	msgs := s.Messages(10)
	fmt.Println("embeds:", len(msgs[0].Embeds))
	fmt.Println("kind gifv:", msgs[0].Embeds[0].Kind == store.EmbedGIFV)
	// Output:
	// embeds: 1
	// kind gifv: true
}

// AddReaction and RemoveReaction maintain the live reaction list on a message.
func ExampleStore_AddReaction() {
	s := store.New(0)
	s.AppendMessage(store.Message{ID: 1, ChannelID: 10})

	s.AddReaction(10, 1, store.Reaction{EmojiName: "👍", Count: 1})
	s.AddReaction(10, 1, store.Reaction{EmojiName: "👍", Count: 1, Me: true})

	rxs := s.Messages(10)[0].Reactions
	fmt.Printf("👍 count=%d me=%v\n", rxs[0].Count, rxs[0].Me)
	// Output:
	// 👍 count=2 me=true
}

// RemoveReaction decrements a reaction and removes the entry when count
// reaches zero.
func ExampleStore_RemoveReaction() {
	s := store.New(0)
	s.AppendMessage(store.Message{
		ID:        1,
		ChannelID: 10,
		Reactions: []store.Reaction{{EmojiName: "❤️", Count: 2, Me: true}},
	})

	s.RemoveReaction(10, 1, "❤️", 0, true) // current user un-reacted

	rxs := s.Messages(10)[0].Reactions
	fmt.Printf("count=%d me=%v\n", rxs[0].Count, rxs[0].Me)
	// Output:
	// count=1 me=false
}

// MemberColor follows Discord's rule: the highest-position colored role wins.
func ExampleStore_MemberColor() {
	s := store.New(0)
	s.UpsertMember(1, store.Member{ID: 42, RoleIDs: []store.RoleID{100, 200}})
	s.UpsertRole(1, store.Role{ID: 100, Name: "member", Position: 1, Color: 0x808080})
	s.UpsertRole(1, store.Role{ID: 200, Name: "mod", Position: 10, Color: 0x00BFFF})

	color := s.MemberColor(1, 42)
	fmt.Printf("0x%06X\n", color)
	// Output:
	// 0x00BFFF
}

// LerpColor blends two 0xRRGGBB values.
func ExampleLerpColor() {
	black := uint32(0x000000)
	white := uint32(0xFFFFFF)

	fmt.Printf("0x%06X\n", store.LerpColor(black, white, 0.0))
	fmt.Printf("0x%06X\n", store.LerpColor(black, white, 0.5))
	fmt.Printf("0x%06X\n", store.LerpColor(black, white, 1.0))
	// Output:
	// 0x000000
	// 0x808080
	// 0xFFFFFF
}

// GradientAt returns the role color at position t along the name.
func ExampleRole_GradientAt() {
	// Two-stop gradient: red → blue
	r := store.Role{Colors: [3]uint32{0xFF0000, 0x0000FF, 0}}

	fmt.Printf("t=0:   0x%06X\n", r.GradientAt(0))
	fmt.Printf("t=0.5: 0x%06X\n", r.GradientAt(0.5))
	fmt.Printf("t=1:   0x%06X\n", r.GradientAt(1))
	// Output:
	// t=0:   0xFF0000
	// t=0.5: 0x800080
	// t=1:   0x0000FF
}
