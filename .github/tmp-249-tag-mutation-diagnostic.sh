#!/usr/bin/env bash
set -euo pipefail

{
  echo '=== mutateTagPaths references ==='
  rg -n -C 12 'mutateTagPaths' . || true
  echo
  echo '=== TagFiles definitions and calls ==='
  rg -n -C 12 'func .*TagFiles|TagFiles\(' . || true
  echo
  echo '=== SetTags/Untag definitions and calls ==='
  rg -n -C 10 'func .*SetTagsForFiles|func .*UntagFiles|SetTagsForFiles\(|UntagFiles\(' . || true
  echo
  echo '=== TagOperationResult definitions ==='
  rg -n -C 10 'type TagOperationResult|TagOperationResult' internal pkg || true
} > .github/tmp-249-tag-mutation-diagnostic.txt

git config user.name "gooru-maintainer-bot"
git config user.email "maintainer@localhost"
git add .github/tmp-249-tag-mutation-diagnostic.txt
git commit -m 'ci: capture extended #249 tag mutation diagnostic'
git push origin HEAD:feat/249-upload-item-tags
