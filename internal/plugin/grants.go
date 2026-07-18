package plugin

import (
	"io"
	"net/http"
	"path/filepath"
	"strings"

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
	// A grant alone is insufficient: an empty DataDir deliberately leaves
	// fsRoot nil and therefore exposes no filesystem API.
	if pctx.grants[CapFS] && pctx.fsRoot != nil {
		tbl.RawSetString("fs", newFSTable(L, pctx))
	}
	if pctx.grants[CapNet] {
		tbl.RawSetString("http", newHTTPTable(L, pctx))
	}
}

// newFSTable exposes read/write/list through os.Root. Root rejects absolute
// names, parent traversal, and symlinks that resolve outside the plugin's data
// directory, including path-swap races between validation and use.
func newFSTable(L *lua.LState, pctx *pluginContext) *lua.LTable {
	fs := L.NewTable()

	fs.RawSetString("read", L.NewFunction(func(L *lua.LState) int {
		data, err := pctx.fsRoot.ReadFile(L.CheckString(1))
		if err != nil {
			L.Push(lua.LNil)
			L.Push(lua.LString(err.Error()))
			return 2
		}
		L.Push(lua.LString(data))
		return 1
	}))

	fs.RawSetString("write", L.NewFunction(func(L *lua.LState) int {
		name := L.CheckString(1)
		if err := pctx.fsRoot.MkdirAll(filepath.Dir(name), 0o755); err != nil {
			L.Push(lua.LFalse)
			L.Push(lua.LString(err.Error()))
			return 2
		}
		if err := pctx.fsRoot.WriteFile(name, []byte(L.CheckString(2)), 0o644); err != nil {
			L.Push(lua.LFalse)
			L.Push(lua.LString(err.Error()))
			return 2
		}
		L.Push(lua.LTrue)
		return 1
	}))

	fs.RawSetString("list", L.NewFunction(func(L *lua.LState) int {
		rootDir, err := pctx.fsRoot.Open(".")
		if err != nil {
			L.Push(lua.LNil)
			L.Push(lua.LString(err.Error()))
			return 2
		}
		entries, readErr := rootDir.ReadDir(-1)
		closeErr := rootDir.Close()
		if readErr != nil {
			err = readErr
		} else if closeErr != nil {
			err = closeErr
		}
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
	client := &http.Client{}
	tbl := L.NewTable()

	tbl.RawSetString("get", L.NewFunction(func(L *lua.LState) int {
		url := L.CheckString(1)
		if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
			L.Push(lua.LNil)
			L.Push(lua.LString("url must be http(s)"))
			return 2
		}
		// safeDoFile/safeCall installs the execution deadline on the LState.
		// Binding the request to that exact context makes both the Lua timeout
		// and Manager.Close interrupt blocked network I/O while preserving the
		// single worker/LState execution contract.
		ctx := L.Context()
		if ctx == nil {
			L.Push(lua.LNil)
			L.Push(lua.LString("http.get requires an active Lua execution context"))
			return 2
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			L.Push(lua.LNil)
			L.Push(lua.LString(err.Error()))
			return 2
		}
		resp, err := client.Do(req)
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
