# Server configuration reference

This is the exhaustive reference for `gooru serve` YAML. For a deployment walkthrough, start with [SERVE.md](SERVE.md) instead.

Generate the defaults for the exact binary you are running:

```bash
gooru serve --print-default-config
```

Unknown YAML fields are rejected, so misspelled options fail fast. Paths documented as absolute are validated as such.

The safest starting point is the default configuration: loopback-only listening, authentication enabled, uploads enabled with a private default destination, and encryption disabled until a key is explicitly configured.

## `server`

| Option | Default | Description |
| --- | --- | --- |
| `server.listen` | `127.0.0.1:5678` | HTTP listen address in `host:port` form. |
| `server.public_url` | empty | Optional externally visible base URL. Can also be overridden with `--public-url`. |
| `server.cors_origins` | empty list | Origins allowed by the server's CORS policy. Leave empty for same-origin browser use. |
| `server.expose_paths` | `false` | Include absolute filesystem paths in API file responses. Keep disabled unless clients genuinely need them. |
| `server.frontend_dir` | `frontend/build` | Directory containing the built static frontend. |
| `server.max_request_body_bytes` | `0` | Optional deployment-wide request body guard. `0` disables the global guard; negative values are invalid. Metadata JSON endpoints enforce their own bounded request sizes, while uploads are governed separately by `uploads.max_file_size_bytes`. |

Server read/write/idle timeouts are internal defaults and are not YAML options. The request read timeout is enforced as an inactivity limit after headers are accepted, so a steadily progressing large upload is not rejected merely for taking longer than the default timeout.

## `database`

| Option | Default | Description |
| --- | --- | --- |
| `database.path` | the normal Gooru DB path | SQLite database used by the library, users, and sessions. Must not be empty. Can be overridden for `serve` with `--database`. Using a different path is the supported way to run an independent database-backed instance. |

The default database path is normally `~/.config/gooru/gooru.db`. `gooru serve --config instance.yaml --database /absolute/path/instance.db` can therefore run against a separate database without changing global state.

## `encryption`

| Option | Default | Description |
| --- | --- | --- |
| `encryption.enabled` | `false` | Enable protected storage for the SQLite database and Gooru-managed upload contents. Requires exactly one supported key source. |
| `encryption.key_file` | empty | Path to a regular file containing a base64-encoded 256-bit key. Mutually exclusive with the environment key sources. |
| `encryption.opaque_url_state` | `true` | In protected mode, keep library query/sort/preview state out of browser-visible URLs by using authenticated opaque state tokens. |

When enabled, configure exactly one of `encryption.key_file`, `GOORU_ENCRYPTION_KEY`, or `GOORU_ENCRYPTION_KEY_FILE`. See [ENCRYPTION.md](ENCRYPTION.md) for setup and key generation, migration/cache behavior, the security boundary, browser limitations, recovery-critical key handling, and the supported disable/re-key lifecycle.

## `auth`

| Option | Default | Description |
| --- | --- | --- |
| `auth.enabled` | `true` | Enable DB-backed login and server-side sessions. |
| `auth.session_ttl` | `720h` | Session lifetime as a Go duration. Must be positive. |
| `auth.cookie_name` | `gooru_session` | Name of the session cookie. Must not be empty. |
| `auth.cookie_secure` | `auto` | Secure-cookie policy: `auto`, `true`, or `false`. |
| `auth.cookie_same_site` | `lax` | SameSite policy: `lax`, `strict`, or `none`. |
| `auth.allow_unsafe_no_auth_non_loopback` | `false` | Explicit escape hatch allowing `auth.enabled: false` while listening on a non-loopback address. Intended only for trusted development environments. |

For safety, unauthenticated serving on a non-loopback address is rejected unless `auth.allow_unsafe_no_auth_non_loopback` is explicitly enabled.

The obsolete `auth.token`, `auth.token_env`, and `auth.token_file` options are rejected. Create DB-backed users with `gooru user create-admin` instead.

