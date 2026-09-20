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
test "$(stat -c '%U:%G:%a' /var/lib/gooru-citest)" = "_gooru-citest:_gooru-citest:700"
test "$(stat -c '%U:%G:%a' /var/cache/gooru-citest)" = "_gooru-citest:_gooru-citest:700"
if sudo -u _gooru-second test -r /var/lib/gooru-citest; then
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
test "$(sudo stat -c '%U:%G:%a' /var/lib/gooru-citest/gooru.db)" = "_gooru-citest:_gooru-citest:600"

sudo systemctl stop gooru@citest.service
sudo -u _gooru-citest env GOORU_ADMIN_PASSWORD='ephemeral-ci-password' \
  /usr/bin/gooru --config /etc/gooru/citest/serve.yaml user create-admin --username ci-admin
sudo systemctl start gooru@citest.service
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
