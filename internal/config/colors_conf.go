package config

import (
	"bufio"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"

	"awesomeProject/internal/tui/screen"
)

// ColorRule is a partial cell-style override. Unset fields inherit from the
// semantic default, so a rule can change only a foreground, background, or
// attribute without disturbing the rest of the cell.
type ColorRule struct {
	Fg       screen.Color
	Bg       screen.Color
	Attrs    screen.Attr
	HasFg    bool
	HasBg    bool
	HasAttrs bool
}

// loadColorOverrides parses colors.conf rules. Selectors are retained rather
// than hard-coded so new UI surfaces can become customizable without changing
// the file format.
func loadColorOverrides(path string) (ColorOverrides, error) {
	file, err := os.Open(path)
	if err != nil {
		return ColorOverrides{}, err
	}
	defer file.Close()

	overrides := ColorOverrides{Rules: make(map[string]ColorRule)}
	scanner := bufio.NewScanner(file)
	line := 0
	for scanner.Scan() {
		line++
		text := strings.TrimSpace(scanner.Text())
		if text == "" || strings.HasPrefix(text, "#") {
			continue
		}
		if comment := strings.Index(text, "//"); comment >= 0 {
			text = strings.TrimSpace(text[:comment])
		}
		key, value, ok := strings.Cut(text, "=")
		if !ok {
			return ColorOverrides{}, fmt.Errorf("%s:%d: want key=value", path, line)
		}
		if err := overrides.set(strings.TrimSpace(key), strings.TrimSpace(value)); err != nil {
			return ColorOverrides{}, fmt.Errorf("%s:%d: %w", path, line, err)
		}
	}
	if err := scanner.Err(); err != nil {
		return ColorOverrides{}, err
	}
	return overrides, nil
}

func (o *ColorOverrides) set(key, value string) error {
	parts := strings.Split(strings.ReplaceAll(strings.TrimSpace(key), ",", "."), ".")
	if len(parts) < 2 {
		return fmt.Errorf("unsupported selector %q", key)
	}
	property := strings.ReplaceAll(parts[len(parts)-1], "-", "_")
	selector := strings.Join(parts[:len(parts)-1], ".")
	selector = strings.ReplaceAll(selector, "{n}", "*")
	rule := o.Rules[selector]
	switch property {
	case "fg", "fg_color":
		color, err := ParseColor(value)
		if err != nil {
			return err
		}
		rule.Fg, rule.HasFg = color, true
	case "bg", "bg_color":
		color, err := ParseColor(value)
		if err != nil {
			return err
		}
		rule.Bg, rule.HasBg = color, true
	case "attrs":
		attrs, err := parseAttrs(value)
		if err != nil {
			return err
		}
		rule.Attrs, rule.HasAttrs = attrs, true
	case "bold", "italic", "underline", "underlined", "strike", "strikethrough", "spoiler":
		enabled, err := strconv.ParseBool(strings.TrimSpace(value))
		if err != nil {
			return fmt.Errorf("invalid boolean %q", value)
		}
		attr, err := parseAttr(property)
		if err != nil {
			return err
		}
		if enabled {
			rule.Attrs |= attr
		}
		rule.HasAttrs = true
	default:
		return fmt.Errorf("unsupported color property %q", property)
	}
	o.Rules[selector] = rule
	return nil
}

func parseAttrs(value string) (screen.Attr, error) {
	var attrs screen.Attr
	for _, name := range strings.FieldsFunc(strings.ToLower(value), func(r rune) bool { return r == ',' || r == '|' || r == ' ' }) {
		attr, err := parseAttr(name)
		if err != nil {
			return 0, err
		}
		attrs |= attr
	}
	return attrs, nil
}

func parseAttr(name string) (screen.Attr, error) {
	switch strings.ReplaceAll(name, "-", "_") {
	case "bold":
		return screen.Bold, nil
	case "dim":
		return screen.Dim, nil
	case "italic":
		return screen.Italic, nil
	case "underline", "underlined":
		return screen.Underline, nil
	case "reverse":
		return screen.Reverse, nil
	case "strike", "strikethrough":
		return screen.Strike, nil
	default:
		return 0, fmt.Errorf("unsupported attribute %q", name)
	}
}

