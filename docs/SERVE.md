# Serving the Web Application

`gooru serve` runs the REST API, authenticated media routes, upload/import
endpoint, in-memory jobs, and the static SvelteKit frontend from one Go process.

## Build the Frontend

The production server serves static files from `frontend/build` by default. Build
those assets before running `gooru serve` from a source checkout or before
packaging a release:

```bash
cd frontend
npm ci
npm run check
npm run test:unit
npm run build
```

The Go server does not require a Node server at runtime. If you deploy outside a
source checkout, copy `frontend/build` with the `gooru` binary and point
`server.frontend_dir` at that directory.

## Create a Serve Config

Start from the generated default config:

```bash
go run ./cmd/gooru serve --print-default-config > serve.yaml
```

The default listen address is `127.0.0.1:5678`, which is suitable for local use.
Authentication is enabled by default and uses DB-backed users plus opaque
server-side sessions. If uploads are desired, configure at least one absolute
upload directory:

```yaml
server:
  listen: "127.0.0.1:5678"
  frontend_dir: "frontend/build"

database:
  path: "/home/alice/.config/gooru/gooru.db"

auth:
  enabled: true
  session_ttl: "720h"
  cookie_name: "gooru_session"
  cookie_secure: "auto"
  cookie_same_site: "lax"

uploads:
  enabled: true
  directories:
    - name: "default"
      path: "/home/alice/Pictures/incoming"
  max_file_size_bytes: 104857600

media:
  cache_dir: "/home/alice/.cache/gooru/media"
  thumbnail_sizes: [256, 512]
  thumbnail_format: "jpeg"
  preview_size: 1280

jobs:
  completed_ttl: "1h"
  max_queued: 100
  max_running: 2
  max_result_bytes: 10485760

tools:
  ffmpeg_path: "ffmpeg"
  ffprobe_path: "ffprobe"
```

Create the first admin user, then run the server:

```bash
go run ./cmd/gooru user create-admin --username alice --config serve.yaml
go run ./cmd/gooru serve --config serve.yaml
```

For automation, provide the password through the clearly named
`GOORU_ADMIN_PASSWORD` environment variable. Avoid passing passwords as command
arguments.

Login uses `POST /api/v1/auth/login`. The server sets an HttpOnly
`gooru_session` cookie and returns a CSRF token. Mutating cookie-authenticated
requests must send that token in `X-Gooru-CSRF`; `GET /api/v1/auth/me` returns a
fresh token for browser reloads. Original media, thumbnails, and previews are
loaded with same-origin session cookies, so normal `<img>`, `<video>`, and
`<audio>` elements can use range requests without JavaScript blob fetching.

The old `auth.token`, `auth.token_env`, `auth.token_file`, and `--auth-token`
browser auth configuration is rejected with a migration message. Do not store
normal usernames or passwords in YAML config.

## Local Development

For Go/backend development:

```bash
go build ./...
go test ./...
go run ./cmd/gooru serve --config serve.yaml
```

For frontend development, use the Vite dev server from `frontend/` and keep a
`go run ./cmd/gooru serve` process available for API/media routes when testing
against a real library. If you need same-origin frontend-only development, add a
local Vite proxy for that workflow. Production deployments should use
`npm run build` and the Go server, not `npm run dev`.

## Build a Release Binary

For packaged or copied deployments, build a specific binary and run that path
explicitly:

```bash
go build -o ./bin/gooru ./cmd/gooru
./bin/gooru serve --config serve.yaml
```

Using `go run ./cmd/gooru ...` from a checkout or `./bin/gooru ...` from a
freshly built binary avoids accidentally running an older `gooru` from `PATH`.

## Network Access

For local-only use, keep `server.listen` on `127.0.0.1`. To access the app from
another device, bind to a private interface or `0.0.0.0` and keep session auth
enabled:

```yaml
server:
  listen: "0.0.0.0:5678"

auth:
  enabled: true
```

`gooru serve` refuses `auth.enabled: false` on non-loopback binds unless the
intentionally named escape hatch `auth.allow_unsafe_no_auth_non_loopback: true`
is set for trusted throwaway environments.

TLS is intentionally not managed by `gooru serve` in this stack. Use a reverse
proxy, VPN, SSH tunnel, or private network tunnel when exposing the app beyond a
trusted LAN.

## Media Tooling

Default builds use the built-in Go image thumbnail path. To enable libvips as
the primary image thumbnail backend, build with the `govips` tag:

```bash
go build -tags govips -o ./bin/gooru ./cmd/gooru
```

`govips` builds require libvips development files at build time and the libvips
shared library at runtime. Without that tag, image thumbnailing still works for
formats supported by the pure-Go fallback. Video thumbnails use `ffmpeg`;
`ffprobe` is included in thumbnail cache versioning.

Missing optional tools do not prevent browse, tagging, upload, or original media
routes from working. Affected derivative requests return a machine-readable
`unsupported_media` error until the relevant tool is installed or configured.
The built-in pure-Go image fallback uses a quality-preserving scaler for minimal
builds and tests. Video thumbnails prefer a frame shortly after the beginning of
the video, using `ffprobe` duration when available, then fall back to the first
decodable frame.

## Jobs

Long-running mutations use the in-memory job manager. Current limits are:

```yaml
jobs:
  completed_ttl: "1h"
  max_queued: 100
  max_running: 2
  max_result_bytes: 10485760
```

`max_queued` caps pending jobs, `max_running` caps concurrent async job
execution, and `max_result_bytes` prevents large completed results from being
kept in memory. A full queue returns the normal JSON error envelope with
`job_queue_full`.

## Upload Safety

Uploads are disabled unless `uploads.enabled` is true and at least one upload
directory has an absolute path. Uploaded filenames are reduced to safe basenames,
path traversal is rejected by construction, and conflicts are resolved with
numbered suffixes.

Uploads are staged into temporary files in the target directory before they are
atomically linked into their final names. Failed batches remove staged files. If
an async upload/import job is canceled before it starts running, the staged files
are cleaned up when the queued job is drained.

## API Contract Drift

`docs/openapi.yaml` is the source for the public HTTP contract. When server DTOs
or endpoint behavior changes, update the OpenAPI file and the frontend types in
`frontend/src/lib/api/types.ts` in the same change. Go tests parse the OpenAPI
document and assert the key path and schema shapes used by the server/frontend
contract, while frontend checks cover the hand-maintained TypeScript types.
