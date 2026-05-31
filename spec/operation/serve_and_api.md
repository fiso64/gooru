
# Spec: The `gooru serve` Command and REST API

**Version:** 1.2
**Status:** Proposed

---

## 1. Abstract

This document specifies the `gooru serve` command, which runs a persistent HTTP process providing the first-party browser app, REST API, authenticated media routes, upload/import, thumbnails/previews, and in-memory jobs for a Gooru database. It is designed to be stable, responsive, and safe by managing concurrent requests and handling long-running operations according to client preference.

## 2. Problem Statement / Motivation

While the Gooru CLI is powerful for direct user interaction, it is not suitable for programmatic control by other applications. A third-party GUI or a Python script cannot easily or reliably parse CLI output to get structured data.

*   **User Story (Browser User):** As a user, I want `gooru serve` to open a usable media browser backed by the same API used by tools and scripts.
*   **User Story (GUI Developer):** As a GUI developer, I need a stable, documented, and machine-readable API so I can build graphical interfaces on top of Gooru without shelling out to the CLI.
*   **User Story (Scripter):** As a data scientist, I want to tag thousands of files from a Python script based on their contents, so I need an efficient way to send batch commands to Gooru and handle the results without blocking my script unnecessarily.

## 3. Goals and Non-Goals

### Goals

*   Provide a `gooru serve` command to launch a stable, long-running HTTP server.
*   Serve the static SvelteKit frontend and expose current core Gooru library functionality via a RESTful JSON API.
*   Serve authenticated media content, thumbnails, and previews from stable opaque file IDs.
*   Handle concurrent API requests safely, ensuring database integrity.
*   Provide a clear, client-driven mechanism for performing and monitoring long-running operations asynchronously to prevent client-side timeouts.
*   Keep the API as the single point of contact for the browser app and third-party applications.

### Non-Goals

*   This issue does not add broad rate limiting, multi-role permission modeling, native TLS flags, or OAuth/OIDC/LDAP support.

## 4. Proposed Solution & Technical Design

### 4.1. Command and Core Architecture

*   **Command:** `gooru serve --config serve.yaml`
    *   `server.listen` defaults to `127.0.0.1:5678`.
    *   `server.frontend_dir` points at the built static frontend.
    *   The command runs in the foreground until interrupted.

*   **Current Auth Model**
    *   The server uses DB-backed users and server-side sessions. Browser clients receive an opaque `HttpOnly` cookie such as `gooru_session`; only a SHA-256 hash of that token is stored in SQLite.
    *   Passwords are stored as self-describing Argon2id hashes. The first admin is created with `gooru user create-admin --username <name> --config serve.yaml`.
    *   Mutating cookie-authenticated requests must send `X-Gooru-CSRF` with a token returned from `POST /api/v1/auth/login` or `GET /api/v1/auth/me`. Safe `GET` media routes do not require CSRF.
    *   `auth.token`, `auth.token_env`, `auth.token_file`, and `--auth-token` are rejected with a migration message. `auth.enabled: false` is reserved for explicit trusted local development and is rejected on non-loopback binds unless the unsafe override is set.

*   **Concurrency Model: Bounded In-Memory Jobs**
    *   Mutations can run synchronously or asynchronously through an in-memory job manager.
    *   `jobs.max_queued` bounds pending work, `jobs.max_running` bounds concurrently running async jobs, `jobs.completed_ttl` expires terminal jobs, and `jobs.max_result_bytes` prevents large results from being retained.
    *   Read requests do not enter the job queue.

### 4.2. Synchronous vs. Asynchronous API Behavior

The API will support a hybrid model to provide both speed for fast operations and robustness for slow ones. The choice is **always driven by the client**.

*   **Default Behavior (Synchronous):**
    *   If a client sends a write request **without** the `Prefer: respond-async` header, the server will process it **synchronously**.
    *   The request will be placed in the write queue, and the server will wait for the job to be completed before sending a response.
    *   **Response:** `200 OK` or `201 Created` with the full result in the body.
    *   **Use Case:** Ideal for operations the client expects to be fast (e.g., tagging a single file) or for simple scripts where blocking behavior is acceptable. The client is responsible for setting an appropriate HTTP timeout.

*   **Asynchronous Opt-In:**
    *   If a client sends a write request **with** the `Prefer: respond-async` HTTP header, the server will **always** handle it **asynchronously**.
    *   The request will be placed in the write queue, and the server will immediately respond without waiting for the job to complete.
    *   **Response:** `202 Accepted` with a Job object in the body, containing a unique `id` for polling.
    *   **Use Case:** The recommended method for any potentially long-running operation (`relinkall`, batch operations on thousands of files) or for applications that must remain responsive (e.g., GUIs).

### 4.3. Job Management API

