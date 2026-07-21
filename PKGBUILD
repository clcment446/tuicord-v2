# Maintainer: Styly <claudiotorresptpt@gmail.com>
_pkgname=tuicord
pkgname=tuicord-git
pkgver=v0.1.0
pkgrel=1
pkgdesc="A Discord client that runs in your terminal, written in Go"
arch=('x86_64' 'aarch64')
url="https://github.com/clcment446/tuicord-v2"
license=('custom:unknown')
depends=('glibc' 'mpv')
makedepends=('go' 'git')
optdepends=('xdg-utils: open links in a browser'
            'wl-clipboard: clipboard support on Wayland'
            'xclip: clipboard support on X11'
            'xsel: alternative clipboard support on X11'
            'firefox: captcha-based login flow')
provides=('tuicord')
conflicts=('tuicord')
source=("git+https://github.com/clcment446/tuicord-v2.git")
sha256sums=('SKIP')

prepare() {
	cd "$srcdir/tuicord-v2"
	# Fetch modules now so build() can run offline. -modcacherw keeps the
	# module cache writable so it can be cleaned up afterwards.
	export GOPATH="$srcdir/gopath"
	export GOFLAGS="-modcacherw"
	go mod download
}

build() {
	cd "$srcdir/tuicord-v2"
	export GOPATH="$srcdir/gopath"
	export CGO_CPPFLAGS="${CPPFLAGS}"
	export CGO_CFLAGS="${CFLAGS}"
	export CGO_CXXFLAGS="${CXXFLAGS}"
	export CGO_LDFLAGS="${LDFLAGS}"
	export GOFLAGS="-buildmode=pie -trimpath -mod=readonly -modcacherw"
	go build -ldflags "-linkmode=external -compressdwarf=false" -o build/tuicord ./cmd/tuicord
}

package() {
	cd "$srcdir/tuicord-v2"
	install -Dm755 build/tuicord "$pkgdir/usr/bin/$_pkgname"
	install -Dm644 README.md "$pkgdir/usr/share/doc/$pkgname/README.md"
}
