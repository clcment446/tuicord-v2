# tuicord Lua plugins

tuicord embeds a Lua interpreter ([gopher-lua](https://github.com/yuin/gopher-lua),
Lua 5.1) so you can extend the client without recompiling. Drop `.lua` files into
your plugins directory and they load on startup.

## Location

Plugins live beside `config.toml`:

```
~/.config/tuicord-v2/plugins/          # honors XDG_CONFIG_HOME
  hello.lua                            # a single-file plugin named "hello"
  my-plugin/init.lua                   # a directory plugin named "my-plugin"
```

The directory is created for you on first run. See `hello.lua` in this folder
for a working example.

## config.lua (settings & keybindings in Lua)

A `config.lua` beside `config.toml` is loaded on every startup **independently
of the plugin system** — even when `[plugins] enabled = false`. It runs with the
same sandboxed `tuicord` API, so it's the place to keep personal keybindings and
setup expressed in Lua rather than as a plugin. It complements `config.toml`
(static settings) rather than replacing it. See `config.lua` in this folder.

```lua
-- ~/.config/tuicord-v2/config.lua
tuicord.keymap("ctrl+j", function() tuicord.send("gg") end)
tuicord.command("shrug", function() tuicord.send("¯\\_(ツ)_/¯") end, "send a shrug")
```

## Themes

Plugins (or `config.lua`) register named palettes with `tuicord.theme(name,
palette)`; switch between them in-app with `;theme <name>` (or `;theme` to list
them). A palette sets the seven semantic colors:

```lua
tuicord.theme("dracula", {
  background = "#282a36", text = "#f8f8f2", muted = "#6272a4",
  accent = "#bd93f9", selection = "#44475a", border = "#44475a", error = "#ff5555",
})
```

Switching updates surfaces that resolve their color live; a few surfaces that
snapshot styles at construction only pick up the change on restart.

## The `tuicord` API

A global `tuicord` table is available to every plugin.

| Call | Purpose |
| --- | --- |
| `tuicord.on(event, fn)` | Subscribe to an event (see below). |
| `tuicord.command(name, fn, help)` | Register a `;name` composer command; `fn` receives an args array. |
| `tuicord.keymap(spec, fn)` | Bind a key (e.g. `"ctrl+g"`) to a callback. Fires only when no built-in binding or the composer consumes the key. |
| `tuicord.send(content)` | Send to the active channel. |
| `tuicord.send_to(channel_id, content)` | Send to a specific channel. |
| `tuicord.reply(channel_id, message_id, content, mention)` | Reply to a message. |
| `tuicord.react(channel_id, message_id, emoji)` | Add a reaction. |
| `tuicord.notify(title, body)` | Show a transient notice. |
| `tuicord.style(selector, opts)` | Recolor a semantic surface at runtime; `opts` is `{fg=, bg=, attrs=, bold=true, ...}`. Selectors mirror `colors.conf`. |
| `tuicord.theme(name, palette)` | Register a named palette (`{background=, text=, muted=, accent=, selection=, border=, error=}`). Switch to it in-app with `;theme <name>`. |
| `tuicord.overlay(title, lines)` | Open a read-only panel of text `lines` (dismiss with Esc). Rendered with the active theme. |
| `tuicord.active_channel()` / `active_guild()` / `self_id()` | Current IDs (as strings). |
| `tuicord.log(...)` | Write to `~/.config/tuicord-v2/plugin.log`. `print` is redirected here too. |

### Events

`ready`, `message.create`, `message.update`, `message.delete`, `reaction.add`,
`reaction.remove`, `channel.switch`, `error`. Payloads are tables; see the
constants in `internal/plugin/events.go` for each shape.

### Identifiers

Discord snowflake IDs exceed the range Lua numbers represent exactly, so **every
ID is a decimal string** on the Lua side (`m.channel_id`, `tuicord.self_id()`,
etc.). Pass them straight back to API calls.

## Sandbox and grants

Plugins run without `os`, `io`, `require`, or arbitrary code loading. Grant extra
capabilities per plugin in `config.toml`:

```toml
[plugins.grants]
my-plugin = ["fs", "net"]
```

- `fs` adds `tuicord.fs.read/write/list`, confined to the plugin's own data
  directory (`~/.config/tuicord-v2/plugin-data/<name>/`).
- `net` adds `tuicord.http.get(url)`.

## Disabling

```toml
[plugins]
enabled = false               # turn the whole system off
disabled = ["hello"]          # or skip specific plugins by name
```
