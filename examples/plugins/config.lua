-- config.lua — Lua configuration, loaded separately from plugins.
--
-- Place this file at ~/.config/tuicord-v2/config.lua (beside config.toml). It
-- is loaded on every startup whether or not the plugin system is enabled, so
-- it is the place to keep personal keybindings and setup expressed in Lua
-- rather than as a plugin. It has the same sandboxed `tuicord` API as plugins.
--
-- This complements config.toml; it does not replace it. Use it for things Lua
-- expresses well (key bindings to actions), and TOML for static settings.

-- Personal keybindings. These fire only when no built-in binding or the
-- composer consumes the key, so they cannot shadow core navigation.
tuicord.keymap("ctrl+j", function()
  tuicord.send("gg")
end)

tuicord.keymap("ctrl+t", function()
  tuicord.overlay("Quick info", {
    "guild:   " .. tuicord.active_guild(),
    "channel: " .. tuicord.active_channel(),
  })
end)

-- A personal ;-command.
tuicord.command("shrug", function()
  tuicord.send("\194\175\\_(\227\131\132)_/\194\175")
end, "send a shrug")

-- Register a theme here too, then switch to it with `;theme mono`.
tuicord.theme("mono", {
  background = "#000000",
  text       = "#cccccc",
  muted      = "#666666",
  accent     = "#ffffff",
  selection  = "#333333",
  border     = "#444444",
  error      = "#ff0000",
})

tuicord.log("config.lua loaded")
