---
name: autobot-hunt-state-aware-dedup
summary: Hunt follow-up attacks must deduplicate by rendered message state, not only Discord message ID.
tags: [#autobot, #lua, #plugins, #hunt, #combat, #deduplication]
impact: normal
commit: none (dirty)
date: 2026-07-22
created_at: 2026-07-22T09:31:49Z
scope: /home/clement/.config/tuicord-v2/plugins/autobot/init.lua:139-147
---

## Problem

When a hunt attack did not defeat the enemy, the bot ignored the next attack
because Discord updated the existing combat message instead of creating a new
one.

## Cause

`once` keyed actions by `message.id` and action kind only. A combat message can
retain both values across turns while its HP and available controls change.

## Resolution

The deduplication key now also includes `body(message)`, allowing a changed
combat state to trigger the next attack while suppressing duplicate events for
the same rendered state. The regression is covered by
`plugins/autobot/tests/autobot_spec.lua:26-27` and matching fixtures.

## Notes

Run the standalone harness with:
`lua /home/clement/.config/tuicord-v2/plugins/autobot/tests/autobot_spec.lua /home/clement/.config/tuicord-v2/plugins/autobot /home/clement/.config/tuicord-v2/plugins/autobot/tests`
