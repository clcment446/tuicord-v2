-- config.lua — primary declarative configuration, loaded exactly once before
-- login and before any widget is constructed.
--
-- Place at ~/.config/tuicord-v2/config.lua (honors XDG_CONFIG_HOME). Existing
-- config.lua files without tuicord.configure remain valid and use defaults.

tuicord.configure({
  layout = {
    guilds_width = 3,
    channels_width = 20,
    members_width = 20,
    members_auto_hide = true,
    members_hide_below = 120,
    elements = {
      accounts = { visible = true, width = 16, min_width = 8, max_width = 28 },
    },
  },
  keys = {
    quick_switcher = "ctrl+k",
    help = "ctrl+/",
    next_panel = "tab",
    focus_composer = "esc",
    picker = "ctrl+e",
    paste_image = "ctrl+v",
    video_pause = "space",
    video_seek_backward = "left",
    video_seek_forward = "right",
    video_replay = "r",
  },
  plugins = {
    enabled = true,
    disabled = {},
    grants = {
      -- ["everything"] = {"fs", "net"},
    },
  },
})

-- Preferred theme form: a complete seven-color palette plus validated semantic
-- styles. Partial palettes inherit from the built-in default, not the previous
-- active theme. The legacy flat seven-color form remains supported.
tuicord.theme("mono", {
  palette = {
    background = "#000000",
    text       = "#cccccc",
    muted      = "#777777",
    accent     = "#ffffff",
    selection  = "#333333",
    border     = "#444444",
    error      = "#ff5555",
  },
  styles = {
    ["messages.author"] = { fg = "#ffffff", bold = true },
    ["messages.link"] = { underline = true },
    ["guilds.selected"] = { bg = "#333333", bold = true },
    ["quick_switcher.selected"] = { bg = "#333333" },
  },
})

-- Startup selection is resolved before login and every initial widget.
tuicord.use_theme("mono")

-- Config registrations survive bootstrap and use the live Host after the UI is
-- attached.
tuicord.keymap("ctrl+j", function()
  tuicord.send("gg")
end)

tuicord.keymap("ctrl+t", function()
  tuicord.overlay("Quick info", {
    "guild:   " .. tuicord.active_guild(),
    "channel: " .. tuicord.active_channel(),
  })
end)

tuicord.command("shrug", function()
  tuicord.send("\194\175\\_(\227\131\132)_/\194\175")
end, "send a shrug")

tuicord.log("config.lua loaded")
