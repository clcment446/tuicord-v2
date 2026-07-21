package ui

import (
	"strings"
	"testing"

	"awesomeProject/internal/app"
	"awesomeProject/internal/config"
	"awesomeProject/internal/discord"
	"awesomeProject/internal/store"
	"awesomeProject/internal/tui/input"
	"awesomeProject/internal/tui/screen"
	"awesomeProject/internal/tui/tui"
	"awesomeProject/internal/tui/widget"
	"github.com/diamondburned/arikawa/v3/session"
)

func TestProfilePopupRendersIdentityRolesAndSharedDM(t *testing.T) {
	opened := store.ChannelID(0)
	p := NewProfilePopup(profileDetails{
		ID: 42, Name: "Alice", Username: "alice", Nick: "ali",
		Roles:  []profileRole{{Name: "Admin", Color: 0xff0000}, {Name: "Member"}},
		Guilds: []string{"Server One", "Server Two"},
		DMs:    []profileDM{{ID: 9, Name: "alice"}},
	}, Styles{}, func(id store.ChannelID) { opened = id }, nil)
	buf := screen.NewBuffer(60, 24)
	p.Draw(buf.Clip(buf.Bounds()))
	contents := rowsText(buf)
	for _, want := range []string{"Alice", "alice", "ali", "42", "Admin", "Servers in common", "Server One", "Server Two", "Open DM"} {
		if !strings.Contains(contents, want) {
			t.Fatalf("profile missing %q:\n%s", want, contents)
		}
	}
	// The Admin role chip must render in the role's color.
	adminStyle := screen.Style{}
	for y := 0; y < buf.Height(); y++ {
		for x := 0; x < buf.Width(); x++ {
			if buf.Cell(x, y).Content == "@" && buf.Cell(x+1, y).Content == "A" {
				adminStyle = buf.Cell(x+1, y).Style
			}
		}
	}
	if adminStyle.Fg != screen.RGB(0xff, 0, 0) {
		t.Errorf("Admin role fg = %+v, want red role color", adminStyle.Fg)
	}
	box := p.box(60, 24)
	if !p.Handle(input.MouseEvent{Kind: input.MousePress, Btn: input.ButtonLeft, X: box.X + 2, Y: box.Y + p.dmRow}) {
		t.Fatal("DM row click was not handled")
	}
	if opened != 9 {
		t.Fatalf("opened DM = %d, want 9", opened)
	}
}

func TestVimUserActionOpensProfileFromMessageIdentity(t *testing.T) {
	st := store.New(0)
	tuiApp := tui.New()
	logic := app.New(discord.WrapSession(session.New("")), st, tuiApp)
	mv := &MainView{app: logic, Root: widget.NewText("main")}
	shell := &Shell{app: logic, mv: mv, cfg: config.Default()}
	msg := store.Message{AuthorID: 42, Author: "alice", AuthorAvatarURL: "https://cdn.example/alice.png"}

	shell.handleMessageAction('u', msg)

	popup, ok := shell.popup.(*ProfilePopup)
	if !ok {
		t.Fatalf("popup = %T, want *ProfilePopup", shell.popup)
	}
	if popup.details.ID != 42 || popup.details.Name != "alice" || popup.details.AvatarURL != msg.AuthorAvatarURL {
		t.Fatalf("profile details = %+v, want selected message author identity", popup.details)
	}
	if len(shell.Toasts()) != 0 {
		t.Fatalf("profile action created %d notices, want none", len(shell.Toasts()))
	}
}

func TestProfilePopupCanBeDraggedByTitleBar(t *testing.T) {
	p := NewProfilePopup(profileDetails{ID: 42, Name: "Alice"}, Styles{}, nil, nil)
	buf := screen.NewBuffer(60, 20)
	p.Draw(buf.Clip(buf.Bounds()))
	before := p.box(60, 20)
	p.Handle(input.MouseEvent{Kind: input.MousePress, Btn: input.ButtonLeft, X: before.X + 2, Y: before.Y})
	p.Handle(input.MouseEvent{Kind: input.MouseMotion, X: before.X + 8, Y: before.Y + 3})
	p.Handle(input.MouseEvent{Kind: input.MouseRelease, Btn: input.ButtonLeft, X: before.X + 8, Y: before.Y + 3})
	after := p.box(60, 20)
	if after.X <= before.X || after.Y <= before.Y {
		t.Fatalf("profile did not move: before=%+v after=%+v", before, after)
	}
}

func TestBuildProfileDetailsFindsRolesAndDMByStableIDs(t *testing.T) {
	st := store.New(0)
	st.UpsertMember(1, store.Member{ID: 42, Name: "Ali", Username: "alice", Nick: "ali", RoleIDs: []store.RoleID{7}})
	st.UpsertRole(1, store.Role{ID: 7, Name: "Admin"})
	st.UpsertChannel(store.Channel{ID: 9, GuildID: 99, Kind: store.ChannelDM, Name: "Alice", RecipientIDs: []store.UserID{42}})
	details := buildProfileDetails(st, 1, 99, 42)
	if details.Username != "alice" || details.Nick != "ali" || len(details.Roles) != 1 || details.Roles[0].Name != "Admin" || len(details.DMs) != 1 || details.DMs[0].ID != 9 {
		t.Fatalf("profile details = %+v", details)
	}
}

func TestProfilePopupKeyboardNavigationAndClose(t *testing.T) {
	opened := store.ChannelID(0)
	closed := 0
	p := NewProfilePopup(profileDetails{DMs: []profileDM{{ID: 8, Name: "first"}, {ID: 9, Name: "second"}}}, Styles{}, func(id store.ChannelID) {
		opened = id
	}, func() { closed++ })
	if !p.CanFocus() || !p.PreferredFocus() || p.Layout() == nil {
		t.Fatal("profile popup is not a focusable floating widget")
	}
	p.Measure(tui.Size{W: 60, H: 20})
	p.Handle(input.KeyEvent{Key: input.KeyDown})
	p.Handle(input.KeyEvent{Key: input.KeyEnter})
	if opened != 9 {
		t.Fatalf("keyboard opened %d, want second DM 9", opened)
	}
	p.Handle(input.KeyEvent{Key: input.KeyUp})
	p.Handle(input.KeyEvent{Key: input.KeyRune, Rune: 'U'})
	p.Handle(input.KeyEvent{Key: input.KeyEsc})
	if closed != 2 {
		t.Fatalf("close callback count = %d, want U and Esc", closed)
	}
}
