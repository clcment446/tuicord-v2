package app

import (
	"testing"

	"awesomeProject/internal/store"

	"github.com/diamondburned/arikawa/v3/discord"
)

func TestConvertChannelKind(t *testing.T) {
	cases := []struct {
		in   discord.ChannelType
		want store.ChannelKind
	}{
		{discord.GuildText, store.ChannelText},
		{discord.GuildVoice, store.ChannelVoice},
		{discord.GuildStageVoice, store.ChannelVoice},
		{discord.GuildCategory, store.ChannelCategory},
		{discord.DirectMessage, store.ChannelDM},
		{discord.GroupDM, store.ChannelDM},
		{discord.GuildAnnouncement, store.ChannelAnnouncement},
		{discord.GuildForum, store.ChannelForum},
		{guildMedia, store.ChannelForum},
		{discord.GuildAnnouncementThread, store.ChannelThread},
		{discord.GuildPublicThread, store.ChannelThread},
		{discord.GuildPrivateThread, store.ChannelThread},
		{discord.ChannelType(999), store.ChannelText}, // unknown degrades to text
	}
	for _, c := range cases {
		if got := convertChannelKind(c.in); got != c.want {
			t.Errorf("convertChannelKind(%d) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestConvertDMChannelPreservesRecipientIDs(t *testing.T) {
	got := convertChannel(discord.Channel{ID: 9, Type: discord.DirectMessage, DMRecipients: []discord.User{{ID: 42, Username: "alice"}}})
	if len(got.RecipientIDs) != 1 || got.RecipientIDs[0] != 42 {
		t.Fatalf("recipient IDs = %v, want [42]", got.RecipientIDs)
	}
}

func TestConvertChannelThreadMeta(t *testing.T) {
	ch := discord.Channel{
		ID:           555,
		Type:         discord.GuildPublicThread,
		Name:         "hot-take",
		ParentID:     100,
		MessageCount: 7,
		MemberCount:  3,
		OwnerID:      42,
		ThreadMember: &discord.ThreadMember{},
		ThreadMetadata: &discord.ThreadMetadata{
			Archived: true,
			Locked:   true,
		},
		AppliedTags: []discord.TagID{9, 8},
	}
	got := convertChannel(ch)
	if got.Kind != store.ChannelThread {
		t.Fatalf("kind = %v, want thread", got.Kind)
	}
	if got.Thread == nil {
		t.Fatal("thread meta nil")
	}
	if !got.Thread.Archived || !got.Thread.Locked {
		t.Error("archived/locked not carried")
	}
	if got.Thread.MessageCount != 7 || got.Thread.MemberCount != 3 {
		t.Errorf("counts = %d/%d", got.Thread.MessageCount, got.Thread.MemberCount)
	}
	if got.Thread.OwnerID != 42 {
		t.Errorf("owner = %d", got.Thread.OwnerID)
	}
	if !got.Thread.Joined {
		t.Error("Joined should be true when ThreadMember present")
	}
	if len(got.Thread.AppliedTags) != 2 || got.Thread.AppliedTags[0] != 9 {
		t.Errorf("applied tags = %v", got.Thread.AppliedTags)
	}
	if got.ParentID != 100 {
		t.Errorf("parent = %d", got.ParentID)
	}
}

func TestConvertChannelForumMeta(t *testing.T) {
	sort := discord.SoftOrderTypeCreationDate
	name := "bug"
	ch := discord.Channel{
		ID:               200,
		Type:             discord.GuildForum,
		Name:             "help",
		DefaultSoftOrder: &sort,
		AvailableTags: []discord.Tag{
			{ID: 1, Name: "bug", ForumReaction: discord.ForumReaction{EmojiName: &name}},
			{ID: 2, Name: "idea"},
		},
	}
	got := convertChannel(ch)
	if got.Kind != store.ChannelForum || got.Forum == nil {
		t.Fatalf("forum meta missing: kind=%v forum=%v", got.Kind, got.Forum)
	}
	if got.Forum.DefaultSort != store.SortCreationDate {
		t.Errorf("sort = %v, want creation date", got.Forum.DefaultSort)
	}
	if len(got.Forum.Tags) != 2 {
		t.Fatalf("tags = %v", got.Forum.Tags)
	}
	if got.Forum.Tags[0].Name != "bug" || got.Forum.Tags[0].Emoji != "bug" {
		t.Errorf("tag[0] = %+v", got.Forum.Tags[0])
	}
}

func TestConvertChannelOverwrites(t *testing.T) {
	ch := discord.Channel{
		ID:   1,
		Type: discord.GuildText,
		Overwrites: []discord.Overwrite{
			{ID: 5, Type: discord.OverwriteRole, Deny: discord.Permissions(store.PermSendMessages)},
			{ID: 6, Type: discord.OverwriteMember, Allow: discord.Permissions(store.PermSendMessages)},
		},
	}
	got := convertChannel(ch)
	if len(got.Overwrites) != 2 {
		t.Fatalf("overwrites = %v", got.Overwrites)
	}
	if !got.Overwrites[0].Role || got.Overwrites[0].Deny != store.PermSendMessages {
		t.Errorf("overwrite[0] = %+v", got.Overwrites[0])
	}
	if got.Overwrites[1].Role || got.Overwrites[1].Allow != store.PermSendMessages {
		t.Errorf("overwrite[1] = %+v", got.Overwrites[1])
	}
}
