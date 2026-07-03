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

	guildList   *widget.ItemList
	channelList *widget.ItemList
	memberList  *widget.ItemList
	chat        *ChatView
	composer    *widget.TextInput

	guildIDs   []store.GuildID
	channelIDs []store.ChannelID

	// Root is the composed widget passed to App.Run.
	Root tui.Widget
}

// NewMainView assembles the four-panel layout and wires selection callbacks.
func NewMainView(a *app.App, cfg config.Config, styles Styles) *MainView {
	mv := &MainView{app: a, cfg: cfg, styles: styles}

	mv.guildList = widget.NewItemList(nil)
	mv.guildList.SetSelectedStyle(styles.Accent)
	mv.guildList.OnSelect(mv.onGuildSelected)

	mv.channelList = widget.NewItemList(nil)
	mv.channelList.SetSelectedStyle(styles.Accent)
	mv.channelList.OnSelect(mv.onChannelSelected)

	mv.memberList = widget.NewItemList(nil)

	mv.chat = NewChatView(a.Store(), a.ActiveChannel, mv.resolver, styles)

	mv.composer = widget.NewTextInput("Message")
	mv.composer.SetStyle(styles.Text)
	mv.composer.OnSubmit(mv.onSend)

	mv.Root = mv.compose()
	return mv
}

func (mv *MainView) compose() tui.Widget {
	guildRail := titled("Servers", mv.guildList)
	channels := titled("Channels", mv.channelList)
	members := titled("Members", mv.memberList)

	chatColumn := widget.Column(
		titled("Messages", mv.chat),
		titled("Composer", mv.composer),
	)
	// Chat grows, composer is a fixed 3-row box (border + one line).
	chatColumn.Children()[0].Layout().Grow = 1
	composerNode := chatColumn.Children()[1].Layout()
	composerNode.Basis = 3
	composerNode.Grow = 0

	// members | (chat) split so the members panel can auto-hide when narrow.
	chatAndMembers := widget.NewSplit(chatColumn, members).
		Basis(mv.termWidthMinusMembers()).
		Horizontal()
	membersNode := members.Layout()
	membersNode.HideBelow = mv.cfg.Layout.MembersHideBelow

	channelsAndRest := widget.NewSplit(channels, chatAndMembers).
		Basis(mv.cfg.Layout.ChannelsWidth).
		MinFirst(12).
		MaxFirst(40)

	root := widget.NewSplit(guildRail, channelsAndRest).
		Basis(mv.cfg.Layout.GuildsWidth).
		MinFirst(3).
		MaxFirst(24)
	return root
}

// termWidthMinusMembers biases the initial split so members takes MembersWidth.
func (mv *MainView) termWidthMinusMembers() int {
	// Split basis is the first child's width; a large value lets chat take the
	// remainder while members keeps its configured width via its own Min.
	return 200
}

// Refresh repopulates the guild and channel lists from the store. Call on the
// UI goroutine (e.g. from App.OnReady) after state changes.
func (mv *MainView) Refresh() {
	guilds := mv.app.Store().Guilds()
	mv.guildIDs = mv.guildIDs[:0]
	items := make([]widget.Item, 0, len(guilds))
	for _, g := range guilds {
		mv.guildIDs = append(mv.guildIDs, g.ID)
		items = append(items, widget.Item{Label: g.Name})
	}
	mv.guildList.SetItems(items)

	if mv.app.ActiveGuild() == 0 && len(mv.guildIDs) > 0 {
		mv.onGuildSelected(0)
	} else {
		mv.refreshChannels()
	}
}

func (mv *MainView) onGuildSelected(index int) {
	if index < 0 || index >= len(mv.guildIDs) {
		return
	}
	guild := mv.guildIDs[index]
	mv.app.SetActive(guild, 0)
	mv.refreshChannels()
	// Auto-select the first text channel.
	if len(mv.channelIDs) > 0 {
		mv.channelList.SetSelected(0)
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
	for _, c := range channels {
		if c.Kind != store.ChannelText {
			continue
		}
		mv.channelIDs = append(mv.channelIDs, c.ID)
		items = append(items, widget.Item{Label: "# " + c.Name, Badge: unreadBadge(mv.app.Store().Unread(c.ID))})
	}
	mv.channelList.SetItems(items)
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
	mv.app.SetActive(mv.app.ActiveGuild(), mv.channelIDs[index])
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
	}
}

func (mv *MainView) onSend(content string) {
	if content == "" {
		return
	}
	mv.app.Send(content)
	mv.composer.SetValue("")
}

func titled(title string, child tui.Widget) *widget.Border {
	b := widget.NewBorder(child)
	b.SetTitle(title)
	b.SetStyle(screen.Style{})
	return b
}
