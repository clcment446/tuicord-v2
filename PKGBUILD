# Maintainer: Styly <claudiotorresptpt@gmail.com>
_pkgname=tuicord
pkgname=tuicord-git
pkgver=r84.9625f752
pkgrel=1
pkgdesc="A Discord client that runs in your terminal, written in Go"
arch=('x86_64' 'aarch64')
url="https://github.com/clcment446/tuicord-v2"
license=('custom:unknown')
depends=('glibc')
makedepends=('go' 'git')
provides=('tuicord')
conflicts=('tuicord')
source=("git+https://github.com/clcment446/tuicord-v2.git")
sha256sums=('SKIP')

pkgver() {
	cd "$srcdir/tuicord-v2"
	printf "r%s.%s" "$(git rev-list --count HEAD)" "$(git rev-parse --short=8 HEAD)"
}

prepare() {
	cd "$srcdir/tuicord-v2"
	# Fetch modules now so build() can run offline.
	export GOPATH="$srcdir/gopath"
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
