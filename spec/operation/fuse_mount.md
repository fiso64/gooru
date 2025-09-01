# Spec: FUSE Virtual Filesystem (`mount` command)

**Version:** 1.2
**Status:** Implemented

---

## 1. Abstract

This document specifies the `gooru mount` command, which provides a cross-platform virtual filesystem. The filesystem presents a live, navigable view of the Gooru database, allowing users to interact with their tagged files via standard GUI file managers and command-line tools. The presentation of the filesystem can be explicitly controlled by the user to be either a flat list of results or a hierarchical, tag-based browsing structure.

## 2. Problem Statement / Motivation

Users need an intuitive way to explore their tagged file collection that goes beyond simple command-line lists. A visual, interactive representation of the tag data allows for faster discovery and management.

*   **User Story 1:** As a user, I want to browse my files in my standard file manager by clicking through tags as if they were folders, so I can visually discover and filter my collection.
*   **User Story 2:** As a user, after running a specific search, I want to see all the resulting files in a single folder, so I can easily select them, open them, or drag them into another application.
*   **User Story 3:** As a user, I want to be able to tag a file I found in the virtual filesystem by its virtual path, so I don't have to find its "real" location first.

## 3. Goals and Non-Goals

### Goals

*   Provide a `mount` command that creates a virtual filesystem view of the database based on a query expression.
*   Allow the user to explicitly choose between a flat or hierarchical presentation via a command-line flag.
*   Provide an optional "live" mode that automatically refreshes the view when accessed, at a potential performance cost.
*   Ensure full support for OS-native features like thumbnails and previews.
*   Allow other `gooru` commands to operate correctly on files referenced by their virtual paths.
*   Provide an IPC mechanism for other applications to dynamically update the query powering the filesystem view.
*   Ensure all filesystem operations are performant and reflect the live state of the database.

### Non-Goals

*   The virtual filesystem will be read-only in terms of file content modification (e.g., no `mv`, `rm`, or editing files in-place). Tagging operations are handled by the `gooru` CLI.

## 4. Proposed Solution & Technical Design

The user will have direct control over the filesystem's structure via a command-line flag.

*   **Command Syntax:** `gooru mount <mountpoint> [expression..] [--hierarchical] [--open] [--live]`
*   **Aliases:** `-r` for `--hierarchical`, `-o` for `--open`, `-l` for `--live`.
*   **Flags:**
    *   `--open, -o`: After a successful mount, open the mount point in the system's default file explorer.
    *   `--live, -l`: Enables live mode. The database query is re-run every time a directory is accessed (e.g., on a file explorer refresh). This ensures the view is always up-to-date but may impact performance on very large queries.

### 4.1. Refresh Behavior: Snapshot vs. Live Mode

The user can control how "live" the filesystem view is, which involves a direct trade-off with performance.

*   **Snapshot Mode (Default):**
    *   **Trigger:** The `--live` flag is **not** present.
    *   **Behavior:** The mount process executes the query **once** at startup (or when updated via `gooru remote`) and caches the results in memory. All subsequent directory listings read from this in-memory cache.
    *   **Pros:** Extremely fast and responsive, even for queries with hundreds of thousands of results. Puts zero load on the database during browsing.
    *   **Cons:** The view can become stale. If files are tagged or untagged in another terminal, the changes will not be reflected in the mount until the query is manually re-run with `gooru remote query ...`.

*   **Live Mode:**
    *   **Trigger:** The `--live` or `-l` flag **is** present.
    *   **Behavior:** The mount process re-executes its database query every time a directory's contents are requested (e.g., when a user presses F5 in a file explorer or runs `ls`).
    *   **Pros:** The view is always up-to-date, reflecting the current state of the database without any manual intervention.
    *   **Cons:** Introduces latency. Each refresh takes as long as the database query. This can be noticeable for very large or complex queries. It also puts a recurring load on the database, as background OS processes (thumbnailers, indexers) may trigger refreshes frequently.

### 4.2. Filesystem Structure Modes

The structural mode is determined by the presence of the `--hierarchical` flag, not by the expression.

#### Mode 1: Flat Mode (Default)

