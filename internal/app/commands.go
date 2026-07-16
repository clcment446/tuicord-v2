package app

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"time"

	"awesomeProject/internal/store"

	"github.com/diamondburned/arikawa/v3/api"
	"github.com/diamondburned/arikawa/v3/discord"
	"github.com/diamondburned/arikawa/v3/session"
)

// ApplicationCommand pairs Arikawa's convenient public model with the exact
// catalog object Discord sent. User-originated interaction requests must echo
// that object verbatim; re-marshalling the public model can lose client-only
// fields such as contexts and integration_types.
type ApplicationCommand struct {
	discord.Command
	raw json.RawMessage
}

func (c *ApplicationCommand) UnmarshalJSON(data []byte) error {
	var command discord.Command
	if err := json.Unmarshal(data, &command); err != nil {
		return err
	}
	c.Command = command
	c.raw = append(c.raw[:0], data...)
	return nil
}

func (c ApplicationCommand) MarshalJSON() ([]byte, error) {
	if len(c.raw) != 0 {
		return append([]byte(nil), c.raw...), nil
	}
	return json.Marshal(c.Command)
}

// CommandContext identifies the Discord surface whose commands may be run.
// GuildID is zero for a DM, where Discord exposes a channel-scoped search API.
type CommandContext struct {
	GuildID   store.GuildID
	ChannelID store.ChannelID
}

// commandCatalogLoader is deliberately separate from the interaction poster:
// discovery is read-only and can be cached safely without coupling it to a
// command execution attempt.
type commandCatalogLoader interface {
	LoadCommandCatalog(CommandContext) ([]ApplicationCommand, error)
}

type commandCacheEntry struct {
	commands  []ApplicationCommand
	expiresAt time.Time
}

const commandCatalogTTL = 5 * time.Minute

// applicationCommandIndex is the contextual catalog returned by Discord for a
// guild. Applications are retained only for JSON compatibility; command
// selection needs application_id embedded in each command.
type applicationCommandIndex struct {
	Applications        []commandApplication `json:"applications"`
	ApplicationCommands []ApplicationCommand `json:"application_commands"`
}

type commandApplication struct {
	ID   discord.AppID `json:"id"`
	Name string        `json:"name"`
}

type applicationCommandSearch struct {
	ApplicationCommands []ApplicationCommand `json:"application_commands"`
}

type restCommandCatalogLoader struct{ sess *session.Session }

func (r restCommandCatalogLoader) LoadCommandCatalog(ctx CommandContext) ([]ApplicationCommand, error) {
	if r.sess == nil || ctx.ChannelID == 0 {
		return nil, fmt.Errorf("discord command catalog requires a channel")
	}
	var commands []ApplicationCommand
	if ctx.GuildID != 0 && ctx.GuildID != DirectMessagesGuildID {
		var index applicationCommandIndex
		url := api.EndpointGuilds + strconv.FormatUint(uint64(ctx.GuildID), 10) + "/application-command-index"
		if err := r.sess.RequestJSON(&index, "GET", url); err != nil {
			return nil, err
		}
		commands = index.ApplicationCommands
	} else {
		var search applicationCommandSearch
		url := api.EndpointChannels + strconv.FormatUint(uint64(ctx.ChannelID), 10) + "/application-commands/search?type=1&query=&limit=100"
		if err := r.sess.RequestJSON(&search, "GET", url); err != nil {
			return nil, err
		}
		commands = search.ApplicationCommands
	}
	return chatInputCommands(commands), nil
}

func chatInputCommands(commands []ApplicationCommand) []ApplicationCommand {
	filtered := make([]ApplicationCommand, 0, len(commands))
	for _, command := range commands {
		if command.Type == 0 || command.Type == discord.ChatInputCommand {
			filtered = append(filtered, command)
		}
	}
	sort.SliceStable(filtered, func(i, j int) bool {
		if filtered[i].Name != filtered[j].Name {
			return filtered[i].Name < filtered[j].Name
		}
		return filtered[i].AppID < filtered[j].AppID
	})
	return filtered
}

// LoadCommands returns the current context's chat-input command catalog. It
// serializes refreshes and stores immutable snapshots for five minutes, so
// opening and closing the composer cannot hammer Discord's API.
func (a *App) LoadCommands(ctx CommandContext) ([]ApplicationCommand, error) {
	if a == nil || a.commandCatalog == nil {
		return nil, fmt.Errorf("discord command catalog is unavailable")
	}
	if ctx.ChannelID == 0 {
		return nil, fmt.Errorf("discord command catalog requires a channel")
	}
	now := time.Now
	if a.now != nil {
		now = a.now
	}
	a.commandMu.Lock()
	defer a.commandMu.Unlock()
	if cached, ok := a.commandCache[ctx]; ok && now().Before(cached.expiresAt) {
		return append([]ApplicationCommand(nil), cached.commands...), nil
	}
	commands, err := a.commandCatalog.LoadCommandCatalog(ctx)
	if err != nil {
		return nil, err
	}
	snapshot := append([]ApplicationCommand(nil), commands...)
	if a.commandCache == nil {
		a.commandCache = make(map[CommandContext]commandCacheEntry)
	}
	a.commandCache[ctx] = commandCacheEntry{commands: snapshot, expiresAt: now().Add(commandCatalogTTL)}
	return append([]ApplicationCommand(nil), snapshot...), nil
}

// InvalidateCommandCache drops cached command catalogs after reconnects or
// when the active context's integrations could have changed.
func (a *App) InvalidateCommandCache() {
	if a == nil {
		return
	}
	a.commandMu.Lock()
	a.commandCache = make(map[CommandContext]commandCacheEntry)
	a.commandMu.Unlock()
}

var _ commandCatalogLoader = restCommandCatalogLoader{}
