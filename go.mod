module awesomeProject

go 1.26

require (
	github.com/BurntSushi/toml v1.6.0
	github.com/diamondburned/arikawa/v3 v3.6.1-0.20260311205148-176ad9b9440f
	github.com/diamondburned/ningen/v3 v3.0.1-0.20260306213430-5a08d3a709b4
	github.com/google/uuid v1.6.0
	github.com/gorilla/websocket v1.5.3
	github.com/mattn/go-runewidth v0.0.24
	github.com/rivo/uniseg v0.4.7
	github.com/skip2/go-qrcode v0.0.0-20200617195104-da1b6568686e
	github.com/yuin/gopher-lua v1.1.2
	github.com/zalando/go-keyring v0.2.8
	golang.org/x/image v0.43.0
	golang.org/x/sys v0.41.0
	golang.org/x/term v0.40.0
)

require (
	github.com/clipperhouse/uax29/v2 v2.2.0 // indirect
	github.com/danieljoos/wincred v1.2.3 // indirect
	github.com/godbus/dbus/v5 v5.2.2 // indirect
	github.com/gorilla/schema v1.4.1 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/twmb/murmur3 v1.1.8 // indirect
	golang.org/x/time v0.14.0 // indirect
)

replace github.com/diamondburned/arikawa/v3 => ./third_party/arikawa
