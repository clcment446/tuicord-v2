// Package ui builds the tuicord widget tree over the internal/tui toolkit.
//
// The main view is a four-panel row — guild rail, channel sidebar, chat column,
// and members panel — with the chat column split into the message view and the
// always-live composer. Both sidebars are drag-resizable (via widget.Split) and
// the members panel auto-hides on narrow terminals.
package ui

import (
	"image"
	"os"
	"strconv"
	"strings"

	"awesomeProject/internal/app"
	"awesomeProject/internal/config"
	"awesomeProject/internal/markup"
	"awesomeProject/internal/media"
	"awesomeProject/internal/store"
	"awesomeProject/internal/tui/screen"
	"awesomeProject/internal/tui/term"
	"awesomeProject/internal/tui/tui"
	"awesomeProject/internal/tui/widget"
	"awesomeProject/internal/uistate"
)

// Sidebar row glyphs: collapse arrows and the local-pin marker.
const (
	glyphExpanded  = "▾"
	glyphCollapsed = "▸"
	glyphPinned    = "★"
)

// MainView owns the widget tree and the index→ID maps needed to translate list
// selections back into store identifiers.
type MainView struct {
	app    *app.App
	cfg    config.Config
	styles Styles

	guildList        *widget.ItemList
	channelList      *widget.ItemList
	memberList       *widget.ItemList
	chat             *ChatView
	chatBorder       *widget.Border
	composerBorder   *widget.Border
	composerStatus   *widget.Text
	composer         *widget.TextInput
	composerMode     composerMode
	composerTarget   store.Message
	replyMention     bool
	composerReadOnly bool

	forumView       *ForumView
	forumPreviewID  store.ChannelID
	forumPreview    *ChatView
	forumPreviewBox *widget.Border
	forumActive     bool
	forumID         store.ChannelID
	onNewForumPost  func(string)
	onForumFilter   func()
	onForumHover    func(store.ChannelID, bool)

	state     *uistate.State
	reportErr func(error)
	// ascii forces ASCII-only sidebar glyphs (config or NO_COLOR environment).
	ascii bool

	guildRows   []store.GuildRow
	channelRows []store.ChannelRow

	// Root is the composed widget passed to App.Run.
	Root tui.Widget
}

type composerMode int

const (
	composerNormal composerMode = iota
	composerReply
	composerEdit
)

// NewMainView assembles the four-panel layout and wires selection callbacks.
func NewMainView(a *app.App, cfg config.Config, styles Styles) *MainView {
	state, _ := uistate.Load()
	if state == nil {
		state = &uistate.State{}
	}
	mv := &MainView{app: a, cfg: cfg, styles: styles, state: state,
		ascii: cfg.Display.ASCII || os.Getenv("NO_COLOR") != ""}

	mv.guildList = widget.NewItemList(nil)
	mv.guildList.SetStyle(styles.Text)
	mv.guildList.SetSelectedStyle(styles.Accent)
	mv.guildList.OnSelect(mv.onGuildSelected)

	mv.channelList = widget.NewItemList(nil)
	mv.channelList.SetStyle(styles.Text)
	mv.channelList.SetSelectedStyle(styles.Accent)
	mv.channelList.SetBadgeStyle(styles.Accent)
	mv.channelList.OnSelect(mv.onChannelSelected)
	mv.channelList.OnHover(mv.onChannelHovered)

	mv.memberList = widget.NewItemList(nil)
	mv.memberList.SetStyle(styles.Text)

	mv.chat = NewChatView(a.Store(), a.ActiveChannel, mv.resolver, styles)
	mv.chat.OnReachTop(func() { a.LoadOlderHistory(a.ActiveChannel()) })
	mediaCfg := chatMediaConfig()
	if fetcher := newChatMediaFetcher(mediaCfg); fetcher != nil {
		mv.chat.SetMedia(fetcher, mediaCfg, a.Post)
	}

	mv.composerStatus = widget.NewText("")
	mv.composerStatus.SetStyle(styles.Muted)
	mv.composerStatus.SetWrap(false)

	mv.composer = widget.NewTextInput("Message")
	mv.composer.SetStyle(styles.Text)
	mv.composer.OnSubmit(mv.onSend)

	mv.Root = mv.compose()
	return mv
}

