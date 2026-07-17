-- everything.lua — exercises every tuicord plugin API surface.
--
-- Drop it in ~/.config/tuicord-v2/plugins/ and, to test the opt-in grants, add
-- to config.toml:
--
--   [plugins.grants]
--   everything = ["fs", "net"]
--
-- Then use the ;commands below. `;status` opens an overlay summarizing live
-- state and how many of each event has fired.

tuicord.name = "everything"

-- ---------------------------------------------------------------------------
-- Events: subscribe to all of them and keep counters.
-- ---------------------------------------------------------------------------
local EVENTS = {
  "ready", "message.create", "message.update", "message.delete",
  "reaction.add", "reaction.remove", "channel.switch", "error",
}

local counts = {}
local last_message = {} -- {channel_id, id} of the most recent message.create

for _, name in ipairs(EVENTS) do
  counts[name] = 0
  tuicord.on(name, function(payload)
    counts[name] = counts[name] + 1
    tuicord.log("event " .. name .. " (#" .. counts[name] .. ")")
    if name == "message.create" and payload then
      last_message.channel_id = payload.channel_id
      last_message.id = payload.id
    end
  end)
end

-- ---------------------------------------------------------------------------
-- Helpers.
-- ---------------------------------------------------------------------------
local function has(cap) return cap ~= nil end

local function status_lines()
  local lines = {
    "tuicord plugin self-test",
    "",
    "active guild:   " .. tuicord.active_guild(),
    "active channel: " .. tuicord.active_channel(),
    "self id:        " .. tuicord.self_id(),
    "",
    "grants:",
    "  fs:  " .. (has(tuicord.fs) and "granted" or "not granted"),
    "  net: " .. (has(tuicord.http) and "granted" or "not granted"),
    "",
    "event counts:",
  }
  for _, name in ipairs(EVENTS) do
    table.insert(lines, "  " .. name .. ": " .. counts[name])
  end
  if last_message.id then
    table.insert(lines, "")
    table.insert(lines, "last message " .. last_message.id .. " in channel " .. last_message.channel_id)
  end
  return lines
end

-- ---------------------------------------------------------------------------
-- Commands: one per action so each surface can be triggered by hand.
-- ---------------------------------------------------------------------------

-- ;status — open a custom overlay summarizing everything (tuicord.overlay,
-- accessors).
tuicord.command("status", function()
  tuicord.overlay("Plugin self-test", status_lines())
end, "show plugin status overlay")

-- ;echo <text...> — send to the active channel (tuicord.send).
tuicord.command("echo", function(args)
  tuicord.send(table.concat(args, " "))
end, "send text to the active channel")

-- ;dm <channel_id> <text...> — send to a specific channel (tuicord.send_to).
tuicord.command("dm", function(args)
  local channel = args[1]
  if not channel then
    tuicord.notify("dm", "usage: ;dm <channel_id> <text>")
    return
  end
  local rest = {}
  for i = 2, #args do rest[#rest + 1] = args[i] end
  tuicord.send_to(channel, table.concat(rest, " "))
end, "send text to a channel id")

-- ;quote <text...> — reply to the most recent message (tuicord.reply).
tuicord.command("quote", function(args)
  if not last_message.id then
    tuicord.notify("quote", "no message seen yet")
    return
  end
  tuicord.reply(last_message.channel_id, last_message.id, table.concat(args, " "), false)
end, "reply to the last message")

-- ;thumbsup — react to the most recent message (tuicord.react).
tuicord.command("thumbsup", function()
  if not last_message.id then
    tuicord.notify("thumbsup", "no message seen yet")
    return
  end
  tuicord.react(last_message.channel_id, last_message.id, "\240\159\145\141") -- 👍
end, "react to the last message")

-- ;paint — recolor a semantic surface (tuicord.style).
tuicord.command("paint", function()
  tuicord.style("messages.author", { fg = "#ff0000", bold = true })
  tuicord.notify("theme", "author names are now bold red")
end, "recolor message authors")

-- ;fscheck — round-trip a file through the fs grant.
tuicord.command("fscheck", function()
  if not has(tuicord.fs) then
    tuicord.notify("fs", "not granted (add fs to [plugins.grants])")
    return
  end
  local ok, err = tuicord.fs.write("counter.txt", tostring(counts["message.create"]))
  if not ok then
    tuicord.notify("fs", "write failed: " .. tostring(err))
    return
  end
  local data = tuicord.fs.read("counter.txt")
  tuicord.notify("fs", "wrote and read back: " .. tostring(data))
end, "test the filesystem grant")

-- ;netcheck — fetch a URL through the net grant.
tuicord.command("netcheck", function(args)
  if not has(tuicord.http) then
    tuicord.notify("net", "not granted (add net to [plugins.grants])")
    return
  end
  local url = args[1] or "https://example.com"
  local body, status = tuicord.http.get(url)
  if not body then
    tuicord.notify("net", "GET failed: " .. tostring(status))
    return
  end
  tuicord.notify("net", "GET " .. url .. " -> " .. tostring(status) .. " (" .. #body .. " bytes)")
end, "test the network grant")

-- ---------------------------------------------------------------------------
-- Keybindings (tuicord.keymap). Fire only when nothing else consumes the key.
-- ---------------------------------------------------------------------------
tuicord.keymap("ctrl+g", function() tuicord.send("gg") end)
tuicord.keymap("ctrl+b", function() tuicord.overlay("Plugin self-test", status_lines()) end)

tuicord.log("everything.lua loaded: " .. #EVENTS .. " events, commands and keymaps registered")
