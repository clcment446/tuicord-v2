package discord

import (
	"os"
	"strings"
	"testing"

	"github.com/diamondburned/arikawa/v3/discord"
)

func TestNewSessionDoesNotPanic(t *testing.T) {
	sess, err := NewSession("testtoken")
	if err != nil {
		t.Fatal(err)
	}
	if sess == nil {
		t.Fatal("session is nil")
	}
}

func TestSuperProperties(t *testing.T) {
	got, err := superProperties()
	if err != nil {
		t.Fatal(err)
	}
	if got == "" {
		t.Fatal("super properties is empty")
	}
}

func TestSessionLoadsCurrentUserFromDiscordAPI(t *testing.T) {
	token := testToken(t)
	sess, err := NewSession(token)
	if err != nil {
		t.Fatal(err)
	}
	me, err := sess.Me()
	if err != nil {
		t.Fatalf("load current user from Discord API: %v", err)
	}
	if me == nil || me.ID == 0 || me.Username == "" {
		t.Fatalf("current user = %+v, want id and username from Discord API", me)
	}
}

func TestSessionLoadsNamesAndHistoryFromDiscordAPI(t *testing.T) {
	token := testToken(t)
	sess, err := NewSession(token)
	if err != nil {
		t.Fatal(err)
	}

	t.Run("server names", func(t *testing.T) {
		guilds, err := sess.Guilds(10)
		if err != nil {
			t.Fatalf("load guilds from Discord API: %v", err)
		}
		if len(guilds) == 0 {
			t.Skip("token has no guilds")
		}
		if guilds[0].ID == 0 || guilds[0].Name == "" {
			t.Fatalf("guild = %+v, want id and name", guilds[0])
		}
	})

	t.Run("DM user names", func(t *testing.T) {
		dms, err := sess.PrivateChannels()
		if err != nil {
			t.Fatalf("load private channels from Discord API: %v", err)
		}
		for _, dm := range dms {
			if dm.Type != discord.DirectMessage && dm.Type != discord.GroupDM {
				continue
			}
			if dm.Name != "" {
				return
			}
			for _, user := range dm.DMRecipients {
				if user.DisplayOrUsername() != "" {
					return
				}
			}
		}
		t.Skip("token has no named DM channels")
	})

	t.Run("channel names and message history", func(t *testing.T) {
		channel, ok := firstHistoryChannel(t, sess)
		if !ok {
			t.Skip("token has no accessible text channel or DM")
		}
		if channel.ID == 0 || channelName(channel) == "" {
			t.Fatalf("channel = %+v, want id and resolved name", channel)
		}
		messages, err := sess.Messages(channel.ID, 5)
		if err != nil {
			t.Fatalf("load message history from Discord API: %v", err)
		}
		if len(messages) == 0 {
			t.Skip("channel has no recent messages")
		}
		if messages[0].ID == 0 || messages[0].ChannelID != channel.ID {
			t.Fatalf("message = %+v, want id and matching channel", messages[0])
		}
		for i := 1; i < len(messages); i++ {
			if messages[i-1].ID < messages[i].ID {
				t.Fatalf("messages not latest-first: %d before %d", messages[i-1].ID, messages[i].ID)
			}
		}
	})
}

type namedHistoryClient interface {
	Guilds(uint) ([]discord.Guild, error)
	Channels(discord.GuildID) ([]discord.Channel, error)
	PrivateChannels() ([]discord.Channel, error)
}

func firstHistoryChannel(t *testing.T, sess namedHistoryClient) (discord.Channel, bool) {
	t.Helper()
	guilds, err := sess.Guilds(10)
	if err == nil {
		for _, guild := range guilds {
			channels, err := sess.Channels(guild.ID)
			if err != nil {
				continue
			}
			for _, channel := range channels {
				if channel.Type == discord.GuildText && channel.Name != "" {
					return channel, true
				}
			}
		}
	}
	dms, err := sess.PrivateChannels()
	if err != nil {
		return discord.Channel{}, false
	}
	for _, dm := range dms {
		if dm.Type == discord.DirectMessage || dm.Type == discord.GroupDM {
			return dm, true
		}
	}
	return discord.Channel{}, false
}

func channelName(channel discord.Channel) string {
	if channel.Name != "" {
		return channel.Name
	}
	names := make([]string, 0, len(channel.DMRecipients))
	for _, user := range channel.DMRecipients {
		if name := user.DisplayOrUsername(); name != "" {
			names = append(names, name)
		}
	}
	return strings.Join(names, ", ")
}

func testToken(t *testing.T) string {
	t.Helper()
	token := os.Getenv("TOKEN")
	if token == "" {
		raw, err := os.ReadFile("../../.env")
		if err != nil {
			t.Skip("TOKEN not set and .env not found")
		}
		for _, line := range strings.Split(string(raw), "\n") {
			key, value, ok := strings.Cut(strings.TrimSpace(line), "=")
			if ok && key == "TOKEN" {
				token = strings.Trim(value, `"'`)
				break
			}
		}
	}
	if token == "" {
		t.Skip("TOKEN not set")
	}
	return token
}
