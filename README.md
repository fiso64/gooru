# Gooru

Gooru is a media library for browsing, tagging, uploading, and viewing files through a web interface inspired by boorus. The CLI can also be used as a standalone tagging tool and does not require a running server.

Files are identified by content rather than only by path. This allows Gooru to recognize the same content after a rename or move, or even function as an archive capable of checking file integrity when using [full hashing](docs/HASHING.md).

Gooru is intended primarily for personal and private multi-user libraries rather than public imageboard-style communities.

  <img width="700" alt="Gooru demo showing the media library interface" src="https://github.com/user-attachments/assets/cf8a4feb-c508-474a-9b53-03e1bd077151" />



## Features

- Webui library with search, tags, saved searches, and bulk actions.
- View supported media, including images, videos, gifs, and `.cbz` comic archives. 
- Fast file uploads to configurable upload target dirs.
- Content-based file identity that survives renames and moves.
- Optional encryption for the database and uploads.
- Many UI styling options.
- An ergonomic CLI.

## Project status

Under development. Back up your data!

This project is fully maintained by an LLM (aka vibecoded); don't be surprised if there are rough edges. I only request features, test, and report bugs. Of course, if you decide to contribute with a bug report or a PR, I will look at it and ensure the bug is fixed/the feature is working correctly before replying myself (you will **never** get an automated response). 

My unironic proudest contribution is the name Gooru, which sounds like the word "guru", and is also for a booru written in go.

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

## More pictures

| Media viewer | Booru-style theme |
| --- | --- |
| <img width="800" alt="Gooru library view" src="https://github.com/user-attachments/assets/68a9f449-29e4-48d1-8f17-122825d6f467" /> | <img width="800" alt="Gooru media viewer" src="https://github.com/user-attachments/assets/d2501af1-4dff-4d76-9e6e-a1b14efbd8b8" /> |
| *It should be pretty fast. Supports fullscreen and various fit and scaling modes.* | *Stolen from Danbooru. A different vibe for your library of slop.* |

## License

Gooru is licensed under **AGPL-3.0-only**. See [LICENSE](LICENSE) for the license text and [THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md) for third-party components and assets.
