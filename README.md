# Gooru

Gooru is a media library for browsing, searching, tagging, uploading, and previewing files through a web interface.

Files are identified by content rather than only by path. This lets Gooru recognize the same content after a rename or move, and associate tags with the file's content instead of a particular filename.

This project is fully maintained by an LLM.

## Features

- Browser-based library with search, tags, saved searches, and bulk actions.
- Thumbnails and in-browser previews for supported media.
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
| Search and query syntax | [docs/QUERY.md](docs/QUERY.md) |
| Command-line interface | [docs/CLI.md](docs/CLI.md) |
| HTTP API | [docs/openapi.yaml](docs/openapi.yaml) |
| Go packages | [docs/LIBRARY.md](docs/LIBRARY.md) |
| Development | [docs/DEVELOPMENT.md](docs/DEVELOPMENT.md) |
| Frontend development | [frontend/README.md](frontend/README.md) |

The [documentation index](docs/README.md) provides the same material organized by task.

## Project status

Gooru is under active development. Treat the CLI, configuration schema, and HTTP API as evolving unless a release explicitly documents compatibility guarantees.

## License

Gooru is licensed under **AGPL-3.0-only**. See [LICENSE](LICENSE) for the license text and [THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md) for third-party components and assets.
