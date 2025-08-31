
Status: ???

To optimize performance, Gooru will occasionally use mod time and filesize instead of computing the file's hash.

### Case 1: `ModTime/Size` as a Heuristic for an **UNCHANGED** File

In these scenarios, a matching modification time (and size) is used as a shortcut to trust the database's information and **avoid expensive operations**.

1.  **Skipping Re-Hashing During Tagging (`TagFiles`, `SetTagsForFiles`)**
    *   **Context:** When you run the `tag` or `settags` command.
    *   **Mechanism:** Before adding a file to the list of files that need to be hashed, the system compares its current on-disk `size` and `ModTime` with the values stored in the database for that file path.
    *   **Result if Match:** If they match, the system assumes the file's content is identical to what's recorded. It skips reading and hashing the file entirely and reuses the existing content hash from the database.
    *   **Purpose:** This is a critical **performance optimization**. It avoids the significant I/O and CPU cost of re-hashing large files that have not been modified, making the tagging of existing files nearly instantaneous.

2.  **Skipping a Full Filesystem Scan (`NeedsRelink`)**
    *   **Context:** During the initial, fast pre-check phase of the `relinkall` command.
    *   **Mechanism:** The system queries the database for all known file paths within the specified directories. It then iterates through this list, performing a quick `stat` call on each file to get its current `size` and `ModTime`.
    *   **Result if All Match:** If every single file in the database exists on disk and its metadata matches the stored record, the `NeedsRelink` check returns `false`.
    *   **Purpose:** To determine that the database is perfectly synchronized with the filesystem **without performing a full scan**. This allows the `relinkall` command to finish immediately with an "up-to-date" message, providing a very fast user experience when no files have been moved or changed.

### Case 2: `ModTime/Size` as a Heuristic for a **CHANGED** File

In these scenarios, a mismatched modification time (or size) indicates that the file on disk is different from the one recorded in the database. This triggers a specific action to either update the database or prevent an unsafe operation.

1.  **Triggering Re-Hashing and Record Updates (`TagFiles`, `SetTagsForFiles`)**
    *   **Context:** When you run the `tag` or `settags` command.
    *   **Mechanism:** If a file's on-disk `size` or `ModTime` *does not* match what's in the database.
    *   **Result if Mismatch:** The system assumes the content has changed. It proceeds to re-read and re-hash the file to generate a new content hash. The database record for that file path is updated with this new hash and metadata *before* the new tags are applied.
    *   **Purpose:** To **ensure data integrity**. This guarantees that tags are always associated with the content that is *currently* present at a given file path, correctly handling file edits.

2.  **Preventing an `Untag` Operation as a Safety Measure (`UntagFiles`)**
    *   **Context:** When you run the `untag` command with specific tags to remove.
    *   **Mechanism:** If the file's on-disk `size` or `ModTime` does not match the database record.
    *   **Result if Mismatch:** The `untag` operation is **aborted for that specific file**, and an error is reported (e.g., "file has been modified"). The system refuses to remove tags from the old content record.
    *   **Purpose:** ???

3.  **Withholding Stale Information (`GetTagsForFile`, `GetFileInfoForFile`)**
    *   **Context:** When you ask for the tags of a single file using the `gettags` command.
    *   **Mechanism:** If the file's on-disk `size` or `ModTime` does not match the database record.
    *   **Result if Mismatch:** The system acts as if the file is not in the database and returns an empty list of tags.
    *   **Purpose:** To **prevent displaying misleading data**. The tags stored in the database belong to the *previous* version of the file's content. Showing them would be incorrect for the new, modified file.

4.  **Triggering a Full Filesystem Scan (`NeedsRelink`)**
    *   **Context:** During the initial pre-check of the `relinkall` command.
    *   **Mechanism:** If the metadata check finds *any* file whose on-disk `size` or `ModTime` does not match its database record, or if a file in the database is missing from disk.
    *   **Result if Mismatch:** The `NeedsRelink` check immediately returns `true`.
    *   **Purpose:** To signal that the database is out of sync with the filesystem. This result instructs the `relinkall` command to proceed with the more comprehensive and resource-intensive `Relink` scan to find out exactly what has moved, been renamed, or been deleted.