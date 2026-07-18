// Package ui builds the tuicord widget tree over the internal/tui toolkit.
//
// The main view is a four-panel row — guild rail, channel sidebar, chat column,
// and members panel — with the chat column split into the message view and the
// always-live composer. Both sidebars are drag-resizable (via widget.Split) and
// the members panel auto-hides on narrow terminals.
package ui

import (
	"fmt"
	"image"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"awesomeProject/internal/app"
	"awesomeProject/internal/config"
	"awesomeProject/internal/markup"
	"awesomeProject/internal/media"
	"awesomeProject/internal/store"
	"awesomeProject/internal/tui/layout"
	"awesomeProject/internal/tui/screen"
	"awesomeProject/internal/tui/term"
	"awesomeProject/internal/tui/tui"
	"awesomeProject/internal/tui/widget"
	"awesomeProject/internal/uistate"

	"github.com/diamondburned/arikawa/v3/utils/sendpart"
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
	app      *app.App
	cfg      config.Config
	styles   Styles
	mediaCfg media.Config

	guildList        *widget.ItemList
	channelList      *widget.ItemList
	memberList       *widget.ItemList
	chat             *ChatView
	chatBorder       *widget.Border
	composerBorder   *widget.Border
	composerStatus   *widget.Text
	composerFiles    *widget.Text
	composerPreview  *widget.Node
	composerNode     *layout.Node
	previewCellW     int
	previewCellH     int
	composer         *widget.TextInput
	attachments      []queuedAttachment
	composerMode     composerMode
	composerTarget   store.Message
	replyMention     bool
	composerReadOnly bool
	inputMode        bool
	onComposerChange func(string, int)
	onLocalCommand   func(string) bool

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
	mediaCfg := chatMediaConfig(cfg)
	mv := &MainView{app: a, cfg: cfg, styles: styles, mediaCfg: mediaCfg, state: state,
		ascii: cfg.Display.ASCII || os.Getenv("NO_COLOR") != ""}

	mv.guildList = widget.NewItemList(nil)
	mv.guildList.SetStyle(styles.Cell("guilds"))
	mv.guildList.SetSelectedStyle(styles.Cell("guilds.selected"))
	mv.guildList.SetBadgeStyle(styles.Cell("guilds.badge"))
	mv.guildList.OnSelect(mv.onGuildSelected)
	mv.guildList.OnVimFocus(mv.unfoldSelectedGuildFolder)
	mv.guildList.SetVimNavigation(cfg.Accessibility.VimNavigation)

	mv.channelList = widget.NewItemList(nil)
	mv.channelList.SetStyle(styles.Cell("guilds.channels"))
	mv.channelList.SetSelectedStyle(styles.Cell("guilds.selected"))
	mv.channelList.SetBadgeStyle(styles.Cell("guilds.badge"))
	mv.channelList.OnSelect(mv.onChannelSelected)
	mv.channelList.OnHover(mv.onChannelHovered)
	mv.channelList.OnVimFocus(mv.unfoldSelectedChannelCategory)
	mv.channelList.SetVimNavigation(cfg.Accessibility.VimNavigation)

	mv.memberList = widget.NewItemList(nil)
	mv.memberList.SetVimNavigation(cfg.Accessibility.VimNavigation)
	mv.memberList.SetStyle(styles.Cell("guilds.members"))

	mv.chat = NewChatView(a.Store(), a.ActiveChannel, mv.resolver, styles)
	mv.chat.SetRoleGradients(cfg.Display.RoleGradients, cfg.Display.RoleGradientAnimations)
	mv.chat.SetVimNavigation(cfg.Accessibility.VimNavigation)
	mv.chat.SetMouseBreakpointTracking(cfg.Accessibility.MouseBreakpointTracking)
	mv.chat.SetHighlightFocusBlock(cfg.Accessibility.HighlightFocusBlock)
	mv.chat.OnReachTop(func() { a.LoadOlderHistory(a.ActiveChannel()) })
	if fetcher := newChatMediaFetcher(mediaCfg); fetcher != nil {
		mv.chat.SetMedia(fetcher, mediaCfg, a.Post)
		mv.chat.SetInvalidate(a.Invalidate)
	}

	mv.composerStatus = widget.NewText("")
	mv.composerStatus.SetStyle(styles.Cell("composer.status"))
	mv.composerStatus.SetWrap(false)
	mv.composerFiles = widget.NewText("")
	mv.composerFiles.SetStyle(styles.Cell("messages.attachment"))
	mv.composerFiles.SetWrap(true)

	// Inline image thumbnails render above the composer; size them against the
	// terminal's cell pixel geometry so Kitty graphics are crisp.
	mv.previewCellW, mv.previewCellH = mediaCfg.CellPixels()
	mv.composerPreview = widget.Column()
	mv.composerPreview.Layout().Grow = 0
	mv.composerPreview.Layout().Hidden = true

	mv.composer = widget.NewTextInput("Message")
	mv.composer.SetPreferredFocus(!cfg.Accessibility.VimNavigation)
	mv.composer.SetInputFocusEnabled(!cfg.Accessibility.VimNavigation)
	mv.composer.SetStyle(styles.Cell("composer"))
	mv.composer.SetPlaceholderStyle(styles.Cell("composer.placeholder"))
	mv.composer.SetCursorStyle(styles.Cell("composer.cursor"))
	mv.composer.SetMultiline(4)
	mv.composer.OnSubmit(mv.onSend)
	mv.composer.OnPaste(mv.importPastedAttachments)
	mv.composer.OnChange(func(value string) {
		if mv.onComposerChange != nil {
			mv.onComposerChange(value, mv.composer.Cursor())
		}
	})

	mv.Root = mv.compose()
	return mv
}