## `uploads`

| Option | Default | Description |
| --- | --- | --- |
| `uploads.enabled` | `true` when authentication is enabled; `false` otherwise | Browser/API uploads require at least one valid target. Explicit `false` disables uploads, even if targets are configured. An unauthenticated deployment must explicitly opt in, including on loopback. |
| `uploads.targets` | one automatically provisioned private target when enabled and omitted | Allowed upload destinations. Each target has `id`, `name`, `path`, and optional `added_at_strategy`. Explicitly setting `targets: []` prevents automatic target creation; if uploads are explicitly enabled at the same time, startup rejects the missing target. |
| `uploads.max_file_size_bytes` | `0` | Optional upload per-file size setting. A zero value leaves the upload-specific size limit unset; set this explicitly when deployments need a hard upload cap. The generic `server.max_request_body_bytes` limit does not cap `/uploads`. |
| `uploads.preserve_modtime` | `true` | Preserve each browser-uploaded file's source modification timestamp on the stored destination. Source timestamps are still carried through upload processing when disabled. |

When uploads are enabled and `uploads.targets` is omitted, Gooru prepares one private persistent `default` target under the current user's application data: `$XDG_DATA_HOME/gooru/uploads` (or `~/.local/share/gooru/uploads`) on Linux, `~/Library/Application Support/Gooru/uploads` on macOS, and `%LOCALAPPDATA%\\Gooru\\uploads` on Windows. A managed Linux/systemd instance uses its private state directory's `uploads` subdirectory (for example, `/var/lib/gooru-main/uploads`). A configured target list always takes precedence. Gooru fails startup if it cannot safely create and write the default target; it does not select a temporary fallback. Keep any explicitly configured targets outside application-owned paths.\n\nUpload throughput concurrency is not currently configurable through YAML or a `serve` flag. The upload worker keeps mutation/protected-storage transitions serialized for correctness, while the safe per-file analysis/read/hash/status stage uses an internal bounded pool of four workers.

Each entry in `uploads.targets` supports `id`, `name`, `path`, and optional `added_at_strategy`. The strategy defaults to `queue` and accepts `queue`, `reverse_queue`, or `modtime`.

| Field | Description |
| --- | --- |
| `id` | Stable client-facing identifier. Required, unique, and limited to letters, numbers, `_`, and `-`; the first character must be alphanumeric. |
| `name` | Human-readable target name. Required. |
| `path` | Absolute destination directory. Required. The path itself is not returned by the upload-target API. |
| `added_at_strategy` | Optional default for library-added ordering: `queue` (default), `reverse_queue`, or `modtime`. An upload request may override the target default. |
| `default_tags` | Optional list of tags used to pre-populate the Upload tab when this destination is selected. Defaults are additive tags only; removal/exclusion syntax is invalid. These values remain visible and editable before upload. |

Upload targets must not overlap Gooru-owned application paths. Startup rejects a target that contains, is contained by, or resolves through symlinks onto the configured database, encryption key file, media cache, frontend directory, or an explicitly configured absolute ffmpeg/ffprobe executable path. Keep application state and executables outside directories that Gooru is allowed to upload into or delete from.

Example:

```yaml
uploads:
  enabled: true
  targets:
    - id: default
      name: Default
      path: /srv/gooru/incoming
      added_at_strategy: queue
      default_tags:
        - project:inbox
        - source:upload
  max_file_size_bytes: 104857600
  preserve_modtime: true
```

## `media`

