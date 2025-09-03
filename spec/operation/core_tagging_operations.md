# Spec: Core Tagging Operations

**Version:** 1.1
**Status:** Implemented

---

## 1. Abstract

This document specifies the behavior of the primary database modification commands: `tag`, `settags`, and `untag`. These commands allow users to manage the relationship between content (identified by hash) and descriptive tags. The spec covers both path-based and expression-based operations and defines a clear, consistent policy for handling files that have been modified on disk.

## 2. Problem Statement / Motivation

The core utility of Gooru is applying and managing tags. Users need a clear, fast, and predictable way to perform the three fundamental tagging actions: adding tags, replacing all tags, and removing specific tags. The behavior of these commands must be safe and robust, especially when the filesystem's state has changed since the last operation. It should be clear to the user when the system is automatically updating its records for a modified file.

## 3. Goals and Non-Goals

### Goals

*   Define the syntax and behavior for `tag` (additive), `settags` (declarative/destructive), and `untag` (subtractive).
*   Support both individual file paths and bulk operations via query expressions.
*   Establish a clear, safe, and predictable policy for handling files that have been modified on disk.
*   Ensure users are always notified when a file modification is detected and handled automatically.
*   Ensure all database modifications are transactional.

### Non-Goals

*   This spec does not cover the query expression syntax itself, which is detailed in a separate document.
*   It does not cover the process of discovering moved/renamed files (see Filesystem Synchronization Spec).

## 4. Proposed Solution & Technical Design

The three commands share common logic for parsing arguments (files vs. expressions) and interacting with the database. Their primary distinction lies in the final database operation. Their behavior when encountering modified files is now unified.

### 4.1. Common Behavior

*   **Input Sources:** All three commands will support two modes of identifying target files, controlled by an `-e`/`--expression` flag.
    1.  **Path Mode (default):** The input is one or more file paths, globs, or directories.
    2.  **Expression Mode:** The input is a query expression that resolves to a set of content hashes.
*   **Transactional Integrity:** All database writes for a single command invocation must occur within a single transaction. If any part of the operation fails, the entire transaction is rolled back, leaving the database unchanged.

### 4.2. Behavior on File Modification (Path Mode Only)

This is a critical rule defining how the system handles a file path that points to content different from what the database has on record for that path. The behavior is consistent across all tagging commands (`tag`, `add`, `settags`, `untag`).

*   **Default Behavior ("Always Hash"):** To guarantee correctness, the system will **always** perform a content hash on a file provided by its path to get its definitive, current content hash (`H_CURRENT`). It compares this to the hash stored in the database for that path (`H_DB`).
    *   If the hashes differ (`H_CURRENT != H_DB`), the file has been modified.
    *   The system updates the `locations` table to associate the file path with the new hash (`H_CURRENT`).
    *   The user is notified that the file's record has been updated, and a warning is shown if the old content (`H_DB`) had tags that are now orphaned.
    *   The requested tagging operation then proceeds on the new content hash (`H_CURRENT`).

*   **Performance Opt-In (`--use-metadata`):**
    *   All affected commands will gain a `--use-metadata` flag.
    *   When this flag is present, the system reverts to the faster, heuristic-based logic: it first checks if the file's `size + modification time` match the database record.
    *   Only if the metadata differs will it perform the "Always Hash" logic described above. This is faster for batch operations on unchanged files but carries a risk of missing content changes where the metadata did not update.

This "correctness by default, performance by choice" policy ensures that user actions are safe and predictable, while providing an explicit path for performance tuning.

### 4.3. Command-Specific Behavior

*   **`add`**
    *   **Operation:** Adds files to the database without applying tags. This is the canonical way to start tracking files. It uses the same powerful file ingestion logic as `tag` to handle new, moved, and duplicate content.

*   **`tag`**
    *   **Operation:** Additive. Requires at least one tag. `INSERT OR IGNORE` into `content_tags`. It never removes existing tags.

*   **`settags`**
    *   **Operation:** Declarative/Destructive. It first performs a `DELETE` on all existing tags for the content, then `INSERT`s the new tags. If no new tags are provided, this effectively clears all tags from the content.

*   **`untag`**
    *   **Operation:** Subtractive. It performs a `DELETE` on specific tags.

## 5. Edge Cases & Unresolved Questions

*   **What if a file has no tags, and the user runs `untag`?**
    *   **Decision:** This is a successful no-op. The command should succeed silently.
*   **What if a `settags` operation fails midway through a batch of 1000 files?**
    *   **Decision:** The entire operation is within a transaction. The transaction will be rolled back, and the database will be left as it was before the command was run. An error message will be displayed.
*   **How are permissions errors handled?**
    *   **Decision:** If a file cannot be read for hashing or stat-ing, an error for that specific file is reported via the progress callback. The operation continues for other valid files.

## 6. Alternatives

*   **Alternative: Differentiate behavior based on command risk.**
    *   **Description:** The previous version of this spec proposed that "safe" commands like `tag` would auto-update, while "unsafe" commands like `untag` would error out on a modified file.
    *   **Rejected:** This leads to inconsistent and unpredictable behavior for the user. A user should not have to remember which commands have which safety profile. The new unified approach is simpler, more transparent, and safer because it always informs the user of what happened and why.
*   **Alternative: Always error on mismatch.**
    *   **Description:** Never automatically re-hash and update a modified file.
    *   **Rejected:** Too restrictive. A user who intentionally edits and then tags a file would be forced to run a separate command in between, which is poor UX. The notification and warning system provides the necessary safety net.