func (mv *MainView) compose() tui.Widget {
	guildRail := mv.titled("Servers", mv.guildList)
	channels := mv.titled("Channels", mv.channelList)
	members := mv.titled("Members", mv.memberList)

	// Order: status, image thumbnails, filename chips, then the input — so a
	// pasted image previews above its caption and the text field.
	composerArea := widget.Column(mv.composerStatus, mv.composerPreview, mv.composerFiles, mv.composer)
	composerArea.Children()[0].Layout().Basis = 1
	composerArea.Children()[0].Layout().Grow = 0
	composerArea.Children()[2].Layout().Basis = 1
	composerArea.Children()[2].Layout().Grow = 0
	composerArea.Children()[3].Layout().Basis = 4
	composerArea.Children()[3].Layout().Grow = 0

	mv.chatBorder = mv.titled("Messages", mv.chat)
	mv.composerBorder = mv.titled("Composer", composerArea)
	chatColumn := widget.Column(
		mv.chatBorder,
		mv.composerBorder,
	)
	// Chat grows; the composer reserves room for four wrapped input rows and
	// a compact attachment-chip line.
	chatColumn.Children()[0].Layout().Grow = 1
	composerNode := chatColumn.Children()[1].Layout()
	composerNode.Basis = composerBaseBasis
	composerNode.Grow = 0
	mv.composerNode = composerNode
	mv.cfg.Layout.Element("messages").Apply(mv.chatBorder.Layout(), layout.Column)
	mv.cfg.Layout.Element("composer").Apply(mv.composerBorder.Layout(), layout.Column)

	membersPolicy := mv.cfg.Layout.Element("members")
	membersWidth := mv.cfg.Layout.MembersWidth
	if membersPolicy.Width > 0 {
		membersWidth = membersPolicy.Width
	}

	// chat | members split so members stays beside the messages/composer stack.
	chatAndMembers := widget.NewSplit(chatColumn, members).
		Basis(mv.termWidthMinusMembers()).
		MinSecond(membersWidth).
		CollapsibleSecond().
		Vertical()
	chatAndMembers.SetStyle(mv.styles.Cell("panels.border"))
	membersNode := members.Layout()
	membersPolicy.Apply(membersNode, layout.Row)
	membersNode.HideBelow = mv.cfg.Layout.MembersHideBelow
	if membersPolicy.Visible != nil {
		chatAndMembers.HideSecond(!*membersPolicy.Visible)
	}

	channelsPolicy := mv.cfg.Layout.Element("channels")
	channelsWidth := mv.cfg.Layout.ChannelsWidth
	if channelsPolicy.Width > 0 {
		channelsWidth = channelsPolicy.Width
	}

	channelsAndRest := widget.NewSplit(channels, chatAndMembers).
		Basis(channelsWidth).
		MinFirst(12).
		MaxFirst(40).
		CollapsibleFirst()
	channelsAndRest.SetStyle(mv.styles.Cell("panels.border"))
	channelsPolicy.Apply(channels.Layout(), layout.Row)
	if channelsPolicy.Visible != nil {
		channelsAndRest.HideFirst(!*channelsPolicy.Visible)
	}

	guildsPolicy := mv.cfg.Layout.Element("guilds")
	guildsWidth := mv.cfg.Layout.GuildsWidth
	if guildsPolicy.Width > 0 {
		guildsWidth = guildsPolicy.Width
	}

	root := widget.NewSplit(guildRail, channelsAndRest).
		Basis(guildsWidth).
		MinFirst(3).
		MaxFirst(24).
		CollapsibleFirst()
	root.SetStyle(mv.styles.Cell("panels.border"))
	guildsPolicy.Apply(guildRail.Layout(), layout.Row)
	if guildsPolicy.Visible != nil {
		root.HideFirst(!*guildsPolicy.Visible)
	}
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
	cfg = cfg.Bounded()
	cache.ConfigureDisk(cfg.DiskCacheMaxBytes, cfg.DiskCacheTTL)
	return &media.Fetcher{
		Cache:              cache,
		MaxPixels:          chatMediaPixelBudget(cfg),
		MaxResponseBytes:   cfg.MaxResponseBytes,
		MaxSourcePixels:    cfg.MaxSourcePixels,
		MaxSourceDimension: cfg.MaxSourceDimension,
		GIFMaxFrames:       cfg.GIFMaxFrames,
		GIFMaxMemoryBytes:  cfg.GIFMaxMemoryBytes,
		RequestTimeout:     cfg.RequestTimeout,
		DisableDiskCache:   !cfg.DiskCacheEnabled,
	}
}