func (mv *MainView) compose() tui.Widget {
	guildRail := mv.titled("Servers", mv.guildList)
	channels := mv.titled("Channels", mv.channelList)
	members := mv.titled("Members", mv.memberList)

	composerArea := widget.Column(mv.composerStatus, mv.composer)
	composerArea.Children()[0].Layout().Basis = 1
	composerArea.Children()[0].Layout().Grow = 0
	composerArea.Children()[1].Layout().Basis = 1
	composerArea.Children()[1].Layout().Grow = 0

	mv.chatBorder = mv.titled("Messages", mv.chat)
	mv.composerBorder = mv.titled("Composer", composerArea)
	chatColumn := widget.Column(
		mv.chatBorder,
		mv.composerBorder,
	)
	// Chat grows, composer is a fixed 3-row box (border + one line).
	chatColumn.Children()[0].Layout().Grow = 1
	composerNode := chatColumn.Children()[1].Layout()
	composerNode.Basis = 4
	composerNode.Grow = 0

	// chat | members split so members stays beside the messages/composer stack.
	chatAndMembers := widget.NewSplit(chatColumn, members).
		Basis(mv.termWidthMinusMembers()).
		MinSecond(mv.cfg.Layout.MembersWidth).
		CollapsibleSecond().
		Vertical()
	membersNode := members.Layout()
	membersNode.HideBelow = mv.cfg.Layout.MembersHideBelow

	channelsAndRest := widget.NewSplit(channels, chatAndMembers).
		Basis(mv.cfg.Layout.ChannelsWidth).
		MinFirst(12).
		MaxFirst(40).
		CollapsibleFirst()

	root := widget.NewSplit(guildRail, channelsAndRest).
		Basis(mv.cfg.Layout.GuildsWidth).
		MinFirst(3).
		MaxFirst(24).
		CollapsibleFirst()
	return root
}

// termWidthMinusMembers biases the initial split so members takes MembersWidth.
func (mv *MainView) termWidthMinusMembers() int {
	// Split basis is the first child's width; a large value lets chat take the
	// remainder while members keeps its configured width via its own Min.
	return 200
}

func newChatMediaFetcher(cfg media.Config) *media.Fetcher {
	cache, err := media.NewCache(0, "")
	if err != nil {
		return nil
	}
	return &media.Fetcher{Cache: cache, MaxPixels: chatMediaPixelBudget(cfg)}
}

// chatMediaConfig resolves the media settings for this terminal, filling in the
// cell pixel size when the terminal reports one. A terminal that reports
// nothing (tmux, some emulators) leaves the zero values, and media.Config
// substitutes conventional defaults.
func chatMediaConfig() media.Config {
	cfg := media.DefaultConfig()
	if sz, err := term.ProbeSize(); err == nil {
		if w, h, ok := sz.CellPixels(); ok {
			cfg.CellPixelWidth, cfg.CellPixelHeight = w, h
		}
	}
	return cfg
}

// chatMediaPixelBudget is the largest pixel size an inline media block can
// occupy: the full terminal width, and MaxHeightCells tall. Fetched images are
// downscaled to this once, rather than pushing full-resolution pixels through
// the Kitty encoder on every cache miss.
func chatMediaPixelBudget(cfg media.Config) image.Point {
	cellW, cellH := cfg.CellPixels()
	rows := cfg.MaxHeightCells
	if rows <= 0 {
		rows = media.DefaultConfig().MaxHeightCells
	}
	// A generous column budget: the height cap is the binding constraint for
	// inline media, and over-wide sources are rare.
	const maxCols = 200
	return image.Point{X: maxCols * cellW, Y: rows * cellH}
}

// Refresh repopulates the guild and channel lists from the store. Call on the
// UI goroutine (e.g. from App.OnReady) after state changes.
func (mv *MainView) Refresh() {
	mv.rebuildGuilds()
	switch {
	case mv.guildRowIndex(mv.app.ActiveGuild()) >= 0:
		mv.refreshChannels()
		mv.refreshMembers(mv.app.ActiveGuild())
	case mv.app.ActiveGuild() == 0 && mv.firstGuildRow() >= 0:
		i := mv.firstGuildRow()
		mv.guildList.SetSelectedSilent(i)
		mv.onGuildSelected(i)
	default:
		mv.refreshChannels()
	}
}

