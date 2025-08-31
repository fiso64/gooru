# Spec: File Change Heuristics

**Version:** 1.0
**Status:** Implemented

---

## 1. Abstract

To optimize performance and ensure data integrity, Gooru uses file metadata (size and modification time) as a heuristic to quickly determine if a file's content has changed since it was last recorded in the database. This document specifies how different commands react to this heuristic, distinguishing between a "fast path" for unchanged files and a "safe update path" for changed files.

## 2. Scenario 1: On-Disk Metadata MATCHES Database Record

This is the **fast path**. When a file's current size and modification time are identical to the values stored in the database for its path, the system assumes the content is unchanged. This allows it to avoid expensive operations.

*   **Operations Affected:**
    *   `tag`, `settags`, `untag`: The system reuses the known content hash from the database without reading or re-hashing the file. This makes tagging operations on existing, unchanged files nearly instantaneous.
    *   `gettags`: The system confidently returns the cached tags associated with the known content hash.
    *   `relinkall` (pre-check): The `NeedsRelink` check passes for this file. If all files pass, the entire `relinkall` command can exit immediately, reporting that the database is synchronized.

*   **Purpose:** Performance optimization. Avoids redundant I/O and CPU work.

## 3. Scenario 2: On-Disk Metadata DOES NOT MATCH Database Record

This is the **safe update path**. When a file's size or modification time differs from the database record, the system assumes the content has changed and that the database record for that path is stale.

The system's reaction depends on the nature of the command.

### Behavior A: Update and Proceed

This behavior ensures that user actions always apply to the *current* state of the file on disk.

*   **Operations Affected:**
    *   `tag`, `settags`, `untag`

*   **Mechanism:**
    1.  The system assumes the content has changed.
    2.  It re-reads and re-hashes the file to generate a new content hash.
    3.  It updates the database record for that file path, linking it to the new hash and metadata.
    4.  The user is notified that the database was updated for a modified file. If the old content had tags, a warning about the orphaned tags is also displayed.
    5.  The original tagging operation then proceeds on the new content hash.

*   **Purpose:** Data integrity and a seamless user experience. Tags are always associated with the content that is currently present at a given file path.

### Behavior B: Withhold Stale Information

This behavior prevents the system from displaying misleading information to the user.

*   **Operations Affected:**
    *   `gettags`, `GetFileInfoForFile`

*   **Mechanism:**
    1.  The system sees that the on-disk file does not match the database record.
    2.  It concludes that the tags stored in the database belong to the *previous* version of the file and are not valid for the current content.
    3.  It acts as if the file is not in the database, returning an empty list of tags.

*   **Purpose:** Accuracy. Avoids showing tags that belong to old, different content.

### Behavior C: Trigger Full Scan

This behavior signals that a broader synchronization is needed.

*   **Operations Affected:**
    *   `relinkall` (pre-check)

*   **Mechanism:**
    1.  The `NeedsRelink` check finds a file whose metadata does not match its database record (or a file that is missing entirely).
    2.  It immediately returns `true`.

*   **Purpose:** To confirm that the database is out of sync with the filesystem, instructing the `relinkall` command to proceed with its more comprehensive scan to find all moves, duplicates, and deletions.