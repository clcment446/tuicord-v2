package ui

import (
	"context"
	"os"
	"strconv"

	"awesomeProject/internal/app"
	"awesomeProject/internal/config"
	"awesomeProject/internal/store"
	"awesomeProject/internal/tui/input"
	"awesomeProject/internal/tui/layout"
	"awesomeProject/internal/tui/screen"
	"awesomeProject/internal/tui/term"
	"awesomeProject/internal/tui/tui"
	"awesomeProject/internal/tui/widget"
)

// Shell is the root widget. It shows the main view and can swap in a
// full-screen overlay (quick switcher or help). Overlays are implemented as a
// tree swap rather than a z-ordered layer, which the toolkit supports directly:
// Children returns whichever subtree is active, so focus, hit-testing, and
// drawing all follow.
type Shell struct {
	mv      *MainView
	app     *app.App
	cfg     config.Config
	styles  Styles
	overlay tui.Widget // nil = show the main view
	popup   tui.Widget // small interactive layer drawn over current()
	toast   *Toast
	cancel  context.CancelFunc
	node    layout.Node
}

// NewShell wraps a MainView with overlay handling.
func NewShell(a *app.App, mv *MainView, cfg config.Config, styles Styles, cancel context.CancelFunc) *Shell {
	return &Shell{mv: mv, app: a, cfg: cfg, styles: styles, cancel: cancel, node: layout.Node{Grow: 1}}
}

func (s *Shell) current() tui.Widget {
	if s.overlay != nil {
		return s.overlay
	}
	return s.mv.Root
}

// Children exposes the active subtree.
func (s *Shell) Children() []tui.Widget { return []tui.Widget{s.current()} }

// Measure delegates to the active subtree.
func (s *Shell) Measure(avail tui.Size) tui.Size { return s.current().Measure(avail) }

// Layout returns the shell node wrapping the active subtree.
func (s *Shell) Layout() *layout.Node {
	s.node.Children = []*layout.Node{s.current().Layout()}
	return &s.node
}

// Draw is a no-op; children draw themselves.
func (s *Shell) Draw(screen.Region) {}

func (s *Shell) DrawOverlay(r screen.Region) {
	if s != nil && s.popup != nil {
		s.popup.Measure(tui.Size{W: r.Width(), H: r.Height()})
		s.popup.Draw(r)
	}
	if s != nil && s.toast != nil {
		s.toast.Draw(r)
	}
}

// Handle routes global shortcuts and overlay dismissal, delegating everything
// else to the active subtree.
func (s *Shell) Handle(ev tui.Event) bool {
	key, isKey := ev.(input.KeyEvent)

	if s.toast != nil && s.toast.Handle(ev) {
		if s.toast.wantsDismiss(ev) {
			s.toast = nil
		}
		return true
	}

	if s.popup != nil {
		return s.popup.Handle(ev)
	}

	if s.overlay != nil {
		// The help overlay has no focusable widgets, so its keys arrive here;
		// the quick switcher's dismissal (Esc) arrives here via root fallback.
		if isKey && (keyMatches(key, s.cfg.Keys.Help) || key.Key == input.KeyEsc) {
			s.closeOverlay()
			return true
		}
		return s.overlay.Handle(ev)
	}

	if mouse, ok := ev.(input.MouseEvent); ok && mouse.Kind == input.MousePress && mouse.Btn == input.ButtonRight {
		if msg, ok := s.mv.chat.TakeContextMessage(); ok {
			s.openMessageMenu(msg, mouse.X, mouse.Y)
			return true
		}
		if row, ok := s.mv.ChannelContext(); ok {
			s.openChannelMenu(row, mouse.X, mouse.Y)
			return true
		}
		if row, ok := s.mv.GuildContext(); ok {
			s.openGuildMenu(row, mouse.X, mouse.Y)
			return true
		}
	}

	if isKey {
		switch {
		case key.Key == input.KeyEsc && s.mv.CancelComposerMode():
			return true
		case keyMatches(key, s.cfg.Keys.QuickSwitcher):
			s.openQuickSwitcher()
			return true
		case keyMatches(key, s.cfg.Keys.Picker):
			s.openPicker()
			return true
		case keyMatches(key, s.cfg.Keys.Help):
			s.overlay = NewHelpOverlay(s.cfg)
			return true
		}
	}
	handled := s.mv.Root.Handle(ev)
	if action, ok := s.mv.chat.TakeComponentAction(); ok {
		s.dispatchComponentAction(action)
		return true
	}
	return handled
}

