// Package plugin embeds a Lua interpreter (gopher-lua) so users can extend
// tuicord at runtime with .lua files dropped into their config directory.
//
// # Concurrency
//
// A gopher-lua *LState is not safe for concurrent use. Every plugin therefore
// runs on a single dedicated goroutine owned by the runtime: plugin loading,
// event callbacks, command handlers and key handlers are all serialized onto
// that goroutine through a bounded job queue. This keeps the LState invariant
// and prevents a slow plugin from blocking the terminal render loop.
//
// Side effects flow the other way: the Lua API never touches the store or
// widgets directly. Instead each binding calls a Host function, which is
// responsible for marshalling the real mutation onto the UI goroutine (via
// tui.App.Post). The rule is: Lua code runs on the plugin goroutine, its
// effects land on the UI goroutine. The exception is bootstrap-only
// declarative configuration and startup theme selection: those mutate a typed
// config target synchronously before a live UI Host is attached.
//
// # Identifiers
//
// Discord snowflake IDs are 64-bit and exceed the range Lua numbers (float64)
// can represent exactly, so every ID crosses the Lua boundary as a decimal
// string. Bindings parse them back with parseID.
package plugin
