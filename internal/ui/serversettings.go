package ui

import (
	"awesomeProject/internal/store"
	"awesomeProject/internal/tui/layout"
	"awesomeProject/internal/tui/screen"
	"awesomeProject/internal/tui/tui"
	"awesomeProject/internal/tui/widget"
)

// ServerSettings is the tabbed channel/role management surface.
type ServerSettings struct {
	tabs *widget.Tabs
	root *widget.Border
	node layout.Node
}

func NewServerSettings(st *store.Store, guild store.GuildID, styles Styles, channel func(store.Channel), role func(store.Role)) *ServerSettings {
	channels := st.Channels(guild)
	channelItems := make([]widget.Item, len(channels))
	for i, c := range channels {
		channelItems[i] = widget.Item{Label: c.Name}
	}
	cl := widget.NewItemList(channelItems)
	cl.SetStyle(styles.Cell("settings"))
	cl.SetSelectedStyle(styles.Cell("settings.selected"))
	cl.OnSelect(func(i int) {
		if i >= 0 && i < len(channels) && channel != nil {
			channel(channels[i])
		}
	})
	roles := st.Roles(guild)
	roleItems := make([]widget.Item, len(roles))
	for i, r := range roles {
		roleItems[i] = widget.Item{Label: r.Name}
	}
	rl := widget.NewItemList(roleItems)
	rl.SetStyle(styles.Cell("settings"))
	rl.SetSelectedStyle(styles.Cell("settings.selected"))
	rl.OnSelect(func(i int) {
		if i >= 0 && i < len(roles) && role != nil {
			role(roles[i])
		}
	})
	tabs := widget.NewTabs([]widget.Tab{{Label: "Channels", Content: cl}, {Label: "Roles", Content: rl}})
	tabs.SetStyle(styles.Cell("settings.tab"))
	tabs.SetActiveStyle(styles.Cell("settings.tab.active"))
	root := titled(styles, "Server settings · Enter options · ←/→ tabs · Esc close", tabs)
	root.SetStyle(styles.Cell("panels.border"))
	return &ServerSettings{tabs: tabs, root: root, node: layout.Node{Grow: 1}}
}
func (s *ServerSettings) Children() []tui.Widget      { return []tui.Widget{s.root} }
func (s *ServerSettings) Measure(z tui.Size) tui.Size { return s.root.Measure(z) }
func (s *ServerSettings) Layout() *layout.Node {
	s.node.Children = []*layout.Node{s.root.Layout()}
	return &s.node
}
func (s *ServerSettings) Draw(screen.Region)       {}
func (s *ServerSettings) Handle(ev tui.Event) bool { return s.tabs.Handle(ev) }
