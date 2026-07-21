# tuicord

A Discord client that runs in your terminal, written in Go.

## Build

Needs Go 1.26+.

```sh
go build -o tuicord ./cmd/tuicord
./tuicord
```

First launch creates `~/.config/tuicord-v2/config.lua` and `plugins/`, then
walks you through login (token, QR, or captcha). The token is kept in your
system keyring afterward.

## Configuration

`config.lua` is the primary authored configuration. It is executed once before
login and UI construction. Use strict `tuicord.configure({...})`, register typed
themes with `tuicord.theme`, and select the startup theme with
`tuicord.use_theme`. See [`examples/plugins/config.lua`](examples/plugins/config.lua)
and [`examples/plugins/README.md`](examples/plugins/README.md).

Existing Lua files without `configure` continue to use defaults. When only a
legacy `config.toml` exists, tuicord uses it and `colors.conf` for that launch,
atomically generates an equivalent Lua migration, and leaves both legacy files
untouched. Machine-managed accounts and auth-mode preference live in
`~/.local/state/tuicord/ui.toml`; tokens remain in the OS keyring.

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