This is the default behavior. It is optimized for displaying a final result set.

*   **Trigger:** The `--hierarchical` flag is **not** present.
*   **Structure:** The mount point is a single, flat directory containing all files that match the given `expression`. If no expression is provided, it lists all files in the database.
*   **Justification:** This provides a simple, direct view of query results, which is the most common use case for specific searches.

**Example (`gooru mount /mnt/gooru vacation`):**
Given files:
*   `A.jpg`: `photo`, `vacation`
*   `B.pdf`: `work`, `vacation`
The structure would be a single directory containing `A.jpg` and `B.pdf`.

#### Mode 2: Hierarchical Mode

This mode is for discovery and interactive filtering.

*   **Trigger:** The `--hierarchical` or `-r` flag **is** present.
*   **Structure:** The filesystem is a multi-level directory hierarchy representing tag intersections based on the files matching the initial `expression`.
    *   The **root** of the mount contains directories for each tag present on the files matching the initial expression. It also contains the files themselves.
    *   Navigating into a subdirectory (`/<tag1>/<tag2>/`) acts as an implicit `AND` query, further filtering the view to show only files and co-occurring tags that match all tags in the path *in addition to* the initial expression.

**Example (`gooru mount -h /mnt/gooru vacation`):**
Given files:
*   `A.jpg`: `photo`, `vacation`
*   `B.pdf`: `work`, `vacation`
*   `C.txt`: `photo` (Does not match initial expression, will not appear)

The initial query for `vacation` returns `A.jpg` and `B.pdf`. The unique tags across this set are `photo`, `work`, and `vacation`. The structure would be:
```
/mnt/gooru/
├── photo/
│   ├── vacation/
│   └── A.jpg
├── vacation/
│   ├── photo/
│   ├── work/
│   ├── A.jpg
│   └── B.pdf
└── work/
    ├── vacation/
    └── B.pdf
```
Navigating to `/mnt/gooru/photo/` shows `A.jpg` (as it has both `vacation` and `photo`). The user can interactively drill down from the initial result set.

### 4.2. Cross-Platform Technical Implementation

The virtual filesystem will be implemented using the `cgofuse` library, which provides a single, consistent, cross-platform API for Go. This allows Gooru to maintain one codebase for the virtual filesystem that runs natively on all supported operating systems.