| Option | Default | Description |
| --- | --- | --- |
| `media.cache_dir` | empty | Optional absolute derivative cache directory. When empty, the server chooses its normal cache location. |
| `media.thumbnail_sizes` | `[256, 512]` | Thumbnail dimensions to generate/cache. The list must be non-empty; each value must be between 1 and 4096. |
| `media.thumbnail_format` | `jpeg` | Thumbnail output format: `jpeg` or `png`. |
| `media.preview_size` | `1280` | Requested long-edge size for image previews. Must be greater than zero. |
| `media.preview_enabled` | `true` | Generate and serve derived viewer previews. When disabled, preview requests fall back to original media and the WebUI treats original media as the only viewer source. Grid thumbnails remain enabled. |
| `media.preview_jpeg_quality` | `92` | JPEG quality for generated viewer previews, from `1` to `100`. This does not change grid-thumbnail JPEG quality. |


## `tools`

| Option | Default | Description |
| --- | --- | --- |
| `tools.ffmpeg_path` | `ffmpeg` | Executable/path used for ffmpeg-backed media processing. |
| `tools.ffprobe_path` | `ffprobe` | Executable/path used for ffprobe-backed media probing. |

Tool paths may be executable names resolved through `PATH` or explicit paths appropriate for the host.

## `ui`

| Option | Default | Description |
| --- | --- | --- |
| `ui.theme` | `default` | WebUI presentation: `default` keeps the standard Gooru interface, `booru-light` uses the booru-oriented light shell/viewer presentation, and `booru-dark` uses the same shared booru structure with the reference-derived dark palette. Booru themes keep their reference UI palette; `ui.accent_color` affects only Gooru spiral branding/favicon there, while an explicitly configured `ui.font_style` can override the native booru typography. |
| `ui.accent_color` | empty | Optional runtime accent in six-digit hex form such as `#2f80ed`. When empty, Gooru spiral branding/favicon use the built-in yellow. In the default theme this continues to drive the broader UI accent and readable foreground tokens. In booru themes it changes only the Gooru spiral branding and favicon; links, buttons, focus colors, and the rest of the booru palette keep the theme's fixed reference colors. |
| `ui.font_style` | `comic`; native booru typography when omitted with a booru theme | Typography preset: `editorial` uses the serif display face with the sans-serif UI face, `modern` uses the sans-serif UI face for display text too, and `comic` uses a Comic Sans-style stack for most UI/display text and the Gooru wordmark while retaining the mono face for code/data. The default theme continues to use `comic` by default. A booru theme preserves its native Tahoma/Verdana typography when this field is omitted; if the field is explicitly present, the selected preset is applied to the booru UI. |
| `ui.grid_size` | `200`; `180` when omitted with a booru theme | Base gallery cell size in pixels. Must be between `64` and `1024`. `square` uses this value directly; `fit` and `tile` receive a fixed 40px layout boost. Explicit values apply to every theme. |
| `ui.grid_type` | `square`; `fit` when omitted with a booru theme | Gallery layout: `square` keeps the existing cropped square grid, `fit` keeps square cells but contains the whole image with transparent surrounding space, and `tile` uses justified non-square aspect-preserving rows. An explicitly configured value always overrides the theme default. All modes keep a bounded virtual DOM for large libraries. |
| `ui.pagination_mode` | `infinite`; `paged` when omitted with a booru theme | Library browsing mode: `infinite` incrementally appends results while scrolling; `paged` keeps only the current transport page in browser query state and shows Previous/Next controls. An explicit server value overrides the theme default, and the browser-local pagination preference remains the final per-browser override. |
| `ui.items_per_page` | `60` | Number of files requested per library page. Used by both modes as the transport page size; must be between `1` and `200`. In paged mode this is the visible page size. |
| `ui.hidden_tags` | empty list | Tags whose files the WebUI excludes by default from library and filtered file views. If a query positively requests a hidden tag, that tag's default exclusion is lifted for the request while other hidden tags remain excluded. Library/result counts and kind facets follow the same visibility policy. Hidden tags remain available in tag browsing and search completions. This affects WebUI/API browsing only; it does not change core or CLI query semantics. |
| `ui.viewer_fit_mode` | `fit_window` | Default viewer fit policy: `fit_window` (legacy alias `screen`) fills the available viewer bounds, `fit_down_only` never enlarges smaller media, `original_size_if_fit` keeps media at 1:1 when it fits and otherwise scales down, and `actual` starts from 1:1 semantics. Press `V` to cycle the fit policies for the current browser session. |
| `ui.viewer_actual_size_fit_cap` | `true` | When enabled, `actual`/1:1 mode starts no larger than fit-window size for oversized media. Manual zoom remains available above that fitted baseline. Disable this to preserve an uncapped intrinsic 1:1 starting size. |
| `ui.viewer_scaling` | `smooth` | Browser-side image interpolation: `smooth` uses normal browser filtering and `nearest` uses nearest-neighbor/pixelated scaling. Press `S` in the viewer to toggle it for the current browser session. |
| `ui.load_full_media_by_default` | `false` | Start image viewers on the original/full media instead of the derived preview when an original is available. The viewer button remains available to switch back to the preview for the current viewer session. |
| `ui.fullscreen_media_by_default` | `false` | Request browser fullscreen for the media viewer whenever a file is opened. Browsers may deny fullscreen when the opening interaction does not provide user activation; the viewer remains usable normally in that case. |
| `ui.hover_play_videos` | `false` | Play muted, inline, looping video previews after the grid hover dwell. Playback stops when the pointer leaves, is limited to near-viewport media, and is disabled when the browser requests reduced motion. |
| `ui.hover_play_gifs` | `true` | Play animated GIF previews after the grid hover dwell. Playback stops when the pointer leaves, is limited to near-viewport media, and is disabled when the browser requests reduced motion. |

