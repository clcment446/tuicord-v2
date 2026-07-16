package app

import (
	"encoding/json"
	"errors"
	"testing"

	"awesomeProject/internal/store"

	"github.com/diamondburned/arikawa/v3/discord"
)

func TestInteractionApplicationCommandPreservesCatalogJSON(t *testing.T) {
	raw := json.RawMessage(`{"id":"900","application_id":"555","name":"weather","type":1,"integration_types":[0],"contexts":[0],"dm_permission":false}`)
	encoded, err := json.Marshal(interactionApplicationCommand{raw: raw})
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(encoded, &got); err != nil {
		t.Fatal(err)
	}
	if got["dm_permission"] != false {
		t.Fatalf("dm_permission = %#v, want false", got["dm_permission"])
	}
	if _, ok := got["contexts"]; !ok {
		t.Fatalf("contexts was dropped: %s", encoded)
	}
}

// fakeInteractionPoster records the interaction payload and signals done. A
// non-nil release channel blocks completion until the test closes it.
type fakeInteractionPoster struct {
	payload componentInteraction
	err     error
	done    chan struct{}
	release chan struct{}
}

type fakeCommandInteractionPoster struct {
	payload commandInteraction
	done    chan struct{}
	err     error
}

type fakeAutocompletePoster struct {
	payload commandAutocompleteInteraction
	choices []CommandChoice
	done    chan struct{}
	err     error
}

func (f *fakeAutocompletePoster) postCommandAutocomplete(p commandAutocompleteInteraction) ([]CommandChoice, error) {
	f.payload = p
	close(f.done)
	return append([]CommandChoice(nil), f.choices...), f.err
}

func (f *fakeCommandInteractionPoster) postCommandInteraction(p commandInteraction) error {
	f.payload = p
	close(f.done)
	return f.err
}

func (f *fakeInteractionPoster) postComponentInteraction(p componentInteraction) error {
	f.payload = p
	close(f.done)
	if f.release != nil {
		<-f.release
	}
	return f.err
}

func componentTestMessage() store.Message {
	return store.Message{
		ID:            900,
		ChannelID:     42,
		AuthorID:      77,
		ApplicationID: 555,
		ComponentTree: []store.ComponentNode{{
			Kind: store.ComponentActionRow,
			Children: []store.ComponentNode{{
				Kind:     store.ComponentSelect,
				RawType:  3,
				CustomID: "sell_items",
			}},
		}},
	}
}

func TestSubmitComponentPostsInteractionPayload(t *testing.T) {
	// Arrange
	fi := &fakeInteractionPoster{done: make(chan struct{})}
	a := &App{store: store.New(0), ui: syncPoster{}, interact: fi, sessionID: "sess-1"}
	a.SetActive(7, 42)
	msg := componentTestMessage()
	msg.Flags = 1 << 15
	a.store.AppendMessage(msg)

	// Act
	a.SubmitComponent(ComponentSubmit{
		Message:       msg,
		ComponentType: 3,
		CustomID:      "sell_items",
		Values:        []string{"101", "102"},
	})
	<-fi.done

	// Assert
	p := fi.payload
	if p.Type != 3 || p.ChannelID != "42" || p.MessageID != "900" {
		t.Fatalf("payload = %+v, want type 3 for message 900 in channel 42", p)
	}
	if p.ApplicationID != "555" {
		t.Fatalf("application id = %q, want 555 (message ApplicationID)", p.ApplicationID)
	}
	if p.GuildID != "7" || p.SessionID != "sess-1" || p.Nonce == "" {
		t.Fatalf("payload routing = %+v, want guild 7, session sess-1, nonce set", p)
	}
	if p.Data.ComponentType != 3 || p.Data.CustomID != "sell_items" {
		t.Fatalf("data = %+v, want string select sell_items", p.Data)
	}
	if p.MessageFlags != 1<<15 {
		t.Fatalf("message flags = %d, want Components V2 flag", p.MessageFlags)
	}
	if got := len(p.Data.Values); got != 2 || p.Data.Values[0] != "101" {
		t.Fatalf("values = %v, want [101 102]", p.Data.Values)
	}
}

func TestSubmitComponentFallsBackToAuthorAndButton(t *testing.T) {
	// Arrange
	fi := &fakeInteractionPoster{done: make(chan struct{})}
	a := &App{store: store.New(0), ui: syncPoster{}, interact: fi}
	a.SetActive(DirectMessagesGuildID, 42)
	msg := componentTestMessage()
	msg.ApplicationID = 0

	// Act
	a.SubmitComponent(ComponentSubmit{Message: msg, CustomID: "confirm"})
	<-fi.done

	// Assert
	p := fi.payload
	if p.ApplicationID != "77" {
		t.Fatalf("application id = %q, want author fallback 77", p.ApplicationID)
	}
	if p.GuildID != "" {
		t.Fatalf("guild id = %q, want empty for DMs", p.GuildID)
	}
	if p.Data.ComponentType != 2 {
		t.Fatalf("component type = %d, want button fallback 2", p.Data.ComponentType)
	}
}

