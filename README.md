# Gooru

Gooru is a media library for browsing, tagging, uploading, and viewing files through a web interface inspired by boorus. The CLI can also be used as a standalone tagging tool and does not require a running server.

Your files can remain at their original locations; they do not have to be copied/imported into a Gooru-managed library in order to be tagged (but can be). Tagged items are identified by content rather than only by path, which allows Gooru to preserve tags after renames and moves.

Gooru is intended primarily for personal and private multi-user libraries rather than public imageboard communities.

  <img width="700" alt="Gooru demo showing the media library interface" src="https://github.com/user-attachments/assets/cf8a4feb-c508-474a-9b53-03e1bd077151" />  

  
It's a booru written in Go, hence "Gooru", but the name should be pronounced like the word guru. I thought it's clever.

## Features

- Keyboard-friendly webui with search, tags, saved searches, bulk actions, and a good viewer.
- View supported media, including images, videos, gifs, and cbz comic archives. 
- Tag external files without duplicating, or upload to configurable target dirs.
- Hash-based file identity that survives external renames and moves.
- Optional encryption for the database and uploads.
- An ergonomic CLI.

## Getting started

Get an appropriate release for your system from the [releases page](https://github.com/fiso64/gooru/releases/latest). On NixOS, use the [NixOS module](docs/SERVE.md#nixos-deployment).

See [running and deploying Gooru](docs/SERVE.md) for installation and first-run instructions. You can also [build from source](docs/DEVELOPMENT.md#build-from-source).

## Gallery

<table>
  <tr>
    <th width="50%">Media viewer</th>
    <th width="50%">Booru-style theme</th>
  </tr>
  <tr>
    <td width="50%">
      <img width="800" alt="Gooru library view"
           src="https://github.com/user-attachments/assets/68a9f449-29e4-48d1-8f17-122825d6f467" />
    </td>
    <td width="50%">
      <img width="800" alt="Gooru media viewer"
           src="https://github.com/user-attachments/assets/d2501af1-4dff-4d76-9e6e-a1b14efbd8b8" />
    </td>
  </tr>
  <tr>
    <td width="50%">
      <em>The viewer supports prev/next navigation,
      fullscreen, and various fit and scaling modes.</em>
    </td>
    <td width="50%">
      <em>There is an alternative theme (definitely not stolen from Danbooru), if you want a more classic vibe for your library of slop.</em>
    </td>
  </tr>
</table>
<table>
  <tr>
    <th width="50%">Uploads</th>
    <th width="50%">Grid styles</th>
  </tr>
  <tr>
    <td width="50%">
      <img width="800" alt="Gooru uploads tab"
           src="https://github.com/user-attachments/assets/0fe9e828-0751-40f4-80bd-37cf0d0a5dd6" />
    </td>
    <td width="50%">
      <img width="800" alt="Grid styles in Gooru"
           src="https://github.com/user-attachments/assets/8921ced7-7419-4468-94ef-46fc8f321ab8" />
    </td>
  </tr>
  <tr>
    <td width="50%">
      <em>Drop or paste files anywhere to stage, then tag them or preview
      in the viewer before uploading. As you can see, uploads can also be
      sorted by their file modtime, which is useful for preserving order when importing existing content.</em>
    </td>
    <td width="50%">
      <em>The library grid can be paged or scroll infinitely. There are also three layout modes: Square (seen in the main demo image), fit (as above in the booru theme demo; configurable independently of the theme), and dynamic tile mode shown here.</em>
    </td>
  </tr>
</table>

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

## License

Gooru is licensed under **AGPL-3.0-only**.
