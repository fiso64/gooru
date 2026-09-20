#!/bin/sh
# Reconcile configured instances on package installation and upgrade, including
# the previous DynamicUser=yes private directory layout. Keep data on removal.
set -eu
[ "$1" = configure ] || exit 0
if [ -d /run/systemd/system ]; then
  systemctl daemon-reload
fi
[ -d /etc/gooru ] || exit 0
for cfg in /etc/gooru/*/serve.yaml; do
  [ -f "$cfg" ] || continue
  name="$(basename "$(dirname "$cfg")")"
  was_active=no
  if systemctl is-active --quiet "gooru@$name.service"; then
    was_active=yes
    systemctl stop "gooru@$name.service"
  fi
  /usr/sbin/gooru-instance-setup "$name"
  if [ "$was_active" = yes ]; then
    systemctl start "gooru@$name.service"
  fi
done
exit 0
