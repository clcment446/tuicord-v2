-- hello.lua — a sample tuicord plugin.
--
-- Copy this file into your plugins directory to try it:
--   ~/.config/tuicord-v2/plugins/hello.lua   (honors XDG_CONFIG_HOME)
--
-- Plugins run sandboxed: no filesystem, network, or os access unless the user
-- grants it in config.toml under [plugins.grants]. All Discord snowflake IDs
-- cross into Lua as decimal strings.

tuicord.name = "hello"

-- A local ;-command. Type `;hi` (or `;hi <name>`) in the composer.
tuicord.command("hi", function(args)
  local who = args[1] or "world"
  tuicord.notify("hello plugin", "hi, " .. who .. "!")
end, "greet someone")

-- React to every incoming message from a bot.
tuicord.on("message.create", function(m)
  if m.bot then
    tuicord.log("bot message in channel " .. m.channel_id .. ": " .. m.content)
  end
end)

-- Bind a key to send a canned message to the active channel.
tuicord.keymap("ctrl+g", function()
  tuicord.send("gg")
end)

-- React when the user switches channels.
tuicord.on("channel.switch", function(ev)
  tuicord.log("switched to channel " .. ev.channel_id)
end)