// rebuildGuilds rebuilds the guild rail rows (folders + guilds + pins) and keeps
// the selection on the active guild.
func (mv *MainView) rebuildGuilds() {
	st := mv.app.Store()
	mv.guildRows = store.OrderGuilds(st.GuildFolders(), st.Guilds(), mv.pinnedGuildIDs(), mv.state.CollapsedFolderSet())
	items := make([]widget.Item, len(mv.guildRows))
	for i, row := range mv.guildRows {
		items[i] = mv.guildItem(row)
	}
	mv.guildList.SetItems(items)
	if i := mv.guildRowIndex(mv.app.ActiveGuild()); i >= 0 {
		mv.guildList.SetSelectedSilent(i)
	}
}

func (mv *MainView) guildItem(row store.GuildRow) widget.Item {
	if row.Folder {
		arrow := glyphExpanded
		if row.Collapsed {
			arrow = glyphCollapsed
		}
		return widget.Item{Label: arrow + " " + row.Name, Style: mv.headerStyle()}
	}
	label := row.Name
	switch {
	case row.Pinned:
		label = glyphPinned + " " + row.Name
	case row.Indent:
		label = "  " + row.Name
	}
	return widget.Item{Label: label}
}

func (mv *MainView) guildRowIndex(id store.GuildID) int {
	if id == 0 {
		return -1
	}
	for i, row := range mv.guildRows {
		if !row.Folder && row.GuildID == id {
			return i
		}
	}
	return -1
}

func (mv *MainView) firstGuildRow() int {
	for i, row := range mv.guildRows {
		if !row.Folder {
			return i
		}
	}
	return -1
}

func (mv *MainView) onGuildSelected(index int) {
	if index < 0 || index >= len(mv.guildRows) {
		return
	}
	row := mv.guildRows[index]
	if row.Folder {
		mv.state.ToggleCollapsedFolder(row.FolderID)
		mv.persist()
		mv.rebuildGuilds()
		return
	}
	guild := row.GuildID
	mv.showChat() // leave any forum view from the previous guild
	mv.app.SetActive(guild, 0)
	mv.app.LoadRoles(guild)
	mv.app.LoadChannels(guild)
	mv.refreshChannels()
	// Auto-select the first navigable channel.
	if i := mv.firstNavigableChannel(); i >= 0 {
		mv.channelList.SetSelectedSilent(i)
		mv.onChannelSelected(i)
	}
	mv.refreshMembers(guild)
}

// RefreshChannels rebuilds the channel list (and its unread badges) from the
// store. Safe to call on the UI goroutine, e.g. from App.OnChange.
func (mv *MainView) RefreshChannels() {
	mv.refreshChannels()
	mv.refreshForum()
}

func (mv *MainView) refreshChannels() {
	st := mv.app.Store()
	guild := mv.app.ActiveGuild()
	channels := st.Channels(guild)
	mv.channelRows = store.GroupChannels(channels, mv.pinnedChannelIDs(), mv.collapsedCategorySet())
	items := make([]widget.Item, len(mv.channelRows))
	for i, row := range mv.channelRows {
		items[i] = mv.channelItem(row)
	}
	mv.channelList.SetItems(items)
	switch {
	case mv.channelRowIndex(mv.app.ActiveChannel()) >= 0:
		mv.channelList.SetSelectedSilent(mv.channelRowIndex(mv.app.ActiveChannel()))
	case mv.app.ActiveChannel() == 0 && mv.firstNavigableChannel() >= 0:
		i := mv.firstNavigableChannel()
		mv.channelList.SetSelectedSilent(i)
		mv.onChannelSelected(i)
	}
	mv.updateChannelChrome()
}

func (mv *MainView) channelItem(row store.ChannelRow) widget.Item {
	if row.Category {
		arrow := glyphExpanded
		if row.Collapsed {
			arrow = glyphCollapsed
		}
		return widget.Item{Label: arrow + " " + row.Name, Style: mv.headerStyle()}
	}
	label := channelPrefixBadge(row.Kind, mv.isRulesChannel(row.ChannelID), mv.ascii) + row.Name
	switch {
	case row.Pinned:
		label = glyphPinned + " " + label
	default:
		label = strings.Repeat("  ", row.Depth) + label
	}
	badge := ""
	if row.Navigable() {
		badge = unreadBadge(mv.app.Store().Unread(row.ChannelID))
	}
	return widget.Item{Label: label, Badge: badge}
}

