package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"awesomeProject/examples/internal/demo"
	"awesomeProject/internal/store"
	"awesomeProject/internal/tui/input"
	"awesomeProject/internal/tui/layout"
	"awesomeProject/internal/tui/screen"
	"awesomeProject/internal/tui/tui"
	"awesomeProject/internal/tui/widget"
	"awesomeProject/internal/ui"
)

const channelID store.ChannelID = 1

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	root := build(cancel)
	if os.Getenv("TUI_EXAMPLE_RENDER") == "1" {
		fmt.Println(demo.Dump(tui.New().Render(root, tui.Size{W: 96, H: 44})))
		return
	}
	if err := tui.New().RunContext(ctx, root); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func build(cancel context.CancelFunc) tui.Widget {
	st := store.New(0)
	for _, msg := range mockMessages() {
		st.AppendMessage(msg)
	}
	styles := ui.Styles{
		Text:    screen.Style{Fg: screen.RGB(220, 224, 230)},
		Muted:   screen.Style{Fg: screen.RGB(145, 152, 164)},
		Accent:  screen.Style{Fg: screen.RGB(130, 170, 255), Attrs: screen.Bold},
		Pending: screen.Style{Fg: screen.RGB(245, 205, 100)},
		Error:   screen.Style{Fg: screen.RGB(240, 105, 105)},
	}
	chat := ui.NewChatView(st, func() store.ChannelID { return channelID }, nil, styles)
	status := widget.NewText("Press 1-9/0 or click a control. q/Esc exits.")
	status.SetStyle(screen.Style{Fg: screen.RGB(170, 178, 190)})
	status.Layout().Basis = 1
	status.Layout().Grow = 0
	app := &mockApp{
		node:   layout.Node{Dir: layout.Column, Grow: 1},
		chat:   chat,
		status: status,
		cancel: cancel,
	}
	app.node.Children = []*layout.Node{chat.Layout(), status.Layout()}
	return demo.Column(titled("Components V2 Mock Playground", app)).WithCancel(cancel)
}

type mockApp struct {
	node   layout.Node
	chat   *ui.ChatView
	status *widget.Text
	cancel context.CancelFunc
}

func (a *mockApp) Measure(avail tui.Size) tui.Size { return avail }
func (a *mockApp) Layout() *layout.Node            { return &a.node }
func (a *mockApp) Draw(screen.Region)              {}
func (a *mockApp) Children() []tui.Widget          { return []tui.Widget{a.chat, a.status} }

func (a *mockApp) Handle(ev tui.Event) bool {
	if key, ok := ev.(input.KeyEvent); ok && !key.Release {
		if key.Key == input.KeyEsc || (key.Key == input.KeyRune && key.Rune == 'q') {
			a.cancel()
			return true
		}
	}
	if a.chat.Handle(ev) {
		if action, ok := a.chat.TakeComponentAction(); ok {
			target := action.CustomID
			if target == "" {
				target = action.URL
			}
			if len(action.Values) > 1 {
				target += "=" + strings.Join(action.Values, ",")
			} else if action.Value != "" {
				target += "=" + action.Value
			}
			a.status.SetContent(fmt.Sprintf("Activated [%c] %s -> %s", action.Shortcut, action.Label, target))
		}
		return true
	}
	return false
}

func titled(title string, child tui.Widget) *widget.Border {
	box := widget.NewBorder(child)
	box.SetTitle(title)
	box.SetStyle(screen.Style{Fg: screen.RGB(130, 170, 255), Attrs: screen.Bold})
	return box
}