The end-user will need to install the appropriate underlying driver for their platform:
*   **Windows:** [WinFsp (Windows File System Proxy)](https://winfsp.dev/) must be installed.
*   **macOS:** [macFUSE](https://osxfuse.github.io/) must be installed.
*   **Linux:** The `libfuse` library must be installed (e.g., via `sudo apt-get install libfuse-dev` on Debian/Ubuntu or `sudo dnf install fuse-devel` on Fedora).

The application must provide clear instructions for these prerequisites.

*   **File Proxies:** On all platforms, files in the virtual filesystem will be presented as regular files, not symlinks. The `cgofuse` driver will proxy I/O requests (e.g., `read`) to the real files on disk to ensure universal compatibility with features like thumbnail generation and file previews.

*   **File Proxies:** All files in the virtual filesystem will be presented as **regular files**, not symlinks. The FUSE driver will proxy `read` requests to the real files on disk to ensure universal thumbnail and preview support.

*   **Instance Management and IPC:** To robustly manage multiple concurrent mounts, each `mount` process will register itself in a well-known runtime directory.
    1.  **Runtime Directory:** A directory at `~/.config/gooru/run/` will store information about active mounts.
    2.  **Mount Lifecycle:**
        *   **On start:** The `gooru mount` command will canonicalize its mount point path. It will generate a stable identifier (e.g., a short hash of the canonical path). It will then create a file in the runtime directory (e.g., `~/.config/gooru/run/<id>.json`) containing its Process ID (PID), the path to its unique IPC socket, and the mount point path itself. If this file already exists and the PID is active, the command will fail, preventing two mounts on the same directory.
        *   **On exit:** The `mount` process will register a signal handler to ensure it cleans up its runtime file and IPC socket upon termination (`SIGINT`, `SIGTERM`).
    3.  **IPC Protocol:** The process will listen on its unique IPC socket for simple, newline-delimited commands:
        *   `SET_QUERY <expression>`: Updates the filesystem's view to match the new expression.
        *   `GET_QUERY`: Returns the current query expression.
        *   `RESOLVE <virtual_path>`: Returns the canonical, real path for a given virtual path.
        *   `PING`: A health check command, returns `PONG`.

*   **Remote Control and Path Resolution:**
    *   A new command group, `gooru remote`, will be the user's interface for managing active mounts.
    *   **`gooru remote list`**: This command will scan the runtime directory, check that the PIDs in the files are active, and print a table of all running mounts (e.g., Mount Point, PID, Current Query).
    *   **`gooru remote <mountpoint> query [new_expression...]`**: This command will find the correct mount by reading the runtime files, connect to its specific IPC socket, and send the `SET_QUERY` or `GET_QUERY` command.
    *   **Internal Path Resolution:** Other `gooru` commands (like `tag`, `untag`) will use the same discovery mechanism. When given a virtual path, they will find the corresponding running mount via the runtime files, connect to its IPC socket, send a `RESOLVE` command, and then operate on the returned real path.

This design provides a robust way to discover, manage, and communicate with multiple, independent `gooru mount` instances.

## 5. Edge Cases & Unresolved Questions

*   **Filename Collisions:** Two files with the same name (e.g., `/a/report.pdf`, `/b/report.pdf`) might appear in the same virtual directory.
    *   **Decision:** The FUSE driver must disambiguate them. It will append a differentiator based on the file's content hash, like `report-<hash_prefix>.pdf`.
*   **Invalid Directory Names:** Tags can contain characters that are invalid in filenames on some operating systems (e.g., `:` is disallowed in Windows filenames).
    *   **Decision:** The virtual filesystem driver will sanitize tag names for presentation, replacing invalid characters with a safe substitute (e.g., replacing `:` with `_`). The internal logic will map the sanitized name back to the original tag when processing paths.
*   **Live Updates:**
    *   **In Snapshot Mode (default):** The view is **not** live. Changes made in another terminal will not be visible until the query is manually refreshed via `gooru remote query...`.
    *   **In Live Mode (`--live`):** The view is live. Changes will be reflected the next time the directory is accessed (e.g., via `ls` or a GUI refresh), as this triggers a fresh database query.
*   **Stale Mounts:** A `mount` process might crash without cleaning up its runtime file.
    *   **Decision:** The `gooru remote` and `gooru remote list` commands will always perform a health check by verifying that the PID in the runtime file corresponds to a running process. If not, they will exclude the mount and automatically clean up the orphaned runtime files.

## 6. Performance Considerations

The implementation prioritizes a responsive user experience, especially during interactive browsing.

*   **Thumbnail Generation & File Lookups:** An initial performance bottleneck was identified where OS thumbnail generation in hierarchical mode triggered a cascade of inefficient file lookups. This has been resolved by implementing a short-lived, in-memory cache (`dirCache`) that memoizes a directory's contents for the duration of a single view operation (e.g., one `ls` command or one folder view in a GUI). This makes browsing directories with many files and thumbnails instantaneous.

*   **Browsing Latency (Live Mode):** In live mode (`--live`), each directory navigation or refresh triggers a fresh database query. For most queries, this is imperceptible. For very complex queries against millions of records, a slight latency may be noticeable. In the default "snapshot" mode, browsing is always instant as it reads from the in-memory cache.

*   **Filename Disambiguation at Scale:** The logic for detecting filename collisions and appending a hash prefix (e.g., `report-<hash>.pdf`) runs in Go code every time a directory is listed. While extremely fast for directories containing thousands of files, this can become a minor CPU bottleneck if a single virtual directory contains an exceptionally large number of files (e.g., >50,000). This is a scaling consideration for extreme use cases, not a day-to-day performance issue. Future optimization could involve moving this disambiguation logic into a more complex SQL query.

*   **IPC & Path Resolution:** The overhead for resolving virtual paths to real paths via IPC is minimal. For commands operating on non-virtual paths, the check to see if any mounts are active is extremely fast and has no noticeable impact.