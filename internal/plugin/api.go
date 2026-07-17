package plugin

import (
	"strconv"
	"strings"

	lua "github.com/yuin/gopher-lua"
)

// pluginContext carries everything a single plugin's tuicord API binds against.
type pluginContext struct {
	name     string
	host     *Host
	events   *eventBus
	commands *commandRegistry
	keys     *keyRegistry
	log      func(msg string)
	grants   map[string]bool
	dataDir  string
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
	reg("notify", func(L *lua.LState) int {
		title := L.CheckString(1)
		body := L.OptString(2, "")
		if pctx.host.Notify != nil {
			pctx.host.Notify(title, body)
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
