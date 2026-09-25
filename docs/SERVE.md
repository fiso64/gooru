# Run and deploy Gooru

`gooru serve` runs serves the WebUI. For server settings, see [CONFIG.md](CONFIG.md); for API details, see [openapi.yaml](openapi.yaml).

## Install Gooru

For Debian, Linux tarballs, and Windows ZIPs, get the package from the [latest release](https://github.com/fiso64/gooru/releases/latest).  
On NixOS, use the published Nix package from Gooru's Cachix cache.

### Debian / Ubuntu

Download the `.deb` for your architecture (amd64 or arm64) and install it:

```bash
sudo apt install ./gooru_*.deb
```

The package includes the a `gooru@.service` template. To configure an instance, follow the [Debian/Ubuntu service setup](#systemd-linux) below. If you prefer to run it manually, set `server.frontend_dir: /usr/share/gooru/frontend` in your config and follow [First run](#first-run).

### NixOS via Cachix

Use the prebuilt Gooru package with the declarative [NixOS deployment](#nixos-deployment).

### Linux tarball

Download the `.tar.gz` for your architecture, extract it, and run `./gooru` from the extracted directory.

### Windows

Download and extract the Windows ZIP. Open PowerShell in the extracted directory and run `gooru.exe`.

### Build differences

Debian and Nix builds use libvips for thumbnails; portable Linux and Windows builds do not. To enable libvips in your own build, see [building from source](DEVELOPMENT.md#build-from-source).

## First run

For Linux tarball or Windows ZIP installations, initialize the database, generate a config, and create an administrator:

```bash
gooru init
gooru serve --print-default-config > serve.yaml
gooru user create-admin --username alice --config serve.yaml
gooru serve --config serve.yaml
```

Open <http://127.0.0.1:5678>. By default Gooru listens only on your computer, and provisions one persistent upload destination. Once signed in as an administrator, you can upload files in the WebUI. See [CONFIG.md](CONFIG.md) for other settings and [CLI.md](CLI.md) for library commands.

## Uploads

Uploads are enabled by default. When no targets are specified, Gooru prepares a persistent upload directory. You can upload after creating an administrator and signing in. To choose your own destination instead, configure an explicit target:

```yaml
uploads:
  enabled: true
  targets:
    - id: default
      name: Default
      path: /srv/gooru/incoming
  max_file_size_bytes: 104857600
```

If authentication is disabled, uploads remain disabled unless `uploads.enabled: true` is explicitly configured. The automatic directory must be writable by the server account. 

Same-name uploads are renamed if their content does not exist in the library, otherwise skipped.

## Network access

Keep the default loopback bind for single-machine or reverse proxy use:

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

Alternatively use `GOORU_ENCRYPTION_KEY` or `GOORU_ENCRYPTION_KEY_FILE`. The key is like a password, so back it up separately from the encrypted data and don't lose it.

See [CONFIG.md#encryption](CONFIG.md#encryption) for the complete rules.

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

Create the database, then create the administrator as the instance's dedicated account. Gooru prompts privately for the password:

```bash
sudo gooru-instance main init \
  --if-missing \
  --hashing-strategy partial

sudo gooru-instance main user create-admin \
  --username alice
```

Enable the service at boot and start it:

```bash
sudo systemctl enable --now gooru@main.service
sudo systemctl status --no-pager gooru@main.service
```

Open <http://127.0.0.1:5678> on the server and sign in. If startup fails, inspect `sudo journalctl -u gooru@main.service -e`. The example listens only on the host; see [Network access](#network-access) for other deployments and [CONFIG.md](CONFIG.md) for configuration options.

## NixOS deployment

Use [Gooru's Cachix repo](https://gooru.cachix.org/) as the binary source on NixOS. Add it to your NixOS configuration:

```nix
nix.settings = {
  extra-substituters = [ "https://gooru.cachix.org" ];
  extra-trusted-public-keys = [
    "gooru.cachix.org-1:I2qP1U67fpxRdT1y4pmb0sj/X4Xs846Yi4/P481+SnM="
  ];
};
```

If this is the first time you have configured the cache on the host, it might require a separate rebuild before enabling the Gooru instance so the Nix daemon can use Cachix for the first Gooru build.

Add Gooru to your NixOS flake inputs:

```nix
inputs.gooru.url = "github:fiso64/gooru/main";
```

The `main` branch is the stable release branch; development and unreleased changes remain on `develop`. To upgrade after a new release is published, run `nix flake update gooru` in your system flake directory and rebuild your NixOS system. Gooru publishes Cachix binaries for each release on x86_64 and aarch64 Linux.

Include `gooru` in your flake's `outputs` arguments and add `gooru.nixosModules.default` to the host's `nixosSystem.modules`. In that host's NixOS configuration, enable an instance:

```nix
services.gooru.instances.main = {
  enable = true;
  settings.server.listen = "127.0.0.1:5678";

  admins.primary = {
    username = "admin";
    passwordFile = "/var/lib/gooru-secrets/admin-password";
  };
};
```

<!-- TODO: Document this in a separate small section that applies to both debian and nixos.  -->
<!-- 
Once the NixOS configuration is applied, local CLI operations use the same wrapper as Debian: `sudo gooru-instance main tag /srv/photos/cat.jpg favorite` (or `sudo gooru-instance main count favorite`). -->

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

When upload settings are omitted, each instance uses its own private persistent upload directory under its managed state directory. Create any explicitly configured custom upload directories with permissions appropriate for each instance. Set `services.gooru.instances.<name>.openFirewall = true` only when that instance's literal `HOST:PORT` listen address should be opened in the host firewall.


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