// isRulesChannel reports whether id is the active guild's designated rules
// channel, which the sidebar marks with a rules badge and the composer renders
// read-only.
func (mv *MainView) isRulesChannel(id store.ChannelID) bool {
	if id == 0 {
		return false
	}
	g, ok := mv.app.Store().Guild(mv.app.ActiveGuild())
	return ok && g.RulesChannelID != 0 && g.RulesChannelID == id
}

func (mv *MainView) channelRowIndex(id store.ChannelID) int {
	if id == 0 {
		return -1
	}
	for i, row := range mv.channelRows {
		if row.Navigable() && row.ChannelID == id {
			return i
		}
	}
	return -1
}

func (mv *MainView) firstNavigableChannel() int {
	for i, row := range mv.channelRows {
		if row.Navigable() {
			return i
		}
	}
	return -1
}

func (mv *MainView) onChannelSelected(index int) {
	if index < 0 || index >= len(mv.channelRows) {
		return
	}
	row := mv.channelRows[index]
	if row.Category {
		mv.state.ToggleCollapsedCategory(uint64(row.ChannelID))
		mv.persist()
		mv.refreshChannels()
		return
	}
	if !row.Navigable() {
		return
	}
	mv.app.SetActive(mv.app.ActiveGuild(), row.ChannelID)
	if row.Kind == store.ChannelForum {
		mv.openForum(row.ChannelID)
		return
	}
	mv.showChat()
	mv.app.LoadHistory(row.ChannelID, 50)
	mv.updateChannelChrome()
}

func (mv *MainView) onChannelHovered(index int) {
	if mv == nil || index < 0 || index >= len(mv.channelRows) || mv.onForumHover == nil {
		return
	}
	row := mv.channelRows[index]
	mv.onForumHover(row.ChannelID, row.Kind == store.ChannelForum && row.Navigable())
}

// updateChannelChrome refreshes the chat-panel breadcrumb title and the
// composer's read-only state for the currently active channel.
func (mv *MainView) updateChannelChrome() {
	if mv.forumActive {
		// The forum view manages its own chrome; don't overwrite it.
		return
	}
	id := mv.app.ActiveChannel()
	if mv.chatBorder != nil {
		mv.chatBorder.SetTitle(mv.channelBreadcrumb(id))
	}
	ro := mv.channelReadOnly(id)
	mv.composerReadOnly = ro
	if mv.composer != nil {
		mv.composer.SetReadOnly(ro)
	}
	if mv.composerBorder != nil {
		title := "Composer"
		if ro {
			title = "Composer · read-only"
		}
		mv.composerBorder.SetTitle(title)
	}
	mv.updateComposerStatus()
}

// channelBreadcrumb builds the chat-panel title for the active channel. Threads
// render "parent ⤷ thread"; everything else renders "<badge> name".
func (mv *MainView) channelBreadcrumb(id store.ChannelID) string {
	c, ok := mv.app.Store().Channel(id)
	if !ok {
		return "Messages"
	}
	if c.Kind == store.ChannelThread && c.ParentID != 0 {
		if parent, ok := mv.app.Store().Channel(c.ParentID); ok {
			pb := channelBadge(parent.Kind, mv.isRulesChannel(parent.ID), mv.ascii)
			arrow := "⤷"
			if mv.ascii {
				arrow = ">"
			}
			return pb + " " + parent.Name + " " + arrow + " " + c.Name
		}
	}
	return channelBadge(c.Kind, mv.isRulesChannel(id), mv.ascii) + " " + c.Name
}

// channelReadOnly reports whether the composer should be disabled for a channel:
// no SEND_MESSAGES permission (rules channels, most announcement channels), or
// an archived thread. DMs and forums are never read-only here (forums use the
// post composer instead).
func (mv *MainView) channelReadOnly(id store.ChannelID) bool {
	if id == 0 {
		return false
	}
	c, ok := mv.app.Store().Channel(id)
	if !ok {
		return false
	}
	if c.GuildID == app.DirectMessagesGuildID || c.GuildID == 0 {
		return false
	}
	if c.Kind == store.ChannelForum {
		return false
	}
	if !mv.app.Store().ChannelCan(c.GuildID, mv.app.SelfID(), id, store.PermSendMessages) {
		return true
	}
	if c.Kind == store.ChannelThread && c.Thread != nil && c.Thread.Archived {
		return true
	}
	return false
}

