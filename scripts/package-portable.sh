#!/usr/bin/env bash
set -euo pipefail

os="${1:?usage: package-portable.sh OS ARCH}"
arch="${2:?usage: package-portable.sh OS ARCH}"
version="$(tr -d '\n' < VERSION)"
revision="${GITHUB_SHA:-$(git rev-parse HEAD)}"
mkdir -p dist

stage="$(mktemp -d)"
trap 'rm -rf "$stage"' EXIT
root="$stage/gooru_${version}_${os}_${arch}"
mkdir -p "$root/frontend"
cp -R frontend/build/. "$root/frontend/"
cp LICENSE THIRD_PARTY_NOTICES.md "$root/"

suffix=""
if [[ "$os" == windows ]]; then suffix=".exe"; fi
CGO_ENABLED=0 GOOS="$os" GOARCH="$arch" go build -trimpath \
  -ldflags "-X=gooru.local/internal/buildinfo.Version=$version -X=gooru.local/internal/buildinfo.Revision=$revision -X=gooru.local/internal/buildinfo.Dirty=false -X=gooru.local/internal/buildinfo.Development=false" \
  -o "$root/gooru$suffix" ./cmd/gooru
if [[ "$os" == windows ]]; then
  (cd "$stage" && zip -qr "$OLDPWD/dist/gooru_${version}_${os}_${arch}.zip" "${root##*/}")
else
  tar -C "$stage" -czf "dist/gooru_${version}_${os}_${arch}.tar.gz" "${root##*/}"
fi