// Resolve returns the most specific matching rule. A * matches one selector
// segment; exact rules win over wildcard rules independently per attribute.
func (o *ColorOverrides) Resolve(selector string) ColorRule {
	if o == nil {
		return ColorRule{}
	}
	parts := strings.Split(selector, ".")
	var out ColorRule
	best := [3]int{-1, -1, -1}
	for pattern, rule := range o.Rules {
		patternParts := strings.Split(pattern, ".")
		if len(patternParts) != len(parts) {
			continue
		}
		specificity := 0
		matched := true
		for i := range parts {
			if patternParts[i] == "*" {
				continue
			}
			if strings.HasSuffix(patternParts[i], "*") && strings.HasPrefix(parts[i], strings.TrimSuffix(patternParts[i], "*")) {
				specificity++
				continue
			}
			if patternParts[i] != parts[i] {
				matched = false
				break
			}
			specificity += 2
		}
		if !matched {
			continue
		}
		if rule.HasFg && specificity > best[0] {
			out.Fg, out.HasFg, best[0] = rule.Fg, true, specificity
		}
		if rule.HasBg && specificity > best[1] {
			out.Bg, out.HasBg, best[1] = rule.Bg, true, specificity
		}
		if rule.HasAttrs && specificity > best[2] {
			out.Attrs, out.HasAttrs, best[2] = rule.Attrs, true, specificity
		}
	}
	return out
}

// HasOverride reports whether selector has at least one matching custom cell
// attribute. It lets dynamic Discord colors yield to an explicit user rule.
func (o *ColorOverrides) HasOverride(selector string) bool {
	rule := o.Resolve(selector)
	return rule.HasFg || rule.HasBg || rule.HasAttrs
}

// CellStyles returns the complete semantic cell palette used by the UI.
// Every entry is a screen.Style because the final renderer draws cells, not
// abstract color names.
func CellStyles(colors ColorStyles, overrides *ColorOverrides) map[string]screen.Style {
	bold := func(style screen.Style) screen.Style { style.Attrs |= screen.Bold; return style }
	underline := func(style screen.Style) screen.Style { style.Attrs |= screen.Underline; return style }
	muted := colors.Muted
	accent := colors.Accent
	styles := map[string]screen.Style{
		"background": {Bg: colors.Background},
		"text":       colors.Text, "muted": muted, "accent": accent, "error": colors.Error,
		"pending": muted, "panels.border": colors.Border, "panels.focus": accent,
		"guilds": colors.Text, "guilds.channels": colors.Text, "guilds.members": colors.Text,
		"guilds.header": bold(muted), "guilds.selected": accent, "guilds.badge": colors.Error,
		"guilds.separators.*": colors.Border, "guilds.separators.right": colors.Border,
		"messages.content": colors.Text, "messages.author": accent, "messages.pending": muted,
		"messages.attachment": muted, "messages.reaction": muted,
		"messages.failed": colors.Error, "messages.thread": muted, "messages.quote": muted,
		"messages.code": muted, "messages.bold": screen.Style{Attrs: screen.Bold},
		"messages.small":         muted,
		"messages.italic":        screen.Style{Attrs: screen.Italic},
		"messages.underlined":    screen.Style{Attrs: screen.Underline},
		"messages.strikethrough": screen.Style{Attrs: screen.Strike},
		"messages.spoiler":       screen.Style{Attrs: screen.Reverse},
		"messages.link":          underline(accent), "messages.link.prettyLink": underline(accent),
		"messages.link.channel": underline(accent), "messages.link.message": underline(accent),
		"messages.link.invite": underline(accent), "messages.mention": accent,
		"messages.roleMention": accent, "messages.timestamp": muted,
		"messages.reaction.selected": {Attrs: screen.Reverse},
		"embeds.border":              accent, "embeds.background": colors.Text, "embeds.author": muted,
		"embeds.title": bold(colors.Text), "embeds.title.link": {Attrs: screen.Underline},
		"embeds.field.name": {Attrs: screen.Bold}, "embeds.footer": muted,
		"components.border": accent, "components.background": colors.Text,
		"components.label": accent, "components.description": muted, "components.disabled": muted,
		"components.button": accent, "components.button.disabled": muted,
		"components.link": underline(accent), "components.pending": muted,
		"components.success": screen.Style{Fg: screen.RGB(64, 160, 43)},
		"components.error":   colors.Error,
		"composer":           colors.Text, "composer.status": muted, "toast": colors.Text,
		"toast.title": bold(colors.Error), "toast.detail": muted,
		"picker": colors.Text, "picker.selected": accent, "picker.hint": muted,
		"picker.favorite": accent, "picker.query": colors.Text,
		"menu": colors.Text, "menu.selected": accent, "menu.danger": bold(colors.Error),
		"menu.disabled": muted, "menu.key": muted,
		"settings": colors.Text, "settings.selected": accent,
		"settings.tab": muted, "settings.tab.active": accent,
		"forum.header": muted, "forum.body": colors.Text, "forum.selected": accent,
		"forum.badge": accent, "forum.filter": accent, "forum.archived": muted,
		"forum.title": colors.Text, "forum.tags": colors.Text, "forum.tags.selected": accent,
		"quick_switcher.input": colors.Text, "quick_switcher.selected": accent,
		"login.input": colors.Text, "prompt.input": colors.Text,
		"input.placeholder": muted, "input.cursor": {Attrs: screen.Reverse},
		"composer.placeholder": muted, "composer.cursor": {Attrs: screen.Reverse},
		"login.placeholder": muted, "login.cursor": {Attrs: screen.Reverse},
		"prompt.placeholder": muted, "prompt.cursor": {Attrs: screen.Reverse},
		"quick_switcher.placeholder": muted, "quick_switcher.cursor": {Attrs: screen.Reverse},
		"forum.title.placeholder": muted, "forum.title.cursor": {Attrs: screen.Reverse},
		"forum.body.placeholder": muted, "forum.body.cursor": {Attrs: screen.Reverse},
		"auth.qr": colors.Text, "auth.title": accent, "auth.hint": muted,
		"auth.status": muted, "auth.choice": colors.Text,
		"auth.qr.dark":       {Fg: screen.RGB(0, 0, 0), Bg: screen.RGB(255, 255, 255)},
		"auth.qr.light":      {Fg: screen.RGB(255, 255, 255), Bg: screen.RGB(0, 0, 0)},
		"preview.background": {Bg: colors.Background}, "preview.border": colors.Border,
		"preview.title": bold(accent), "preview.body": colors.Text,
	}
	for level := 1; level <= 6; level++ {
		style := accent
		switch level {
		case 1:
			style = bold(underline(style))
		case 2:
			style = bold(style)
		case 3:
			style = underline(style)
		case 4:
			style = bold(colors.Text)
		case 5, 6:
			style = colors.Text
		}
		styles[fmt.Sprintf("messages.header%d", level)] = style
	}
	for key, style := range styles {
		styles[key] = applyRule(style, overrides.Resolve(key))
	}
	return styles
}

