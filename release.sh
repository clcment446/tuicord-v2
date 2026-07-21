#!/usr/bin/env bash
#
# Build the Arch package from the PKGBUILD and publish it as a GitHub release.
#
# Usage:
#   ./release.sh [tag]
#
# With no argument the release tag defaults to the package version derived by
# the -git PKGBUILD (e.g. "r84.9625f752"). Pass a tag to override, e.g.:
#   ./release.sh v0.1.0
#
# The PKGBUILD clones the repo's pushed HEAD, so the uploaded binary always
# reflects what is on GitHub — commit and push before releasing.

set -euo pipefail

REPO="clcment446/tuicord-v2"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

command -v makepkg >/dev/null || { echo "makepkg not found" >&2; exit 1; }
command -v gh >/dev/null || { echo "gh (GitHub CLI) not found" >&2; exit 1; }
gh auth status >/dev/null 2>&1 || { echo "gh is not authenticated (run: gh auth login)" >&2; exit 1; }

# Build in a throwaway dir so src/, pkg/ and the git clone don't touch the repo.
BUILD_DIR="$(mktemp -d)"
trap 'rm -rf "$BUILD_DIR"' EXIT
cp "$SCRIPT_DIR/PKGBUILD" "$BUILD_DIR/"

echo "==> Building package in $BUILD_DIR"
( cd "$BUILD_DIR" && makepkg -f --noconfirm )

PKG="$(ls "$BUILD_DIR"/*.pkg.tar.zst 2>/dev/null | head -n1)"
[ -n "${PKG:-}" ] && [ -f "$PKG" ] || { echo "No package was produced" >&2; exit 1; }
echo "==> Built $(basename "$PKG")"

# Derive "<pkgver>" from "<pkgname>-<pkgver>-<pkgrel>-<arch>.pkg.tar.zst".
PKGVER="$(basename "$PKG" | sed -E 's/^tuicord-git-(.*)-[0-9]+-[^-]+\.pkg\.tar\.zst$/\1/')"
TAG="${1:-$PKGVER}"

# The build cloned remote HEAD; anchor the release to that exact commit.
SHA="$(git ls-remote "https://github.com/$REPO.git" HEAD | cut -f1)"

if gh release view "$TAG" --repo "$REPO" >/dev/null 2>&1; then
	echo "==> Release $TAG already exists — updating asset"
	gh release upload "$TAG" "$PKG" --repo "$REPO" --clobber
else
	echo "==> Creating release $TAG on $REPO (target $SHA)"
	gh release create "$TAG" "$PKG" \
		--repo "$REPO" \
		--target "$SHA" \
		--title "$TAG" \
		--generate-notes
fi

echo "==> Done: https://github.com/$REPO/releases/tag/$TAG"
