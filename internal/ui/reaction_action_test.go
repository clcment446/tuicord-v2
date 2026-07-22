package ui

import (
	"errors"
	"net/url"
	"strings"
	"testing"
	"time"

	"awesomeProject/internal/app"
	"awesomeProject/internal/config"
	"awesomeProject/internal/discord"
	"awesomeProject/internal/store"
	"awesomeProject/internal/tui/input"
	"awesomeProject/internal/tui/tui"
	"awesomeProject/internal/tui/widget"
	"awesomeProject/internal/uistate"
	"github.com/diamondburned/arikawa/v3/session"
	"github.com/diamondburned/arikawa/v3/utils/httputil/httpdriver"
)

func TestShellAddReactionActionOpensPickerAndReacts(t *testing.T) {
	st := store.New(0)
	st.UpsertGuild(store.Guild{ID: 1, Name: "Home"})
	st.UpsertChannel(store.Channel{ID: 7, GuildID: 1, Name: "general", Kind: store.ChannelText})
	st.SetGuildEmojis(1, []store.GuildEmoji{{ID: 20, Name: "party", Animated: true}})

	requests := make(chan string, 1)
	sess := session.New("")
	sess.Client.OnRequest = append(sess.Client.OnRequest, func(request httpdriver.Request) error {
		requests <- request.GetPath()
		return errors.New("test stops before network I/O")
	})
	runtime := tui.New()
	logic := app.New(discord.WrapSession(sess), st, runtime)
	logic.SetActive(1, 7)
	mv := &MainView{app: logic, state: &uistate.State{}, Root: widget.NewText("main")}
	shell := &Shell{app: logic, mv: mv, cfg: config.Default()}

	shell.handleMessageAction('a', store.Message{ID: 9, ChannelID: 7, Author: "alice"})
	picker, ok := shell.overlay.(*InlinePicker)
	if !ok {
		t.Fatalf("add-reaction overlay = %T, want *InlinePicker", shell.overlay)
	}
	picker.query = "party"
	picker.refilter()
	if len(picker.filtered) == 0 || picker.filtered[0].label != ":party:" || !picker.filtered[0].usable {
		t.Fatalf("first reaction picker result = %+v, want usable party emoji", picker.filtered)
	}
	if !picker.Handle(input.KeyEvent{Key: input.KeyEnter}) {
		t.Fatal("reaction picker did not handle Enter")
	}

	select {
	case path := <-requests:
		decoded, err := url.PathUnescape(path)
		if err != nil {
			t.Fatalf("decode reaction request path: %v", err)
		}
		if want := "/channels/7/messages/9/reactions/party:20/@me"; !strings.Contains(decoded, want) {
			t.Fatalf("reaction request path = %q, want it to contain %q", decoded, want)
		}
	case <-time.After(time.Second):
		t.Fatal("selecting the reaction did not call Discord")
	}
	if shell.overlay != nil {
		t.Fatalf("reaction picker remained open after selection: %T", shell.overlay)
	}
}