// dispatchComponentAction forwards a chat component activation to Discord.
// Link buttons have no interaction to submit; their URL goes to the clipboard.
func (s *Shell) dispatchComponentAction(action ComponentAction) {
	if action.Kind == store.ComponentLinkButton || (action.URL != "" && action.CustomID == "") {
		if err := term.CopyToClipboard(os.Stdout, action.URL); err != nil {
			s.ShowToast("Clipboard error", err)
			return
		}
		s.ShowNotice("Link copied", action.URL)
		return
	}
	switch action.Kind {
	case store.ComponentButton, store.ComponentSelect:
		s.app.SubmitComponent(app.ComponentSubmit{
			Message:       action.Message,
			ComponentType: action.RawType,
			CustomID:      action.CustomID,
			Values:        action.Values,
		})
	}
}

func (s *Shell) openQuickSwitcher() {
	s.overlay = NewQuickSwitcher(s.app.Store(), s.styles,
		func(guild store.GuildID, channel store.ChannelID) {
			s.app.SetActive(guild, channel)
			s.mv.RefreshChannels()
		},
		s.closeOverlay,
	)
}

// openPicker opens the emoji/sticker picker overlay over the composer. Chosen
// entries are inserted at the composer cursor.
func (s *Shell) openPicker() {
	st := s.app.Store()
	s.overlay = NewPicker(st, s.styles, s.app.ActiveGuild(), st.HasNitro(), s.cfg.Nitro.Fake,
		func(text string) { s.mv.InsertIntoComposer(text) },
		s.closeOverlay,
	)
}

func (s *Shell) openMessageMenu(msg store.Message, x, y int) {
	own := msg.AuthorID != 0 && msg.AuthorID == s.app.SelfID()
	canManage := s.canManageMessages(msg.ChannelID)
	canDelete := own || canManage
	deleteLabel := "Delete"
	if !own {
		deleteLabel = "Force delete"
	}
	pinLabel := "Pin"
	if msg.Pinned {
		pinLabel = "Unpin"
	}
	items := []widget.MenuItem{
		{Label: "Reply", OnSelect: func() {
			s.closePopup()
			s.mv.BeginReply(msg, true)
		}},
		{Label: "Reply (no mention)", OnSelect: func() {
			s.closePopup()
			s.mv.BeginReply(msg, false)
		}},
		{Label: "Edit", Disabled: !own, OnSelect: func() {
			s.closePopup()
			s.mv.BeginEdit(msg)
		}},
		{Separator: true},
		{Label: pinLabel, Disabled: !canManage, OnSelect: func() {
			s.closePopup()
			s.app.SetPinned(msg.ChannelID, msg.ID, !msg.Pinned)
		}},
		{Label: deleteLabel, Danger: true, Disabled: !canDelete, OnSelect: func() {
			s.openDeleteMessageConfirm(msg, x, y, deleteLabel)
		}},
		{Separator: true},
		{Label: "Copy message ID", OnSelect: func() {
			s.closePopup()
			id := strconv.FormatUint(uint64(msg.ID), 10)
			if err := term.CopyToClipboard(os.Stdout, id); err != nil {
				s.ShowToast("Clipboard error", err)
				return
			}
			s.ShowNotice("Copied", "Message ID copied")
		}},
	}
	menu := widget.NewMenu(items)
	s.styleMenu(menu)
	menu.SetAnchor(x, y)
	menu.OnDismiss(s.closePopup)
	s.popup = menu
}

func (s *Shell) openDeleteMessageConfirm(msg store.Message, x, y int, label string) {
	items := []widget.MenuItem{
		{Label: label + " — click again", Danger: true, OnSelect: func() {
			s.closePopup()
			s.app.DeleteMessage(msg.ChannelID, msg.ID)
		}},
		{Label: "Cancel", OnSelect: s.closePopup},
	}
	menu := widget.NewMenu(items)
	s.styleMenu(menu)
	menu.SetAnchor(x, y)
	menu.OnDismiss(s.closePopup)
	s.popup = menu
}