*   **Job Object:** A job will be represented by a JSON object:
    ```json
    {
      "id": "uuid-string-123",
      "type": "relink", // The type of job that was started
      "status": "pending" | "running" | "completed" | "failed" | "canceled",
      "progress": 0.75, // Optional, float between 0.0 and 1.0
      "submitted_at": "iso8601-timestamp",
      "result": { ... }, // Present on 'completed' status
      "error": "error message string" // Present on 'failed' status
    }
    ```

*   **Endpoint:** `GET /api/v1/jobs/{job_id}`
    *   This endpoint allows a client to poll for the status of an asynchronous job. It returns the full Job object.

### 4.4. API Endpoint Specification (v1)

All `POST`, `PUT`, `DELETE` endpoints that perform database writes support the `Prefer: respond-async` header.

#### Files & Tags

*   `POST /api/v1/auth/login`: Creates a server-side session and returns the current user, capabilities, and a CSRF token.
*   `POST /api/v1/auth/logout`: Revokes the current session and clears the session cookie.
*   `GET /api/v1/auth/me`: Returns the current user, capabilities, and a fresh CSRF token.
*   `POST /api/v1/auth/change-password`: Verifies the current password and stores a replacement Argon2id hash.
*   `GET /api/v1/files?query=<expr>&limit=<n>&page_token=<token>&sort=<name|modified|size|kind>&order=<asc|desc>`: Lists files matching an expression with bounded database pagination and optional facets.
*   `GET /api/v1/files/{id}`: Returns a file DTO with media URLs and cached optional metadata.
*   `DELETE /api/v1/files/{id}`: Untracks one file location with `{"mode":"untrack"}`.
*   `GET /api/v1/files/{id}/thumbnail?size=256`: Returns a cacheable thumbnail.
*   `GET /api/v1/files/{id}/preview`: Returns a larger preview derivative.
*   `GET /api/v1/files/{id}/content`: Returns original media content with range support.
*   `GET /api/v1/files/{id}/download`: Returns original media content as an attachment with range support.
*   `POST /api/v1/files/tags`: Adds tags to files. (`tag`)
    *   Body: `{"paths": ["..."], "tags": ["..."]}` or `{"query": "...", "tags": ["..."]}`
*   `PUT /api/v1/files/tags`: Sets/replaces tags for files. (`settags`)
    *   Body: `{"paths": ["..."], "tags": ["..."]}` or `{"query": "...", "tags": ["..."]}`
*   `DELETE /api/v1/files/tags`: Removes tags from files. (`untag`)
    *   Body: `{"paths": ["..."], "tags": ["..."]}` or `{"query": "...", "tags": ["..."]}`
*   `GET /api/v1/search/suggestions?q=<prefix>`: Returns deterministic tag autocomplete suggestions.
*   `GET /api/v1/tags/namespaces`: Returns known tag namespaces.
*   `GET /api/v1/saved-searches`: Lists saved searches for the current user.
*   `POST /api/v1/saved-searches`: Creates a saved search for the current user.
*   `PUT /api/v1/saved-searches/{id}`: Replaces a saved search owned by the current user.
*   `DELETE /api/v1/saved-searches/{id}`: Deletes a saved search owned by the current user.
*   `GET /api/v1/upload-targets`: Lists configured upload target IDs and names without exposing filesystem paths.
*   `POST /api/v1/uploads`: Uploads files into a configured upload target and imports them. Multipart requests use `target_id`, `files`, and optional initial `tags`; same-name conflicts follow the server-side `uploads.conflict_policy`.

#### Jobs (Long-Running Operations)

*   `GET /api/v1/jobs`: Lists jobs, optionally filtered by status.
*   `GET /api/v1/jobs/{job_id}`: Gets the status of any async job.
*   `DELETE /api/v1/jobs/{job_id}`: Cancels a pending/running job where possible.
*   `DELETE /api/v1/jobs?status=completed`: Clears finished jobs for the requested status.

## 5. Edge Cases & Unresolved Questions

*   **Server Crash:** If the `gooru serve` process crashes, all in-memory state (including the job queue) is lost. Running jobs (goroutines) are terminated.
*   **Database Locking:** The in-memory job manager bounds mutation concurrency but is not a durable queue. Later database/session work may replace parts of this model.
*   **Invalid API Input:** Endpoints will return `400 Bad Request` with a clear JSON error message detailing the validation failure.
*   **Pagination:** The public page-token shape is intentionally stable and currently uses opaque offset tokens backed by bounded database queries.
*   **Media Metadata:** DTOs include a metadata object with optional image/video/audio fields. Browse and detail responses read cached metadata and do not synchronously probe media files in result construction.

## 6. Alternatives Considered

*   **gRPC API:** An alternative to REST/JSON. Can achieve much higher performance due to smaller payloads, multiplexing and streaming, but is not as common. Might want to offer both.
*   **Exposing mounting through the server:** Rejected for now due to higher complexity and no use case.
