package ui

import (
	"strings"

	"awesomeProject/internal/app"
	"awesomeProject/internal/tui/input"
	"awesomeProject/internal/tui/layout"
	"awesomeProject/internal/tui/screen"
	tuitext "awesomeProject/internal/tui/text"
	"awesomeProject/internal/tui/tui"
	"awesomeProject/internal/tui/widget"
)

// CommandPicker is the experimental Discord slash-command catalog. It owns its
// query while open so catalog navigation never causes more REST requests.
type CommandPicker struct {
	commands []app.ApplicationCommand
	filtered []app.ApplicationCommand
	query    string
	list     *widget.ItemList
	body     *widget.Node
	node     layout.Node
	onPick   func(app.ApplicationCommand)
	onClose  func()
}

func NewCommandPicker(commands []app.ApplicationCommand, styles Styles, query string, onPick func(app.ApplicationCommand), onClose func()) *CommandPicker {
	p := &CommandPicker{
		commands: append([]app.ApplicationCommand(nil), commands...), query: query,
		list: widget.NewItemList(nil), onPick: onPick, onClose: onClose, node: layout.Node{Grow: 1},
	}
	p.list.SetStyle(styles.Text)
	p.list.SetSelectedStyle(styles.Accent)
	p.body = widget.Column(titled("Slash commands", p.list))
	p.body.Children()[0].Layout().Grow = 1
	p.refilter()
	return p
}

func (p *CommandPicker) refilter() {
	selected := p.list.Selected()
	p.filtered = p.filtered[:0]
	items := make([]widget.Item, 0, len(p.commands))
	query := strings.ToLower(strings.TrimSpace(p.query))
	for _, command := range p.commands {
		key := strings.ToLower(command.Name + " " + command.Description)
		if _, ok := fuzzyScore(key, query); !ok {
			continue
		}
		p.filtered = append(p.filtered, command)
		label := "/" + command.Name
		if command.Description != "" {
			label += " — " + command.Description
		}
		items = append(items, widget.Item{Label: label})
	}
	p.list.SetItems(items)
	p.list.SetSelectedSilent(selected)
}

func (p *CommandPicker) Children() []tui.Widget          { return []tui.Widget{p.body} }
func (p *CommandPicker) Measure(avail tui.Size) tui.Size { return p.body.Measure(avail) }
func (p *CommandPicker) Layout() *layout.Node {
	p.node.Children = []*layout.Node{p.body.Layout()}
	return &p.node
}
func (p *CommandPicker) Draw(screen.Region)   {}
func (p *CommandPicker) CanFocus() bool       { return true }
func (p *CommandPicker) PreferredFocus() bool { return true }

func (p *CommandPicker) Handle(ev tui.Event) bool {
	key, ok := ev.(input.KeyEvent)
	if !ok || key.Release {
		return false
	}
	switch key.Key {
	case input.KeyEsc:
		p.onClose()
		return true
	case input.KeyEnter:
		i := p.list.Selected()
		if i >= 0 && i < len(p.filtered) && p.onPick != nil {
			p.onPick(p.filtered[i])
		}
		return true
	case input.KeyUp, input.KeyDown, input.KeyHome, input.KeyEnd, input.KeyPageUp, input.KeyPageDown:
		return p.list.Handle(ev)
	case input.KeyBackspace:
		if p.query == "" {
			p.onClose()
			return true
		}
		p.query = p.query[:tuitext.PrevBoundary(p.query, len(p.query))]
		p.refilter()
		return true
	case input.KeyRune:
		p.query += string(key.Rune)
		p.refilter()
		return true
	}
	return false
}
