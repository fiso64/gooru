# Gooru

Gooru is a media library for browsing, tagging, uploading, and viewing files through a web interface inspired by boorus. The CLI can also be used as a standalone tagging tool and does not require a running server. It can tag external files and does not require copying them to a managed library.

Files are identified by content rather than only by path. This allows Gooru to recognize the same content after a rename or move, or even function as an archive capable of checking file integrity when using [full hashing](docs/HASHING.md).

Gooru is intended primarily for personal and private multi-user libraries rather than public imageboard-style communities.

  <img width="700" alt="Gooru demo showing the media library interface" src="https://github.com/user-attachments/assets/cf8a4feb-c508-474a-9b53-03e1bd077151" />

It's a booru written in Go, hence "Gooru", but should be pronounced like the word guru. I thought it's clever.

## Features

- Keyboard-friendly webui with search, tags, saved searches, bulk actions, and a good viewer.
- View supported media, including images, videos, gifs, and `.cbz` comic archives. 
- Tag external files without duplicating, or upload to configurable target dirs.
- Hash-based file identity that survives external renames and moves.
- Optional encryption for the database and uploads.
- Many UI styling options.
- An ergonomic CLI.

## Getting started

Get an appropriate release for your system from the [releases page](https://github.com/fiso64/gooru/releases/latest). On NixOS, use the [NixOS module](docs/SERVE.md#nixos-deployment).

See [running and deploying Gooru](docs/SERVE.md) for installation and first-run instructions. You can also [build from source](docs/DEVELOPMENT.md#build-from-source).

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

Under development. Back up your data!

## More pictures

| Media viewer | Booru-style theme |
| --- | --- |
| <img width="800" alt="Gooru library view" src="https://github.com/user-attachments/assets/68a9f449-29e4-48d1-8f17-122825d6f467" /> | <img width="800" alt="Gooru media viewer" src="https://github.com/user-attachments/assets/d2501af1-4dff-4d76-9e6e-a1b14efbd8b8" /> |
| *It should be pretty fast. Supports prev/next navigation, fullscreen, and various fit and scaling modes.* | *Stolen from Danbooru. A different vibe for your library of slop.* |

## License

Gooru is licensed under **AGPL-3.0-only**.