// newViewerMediaFetcher uses the viewer's separately configured (larger, but
// still bounded) response/decode limits and deliberately avoids inline
// downscaling so enlargement does not magnify the chat thumbnail.
func newViewerMediaFetcher(cfg media.Config) *media.Fetcher {
	fetcher := newChatMediaFetcher(cfg)
	if fetcher != nil {
		fetcher.MaxPixels = image.Point{}
	}
	return fetcher
}

// chatMediaConfig projects user media/privacy settings into the media package
// and fills terminal cell geometry when available.
func chatMediaConfig(appCfg config.Config) media.Config {
	defaults := media.DefaultConfig()
	m := appCfg.Media
	cfg := media.Config{
		Enabled:            m.Enabled && appCfg.Privacy.FetchExternalMedia,
		MaxHeightCells:     m.MaxHeightCells,
		Animate:            m.AnimateGIFs,
		EmojiImages:        m.EmojiImages,
		MaxResponseBytes:   m.MaxResponseBytes,
		MaxSourcePixels:    m.MaxSourcePixels,
		MaxSourceDimension: m.MaxSourceDimension,
		GIFMaxFrames:       m.MaxGIFFrames,
		GIFMaxMemoryBytes:  m.MaxGIFMemoryBytes,
		RequestTimeout:     time.Duration(m.RequestTimeoutSeconds) * time.Second,
		ConcurrentFetches:  m.ConcurrentFetches,
		QueuedFetches:      m.QueuedFetches,
		DiskCacheEnabled:   appCfg.Privacy.PersistMediaCache,
		DiskCacheMaxBytes:  m.CacheMaxBytes,
		DiskCacheTTL:       time.Duration(m.CacheTTLHours) * time.Hour,
		Prefetch:           appCfg.Privacy.PrefetchMedia,
		MpvPath:            m.MpvPath,
		VideoEnabled:       m.VideoEnabled && appCfg.Privacy.PlayVideos,
		VideoAudio:         m.VideoAudio,
	}
	cfg = cfg.Bounded()
	local := os.Getenv("SSH_CONNECTION") == "" && os.Getenv("SSH_TTY") == ""
	switch strings.ToLower(strings.TrimSpace(m.VideoUseSHM)) {
	case "true", "yes", "on":
		cfg.VideoUseSHM = true
	case "false", "no", "off":
		cfg.VideoUseSHM = false
	default:
		cfg.VideoUseSHM = local
	}
	if m.MpvPath == "" {
		cfg.MpvPath = defaults.MpvPath
	}
	if sz, err := term.ProbeSize(); err == nil {
		if w, h, ok := sz.CellPixels(); ok {
			cfg.CellPixelWidth, cfg.CellPixelHeight = w, h
		}
	}
	return cfg
}

