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

	guildIDs   []store.GuildID
	channelIDs []store.ChannelID

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
	mv := &MainView{app: a, cfg: cfg, styles: styles}

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
	guilds := mv.app.Store().Guilds()
	mv.guildIDs = mv.guildIDs[:0]
	items := make([]widget.Item, 0, len(guilds))
	activeIndex := -1
	for _, g := range guilds {
		if g.ID == mv.app.ActiveGuild() {
			activeIndex = len(mv.guildIDs)
		}
		mv.guildIDs = append(mv.guildIDs, g.ID)
		items = append(items, widget.Item{Label: g.Name})
	}
	mv.guildList.SetItems(items)

	switch {
	case activeIndex >= 0:
		mv.guildList.SetSelectedSilent(activeIndex)
		mv.refreshChannels()
		mv.refreshMembers(mv.app.ActiveGuild())
	case mv.app.ActiveGuild() == 0 && len(mv.guildIDs) > 0:
		mv.onGuildSelected(0)
	default:
		mv.refreshChannels()
	}
}

func (mv *MainView) onGuildSelected(index int) {
	if index < 0 || index >= len(mv.guildIDs) {
		return
	}
	guild := mv.guildIDs[index]
	mv.app.SetActive(guild, 0)
	mv.app.LoadRoles(guild)
	mv.app.LoadChannels(guild)
	mv.refreshChannels()
	// Auto-select the first text channel.
	if len(mv.channelIDs) > 0 {
		mv.channelList.SetSelectedSilent(0)
		mv.onChannelSelected(0)
	}
	mv.refreshMembers(guild)
}

// RefreshChannels rebuilds the channel list (and its unread badges) from the
// store. Safe to call on the UI goroutine, e.g. from App.OnChange.
func (mv *MainView) RefreshChannels() { mv.refreshChannels() }

func (mv *MainView) refreshChannels() {
	guild := mv.app.ActiveGuild()
	channels := mv.app.Store().Channels(guild)
	mv.channelIDs = mv.channelIDs[:0]
	items := make([]widget.Item, 0, len(channels))
	activeIndex := -1
	for _, c := range channels {
		if c.Kind != store.ChannelText && c.Kind != store.ChannelDM {
			continue
		}
		label := "# " + c.Name
		if c.Kind == store.ChannelDM {
			label = c.Name
		}
		if c.ID == mv.app.ActiveChannel() {
			activeIndex = len(mv.channelIDs)
		}
		mv.channelIDs = append(mv.channelIDs, c.ID)
		items = append(items, widget.Item{Label: label, Badge: unreadBadge(mv.app.Store().Unread(c.ID))})
	}
	mv.channelList.SetItems(items)
	switch {
	case activeIndex >= 0:
		mv.channelList.SetSelectedSilent(activeIndex)
	case mv.app.ActiveChannel() == 0 && len(mv.channelIDs) > 0:
		mv.channelList.SetSelectedSilent(0)
		mv.onChannelSelected(0)
	}
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

func (mv *MainView) onChannelSelected(index int) {
	if index < 0 || index >= len(mv.channelIDs) {
		return
	}
	channel := mv.channelIDs[index]
	mv.app.SetActive(mv.app.ActiveGuild(), channel)
	mv.app.LoadHistory(channel, 50)
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
