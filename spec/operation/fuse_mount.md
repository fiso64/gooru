# Spec: FUSE Virtual Filesystem (`mount` command)

**Version:** 1.1
**Status:** Proposed

---

## 1. Abstract

This document specifies the `gooru mount` command, which provides a FUSE-based virtual filesystem. The filesystem presents a live, navigable view of the Gooru database, allowing users to interact with their tagged files via standard GUI file managers and command-line tools. The presentation of the filesystem can be explicitly controlled by the user to be either a flat list of results or a hierarchical, tag-based browsing structure.

## 2. Problem Statement / Motivation

Users need an intuitive way to explore their tagged file collection that goes beyond simple command-line lists. A visual, interactive representation of the tag data allows for faster discovery and management.

*   **User Story 1:** As a user, I want to browse my files in my standard file manager by clicking through tags as if they were folders, so I can visually discover and filter my collection.
*   **User Story 2:** As a user, after running a specific search, I want to see all the resulting files in a single folder, so I can easily select them, open them, or drag them into another application.
*   **User Story 3:** As a user, I want to be able to tag a file I found in the virtual filesystem by its virtual path, so I don't have to find its "real" location first.
*   **User Story 4:** As a developer of a GUI front-end, I want to be able to programmatically change the query of a mounted Gooru filesystem so that I can provide a search bar that dynamically updates the file manager's view.
*   **User Story 5:** As a user running multiple `gooru mount` instances, I want an easy way to list them and send commands to the correct one without ambiguity.

## 3. Goals and Non-Goals

### Goals

*   Provide a `mount` command that creates a virtual filesystem view of the database based on a query expression.
*   Allow the user to explicitly choose between a flat or hierarchical presentation via a command-line flag.
*   Ensure full support for OS-native features like thumbnails and previews.
*   Allow other `gooru` commands to operate correctly on files referenced by their virtual paths.
*   Provide an IPC mechanism for other applications to dynamically update the query powering the filesystem view.
*   Ensure all filesystem operations are performant and reflect the live state of the database.

### Non-Goals

*   The virtual filesystem will be read-only in terms of file content modification (e.g., no `mv`, `rm`, or editing files in-place). Tagging operations are handled by the `gooru` CLI.

## 4. Proposed Solution & Technical Design

The user will have direct control over the filesystem's structure via a command-line flag.

*   **Command Syntax:** `gooru mount <mountpoint> [expression..] [--hierarchical]`
*   **Alias:** `-h` for `--hierarchical`.

### 4.1. Filesystem Modes

The mode is determined by the presence of the `--hierarchical` flag, not by the expression.

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

*   **Trigger:** The `--hierarchical` or `-h` flag **is** present.
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

### 4.2. Common Technical Implementation

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
*   **Invalid Directory Names:** Tags can contain characters that are invalid in filenames on some operating systems (e.g., `:` on Windows).
    *   **Decision:** The FUSE driver will sanitize tag names for presentation, replacing invalid characters with a safe substitute (e.g., `_`). The internal logic will map the sanitized name back to the original tag.
*   **Live Updates:** The filesystem is a live view. If a file is tagged in another terminal, the changes will be reflected the next time a directory in the mount is accessed (e.g., via `ls` or a GUI refresh), as this triggers a new database query.
*   **Stale Mounts:** A `mount` process might crash without cleaning up its runtime file.
    *   **Decision:** The `gooru remote` and `gooru remote list` commands will always perform a health check by verifying that the PID in the runtime file corresponds to a running process. If not, they will exclude the mount and automatically clean up the orphaned runtime files.

## 6. Performance Considerations

The design prioritizes responsive user interaction, with performance characteristics varying by use case.

*   **Mount Launch & Query Updates:** Performance is dictated by database query speed. For most queries, this will be nearly instantaneous.
*   **Browsing:**
    *   **Flat Mode (default):** Excellent performance. The initial query result is cached in memory, making directory listings instant.
    *   **Hierarchical Mode:** Good performance. Each navigation triggers a live database query. A slight latency may be noticeable when entering virtual directories corresponding to very large file sets.
*   **Tagging via Virtual Filesystem:** Operations on single virtual paths will have a negligible performance impact. Batch operations on thousands of virtual paths may be slower than on real paths due to the per-file IPC path resolution overhead.
*   **Tagging Normal (Non-Virtual) Files:** The impact is imperceptible. The new path resolution logic requires that every gooru api method taking a file path first checks if that path belongs to a virtual filesystem. This check involves scanning the ~/.config/gooru/run/ directory for active mount configurations. 
    *   If no mounts are active, this check is extremely fast (a single directory existence check). Instant.
    *   If mounts are active, the command must read one or more small JSON files and perform a string prefix comparison. Still very fast.