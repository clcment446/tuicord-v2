package app

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	clientdiscord "awesomeProject/internal/discord"
	"awesomeProject/internal/store"

	"github.com/diamondburned/arikawa/v3/api"
	"github.com/diamondburned/arikawa/v3/discord"
	"github.com/diamondburned/arikawa/v3/gateway"
	"github.com/diamondburned/arikawa/v3/session"
)

const (
	liveTargetGuild       = "Heavenly Dao | 天道宗"
	liveTargetApplication = "Heavenly Dao"
)

// TestLiveCommandAutocomplete validates the user-session type-4 request
// against Discord. It is deliberately opt-in because it reaches a third-party
// application, though autocomplete itself does not execute a command.
func TestLiveCommandAutocomplete(t *testing.T) {
	if os.Getenv("TUICORD_LIVE_AUTOCOMPLETE") != "1" {
		t.Skip("set TUICORD_LIVE_AUTOCOMPLETE=1 to run against Discord")
	}
	token := os.Getenv("TOKEN")
	if token == "" {
		t.Fatal("TOKEN is required")
	}
	sess, err := clientdiscord.NewSession(token)
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	defer sess.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	ready := make(chan string, 1)
	sess.AddHandler(func(event *gateway.ReadyEvent) { ready <- event.SessionID })
	go func() { _ = sess.Connect(ctx) }()
	var sessionID string
	select {
	case sessionID = <-ready:
	case <-ctx.Done():
		t.Fatal("gateway READY did not arrive")
	}

	guild, err := liveGuild(sess)
	if err != nil {
		t.Skip(err.Error())
	}
	channels, err := sess.Channels(guild.ID)
	if err != nil {
		t.Fatalf("Channels: %v", err)
	}
	var channelID discord.ChannelID
	for _, channel := range channels {
		if channel.Type == discord.GuildText {
			channelID = channel.ID
			break
		}
	}
	if channelID == 0 {
		t.Skip("no text channel available for autocomplete validation")
	}

	commands, err := liveTargetCommands(sess, guild.ID)
	if err != nil {
		t.Fatalf("LoadCommandCatalog: %v", err)
	}
	command, option, ok := namedAutocompleteCommand(commands, "equip")
	if !ok {
		t.Skip("no autocomplete-enabled command in sampled guild")
	}

	a := &App{
		store:               store.New(0),
		ui:                  syncPoster{},
		commandAutocomplete: restCommandAutocompletePoster{sess: sess},
		sessionID:           sessionID,
	}
	a.SetActive(store.GuildID(guild.ID), store.ChannelID(channelID))
	result := make(chan error, 1)
	a.AutocompleteCommand(command, []CommandOption{{Name: option.Name(), Type: option.Type(), Value: "", Focused: true}}, func(_ []CommandChoice, err error) {
		result <- err
	})
	select {
	case err := <-result:
		if err != nil {
			t.Fatalf("AutocompleteCommand: %v", err)
		}
	case <-ctx.Done():
		t.Fatal("autocomplete request timed out")
	}
}

