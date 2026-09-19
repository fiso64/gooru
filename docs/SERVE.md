# Serve the web app

`gooru serve` runs the HTTP API, authenticated media endpoints, uploads, background jobs, and the static SvelteKit frontend in one Go process.

For every YAML field and default, see [CONFIG.md](CONFIG.md). For endpoint-level integration, see [openapi.yaml](openapi.yaml).

## From a source checkout

### 1. Build the frontend

```bash
cd frontend
npm ci
npm run build
cd ..
```

The generated static site is written to `frontend/build`. Node is not required at runtime once the frontend has been built. Because `frontend/build` is intentionally ignored by Git, pulling newer source does not refresh an existing build directory; rerun `npm run build` after frontend source changes before restarting `gooru serve`, otherwise the new backend can serve an older WebUI bundle.

### 2. Build the Go binary

```bash
go build -o gooru ./cmd/gooru
```

### 3. Initialize the database if needed

```bash
gooru init
```

If you use a non-default database, pass the same `--database` path when initializing it or set `database.path` in the server config.

### 4. Generate a config

```bash
gooru serve --print-default-config > serve.yaml
```

For local use, the most important defaults are already conservative:

- listen on `127.0.0.1:5678`;
- authentication enabled;
- uploads disabled;

### 5. Create the first admin

```bash
gooru user create-admin --username alice --config serve.yaml
```

For non-interactive provisioning, supply the password through `GOORU_ADMIN_PASSWORD`; do not put normal usernames or passwords in YAML and avoid command-line password arguments.

### 6. Start the server

```bash
gooru serve --config serve.yaml
```

Open `http://127.0.0.1:5678` unless you changed `server.listen` or `server.public_url`.

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

A default source build includes a pure-Go image thumbnail path. Video thumbnails require `ffmpeg`; `ffprobe` is also used for media inspection/cache versioning. PDF first-page thumbnails require the optional Poppler `pdftoppm` executable on the server's `PATH`; without it, PDFs remain available as original files but PDF thumbnail generation returns `unsupported_media`. PDF scrolling in the WebUI is not yet supported.

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
