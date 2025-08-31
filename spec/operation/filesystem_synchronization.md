# Spec: Filesystem Synchronization

**Version:** 1.0
**Status:** Implemented

---

## 1. Abstract

This spec defines user-initiated processes for synchronizing the Gooru database with the filesystem. It covers the `relinkall` command, which performs bulk updates for entire directories, and the `editpath` command, which provides a surgical way to update the location of a single known file. These commands ensure that tags remain associated with their content even after files are moved, renamed, or deleted.

## 2. Problem Statement / Motivation

Because Gooru does not actively monitor the filesystem, the database can become stale when users move, rename, or delete files. A file renamed on disk will appear "untagged" because the database still has its old path on record, while a deleted file leaves an obsolete record.

*   **User Story (Bulk):** As a user, after I reorganize an entire project directory, I want a fast way to update the Gooru database to reflect all the new locations of my tagged content, so that I can find my files by their tags again.
*   **User Story (Surgical):** As a user, when I rename a single important file, I want to immediately tell Gooru its new name without scanning the whole directory, so that its record is updated instantly.

## 3. Goals and Non-Goals

### Goals

*   Provide a command (`relinkall`) for broad, directory-level synchronization.
*   Provide a command (`editpath`) for surgical, single-file path updates.
*   Correctly identify content that has been moved or renamed.
*   Identify new file paths that are duplicates of existing content.
*   Identify and propose the removal of database records for files deleted from disk.
*   Ensure the `relinkall` process is safe by proposing changes in a "dry run" and requiring user confirmation.
*   Ensure scans are highly performant by avoiding unnecessary file hashing.

### Non-Goals

*   `relinkall` will **not** add brand new, never-before-seen content to the database. Its purpose is to find existing content, not to perform an initial import. That is the job of the `tag` command.

## 4. Commands and Design

### 4.1 The `relinkall` Command (Bulk Synchronization)

The `relinkall` command is designed for synchronizing large directories where many changes may have occurred.

*   **User-Facing Changes:**
    *   Command: `relinkall <dir1> [dir2...]`.
    *   The command first performs a fast metadata check (`NeedsRelink`). If no changes are detected, it exits immediately, reporting that everything is up-to-date.
    *   If changes are likely, it performs a full scan and presents a summary of proposed changes:
        *   **[MOVED / RENAMED]:** Known content found at a new path, with the old path now empty.
        *   **[NEW LOCATIONS / DUPLICATES]:** Known content found at an additional path.
        *   **[DELETED FROM DISK]:** Paths in the database that no longer exist on the filesystem. This is how obsolete records are pruned.
    *   It prompts the user for confirmation `[Y/n]` before applying any changes. Flags `-y`/`--yes` and `-n`/`--no` can be used for scripting.

*   **Internal Logic (The Scan):**
    1.  **Pre-computation:** Fetch all known file sizes from the DB into a `sizeToHashes` map. This is a critical optimization.
    2.  **Filesystem Walk:** Concurrently walk the target directories.
    3.  **Filtering:** For each file found on disk, check its size. If the size does not exist as a key in the `sizeToHashes` map, the file cannot possibly be a known piece of content. Skip it immediately without reading its content.
    4.  **Targeted Hashing:** Only if a file's size matches a known size, compute its hash using the fast partial hashing method (as performance is paramount for this discovery operation).
    5.  **Reconciliation:** Compare the set of known locations from the DB with the set of locations found on the filesystem to generate the lists of moves, adds (duplicates), and deletes.

*   **Internal Logic (Applying Changes):**
    1.  **Moves:** Perform `UPDATE locations SET path = ? WHERE path = ?`.
    2.  **Adds:** Perform `INSERT INTO locations (...)`.
    3.  **Deletes:** Perform `DELETE FROM locations WHERE path IN (...)`.
    4.  All database modifications occur within a single transaction for safety.

### 4.2 The `editpath` Command (Surgical Update)

The `editpath` command is for the simple case where a user knows a single file has been moved or renamed and wants to update its path directly.

*   **User-Facing Changes:**
    *   Command: `editpath <oldpath> <newpath>`.
    *   Provides a simple success message upon completion.

*   **Internal Logic:**
    1.  The command takes two arguments, `oldpath` and `newpath`.
    2.  It performs a direct `UPDATE locations SET path = ? WHERE path = ?` query.
    3.  It includes safety checks to ensure the `oldpath` exists in the database and the `newpath` does not, preventing accidental overwrites of other tracked files.
    4.  The operation is atomic.

### 4.3 The `add` Command (Implicit Surgical Relink)

In addition to the explicit `editpath` command, the `add` command (and by extension, `tag`) also serves as a powerful and intuitive tool for surgical relinking.

*   **User-Facing Behavior:**
    *   Command: `add <newpath>`
    *   When a user `add`s a file that was moved or renamed, the system intelligently detects the move and updates its internal record instead of creating a new one. The user only needs to know the file's current location.

*   **Internal Logic:**
    1.  When `add <newpath>` is run, the system hashes the file at `newpath`.
    2.  It discovers this hash already exists in the database, associated with `<oldpath>`.
    3.  It checks the filesystem and finds that `<oldpath>` no longer exists.
    4.  It concludes this is a move/rename and atomically updates the `locations` table, changing the path from `<oldpath>` to `<newpath>`.
    5.  This makes `add` a convenient alternative to `editpath`, as it doesn't require the user to remember the file's original path.

## 5. Edge Cases & Unresolved Questions

*   **What if a file is modified in-place and also moved?**
    *   **Decision:** The scan identifies content by hash. A modified file is new content. `relinkall` will see this as one deletion (the old path/hash) and will ignore the new file because it's new content. This is correct behavior, as `relinkall` is not supposed to add new content.
*   **What happens if a user moves a file outside of the scanned directories?**
    *   **Decision:** The scan will report the file as deleted from its original location. This is correct from the perspective of the scanned directories. The user would need to scan the new parent directory to "find" it again.
*   **What happens if content is duplicated and then one copy is deleted?**
    *   **Decision:** The `relinkall` command will simply propose deleting the location record for the deleted path. The other location record remains, and the content and its tags are preserved. This is correct.

## 6. Alternatives Considered

*   **Alternative: Brute-force re-hashing:** We could re-hash every single file in the target directories and compare it to every hash in the database.
    *   **Rejected:** This would be unacceptably slow for large directories. The size-based filtering optimization is essential for meeting the performance goals.