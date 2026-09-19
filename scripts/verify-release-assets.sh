#!/usr/bin/env bash
set -euo pipefail
version="$(tr -d '\n' < VERSION)"
[[ "$version" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]
for target in linux_amd64.tar.gz linux_arm64.tar.gz windows_amd64.zip linux_amd64.deb linux_arm64.deb; do
  artifact="gooru_${version}_${target}"
  test -s "dist/$artifact"
  (cd dist && sha256sum -c "$artifact.sha256")
done
test "$(find dist -maxdepth 1 -type f | wc -l)" -eq 10
