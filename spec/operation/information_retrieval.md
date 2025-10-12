# Spec: Information Retrieval Operations

**Version:** 1.1
**Status:** Proposed

---

## 1. Abstract

This document specifies the behavior of read-only commands such as `gettags`, `list`, `table`, and `listtags`. The guiding principle is that these commands must be fast, predictable, and report on the current state of the database. For path-based commands like `gettags`, the default behavior should be content-centric, correctly identifying files by their content even if they have been moved or copied.

## 2. Problem Statement / Motivation

Users need to query the Gooru database to find files and view tags. These operations should be fast and intuitive. When a user reorganizes files (moves, copies, renames), a strictly path-based lookup can provide confusing results, such as failing to find tags for a known file simply because its location has changed. This contradicts the "content-centric" promise of the tool. The system needs a clear policy that prioritizes content identity while guiding the user toward synchronizing filesystem changes.

## 3. Goals and Non-Goals

### Goals

*   Ensure all information retrieval commands are fast and predictable.
*   Define a content-centric default behavior for `gettags` that correctly identifies content regardless of path.
*   Provide helpful, actionable feedback to the user when a file is in a state of flux (modified, moved, copied).
*   Maintain a strict read-only nature for these commands; they will not modify the database.

### Non-Goals

*   Expression-based commands (`list`, `table`) will not perform filesystem checks; they report the database state as-is.
*   No information retrieval command will automatically update file locations. That is the responsibility of write commands (`add`, `tag`, etc.) and `relinkall`.

## 4. Proposed Solution & Technical Design

Read-only commands are separated into two categories: path-based and expression-based.

### 4.1. Path-Based Retrieval (`gettags`)

The `gettags` command takes a direct file path, but its primary identifier for a file is its content.

*   **Default Behavior (Content-Centric Hashing):**
    *   **Mechanism:** When `gettags <path>` is run, the command *first* hashes the file at `<path>` to determine its `current_hash`. It then queries the database for both the `<path>` and the `current_hash`.
    *   **Logic & User Feedback:**
        *   **`StatusOK`**: The `<path>` is known and its stored hash matches `current_hash`.
            *   **Action:** Display tags.
        *   **`StatusModified`**: The `<path>` is known, but its stored hash is different from `current_hash`.
            *   **Action:** Display no tags. Show a warning that the file has been modified.
        *   **`StatusUntrackedContent`**: The `<path>` is **not** known, but the `current_hash` **is** known. This indicates a moved file, a copy, or a coincidental duplicate.
            *   **Action:** Display the tags associated with the known content. Show a warning that the file's location is not tracked and suggest running `gooru add`.
        *   **`StatusNotInDB`**: The `<path>` is **not** known, and the `current_hash` is **not** known.
            *   **Action:** Display no tags. Show a message suggesting the user track the file with `gooru add`.

*   **Performance Opt-In (`--use-metadata`):**
    *   The `gettags` command will retain a `--use-metadata` flag.
    *   When present, it reverts to a faster, path-centric logic. It looks up the `<path>` in the database and compares the file's current `size + modification time` to the stored values. This check cannot detect moved or copied files and is only for users who need maximum speed and understand the trade-off.

### 4.2. Expression-Based Retrieval (`list`, `table`, `listtags`)

These commands operate on abstract data within the database (tags, content hashes) and do not take specific filesystem paths as direct inputs.

*   **Mechanism:** They parse the user's expression, build a SQL query, and execute it against the database. They have no direct interaction with the filesystem.
*   **Result:** They return a list of paths or a table of information exactly as it is recorded in the database. If the filesystem has changed, the paths returned may be stale. This is the expected behavior, and the user must run `relinkall` to synchronize these paths.