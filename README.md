# tuicord

A Discord client that runs in your terminal, written in Go.

## Build

Needs Go 1.26+.

```sh
go build -o tuicord ./cmd/tuicord
./tuicord
```

First launch walks you through login (token, QR, or captcha). The token is kept
in your system keyring afterward.

## Testing

```sh
go test ./...
go test -race ./...
(cd third_party/arikawa && go test ./...)
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go test -run '^$' -exec=true ./...
```

`third_party/arikawa` has its own `go.mod`. Go module boundaries mean the root
`go test ./...` and race command do not include it, so its suite must be run
separately from that directory.

## What it does

Servers, channels, and DMs in a keyboard-driven TUI. Embeds and message
components render, images show inline on terminals that support Kitty graphics,
and there's a Lua plugin system for adding commands and restyling things.

## Layout

Entry point is `cmd/tuicord`. Everything else is under `internal/` — `discord`
(gateway), `tui`/`ui` (rendering), `plugin` (Lua), `media` (images),
`auth`/`keyring` (login). Demos live in `examples/`.

## Release
Modify PKGBUILD pkgver then run `./releash.sh` with the gh cli installed.
