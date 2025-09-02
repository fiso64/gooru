
# Spec: The `gooru serve` Command and REST API

**Version:** 1.1
**Status:** Proposed

---

## 1. Abstract

This document specifies the `gooru serve` command, which runs a persistent daemon process providing a comprehensive, local RESTful API for all Gooru functionality. This server is the primary mechanism for third-party applications (GUIs, scripts, other tools) to programmatically interact with a Gooru database. It is designed to be stable, responsive, and safe by managing concurrent requests, and handling long-running operations according to client preference.

## 2. Problem Statement / Motivation

While the Gooru CLI is powerful for direct user interaction, it is not suitable for programmatic control by other applications. A third-party GUI or a Python script cannot easily or reliably parse CLI output to get structured data.

*   **User Story (GUI Developer):** As a GUI developer, I need a stable, documented, and machine-readable API so I can build a graphical interface on top of Gooru without shelling out to the CLI.
*   **User Story (Scripter):** As a data scientist, I want to tag thousands of files from a Python script based on their contents, so I need an efficient way to send batch commands to Gooru and handle the results without blocking my script unnecessarily.

## 3. Goals and Non-Goals

### Goals

*   Provide a `gooru serve` command to launch a stable, long-running HTTP server.
*   Expose all core Gooru library functionality via a RESTful JSON API.
*   Handle concurrent API requests safely, ensuring database integrity.
*   Provide a clear, client-driven mechanism for performing and monitoring long-running operations asynchronously to prevent client-side timeouts.
*   The API must be the single point of contact for third-party applications.

### Non-Goals

*   This server will not provide any HTML user interface. It is a headless API server only.
*   The server will not implement complex authentication or authorization. It is intended to be bound to `localhost` and is secured by filesystem permissions on the database and runtime directory.

## 4. Proposed Solution & Technical Design

### 4.1. Command and Core Architecture

*   **Command:** `gooru serve [--host <ip>] [--port <port>]`
    *   `--host`: Defaults to `127.0.0.1` (localhost) for security.
    *   `--port`: Defaults to a sensible, often-unused port like `5678`.
    *   The command will run in the foreground, logging to standard output, until terminated by `Ctrl+C`.

*   **Concurrency Model: Serial Write Queue**
    *   To prevent database locking issues between concurrent write requests, the server will implement a **serial write queue**.
    *   All incoming API requests that require writing to the database (`tag`, `settags`, `untag`, `relinkall`, `rehash`, `delete`, etc.) will be placed into a single, in-memory queue.
    *   A dedicated worker goroutine will process this queue one job at a time, in FIFO order.
    *   Read requests (`list`, `gettags`, `table`, etc.) do not enter this queue and can be executed concurrently, as SQLite's WAL mode allows for concurrent readers.
    *   This architecture guarantees that no two write operations ever conflict, eliminating "database is locked" errors and ensuring strict serializability.

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
      "status": "pending" | "running" | "completed" | "failed",
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

*   `GET /api/v1/files?query=<expr>`: Lists files matching an expression. (`list`, `table`)
*   `POST /api/v1/files/tags`: Adds tags to files. (`tag`)
    *   Body: `{"paths": ["..."], "tags": ["..."]}` or `{"query": "...", "tags": ["..."]}`
*   `PUT /api/v1/files/tags`: Sets/replaces tags for files. (`settags`)
    *   Body: `{"paths": ["..."], "tags": ["..."]}` or `{"query": "...", "tags": ["..."]}`
*   `DELETE /api/v1/files/tags`: Removes tags from files. (`untag`)
    *   Body: `{"paths": ["..."], "tags": ["..."]}` or `{"query": "...", "tags": ["..."]}`
*   `DELETE /api/v1/files`: Deletes file records from the database. (`delete`)
    *   Body: `{"paths": ["..."]}` or `{"query": "..."}`

#### Jobs (Long-Running Operations)

*   `POST /api/v1/jobs/relink`: Initiates a `relinkall` operation.
    *   Body: `{"directories": ["/path/one", "/path/two"]}`
*   `POST /api/v1/jobs/rehash`: Initiates a `rehash` operation.
    *   Body: `{"paths": ["/path/one", "/path/two"]}`
*   `GET /api/v1/jobs/{job_id}`: Gets the status of any async job.

## 5. Edge Cases & Unresolved Questions

*   **Server Crash:** If the `gooru serve` process crashes, all in-memory state (including the job queue) is lost. Running jobs (goroutines) are terminated.
*   **Database Locking:** The serial write queue architecture is the explicit solution to prevent the server from deadlocking itself or failing due to "database is locked" errors.
*   **Invalid API Input:** Endpoints will return `400 Bad Request` with a clear JSON error message detailing the validation failure.

## 6. Alternatives Considered

*   **gRPC API:** An alternative to REST/JSON. Can achieve much higher performance due to smaller payloads, multiplexing and streaming, but is not as common. Might want to offer both.
*   **Exposing mounting through the server:** Rejected for now due to higher complexity and no use case.