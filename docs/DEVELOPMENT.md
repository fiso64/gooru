# Development

## Build from source

From the repository root, with Go 1.25 and Node.js 24 installed:

```bash
cd frontend
npm ci
npm run build
cd ..
go build -o gooru ./cmd/gooru
./gooru serve --print-default-config > serve.yaml
```

Run Gooru from the repository root so it finds `frontend/build`. Rebuild the frontend after making changes to it. See [first run](SERVE.md#first-run) to create a library and admin account, or [optional build tags](#optional-build-tags) for libvips support.

## Backend

```bash
go build ./...
go test ./...
```

Run a local server with:

```bash
go run ./cmd/gooru serve --config serve.yaml
```

The module currently declares Go 1.25.

## Frontend

```bash
cd frontend
npm ci
npm run check
npm run test:unit
npm run dev
```

The Vite development server is for frontend work. The production application is a static build served by the Go server.

Run end-to-end tests with:

```bash
npm run test:e2e
```

## Build the production frontend

```bash
cd frontend
npm run build
```

The output is `frontend/build`.

## OpenAPI workflow

`docs/openapi.yaml` is the source of truth for the public HTTP contract.

After changing endpoint shapes:

```bash
cd frontend
npm run generate:api
```

Commit the regenerated `src/lib/api/openapi.ts` together with the OpenAPI change. Go tests also parse the specification to catch drift.

## Logging policy

Server logs are structured text written to stderr. Levels are `debug`, `info`, `warn`, and `error`.

Logging should describe operational events without copying sensitive or library-specific content into logs. In particular, avoid passwords, session/CSRF values, request bodies, raw query strings, concrete media URL paths, filesystem paths, filenames, tags/search expressions, and unbounded error strings that may embed them.

Prefer fixed event names, route patterns, opaque IDs when correlation is necessary, bounded counts, statuses, and durations.

## Optional build tags

### FUSE virtual filesystem

```bash
go build -tags fuse -o gooru ./cmd/gooru
```

Requires the platform's FUSE development/runtime support.

### libvips thumbnails

```bash
go build -tags govips -o gooru ./cmd/gooru
```

Requires libvips development files at build time and the shared library at runtime.

## Release policy

`VERSION` defines the canonical version. Tag reviewed releases on `main` using `vMAJOR.MINOR.PATCH`. Maintain `CHANGELOG.md` as changes land, moving the Unreleased notes to a matching version heading before tagging. The tag-triggered release workflow validates the version and notes; ordinary push/PR CI does not require release notes.

Official native Linux Debian and Nix packages dynamically link libvips. Portable Linux archives and the Windows zip are built without libvips; these builds do not include optional libvips-backed thumbnail support. To enable it in a source build, install libvips development files and build with `go build -tags govips -o gooru ./cmd/gooru` as shown above.