// openChannelMenu shows the sidebar context menu for a channel or category:
// pin/unpin and copy id for channels, collapse/expand for categories.
func (s *Shell) openChannelMenu(row store.ChannelRow, x, y int) {
	var items []widget.MenuItem
	if row.Category {
		label := "Collapse"
		if row.Collapsed {
			label = "Expand"
		}
		items = []widget.MenuItem{{Label: label, OnSelect: func() {
			s.closePopup()
			s.mv.ToggleCollapseCategory(row.ChannelID)
		}}}
	} else {
		pinLabel := "Pin channel"
		if s.mv.IsChannelPinned(row.ChannelID) {
			pinLabel = "Unpin channel"
		}
		items = []widget.MenuItem{
			{Label: pinLabel, OnSelect: func() {
				s.closePopup()
				s.mv.TogglePinChannel(row.ChannelID)
			}},
			{Separator: true},
			{Label: "Copy channel ID", OnSelect: func() {
				s.closePopup()
				s.copyID("Channel ID copied", uint64(row.ChannelID))
			}},
		}
	}
	s.showPopupMenu(items, x, y)
}

// openGuildMenu shows the sidebar context menu for a guild or folder: pin/unpin
// and copy id for guilds, collapse/expand for folders.
func (s *Shell) openGuildMenu(row store.GuildRow, x, y int) {
	var items []widget.MenuItem
	if row.Folder {
		label := "Collapse"
		if row.Collapsed {
			label = "Expand"
		}
		items = []widget.MenuItem{{Label: label, OnSelect: func() {
			s.closePopup()
			s.mv.ToggleCollapseFolder(row.FolderID)
		}}}
	} else {
		pinLabel := "Pin server"
		if s.mv.IsGuildPinned(row.GuildID) {
			pinLabel = "Unpin server"
		}
		items = []widget.MenuItem{
			{Label: pinLabel, OnSelect: func() {
				s.closePopup()
				s.mv.TogglePinGuild(row.GuildID)
			}},
			{Separator: true},
			{Label: "Copy server ID", OnSelect: func() {
				s.closePopup()
				s.copyID("Server ID copied", uint64(row.GuildID))
			}},
		}
	}
	s.showPopupMenu(items, x, y)
}

// copyID places a snowflake on the clipboard and reports the outcome.
func (s *Shell) copyID(notice string, id uint64) {
	if err := term.CopyToClipboard(os.Stdout, strconv.FormatUint(id, 10)); err != nil {
		s.ShowToast("Clipboard error", err)
		return
	}
	s.ShowNotice("Copied", notice)
}

// showPopupMenu styles, anchors, and installs a context menu as the active popup.
func (s *Shell) showPopupMenu(items []widget.MenuItem, x, y int) {
	menu := widget.NewMenu(items)
	s.styleMenu(menu)
	menu.SetAnchor(x, y)
	menu.OnDismiss(s.closePopup)
	s.popup = menu
}

func (s *Shell) styleMenu(menu *widget.Menu) {
	if menu == nil {
		return
	}
	menu.SetStyle(s.styles.Text)
	menu.SetSelectedStyle(s.styles.Accent)
	menu.SetBorderStyle(s.styles.Border)
	danger := s.styles.Error
	danger.Attrs |= screen.Bold
	menu.SetDangerStyle(danger)
	menu.SetDisabledStyle(s.styles.Muted)
	menu.SetKeyStyle(s.styles.Muted)
}

func (s *Shell) canManageMessages(channel store.ChannelID) bool {
	if s == nil || s.app == nil {
		return false
	}
	if c, ok := s.app.Store().Channel(channel); ok {
		if c.GuildID == app.DirectMessagesGuildID || c.GuildID == 0 {
			return true
		}
		return s.app.Store().MemberCan(c.GuildID, s.app.SelfID(), store.PermManageMessages)
	}
	return false
}

func (s *Shell) closeOverlay() { s.overlay = nil }

func (s *Shell) closePopup() { s.popup = nil }

// ShowToast displays a dismissible error popup over the active view.
func (s *Shell) ShowToast(title string, err error) {
	if s == nil || err == nil {
		return
	}
	s.toast = NewToast(title, err.Error(), s.styles)
}

// ShowNotice displays a short dismissible informational popup.
func (s *Shell) ShowNotice(title, detail string) {
	if s == nil {
		return
	}
	s.toast = NewToast(title, detail, s.styles)
}

// Toast returns the current popup, if any.
func (s *Shell) Toast() *Toast {
	if s == nil {
		return nil
	}
	return s.toast
}
