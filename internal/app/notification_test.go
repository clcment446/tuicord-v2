package app

import (
	"testing"

	"awesomeProject/internal/store"

	"github.com/diamondburned/arikawa/v3/discord"
	"github.com/diamondburned/arikawa/v3/gateway"
)

func TestMessageCreateEmitsIncomingMessageOnlyForNewRemotePings(t *testing.T) {
	a := newTestApp(&fakeSender{})
	a.selfID = 7
	var got []store.Message
	a.OnIncomingMessage(func(message store.Message) { got = append(got, message) })

	a.handleMessageCreate(&gateway.MessageCreateEvent{Message: discord.Message{
		ID: 1, ChannelID: 10, Author: discord.User{ID: 8}, Content: "hello",
	}})
	a.handleMessageCreate(&gateway.MessageCreateEvent{Message: discord.Message{
		ID: 2, ChannelID: 10, Author: discord.User{ID: 8}, Content: "ping",
		Mentions: []discord.GuildUser{{User: discord.User{ID: 7}}},
	}})
	a.handleMessageCreate(&gateway.MessageCreateEvent{Message: discord.Message{
		ID: 3, ChannelID: 10, Author: discord.User{ID: 7}, Content: "mine",
	}})

	if len(got) != 1 || got[0].ID != 2 {
		t.Fatalf("incoming messages = %+v, want remote ping only", got)
	}
}