func mockMessages() []store.Message {
	return []store.Message{
		{
			ID:        1,
			ChannelID: channelID,
			Author:    "Heavenly Dao",
			Content:   "Legacy rich embed, kept for comparison.",
			Embeds: []store.Embed{{
				Kind:        store.EmbedRich,
				Color:       0x5865F2,
				AuthorName:  "System",
				Title:       "Legacy Realm Card",
				Description: "**Foundation Realm**\nQi: 120 / 200",
				Fields: []store.EmbedField{
					{Name: "Technique", Value: "Cloud Step", Inline: true},
					{Name: "Cooldown", Value: "Ready", Inline: true},
				},
				FooterText: "V1 embed renderer",
			}},
		},
		{
			ID:        2,
			ChannelID: channelID,
			Author:    "Heavenly Dao",
			Flags:     1 << 15,
			ComponentTree: []store.ComponentNode{{
				Kind:        store.ComponentContainer,
				AccentColor: 0x57F287,
				Children: []store.ComponentNode{
					{Kind: store.ComponentTextDisplay, Content: "## Cultivation Status\n**Realm:** Foundation\n**Qi:** 120 / 200"},
					{
						Kind: store.ComponentSection,
						Children: []store.ComponentNode{
							{Kind: store.ComponentTextDisplay, Content: "A quiet room, a full incense stick, and one decision."},
						},
						Accessory: &store.ComponentNode{
							Kind:    store.ComponentThumbnail,
							Media:   []store.ComponentMedia{{Description: "realm sigil", URL: "mock://realm"}},
							RawType: 11,
						},
					},
					{
						Kind: store.ComponentSection,
						Children: []store.ComponentNode{
							{Kind: store.ComponentTextDisplay, Content: "A section with a button accessory should stay keyboard reachable."},
						},
						Accessory: &store.ComponentNode{
							Kind:     store.ComponentButton,
							Label:    "Inspect",
							CustomID: "inspect",
							Style:    2,
						},
					},
					{Kind: store.ComponentActionRow, Children: []store.ComponentNode{
						{Kind: store.ComponentButton, Label: "Cultivate", CustomID: "cultivate", Style: 1},
						{Kind: store.ComponentButton, Label: "Breakthrough", CustomID: "breakthrough", Style: 3, State: store.ComponentStatePending},
						{Kind: store.ComponentButton, Label: "Accepted", CustomID: "accepted", Style: 3, State: store.ComponentStateSuccess},
						{Kind: store.ComponentButton, Label: "Failed", CustomID: "failed", Style: 4, State: store.ComponentStateError},
						{Kind: store.ComponentButton, Label: "Forbidden Art", CustomID: "forbidden", Style: 4, Disabled: true},
					}},
					{Kind: store.ComponentSelect, Placeholder: "Choose technique", CustomID: "technique", Options: []store.ComponentOption{
						{Label: "Cloud Step", Value: "cloud_step", Description: "Mobility"},
						{Label: "Iron Bone", Value: "iron_bone", Description: "Defense"},
					}},
					{Kind: store.ComponentActionRow, Children: []store.ComponentNode{
						{Kind: store.ComponentLinkButton, Label: "Manual", URL: "https://example.invalid/dao", Style: 5},
						{Kind: store.ComponentButton, Label: "Inventory", CustomID: "inventory", Style: 2},
						{Kind: store.ComponentButton, Label: "Sect", CustomID: "sect", Style: 2},
						{Kind: store.ComponentButton, Label: "Tribulation", CustomID: "tribulation", Style: 4},
						{Kind: store.ComponentSelect, Placeholder: "Choose user", CustomID: "user_select", Disabled: true},
						{Kind: store.ComponentSelect, Placeholder: "Choose role", CustomID: "role_select", Disabled: true},
						{Kind: store.ComponentSelect, Placeholder: "Choose channel", CustomID: "channel_select", Disabled: true},
					}},
				},
			}},
		},
		{
			ID:        3,
			ChannelID: channelID,
			Author:    "Gallery Bot",
			Flags:     1 << 15,
			ComponentTree: []store.ComponentNode{{
				Kind:        store.ComponentContainer,
				AccentColor: 0xFEE75C,
				Children: []store.ComponentNode{
					{Kind: store.ComponentTextDisplay, Content: "**Media gallery shapes**"},
					{Kind: store.ComponentMediaGallery, Media: []store.ComponentMedia{
						{Description: "single item gallery", URL: "mock://one"},
					}},
					{Kind: store.ComponentMediaGallery, Media: []store.ComponentMedia{
						{Description: "two item A", URL: "mock://two-a"},
						{Description: "two item B", URL: "mock://two-b"},
					}},
					{Kind: store.ComponentMediaGallery, Media: []store.ComponentMedia{
						{Description: "three item A", URL: "mock://three-a"},
						{Description: "three item B", URL: "mock://three-b"},
						{Description: "spoiler art", URL: "mock://three-c", Spoiler: true},
					}},
					{Kind: store.ComponentMediaGallery, Media: []store.ComponentMedia{
						{Description: "ten item 1", URL: "mock://ten-1"},
						{Description: "ten item 2", URL: "mock://ten-2"},
						{Description: "ten item 3", URL: "mock://ten-3"},
						{Description: "ten item 4", URL: "mock://ten-4"},
						{Description: "ten item 5", URL: "mock://ten-5"},
						{Description: "ten item 6", URL: "mock://ten-6"},
						{Description: "ten item 7", URL: "mock://ten-7"},
						{Description: "ten item 8", URL: "mock://ten-8"},
						{Description: "ten item 9", URL: "mock://ten-9"},
						{Description: "ten item 10", URL: "mock://ten-10"},
					}},
					{Kind: store.ComponentFile, Media: []store.ComponentMedia{{Name: "dao-log.txt", Size: 2048, URL: "attachment://dao-log.txt"}}},
					{Kind: store.ComponentSeparator, Divider: true, Spacing: 2},
					{Kind: store.ComponentSeparator, Divider: false, Spacing: 1},
				},
			}},
		},
		{
			ID:        4,
			ChannelID: channelID,
			Author:    "Combat Bot",
			Flags:     1 << 15,
			ComponentTree: []store.ComponentNode{{
				Kind:        store.ComponentContainer,
				AccentColor: 0xED4245,
				Children: []store.ComponentNode{
					{Kind: store.ComponentTextDisplay, Content: "**Combat**\n[enemy HP]: `||||||---- 60%`\n[user HP] : `|||||||||| 100%`\nTechniques: SSS heaven ; SS hope ; S light"},
					{Kind: store.ComponentSelect, Placeholder: "Choose a technique", CustomID: "combat_technique", Options: []store.ComponentOption{
						{Label: "Heaven", Value: "heaven", Description: "SSS technique"},
						{Label: "Hope", Value: "hope", Description: "SS technique"},
						{Label: "Light", Value: "light", Description: "S technique"},
					}},
					{Kind: store.ComponentActionRow, Children: []store.ComponentNode{
						{Kind: store.ComponentButton, Label: "Strike", CustomID: "combat_strike", Style: 4},
						{Kind: store.ComponentButton, Label: "Guard", CustomID: "combat_guard", Style: 2},
						{Kind: store.ComponentButton, Label: "Recover", CustomID: "combat_recover", Style: 3},
						{Kind: store.ComponentButton, Label: "Flee", CustomID: "combat_flee", Style: 2},
					}},
				},
			}},
		},
		{
			ID:        5,
			ChannelID: channelID,
			Author:    "Inputs Bot",
			Flags:     1 << 15,
			ComponentTree: []store.ComponentNode{{
				Kind:        store.ComponentContainer,
				AccentColor: 0xEB459E,
				Children: []store.ComponentNode{
					{Kind: store.ComponentTextDisplay, Content: "**Input component coverage**"},
					{Kind: store.ComponentLabel, Label: "Battle note", Description: "Text input wrapped by a label.", Required: true, Children: []store.ComponentNode{{
						Kind:        store.ComponentTextInput,
						CustomID:    "battle_note",
						Placeholder: "Type a tactic",
						InputField:  true,
						Required:    true,
					}}},
					{Kind: store.ComponentRadioGroup, Label: "Choose stance", CustomID: "stance", InputField: true, Options: []store.ComponentOption{
						{Label: "Aggressive", Value: "aggressive", Description: "More damage", Default: true},
						{Label: "Balanced", Value: "balanced", Description: "Steady"},
						{Label: "Defensive", Value: "defensive", Description: "Less incoming damage"},
					}},
					{Kind: store.ComponentCheckboxGroup, Label: "Preparations", CustomID: "prep", InputField: true, Options: []store.ComponentOption{
						{Label: "Potion ready", Value: "potion", Default: true},
						{Label: "Talisman equipped", Value: "talisman"},
						{Label: "Formation active", Value: "formation"},
					}},
					{Kind: store.ComponentActionRow, Children: []store.ComponentNode{
						{Kind: store.ComponentCheckbox, Label: "Auto counter", CustomID: "auto_counter", InputField: true, Value: "true"},
						{Kind: store.ComponentFileUpload, Label: "Upload log", CustomID: "combat_log", InputField: true},
					}},
				},
			}},
		},
	}
}
