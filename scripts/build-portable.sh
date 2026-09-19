#!/usr/bin/env bash
set -euo pipefail

os="${1:?usage: build-portable.sh OS ARCH}"
arch="${2:?usage: build-portable.sh OS ARCH}"
case "$os/$arch" in
  linux/amd64|linux/arm64|windows/amd64) ;;
  *) echo "unsupported target: $os/$arch" >&2; exit 2 ;;
esac
version="$(tr -d '\n' < VERSION)"
revision="${GITHUB_SHA:-$(git rev-parse HEAD)}"
mkdir -p dist
[[ -d frontend/build ]] || { echo "build frontend first" >&2; exit 1; }

stage="$(mktemp -d)"
trap 'rm -rf "$stage"' EXIT
root="$stage/gooru_${version}_${os}_${arch}"
mkdir -p "$root/frontend/build"
cp -R frontend/build/. "$root/frontend/build/"
cp LICENSE THIRD_PARTY_NOTICES.md docs/DEVELOPMENT.md "$root/"

suffix=""
if [[ "$os" == windows ]]; then suffix=".exe"; fi
CGO_ENABLED=0 GOOS="$os" GOARCH="$arch" go build -trimpath \
  -ldflags "-X=gooru.local/internal/buildinfo.Version=$version -X=gooru.local/internal/buildinfo.Revision=$revision -X=gooru.local/internal/buildinfo.Dirty=false -X=gooru.local/internal/buildinfo.Development=false" \
  -o "$root/gooru$suffix" ./cmd/gooru
if [[ "$os" == windows ]]; then
  output_dir="$(pwd)/dist"
  (cd "$stage" && zip -qr "$output_dir/gooru_${version}_${os}_${arch}.zip" "${root##*/}")
else
  tar -C "$stage" -czf "dist/gooru_${version}_${os}_${arch}.tar.gz" "${root##*/}"
fi

artifact="dist/gooru_${version}_${os}_${arch}.tar.gz"
if [[ "$os" == windows ]]; then artifact="dist/gooru_${version}_${os}_${arch}.zip"; fi
sha256sum "$artifact" > "$artifact.sha256"
