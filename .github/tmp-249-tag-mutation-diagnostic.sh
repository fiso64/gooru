#!/usr/bin/env bash
set -euo pipefail

{
  echo '=== mutateTagPaths references ==='
  rg -n -C 12 'mutateTagPaths' . || true
  echo
  echo '=== tag mutation helper candidates ==='
  rg -n -C 8 'func .*Tag.*Path|mutate.*Tag|TagPaths|tagPaths' internal || true
} > .github/tmp-249-tag-mutation-diagnostic.txt

git config user.name "gooru-maintainer-bot"
git config user.email "maintainer@localhost"
git add .github/tmp-249-tag-mutation-diagnostic.txt
git commit -m 'ci: capture #249 tag mutation diagnostic'
git push origin HEAD:feat/249-upload-item-tags
