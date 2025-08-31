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

This is a critical rule defining how the system handles a file path that points to content different from what the database has on record for that path. The behavior is now consistent across all tagging commands (`tag`, `settags`, `untag`).

If a file on disk has a different size or modification time than its database record, the system will **always** perform the following pre-flight steps before executing the requested command:

1.  **Re-hash:** The file is re-hashed to get its new content hash (`H_B`).
2.  **Update Location:** The `locations` table is updated to associate the file path with this new hash, size, and modification time. This action orphans the old content hash (`H_A`) which is no longer associated with this path. The orphaned hash and its associated tags remain in the database.
3.  **Notify User:** A message is printed to standard output informing the user that the file was updated. For example: `Updated database for modified file: 'path/to/file.txt'`.
4.  **Warn on Orphaned Tags:** After the update, the system checks if the orphaned hash (`H_A`) had any tags.
    *   If it did, an additional, prominent **WARNING** is printed. For example: `WARNING: 'path/to/file.txt' was modified. The old version's tags [tag1, tag2] are now orphaned. Run 'gooru relinkall' to find moved copies or 'gooru prune' to clean up.`
5.  **Proceed:** After these steps are complete for the modified file, the original command (`tag`, `settags`, or `untag`) proceeds as requested, operating on the **new** content hash (`H_B`).

This policy ensures that user actions always apply to the current state of the file on disk while preventing silent data loss by explicitly informing the user about orphaned tags.

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