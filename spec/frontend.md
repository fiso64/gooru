# Spec: Frontend Stack and Browser App Scope

**Version:** 1.1
**Status:** Proposed

---

## 1. Abstract

Gooru should eventually include a booru-style browser frontend for searching, browsing, tagging, uploading, and previewing local media. This document records the frontend-related architecture decisions made so far. It intentionally does not specify detailed UI design, page layout, component structure, or exact user flows.

## 2. Product Direction

The frontend is a first-party browser app backed by `gooru serve`. It is meant for personal media-library workflows, especially large collections of images, video, audio, and related files.

Core user capabilities:

*   Browse a paginated, thumbnail-based media grid.
*   Search by Gooru queries and tags.
*   View larger previews and original media.
*   Add, remove, and set tags from the browser.
*   Upload/import files and optionally tag them during upload.
*   Track long-running jobs such as uploads, relink, rehash, and thumbnail generation.

## 3. Goals and Non-Goals

### Goals

*   Use a modern, type-safe frontend stack.
*   Build static frontend assets that can be served by the Go server.
*   Avoid requiring a Node server in production.
*   Keep the frontend/API contract strong enough for future remote use.
*   Use server-state caching for search results, pagination, file details, tag lists, mutations, and job polling.
*   Support fast media browsing through thumbnails, previews, HTTP caching, and range-capable original media URLs.
*   Keep local UI state separate from server-state caching.
*   Keep the frontend structure compatible with a future generated API client and future CLI remote backend.

### Non-Goals

*   This document does not define the final visual design.
*   This document does not define all routes, pages, keyboard shortcuts, batch UI flows, or component hierarchy.
*   The v1 frontend does not require server-side rendering in production.
*   The v1 deployment does not require a separate frontend service.
*   Offline-first behavior and service-worker media caching are not required in v1.
*   Full video transcoding, HLS, and DASH are not frontend requirements for v1.

## 4. Frontend Stack

Recommended stack:

```text
SvelteKit
TypeScript
Vite
TanStack Svelte Query
OpenAPI-described API contract with generated-client migration path
Vitest
Playwright
virtualized media grid
```

SvelteKit should be used as the application framework, but the production build should be static. The frontend should be built with SvelteKit's static adapter or an equivalent static-output configuration, then served by `gooru serve`.

TypeScript is required for frontend code. The current Issue #1 implementation has a small hand-maintained API client and type file; any DTO or endpoint change must update `docs/openapi.yaml` and `frontend/src/lib/api/types.ts` together. A generated client remains the preferred direction once the contract stabilizes.

Tailwind CSS should be the default styling utility layer. It should be used for layout, spacing, responsive behavior, dark mode, and shared design tokens. Svelte scoped styles should still be used where Tailwind becomes awkward or where a component needs bespoke media-grid, preview, or interaction polish.

A heavy component framework is not required initially. The project should prefer owned Svelte components over adopting a large component library. Copied or generated components are acceptable later if they are restyled to match the app rather than defining the app's visual identity.

A separate global state library is not required initially. Svelte state/stores should be enough for local UI state.

## 5. Serving Model

The frontend should live in the same repository, likely under:

```text
frontend/
```

Production serving model:

```text
frontend/ builds static assets
Go embeds or copies the built assets
gooru serve serves the app, API, and media routes
```

Representative route ownership:

```text
/                 frontend app
/api/v1/...       JSON API
/api/v1/files/... authenticated media routes
```

The Go server should serve hashed static assets with long-lived cache headers. The frontend entrypoint, such as `index.html`, should use no-cache or short-cache headers so deployments can update cleanly.

A Node server should not be required in production for v1. Node tooling is only required at build/dev time.

The current auth flow uses DB-backed login and HttpOnly same-origin session
cookies. The browser keeps only the CSRF token returned by `/api/v1/auth/login`
or `/api/v1/auth/me` in memory and sends it in `X-Gooru-CSRF` for mutating
requests. Media routes should load directly through same-origin cookies.

## 5.1. Styling Direction

Use Tailwind CSS as the default styling approach, with project-owned design tokens and custom Svelte components. Tailwind is a utility layer, not a component theme; the app should avoid relying on default-looking component kits or copied dashboard aesthetics.

The desired visual direction is a dense, fast, media-first local library tool: thumbnail-forward, keyboard-friendly, dark-mode-friendly, and optimized for browsing large collections.

Svelte scoped styles remain appropriate for custom grid behavior, transitions, overlays, lightbox polish, media metadata layouts, and any CSS that is clearer outside utility classes.

Do not adopt a large component framework for v1. Optional copied components may be used later when they reduce routine UI work, but they should be owned and restyled as part of the app.

## 6. Server-State Caching

Use TanStack Svelte Query for remote/server state.

Good uses:

