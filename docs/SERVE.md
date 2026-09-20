# Run and deploy Gooru

`gooru serve` runs the API and WebUI in one process. For server settings, see [CONFIG.md](CONFIG.md); for API details, see [openapi.yaml](openapi.yaml).

## Install Gooru

For Debian, Linux tarballs, and Windows ZIPs, get the package from the [latest release](https://github.com/fiso64/gooru/releases/latest). On NixOS, use the published Nix package from Gooru's Cachix cache.

### Debian / Ubuntu

Download the `.deb` for your architecture (amd64 or arm64) and install it:

```bash
sudo apt install ./gooru_*.deb
```

Run this in the download directory with just the Gooru package matching your system. The package includes the WebUI and a `gooru@.service` template. To configure an instance, create an administrator, and run Gooru in the background, follow the [Debian/Ubuntu service setup](#systemd-linux) below. If you prefer to run it manually, set `server.frontend_dir: /usr/share/gooru/frontend` in your config and follow [First run](#first-run).

### NixOS via Cachix

Use the prebuilt Gooru package from [Cachix](https://gooru.cachix.org/) with the declarative [NixOS deployment](#nixos-deployment). That guide includes the cache settings, flake input, and service configuration.

### Linux tarball

Download the `.tar.gz` for your architecture, extract it, and run `./gooru` from the extracted directory. Keep the included `frontend/build` directory beside the binary; Gooru uses it to serve the WebUI.

### Windows

Download and extract the Windows ZIP. Open PowerShell in the extracted directory and run `.\gooru.exe`. Keep the included `frontend/build` directory alongside the executable.

Debian and Nix builds use libvips for thumbnails; portable Linux and Windows builds do not. To enable libvips in your own build, see [building from source](DEVELOPMENT.md#build-from-source).

## First run

For manually run Debian, Linux tarball, or Windows ZIP installations, initialize the database, generate a config, and create an administrator. For the Debian/Ubuntu `gooru@.service`, use the [managed service setup](#systemd-linux) instead; do not initialize a separate database with these commands:

```bash
gooru init
gooru serve --print-default-config > serve.yaml
gooru user create-admin --username alice --config serve.yaml
gooru serve --config serve.yaml
```

For NixOS, use the [declarative module configuration](#nixos-deployment) instead of these manual commands.

Use `./gooru` for a Linux tarball or `.\gooru.exe` for the Windows ZIP instead of `gooru` above. On Windows PowerShell 5, generate `serve.yaml` with `cmd /c ".\gooru.exe serve --print-default-config > serve.yaml"` so the file is UTF-8.

Set `server.frontend_dir` to `/usr/share/gooru/frontend` for a Debian install. The extracted Linux and Windows archives work with the default `frontend/build` when run from their directory. When using a different database path, use it for both initialization and serving.

Open <http://127.0.0.1:5678>. By default Gooru listens only on your computer, requires login, and disables uploads. See [CONFIG.md](CONFIG.md) for other settings and [CLI.md](CLI.md) for library commands.

## Uploads

Uploads are off by default. Enable them only with an explicit target:

```yaml
uploads:
  enabled: true
  targets:
    - id: default
      name: Default
      path: /srv/gooru/incoming
  max_file_size_bytes: 104857600
```

Upload target paths must be absolute. Gooru exposes target IDs and display names to clients, not the configured filesystem paths.

Uploads are staged before being committed to their final names. Same-name uploads are renamed by default; API clients may explicitly request `conflict_policy=error` to reject the request when a destination path collides.

## Network access

Keep the default loopback bind for single-machine use:

```yaml
server:
  listen: 127.0.0.1:5678
```

For LAN access:

```yaml
server:
  listen: 0.0.0.0:5678

auth:
  enabled: true
```

Gooru refuses unauthenticated non-loopback serving unless `auth.allow_unsafe_no_auth_non_loopback` is deliberately enabled.

`gooru serve` does not terminate TLS. Use a reverse proxy, VPN, SSH tunnel, or another trusted network layer for access beyond a trusted LAN.

## Authentication and cookies

Authentication uses DB-backed users and server-side sessions. The browser receives an HttpOnly session cookie. Mutating cookie-authenticated API requests also require a CSRF token in `X-Gooru-CSRF`.

`auth.cookie_secure: auto` is the normal choice; configure the cookie and proxy/public URL settings to match your deployment.

Legacy token-auth options (`auth.token`, `auth.token_env`, `auth.token_file`, and `--auth-token`) are intentionally rejected.

## Encryption at rest

Protected storage can encrypt:

- the SQLite database;
- files under configured upload-target roots;
- generated media derivatives.

It does **not** rewrite arbitrary external library files outside managed upload roots.

Generate a 256-bit key and protect the file permissions:

```bash
umask 077
openssl rand -base64 32 > /srv/gooru/encryption.key
```

Then configure one key source, for example:

```yaml
encryption:
  enabled: true
  key_file: /srv/gooru/encryption.key
```

Alternatively use `GOORU_ENCRYPTION_KEY` or `GOORU_ENCRYPTION_KEY_FILE`. Exactly one key source must be active. Back up the key separately from encrypted data; the wrong key cannot decrypt content written with the original key.

See [CONFIG.md#encryption](CONFIG.md#encryption) for the complete rules.

## Media tooling

Debian and Nix packages enable libvips; portable Linux and Windows builds use the pure-Go image path. Video thumbnails need `ffmpeg`, and media inspection uses `ffprobe`. See [optional build tags](DEVELOPMENT.md#optional-build-tags) if you want a custom build.

## systemd (Linux)

The Debian/Ubuntu package includes a `gooru@.service` template and a `gooru-instance-setup` command. This setup is for **new installations**. For an instance named `main`, create its dedicated system account and private state/cache directories:

```bash
sudo gooru-instance-setup main
```

Create the instance config, pointing to the packaged frontend and the instance-owned database and cache:

```bash
sudo install -d -m 0755 /etc/gooru/main
sudo tee /etc/gooru/main/serve.yaml > /dev/null <<'YAML'
server:
  listen: 127.0.0.1:5678
  frontend_dir: /usr/share/gooru/frontend
database:
  path: /var/lib/gooru-main/gooru.db
media:
  cache_dir: /var/cache/gooru-main/media
YAML
sudo chmod 0644 /etc/gooru/main/serve.yaml
```

Start and stop the service once so its normal initialization creates the database, then create the administrator as the instance's dedicated account. Gooru prompts privately for the password:

```bash
sudo systemctl daemon-reload
sudo systemctl start gooru@main.service
sudo systemctl stop gooru@main.service
sudo -u _gooru-main /usr/bin/gooru --config /etc/gooru/main/serve.yaml \
  user create-admin --username alice
```

Enable the service at boot and start it:

```bash
sudo systemctl enable --now gooru@main.service
sudo systemctl status --no-pager gooru@main.service
```

Open <http://127.0.0.1:5678> on the server and sign in as `alice`. If startup fails, inspect `sudo journalctl -u gooru@main.service -e`. The example listens only on the host; see [Network access](#network-access) for other deployments and [CONFIG.md](CONFIG.md) for configuration options.

Each named instance receives a separate `_gooru-<name>` system account, `/var/lib/gooru-<name>` state directory and `/var/cache/gooru-<name>` cache directory. For another instance, run `sudo gooru-instance-setup <name>`, create its own `/etc/gooru/<name>/serve.yaml`, choose a different listen port, and substitute its name in the commands above. Do not share database or mutable cache paths between instances. Stop an instance before running CLI commands that modify its database.

For access to external library or upload directories, grant the instance account the necessary read/write permissions explicitly. For encrypted storage, keep plaintext keys out of the YAML and logs; use a systemd `LoadCredential` drop-in and `GOORU_ENCRYPTION_KEY_FILE=%d/encryption-key` for the service, and provide the same key securely when running administrator CLI commands against an encrypted database.

## NixOS deployment

Use [Gooru's Cachix cache](https://gooru.cachix.org/) as the binary source for NixOS. Add its substituter and public signing key to your NixOS configuration:

```nix
nix.settings = {
  extra-substituters = [ "https://gooru.cachix.org" ];
  extra-trusted-public-keys = [
    "gooru.cachix.org-1:I2qP1U67fpxRdT1y4pmb0sj/X4Xs846Yi4/P481+SnM="
  ];
};
```

If this is the first time you have configured the cache on the host, apply these settings in a system rebuild before enabling the Gooru instance so the Nix daemon can use Cachix for the first Gooru build.

Add Gooru to your NixOS flake inputs:

```nix
inputs.gooru.url = "github:fiso64/gooru/main";
```

The `main` branch contains the latest merged changes, which can be newer than the latest tagged release. Gooru publishes Cachix binaries for `main` on x86_64 and aarch64 Linux; if you update before that revision's cache build finishes, Nix may build the package locally. Your `flake.lock` pins the revision in use; to upgrade Gooru, run `nix flake update gooru` in your system flake directory and rebuild your NixOS system. Include `gooru` in your flake's `outputs` arguments and add `gooru.nixosModules.default` to the host's `nixosSystem.modules`. In that host's NixOS configuration, enable an instance:

```nix
services.gooru.instances.main = {
  enable = true;
  settings.server.listen = "127.0.0.1:5678";
};
```

Your `flake.lock` pins the selected revision. When the matching substitute is available, Nix downloads Gooru from Cachix instead of building it locally. The module sets the bundled WebUI path, initializes a missing database, and manages the service. No per-user installation or manual server startup is needed.

To create the first administrator, configure `services.gooru.instances.main.admins` with a runtime `passwordFile`, as shown below. Keep secrets out of the Nix store.

### Two-instance example

```nix
services.gooru.instances = {
  main = {
    enable = true;
    settings = {
      server.listen = "127.0.0.1:5678";
      uploads = {
        enabled = true;
        targets = [{
          id = "default";
          name = "Default";
          path = "/srv/gooru/incoming";
        }];
      };
    };
    admins.primary = {
      username = "admin";
      passwordFile = "/run/gooru-main-admin";
    };
  };

  test = {
    enable = true;
    settings = {
      server.listen = "127.0.0.1:5679";
      encryption = {
        enabled = true;
        key_file = "/run/gooru-test-key";
      };
      ui.accent_color = "#2f80ed";
    };
    initialDatabase.hashingStrategy = "full";
  };
};
```

Set `services.gooru.package` to change the shared default package. Set `services.gooru.instances.<name>.package` for an instance-specific override; its generated `server.frontend_dir` follows the effective package.

### First boot and administrators

`services.gooru.instances.<name>.initialDatabase.hashingStrategy` accepts `"partial"` or `"full"` and applies only when that instance creates a missing database. Existing database state remains authoritative.

`services.gooru.instances.<name>.admins` is ongoing declarative state. The attribute name is a stable declaration identity; `username` is mutable desired state. Password files are runtime paths loaded through namespaced systemd credentials and should not contain plaintext via Nix-store paths. Removing an admin declaration does not delete the Gooru user.

### Per-instance resources

For an instance named `main`, the defaults are:

- unit, user, and group: `gooru-main`;
- config: `/etc/gooru/main/serve.yaml`;
- state/database: `/var/lib/gooru-main`;
- media cache: `/var/cache/gooru-main`;
- declarative-admin state below `/var/lib/gooru-main/declarative-admins`.

Other names receive the same collision-free namespacing. If you override an instance's `user` or `group`, create it separately; the module automatically creates only the default `gooru-<name>` identity.

Create upload directories with permissions appropriate for each instance. Set `services.gooru.instances.<name>.openFirewall = true` only when that instance's literal `HOST:PORT` listen address should be opened in the host firewall.


## Troubleshooting

Print the defaults supported by the exact binary you are running:

```bash
gooru serve --print-default-config
```

For server diagnostics, set:

```yaml
logging:
  level: debug
```

Debug request logging intentionally avoids raw query strings, request bodies, filenames, filesystem paths, credentials, and session/CSRF secrets.
