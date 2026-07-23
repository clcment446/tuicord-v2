# tuicord

A Discord and Matrix client that runs in your terminal, written in Go.

## Build

Needs Go 1.26+ and a C toolchain (cgo, for the Matrix E2EE SQLite store).

```sh
go build -tags goolm -o tuicord ./cmd/tuicord
./tuicord
```

The `goolm` build tag selects the pure-Go olm implementation for Matrix
end-to-end encryption, so no system `libolm` is required. **All `go` commands
(`build`, `test`, `vet`, `run`) need `-tags goolm`** — without it the build
fails looking for `olm/olm.h`.

First launch creates `~/.config/tuicord-v2/config.lua` and `plugins/`, then
walks you through login. Discord (token, QR, or captcha) and Matrix (homeserver
+ password, or access-token paste) are both offered on the login screen. Secrets
are kept in your system keyring afterward; the Matrix E2EE crypto store lives
under `~/.local/share/tuicord/matrix/<account>/`.

## Configuration

`config.lua` is the primary authored configuration. It is executed once before
login and UI construction. Use strict `tuicord.configure({...})`, register typed
themes with `tuicord.theme`, and select the startup theme with
`tuicord.use_theme`. See [`examples/plugins/config.lua`](examples/plugins/config.lua)
and [`examples/plugins/README.md`](examples/plugins/README.md).

Existing Lua files without `configure` continue to use defaults. When only a
legacy `config.toml` exists, tuicord uses it and `colors.conf` for that launch,
atomically generates an equivalent Lua migration, and leaves both legacy files
untouched. Background GIF and role-gradient animations are disabled by default
when an SSH session is detected; set `display.no_animations_over_ssh = false`
in `tuicord.configure` to opt out. Set `display.border_style` to `rounded`,
`square`, `heavy`, `double`, or `ascii` to choose the frame glyphs used by
embeds, component sections, panels, menus, modals, and split dividers.
Machine-managed accounts and auth-mode
preference live in `~/.local/state/tuicord/ui.toml`; tokens remain in the OS
keyring.

## Testing

```sh
go test -tags goolm ./...
go test -tags goolm -race ./...
(cd third_party/arikawa && go test ./...)
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go test -tags goolm -run '^$' -exec=true ./...
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
