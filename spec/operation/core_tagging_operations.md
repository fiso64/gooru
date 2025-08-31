# Spec: Core Tagging Operations

**Version:** 1.0
**Status:** ???

---

## 1. Abstract

This document specifies the behavior of the primary database modification commands: `tag`, `settags`, and `untag`. These commands allow users to manage the relationship between content (identified by hash) and descriptive tags. The spec covers both path-based and expression-based operations and defines a clear, consistent policy for handling files that have been modified on disk.

## 2. Problem Statement / Motivation

The core utility of Gooru is applying and managing tags. Users need a clear, fast, and predictable way to perform the three fundamental tagging actions: adding tags, replacing all tags, and removing specific tags. The behavior of these commands must be safe and robust, especially when the filesystem's state has changed since the last operation.

## 3. Goals and Non-Goals

### Goals

*   Define the syntax and behavior for `tag` (additive), `settags` (declarative/destructive), and `untag` (subtractive).
*   Support both individual file paths and bulk operations via query expressions.
*   Establish a clear, safe, and predictable policy for handling files that have been modified on disk.
*   Ensure all database modifications are transactional.

### Non-Goals

*   This spec does not cover the query expression syntax itself, which is detailed in a separate document.
*   It does not cover the process of discovering moved/renamed files (see Filesystem Synchronization Spec).

## 4. Proposed Solution & Technical Design

The three commands share common logic for parsing arguments (files vs. expressions) and interacting with the database. Their primary distinction lies in the final database operation and, crucially, in their safety-check behavior.

### 4.1. Common Behavior

*   **Input Sources:** All three commands will support two modes of identifying target files, controlled by an `-e`/`--expression` flag.
    1.  **Path Mode (default):** The input is one or more file paths, globs, or directories.
    2.  **Expression Mode:** The input is a query expression that resolves to a set of content hashes.
*   **Transactional Integrity:** All database writes for a single command invocation must occur within a single transaction. If any part of the operation fails, the entire transaction is rolled back, leaving the database unchanged.

### 4.2. Behavior on File Modification (Path Mode Only)

This is the most critical rule. It defines how the system resolves the ambiguity of a file path pointing to content that is different from what the database has on record.

The behavior is determined by the **risk profile** of the command:

*   **Low-Risk (Declarative/Additive Actions): `tag`, `settags [tags...]`**
    *   **Action:** If a file on disk has a different size or modification time than its database record, the system will:
        1.  Re-hash the file to get its new content hash (`H_B`).
        2.  Update the `locations` table to associate the file path with this new hash. This orphans the old hash (`H_A`).
        3.  Apply the requested tags to the new hash (`H_B`).
    *   **Feedback:** The command will succeed but will print a clear warning at the end of the operation, informing the user that one or more files were modified and that old tag data may be orphaned. It will advise running `relinkall` to review.

*   **High-Risk (Corrective/Destructive Actions): `untag`, `settags` (with no tags)**
    *   **Action:** If a file on disk has a different size or modification time than its database record, the system will **halt the operation for that specific file.**
    *   **Feedback:** An error will be printed for the specific file (e.g., `"Failed to process 'file.txt': file has been modified; please re-tag it first"`). This prevents the user from accidentally performing a destructive action on the wrong content and ensures they are aware of the desynchronization.

### 4.3. Command-Specific Behavior

*   **`tag`**
    *   **Operation:** Additive. `INSERT OR IGNORE` into `content_tags`. It never removes existing tags.
    *   **Safety Profile:** Low-Risk.

*   **`settags`**
    *   **Operation:** Declarative/Destructive. It first performs a `DELETE` on all existing tags for the content, then `INSERT`s the new tags.
    *   **Safety Profile:**
        *   With tags provided: **Low-Risk**.
        *   With **no tags** provided (clearing all tags): **High-Risk**.

*   **`untag`**
    *   **Operation:** Subtractive. It performs a `DELETE` on specific tags.
    *   **Safety Profile:** **High-Risk**.

## 5. Edge Cases & Unresolved Questions

*   **What if a file has no tags, and the user runs `untag`?**
    *   **Decision:** This is a successful no-op. The command should succeed silently.
*   **What if a `settags` operation fails midway through a batch of 1000 files?**
    *   **Decision:** The entire operation is within a transaction. The transaction will be rolled back, and the database will be left as it was before the command was run. An error message will be displayed.
*   **How are permissions errors handled?**
    *   **Decision:** If a file cannot be read for hashing or stat-ing, an error for that specific file is reported via the progress callback. The operation continues for other valid files.

## 6. Alternatives

*   **Alternative: Always error on mismatch.**
    *   **Rejected:** Too restrictive. A user who intentionally edits and then tags a file would be forced to run a separate command in between, which is poor UX.
*   **Alternative: Prompt the user, and add --yes, --no.**