package ui

import (
	"strings"

	"awesomeProject/internal/tui/input"
	"awesomeProject/internal/tui/layout"
	"awesomeProject/internal/tui/screen"
	"awesomeProject/internal/tui/tui"
	"awesomeProject/internal/tui/widget"
)

// Prompt is a single-line text overlay used for short inputs like a new thread
// name. Enter confirms (invoking onSubmit with the trimmed value when non-empty);
// Esc cancels. It follows the QuickSwitcher pattern: the input holds focus and
// confirmation/dismissal arrive via Handle from the root fallback.
type Prompt struct {
	input    *widget.TextInput
	body     *widget.Node
	node     layout.Node
	title    string
	onSubmit func(string)
	onClose  func()
}

// NewPrompt builds a prompt titled title with the given placeholder. onSubmit
// receives the trimmed, non-empty value; onClose dismisses the overlay.
func NewPrompt(title, placeholder string, styles Styles, onSubmit func(string), onClose func()) *Prompt {
	p := &Prompt{
		input:    widget.NewTextInput(placeholder),
		title:    title,
		onSubmit: onSubmit,
		onClose:  onClose,
		node:     layout.Node{Grow: 1},
	}
	p.input.SetStyle(styles.Text)
	p.input.OnSubmit(func(string) { p.confirm() })
	p.body = widget.Column(titled(title, p.input))
	p.body.Children()[0].Layout().Basis = 3
	p.body.Children()[0].Layout().Grow = 0
	return p
}

// Children exposes the composed body.
func (p *Prompt) Children() []tui.Widget { return []tui.Widget{p.body} }

// Measure delegates to the body.
func (p *Prompt) Measure(avail tui.Size) tui.Size { return p.body.Measure(avail) }

// Layout returns the prompt layout node.
func (p *Prompt) Layout() *layout.Node {
	p.node.Children = []*layout.Node{p.body.Layout()}
	return &p.node
}

// Draw is a no-op; children draw themselves.
func (p *Prompt) Draw(screen.Region) {}

// CanFocus lets the prompt receive keys.
func (p *Prompt) CanFocus() bool { return true }

// Handle confirms on Enter and dismisses on Esc.
func (p *Prompt) Handle(ev tui.Event) bool {
	key, ok := ev.(input.KeyEvent)
	if !ok || key.Release {
		return false
	}
	switch key.Key {
	case input.KeyEsc:
		p.onClose()
		return true
	case input.KeyEnter:
		p.confirm()
		return true
	}
	return false
}

func (p *Prompt) confirm() {
	value := strings.TrimSpace(p.input.Value())
	if value == "" {
		p.onClose()
		return
	}
	if p.onSubmit != nil {
		p.onSubmit(value)
	}
	p.onClose()
}
