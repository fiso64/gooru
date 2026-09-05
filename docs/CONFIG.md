# Server configuration reference

This is the exhaustive reference for `gooru serve` YAML. For a deployment walkthrough, start with [SERVE.md](SERVE.md) instead.

Generate the defaults for the exact binary you are running:

```bash
gooru serve --print-default-config
```

Unknown YAML fields are rejected, so misspelled options fail fast. Paths documented as absolute are validated as such.

The safest starting point is the default configuration: loopback-only listening, authentication enabled, uploads disabled, and encryption disabled until a key is explicitly configured.

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
| `encryption.enabled` | `false` | Enable protected storage for the SQLite database, managed uploads, and generated media derivatives. Requires exactly one encryption-key source described below. |
| `encryption.key_file` | empty | Path to a regular file containing a base64-encoded 256-bit key. On non-Windows systems the file must not be readable or writable by group or others. Mutually exclusive with the environment key sources below. |

Encryption key **material** is never stored directly in YAML. When `encryption.enabled: true`, configure exactly one source: `encryption.key_file`, `GOORU_ENCRYPTION_KEY`, or `GOORU_ENCRYPTION_KEY_FILE`.

| Environment variable | Value |
| --- | --- |
| `GOORU_ENCRYPTION_KEY` | Base64 encoding of exactly 32 random bytes (256 bits). |
| `GOORU_ENCRYPTION_KEY_FILE` | Path to a regular file containing that same base64-encoded 32-byte key. On non-Windows systems the file must not be readable or writable by group or others. |

For example, generate a key once and store it in an owner-only file:

```bash
umask 077
openssl rand -base64 32 > /srv/gooru/encryption.key
```

Then choose one configuration source. To use the environment-variable path source:

```bash
export GOORU_ENCRYPTION_KEY_FILE=/srv/gooru/encryption.key
go run ./cmd/gooru serve --config serve.yaml
```

with:

```yaml
encryption:
  enabled: true
```

Alternatively, do not set either encryption-key environment variable and put only the key-file path in YAML:

```yaml
encryption:
  enabled: true
  key_file: /srv/gooru/encryption.key
```

This also composes directly with declarative secret managers. For example, a NixOS module configuration using agenix can pass the generated secret path without copying key material into the Nix store:

```nix
services.gooru.settings.encryption = {
  enabled = true;
  key_file = config.age.secrets."gooru-encryption-key".path;
};
```

Keep the key backed up separately from the encrypted data. Starting protected mode without a valid key fails closed; using a different key cannot decrypt data encrypted with the original key.

When protected mode starts, Gooru migrates its database and files registered under configured `uploads.targets` to encrypted storage before serving requests. Indexed media outside those managed upload roots is deliberately left unchanged because Gooru does not rewrite arbitrary external library files. New managed uploads and generated derivatives are encrypted while protected mode is enabled. Media responses that contain decrypted protected content use no-store cache policy to reduce plaintext traces in client/proxy caches.


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
| `uploads.enabled` | `false` | Enable browser/API uploads. Enabling uploads requires at least one valid target. |
| `uploads.targets` | empty list | Allowed upload destinations. Each target has `id`, `name`, `path`, and optional `added_at_strategy`. |
| `uploads.max_file_size_bytes` | `0` | Optional upload per-file size setting. A zero value leaves the upload-specific size limit unset; set this explicitly when deployments need a hard upload cap. The generic `server.max_request_body_bytes` limit does not cap `/uploads`. |
| `uploads.preserve_modtime` | `true` | Preserve each browser-uploaded file's source modification timestamp on the stored destination. Source timestamps are still carried through upload processing when disabled. |
| `uploads.conflict_policy` | `rename` | Default same-name behavior: `skip`, `rename`, `replace`, or `error`. |

Each entry in `uploads.targets` supports `id`, `name`, `path`, and optional `added_at_strategy`. The strategy defaults to `queue` and accepts `queue`, `reverse_queue`, or `modtime`.


| Field | Description |
| --- | --- |
| `id` | Stable client-facing identifier. Required, unique, and limited to letters, numbers, `_`, and `-`; the first character must be alphanumeric. |
| `name` | Human-readable target name. Required. |
| `path` | Absolute destination directory. Required. The path itself is not returned by the upload-target API. |
| `added_at_strategy` | Optional default for library-added ordering: `queue` (default), `reverse_queue`, or `modtime`. An upload request may override the target default. |
| `default_tags` | Optional list of tags and `-tag` exclusions used to pre-populate the Upload tab when this destination is selected. These values remain visible and editable before upload. |

