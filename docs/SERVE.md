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
For a usable media browser, set an auth token and, if uploads are desired, at
least one absolute upload directory:

```yaml
server:
  listen: "127.0.0.1:5678"
  frontend_dir: "frontend/build"

database:
  path: "/home/alice/.config/gooru/gooru.db"

auth:
  token_env: "GOORU_AUTH_TOKEN"

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

tools:
  ffmpeg_path: "ffmpeg"
  ffprobe_path: "ffprobe"
```

Then run:

```bash
export GOORU_AUTH_TOKEN="$(openssl rand -base64 32)"
go run ./cmd/gooru serve --config serve.yaml
```

Open `http://127.0.0.1:5678/`, paste the token into the auth field, and save it.
The browser stores the token locally and sends it as bearer auth for API calls.
Original media opens use a same-origin cookie scoped to `/api/v1/files/` so
browser navigation can keep range requests for audio/video seeking.

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
another device, bind to a private interface or `0.0.0.0` and configure an auth
token:

```yaml
server:
  listen: "0.0.0.0:5678"

auth:
  token_env: "GOORU_AUTH_TOKEN"
```

`gooru serve` refuses unauthenticated non-loopback binds. The intentionally
named escape hatch `auth.allow_unsafe_no_auth_non_loopback: true` is only for
trusted throwaway environments.

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

## Upload Safety

Uploads are disabled unless `uploads.enabled` is true and at least one upload
directory has an absolute path. Uploaded filenames are reduced to safe basenames,
path traversal is rejected by construction, and conflicts are resolved with
numbered suffixes.
