package store

import (
	"testing"
)

// ── UpdateMessage ────────────────────────────────────────────────────────────

func TestUpdateMessage_PatchesEmbeds(t *testing.T) {
	// Arrange
	s := New(0)
	s.AppendMessage(Message{ID: 1, ChannelID: 5, Content: "hello"})

	// Act
	ok := s.UpdateMessage(5, 1, func(m *Message) {
		m.Embeds = []Embed{{Kind: EmbedGIFV, URL: "https://tenor.com/x"}}
	})

	// Assert
	if !ok {
		t.Fatal("UpdateMessage returned false for known message")
	}
	msgs := s.Messages(5)
	if len(msgs[0].Embeds) != 1 || msgs[0].Embeds[0].URL != "https://tenor.com/x" {
		t.Errorf("embeds not patched: %+v", msgs[0].Embeds)
	}
}

func TestUpdateMessage_ReturnsFalseForUnknownMessage(t *testing.T) {
	// Arrange
	s := New(0)
	s.AppendMessage(Message{ID: 1, ChannelID: 5})

	// Act / Assert
	if s.UpdateMessage(5, 999, func(m *Message) { m.Content = "oops" }) {
		t.Error("expected false for unknown message ID")
	}
}

func TestUpdateMessage_ReturnsFalseForUnknownChannel(t *testing.T) {
	s := New(0)
	if s.UpdateMessage(99, 1, func(m *Message) {}) {
		t.Error("expected false for unknown channel")
	}
}

func TestUpdateMessage_PatchesContent(t *testing.T) {
	s := New(0)
	s.AppendMessage(Message{ID: 10, ChannelID: 1, Content: "original"})

	s.UpdateMessage(1, 10, func(m *Message) { m.Content = "edited" })

	if got := s.Messages(1)[0].Content; got != "edited" {
		t.Errorf("content = %q, want %q", got, "edited")
	}
}

// ── AddReaction ──────────────────────────────────────────────────────────────

func TestAddReaction_AppendsNewEmoji(t *testing.T) {
	// Arrange
	s := New(0)
	s.AppendMessage(Message{ID: 1, ChannelID: 5})

	// Act
	ok := s.AddReaction(5, 1, Reaction{EmojiName: "👍", Count: 1})

	// Assert
	if !ok {
		t.Fatal("AddReaction returned false")
	}
	rxs := s.Messages(5)[0].Reactions
	if len(rxs) != 1 || rxs[0].EmojiName != "👍" || rxs[0].Count != 1 {
		t.Errorf("unexpected reactions: %+v", rxs)
	}
}

func TestAddReaction_IncrementsExistingEntry(t *testing.T) {
	// Arrange
	s := New(0)
	s.AppendMessage(Message{ID: 1, ChannelID: 5, Reactions: []Reaction{{EmojiName: "👍", Count: 2}}})

	// Act
	s.AddReaction(5, 1, Reaction{EmojiName: "👍", Count: 1})

	// Assert
	rxs := s.Messages(5)[0].Reactions
	if len(rxs) != 1 || rxs[0].Count != 3 {
		t.Errorf("count = %d, want 3", rxs[0].Count)
	}
}

func TestAddReaction_SetsMe(t *testing.T) {
	s := New(0)
	s.AppendMessage(Message{ID: 1, ChannelID: 5, Reactions: []Reaction{{EmojiName: "❤️", Count: 1, Me: false}}})

	s.AddReaction(5, 1, Reaction{EmojiName: "❤️", Me: true})

	if !s.Messages(5)[0].Reactions[0].Me {
		t.Error("Me not set after AddReaction with Me=true")
	}
}

func TestAddReaction_DistinguishesByEmojiID(t *testing.T) {
	s := New(0)
	s.AppendMessage(Message{ID: 1, ChannelID: 5, Reactions: []Reaction{{EmojiName: "pepe", EmojiID: 111, Count: 1}}})

	s.AddReaction(5, 1, Reaction{EmojiName: "pepe", EmojiID: 222, Count: 1})

	rxs := s.Messages(5)[0].Reactions
	if len(rxs) != 2 {
		t.Errorf("expected 2 distinct custom emoji reactions, got %d", len(rxs))
	}
}