Upload targets must not overlap Gooru-owned application paths. Startup rejects a target that contains, is contained by, or resolves through symlinks onto the configured database, encryption key file, media cache, frontend directory, or an explicitly configured absolute ffmpeg/ffprobe executable path. Keep application state and executables outside directories that Gooru is allowed to upload into, replace within, or delete from.

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
        - -project:archive
  max_file_size_bytes: 104857600
  preserve_modtime: true
  conflict_policy: rename
```

## `media`

| Option | Default | Description |
| --- | --- | --- |
| `media.cache_dir` | empty | Optional absolute derivative cache directory. When empty, the server chooses its normal cache location. |
| `media.thumbnail_sizes` | `[256, 512]` | Thumbnail dimensions to generate/cache. The list must be non-empty; each value must be between 1 and 4096. |
| `media.thumbnail_format` | `jpeg` | Thumbnail output format: `jpeg` or `png`. |
| `media.preview_size` | `1280` | Requested long-edge size for image previews. Must be greater than zero. |

## `jobs`

| Option | Default | Description |
| --- | --- | --- |
| `jobs.completed_ttl` | `1h` | How long completed/failed/canceled in-memory jobs are retained. Go duration; must be positive. |
| `jobs.max_queued` | `100` | Maximum number of pending jobs. Must be positive. |
| `jobs.max_running` | `2` | Maximum number of concurrently running jobs. Must be positive. |
| `jobs.max_result_bytes` | `10485760` (10 MiB) | Maximum result payload retained for a completed in-memory job. Oversized results are omitted from retained job state without changing a successful job to failed; synchronous API calls still receive their immediate result. Must be positive. |

## `tools`

| Option | Default | Description |
| --- | --- | --- |
| `tools.ffmpeg_path` | `ffmpeg` | Executable/path used for ffmpeg-backed media processing. |
| `tools.ffprobe_path` | `ffprobe` | Executable/path used for ffprobe-backed media probing. |

Tool paths may be executable names resolved through `PATH` or explicit paths appropriate for the host.

## `ui`

| Option | Default | Description |
| --- | --- | --- |
| `ui.accent_color` | empty | Optional runtime UI accent in six-digit hex form such as `#2f80ed`. When empty, the built-in yellow accent is used. The UI derives readable foreground and translucent accent tokens from this color. |
| `ui.font_style` | `editorial` | Typography preset: `editorial` keeps the serif display face, `modern` uses the sans-serif UI face for display text too, and `comic` uses a Comic Sans-style stack for most UI/display text and the Gooru wordmark while retaining the mono face for code/data. |
| `ui.grid_size` | `180` | Minimum media-grid cell width in pixels. Must be between `64` and `1024`. The grid remains fluid: cells expand to fill each row rather than becoming fixed-width. |
| `ui.viewer_fit_mode` | `fit_window` | Default viewer fit policy: `fit_window` (legacy alias `screen`) fills the available viewer bounds, `fit_down_only` never enlarges smaller media, `original_size_if_fit` keeps media at 1:1 when it fits and otherwise scales down, and `actual` starts at original size. Press `V` to cycle the fit policies for the current browser session. |
| `ui.viewer_scaling` | `smooth` | Browser-side image interpolation: `smooth` uses normal browser filtering and `nearest` uses nearest-neighbor/pixelated scaling. Press `S` in the viewer to toggle it for the current browser session. |
| `ui.load_full_media_by_default` | `false` | Start image viewers on the original/full media instead of the derived preview when an original is available. The viewer button remains available to switch back to the preview for the current viewer session. |
| `ui.fullscreen_media_by_default` | `false` | Request browser fullscreen for the media viewer whenever a file is opened. Browsers may deny fullscreen when the opening interaction does not provide user activation; the viewer remains usable normally in that case. |

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
  conflict_policy: rename

media:
  cache_dir: /srv/gooru/cache/media
  thumbnail_sizes: [256, 512]
  thumbnail_format: jpeg
  preview_size: 1280

jobs:
  completed_ttl: 1h
  max_queued: 100
  max_running: 2
  max_result_bytes: 10485760

tools:
  ffmpeg_path: ffmpeg
  ffprobe_path: ffprobe

logging:
  level: info

ui:
  accent_color: "#2f80ed"
  font_style: editorial
  grid_size: 180
  viewer_fit_mode: fit_window
  viewer_scaling: smooth
  load_full_media_by_default: false
  fullscreen_media_by_default: false
```

For deployment and API behavior, see [SERVE.md](SERVE.md). The generated default remains the quickest way to verify defaults for the exact binary being run:

```bash
go run ./cmd/gooru serve --print-default-config
```

