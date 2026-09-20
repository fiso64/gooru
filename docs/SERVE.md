# Install and run Gooru

`gooru serve` runs the HTTP API, authenticated media endpoints, uploads, background jobs, and the static SvelteKit frontend in one Go process.

Install a published package first; you do **not** need a source checkout, Go, or Node to run Gooru. For every YAML field and default, see [CONFIG.md](CONFIG.md). For endpoint-level integration, see [openapi.yaml](openapi.yaml).

## Install a release

Get the appropriate archive or package and its matching `.sha256` file from the [latest GitHub Release](https://github.com/fiso64/gooru/releases/latest). The examples below use v0.1.0; substitute the filenames from the release you downloaded when installing another version. On Linux, verify a downloaded asset from the directory containing both files with `sha256sum -c <asset-name>.sha256` before installing or extracting it.

### Debian or Ubuntu (amd64 / arm64)

Download `gooru_0.1.0_linux_amd64.deb` or `gooru_0.1.0_linux_arm64.deb` to match your machine, then install the local package (change the filename for arm64):

```bash
sudo apt install ./gooru_0.1.0_linux_amd64.deb
```

The package installs `gooru` at `/usr/bin/gooru`, the WebUI at `/usr/share/gooru/frontend`, and the `gooru@.service` systemd template. It uses the system's dynamically linked libvips runtime; APT installs its declared shared-library dependencies. For an interactive first run, set `server.frontend_dir` to `/usr/share/gooru/frontend` in your server config below. For a managed service instead, follow [systemd (Linux)](#systemd-linux).

### Nix / NixOS (x86_64-linux / aarch64-linux)

The versioned flake contains the Gooru binary, bundled WebUI, and dynamically linked libvips. Optionally configure the public `gooru` Cachix binary cache (for example, with `cachix use gooru` on a machine where the Cachix CLI and cache trust are configured) to substitute published builds. The cache is optional; Nix can build the package if a substitute is unavailable. No write token is needed to download public cache artifacts.

For a standalone Nix installation:

```bash
nix profile install 'github:fiso64/gooru/v0.1.0'
```

For a standalone `gooru serve` process, set `server.frontend_dir` to `<package-store-path>/share/gooru/frontend`; `nix path-info 'github:fiso64/gooru/v0.1.0'` prints the package store path. NixOS users should instead use the [flake module](#nixos), which configures the bundled frontend and per-instance state paths automatically. Pin the flake input to the release tag rather than a moving branch.

### Other Linux (amd64 / arm64)

Download the matching `gooru_0.1.0_linux_amd64.tar.gz` or `gooru_0.1.0_linux_arm64.tar.gz` and extract it. For amd64:

```bash
tar -xzf gooru_0.1.0_linux_amd64.tar.gz
cd gooru_0.1.0_linux_amd64
./gooru version
```

The extracted directory contains the `gooru` binary and `frontend/build` WebUI. **Run Gooru from that directory** so the default relative frontend path resolves, or set `server.frontend_dir` to the absolute path of the extracted `frontend/build`. Keep that directory with the binary when moving or upgrading the installation.

### Windows (amd64)

Download `gooru_0.1.0_windows_amd64.zip`, extract it, and open a terminal **inside** the extracted `gooru_0.1.0_windows_amd64` directory. Run `gooru.exe` from there (PowerShell: `\.\gooru.exe`) so the included `frontend/build` remains available. If you move the executable elsewhere, configure `server.frontend_dir` to the full path of the extracted WebUI. Do not use only the `.exe` without the bundled frontend for the web app.

Portable Linux and Windows archives omit optional libvips-backed thumbnails; Debian and Nix packages enable libvips. See [media tooling](#media-tooling) for optional dependencies and [building from source](DEVELOPMENT.md#build-from-source) for custom builds.

## First run

Run these commands using the installed `gooru` binary (`./gooru` for a Linux tarball or `\.\gooru.exe` in Windows PowerShell), from the working directory described above:

```bash
gooru init
gooru serve --print-default-config > serve.yaml
```

If you use a non-default database, select the same database path when initializing it or set `database.path` in the server config. For the Debian package, set `server.frontend_dir: /usr/share/gooru/frontend` in `serve.yaml`; for standalone Nix, set it to the package's `share/gooru/frontend` directory. Portable archive users can keep the default `frontend/build` when running from the extracted directory. On Windows, save the generated YAML as UTF-8 (older Windows PowerShell redirects output as UTF-16).

Conservative defaults bind to `127.0.0.1:5678`, require authentication, and disable uploads. Create an administrator and start the server:

```bash
gooru user create-admin --username alice --config serve.yaml
gooru serve --config serve.yaml
```

Open `http://127.0.0.1:5678` unless you changed `server.listen` or `server.public_url`. For non-interactive admin provisioning, supply the password through `GOORU_ADMIN_PASSWORD`; do not put passwords in YAML or command-line arguments.

See [CONFIG.md](CONFIG.md) for all settings, and [CLI.md](CLI.md) for library and user-management commands. [Building from source](DEVELOPMENT.md#build-from-source) is an alternative for development or custom builds, not a prerequisite for getting started.
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

A default source build includes a pure-Go image thumbnail path. Video thumbnails require `ffmpeg`; `ffprobe` is also used for media inspection/cache versioning.

To build with libvips as the primary image thumbnail backend:

```bash
go build -tags govips -o gooru ./cmd/gooru
```

That build requires libvips development files at build time and the shared library at runtime. The repository's Nix package enables this libvips backend by default because Nix supplies and tracks the native dependency reproducibly.

If an optional media tool is missing, core browse/tag/upload/original-media functionality remains available; derivative requests can return `unsupported_media`.

## Production packaging

Build and run a specific binary so you do not accidentally execute an older `gooru` from `PATH`:

```bash
go build -o gooru ./cmd/gooru
gooru serve --config serve.yaml
```

When packaging outside the source tree, ship the built frontend directory too and point `server.frontend_dir` at it.

## systemd (Linux)

The Linux package includes a `gooru@.service` template. Install the binary at
`/usr/bin/gooru` and the template under the systemd system-unit directory. For
an instance named `main`, create `/etc/gooru/main/serve.yaml` with unique
`server.listen`, `database.path: /var/lib/gooru-main/gooru.db`,
`media.cache_dir: /var/cache/gooru-main/media`, and
`server.frontend_dir: /usr/share/gooru/frontend`. Start it using
`systemctl enable --now gooru@main.service`.

The template creates separate, private state/cache directories and a dynamic
service identity for each instance, initializes a missing database on first
start with the partial hashing strategy, and does not put credentials in the
unit or YAML. Provision the first admin through the CLI using the instance
config and a password supplied privately. If using encryption, load the key
with a systemd `LoadCredential` drop-in and set
`GOORU_ENCRYPTION_KEY_FILE=%d/encryption-key` in that drop-in; keep the
plaintext key out of configuration files and service logs. External libraries
and upload targets require explicit read/write access for the service identity;
use a dedicated static per-instance user and matching unit override where
stable filesystem ACLs or group membership are required. Each instance needs
its own listen address and should not share a database or mutable cache with
another instance.

## NixOS

The flake exports `gooru.nixosModules.default`. Keep a shared default package at `services.gooru.package` and define deployments under `services.gooru.instances.<name>`. Each enabled instance must set `settings.server.listen` explicitly.

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