func TestAddReaction_ReturnsFalseForUnknownMessage(t *testing.T) {
	s := New(0)
	s.AppendMessage(Message{ID: 1, ChannelID: 5})
	if s.AddReaction(5, 999, Reaction{EmojiName: "👍"}) {
		t.Error("expected false for unknown message")
	}
}

func TestAddReaction_ReturnsFalseForUnknownChannel(t *testing.T) {
	s := New(0)
	if s.AddReaction(99, 1, Reaction{EmojiName: "👍"}) {
		t.Error("expected false for unknown channel")
	}
}

// ── RemoveReaction ───────────────────────────────────────────────────────────

func TestRemoveReaction_DecrementsCount(t *testing.T) {
	// Arrange
	s := New(0)
	s.AppendMessage(Message{ID: 1, ChannelID: 5, Reactions: []Reaction{{EmojiName: "👍", Count: 3}}})

	// Act
	ok := s.RemoveReaction(5, 1, "👍", 0, false)

	// Assert
	if !ok {
		t.Fatal("RemoveReaction returned false")
	}
	rxs := s.Messages(5)[0].Reactions
	if len(rxs) != 1 || rxs[0].Count != 2 {
		t.Errorf("count = %d, want 2", rxs[0].Count)
	}
}

func TestRemoveReaction_RemovesEntryWhenCountReachesZero(t *testing.T) {
	s := New(0)
	s.AppendMessage(Message{ID: 1, ChannelID: 5, Reactions: []Reaction{{EmojiName: "👍", Count: 1}}})

	s.RemoveReaction(5, 1, "👍", 0, false)

	rxs := s.Messages(5)[0].Reactions
	if len(rxs) != 0 {
		t.Errorf("expected empty reactions, got %+v", rxs)
	}
}

func TestRemoveReaction_ClearsMe(t *testing.T) {
	s := New(0)
	s.AppendMessage(Message{ID: 1, ChannelID: 5, Reactions: []Reaction{{EmojiName: "👍", Count: 2, Me: true}}})

	s.RemoveReaction(5, 1, "👍", 0, true)

	rxs := s.Messages(5)[0].Reactions
	if len(rxs) != 1 || rxs[0].Me {
		t.Errorf("Me should be cleared: %+v", rxs)
	}
}

func TestRemoveReaction_ReturnsFalseForUnknownEmoji(t *testing.T) {
	s := New(0)
	s.AppendMessage(Message{ID: 1, ChannelID: 5, Reactions: []Reaction{{EmojiName: "👍", Count: 1}}})

	if s.RemoveReaction(5, 1, "❤️", 0, false) {
		t.Error("expected false for emoji not present")
	}
}

func TestRemoveReaction_ReturnsFalseForUnknownChannel(t *testing.T) {
	s := New(0)
	if s.RemoveReaction(99, 1, "👍", 0, false) {
		t.Error("expected false for unknown channel")
	}
}

func TestRemoveReaction_RetainsSiblingReactions(t *testing.T) {
	s := New(0)
	s.AppendMessage(Message{ID: 1, ChannelID: 5, Reactions: []Reaction{
		{EmojiName: "👍", Count: 1},
		{EmojiName: "❤️", Count: 2},
	}})

	s.RemoveReaction(5, 1, "👍", 0, false)

	rxs := s.Messages(5)[0].Reactions
	if len(rxs) != 1 || rxs[0].EmojiName != "❤️" {
		t.Errorf("sibling reaction lost: %+v", rxs)
	}
}

// ── MemberColor ──────────────────────────────────────────────────────────────

func TestMemberColor_ReturnsHighestPositionColoredRole(t *testing.T) {
	// Arrange
	s := New(0)
	s.UpsertMember(1, Member{ID: 10, RoleIDs: []RoleID{100, 200}})
	s.UpsertRole(1, Role{ID: 100, Position: 5, Color: 0xFF0000})
	s.UpsertRole(1, Role{ID: 200, Position: 10, Color: 0x00FF00})

	// Act
	color := s.MemberColor(1, 10)

	// Assert: role 200 has higher position → green wins
	if color != 0x00FF00 {
		t.Errorf("color = 0x%06X, want 0x00FF00", color)
	}
}

