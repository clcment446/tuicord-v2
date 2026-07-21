package app

import (
	"testing"
	"time"

	"awesomeProject/internal/store"

	"github.com/diamondburned/arikawa/v3/discord"
)

func TestConvertMessageMapsReply(t *testing.T) {
	msg := discord.Message{
		ID:        7,
		ChannelID: 3,
		Type:      discord.InlinedReplyMessage,
		Author:    discord.User{ID: 42, Username: "alice"},
		Content:   "sure!",
		Reference: &discord.MessageReference{Type: discord.MessageReferenceTypeDefault, MessageID: 5, ChannelID: 3},
		ReferencedMessage: &discord.Message{
			ID:        5,
			ChannelID: 3,
			Author:    discord.User{ID: 99, Username: "bob"},
			Content:   "can you?",
		},
	}
	got := convertMessage(msg)
	if got.Reply == nil {
		t.Fatal("Reply = nil, want populated reference summary")
	}
	if got.Reply.MessageID != 5 || got.Reply.AuthorID != 99 || got.Reply.Author != "bob" || got.Reply.Content != "can you?" || got.Reply.Deleted {
		t.Errorf("Reply = %+v", got.Reply)
	}
}

func TestConvertMessageMarksDeletedReplyTarget(t *testing.T) {
	msg := discord.Message{
		ID:        7,
		ChannelID: 3,
		Type:      discord.InlinedReplyMessage,
		Author:    discord.User{ID: 42, Username: "alice"},
		Content:   "sure!",
		Reference: &discord.MessageReference{Type: discord.MessageReferenceTypeDefault, MessageID: 5, ChannelID: 3},
	}
	got := convertMessage(msg)
	if got.Reply == nil || !got.Reply.Deleted || got.Reply.MessageID != 5 {
		t.Fatalf("Reply = %+v, want deleted marker for message 5", got.Reply)
	}
}

func TestConvertMessageIgnoresCrosspostReference(t *testing.T) {
	msg := discord.Message{
		ID:        7,
		ChannelID: 3,
		Author:    discord.User{ID: 42, Username: "alice"},
		Content:   "announcement",
		Reference: &discord.MessageReference{Type: discord.MessageReferenceTypeDefault, MessageID: 5, ChannelID: 9},
	}
	if got := convertMessage(msg); got.Reply != nil {
		t.Fatalf("Reply = %+v, want nil for a non-reply reference", got.Reply)
	}
}

func TestConvertMessageMapsForwardSnapshots(t *testing.T) {
	sent := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)
	msg := discord.Message{
		ID:        7,
		ChannelID: 3,
		Author:    discord.User{ID: 42, Username: "alice"},
		Reference: &discord.MessageReference{Type: discord.MessageReferenceTypeForward, MessageID: 5, ChannelID: 9},
		MessageSnapshots: []discord.MessageSnapshot{{
			Message: discord.MessageSnapshotMessage{
				Content:   "forwarded text",
				Timestamp: discord.Timestamp(sent),
				Attachments: []discord.Attachment{{
					Filename: "cat.png", ContentType: "image/png",
					URL: "https://cdn/cat.png", Proxy: "https://proxy/cat.png",
				}},
				Embeds: []discord.Embed{{Title: "t", Description: "d"}},
			},
		}},
	}
	got := convertMessage(msg)
	if len(got.Forwards) != 1 {
		t.Fatalf("Forwards = %+v, want one snapshot", got.Forwards)
	}
	f := got.Forwards[0]
	if f.Content != "forwarded text" || !f.Timestamp.Equal(sent) {
		t.Errorf("snapshot = %+v", f)
	}
	if len(f.Attachments) != 1 || f.Attachments[0].ProxyURL != "https://proxy/cat.png" {
		t.Errorf("snapshot attachments = %+v", f.Attachments)
	}
	if len(f.Embeds) != 1 || f.Embeds[0].Title != "t" {
		t.Errorf("snapshot embeds = %+v", f.Embeds)
	}
	if got.Reply != nil {
		t.Errorf("Reply = %+v, want nil for a forward", got.Reply)
	}
}

func TestMessageUpdateKeepsReplyAndForwards(t *testing.T) {
	st := store.New(0)
	st.AppendMessage(store.Message{
		ID: 7, ChannelID: 3, Author: "alice",
		Reply:    &store.MessageReply{MessageID: 5, Author: "bob", Content: "hi"},
		Forwards: []store.ForwardedMessage{{Content: "fwd"}},
	})
	// A partial gateway patch (embed unfurl) carries neither reference field.
	st.UpdateMessage(3, 7, func(m *store.Message) {})
	msgs := st.Messages(3)
	if len(msgs) != 1 || msgs[0].Reply == nil || msgs[0].Reply.Author != "bob" || len(msgs[0].Forwards) != 1 {
		t.Fatalf("message after update = %+v", msgs)
	}
}
