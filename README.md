# Gooru

Gooru is a media library for browsing, tagging, uploading, and viewing files through a web interface inspired by boorus. The CLI can also be used as a standalone tagging tool and does not require a running server.

Files are identified by content rather than only by path. This allows Gooru to recognize the same content after a rename or move, or even function as an archive capable of checking file integrity when using [full hashing](docs/HASHING.md).

Gooru is intended primarily for personal and private multi-user libraries rather than public imageboard-style communities.

This project is fully maintained by an LLM.

## Features

- Browser-based library with search, tags, tag namespaces, saved searches, and bulk actions.
- Thumbnails and in-browser previews for supported media, including images, videos, gifs, and basic support for `.cbz` comic archives. 
- Uploads and imports with background job tracking.
- Content-based file identity that survives renames and moves.
- Optional encryption for the database, managed uploads, and generated media.
- HTTP API, CLI, and Go packages for automation and integration. 

## Getting started

See [Running Gooru](docs/SERVE.md) for building the application, creating the first user, configuring the server, and deployment options.

The complete server configuration is documented in [docs/CONFIG.md](docs/CONFIG.md).

## Documentation

| Topic | Documentation |
| --- | --- |
| Run and deploy Gooru | [docs/SERVE.md](docs/SERVE.md) |
| Server configuration | [docs/CONFIG.md](docs/CONFIG.md) |
| Command-line interface | [docs/CLI.md](docs/CLI.md) |
| Development | [docs/DEVELOPMENT.md](docs/DEVELOPMENT.md) |
| Frontend development | [frontend/README.md](frontend/README.md) |

[Read the full documentation](docs)

## Project status

Gooru is under active development. Treat the CLI, configuration schema, and HTTP API as evolving unless a release explicitly documents compatibility guarantees.

## License

Gooru is licensed under **AGPL-3.0-only**. See [LICENSE](LICENSE) for the license text and [THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md) for third-party components and assets.