For a booru theme, omit `ui.font_style` to keep its native Tahoma/Verdana typography. Including `font_style: comic`, `font_style: modern`, or `font_style: editorial` is an explicit override.

## `logging`

| Option | Default | Description |
| --- | --- | --- |
| `logging.level` | `info` | Server logging threshold. Supported levels are `debug`, `info`, `warn`, and `error`. Logs are written to stderr. |

Request debug logging is intentionally privacy-conscious: it records matched route patterns and request lifecycle information rather than raw URL paths/query strings, request bodies, headers, or filenames.

## Complete example

```yaml
server:
  listen: 127.0.0.1:5678
  public_url: ""
  cors_origins: []
  expose_paths: false
  frontend_dir: frontend/build
  max_request_body_bytes: 0

database:
  path: /srv/gooru/gooru.db

encryption:
  enabled: false
  key_file: ""
  opaque_url_state: true

auth:
  enabled: true
  session_ttl: 720h
  cookie_name: gooru_session
  cookie_secure: auto
  cookie_same_site: lax
  allow_unsafe_no_auth_non_loopback: false

uploads:
  enabled: true
  targets:
    - id: default
      name: Default
      path: /srv/gooru/incoming
      added_at_strategy: queue
  max_file_size_bytes: 104857600
  preserve_modtime: true
media:
  cache_dir: /srv/gooru/cache/media
  thumbnail_sizes: [256, 512]
  thumbnail_format: jpeg
  preview_size: 1280
  preview_enabled: true
  preview_jpeg_quality: 92


tools:
  ffmpeg_path: ffmpeg
  ffprobe_path: ffprobe

logging:
  level: info

ui:
  theme: default
  accent_color: "#2f80ed"
  font_style: comic
  grid_size: 200
  grid_type: square
  pagination_mode: infinite
  items_per_page: 60
  hidden_tags: []
  viewer_fit_mode: fit_window
  viewer_actual_size_fit_cap: true
  viewer_scaling: smooth
  load_full_media_by_default: false
  fullscreen_media_by_default: false
  hover_play_videos: false
  hover_play_gifs: true
```

For deployment and API behavior, see [SERVE.md](SERVE.md). The generated default remains the quickest way to verify defaults for the exact binary being run:

```bash
gooru serve --print-default-config
```