func viewerMediaConfig(appCfg config.Config) media.Config {
	cfg := chatMediaConfig(appCfg)
	m := appCfg.Media
	cfg.MaxResponseBytes = m.ViewerMaxResponseBytes
	cfg.MaxSourcePixels = m.ViewerMaxSourcePixels
	cfg.MaxSourceDimension = m.ViewerMaxSourceDimension
	cfg.GIFMaxFrames = m.ViewerMaxGIFFrames
	cfg.GIFMaxMemoryBytes = m.ViewerMaxGIFMemoryBytes
	return cfg.Bounded()
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
	selectedGuild := store.GuildID(0)
	if i := mv.guildList.Selected(); i >= 0 && i < len(mv.guildRows) {
		selectedGuild = mv.guildRows[i].GuildID
	}
	mv.guildRows = store.OrderGuilds(st.GuildFolders(), st.Guilds(), mv.pinnedGuildIDs(), mv.state.CollapsedFolderSet())
	items := make([]widget.Item, len(mv.guildRows))
	for i, row := range mv.guildRows {
		items[i] = mv.guildItem(row)
	}
	mv.guildList.SetItems(items)
	if i := mv.guildRowIndex(selectedGuild); i >= 0 {
		mv.guildList.SetSelectedSilent(i)
	} else if i := mv.guildRowIndex(mv.app.ActiveGuild()); i >= 0 {
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
	badge := ""
	if mv.app != nil {
		badge = unreadBadge(mv.app.Store().GuildPings(row.GuildID))
	}
	return widget.Item{Label: label, Badge: badge}
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

func (mv *MainView) unfoldSelectedGuildFolder(_ bool) bool {
	if mv == nil || mv.guildList == nil {
		return false
	}
	index := mv.guildList.Selected()
	if index < 0 || index >= len(mv.guildRows) {
		return false
	}
	row := mv.guildRows[index]
	if !row.Folder || !row.Collapsed {
		return false
	}
	mv.state.ToggleCollapsedFolder(row.FolderID)
	mv.persist()
	mv.rebuildGuilds()
	return true
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
	selectedChannel := store.ChannelID(0)
	if i := mv.channelList.Selected(); i >= 0 && i < len(mv.channelRows) {
		selectedChannel = mv.channelRows[i].ChannelID
	}
	channels := st.Channels(guild)
	mv.channelRows = store.GroupChannelsWithPriority(channels, mv.pinnedChannelIDs(), mv.collapsedCategorySet(), st.PingedChannels())
	items := make([]widget.Item, len(mv.channelRows))
	for i, row := range mv.channelRows {
		items[i] = mv.channelItem(row)
	}
	mv.channelList.SetItems(items)
	switch {
	case mv.channelListRowIndex(selectedChannel) >= 0:
		mv.channelList.SetSelectedSilent(mv.channelListRowIndex(selectedChannel))
	case mv.channelRowIndex(mv.app.ActiveChannel()) >= 0:
		mv.channelList.SetSelectedSilent(mv.channelRowIndex(mv.app.ActiveChannel()))
	case mv.app.ActiveChannel() == 0 && mv.firstNavigableChannel() >= 0:
		i := mv.firstNavigableChannel()
		mv.channelList.SetSelectedSilent(i)
		mv.onChannelSelected(i)
	}
	mv.updateChannelChrome()
}

// channelListRowIndex finds every rendered row, including categories, so a
// background refresh cannot snap a user browsing the sidebar back to active.
func (mv *MainView) channelListRowIndex(id store.ChannelID) int {
	if id == 0 {
		return -1
	}
	for i, row := range mv.channelRows {
		if row.ChannelID == id {
			return i
		}
	}
	return -1
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
		badge = unreadBadge(mv.app.Store().Pings(row.ChannelID))
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

// NavigateToChannel activates a known channel from an out-of-band UI target
// such as an incoming-message notification.
func (mv *MainView) NavigateToChannel(id store.ChannelID) bool {
	if mv == nil || mv.app == nil || id == 0 {
		return false
	}
	channel, ok := mv.app.Store().Channel(id)
	if !ok {
		return false
	}
	if mv.app.ActiveGuild() != channel.GuildID {
		mv.app.SetActive(channel.GuildID, id)
		if guildIndex := mv.guildRowIndex(channel.GuildID); guildIndex >= 0 {
			mv.guildList.SetSelectedSilent(guildIndex)
		}
		mv.refreshChannels()
		mv.refreshMembers(channel.GuildID)
	}
	if index := mv.channelRowIndex(id); index >= 0 {
		mv.channelList.SetSelectedSilent(index)
		mv.onChannelSelected(index)
		return true
	}
	mv.app.SetActive(channel.GuildID, id)
	mv.showChat()
	mv.app.LoadHistory(id, 50)
	mv.updateChannelChrome()
	return true
}

func (mv *MainView) unfoldSelectedChannelCategory(_ bool) bool {
	if mv == nil || mv.channelList == nil {
		return false
	}
	index := mv.channelList.Selected()
	if index < 0 || index >= len(mv.channelRows) {
		return false
	}
	row := mv.channelRows[index]
	if !row.Category || !row.Collapsed {
		return false
	}
	mv.state.ToggleCollapsedCategory(uint64(row.ChannelID))
	mv.persist()
	mv.refreshChannels()
	return true
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
		mv.forumView.SetVimNavigation(mv.cfg.Accessibility.VimNavigation)
		mv.forumView.onFilterCycle = mv.refreshForum
		mv.forumView.onFilterMenu = mv.onForumFilter
		mv.forumView.onNavigate = mv.navigateForum
		mv.forumPreview = NewChatView(mv.app.Store(), func() store.ChannelID { return mv.forumPreviewID }, mv.resolver, mv.styles)
		if fetcher := newChatMediaFetcher(mv.mediaCfg); fetcher != nil {
			mv.forumPreview.SetMedia(fetcher, mv.mediaCfg, mv.app.Post)
			mv.forumPreview.SetInvalidate(mv.app.Invalidate)
		}
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
	return mv.styles.Cell("guilds.header")
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
	mv.persist()
}

func (mv *MainView) favoriteEmojis() []string {
	return append([]string(nil), mv.state.FavoriteEmojis...)
}
func (mv *MainView) favoriteStickers() []uint64 {
	return append([]uint64(nil), mv.state.FavoriteStickers...)
}
func (mv *MainView) toggleFavorite(emoji string, sticker uint64) {
	if sticker != 0 {
		mv.state.ToggleFavoriteSticker(sticker)
	} else {
		mv.state.ToggleFavoriteEmoji(emoji)
	}
	mv.persist()
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
		return "99"
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
	channel := mv.app.ActiveChannel()
	return markup.Resolver{
		Member: func(id uint64) (string, bool) {
			m, ok := memberForContext(st, guild, channel, store.UserID(id))
			return m.Name, ok
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

// memberForContext resolves guild members normally and falls back to the
// active conversation's participant list for direct and group DMs.
func memberForContext(st *store.Store, guild store.GuildID, channel store.ChannelID, id store.UserID) (store.Member, bool) {
	if m, ok := st.Member(guild, id); ok {
		return m, true
	}
	return st.ChannelRecipient(channel, id)
}

func (mv *MainView) onSend(content string) {
	if updated, attachments, err := importDollarPaths(mv.workspaceRoot(), content); err != nil {
		mv.reportUploadError(err)
		return
	} else {
		content = updated
		for _, attachment := range attachments {
			if !mv.hasAttachment(attachment.path) {
				mv.attachments = append(mv.attachments, attachment)
			}
		}
	}
	if (strings.TrimSpace(content) == "" && len(mv.attachments) == 0) || mv.composerReadOnly {
		return
	}
	if mv.composerMode == composerNormal && mv.onLocalCommand != nil && mv.onLocalCommand(content) {
		mv.composer.SetValue("")
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
		files, optimistic, cleanup, err := mv.openAttachments()
		if err != nil {
			mv.reportUploadError(err)
			return
		}
		mv.app.SendFiles(content, files, optimistic, cleanup)
	}
	mv.composer.SetValue("")
	mv.clearAttachments()
}

func (mv *MainView) importPastedAttachments(value string) bool {
	attachments, imported, err := pastedWorkspacePaths(mv.workspaceRoot(), value)
	if err != nil {
		mv.reportUploadError(err)
		return true
	}
	if !imported {
		return false
	}
	for _, attachment := range attachments {
		if !mv.hasAttachment(attachment.path) {
			mv.attachments = append(mv.attachments, attachment)
		}
	}
	mv.updateAttachmentChips()
	return true
}

func (mv *MainView) workspaceRoot() string {
	root, err := os.Getwd()
	if err != nil {
		return "."
	}
	return root
}

func (mv *MainView) hasAttachment(path string) bool {
	for _, attachment := range mv.attachments {
		if attachment.path == path {
			return true
		}
	}
	return false
}

func (mv *MainView) openAttachments() ([]sendpart.File, []store.Attachment, func(), error) {
	files := make([]sendpart.File, 0, len(mv.attachments))
	optimistic := make([]store.Attachment, 0, len(mv.attachments))
	closers := make([]*os.File, 0, len(mv.attachments))
	for _, attachment := range mv.attachments {
		file, err := os.Open(attachment.path)
		if err != nil {
			for _, open := range closers {
				_ = open.Close()
			}
			return nil, nil, nil, fmt.Errorf("open %q: %w", attachment.meta.Filename, err)
		}
		closers = append(closers, file)
		files = append(files, sendpart.File{Name: attachment.meta.Filename, Reader: file})
		optimistic = append(optimistic, attachment.meta)
	}
	cleanup := func() {
		for _, file := range closers {
			_ = file.Close()
		}
	}
	return files, optimistic, cleanup, nil
}

// StageTempImage queues a temporary file (e.g. an image read from the system
// clipboard) as an attachment. The file is owned by the composer: it is deleted
// once the message is sent or the attachments are cleared. It returns an error
// if the file exceeds the upload limit, in which case the caller should remove
// the temp file.
func (mv *MainView) StageTempImage(path, filename string, size int64) error {
	if size > MaxUploadBytes {
		return fmt.Errorf("image is larger than the %d MiB upload limit", MaxUploadBytes/(1024*1024))
	}
	if mv.composerReadOnly {
		return fmt.Errorf("this channel does not accept attachments")
	}
	if mv.hasAttachment(path) {
		return nil
	}
	mv.attachments = append(mv.attachments, queuedAttachment{
		path: path,
		meta: store.Attachment{Filename: filename, Size: size},
		temp: true,
	})
	mv.updateAttachmentChips()
	return nil
}

// imageAttachments returns the staged attachments whose filename looks like an
// image, for previewing.
func (mv *MainView) imageAttachments() []queuedAttachment {
	var out []queuedAttachment
	for _, attachment := range mv.attachments {
		if isImageFilename(attachment.meta.Filename) {
			out = append(out, attachment)
		}
	}
	return out
}

// isImageFilename reports whether name has a raster image extension the client
// can decode for a preview.
func isImageFilename(name string) bool {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".png", ".jpg", ".jpeg", ".gif", ".webp", ".bmp":
		return true
	default:
		return false
	}
}

func (mv *MainView) clearAttachments() {
	for _, attachment := range mv.attachments {
		if attachment.temp {
			_ = os.Remove(attachment.path)
		}
	}
	mv.attachments = nil
	mv.updateAttachmentChips()
}

func (mv *MainView) updateAttachmentChips() {
	if mv.composerFiles == nil {
		return
	}
	chips := make([]string, 0, len(mv.attachments))
	for _, attachment := range mv.attachments {
		chips = append(chips, attachment.meta.Filename+" ("+formatAttachmentSize(attachment.meta.Size)+")")
	}
	mv.composerFiles.SetContent(strings.Join(chips, " · "))
	mv.updateComposerPreview()
}

func formatAttachmentSize(size int64) string {
	if size < 1024 {
		return strconv.FormatInt(size, 10) + " B"
	}
	return strconv.FormatInt((size+1023)/1024, 10) + " KiB"
}

func (mv *MainView) reportUploadError(err error) {
	if mv.reportErr != nil {
		mv.reportErr(err)
	}
}

// InsertIntoComposer drops s at the composer cursor. The picker uses it to
// insert a chosen emoji, custom-emoji mention, or marked fake-Nitro link.
func (mv *MainView) InsertIntoComposer(s string) {
	if mv == nil || mv.composer == nil {
		return
	}
	mv.composer.Insert(s)
}

// ReplaceComposerRange replaces a completion token in the composer. TextInput
// normalizes offsets to grapheme boundaries before applying the replacement.
func (mv *MainView) ReplaceComposerRange(start, end int, s string) {
	if mv == nil || mv.composer == nil {
		return
	}
	mv.composer.Replace(start, end, s)
}

// SetComposerChange registers a callback for user and programmatic composer
// edits. Shell uses it to detect inline autocomplete triggers.
func (mv *MainView) SetComposerChange(fn func(string, int)) { mv.onComposerChange = fn }

// SetLocalCommandHandler installs the local ';' command dispatcher. A true
// result consumes the composer text instead of sending it to Discord.
func (mv *MainView) SetLocalCommandHandler(fn func(string) bool) { mv.onLocalCommand = fn }

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
	if mv.composer != nil {
		mv.composer.SetValue("")
	}
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
	modePrefix := ""
	if mv.inputMode {
		modePrefix = "INPUT"
	}
	withMode := func(detail string) string {
		if modePrefix == "" {
			return detail
		}
		return modePrefix + " · " + detail
	}
	switch mv.composerMode {
	case composerReply:
		mode := "on"
		if !mv.replyMention {
			mode = "off"
		}
		mv.composerStatus.SetContent(withMode("replying to " + mv.composerTarget.Author + " [mention: " + mode + "]"))
	case composerEdit:
		mv.composerStatus.SetContent(withMode("editing"))
	default:
		mv.composerStatus.SetContent(modePrefix)
	}
}

// SetInputMode updates the composer mode indicator used by Vim navigation.
func (mv *MainView) SetInputMode(enabled bool) {
	if mv == nil {
		return
	}
	mv.inputMode = enabled
	if mv.composer != nil && mv.cfg.Accessibility.VimNavigation {
		mv.composer.SetInputFocusEnabled(enabled)
	}
	mv.updateComposerStatus()
}

func titled(title string, child tui.Widget) *widget.Border {
	b := widget.NewBorder(child)
	b.SetTitle(title)
	b.SetStyle(screen.Style{})
	return b
}

func (mv *MainView) titled(title string, child tui.Widget) *widget.Border {
	b := titled(title, child)
	b.SetStyle(mv.styles.Cell("panels.border"))
	b.SetFocusStyle(mv.styles.Cell("panels.focus"))
	return b
}
