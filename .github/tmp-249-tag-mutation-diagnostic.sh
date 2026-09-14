#!/usr/bin/env bash
set -euo pipefail

{
  echo '=== exact mutation function definitions ==='
  rg -n '^func .*TagFiles|^func .*SetTagsForFiles|^func .*UntagFiles' . || true
  echo
  echo '=== transaction helpers near tag mutation code ==='
  rg -n -C 6 'BeginTx|Begin\(|Commit\(|Rollback\(' gooru internal | head -n 400 || true
} > .github/tmp-249-tag-mutation-diagnostic.txt

git config user.name "gooru-maintainer-bot"
git config user.email "maintainer@localhost"
git add .github/tmp-249-tag-mutation-diagnostic.txt
git commit -m 'ci: capture narrowed #249 mutation diagnostic'
git push origin HEAD:feat/249-upload-item-tags