// showChat restores the chat view as the chat-column body when leaving a forum
// post-list view.
func (mv *MainView) showChat() {
	mv.forumActive = false
	if mv.chatBorder != nil && mv.chatBorder.Child() != mv.chat {
		mv.chatBorder.SetChild(mv.chat)
	}
	if mv.composer != nil {
		mv.composer.SetPlaceholder("Message")
	}
}

// openForum swaps the chat column to a forum post-list view: the composer turns
// into a "new post" title box and the list shows the forum's posts.
func (mv *MainView) openForum(id store.ChannelID) {
	forum, ok := mv.app.Store().Channel(id)
	if !ok || forum.Kind != store.ChannelForum {
		return
	}
	mv.forumActive = true
	mv.forumID = id
	mv.app.LoadForumMetadata(id)
	mv.app.LoadArchivedThreads(id)
	if mv.forumView == nil {
		mv.forumView = NewForumView(mv.styles, mv.ascii, mv.onOpenPost, mv.app.LoadArchivedThreads)
		mv.forumView.onFilterCycle = mv.refreshForum
		mv.forumView.onFilterMenu = mv.onForumFilter
		mv.forumView.onNavigate = mv.navigateForum
		mv.forumPreview = NewChatView(mv.app.Store(), func() store.ChannelID { return mv.forumPreviewID }, mv.resolver, mv.styles)
		mv.forumPreviewBox = mv.titled("Post preview", mv.forumPreview)
		mv.forumView.SetPreview(mv.forumPreviewBox)
		mv.forumView.onPreview = func(post store.ChannelID) {
			mv.forumPreviewID = post
			mv.app.LoadHistory(post, 50)
			mv.forumPreviewBox.SetTitle("Post preview")
		}
	}
	if mv.chatBorder != nil {
		mv.chatBorder.SetChild(mv.forumView)
		mv.chatBorder.SetTitle(channelBadge(forum.Kind, false, mv.ascii) + " " + forum.Name)
	}
	// The composer creates posts here rather than sending chat messages.
	mv.composerReadOnly = false
	if mv.composer != nil {
		mv.composer.SetReadOnly(false)
		mv.composer.SetPlaceholder("New post title…")
	}
	if mv.composerBorder != nil {
		mv.composerBorder.SetTitle("New post")
	}
	mv.updateComposerStatus()
	mv.refreshForum()
}

// navigateForum switches to the adjacent forum when the forum post list is at
// either edge. This keeps normal Up/Down post navigation intact while making
// adjacent forums reachable without leaving the forum view.
func (mv *MainView) navigateForum(delta int) {
	if delta == 0 || mv == nil {
		return
	}
	forums := make([]int, 0)
	for i, row := range mv.channelRows {
		if row.Kind == store.ChannelForum && row.Navigable() {
			forums = append(forums, i)
		}
	}
	current := -1
	for i, row := range forums {
		if mv.channelRows[row].ChannelID == mv.forumID {
			current = i
			break
		}
	}
	if current < 0 || len(forums) < 2 {
		return
	}
	next := (current + delta + len(forums)) % len(forums)
	mv.channelList.SetSelectedSilent(forums[next])
	mv.onChannelSelected(forums[next])
}

// refreshForum repopulates the forum post list from the store. Safe to call from
// OnChange so new posts and archived pages appear as they load.
func (mv *MainView) refreshForum() {
	if !mv.forumActive || mv.forumView == nil {
		return
	}
	forum, ok := mv.app.Store().Channel(mv.forumID)
	if !ok {
		return
	}
	st := mv.app.Store()
	mv.forumView.SetForum(forum, st.Threads(mv.forumID), st.ArchivedThreads(mv.forumID), st.Unread)
}

// onOpenPost opens a forum post as the active chat channel, leaving the forum
// list behind.
func (mv *MainView) onOpenPost(id store.ChannelID) {
	mv.app.SetActive(mv.app.ActiveGuild(), id)
	mv.showChat()
	mv.app.LoadHistory(id, 50)
	mv.updateChannelChrome()
}

