package ui

import (
	"awesomeProject/internal/config"
	"awesomeProject/internal/tui/screen"
)

// Version returns the current shared style generation.
func (s Styles) Version() uint64 {
	if s.State == nil {
		return 0
	}
	return s.State.Generation
}

// Install repopulates shared semantic maps and refreshes the legacy aliases on
// this Styles value. Other copies resolve Cells live; MainView/Shell are given
// the refreshed value for straightforward snapshot-style widgets.
func (s *Styles) Install(cells map[string]screen.Style, custom map[string]bool) {
	if s == nil {
		return
	}
	if s.Cells == nil {
		s.Cells = make(map[string]screen.Style)
	}
	for key := range s.Cells {
		delete(s.Cells, key)
	}
	for key, style := range cells {
		s.Cells[key] = style
	}
	if s.Custom == nil {
		s.Custom = make(map[string]bool)
	}
	for key := range s.Custom {
		delete(s.Custom, key)
	}
	for key, value := range custom {
		s.Custom[key] = value
	}
	s.Text = s.Cells["messages.content"]
	s.Muted = s.Cells["muted"]
	s.Accent = s.Cells["accent"]
	s.Border = s.Cells["panels.border"]
	s.Pending = s.Cells["pending"]
	s.Error = s.Cells["error"]
	if s.State == nil {
		s.State = &StyleState{}
	}
	s.State.Generation++
}

// Cell returns a semantic cell style, falling back to the legacy palette for
// callers that use a surface not present in Cells.
func (s Styles) Cell(name string) screen.Style {
	if style, ok := s.Cells[name]; ok {
		return config.ApplyColorRule(style, s.Overrides.Resolve(name))
	}
	var style screen.Style
	switch name {
	case "text", "messages.content":
		style = s.Text
	case "muted", "messages.thread", "messages.quote", "messages.code", "messages.attachment", "messages.reaction", "messages.timestamp":
		style = s.Muted
	case "messages.small":
		style = s.Muted
	case "accent", "messages.author", "messages.mention", "messages.roleMention", "messages.link", "messages.link.prettyLink", "messages.link.channel", "messages.link.message", "messages.link.invite":
		style = s.Accent
	case "messages.header1", "messages.header2", "messages.header3", "messages.header4", "messages.header5", "messages.header6":
		style := s.Accent
		style.Attrs |= screen.Bold | screen.Underline
		return config.ApplyColorRule(style, s.Overrides.Resolve(name))
	case "border", "panels.border":
		style = s.Border
	case "pending", "messages.pending":
		style = s.Pending
	case "error", "messages.failed":
		style = s.Error
	case "messages.focused":
		style = screen.Style{Attrs: screen.Reverse}
	case "messages.bold":
		style = screen.Style{Attrs: screen.Bold}
	case "messages.italic":
		style = screen.Style{Attrs: screen.Italic}
	case "messages.underlined":
		style = screen.Style{Attrs: screen.Underline}
	case "messages.strikethrough":
		style = screen.Style{Attrs: screen.Strike}
	case "messages.spoiler":
		style = screen.Style{Attrs: screen.Reverse}
	default:
		style = s.Text
	}
	return config.ApplyColorRule(style, s.Overrides.Resolve(name))
}

func (s Styles) HasCustom(name string) bool {
	return s.Custom[name] || s.Overrides.HasOverride(name)
}
