# Gooru

Gooru is a high-performance, content-centric command-line tool for tagging and organizing local files. It uses content hashing to identify files, making it resilient to renames and moves.

This project is fully maintained by an LLM.

## Web Application

The `gooru serve` command runs the REST API, authenticated media backend, upload
import endpoint, job polling routes, and static SvelteKit frontend. See
[docs/SERVE.md](docs/SERVE.md) for local setup, production static-asset
deployment, secure network access, upload configuration, and optional media tool
dependencies. Every supported YAML option is documented in the
[serve configuration reference](docs/CONFIG.md).

## NixOS

The repository flake provides both the packaged application and a NixOS module.
The package builds the static frontend and installs it beside the Go binary, so a
Node runtime is not required on the server. The module creates a `gooru` system
user by default, stores application state under `/var/lib/gooru`, media cache
under `/var/cache/gooru`, installs `ffmpeg`/`ffprobe` in the service PATH, and
writes the generated serve configuration to `/etc/gooru/serve.yaml`.

Add Gooru as a flake input and import its module:

```nix
{
  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    gooru.url = "github:fiso64/gooru";
  };

  outputs = { nixpkgs, gooru, ... }: {
    nixosConfigurations.my-host = nixpkgs.lib.nixosSystem {
      system = "x86_64-linux";
      modules = [
        gooru.nixosModules.default
        {
          services.gooru = {
            enable = true;
            settings = {
              # The default is 127.0.0.1:5678. Bind more broadly only when
              # authenticated access on your network is intentional.
              server.listen = "127.0.0.1:5678";

              uploads = {
                enabled = true;
                targets = [
                  {
                    id = "default";
                    name = "Default";
                    path = "/srv/gooru/incoming";
                  }
                ];
              };
            };
          };
        }
      ];
    };
  };
}
```

Create any configured upload directories with permissions appropriate for the
service user. After rebuilding the system, create the first admin account with:

```bash
sudo -u gooru gooru user create-admin --username alice --config /etc/gooru/serve.yaml
```

For non-interactive provisioning, the same command accepts the password through
the `GOORU_ADMIN_PASSWORD` environment variable. `services.gooru.settings` maps
directly to the normal serve YAML configuration, so options documented in
[docs/CONFIG.md](docs/CONFIG.md) can be expressed as Nix attributes. Set
`services.gooru.openFirewall = true` only when the configured listen port should
be reachable through the NixOS firewall.

The package can also be built or run without enabling the service:

```bash
nix build .#
nix run .# -- --help
```

## TODO

- Priority #1: Extensive tests to ensure correctness of all operations.
- Test database performance on a large and a huge db. (potential optimization: FTS)

### 1. Saved Query Aliases

Allow users to save complex query expressions under a simple, memorable alias. This alias could then be used in any command that accepts an expression.

*   **Use Case:** A photographer frequently searches for all media files that are not yet part of an archived project. Instead of typing `gooru list "(type:img | type:vid) -project:archive"` repeatedly, they could save it as an alias: `gooru query save unsorted "(type:img | type:vid) -project:archive"`. From then on, they can simply run `gooru list @unsorted` or `gooru tag @unsorted needs_review`.

### 2. Directory-based Tag Inheritance

Automatically apply a common set of tags to any file being added or tagged within a specific directory tree. This could be configured by placing a special file (e.g., `.gooru-tags`) in a directory, or in a global config file.

*   **Use Case:** A user organizes their work by client and project (e.g., `/work/client-a/project-x/`). They place a `.gooru-tags` file in `/work/client-a/` containing `client:a`. Now, any file they add from within that directory or its subdirectories, like `gooru add /work/client-a/project-x/brief.pdf`, will automatically be tagged with `client:a`, reducing manual effort and ensuring consistency.
*   Question/Problem: Implemented as above, a new file in a directory with associated auto-tags will not automatically obtain these tags until user `add`s it. Confusing/unintuitive?
