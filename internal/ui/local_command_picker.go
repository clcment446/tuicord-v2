package ui

import (
	"strings"

	"awesomeProject/internal/tui/input"
	"awesomeProject/internal/tui/layout"
	"awesomeProject/internal/tui/screen"
	tuitext "awesomeProject/internal/tui/text"
	"awesomeProject/internal/tui/tui"
	"awesomeProject/internal/tui/widget"
)

type localCommandSpec struct {
	Name        string
	Description string
}

// LocalCommandPicker completes the local ';' command namespace. It deliberately
// only inserts a command name; arguments remain ordinary composer input.
type LocalCommandPicker struct {
	commands []localCommandSpec
	search   *queryList[localCommandSpec]
	body     *widget.Node
	node     layout.Node
	onPick   func(string)
	onClose  func()
}

func NewLocalCommandPicker(commands []localCommandSpec, styles Styles, query string, onPick func(string), onClose func()) *LocalCommandPicker {
	p := &LocalCommandPicker{
		commands: append([]localCommandSpec(nil), commands...), onPick: onPick, onClose: onClose, node: layout.Node{Grow: 1},
	}
	list := widget.NewItemList(nil)
	list.SetStyle(styles.Cell("picker"))
	list.SetSelectedStyle(styles.Cell("picker.selected"))
	p.search = newQueryList(list, p.filter)
	p.search.SetQuery(query)
	p.body = widget.Column(titled(styles, "Local commands", list))
	p.body.Children()[0].Layout().Grow = 1
	return p
}

func (p *LocalCommandPicker) filter(query string) ([]localCommandSpec, []widget.Item) {
	query = strings.ToLower(strings.TrimSpace(query))
	commands := make([]localCommandSpec, 0, len(p.commands))
	items := make([]widget.Item, 0, len(p.commands))
	for _, command := range p.commands {
		if _, ok := fuzzyScore(strings.ToLower(command.Name+" "+command.Description), query); !ok {
			continue
		}
		commands = append(commands, command)
		label := ";" + command.Name
		if command.Description != "" {
			label += " — " + command.Description
		}
		items = append(items, widget.Item{Label: label})
	}
	return commands, items
}

func (p *LocalCommandPicker) Query() string                   { return p.search.Query() }
func (p *LocalCommandPicker) Children() []tui.Widget          { return []tui.Widget{p.body} }
func (p *LocalCommandPicker) Measure(avail tui.Size) tui.Size { return p.body.Measure(avail) }
func (p *LocalCommandPicker) Layout() *layout.Node {
	p.node.Children = []*layout.Node{p.body.Layout()}
	return &p.node
}
func (*LocalCommandPicker) Draw(screen.Region)   {}
func (*LocalCommandPicker) CanFocus() bool       { return true }
func (*LocalCommandPicker) PreferredFocus() bool { return true }

func (p *LocalCommandPicker) Handle(ev tui.Event) bool {
	key, ok := ev.(input.KeyEvent)
	if !ok || key.Release {
		return false
	}
	switch key.Key {
	case input.KeyEsc:
		p.onClose()
		return true
	case input.KeyEnter:
		if command, ok := p.search.Selected(); ok && p.onPick != nil {
			p.onPick(command.Name)
		}
		return true
	case input.KeyUp, input.KeyDown, input.KeyHome, input.KeyEnd, input.KeyPageUp, input.KeyPageDown:
		return p.search.List().Handle(ev)
	case input.KeyBackspace:
		query := p.search.Query()
		if query == "" {
			p.onClose()
			return true
		}
		p.search.SetQuery(query[:tuitext.PrevBoundary(query, len(query))])
		return true
	case input.KeyRune:
		p.search.SetQuery(p.search.Query() + string(key.Rune))
		return true
	}
	return false
}
