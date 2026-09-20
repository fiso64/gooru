#!/usr/bin/env bash
# Smoke-test the real Debian package on a disposable GitHub-hosted Ubuntu VM.
set -euo pipefail
if [[ "$GITHUB_ACTIONS" != true || "$RUNNER_ENVIRONMENT" != github-hosted ]]; then
  echo "Only run this script on a disposable GitHub-hosted runner" >&2
  exit 1
fi
test "$#" = 1
sudo apt-get install -y "./$1"

sudo gooru-instance-setup citest
sudo gooru-instance-setup citest
sudo gooru-instance-setup second
test "$(stat -c '%U:%G:%a' /var/lib/gooru-citest)" = "gooru-citest:gooru-citest:700"
test "$(stat -c '%U:%G:%a' /var/cache/gooru-citest)" = "gooru-citest:gooru-citest:700"
if sudo -u gooru-second test -r /var/lib/gooru-citest; then
  echo "second instance can access first instance's state" >&2
  exit 1
fi

sudo install -d -m 0755 /etc/gooru/citest
printf 'server:\n  listen: 127.0.0.1:45738\n  frontend_dir: /usr/share/gooru/frontend\ndatabase:\n  path: /var/lib/gooru-citest/gooru.db\nmedia:\n  cache_dir: /var/cache/gooru-citest/media\n' |
  sudo tee /etc/gooru/citest/serve.yaml >/dev/null
sudo chmod 0644 /etc/gooru/citest/serve.yaml
sudo systemctl daemon-reload
sudo systemctl start gooru@citest.service
trap 'sudo systemctl stop gooru@citest.service || true' EXIT
sudo systemctl is-active --quiet gooru@citest.service
test "$(sudo stat -c '%U:%G:%a' /var/lib/gooru-citest/gooru.db)" = "gooru-citest:gooru-citest:600"
test "$(sudo stat -c '%U:%G:%a' /var/lib/gooru-citest/uploads)" = "gooru-citest:gooru-citest:700"
if sudo -u gooru-second /usr/bin/gooru --config /etc/gooru/citest/serve.yaml count >/tmp/gooru-wrong-user.log 2>&1; then
  echo "another account unexpectedly loaded the instance upload state" >&2
  exit 1
fi
grep -q "must belong to the current service user" /tmp/gooru-wrong-user.log
test "$(sudo stat -c '%U:%G:%a' /var/lib/gooru-citest/uploads)" = "gooru-citest:gooru-citest:700"

sudo systemctl stop gooru@citest.service
sudo env GOORU_ADMIN_PASSWORD='ephemeral-ci-password' \
  gooru-instance citest user create-admin --username ci-admin
# Run the CLI against the instance while its service is online.
sudo systemctl start gooru@citest.service
sudo install -d -m 0755 /srv/gooru-ci
printf 'ci test file\n' | sudo tee /srv/gooru-ci/note.txt >/dev/null
sudo chmod 0644 /srv/gooru-ci/note.txt
sudo gooru-instance citest tag /srv/gooru-ci/note.txt favorite
test "$(sudo gooru-instance citest count favorite)" = 1
if sudo gooru-instance absent count favorite; then
  echo "unknown instance unexpectedly accepted by wrapper" >&2
  exit 1
fi
if gooru-instance citest count favorite; then
  echo "unprivileged CLI wrapper call unexpectedly succeeded" >&2
  exit 1
fi
ready=no
for _ in $(seq 1 40); do
  if curl -fsS http://127.0.0.1:45738/ >/tmp/gooru-citest-index.html 2>/dev/null; then
    ready=yes
    break
  fi
  sleep 0.5
done
if [[ "$ready" != yes ]]; then
  sudo journalctl -u gooru@citest.service -n 100 --no-pager
  exit 1
fi
grep -Eiq '<!doctype html|<html' /tmp/gooru-citest-index.html
echo "Packaged Debian service: init, isolated accounts, admin CLI and WebUI passed"
