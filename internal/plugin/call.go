package plugin

import (
	"context"
	"time"

	lua "github.com/yuin/gopher-lua"
)

// callTimeout bounds a single plugin callback. gopher-lua checks the state's
// context between instructions, so a runaway loop is aborted rather than
// wedging the plugin goroutine forever.
const callTimeout = 5 * time.Second

// safeCall invokes fn (owned by L) with args under a timeout, protecting the
// host from Lua errors (returned) and infinite loops (context deadline).
func safeCall(L *lua.LState, fn *lua.LFunction, args ...lua.LValue) error {
	ctx, cancel := context.WithTimeout(context.Background(), callTimeout)
	defer cancel()
	L.SetContext(ctx)
	defer L.RemoveContext()
	return L.CallByParam(lua.P{Fn: fn, NRet: 0, Protect: true}, args...)
}
