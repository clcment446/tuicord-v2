package config

import (
	"fmt"
	"io"
	"reflect"
	"sort"
	"strconv"
	"strings"

	"awesomeProject/internal/tui/screen"
)

func writeLuaMigration(path string, cfg Config) error {
	contents, err := renderLuaConfig(cfg, "legacy-migrated")
	if err != nil {
		return err
	}
	return writeFirstRunFile(path, contents)
}

func renderLuaConfig(cfg Config, themeName string) (string, error) {
	if err := ValidateColors(cfg.Colors); err != nil {
		return "", fmt.Errorf("render Lua config: colors: %w", err)
	}
	var b strings.Builder
	b.WriteString("-- Generated atomically from legacy config.toml and colors.conf.\n")
	b.WriteString("-- The legacy files are intentionally left untouched.\n\n")
	b.WriteString("tuicord.configure(")
	if err := writeLuaValue(&b, reflect.ValueOf(cfg), 0, true); err != nil {
		return "", err
	}
	b.WriteString(")\n\n")
	fmt.Fprintf(&b, "tuicord.theme(%s, {\n  palette = {\n", strconv.Quote(themeName))
	for _, entry := range []struct{ key, value string }{
		{"background", cfg.Colors.Background}, {"text", cfg.Colors.Text}, {"muted", cfg.Colors.Muted},
		{"accent", cfg.Colors.Accent}, {"selection", cfg.Colors.Selection}, {"border", cfg.Colors.Border}, {"error", cfg.Colors.Error},
	} {
		fmt.Fprintf(&b, "    %s = %s,\n", entry.key, strconv.Quote(entry.value))
	}
	b.WriteString("  },\n  styles = {")
	if cfg.ColorOverrides != nil && len(cfg.ColorOverrides.Rules) > 0 {
		b.WriteByte('\n')
		selectors := make([]string, 0, len(cfg.ColorOverrides.Rules))
		for selector := range cfg.ColorOverrides.Rules {
			selectors = append(selectors, selector)
		}
		sort.Strings(selectors)
		for _, selector := range selectors {
			rule := cfg.ColorOverrides.Rules[selector]
			fmt.Fprintf(&b, "    [%s] = {", strconv.Quote(selector))
			var props []string
			if rule.HasFg {
				props = append(props, "fg = "+strconv.Quote(colorHex(rule.Fg)))
			}
			if rule.HasBg {
				props = append(props, "bg = "+strconv.Quote(colorHex(rule.Bg)))
			}
			if rule.HasAttrs {
				props = append(props, "attrs = "+strconv.Quote(attrString(rule.Attrs)))
			}
			b.WriteString(strings.Join(props, ", "))
			b.WriteString("},\n")
		}
		b.WriteString("  ")
	}
	b.WriteString("},\n})\n")
	fmt.Fprintf(&b, "tuicord.use_theme(%s)\n", strconv.Quote(themeName))
	return b.String(), nil
}

