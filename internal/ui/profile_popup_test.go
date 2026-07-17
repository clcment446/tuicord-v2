package ui

import (
	"strings"
	"testing"

	"awesomeProject/internal/store"
	"awesomeProject/internal/tui/input"
	"awesomeProject/internal/tui/screen"
	"awesomeProject/internal/tui/tui"
)

func TestProfilePopupRendersIdentityRolesAndSharedDM(t *testing.T) {
	opened := store.ChannelID(0)
	p := NewProfilePopup(profileDetails{
		ID: 42, Name: "Alice", Username: "alice", Nick: "ali",
		Roles: []string{"Admin", "Member"},
		DMs:   []profileDM{{ID: 9, Name: "alice"}},
	}, Styles{}, func(id store.ChannelID) { opened = id }, nil)
	buf := screen.NewBuffer(60, 20)
	p.Draw(buf.Clip(buf.Bounds()))
	contents := rowsText(buf)
	for _, want := range []string{"Alice", "alice", "ali", "42", "Admin", "Open DM"} {
		if !strings.Contains(contents, want) {
			t.Fatalf("profile missing %q:\n%s", want, contents)
		}
	}
	box := p.box(60, 20)
	if !p.Handle(input.MouseEvent{Kind: input.MousePress, Btn: input.ButtonLeft, X: box.X + 2, Y: box.Y + p.dmRow}) {
		t.Fatal("DM row click was not handled")
	}
	if opened != 9 {
		t.Fatalf("opened DM = %d, want 9", opened)
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
	if details.Username != "alice" || details.Nick != "ali" || len(details.Roles) != 1 || details.Roles[0] != "Admin" || len(details.DMs) != 1 || details.DMs[0].ID != 9 {
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
