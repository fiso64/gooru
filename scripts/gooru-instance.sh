#!/usr/bin/env bash
# Run a CLI command against the database belonging to a packaged Gooru instance.
set -euo pipefail

fail() { printf 'gooru-instance: %s\n' "$*" >&2; exit 1; }

if (( EUID != 0 )); then
  fail "run as root: sudo gooru-instance NAME COMMAND [ARGS...]"
fi
if (( $# < 2 )) || [[ ! "$1" =~ ^[a-z][a-z0-9_-]{0,16}$ ]]; then
  fail "usage: sudo gooru-instance NAME COMMAND [ARGS...] (NAME: lowercase letter, then lowercase letters/digits/_/-; max 17 characters)"
fi

name="$1"
shift
account="gooru-$name"
config="/etc/gooru/$name/serve.yaml"

if ! id -u "$account" >/dev/null 2>&1; then
  fail "instance account $account does not exist; run gooru-instance-setup $name first"
fi
if [[ ! -f "$config" ]]; then
  fail "instance configuration $config does not exist"
fi

exec /usr/sbin/runuser -u "$account" -- /usr/bin/gooru --config "$config" "$@"