*   File search result pages.
*   Infinite scrolling / cursor pagination.
*   File detail metadata.
*   Tag lists and tag counts.
*   Job polling.
*   Upload/import mutation state.
*   Tag add/remove/set mutations.
*   Cache invalidation after tag, delete, upload, editpath, relink, or rehash operations.
*   Optimistic tag edits where safe.

Do not use TanStack Query for purely local UI state.

Local UI state examples:

*   Selected files.
*   Active lightbox item.
*   Current unsent search input.
*   Sidebar open/closed state.
*   Drag selection state.
*   Keyboard navigation state.
*   Session and CSRF state.

The frontend should treat TanStack Query as the server-state cache, Svelte state/stores as local UI state, and HTTP caching as the media/static-asset cache.

## 7. Media-Oriented API Expectations

The frontend depends on the server exposing browser-friendly media primitives:

```text
GET /api/v1/files?query=...&limit=...&page_token=...
GET /api/v1/files/{id}
GET /api/v1/files/{id}/thumbnail?size=256&format=jpeg
GET /api/v1/files/{id}/preview
GET /api/v1/files/{id}/content
POST /api/v1/uploads
GET /api/v1/jobs/{id}
DELETE /api/v1/jobs/{id}
```

Browser clients should use stable opaque file ids. They should not need to construct media URLs from raw filesystem paths.

Original video/audio/image content must support range requests and useful cache validators so browser media controls, seeking, and preview loading work well over a network.

Thumbnail and preview URLs should be cacheable. The frontend should be able to request bounded thumbnail sizes from a fixed allowlist rather than arbitrary dimensions.

File DTOs include a metadata object with optional fields for image dimensions,
video dimensions/duration, and audio duration. Current extraction is best-effort;
the frontend must handle missing metadata without visual regressions.

## 8. Thumbnailing and Preview Generation

Fast thumbnails are important enough to shape the backend dependency choices.

Recommended v1 backend choices:

```text
images: libvips through govips as the primary thumbnailer
videos: ffmpeg + ffprobe through os/exec
fallbacks: pure-Go image decoding and/or ffmpeg for unsupported image cases
```

Rationale:

*   libvips is generally a strong fit for high-throughput thumbnail generation and large image libraries.
*   ffmpeg is the practical default for video thumbnails because it handles codec/container variety.
*   Calling ffmpeg/ffprobe as external binaries is preferable to Go FFmpeg bindings for v1 because it avoids cgo/version-binding complexity.
*   A pure-Go image path remains useful for tests, minimal builds, and fallback behavior.

Thumbnailing should be implemented behind interfaces so backends can change without touching handlers or frontend contracts.

Representative backend layout:

```text
internal/media/thumb/
  vips.go
  video_ffmpeg.go
  image_go.go
  ffmpeg_image.go
```

Representative cache key inputs:

```text
content hash
media kind
derivative kind
requested size/profile
output format
thumbnailer backend/version
```

Changing thumbnail algorithms or output settings should naturally invalidate old derivatives by changing the cache key.

If libvips or ffmpeg is unavailable, the server should report a clear machine-readable error for affected media operations while keeping the rest of the server usable.

## 9. Performance Notes

Frontend performance depends on the backend and API contract as much as the UI framework.

Important requirements:

*   Cursor pagination for large result sets. The current API keeps the public cursor/page-token shape but performs compatibility pagination in memory until the library layer exposes store-backed cursors.
*   Virtualized grid rendering.
*   Small metadata responses by default.
*   Aggregate batch mutation responses by default, verbose details only when requested.
*   Stable file ids to avoid path escaping and URL churn.
*   Cacheable thumbnail/preview responses.
*   HTTP range support for original media.
*   No full-size media downloads for normal grid browsing.
*   Query invalidation after writes rather than manual ad-hoc refresh logic.

The frontend should avoid loading all results for a search at once. Infinite scrolling should be backed by paginated API calls.

## 10. Testing Notes

Frontend testing should start with:

*   Unit tests for query-key construction, API adapters, and utility logic.
*   Component tests where useful for search/tag controls.
*   Playwright tests for core browser flows once the app exists.
*   Fake API responses for most frontend tests.
*   A small number of end-to-end tests against `gooru serve` for upload, browse, search, tag, and preview flows.

Backend media behavior should be tested independently from the frontend:

*   Range requests.
*   Cache headers.
*   Thumbnail cache hits/misses.
*   Missing backend errors.
*   Auth on media routes.
*   Upload path safety.

## 11. Deferred Decisions

The following should be decided later, closer to UI implementation:

*   Exact visual design and theme details.
*   Page/component structure.
*   Keyboard shortcut model.
*   Multi-select and batch-edit UX.
*   Whether to add service-worker prefetching/offline behavior after v1.
*   Whether to add native TLS flags after v1 in addition to reverse-proxy deployment.
*   Whether to add more advanced video previews or transcoded derivatives after v1.
