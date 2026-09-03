# Serve configuration reference

`gooru serve` reads YAML with `--config <path>`. Unknown YAML fields are rejected, so a misspelled option fails fast instead of being silently ignored. `gooru serve --print-default-config` prints the defaults for the current build and default database location.

This page documents every supported YAML field in the server configuration. Paths described as absolute are validated as such.

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
| `uploads.targets` | empty list | Allowed upload destinations. Each target has `id`, `name`, and `path`. |
| `uploads.max_file_size_bytes` | `0` | Optional upload per-file size setting. A zero value leaves the upload-specific size limit unset; set this explicitly when deployments need a hard upload cap. The generic `server.max_request_body_bytes` limit does not cap `/uploads`. |
| `uploads.conflict_policy` | `rename` | Default same-name behavior: `skip`, `rename`, `replace`, or `error`. |

Each entry in `uploads.targets` supports:

| Field | Description |
| --- | --- |
| `id` | Stable client-facing identifier. Required, unique, and limited to letters, numbers, `_`, and `-`; the first character must be alphanumeric. |
| `name` | Human-readable target name. Required. |
| `path` | Absolute destination directory. Required. The path itself is not returned by the upload-target API. |

Example:

```yaml
uploads:
  enabled: true
  targets:
    - id: default
      name: Default
      path: /srv/gooru/incoming
  max_file_size_bytes: 104857600
  conflict_policy: rename
```

## `media`

| Option | Default | Description |
| --- | --- | --- |
| `media.cache_dir` | empty | Optional absolute derivative cache directory. When empty, the server chooses its normal cache location. |
| `media.thumbnail_sizes` | `[256, 512]` | Thumbnail dimensions to generate/cache. The list must be non-empty; each value must be between 1 and 4096. |
| `media.thumbnail_format` | `jpeg` | Thumbnail output format: `jpeg` or `png`. |
| `media.preview_size` | `1280` | Requested long-edge size for image previews. Must be greater than zero. |
| `media.load_full_by_default` | `false` | Start image viewers on the original/full media instead of the derived preview when an original is available. The viewer button remains available to switch back to the preview for the current viewer session. |

## `jobs`

| Option | Default | Description |
| --- | --- | --- |
| `jobs.completed_ttl` | `1h` | How long completed/failed/canceled in-memory jobs are retained. Go duration; must be positive. |
| `jobs.max_queued` | `100` | Maximum number of pending jobs. Must be positive. |
| `jobs.max_running` | `2` | Maximum number of concurrently running jobs. Must be positive. |
| `jobs.max_result_bytes` | `10485760` (10 MiB) | Maximum result payload retained for a completed in-memory job. Must be positive. |

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
  max_file_size_bytes: 104857600
  conflict_policy: rename

media:
  cache_dir: /srv/gooru/cache/media
  thumbnail_sizes: [256, 512]
  thumbnail_format: jpeg
  preview_size: 1280
  load_full_by_default: false

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
```

For deployment and API behavior, see [SERVE.md](SERVE.md). The generated default remains the quickest way to verify defaults for the exact binary being run:

```bash
go run ./cmd/gooru serve --print-default-config
```
