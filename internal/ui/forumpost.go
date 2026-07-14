package ui

import (
	"strings"

	"awesomeProject/internal/store"
	"awesomeProject/internal/tui/input"
	"awesomeProject/internal/tui/layout"
	"awesomeProject/internal/tui/screen"
	"awesomeProject/internal/tui/tui"
	"awesomeProject/internal/tui/widget"
)

// ForumPostPrompt collects the title, first-message body, and any number of
// forum tags before creating a post. Tab moves between fields, Space toggles a
// tag, and Ctrl+Enter submits.
type ForumPostPrompt struct {
	title, body *widget.TextInput
	tags        []store.Tag
	selected    map[uint64]bool
	tagList     *widget.ItemList
	focus       int
	onSubmit    func(string, string, []uint64)
	onClose     func()
	root        *widget.Node
	node        layout.Node
}

func NewForumPostPrompt(tags []store.Tag, styles Styles, onSubmit func(string, string, []uint64), onClose func()) *ForumPostPrompt {
	p := &ForumPostPrompt{title: widget.NewTextInput("Post title…"), body: widget.NewTextInput("First message…"), tags: append([]store.Tag(nil), tags...), selected: map[uint64]bool{}, tagList: widget.NewItemList(nil), onSubmit: onSubmit, onClose: onClose, node: layout.Node{Grow: 1}}
	p.title.SetStyle(styles.Text)
	p.body.SetStyle(styles.Text)
	p.tagList.SetStyle(styles.Text)
	p.tagList.SetSelectedStyle(styles.Accent)
	p.root = widget.Column(titled("Title", p.title), titled("Body", p.body), titled("Tags — Space toggles", p.tagList), widget.NewText("Tab fields · Ctrl+Enter create · Esc cancel"))
	p.root.Children()[0].Layout().Basis = 3
	p.root.Children()[1].Layout().Basis = 3
	p.root.Children()[2].Layout().Grow = 1
	p.root.Children()[3].Layout().Basis = 1
	p.refreshTags()
	p.setFocus(0)
	return p
}

func (p *ForumPostPrompt) SetTitle(v string)           { p.title.SetValue(v) }
func (p *ForumPostPrompt) SetBody(v string)            { p.body.SetValue(v) }
func (p *ForumPostPrompt) Children() []tui.Widget      { return []tui.Widget{p.root} }
func (p *ForumPostPrompt) Measure(s tui.Size) tui.Size { return p.root.Measure(s) }
func (p *ForumPostPrompt) Layout() *layout.Node {
	p.node.Children = []*layout.Node{p.root.Layout()}
	return &p.node
}
func (p *ForumPostPrompt) Draw(screen.Region) {}
func (p *ForumPostPrompt) CanFocus() bool     { return true }

func (p *ForumPostPrompt) Handle(ev tui.Event) bool {
	k, ok := ev.(input.KeyEvent)
	if !ok || k.Release {
		return false
	}
	if k.Key == input.KeyEsc {
		p.onClose()
		return true
	}
	if k.Key == input.KeyTab {
		p.setFocus((p.focus + 1) % 3)
		return true
	}
	if k.Key == input.KeyEnter && k.Mods&input.Ctrl != 0 {
		p.submit()
		return true
	}
	if p.focus == 2 {
		if k.Key == input.KeyRune && k.Rune == ' ' {
			p.toggleSelected()
			return true
		}
		return p.tagList.Handle(ev)
	}
	if p.focus == 0 {
		return p.title.Handle(ev)
	}
	return p.body.Handle(ev)
}

func (p *ForumPostPrompt) setFocus(n int) {
	p.focus = n
	p.title.SetFocused(n == 0)
	p.body.SetFocused(n == 1)
}
func (p *ForumPostPrompt) toggleSelected() {
	i := p.tagList.Selected()
	if i < 0 || i >= len(p.tags) {
		return
	}
	id := p.tags[i].ID
	p.selected[id] = !p.selected[id]
	p.refreshTags()
}
func (p *ForumPostPrompt) refreshTags() {
	items := make([]widget.Item, len(p.tags))
	for i, tag := range p.tags {
		mark := "[ ]"
		if p.selected[tag.ID] {
			mark = "[x]"
		}
		items[i] = widget.Item{Label: mark + " " + tag.Name}
	}
	p.tagList.SetItems(items)
}
func (p *ForumPostPrompt) submit() {
	title, body := strings.TrimSpace(p.title.Value()), strings.TrimSpace(p.body.Value())
	if title == "" || body == "" {
		return
	}
	ids := make([]uint64, 0, len(p.selected))
	for _, tag := range p.tags {
		if p.selected[tag.ID] {
			ids = append(ids, tag.ID)
		}
	}
	if p.onSubmit != nil {
		p.onSubmit(title, body, ids)
	}
	p.onClose()
}