// TestLiveCommandInvocation validates the type-2 payload with an exact
// no-argument /help command only. It may cause the selected application's
// normal help response to appear in the sampled channel.
func TestLiveCommandInvocation(t *testing.T) {
	if os.Getenv("TUICORD_LIVE_COMMAND") != "1" {
		t.Skip("set TUICORD_LIVE_COMMAND=1 to run against Discord")
	}
	token := os.Getenv("TOKEN")
	if token == "" {
		t.Fatal("TOKEN is required")
	}
	sess, err := clientdiscord.NewSession(token)
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	defer sess.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	ready := make(chan string, 1)
	sess.AddHandler(func(event *gateway.ReadyEvent) { ready <- event.SessionID })
	go func() { _ = sess.Connect(ctx) }()
	var sessionID string
	select {
	case sessionID = <-ready:
	case <-ctx.Done():
		t.Fatal("gateway READY did not arrive")
	}

	guild, err := liveGuild(sess)
	if err != nil {
		t.Skip(err.Error())
	}
	channels, err := sess.Channels(guild.ID)
	if err != nil {
		t.Fatalf("Channels: %v", err)
	}
	var channelID discord.ChannelID
	for _, channel := range channels {
		if channel.Type == discord.GuildText {
			channelID = channel.ID
			break
		}
	}
	if channelID == 0 {
		t.Skip("no text channel available for command validation")
	}
	commands, err := liveTargetCommands(sess, guild.ID)
	if err != nil {
		t.Fatalf("LoadCommandCatalog: %v", err)
	}
	var command ApplicationCommand
	for _, candidate := range commands {
		if candidate.Name == "tutorial" && len(candidate.Options) == 0 {
			command = candidate
			break
		}
	}
	if command.ID == 0 {
		t.Skip("Heavenly Dao /tutorial command is unavailable")
	}

	payload := commandInteraction{
		Type:          applicationCommandInteractionType,
		Nonce:         newInteractionNonce(),
		GuildID:       guild.ID.String(),
		ChannelID:     channelID.String(),
		ApplicationID: command.AppID.String(),
		SessionID:     sessionID,
		Data: commandInteractionData{
			ID:                 command.ID.String(),
			Name:               command.Name,
			Type:               int(discord.ChatInputCommand),
			Version:            command.Version.String(),
			Attachments:        []any{},
			ApplicationCommand: interactionApplicationCommand{Command: command, IntegrationTypes: commandIntegrationTypes(store.GuildID(guild.ID))},
		},
	}
	if err := (restCommandInteractionPoster{sess: sess}).postCommandInteraction(payload); err != nil {
		t.Fatalf("type-2 interaction rejected: %v", err)
	}
}

func liveGuild(sess interface {
	Guilds(uint) ([]discord.Guild, error)
}) (discord.Guild, error) {
	guilds, err := sess.Guilds(100)
	if err != nil {
		return discord.Guild{}, err
	}
	for _, guild := range guilds {
		if guild.Name == liveTargetGuild {
			return guild, nil
		}
	}
	return discord.Guild{}, fmt.Errorf("target guild %q is unavailable", liveTargetGuild)
}

func liveTargetCommands(sess *session.Session, guildID discord.GuildID) ([]ApplicationCommand, error) {
	var index applicationCommandIndex
	if err := sess.RequestJSON(&index, "GET", api.EndpointGuilds+guildID.String()+"/application-command-index"); err != nil {
		return nil, err
	}
	var appID discord.AppID
	for _, application := range index.Applications {
		if application.Name == liveTargetApplication {
			appID = application.ID
			break
		}
	}
	if appID == 0 {
		return nil, fmt.Errorf("target application %q is unavailable", liveTargetApplication)
	}
	commands := make([]ApplicationCommand, 0)
	for _, command := range index.ApplicationCommands {
		if command.AppID == appID && (command.Type == 0 || command.Type == discord.ChatInputCommand) {
			commands = append(commands, command)
		}
	}
	return commands, nil
}

func firstAutocompleteCommand(commands []ApplicationCommand) (ApplicationCommand, discord.CommandOption, bool) {
	for _, command := range commands {
		for _, option := range command.Options {
			if hasAutocomplete(option) {
				return command, option, true
			}
		}
	}
	return ApplicationCommand{}, nil, false
}

func namedAutocompleteCommand(commands []ApplicationCommand, name string) (ApplicationCommand, discord.CommandOption, bool) {
	for _, command := range commands {
		if command.Name != name {
			continue
		}
		for _, option := range command.Options {
			if hasAutocomplete(option) {
				return command, option, true
			}
		}
	}
	return ApplicationCommand{}, nil, false
}

func hasAutocomplete(option discord.CommandOption) bool {
	switch value := option.(type) {
	case *discord.StringOption:
		return value.Autocomplete
	case *discord.IntegerOption:
		return value.Autocomplete
	case *discord.NumberOption:
		return value.Autocomplete
	}
	return false
}