func CustomCellKeys(styles map[string]screen.Style, overrides *ColorOverrides) map[string]bool {
	custom := make(map[string]bool)
	for key := range styles {
		if overrides.HasOverride(key) {
			custom[key] = true
		}
	}
	return custom
}

// ApplyColorOverrides preserves the legacy semantic palette API while using
// the generic selector resolver for the surfaces supported by that API.
func ApplyColorOverrides(styles ColorStyles, overrides *ColorOverrides) ColorStyles {
	styles.Text = applyRule(styles.Text, overrides.Resolve("guilds.channels"))
	styles.Border = applyRule(styles.Border, overrides.Resolve("guilds.separators.*"))
	styles.Border = applyRule(styles.Border, overrides.Resolve("guilds.separators.right"))
	return styles
}

func applyRule(style screen.Style, rule ColorRule) screen.Style {
	if rule.HasFg {
		style.Fg = rule.Fg
	}
	if rule.HasBg {
		style.Bg = rule.Bg
	}
	if rule.HasAttrs {
		style.Attrs = rule.Attrs
	}
	return style
}

// ApplyColorRule applies a partial override to a cell style. Widgets use this
// for newly introduced semantic selectors that are not yet in the base table.
func ApplyColorRule(style screen.Style, rule ColorRule) screen.Style {
	return applyRule(style, rule)
}

// colorsTemplate returns a fully commented colors.conf containing every
// semantic cell selector currently known by the UI. New selectors added to
// CellStyles automatically appear in the generated template.
func colorsTemplate() string {
	styles := CellStyles(Default().Colors.Styles(), nil)
	selectors := make([]string, 0, len(styles))
	seen := make(map[string]bool)
	for selector := range styles {
		canonical := selector
		for level := 1; level <= 6; level++ {
			canonical = strings.Replace(canonical, fmt.Sprintf("messages.header%d", level), "messages.header{n}", 1)
		}
		if !seen[canonical] {
			seen[canonical] = true
			selectors = append(selectors, canonical)
		}
	}
	sort.Strings(selectors)

	var b strings.Builder
	b.WriteString(`# tuicord-v2 semantic cell color overrides
#
# Every line is commented out intentionally: custom colors are opt-in.
# Uncomment a rule to override one cell style. Rules accept:
#   fg, fg_color       foreground color (#RRGGBB)
#   bg, bg_color       background color (#RRGGBB)
#   attrs              comma/pipe/space-separated attributes
#   bold, italic, underline, underlined, strike, strikethrough, spoiler = true|false
#
# Exact selectors override wildcard selectors. A * matches one segment;
# messages.header{n} matches messages.header1 through messages.header6.

# Property syntax examples:
# messages.author.fg=#ff00ff
# messages.author.fg_color=#ff00ff
# messages.author.bg=#ffffff
# messages.author.bg_color=#ffffff
# messages.author.attrs=bold|underline
# messages.author.bold=true
# messages.author.italic=false
# messages.author.underline=true
# messages.author.underlined=true
# messages.author.strike=true
# messages.author.strikethrough=true
# messages.author.spoiler=false

# Available semantic selectors:
`)
	for _, selector := range selectors {
		fmt.Fprintf(&b, "# %s.fg=#ffffff\n# %s.bg=#000000\n# %s.attrs=bold\n", selector, selector, selector)
	}
	return b.String()
}