func writeLuaValue(w io.Writer, value reflect.Value, indent int, root bool) error {
	if value.Kind() == reflect.Pointer {
		if value.IsNil() {
			_, _ = io.WriteString(w, "nil")
			return nil
		}
		return writeLuaValue(w, value.Elem(), indent, false)
	}
	switch value.Kind() {
	case reflect.Struct:
		_, _ = io.WriteString(w, "{\n")
		for i := 0; i < value.NumField(); i++ {
			field := value.Type().Field(i)
			name := strings.Split(field.Tag.Get("toml"), ",")[0]
			if name == "" {
				name = strings.ToLower(field.Name)
			}
			if name == "-" || field.PkgPath != "" || (root && (name == "accounts" || name == "auth")) {
				continue
			}
			child := value.Field(i)
			if child.Kind() == reflect.Pointer && child.IsNil() {
				continue
			}
			fmt.Fprintf(w, "%s%s = ", strings.Repeat("  ", indent+1), name)
			if err := writeLuaValue(w, child, indent+1, false); err != nil {
				return err
			}
			_, _ = io.WriteString(w, ",\n")
		}
		fmt.Fprintf(w, "%s}", strings.Repeat("  ", indent))
		return nil
	case reflect.Map:
		_, _ = io.WriteString(w, "{")
		if value.Len() > 0 {
			_, _ = io.WriteString(w, "\n")
			keys := value.MapKeys()
			sort.Slice(keys, func(i, j int) bool { return keys[i].String() < keys[j].String() })
			for _, key := range keys {
				fmt.Fprintf(w, "%s[%s] = ", strings.Repeat("  ", indent+1), strconv.Quote(key.String()))
				if err := writeLuaValue(w, value.MapIndex(key), indent+1, false); err != nil {
					return err
				}
				_, _ = io.WriteString(w, ",\n")
			}
			fmt.Fprint(w, strings.Repeat("  ", indent))
		}
		_, _ = io.WriteString(w, "}")
		return nil
	case reflect.Slice:
		_, _ = io.WriteString(w, "{")
		for i := 0; i < value.Len(); i++ {
			if i > 0 {
				_, _ = io.WriteString(w, ", ")
			}
			if err := writeLuaValue(w, value.Index(i), indent, false); err != nil {
				return err
			}
		}
		_, _ = io.WriteString(w, "}")
		return nil
	case reflect.Bool:
		_, _ = io.WriteString(w, strconv.FormatBool(value.Bool()))
	case reflect.String:
		_, _ = io.WriteString(w, strconv.Quote(value.String()))
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		_, _ = io.WriteString(w, strconv.FormatInt(value.Int(), 10))
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		_, _ = io.WriteString(w, strconv.FormatUint(value.Uint(), 10))
	default:
		return fmt.Errorf("render Lua config: unsupported %s", value.Type())
	}
	return nil
}

func attrString(attrs screen.Attr) string {
	var names []string
	for _, entry := range []struct {
		attr screen.Attr
		name string
	}{{screen.Bold, "bold"}, {screen.Dim, "dim"}, {screen.Italic, "italic"}, {screen.Underline, "underline"}, {screen.Reverse, "reverse"}, {screen.Strike, "strike"}} {
		if attrs&entry.attr != 0 {
			names = append(names, entry.name)
		}
	}
	return strings.Join(names, "|")
}

const defaultLuaConfigTemplate = `-- tuicord-v2 configuration
--
-- config.lua is the primary authored configuration. tuicord.configure overlays
-- these values onto the built-in defaults. Existing config.lua files that do
-- not call configure remain valid and simply use all defaults.

tuicord.configure({
  layout = {
    guilds_width = 3,
    channels_width = 20,
    members_width = 20,
    members_auto_hide = true,
    members_hide_below = 120,
  },
  display = {
    ascii = false,
    tty_colors = false,
    role_gradients = false,
    role_gradient_animations = false,
    -- Set false to keep GIF and role-gradient animation enabled over SSH.
    no_animations_over_ssh = true,
    sticky_anchor = true,
  },
  accessibility = {
    mouse_on = true,
    focus_splits = false,
    vim_navigation = false,
    mouse_breakpoint_tracking = false,
    highlight_focus_block = false,
  },
})

-- Themes may use the old flat seven-color table, but the nested form also
-- carries validated semantic cell styles. Partial palettes inherit from the
-- built-in default deterministically.
tuicord.theme("catppuccin-latte", {
  palette = {
    background = "#eff1f5",
    text       = "#4c4f69",
    muted      = "#8c8fa1",
    accent     = "#1e66f5",
    selection  = "#ccd0da",
    border     = "#bcc0cc",
    error      = "#d20f39",
  },
  styles = {
    ["messages.author"] = { bold = true },
    ["guilds.selected"] = { bold = true },
    ["quick_switcher.selected"] = { bold = true },
  },
})

tuicord.use_theme("catppuccin-latte")
`
