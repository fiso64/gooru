# Spec: Filesystem Synchronization

**Version:** 1.0
**Status:** TODO: Complete this spec (add editpath, prune)

---

## 1. Abstract

This spec defines the `relinkall` command, a user-initiated process for bulk synchronization between the Gooru database and the filesystem. It is designed to find all moved, renamed, and deleted files within a large directory where the specific changes may be unknown to the user. It complements the more surgical `add` command by handling these broad, directory-level changes.

## 2. Problem Statement / Motivation

Because Gooru does not actively monitor the filesystem, the database can become stale when users move, rename, or delete files. A file renamed on disk will appear "untagged" because the database still has its old path on record. A deleted file leaves an obsolete record in the database.

*   **User Story:** As a user, after I reorganize my files, I want a fast way to update the Gooru database to reflect the new locations of my tagged content, so that I can find my files by their tags again.

## 3. Goals and Non-Goals

### Goals

*   Provide a command (`relinkall`) to scan directories and find changes.
*   The command must correctly identify content that has been moved or renamed.
*   The command must identify new file paths that are duplicates of existing content.
*   The command must identify database records for files that have been deleted from disk.
*   The process must be safe, first proposing changes in a "dry run" and requiring user confirmation before modifying the database.
*   The scan must be highly performant, avoiding re-hashing files whenever possible.

### Non-Goals

*   `relinkall` will **not** add brand new, never-before-seen content to the database. Its purpose is to find existing content, not to perform an initial import. That is the job of the `tag` command.

## 4. Proposed Solution & Technical Design

*   **User-Facing Changes:**
    *   A new command: `relinkall <dir1> [dir2...]`.
    *   The command will first perform a fast metadata check (`NeedsRelink`). If no changes are detected, it will exit immediately.
    *   If changes are likely, it will perform a full scan and present a summary of proposed changes (Moves, Adds, Deletes).
    *   It will prompt the user for confirmation `[Y/n]` before applying any changes. Flags `-y`/`--yes` and `-n`/`--no` can be used for scripting.

*   **Internal Logic (The Scan):**
    1.  **Pre-computation:** Fetch all known file sizes from the DB into a `sizeToHashes` map. This is a critical optimization.
    2.  **Filesystem Walk:** Concurrently walk the target directories.
    3.  **Filtering:** For each file found on disk, check its size. If the size does not exist as a key in the `sizeToHashes` map, the file cannot possibly be a known piece of content. Skip it immediately without reading its content.
    4.  **Targeted Hashing:** Only if a file's size matches a known size, compute its hash.
    5.  **Reconciliation:** Compare the set of known locations from the DB with the set of locations found on the filesystem to generate the lists of moves, adds (duplicates), and deletes.

*   **Internal Logic (Applying Changes):**
    1.  **Moves:** Perform `UPDATE locations SET path = ? WHERE path = ?`.
    2.  **Adds:** Perform `INSERT INTO locations (...)`.
    3.  **Deletes:** Perform `DELETE FROM locations WHERE path IN (...)`.
    4.  All database modifications must occur within a single transaction.

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