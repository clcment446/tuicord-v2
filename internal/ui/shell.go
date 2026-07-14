package ui

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"awesomeProject/internal/app"
	"awesomeProject/internal/config"
	"awesomeProject/internal/markup"
	"awesomeProject/internal/store"
	"awesomeProject/internal/tui/input"
	"awesomeProject/internal/tui/layout"
	"awesomeProject/internal/tui/screen"
	"awesomeProject/internal/tui/term"
	"awesomeProject/internal/tui/text"
	"awesomeProject/internal/tui/tui"
	"awesomeProject/internal/tui/widget"
)

// Shell is the root widget. It shows the main view and can swap in a
// full-screen overlay (quick switcher or help). Overlays are implemented as a
// tree swap rather than a z-ordered layer, which the toolkit supports directly:
// Children returns whichever subtree is active, so focus, hit-testing, and
// drawing all follow.
type Shell struct {
	mv           *MainView
	app          *app.App
	cfg          config.Config
	styles       Styles
	overlay      tui.Widget // nil = show the main view
	popup        tui.Widget // small interactive layer drawn over current()
	toast        *Toast
	forumPreview *forumPreview
	cancel       context.CancelFunc
	node         layout.Node
}

// NewShell wraps a MainView with overlay handling.
func NewShell(a *app.App, mv *MainView, cfg config.Config, styles Styles, cancel context.CancelFunc) *Shell {
	s := &Shell{mv: mv, app: a, cfg: cfg, styles: styles, cancel: cancel, node: layout.Node{Grow: 1}}
	mv.onNewForumPost = s.openForumPostPrompt
	mv.onForumFilter = s.openForumFilterMenu
	mv.onForumHover = s.setForumHover
	// ForumView is created lazily; MainView invokes this hook when it exists.
	return s
}

func (s *Shell) openForumPostPrompt(title string) {
	forum, ok := s.app.Store().Channel(s.mv.forumID)
	if !ok || forum.Forum == nil {
		return
	}
	p := NewForumPostPrompt(forum.Forum.Tags, s.styles, func(title, body string, tags []uint64) {
		s.app.CreateForumPost(forum.ID, title, body, tags)
	}, s.closeOverlay)
	p.SetTitle(title)
	s.overlay = p
}