func (mv *MainView) headerStyle() screen.Style {
	s := mv.styles.Muted
	s.Attrs |= screen.Bold
	return s
}

// pinnedGuildIDs / pinnedChannelIDs / collapsedCategorySet convert the persisted
// numeric state into the store's ID types for the ordering functions.
func (mv *MainView) pinnedGuildIDs() []store.GuildID {
	out := make([]store.GuildID, 0, len(mv.state.PinnedGuilds))
	for _, id := range mv.state.PinnedGuilds {
		out = append(out, store.GuildID(id))
	}
	return out
}

func (mv *MainView) pinnedChannelIDs() []store.ChannelID {
	out := make([]store.ChannelID, 0, len(mv.state.PinnedChannels))
	for _, id := range mv.state.PinnedChannels {
		out = append(out, store.ChannelID(id))
	}
	return out
}

func (mv *MainView) collapsedCategorySet() map[store.ChannelID]bool {
	out := make(map[store.ChannelID]bool, len(mv.state.CollapsedCategories))
	for _, id := range mv.state.CollapsedCategories {
		out[store.ChannelID(id)] = true
	}
	return out
}

// persist writes the view-state file, surfacing any error through the reporter
// so a failed write is visible rather than silently lost.
func (mv *MainView) persist() {
	if err := mv.state.Save(); err != nil && mv.reportErr != nil {
		mv.reportErr(err)
	}
}

// OnPersistError registers a callback used to report view-state save failures.
func (mv *MainView) OnPersistError(fn func(error)) { mv.reportErr = fn }

func (mv *MainView) recentStickers() []uint64 {
	return append([]uint64(nil), mv.state.RecentStickers...)
}

func (mv *MainView) recordRecentSticker(id uint64) {
	mv.state.RecordRecentSticker(id)
	if err := mv.state.Save(); err != nil && mv.reportErr != nil {
		mv.reportErr(err)
	}
}

// GuildContext returns the guild row a pending right-click landed on, if any.
func (mv *MainView) GuildContext() (store.GuildRow, bool) {
	idx, ok := mv.guildList.TakeContext()
	if !ok || idx < 0 || idx >= len(mv.guildRows) {
		return store.GuildRow{}, false
	}
	return mv.guildRows[idx], true
}

// ChannelContext returns the channel row a pending right-click landed on, if any.
func (mv *MainView) ChannelContext() (store.ChannelRow, bool) {
	idx, ok := mv.channelList.TakeContext()
	if !ok || idx < 0 || idx >= len(mv.channelRows) {
		return store.ChannelRow{}, false
	}
	return mv.channelRows[idx], true
}

// IsGuildPinned reports whether a guild is locally pinned.
func (mv *MainView) IsGuildPinned(id store.GuildID) bool { return mv.state.IsPinnedGuild(uint64(id)) }

// IsChannelPinned reports whether a channel is locally pinned.
func (mv *MainView) IsChannelPinned(id store.ChannelID) bool {
	return mv.state.IsPinnedChannel(uint64(id))
}

// TogglePinGuild flips a guild's local pin and rebuilds the rail.
func (mv *MainView) TogglePinGuild(id store.GuildID) {
	mv.state.TogglePinnedGuild(uint64(id))
	mv.persist()
	mv.rebuildGuilds()
}

// TogglePinChannel flips a channel's local pin and rebuilds the sidebar.
func (mv *MainView) TogglePinChannel(id store.ChannelID) {
	mv.state.TogglePinnedChannel(uint64(id))
	mv.persist()
	mv.refreshChannels()
}

// ToggleCollapseFolder flips a folder's collapsed state and rebuilds the rail.
func (mv *MainView) ToggleCollapseFolder(id int64) {
	mv.state.ToggleCollapsedFolder(id)
	mv.persist()
	mv.rebuildGuilds()
}

// ToggleCollapseCategory flips a category's collapsed state and rebuilds the sidebar.
func (mv *MainView) ToggleCollapseCategory(id store.ChannelID) {
	mv.state.ToggleCollapsedCategory(uint64(id))
	mv.persist()
	mv.refreshChannels()
}

// unreadBadge formats an unread count, capping the display at 99+.
func unreadBadge(n int) string {
	switch {
	case n <= 0:
		return ""
	case n > 99:
		return "99+"
	default:
		return strconv.Itoa(n)
	}
}

