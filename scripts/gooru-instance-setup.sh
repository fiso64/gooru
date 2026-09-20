#!/usr/bin/env bash
# Provision a stable, separate Debian/Ubuntu account for one Gooru instance.
# Provision new Debian/Ubuntu instances; never alter an existing owner's files.
set -euo pipefail
umask 077

fail() { printf 'gooru-instance-setup: %s\n' "$*" >&2; exit 1; }

if (( EUID != 0 )); then fail "run as root (sudo gooru-instance-setup NAME)"; fi
if (( $# != 1 )) || [[ ! "$1" =~ ^[a-z][a-z0-9_-]{0,16}$ ]]; then
  fail "instance name must begin with a lowercase letter and contain only lowercase letters, digits, _ or - (up to 17 characters)"
fi
name="$1"
account="_gooru-$name"
state="/var/lib/gooru-$name"
cache="/var/cache/gooru-$name"

# An existing instance must be stopped before changing its database ownership.
if systemctl is-active --quiet "gooru@$name.service"; then
  fail "stop gooru@$name.service before changing its account settings"
fi

entry="$(getent passwd "$account" || true)"
if [[ -z "$entry" ]]; then
  useradd --system --user-group --no-create-home --home-dir "$state" \
    --shell /usr/sbin/nologin "$account"
else
  IFS=: read -r _ _ _ _ _ home shell <<< "$entry"
  if [[ "$home" != "$state" || "$shell" != /usr/sbin/nologin ]] \
      || [[ "$(id -gn "$account")" != "$account" ]]; then
    fail "existing account $account has unexpected settings; resolve the account conflict manually"
  fi
fi

prepare_dir() {
  local path="$1" expected
  expected="$(id -u "$account"):$(id -g "$account")"
  if [[ -L "$path" || ( -e "$path" && ! -d "$path" ) ]]; then
    fail "unexpected instance path $path; only new installations are supported"
  fi
  if [[ -d "$path" && "$(stat -c '%u:%g' "$path")" != "$expected" ]]; then
    fail "existing directory $path belongs to another account; refusing to change existing data"
  fi
  install -d -m 0700 -o "$account" -g "$account" -- "$path"
}

prepare_dir "$state"
prepare_dir "$cache"
printf 'Gooru instance %s uses account %s (state: %s)\n' "$name" "$account" "$state"
