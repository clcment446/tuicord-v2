package plugin

import lua "github.com/yuin/gopher-lua"

// safeLibs is the set of standard libraries a sandboxed plugin may use. os, io,
// package/require and debug are deliberately excluded so a plugin cannot touch
// the filesystem, spawn processes, or load arbitrary code. Filesystem/network
// access is instead offered as narrow, audited helpers gated behind user grants
// (see installGrants).
var safeLibs = []struct {
	name string
	open lua.LGFunction
}{
	{lua.BaseLibName, lua.OpenBase},
	{lua.TabLibName, lua.OpenTable},
	{lua.StringLibName, lua.OpenString},
	{lua.MathLibName, lua.OpenMath},
	{lua.CoroutineLibName, lua.OpenCoroutine},
}

// unsafeBaseGlobals are functions the base library installs that would defeat
// the sandbox (arbitrary code loading / file access). They are removed after
// the base library is opened.
var unsafeBaseGlobals = []string{
	"dofile", "loadfile", "load", "loadstring", "require", "module", "collectgarbage",
}

// newSandboxedState builds a fresh LState with only the safe standard libraries
// opened and the unsafe base globals removed. The caller installs the tuicord
// API (and any granted capability helpers) before running plugin code.
func newSandboxedState() (*lua.LState, error) {
	L := lua.NewState(lua.Options{SkipOpenLibs: true})
	for _, lib := range safeLibs {
		if err := L.CallByParam(lua.P{
			Fn:      L.NewFunction(lib.open),
			NRet:    0,
			Protect: true,
		}, lua.LString(lib.name)); err != nil {
			L.Close()
			return nil, err
		}
	}
	for _, name := range unsafeBaseGlobals {
		L.SetGlobal(name, lua.LNil)
	}
	return L, nil
}
