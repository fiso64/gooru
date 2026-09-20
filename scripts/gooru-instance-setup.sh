#!/usr/bin/env bash
# Provision a stable, separate Debian/Ubuntu account for one Gooru instance.
# Never delete a system account or an existing library during package removal.
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
  fail "stop gooru@$name.service before provisioning or migrating its files"
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
  local public="$1" private="$2" owner uid target
  uid="$(id -u "$account")"
  if [[ -L "$public" ]]; then
    # The only accepted symlink is systemd's previous DynamicUser= layout.
    target="$(readlink "$public")"
    if [[ "$target" != "private/$(basename "$public")" && "$target" != "$private" ]] \
        || [[ ! -d "$private" || -L "$private" ]]; then
      fail "unexpected legacy symlink at $public; do not replace it automatically"
    fi
    rm -- "$public"
    mv -T -- "$private" "$public"
    chown -hR -- "$account:$account" "$public"
  elif [[ -e "$public" ]]; then
    if [[ ! -d "$public" || -L "$public" || -e "$private" || -L "$private" ]]; then
      fail "conflicting state directories at $public and $private; inspect them before proceeding"
    fi
    owner="$(stat -c '%u' -- "$public")"
    if [[ "$owner" != "$uid" ]]; then
      if [[ "$owner" != 0 ]] || [[ -n "$(find "$public" -mindepth 1 -maxdepth 1 -print -quit)" ]]; then
        fail "existing directory $public is not owned by $account; refusing to change its contents"
      fi
    fi
  elif [[ -d "$private" && ! -L "$private" ]]; then
    # Safe retry if an earlier migration removed the public link before move.
    mv -T -- "$private" "$public"
    chown -hR -- "$account:$account" "$public"
  elif [[ -e "$private" || -L "$private" ]]; then
    fail "unexpected private directory at $private"
  fi
  install -d -m 0700 -o "$account" -g "$account" -- "$public"
}

prepare_dir "$state" "/var/lib/private/gooru-$name"
prepare_dir "$cache" "/var/cache/private/gooru-$name"
printf 'Gooru instance %s uses account %s (state: %s)\n' "$name" "$account" "$state"
