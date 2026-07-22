// Package app orchestrates the Discord session, the normalized store, and the TUI runtime.
package app

import (
	clientdiscord "awesomeProject/internal/discord"
	"awesomeProject/internal/store"
	"fmt"
	"github.com/diamondburned/arikawa/v3/discord"
	"strconv"
	"strings"
)

// SubmitCommand dispatches a chat-input command in the active channel. The UI
// supplies typed, validated option values; a focused option is only valid for
// the separate autocomplete interaction.
func (a *App) SubmitCommand(command ApplicationCommand, options []CommandOption) {
	if a == nil || a.commandInteract == nil || command.ID == 0 || command.AppID == 0 || command.Name == "" || a.activeChannel == 0 {
		return
	}
	if command.Type != 0 && command.Type != discord.ChatInputCommand {
		return
	}
	wireOptions, focused := commandOptionsToWire(options)
	if focused != 0 {
		a.reportError(fmt.Errorf("slash command %q cannot submit a focused autocomplete option", command.Name))
		return
	}
	payload := commandInteraction{
		Type:          applicationCommandInteractionType,
		Nonce:         newInteractionNonce(),
		ChannelID:     strconv.FormatUint(uint64(a.activeChannel), 10),
		ApplicationID: strconv.FormatUint(uint64(command.AppID), 10),
		SessionID:     a.sessionID,
		Data: commandInteractionData{
			ID:                 strconv.FormatUint(uint64(command.ID), 10),
			Name:               command.Name,
			Type:               int(discord.ChatInputCommand),
			Version:            strconv.FormatUint(uint64(command.Version), 10),
			Options:            wireOptions,
			Attachments:        []any{},
			ApplicationCommand: interactionApplicationCommand{Command: command, IntegrationTypes: commandIntegrationTypes(a.activeGuild)},
		},
	}
	if a.activeGuild != 0 && a.activeGuild != DirectMessagesGuildID {
		payload.GuildID = strconv.FormatUint(uint64(a.activeGuild), 10)
	}
	go func() {
		if err := a.commandInteract.postCommandInteraction(payload); err != nil {
			a.ui.Post(func() { a.reportError(err) })
		}
	}()
}

// AutocompleteCommand asks Discord's application for suggestions for one
// focused command option. The caller is always completed on the UI goroutine.
func (a *App) AutocompleteCommand(command ApplicationCommand, options []CommandOption, done func([]CommandChoice, error)) {
	if a == nil || a.commandAutocomplete == nil || done == nil || command.ID == 0 || command.AppID == 0 || command.Name == "" || a.activeChannel == 0 {
		return
	}
	wireOptions, focused := commandOptionsToWire(options)
	if focused != 1 {
		a.ui.Post(func() { done(nil, fmt.Errorf("autocomplete requires exactly one focused option")) })
		return
	}
	payload := commandAutocompleteInteraction(commandInteraction{
		Type:          applicationCommandAutocompleteInteractionType,
		Nonce:         newInteractionNonce(),
		ChannelID:     strconv.FormatUint(uint64(a.activeChannel), 10),
		ApplicationID: strconv.FormatUint(uint64(command.AppID), 10),
		SessionID:     a.sessionID,
		Data: commandInteractionData{
			ID:                 strconv.FormatUint(uint64(command.ID), 10),
			Name:               command.Name,
			Type:               int(discord.ChatInputCommand),
			Version:            strconv.FormatUint(uint64(command.Version), 10),
			Options:            wireOptions,
			Attachments:        []any{},
			ApplicationCommand: interactionApplicationCommand{Command: command, IntegrationTypes: commandIntegrationTypes(a.activeGuild)},
		},
	})
	if a.activeGuild != 0 && a.activeGuild != DirectMessagesGuildID {
		payload.GuildID = strconv.FormatUint(uint64(a.activeGuild), 10)
	}
	go func() {
		choices, err := a.commandAutocomplete.postCommandAutocomplete(payload)
		a.ui.Post(func() { done(append([]CommandChoice(nil), choices...), err) })
	}()
}

func commandIntegrationTypes(guild store.GuildID) []int {
	if guild == 0 || guild == DirectMessagesGuildID {
		return []int{1}
	}
	return []int{0}
}

func commandOptionsToWire(options []CommandOption) ([]commandInteractionOption, int) {
	wire := make([]commandInteractionOption, 0, len(options))
	focused := 0
	for _, option := range options {
		nested, nestedFocused := commandOptionsToWire(option.Options)
		if option.Focused {
			focused++
		}
		focused += nestedFocused
		wire = append(wire, commandInteractionOption{
			Type:    int(option.Type),
			Name:    option.Name,
			Value:   option.Value,
			Focused: option.Focused,
			Options: nested,
		})
	}
	return wire, focused
}

// SearchGIFs searches Discord's Tenor proxy away from the UI thread and posts
// completion back onto the UI event loop.
func (a *App) SearchGIFs(query string, done func([]clientdiscord.GIFResult, error)) {
	query = strings.TrimSpace(query)
	if a == nil || a.gifs == nil || query == "" || done == nil {
		return
	}
	go func() {
		results, err := a.gifs.SearchGIFs(query)
		a.ui.Post(func() {
			if err != nil && a.onError != nil {
				a.onError(err)
			}
			done(results, err)
		})
	}()
}
