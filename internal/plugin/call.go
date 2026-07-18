package plugin

import (
	"context"
	"time"

	lua "github.com/yuin/gopher-lua"
)

// Defaults keep plugin work bounded while allowing normal startup and event
// handling ample time. Options can shorten these values (notably in tests).
const (
	defaultStartupTimeout  = 5 * time.Second
	defaultCallbackTimeout = 5 * time.Second
)

// withDeadline installs a context only for the duration of one worker-owned Lua
// execution. The runtime context is cancelled by Close, so shutdown does not
// need to wait for the full per-call deadline.
func withDeadline(parent context.Context, L *lua.LState, timeout time.Duration, fn func() error) error {
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()
	L.SetContext(ctx)
	defer L.RemoveContext()
	return fn()
}

// safeDoFile runs plugin startup under a deadline just like callbacks.
func safeDoFile(parent context.Context, L *lua.LState, path string, timeout time.Duration) error {
	return withDeadline(parent, L, timeout, func() error { return L.DoFile(path) })
}

// safeCall invokes fn (owned by L) under a timeout, protecting the host from
// Lua errors (returned) and infinite loops (context deadline).
func safeCall(parent context.Context, L *lua.LState, fn *lua.LFunction, timeout time.Duration, args ...lua.LValue) error {
	return withDeadline(parent, L, timeout, func() error {
		return L.CallByParam(lua.P{Fn: fn, NRet: 0, Protect: true}, args...)
	})
}
