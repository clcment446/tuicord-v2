// Package ui builds the tuicord widget tree over the internal/tui toolkit.
//
// The main view is a four-panel row — guild rail, channel sidebar, chat column,
// and members panel — with the chat column split into the message view and the
// always-live composer. Both sidebars are drag-resizable (via widget.Split) and
// the members panel auto-hides on narrow terminals.
package ui

import (
	"strconv"

	"awesomeProject/internal/app"
	"awesomeProject/internal/config"
	"awesomeProject/internal/markup"
	"awesomeProject/internal/media"
	"awesomeProject/internal/store"
	"awesomeProject/internal/tui/screen"
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

	guildList      *widget.ItemList
	channelList    *widget.ItemList
	memberList     *widget.ItemList
	chat           *ChatView
	composerStatus *widget.Text
	composer       *widget.TextInput
	composerMode   composerMode
	composerTarget store.Message
	replyMention   bool

	state     *uistate.State
	reportErr func(error)

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
	mv := &MainView{app: a, cfg: cfg, styles: styles, state: state}

	mv.guildList = widget.NewItemList(nil)
	mv.guildList.SetStyle(styles.Text)
	mv.guildList.SetSelectedStyle(styles.Accent)
	mv.guildList.OnSelect(mv.onGuildSelected)

	mv.channelList = widget.NewItemList(nil)
	mv.channelList.SetStyle(styles.Text)
	mv.channelList.SetSelectedStyle(styles.Accent)
	mv.channelList.SetBadgeStyle(styles.Accent)
	mv.channelList.OnSelect(mv.onChannelSelected)

	mv.memberList = widget.NewItemList(nil)
	mv.memberList.SetStyle(styles.Text)

	mv.chat = NewChatView(a.Store(), a.ActiveChannel, mv.resolver, styles)
	if fetcher := newChatMediaFetcher(); fetcher != nil {
		mv.chat.SetMedia(fetcher, media.DefaultConfig(), a.Post)
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

	chatColumn := widget.Column(
		mv.titled("Messages", mv.chat),
		mv.titled("Composer", composerArea),
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

func newChatMediaFetcher() *media.Fetcher {
	cache, err := media.NewCache(0, "")
	if err != nil {
		return nil
	}
	return &media.Fetcher{Cache: cache}
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
func (mv *MainView) RefreshChannels() { mv.refreshChannels() }

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
}

func (mv *MainView) channelItem(row store.ChannelRow) widget.Item {
	if row.Category {
		arrow := glyphExpanded
		if row.Collapsed {
			arrow = glyphCollapsed
		}
		return widget.Item{Label: arrow + " " + row.Name, Style: mv.headerStyle()}
	}
	label := channelPrefix(row.Kind) + row.Name
	switch {
	case row.Pinned:
		label = glyphPinned + " " + label
	case row.Indent:
		label = "  " + label
	}
	badge := ""
	if row.Navigable() {
		badge = unreadBadge(mv.app.Store().Unread(row.ChannelID))
	}
	return widget.Item{Label: label, Badge: badge}
}

func channelPrefix(kind store.ChannelKind) string {
	switch kind {
	case store.ChannelVoice:
		return "~ "
	case store.ChannelDM:
		return ""
	default:
		return "# "
	}
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
	mv.app.LoadHistory(row.ChannelID, 50)
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
	if content == "" {
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
