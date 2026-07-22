# tuicord Lua plugins

tuicord embeds a Lua interpreter ([gopher-lua](https://github.com/yuin/gopher-lua),
Lua 5.1) so you can extend the client without recompiling. Drop `.lua` files into
your plugins directory and they load on startup.

## Location

Plugins live beside the primary `config.lua`:

```
~/.config/tuicord-v2/plugins/          # honors XDG_CONFIG_HOME
  hello.lua                            # a single-file plugin named "hello"
  my-plugin/init.lua                   # a directory plugin named "my-plugin"
```

The directory is created for you on first run. See `hello.lua` in this folder
for a working example.

## config.lua (primary configuration)

`config.lua` is loaded exactly once before login and before the TUI is built,
independently of whether ordinary plugins are enabled. `tuicord.configure`
strictly overlays the Go `config.Config` defaults using the existing TOML field
names. Unknown keys and wrong types fail startup with a field path. Accounts and
auth mode churn are machine state in `ui.toml`, not authored account data.

```lua
-- ~/.config/tuicord-v2/config.lua
tuicord.configure({
  layout = { channels_width = 24 },
  plugins = { enabled = true, disabled = {}, grants = {} },
})
tuicord.keymap("ctrl+j", function() tuicord.send("gg") end)
```

An older `config.lua` without `configure` remains valid and gets all defaults.
If config.lua is absent but legacy config.toml exists, that TOML plus colors.conf
is used unchanged for one launch and an explicit Lua migration is generated;
the legacy files are not removed.

## Themes

`tuicord.theme(name, definition)` validates and registers a typed theme. The
legacy flat seven-color table remains valid. The preferred form adds semantic
styles, and partial palettes inherit from the built-in default deterministically:

```lua
tuicord.theme("dracula", {
  palette = {
    background = "#282a36", text = "#f8f8f2", muted = "#6272a4",
    accent = "#bd93f9", selection = "#44475a", border = "#44475a", error = "#ff5555",
  },
  styles = {
    ["messages.author"] = { bold = true },
    ["guilds.selected"] = { bg = "#44475a" },
  },
})
tuicord.use_theme("dracula") -- startup selection in config.lua
```

Use `;theme <name>` at runtime (or `;theme` to list). Runtime switching updates
shared semantic cells, the App background/theme, chat cache generation, and
straightforward MainView/Shell surfaces. Some already-open snapshot-based
popups and complex widgets still fully update only when reopened.

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
| `tuicord.click(channel_id, message_id, custom_id)` | Click a button component from an event payload. |
| `tuicord.select(channel_id, message_id, custom_id, values)` | Submit values for a string-select component. |
| `tuicord.notify(title, body)` | Show a transient notice. |
| `tuicord.every(milliseconds, fn)` | Run a callback periodically on the plugin runtime. |
| `tuicord.now_ms()` | Read the current Unix time in milliseconds. |
| `tuicord.style(selector, opts)` | Recolor a semantic surface at runtime; `opts` is `{fg=, bg=, attrs=, bold=true, ...}`. Selectors mirror `colors.conf`. |
| `tuicord.configure(opts)` | Strict typed config overlay; available only in config.lua. |
| `tuicord.theme(name, definition)` | Register a validated flat or `{palette=, styles=}` theme. |
| `tuicord.use_theme(name)` | Select during config startup or apply through the live Host at runtime. |
| `tuicord.overlay(title, lines)` | Open a read-only panel of text `lines` (dismiss with Esc). Rendered with the active theme. |
| `tuicord.active_channel()` / `active_guild()` / `self_id()` | Current IDs (as strings). |
| `tuicord.log(...)` | Write to `~/.config/tuicord-v2/plugin.log`. `print` is redirected here too. |

### Events

`ready`, `message.create`, `message.update`, `message.delete`, `reaction.add`,
`reaction.remove`, `channel.switch`, `error`. Message payloads include normalized
`components` (buttons, selects, text, and nested children) and `embeds`, so a
plugin can safely react to the exact component IDs Discord sent.

### Identifiers

Discord snowflake IDs exceed the range Lua numbers represent exactly, so **every
ID is a decimal string** on the Lua side (`m.channel_id`, `tuicord.self_id()`,
etc.). Pass them straight back to API calls.

## Sandbox and grants

Plugins run without `os`, `io`, `require`, or arbitrary code loading. Grant extra
capabilities per plugin in `config.lua`:

```lua
tuicord.configure({
  plugins = { enabled = true, grants = { ["my-plugin"] = {"fs", "net"} } },
})
```

- `fs` adds `tuicord.fs.read/write/list`, confined to the plugin's own data
  directory (`~/.config/tuicord-v2/plugin-data/<name>/`).
- `net` adds `tuicord.http.get(url)`.

## Disabling

```lua
tuicord.configure({
  plugins = { enabled = false, disabled = {"hello"}, grants = {} },
})
```
