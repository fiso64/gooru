# Gooru

Gooru is a local-first file organizer that tracks files by **content**, not just by path. Tag a file once, then keep finding it even after it is renamed or moved.

Gooru includes:

- a fast CLI for indexing, tagging, querying, and repairing a file library;
- a self-hosted web app for browsing, previewing, tagging, and uploading media;
- content-based identity using BLAKE3 hashing;
- expressive tag queries with AND, OR, NOT, grouping, namespaces, file types, and filename matching;
- optional encrypted storage for the database, managed uploads, and generated media derivatives;
- a Nix package and NixOS service module.

> Gooru is local-first: the library is SQLite-backed and the server defaults to `127.0.0.1:5678`.

## Quick start: CLI

### 1. Build Gooru

Gooru currently targets Go 1.25.

```bash
go build -o ./bin/gooru ./cmd/gooru
```

Or with Nix:

```bash
nix build .#
nix run .# -- --help
```

### 2. Initialize a library

```bash
./bin/gooru init
```

Initialization creates the SQLite database and asks you to choose a hashing strategy:

- **Partial hashing** — recommended for large media libraries; faster, with a theoretical collision risk.
- **Full hashing** — hashes the complete file; slower on large files, but maximizes identity assurance.

The choice is stored with the database.

### 3. Tag some files

You do not need to add files separately before tagging them.

```bash
./bin/gooru tag ~/Pictures/trip/*.jpg photo trip:iceland
./bin/gooru tag ~/Videos/clip.mp4 video trip:iceland
```

Use `add` when you want to track files without assigning tags:

```bash
./bin/gooru add ~/Documents/archive
```

### 4. Search

```bash
./bin/gooru list 'photo trip:iceland'
./bin/gooru table 'photo -favorite'
./bin/gooru count 'type:vid | ext:gif'
./bin/gooru exists '@tagged'
```

Queries support implicit AND, `|` for OR, `-` for NOT, parentheses, namespaced tags such as `trip:iceland`, virtual file types, and meta-queries. See [docs/QUERY.md](docs/QUERY.md).

## Common CLI workflows

| Goal | Command |
| --- | --- |
| Track files without tagging | `gooru add <path...>` |
| Add tags | `gooru tag <source> <tag...>` |
| Replace all tags | `gooru settags <source> [tag...]` |
| Remove tags | `gooru untag <source> [tag...]` |
| List matching files | `gooru list [query]` |
| Show files and tags | `gooru table [query]` |
| Count matches | `gooru count [query]` |
| Test whether a match exists | `gooru exists [query]` |
| List known tags | `gooru listtags --count` |
| Repair moved/renamed locations | `gooru relinkall <dir...>` |
| Re-identify intentionally modified files | `gooru rehash <path...>` |
| Remove tracked locations | `gooru delete <path...>` |
| Run the web app/API | `gooru serve --config serve.yaml` |

See [docs/CLI.md](docs/CLI.md) for command behavior and safety notes.

## Web app

The web app is a static SvelteKit frontend served by the same Go process as the REST API and media endpoints.

From a source checkout:

```bash
cd frontend
npm ci
npm run build
cd ..

./bin/gooru serve --print-default-config > serve.yaml
./bin/gooru user create-admin --username alice --config serve.yaml
./bin/gooru serve --config serve.yaml
```

Then open the configured server address. The default is `http://127.0.0.1:5678`.

Authentication is enabled by default. For uploads, encryption, network exposure, reverse-proxy guidance, and optional media tooling, see [docs/SERVE.md](docs/SERVE.md). Every YAML field is listed in [docs/CONFIG.md](docs/CONFIG.md).

## How file identity works

Paths are locations; content is identity.

When Gooru sees a file, it records a content hash and associates tags with that content. If the same content appears at a new path, Gooru can recognize it as a move, rename, or duplicate instead of treating it as an unrelated file.

Path-based operations hash content by default for correctness. Commands that expose `--use-metadata` can opt into a faster size-and-modification-time shortcut when that trade-off is acceptable.

For library repair and move detection, see [docs/CLI.md#filesystem-changes](docs/CLI.md#filesystem-changes).

## Query examples

```text
photo trip:iceland       # AND
photo | video            # OR
photo -work              # NOT
(photo | video) favorite # grouping
ext:jpg                   # extension
type:img                  # virtual image type
@tagged                   # has at least one tag
-@tagged                  # has no tags
@filename_contains:scan   # filename/path display match
location:*                # any non-empty value for the location key
```

See [docs/QUERY.md](docs/QUERY.md) for exact semantics, especially the difference between `location`, `location:`, and `location:*`.

## NixOS

The flake exports the package and a NixOS module. A minimal service configuration looks like this:

```nix
{
  inputs.gooru.url = "github:fiso64/gooru";

  outputs = { nixpkgs, gooru, ... }: {
    nixosConfigurations.my-host = nixpkgs.lib.nixosSystem {
      system = "x86_64-linux";
      modules = [
        gooru.nixosModules.default
        {
          services.gooru = {
            enable = true;
            settings.server.listen = "127.0.0.1:5678";
          };
        }
      ];
    };
  };
}
```

The module stores application state under `/var/lib/gooru`, media cache under `/var/cache/gooru`, and writes the generated server config to `/etc/gooru/serve.yaml`.

Create the first administrator after rebuilding:

```bash
sudo -u gooru gooru user create-admin --username alice --config /etc/gooru/serve.yaml
```

For upload targets and the complete option mapping, see [docs/SERVE.md](docs/SERVE.md#nixos) and [docs/CONFIG.md](docs/CONFIG.md).

## Documentation

- [Getting around the CLI](docs/CLI.md)
- [Query language](docs/QUERY.md)
- [Serving the web app](docs/SERVE.md)
- [Server configuration reference](docs/CONFIG.md)
- [HTTP API notes](docs/API.md) and [OpenAPI specification](docs/openapi.yaml)
- [Using the Go package](docs/LIBRARY.md)
- [Development](docs/DEVELOPMENT.md)

## Project status

Gooru is under active development. Treat the CLI, configuration schema, and HTTP API as evolving unless a release explicitly documents compatibility guarantees.

## License

Gooru is licensed under **AGPL-3.0-only**. See [LICENSE](LICENSE) for the license text and [THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md) for third-party components and assets.