func TestSubmitCommandPostsChatInputInteraction(t *testing.T) {
	fake := &fakeCommandInteractionPoster{done: make(chan struct{})}
	a := &App{store: store.New(0), ui: syncPoster{}, commandInteract: fake, sessionID: "sess-1"}
	a.SetActive(7, 42)
	command := ApplicationCommand{Command: discord.Command{ID: 900, AppID: 555, Version: 123, Name: "weather", Type: discord.ChatInputCommand}}

	a.SubmitCommand(command, nil)
	<-fake.done

	p := fake.payload
	if p.Type != 2 || p.ChannelID != "42" || p.GuildID != "7" || p.ApplicationID != "555" || p.SessionID != "sess-1" || p.Nonce == "" {
		t.Fatalf("payload routing = %+v", p)
	}
	if p.Data.ID != "900" || p.Data.Version != "123" || p.Data.Name != "weather" || p.Data.Type != 1 {
		t.Fatalf("payload data = %+v", p.Data)
	}
	if p.Data.Attachments == nil || len(p.Data.Attachments) != 0 {
		t.Fatalf("attachments = %#v, want an explicit empty array", p.Data.Attachments)
	}
	if p.Data.ApplicationCommand.Command.Name != "weather" || len(p.Data.ApplicationCommand.IntegrationTypes) != 1 || p.Data.ApplicationCommand.IntegrationTypes[0] != 0 {
		t.Fatalf("application command = %+v", p.Data.ApplicationCommand)
	}
}

func TestSubmitCommandSerializesNestedOptions(t *testing.T) {
	fake := &fakeCommandInteractionPoster{done: make(chan struct{})}
	a := &App{store: store.New(0), ui: syncPoster{}, commandInteract: fake, sessionID: "sess-1"}
	a.SetActive(7, 42)
	command := ApplicationCommand{Command: discord.Command{ID: 900, AppID: 555, Version: 123, Name: "remind", Type: discord.ChatInputCommand}}

	a.SubmitCommand(command, []CommandOption{{
		Name: "create", Type: discord.SubcommandOptionType,
		Options: []CommandOption{{Name: "text", Type: discord.StringOptionType, Value: "stretch"}},
	}})
	<-fake.done

	options := fake.payload.Data.Options
	if len(options) != 1 || options[0].Name != "create" || options[0].Type != int(discord.SubcommandOptionType) {
		t.Fatalf("top-level options = %+v", options)
	}
	if len(options[0].Options) != 1 || options[0].Options[0].Name != "text" || options[0].Options[0].Value != "stretch" {
		t.Fatalf("nested options = %+v", options[0].Options)
	}
}

func TestAutocompleteCommandPostsFocusedOptionAndReturnsChoices(t *testing.T) {
	fake := &fakeAutocompletePoster{done: make(chan struct{}), choices: []CommandChoice{{Name: "Paris", Value: "paris"}}}
	a := &App{store: store.New(0), ui: syncPoster{}, commandAutocomplete: fake, sessionID: "sess-1"}
	a.SetActive(7, 42)
	command := ApplicationCommand{Command: discord.Command{ID: 900, AppID: 555, Version: 123, Name: "weather", Type: discord.ChatInputCommand}}
	result := make(chan []CommandChoice, 1)

	a.AutocompleteCommand(command, []CommandOption{{Name: "city", Type: discord.StringOptionType, Value: "par", Focused: true}}, func(choices []CommandChoice, err error) {
		if err != nil {
			t.Fatal(err)
		}
		result <- choices
	})
	<-fake.done

	p := fake.payload
	if p.Type != 4 || p.ChannelID != "42" || p.GuildID != "7" || p.ApplicationID != "555" || p.SessionID != "sess-1" {
		t.Fatalf("payload routing = %+v", p)
	}
	if len(p.Data.Options) != 1 || !p.Data.Options[0].Focused || p.Data.Options[0].Name != "city" || p.Data.Options[0].Value != "par" {
		t.Fatalf("payload options = %+v", p.Data.Options)
	}
	if p.Data.ApplicationCommand.Command.Name != "weather" || len(p.Data.ApplicationCommand.IntegrationTypes) != 1 || p.Data.ApplicationCommand.IntegrationTypes[0] != 0 {
		t.Fatalf("application command = %+v", p.Data.ApplicationCommand)
	}
	choices := <-result
	if len(choices) != 1 || choices[0].Value != "paris" {
		t.Fatalf("choices = %+v", choices)
	}
}

func TestSubmitComponentMarksStateAndReportsErrors(t *testing.T) {
	// Arrange
	fi := &fakeInteractionPoster{done: make(chan struct{}), release: make(chan struct{}), err: errors.New("boom")}
	a := &App{store: store.New(0), ui: syncPoster{}, interact: fi}
	errCh := make(chan error, 1)
	a.OnError(func(err error) { errCh <- err })
	msg := componentTestMessage()
	a.store.AppendMessage(msg)

	componentState := func() store.ComponentState {
		msgs := a.store.Messages(42)
		return msgs[len(msgs)-1].ComponentTree[0].Children[0].State
	}

	// Act
	a.SubmitComponent(ComponentSubmit{Message: msg, ComponentType: 3, CustomID: "sell_items"})
	<-fi.done // request captured; completion is blocked on release
	if got := componentState(); got != store.ComponentStatePending {
		t.Fatalf("component state = %v, want pending right after submit", got)
	}
	close(fi.release)
	reported := <-errCh // completion ran; the state write precedes OnError

	// Assert
	if got := componentState(); got != store.ComponentStateError {
		t.Fatalf("component state = %v, want error", got)
	}
	if reported == nil || reported.Error() != "boom" {
		t.Fatalf("reported error = %v, want boom", reported)
	}
}

func TestSubmitComponentIgnoresIncompleteActions(t *testing.T) {
	fi := &fakeInteractionPoster{done: make(chan struct{})}
	a := &App{store: store.New(0), ui: syncPoster{}, interact: fi}

	a.SubmitComponent(ComponentSubmit{Message: store.Message{ID: 1, ChannelID: 2}})         // no custom id
	a.SubmitComponent(ComponentSubmit{Message: store.Message{ChannelID: 2}, CustomID: "x"}) // pending message
	select {
	case <-fi.done:
		t.Fatal("incomplete action should not post an interaction")
	default:
	}
}
