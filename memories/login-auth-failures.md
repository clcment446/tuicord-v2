---
name: login-auth-failures
summary: Pasted-token login must survive unavailable Secret Service; QR ticket exchange can be rejected by Discord CAPTCHA.
tags: [#auth, #keyring, #qr-login, #discord, #captcha]
impact: high
commit: f9fae04 (dirty)
date: 2026-07-12
created_at: 2026-07-12T01:45:00+02:00
scope: internal/auth/auth.go, internal/discord/transport.go, internal/ui/qr_remoteauth.go
---

## Problem

On Linux without an active Secret Service provider, saving a pasted token
returned `The name is not activatable` from `internal/auth/auth.go`. QR login
reached Discord's remote-auth ticket exchange and received HTTP 400 with
`captcha-required`.

## Cause

Token persistence was treated as part of successful authentication, so a
keyring write failure aborted the session. The QR response is a server-side
CAPTCHA challenge; it is not a JSON or transport parsing failure.

## Resolution

`ResolveToken` now returns the authenticated token when persistence fails and
reports the wrapped error through `Options.OnStoreError`; the CLI warns and
continues. Discord REST requests also carry the shared browser identity and
JSON content type. Full `go test ./...` passed with a writable GOCACHE.

## Notes

The QR flow cannot solve or bypass a CAPTCHA challenge without a supported
interactive CAPTCHA flow. Users can continue with a pasted token or set
`TOKEN` for subsequent runs when Secret Service is unavailable. The current
implementation uses Firefox WebDriver BiDi for the CAPTCHA surface and routes
real terminal events to the browser; it does not synthesize mouse movement.