func TestMemberColor_SkipsColorlessRoles(t *testing.T) {
	s := New(0)
	s.UpsertMember(1, Member{ID: 10, RoleIDs: []RoleID{100, 200}})
	s.UpsertRole(1, Role{ID: 100, Position: 20, Color: 0}) // colorless despite high position
	s.UpsertRole(1, Role{ID: 200, Position: 5, Color: 0xABCDEF})

	color := s.MemberColor(1, 10)
	if color != 0xABCDEF {
		t.Errorf("color = 0x%06X, want 0xABCDEF", color)
	}
}

func TestMemberColor_ReturnsZeroWhenNoColoredRole(t *testing.T) {
	s := New(0)
	s.UpsertMember(1, Member{ID: 10, RoleIDs: []RoleID{100}})
	s.UpsertRole(1, Role{ID: 100, Position: 5, Color: 0})

	if color := s.MemberColor(1, 10); color != 0 {
		t.Errorf("color = 0x%06X, want 0", color)
	}
}

func TestMemberColor_ReturnsZeroForUnknownMember(t *testing.T) {
	s := New(0)
	if color := s.MemberColor(1, 999); color != 0 {
		t.Errorf("color = 0x%06X, want 0 for unknown member", color)
	}
}

func TestMemberColor_TieBreakByRoleID(t *testing.T) {
	s := New(0)
	s.UpsertMember(1, Member{ID: 10, RoleIDs: []RoleID{200, 100}})
	// Same position — lower ID wins per spec.
	s.UpsertRole(1, Role{ID: 100, Position: 5, Color: 0x0000FF})
	s.UpsertRole(1, Role{ID: 200, Position: 5, Color: 0xFF0000})

	color := s.MemberColor(1, 10)
	if color != 0x0000FF {
		t.Errorf("color = 0x%06X, want 0x0000FF (lower RoleID tie-break)", color)
	}
}

func TestMemberColor_MemberWithNoRoles(t *testing.T) {
	s := New(0)
	s.UpsertMember(1, Member{ID: 10, RoleIDs: nil})
	if color := s.MemberColor(1, 10); color != 0 {
		t.Errorf("color = 0x%06X, want 0 for member with no roles", color)
	}
}

func TestMemberDisplayRoleReturnsHighestGradientRole(t *testing.T) {
	s := New(0)
	s.UpsertMember(1, Member{ID: 10, RoleIDs: []RoleID{100, 200}})
	s.UpsertRole(1, Role{ID: 100, Position: 5, Color: 0xABCDEF})
	s.UpsertRole(1, Role{ID: 200, Position: 10, Colors: [3]uint32{0xFF0000, 0x0000FF}})

	role, ok := s.MemberDisplayRole(1, 10)
	if !ok {
		t.Fatal("MemberDisplayRole returned no role")
	}
	if role.ID != 200 {
		t.Fatalf("role ID = %d, want 200", role.ID)
	}
}

// ── LerpColor ────────────────────────────────────────────────────────────────

func TestLerpColor(t *testing.T) {
	tests := []struct {
		name string
		a, b uint32
		t    float64
		want uint32
	}{
		{"t=0 returns a", 0xFF0000, 0x0000FF, 0.0, 0xFF0000},
		{"t=1 returns b", 0xFF0000, 0x0000FF, 1.0, 0x0000FF},
		{"t<0 clamped to a", 0xFF0000, 0x0000FF, -1.0, 0xFF0000},
		{"t>1 clamped to b", 0xFF0000, 0x0000FF, 2.0, 0x0000FF},
		{"t=0.5 midpoint red+blue", 0xFF0000, 0x0000FF, 0.5, 0x800080}, // round(127.5)=128=0x80 for both channels
		{"t=0.5 black+white", 0x000000, 0xFFFFFF, 0.5, 0x808080},       // round(127.5)=128
		{"same color", 0xABCDEF, 0xABCDEF, 0.5, 0xABCDEF},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := LerpColor(tt.a, tt.b, tt.t)
			if got != tt.want {
				t.Errorf("LerpColor(0x%06X, 0x%06X, %v) = 0x%06X, want 0x%06X",
					tt.a, tt.b, tt.t, got, tt.want)
			}
		})
	}
}

// ── GradientAt ───────────────────────────────────────────────────────────────