func (mv *MainView) refreshMembers(guild store.GuildID) {
	members := mv.app.Store().Members(guild)
	items := make([]widget.Item, 0, len(members))
	for _, m := range members {
		items = append(items, widget.Item{Label: m.Name})
	}
	mv.memberList.SetItems(items)
}

// resolver builds a markup resolver bound to the active guild, so mentions and
// channel references render as display names.
func (mv *MainView) resolver() markup.Resolver {
	st := mv.app.Store()
	guild := mv.app.ActiveGuild()
	return markup.Resolver{
		Member: func(id uint64) (string, bool) {
			return st.MemberName(guild, store.UserID(id))
		},
		Channel: func(id uint64) (string, bool) {
			return st.ChannelName(store.ChannelID(id))
		},
		Role: func(id uint64) (string, uint32, bool) {
			r, ok := st.Role(guild, store.RoleID(id))
			if !ok {
				return "", 0, false
			}
			return r.Name, r.Color, true
		},
		Guild: func(id uint64) (string, bool) {
			return st.GuildName(store.GuildID(id))
		},
	}
}

func (mv *MainView) onSend(content string) {
	if content == "" || mv.composerReadOnly {
		return
	}
	if mv.forumActive {
		if mv.onNewForumPost != nil {
			mv.onNewForumPost(content)
		}
		mv.composer.SetValue("")
		return
	}
	switch mv.composerMode {
	case composerReply:
		mv.app.Reply(content, mv.composerTarget, mv.replyMention)
		mv.CancelComposerMode()
	case composerEdit:
		mv.app.EditMessage(mv.composerTarget.ChannelID, mv.composerTarget.ID, content)
		mv.CancelComposerMode()
	default:
		mv.app.Send(content)
	}
	mv.composer.SetValue("")
}

// InsertIntoComposer drops s at the composer cursor. The picker uses it to
// insert a chosen emoji, custom-emoji mention, or fake-nitro URL.
func (mv *MainView) InsertIntoComposer(s string) {
	if mv == nil || mv.composer == nil {
		return
	}
	mv.composer.Insert(s)
}

// BeginReply puts the composer into inline-reply mode.
func (mv *MainView) BeginReply(msg store.Message, mention bool) {
	if mv == nil {
		return
	}
	mv.composerMode = composerReply
	mv.composerTarget = msg
	mv.replyMention = mention
	mv.updateComposerStatus()
}

// BeginEdit puts the composer into edit mode and preloads the message content.
func (mv *MainView) BeginEdit(msg store.Message) {
	if mv == nil {
		return
	}
	mv.composerMode = composerEdit
	mv.composerTarget = msg
	mv.replyMention = false
	mv.composer.SetValue(msg.Content)
	mv.updateComposerStatus()
}

// CancelComposerMode clears reply/edit state. It reports whether there was a
// mode to cancel, so Shell can consume Esc only when it did useful work.
func (mv *MainView) CancelComposerMode() bool {
	if mv == nil || mv.composerMode == composerNormal {
		return false
	}
	mv.composerMode = composerNormal
	mv.composerTarget = store.Message{}
	mv.replyMention = false
	mv.updateComposerStatus()
	return true
}

func (mv *MainView) updateComposerStatus() {
	if mv == nil || mv.composerStatus == nil {
		return
	}
	if mv.composerReadOnly {
		mv.composerStatus.SetContent("read-only — you can't send messages here")
		return
	}
	switch mv.composerMode {
	case composerReply:
		mode := "on"
		if !mv.replyMention {
			mode = "off"
		}
		mv.composerStatus.SetContent("replying to " + mv.composerTarget.Author + " [mention: " + mode + "]")
	case composerEdit:
		mv.composerStatus.SetContent("editing")
	default:
		mv.composerStatus.SetContent("")
	}
}

func titled(title string, child tui.Widget) *widget.Border {
	b := widget.NewBorder(child)
	b.SetTitle(title)
	b.SetStyle(screen.Style{})
	return b
}

func (mv *MainView) titled(title string, child tui.Widget) *widget.Border {
	b := titled(title, child)
	b.SetStyle(mv.styles.Border)
	b.SetFocusStyle(mv.styles.Accent)
	return b
}
