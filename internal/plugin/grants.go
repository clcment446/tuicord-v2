package plugin

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	lua "github.com/yuin/gopher-lua"
)

// Capability names recognized in the per-plugin grants list.
const (
	CapFS  = "fs"
	CapNet = "net"
)

// installGrants adds capability helper tables to tuicord for the capabilities
// the user granted this plugin. Ungranted capabilities are simply absent, so a
// plugin calling tuicord.fs without the grant gets a clean "nil value" error.
func installGrants(L *lua.LState, tbl *lua.LTable, pctx *pluginContext) {
	if pctx.grants[CapFS] {
		tbl.RawSetString("fs", newFSTable(L, pctx))
	}
	if pctx.grants[CapNet] {
		tbl.RawSetString("http", newHTTPTable(L, pctx))
	}
}

// newFSTable exposes read/write/list confined to the plugin's own data
// directory. Paths are resolved under dataDir and any attempt to escape it
// (via .. or an absolute path) fails, so the grant cannot reach the wider
// filesystem.
func newFSTable(L *lua.LState, pctx *pluginContext) *lua.LTable {
	fs := L.NewTable()

	resolve := func(rel string) (string, bool) {
		clean := filepath.Clean("/" + rel) // force relative, collapse ..
		full := filepath.Join(pctx.dataDir, clean)
		if full != pctx.dataDir && !strings.HasPrefix(full, pctx.dataDir+string(os.PathSeparator)) {
			return "", false
		}
		return full, true
	}

	fs.RawSetString("read", L.NewFunction(func(L *lua.LState) int {
		path, ok := resolve(L.CheckString(1))
		if !ok {
			L.Push(lua.LNil)
			L.Push(lua.LString("path escapes plugin data dir"))
			return 2
		}
		data, err := os.ReadFile(path)
		if err != nil {
			L.Push(lua.LNil)
			L.Push(lua.LString(err.Error()))
			return 2
		}
		L.Push(lua.LString(data))
		return 1
	}))

	fs.RawSetString("write", L.NewFunction(func(L *lua.LState) int {
		path, ok := resolve(L.CheckString(1))
		if !ok {
			L.Push(lua.LFalse)
			L.Push(lua.LString("path escapes plugin data dir"))
			return 2
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			L.Push(lua.LFalse)
			L.Push(lua.LString(err.Error()))
			return 2
		}
		if err := os.WriteFile(path, []byte(L.CheckString(2)), 0o644); err != nil {
			L.Push(lua.LFalse)
			L.Push(lua.LString(err.Error()))
			return 2
		}
		L.Push(lua.LTrue)
		return 1
	}))

	fs.RawSetString("list", L.NewFunction(func(L *lua.LState) int {
		entries, err := os.ReadDir(pctx.dataDir)
		if err != nil {
			L.Push(lua.LNil)
			L.Push(lua.LString(err.Error()))
			return 2
		}
		names := L.NewTable()
		for _, e := range entries {
			names.Append(lua.LString(e.Name()))
		}
		L.Push(names)
		return 1
	}))

	return fs
}

// newHTTPTable exposes a minimal GET helper. It is intentionally small: no
// custom methods, headers, or bodies in v1.
func newHTTPTable(L *lua.LState, pctx *pluginContext) *lua.LTable {
	client := &http.Client{Timeout: 15 * time.Second}
	tbl := L.NewTable()

	tbl.RawSetString("get", L.NewFunction(func(L *lua.LState) int {
		url := L.CheckString(1)
		if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
			L.Push(lua.LNil)
			L.Push(lua.LString("url must be http(s)"))
			return 2
		}
		resp, err := client.Get(url)
		if err != nil {
			L.Push(lua.LNil)
			L.Push(lua.LString(err.Error()))
			return 2
		}
		defer resp.Body.Close()
		body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20)) // cap at 1 MiB
		if err != nil {
			L.Push(lua.LNil)
			L.Push(lua.LString(err.Error()))
			return 2
		}
		L.Push(lua.LString(body))
		L.Push(lua.LNumber(resp.StatusCode))
		return 2
	}))

	return tbl
}
