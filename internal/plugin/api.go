package plugin

import (
	"context"
	"os"
	"strconv"
	"strings"
	"time"

	"awesomeProject/internal/config"
	lua "github.com/yuin/gopher-lua"
)

// pluginContext carries everything a single plugin's tuicord API binds against.
type pluginContext struct {
	name            string
	host            *Host
	events          *eventBus
	commands        *commandRegistry
	keys            *keyRegistry
	themes          *themeRegistry
	log             func(msg string)
	grants          map[string]bool
	fsRoot          *os.Root
	configTarget    *config.Config
	configContext   bool
	starting        bool
	configured      bool
	startupTheme    string
	runtimeTheme    string
	applyTheme      func(string) error
	every           func(time.Duration, *lua.LFunction, *lua.LState)
	submit          func(func()) bool
	context         func() context.Context
	callbackTimeout time.Duration
	onCallbackError func(error)
	// isLive reports whether L is still a committed plugin state. Callbacks the
	// UI retains outside the registries (viewport on_press) consult it before
	// calling into L, which would panic once a rolled-back state is closed.
	isLive func(*lua.LState) bool
}

// installAPI builds the global `tuicord` table in L, wiring every binding to
// pctx. It also redirects the base `print` to the plugin log so plugin output
// never corrupts the terminal UI.
func installAPI(L *lua.LState, pctx *pluginContext) {
	tbl := L.NewTable()

	reg := func(name string, fn lua.LGFunction) {
		tbl.RawSetString(name, L.NewFunction(fn))
	}

	// --- events & commands ---------------------------------------------------
	reg("on", func(L *lua.LState) int {
		event := L.CheckString(1)
		fn := L.CheckFunction(2)
		pctx.events.on(event, L, fn, pctx.name)
		return 0
	})
	reg("command", func(L *lua.LState) int {
		name := strings.ToLower(L.CheckString(1))
		fn := L.CheckFunction(2)
		help := L.OptString(3, "")
		pctx.commands.add(name, handler{L: L, fn: fn, plugin: pctx.name, help: help})
		return 0
	})
	reg("keymap", func(L *lua.LState) int {
		spec := L.CheckString(1)
		fn := L.CheckFunction(2)
		pctx.keys.add(spec, handler{L: L, fn: fn, plugin: pctx.name})
		return 0
	})
	reg("every", func(L *lua.LState) int {
		milliseconds := L.CheckInt(1)
		fn := L.CheckFunction(2)
		if pctx.every != nil && milliseconds > 0 {
			pctx.every(time.Duration(milliseconds)*time.Millisecond, fn, L)
		}
		return 0
	})
	reg("now_ms", func(L *lua.LState) int {
		L.Push(lua.LNumber(time.Now().UnixMilli()))
		return 1
	})

	// --- actions -------------------------------------------------------------
	reg("send", func(L *lua.LState) int {
		content := L.CheckString(1)
		if pctx.host.Send != nil {
			pctx.host.Send(content)
		}
		return 0
	})
	reg("send_to", func(L *lua.LState) int {
		channel := parseID(L.Get(1))
		content := L.CheckString(2)
		if pctx.host.SendTo != nil {
			pctx.host.SendTo(channel, content)
		}
		return 0
	})
	reg("reply", func(L *lua.LState) int {
		channel := parseID(L.Get(1))
		message := parseID(L.Get(2))
		content := L.CheckString(3)
		mention := L.OptBool(4, false)
		if pctx.host.Reply != nil {
			pctx.host.Reply(channel, message, content, mention)
		}
		return 0
	})
	reg("react", func(L *lua.LState) int {
		channel := parseID(L.Get(1))
		message := parseID(L.Get(2))
		emoji := L.CheckString(3)
		if pctx.host.React != nil {
			pctx.host.React(channel, message, emoji)
		}
		return 0
	})
	reg("click", func(L *lua.LState) int {
		channel := parseID(L.Get(1))
		message := parseID(L.Get(2))
		customID := L.CheckString(3)
		if pctx.host.SubmitComponent != nil {
			pctx.host.SubmitComponent(channel, message, 2, customID, nil)
		}
		return 0
	})
	reg("select", func(L *lua.LState) int {
		channel := parseID(L.Get(1))
		message := parseID(L.Get(2))
		customID := L.CheckString(3)
		values := tableToStringSlice(L.CheckTable(4))
		if pctx.host.SubmitComponent != nil {
			pctx.host.SubmitComponent(channel, message, 3, customID, values)
		}
		return 0
	})
	reg("notify", func(L *lua.LState) int {
		title := L.CheckString(1)
		body := L.OptString(2, "")
		if pctx.host.Notify != nil {
			pctx.host.Notify(title, body)
		}
		return 0
	})

	// --- declarative config, theming & custom UI -----------------------------
	reg("configure", func(L *lua.LState) int {
		if !pctx.configContext || pctx.configTarget == nil || !pctx.starting {
			L.RaiseError("tuicord.configure is only available while loading config.lua")
			return 0
		}
		if pctx.configured {
			L.RaiseError("tuicord.configure may only be called once")
			return 0
		}
		if err := config.DecodeLua(pctx.configTarget, L.CheckTable(1)); err != nil {
			L.RaiseError("configure: %v", err)
			return 0
		}
		pctx.configured = true
		return 0
	})
	reg("style", func(L *lua.LState) int {
		selector := L.CheckString(1)
		props := tableToStringMap(L.CheckTable(2))
		validated := config.ColorOverrides{Rules: make(map[string]config.ColorRule)}
		for property, value := range props {
			if err := validated.SetProperty(selector, property, value); err != nil {
				L.RaiseError("style %q.%s: %v", selector, property, err)
				return 0
			}
		}
		if pctx.host.Style != nil {
			pctx.host.Style(selector, props)
		}
		return 0
	})
	reg("overlay", func(L *lua.LState) int {
		title := L.CheckString(1)
		lines := tableToStringSlice(L.CheckTable(2))
		if pctx.host.OpenOverlay != nil {
			pctx.host.OpenOverlay(title, lines)
		}
		return 0
	})
	reg("viewport", func(L *lua.LState) int {
		title := L.CheckString(1)
		lines := tableToStringSlice(L.CheckTable(2))
		actionTable := L.CheckTable(3)
		actions := make([]ViewportAction, 0, actionTable.Len())
		callbacks := make(map[string]*lua.LFunction, actionTable.Len())
		actionTable.ForEach(func(_ lua.LValue, value lua.LValue) {
			action, ok := value.(*lua.LTable)
			if !ok {
				L.RaiseError("viewport actions must be tables")
				return
			}
			id := action.RawGetString("id").String()
			label := action.RawGetString("label").String()
			callback, ok := action.RawGetString("on_press").(*lua.LFunction)
			if id == "" || label == "" || !ok || callbacks[id] != nil {
				L.RaiseError("viewport actions require unique id, label, and on_press")
				return
			}
			actions = append(actions, ViewportAction{ID: id, Label: label})
			callbacks[id] = callback
		})
		if pctx.host.OpenViewport != nil {
			pctx.host.OpenViewport(title, lines, actions, func(id string) {
				callback := callbacks[id]
				if callback == nil || pctx.submit == nil || pctx.context == nil || pctx.onCallbackError == nil {
					return
				}
				pctx.submit(func() {
					// The UI holds this closure outside the registries; if the
					// plugin's startup was rolled back and its state closed, the
					// viewport must not call into it.
					if pctx.isLive != nil && !pctx.isLive(L) {
						return
					}
					if err := safeCall(pctx.context(), L, callback, pctx.callbackTimeout); err != nil {
						pctx.onCallbackError(err)
					}
				})
			})
		}
		return 0
	})
	reg("theme", func(L *lua.LState) int {
		name := strings.TrimSpace(L.CheckString(1))
		if name == "" {
			L.RaiseError("theme name must not be empty")
			return 0
		}
		theme, err := decodeTheme(L.CheckTable(2))
		if err != nil {
			L.RaiseError("theme %q: %v", name, err)
			return 0
		}
		pctx.themes.add(name, theme, L)
		return 0
	})
	reg("use_theme", func(L *lua.LState) int {
		name := strings.TrimSpace(L.CheckString(1))
		if _, ok := pctx.themes.lookup(name); !ok {
			L.RaiseError("unknown theme %q", name)
			return 0
		}
		if pctx.starting {
			if pctx.configContext {
				pctx.startupTheme = name
			} else {
				pctx.runtimeTheme = name
			}
			return 0
		}
		if pctx.applyTheme != nil {
			if err := pctx.applyTheme(name); err != nil {
				L.RaiseError("use theme %q: %v", name, err)
			}
		}
		return 0
	})

	// --- state accessors -----------------------------------------------------
	reg("active_channel", func(L *lua.LState) int {
		L.Push(idValue(pctx.host.ActiveChannel))
		return 1
	})
	reg("active_guild", func(L *lua.LState) int {
		L.Push(idValue(pctx.host.ActiveGuild))
		return 1
	})
	reg("self_id", func(L *lua.LState) int {
		L.Push(idValue(pctx.host.SelfID))
		return 1
	})

	// --- logging -------------------------------------------------------------
	reg("log", func(L *lua.LState) int {
		pctx.log(joinArgs(L))
		return 0
	})

	installGrants(L, tbl, pctx)

	L.SetGlobal("tuicord", tbl)
	tbl.RawSetString("name", lua.LString(pctx.name))

	// Redirect print to the plugin log; stdout is the live terminal UI.
	L.SetGlobal("print", L.NewFunction(func(L *lua.LState) int {
		pctx.log(joinArgs(L))
		return 0
	}))
}

// idValue renders an ID accessor's result as a decimal string, or "0" when the
// accessor is unset.
func idValue(fn func() uint64) lua.LValue {
	if fn == nil {
		return lua.LString("0")
	}
	return lua.LString(strconv.FormatUint(fn(), 10))
}

// joinArgs concatenates all call arguments with spaces, mirroring Lua's print.
func joinArgs(L *lua.LState) string {
	var b strings.Builder
	top := L.GetTop()
	for i := 1; i <= top; i++ {
		if i > 1 {
			b.WriteByte(' ')
		}
		b.WriteString(L.ToStringMeta(L.Get(i)).String())
	}
	return b.String()
}