func (s *Shell) openForumFilterMenu() {
	if s.mv.forumView == nil {
		return
	}
	forum, ok := s.app.Store().Channel(s.mv.forumID)
	if !ok || forum.Forum == nil {
		return
	}
	items := []widget.MenuItem{{Label: "All tags", OnSelect: func() { s.closePopup(); s.mv.forumView.SetFilter(0) }}}
	for _, value := range forum.Forum.Tags {
		tag := value
		items = append(items, widget.MenuItem{Label: tag.Name, OnSelect: func() { s.closePopup(); s.mv.forumView.SetFilter(tag.ID) }})
	}
	s.showPopupMenu(items, 0, 0)
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
	if s != nil && s.forumPreview != nil && s.overlay == nil && s.popup == nil {
		s.forumPreview.Draw(r)
	}
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
	if mouse, ok := ev.(input.MouseEvent); ok && mouse.Kind == input.MousePress {
		s.forumPreview = nil
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
	if action, ok := s.mv.chat.TakeEntityAction(); ok {
		s.dispatchEntityAction(action)
		return true
	}
	if action, ok := s.mv.chat.TakeComponentAction(); ok {
		s.dispatchComponentAction(action)
		return true
	}
	return handled
}

func (s *Shell) setForumHover(id store.ChannelID, isForum bool) {
	if s == nil || !isForum {
		s.forumPreview = nil
		return
	}
	forum, ok := s.app.Store().Channel(id)
	if !ok || forum.Forum == nil {
		s.forumPreview = nil
		return
	}
	posts := s.app.Store().Threads(id)
	tagByID := make(map[uint64]store.Tag, len(forum.Forum.Tags))
	for _, tag := range forum.Forum.Tags {
		tagByID[tag.ID] = tag
	}
	labels := make([]string, 0, len(posts))
	for _, post := range sortForumPosts(posts, forum.Forum.DefaultSort) {
		labels = append(labels, forumPostLabel(post, tagByID, time.Now(), s.mv.ascii))
	}
	s.forumPreview = &forumPreview{title: forum.Name, labels: labels, style: s.styles}
}

type forumPreview struct {
	title  string
	labels []string
	style  Styles
}

func (p *forumPreview) Draw(r screen.Region) {
	if p == nil || r.Width() < 20 || r.Height() < 4 {
		return
	}
	w := min(52, max(28, r.Width()/3))
	h := min(r.Height()-2, len(p.labels)+3)
	x := r.Width() - w - 1
	box := r.Clip(screen.Rect{X: x, Y: 1, W: w, H: h})
	bg := screen.RGB(28, 31, 38)
	border := p.style.Accent
	box.Fill(screen.Rect{W: w, H: h}, screen.Cell{Content: " ", Style: screen.Style{Bg: bg}})
	for xx := 0; xx < w; xx++ {
		box.Set(xx, 0, screen.Cell{Content: "─", Style: border})
		box.Set(xx, h-1, screen.Cell{Content: "─", Style: border})
	}
	for yy := 0; yy < h; yy++ {
		box.Set(0, yy, screen.Cell{Content: "│", Style: border})
		box.Set(w-1, yy, screen.Cell{Content: "│", Style: border})
	}
	box.Set(0, 0, screen.Cell{Content: "╭", Style: border})
	box.Set(w-1, 0, screen.Cell{Content: "╮", Style: border})
	box.Set(0, h-1, screen.Cell{Content: "╰", Style: border})
	box.Set(w-1, h-1, screen.Cell{Content: "╯", Style: border})
	drawPreviewText(box, 2, 1, p.title+" · forum", w-4, screen.Style{Fg: p.style.Accent.Fg, Bg: bg, Attrs: screen.Bold})
	for i, label := range p.labels {
		if i+2 >= h-1 {
			break
		}
		drawPreviewText(box, 2, i+2, "· "+label, w-4, screen.Style{Fg: p.style.Text.Fg, Bg: bg})
	}
}

func drawPreviewText(r screen.Region, x, y int, value string, width int, style screen.Style) {
	col := x
	for cluster := range text.Clusters(value) {
		if cluster.Width == 0 || col-x+cluster.Width > width {
			break
		}
		r.Set(col, y, screen.Cell{Content: cluster.Text, Style: style})
		col += cluster.Width
	}
}

func (s *Shell) dispatchEntityAction(action markup.Action) {
	id, err := strconv.ParseUint(action.Target, 10, 64)
	if err != nil {
		return
	}
	switch action.Kind {
	case markup.ActionUserMention:
		s.openProfile(store.UserID(id))
	case markup.ActionRoleMention:
		s.openRoleOptions(store.RoleID(id))
	}
}

// openProfile uses the gateway member cache as the reliable offline fallback.
func (s *Shell) openProfile(id store.UserID) {
	m, ok := s.app.Store().Member(s.app.ActiveGuild(), id)
	if !ok {
		s.ShowNotice("Profile", "User "+strconv.FormatUint(uint64(id), 10))
		return
	}
	detail := m.Name + "\nUser ID: " + strconv.FormatUint(uint64(id), 10)
	for _, rid := range m.RoleIDs {
		if r, ok := s.app.Store().Role(s.app.ActiveGuild(), rid); ok {
			detail += "\n@" + r.Name
		}
	}
	text := widget.NewText(detail)
	text.SetStyle(s.styles.Text)
	s.overlay = titled("Profile", text)
}

func (s *Shell) openRoleOptions(id store.RoleID) {
	role, ok := s.app.Store().Role(s.app.ActiveGuild(), id)
	if !ok {
		return
	}
	canManage := s.app.Store().MemberCan(s.app.ActiveGuild(), s.app.SelfID(), store.PermManageRoles)
	items := []widget.MenuItem{
		{Label: fmt.Sprintf("%s · #%06X", role.Name, role.Color), Disabled: true},
		{Label: "Create role…", Disabled: !canManage, OnSelect: func() {
			s.closePopup()
			s.overlay = NewPrompt("Create role", "Role name…", s.styles, func(name string) { s.app.CreateRole(s.app.ActiveGuild(), name) }, s.closeOverlay)
		}},
		{Label: "Rename…", Disabled: !canManage, OnSelect: func() {
			s.closePopup()
			s.overlay = NewPrompt("Rename role", "Role name…", s.styles, func(name string) { s.app.RenameRole(s.app.ActiveGuild(), id, name) }, s.closeOverlay)
		}},
		{Label: "Change color…", Disabled: !canManage, OnSelect: func() {
			s.closePopup()
			s.overlay = NewPrompt("Role color", "RRGGBB", s.styles, func(value string) {
				if color, err := strconv.ParseUint(strings.TrimPrefix(value, "#"), 16, 32); err == nil && color <= 0xffffff {
					s.app.SetRoleColor(s.app.ActiveGuild(), id, uint32(color))
				} else {
					s.ShowNotice("Invalid color", "Use six hexadecimal digits")
				}
			}, s.closeOverlay)
		}},
		{Label: "Toggle hoist", Disabled: !canManage, OnSelect: func() { s.closePopup(); s.app.SetRoleHoist(s.app.ActiveGuild(), id, !role.Hoist) }},
		{Label: "Toggle mentionable", Disabled: !canManage, OnSelect: func() { s.closePopup(); s.app.SetRoleMentionable(s.app.ActiveGuild(), id, !role.Mentionable) }},
		{Separator: true},
		{Label: "Delete…", Danger: true, Disabled: !canManage, OnSelect: func() {
			s.showPopupMenu([]widget.MenuItem{{Label: "Delete — click again", Danger: true, OnSelect: func() { s.closePopup(); s.app.DeleteRole(s.app.ActiveGuild(), id) }}, {Label: "Cancel", OnSelect: s.closePopup}}, 0, 0)
		}},
	}
	s.showPopupMenu(items, 0, 0)
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
	p := NewPicker(st, s.styles, s.app.ActiveGuild(), st.HasNitro(), s.cfg.Nitro.Fake,
		func(text string) { s.mv.InsertIntoComposer(text) },
		s.closeOverlay,
	)
	p.SetGIFSearch(s.app.SearchGIFs)
	p.SetRecentStickers(s.mv.recentStickers())
	p.SetStickerSelect(func(id uint64) {
		s.app.SendSticker(id)
	})
	p.SetStickerRecent(s.mv.recordRecentSticker)
	s.overlay = p
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
	ch, _ := s.app.Store().Channel(msg.ChannelID)
	canThread := ch.Kind == store.ChannelText || ch.Kind == store.ChannelAnnouncement
	isAnnouncement := ch.Kind == store.ChannelAnnouncement
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
		{Label: "Create thread…", Disabled: !canThread, OnSelect: func() {
			s.closePopup()
			s.openThreadPrompt(msg)
		}},
	}
	if isAnnouncement {
		items = append(items, widget.MenuItem{Label: "Publish", Disabled: !own, OnSelect: func() {
			s.closePopup()
			s.app.Publish(msg.ChannelID, msg.ID)
			s.ShowNotice("Publishing", "Message crossposted to followers")
		}})
	}
	items = append(items,
		widget.MenuItem{Separator: true},
		widget.MenuItem{Label: pinLabel, Disabled: !canManage, OnSelect: func() {
			s.closePopup()
			s.app.SetPinned(msg.ChannelID, msg.ID, !msg.Pinned)
		}},
		widget.MenuItem{Label: deleteLabel, Danger: true, Disabled: !canDelete, OnSelect: func() {
			s.openDeleteMessageConfirm(msg, x, y, deleteLabel)
		}},
		widget.MenuItem{Separator: true},
		widget.MenuItem{Label: "Copy message ID", OnSelect: func() {
			s.closePopup()
			id := strconv.FormatUint(uint64(msg.ID), 10)
			if err := term.CopyToClipboard(os.Stdout, id); err != nil {
				s.ShowToast("Clipboard error", err)
				return
			}
			s.ShowNotice("Copied", "Message ID copied")
		}},
	)
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
	if row.Thread {
		s.openThreadMenu(row, x, y)
		return
	}
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

func (s *Shell) openServerSettings(guild store.GuildID) {
	s.overlay = NewServerSettings(s.app.Store(), guild, s.styles, func(c store.Channel) { s.openChannelSettings(guild, c) }, func(r store.Role) { s.openRoleOptions(r.ID) })
}
func (s *Shell) openChannelSettings(guild store.GuildID, c store.Channel) {
	can := s.app.Store().MemberCan(guild, s.app.SelfID(), store.PermManageChannels)
	items := []widget.MenuItem{
		{Label: "Create channel…", Disabled: !can, OnSelect: func() {
			s.closePopup()
			s.overlay = NewPrompt("Create channel", "Channel name…", s.styles, func(name string) { s.app.CreateTextChannel(guild, name) }, s.closeOverlay)
		}},
		{Label: "Rename…", Disabled: !can, OnSelect: func() {
			s.closePopup()
			s.overlay = NewPrompt("Rename channel", "Channel name…", s.styles, func(name string) { s.app.RenameChannel(c.ID, name) }, s.closeOverlay)
		}},
		{Label: "Move up", Disabled: !can || c.Position <= 0, OnSelect: func() { s.closePopup(); s.app.MoveChannel(guild, c.ID, c.Position-1) }},
		{Label: "Move down", Disabled: !can, OnSelect: func() { s.closePopup(); s.app.MoveChannel(guild, c.ID, c.Position+1) }},
		{Separator: true}, {Label: "Delete…", Danger: true, Disabled: !can, OnSelect: func() {
			s.showPopupMenu([]widget.MenuItem{{Label: "Delete — click again", Danger: true, OnSelect: func() { s.closePopup(); s.app.DeleteChannel(c.ID) }}, {Label: "Cancel", OnSelect: s.closePopup}}, 0, 0)
		}},
	}
	s.showPopupMenu(items, 0, 0)
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
			{Label: "Server settings…", OnSelect: func() { s.closePopup(); s.openServerSettings(row.GuildID) }},
			{Separator: true},
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

// openThreadMenu shows the sidebar context menu for a thread sub-item:
// join/leave, archive/unarchive (gated by MANAGE_THREADS or ownership), and copy
// id.
func (s *Shell) openThreadMenu(row store.ChannelRow, x, y int) {
	c, _ := s.app.Store().Channel(row.ChannelID)
	joined := c.Thread != nil && c.Thread.Joined
	archived := c.Thread != nil && c.Thread.Archived
	joinLabel := "Join thread"
	if joined {
		joinLabel = "Leave thread"
	}
	archiveLabel := "Archive thread"
	if archived {
		archiveLabel = "Unarchive thread"
	}
	canManage := s.canManageThread(c)
	items := []widget.MenuItem{
		{Label: joinLabel, OnSelect: func() {
			s.closePopup()
			if joined {
				s.app.LeaveThread(row.ChannelID)
			} else {
				s.app.JoinThread(row.ChannelID)
			}
		}},
		{Label: archiveLabel, Disabled: !canManage, OnSelect: func() {
			s.closePopup()
			s.app.SetThreadArchived(row.ChannelID, !archived)
		}},
		{Separator: true},
		{Label: "Copy channel ID", OnSelect: func() {
			s.closePopup()
			s.copyID("Channel ID copied", uint64(row.ChannelID))
		}},
	}
	s.showPopupMenu(items, x, y)
}

// openThreadPrompt asks for a name, then creates a message-anchored thread.
func (s *Shell) openThreadPrompt(msg store.Message) {
	s.overlay = NewPrompt("New thread", "Thread name…", s.styles,
		func(name string) {
			s.app.CreateThreadFromMessage(msg.ChannelID, msg.ID, name)
		},
		s.closeOverlay,
	)
}

// canManageThread reports whether the account may archive/unarchive a thread:
// it owns the thread, or holds MANAGE_THREADS in the guild.
func (s *Shell) canManageThread(c store.Channel) bool {
	if c.Thread != nil && c.Thread.OwnerID != 0 && c.Thread.OwnerID == s.app.SelfID() {
		return true
	}
	if c.GuildID == 0 || c.GuildID == app.DirectMessagesGuildID {
		return true
	}
	return s.app.Store().MemberCan(c.GuildID, s.app.SelfID(), store.PermManageThreads)
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
