---
name: qr-module-colors-use-foreground
summary: QR dark and light semantic styles carry their module colors in Fg; using auth.qr.light.Bg collapses the rendered QR code into a solid black rectangle.
tags: [#auth, #qr-login, #rendering, #colors, #tui]
impact: high
commit: 1742a28 (dirty)
date: 2026-07-19
created_at: 2026-07-19T01:05:33+03:00
scope: internal/config/colors_conf.go, internal/ui/qr.go, internal/ui/qr_test.go
---

## Problem

The Discord login QR panel showed a solid black rectangle instead of visible
modules, so the mobile app could not scan it.

## Cause

The semantic color migration split the old QR style into `auth.qr.dark` and
`auth.qr.light`, but `halfBlockStyled` read the light module color from
`auth.qr.light.Bg`. That default background was black, the same as the dark
foreground, so solid-light and mixed half-block cells were also black.

## Resolution

Both semantic module colors are now explicitly stored in `Fg`. The half-block
renderer uses `dark.Fg` for dark modules and `light.Fg` for light modules,
mapping the latter to the cell background when composing a half block. A
regression test supplies intentionally inverted background values and checks
all four upper/lower module combinations.

## Verification

`go test ./internal/ui ./internal/config` and `go test ./...` pass. The full
suite requires loopback-listener permission for its existing `httptest`
coverage in the CAPTCHA and plugin packages.
