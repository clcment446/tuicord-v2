---
name: autobot-xdg-plugin-policy
summary: The active autobot plugin is installed under the XDG config tree, and aggressive wander policy is tested in its local Lua harness.
tags: [#autobot, #lua, #plugins, #wander, #scripture]
impact: normal
commit: facd583 (dirty)
date: 2026-07-22
created_at: 2026-07-22T08:29:04Z
scope: /home/clement/.config/tuicord-v2/plugins/autobot/init.lua
---

## Problem

The repository's Go autobot source is not the active installed plugin. The
running plugin lives at `/home/clement/.config/tuicord-v2/plugins/autobot/` and
has its own Lua fixtures under `tests/`.

## Cause

The XDG plugin install is a standalone uncommitted Lua tree, so repository
searches do not find the active `!wander` handler.

## Resolution

The active Lua handler now clicks `Leave` for shop encounters, refuses purchase
buttons, attacks ordinary wander encounters, and clicks any enabled button
whose label contains both `steal` and `scripture`. The local harness covers all
three outcomes in `tests/autobot_spec.lua` and `tests/fixtures.lua`.

## Notes

Run the harness with both arguments so its test modules resolve:
`lua tests/autobot_spec.lua /home/clement/.config/tuicord-v2/plugins/autobot /home/clement/.config/tuicord-v2/plugins/autobot/tests`.