func TestGradientAt(t *testing.T) {
	const red = uint32(0xFF0000)
	const blue = uint32(0x0000FF)
	const green = uint32(0x00FF00)

	tests := []struct {
		name   string
		role   Role
		t      float64
		wantFn func(got uint32) bool
		desc   string
	}{
		{
			name:   "no Colors returns flat Color",
			role:   Role{Color: red},
			t:      0.5,
			wantFn: func(got uint32) bool { return got == red },
			desc:   "want red",
		},
		{
			name:   "only primary returns primary",
			role:   Role{Colors: [3]uint32{red, 0, 0}},
			t:      0.5,
			wantFn: func(got uint32) bool { return got == red },
			desc:   "want red",
		},
		{
			name:   "two-stop t=0 returns primary",
			role:   Role{Colors: [3]uint32{red, blue, 0}},
			t:      0,
			wantFn: func(got uint32) bool { return got == red },
			desc:   "want red",
		},
		{
			name:   "two-stop t=1 returns secondary",
			role:   Role{Colors: [3]uint32{red, blue, 0}},
			t:      1,
			wantFn: func(got uint32) bool { return got == blue },
			desc:   "want blue",
		},
		{
			name:   "two-stop midpoint is blend",
			role:   Role{Colors: [3]uint32{0x000000, 0xFFFFFF, 0}},
			t:      0.5,
			wantFn: func(got uint32) bool { return got != 0x000000 && got != 0xFFFFFF },
			desc:   "want intermediate grey",
		},
		{
			name:   "three-stop t=0 returns primary",
			role:   Role{Colors: [3]uint32{red, green, blue}},
			t:      0,
			wantFn: func(got uint32) bool { return got == red },
			desc:   "want red",
		},
		{
			name:   "three-stop t=1 returns tertiary",
			role:   Role{Colors: [3]uint32{red, green, blue}},
			t:      1,
			wantFn: func(got uint32) bool { return got == blue },
			desc:   "want blue",
		},
		{
			name:   "three-stop t=0.5 returns secondary",
			role:   Role{Colors: [3]uint32{red, green, blue}},
			t:      0.5,
			wantFn: func(got uint32) bool { return got == green },
			desc:   "want green (boundary between segments)",
		},
		{
			name:   "all zeros Colors with no Color returns zero",
			role:   Role{Color: 0, Colors: [3]uint32{0, 0, 0}},
			t:      0.5,
			wantFn: func(got uint32) bool { return got == 0 },
			desc:   "want 0",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.role.GradientAt(tt.t)
			if !tt.wantFn(got) {
				t.Errorf("GradientAt(%v) = 0x%06X; %s", tt.t, got, tt.desc)
			}
		})
	}
}

// ── Message struct fields ────────────────────────────────────────────────────

func TestMessageRichFields(t *testing.T) {
	// Arrange: construct a fully populated Message and verify fields survive
	// a round-trip through AppendMessage / Messages.
	s := New(0)
	msg := Message{
		ID:        42,
		ChannelID: 5,
		AuthorID:  99,
		Attachments: []Attachment{{
			URL: "https://cdn.discordapp.com/attachments/a.png",
			W:   800, H: 600, Size: 12345,
		}},
		Embeds:     []Embed{{Kind: EmbedRich, Title: "cool embed", Color: 0x7289DA}},
		Stickers:   []Sticker{{ID: 1, Name: "blob", Format: StickerGIF}},
		Reactions:  []Reaction{{EmojiName: "🔥", Count: 7, Me: true}},
		Components: []Component{{Kind: ComponentButton, Label: "Click me", CustomID: "btn_1"}},
	}

	// Act
	s.AppendMessage(msg)
	got := s.Messages(5)[0]

	// Assert
	if got.AuthorID != 99 {
		t.Errorf("AuthorID = %d, want 99", got.AuthorID)
	}
	if len(got.Attachments) != 1 || got.Attachments[0].W != 800 {
		t.Errorf("Attachments: %+v", got.Attachments)
	}
	if len(got.Embeds) != 1 || got.Embeds[0].Title != "cool embed" {
		t.Errorf("Embeds: %+v", got.Embeds)
	}
	if len(got.Stickers) != 1 || got.Stickers[0].Format != StickerGIF {
		t.Errorf("Stickers: %+v", got.Stickers)
	}
	if len(got.Reactions) != 1 || !got.Reactions[0].Me {
		t.Errorf("Reactions: %+v", got.Reactions)
	}
	if len(got.Components) != 1 || got.Components[0].Label != "Click me" {
		t.Errorf("Components: %+v", got.Components)
	}
}
