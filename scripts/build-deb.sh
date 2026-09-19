#!/usr/bin/env bash
set -euo pipefail
version="$(tr -d '\n' < VERSION)"
arch="$(dpkg --print-architecture)"
case "$arch" in amd64|arm64) ;; *) exit 2 ;; esac
test -f frontend/build/index.html
pkg-config --exists vips
revision="${GITHUB_SHA:-$(git rev-parse HEAD)}"
stage="$(mktemp -d)"
trap 'rm -rf "$stage"' EXIT
root="$stage/gooru"
mkdir -p "$root/DEBIAN" "$root/usr/bin" "$root/usr/share/gooru/frontend" "$root/usr/share/doc/gooru" "$root/lib/systemd/system" "$stage/work/debian"
cp -R frontend/build/. "$root/usr/share/gooru/frontend/"
cp LICENSE THIRD_PARTY_NOTICES.md "$root/usr/share/doc/gooru/"
cp 'systemd/gooru@.service' "$root/lib/systemd/system/"
CGO_ENABLED=1 go build -tags govips -trimpath -ldflags "-X=gooru.local/internal/buildinfo.Version=$version -X=gooru.local/internal/buildinfo.Revision=$revision -X=gooru.local/internal/buildinfo.Dirty=false -X=gooru.local/internal/buildinfo.Development=false" -o "$root/usr/bin/gooru" ./cmd/gooru
ldd "$root/usr/bin/gooru" | grep -F 'libvips.so'
printf 'Source: gooru\nSection: web\nPriority: optional\nMaintainer: Gooru project <noreply@github.com>\nStandards-Version: 4.6.2\n\nPackage: gooru\nArchitecture: any\nDepends: ${shlibs:Depends}\nDescription: personal media library\n' > "$stage/work/debian/control"
substvars="$(cd "$stage/work" && dpkg-shlibdeps -O -e"$root/usr/bin/gooru")"
depends="$(printf '%s\n' "$substvars" | sed -n 's/^shlibs:Depends=//p')"
[[ -n "$depends" && "$depends" == *libvips* ]]
printf 'Package: gooru\nVersion: %s\nSection: web\nPriority: optional\nArchitecture: %s\nMaintainer: Gooru project <noreply@github.com>\nHomepage: https://github.com/fiso64/gooru\nDepends: %s\nDescription: personal media library with WebUI and CLI\n' "$version" "$arch" "$depends" > "$root/DEBIAN/control"
mkdir -p dist
artifact="dist/gooru_${version}_linux_${arch}.deb"
dpkg-deb --build --root-owner-group "$root" "$artifact"
sha256sum "$artifact" > "$artifact.sha256"
