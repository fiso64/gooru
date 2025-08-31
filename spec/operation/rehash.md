# Spec: The `rehash` Command

**Version:** 1.0
**Status:** Proposed

---

## 1. Abstract

This document specifies the `rehash` command, a user-initiated operation to explicitly update a file's record in the database. When a file known to Gooru is modified on disk, this command calculates its new content hash and transactionally transfers all existing tags from the old content record to the new one, effectively preserving the file's tagged identity.

## 2. Problem Statement / Motivation

Currently, when a tagged file is modified, the `tag` command correctly identifies this and creates a new content record, but it orphans the old content and its associated tags. The user has no direct way to say, "This new version of the file should inherit all the tags from the old version." This is especially problematic for heavily tagged documents, project files, or creative works that undergo frequent revision.

Furthermore, while `relinkall` is powerful for synchronizing entire directories, it is too broad and slow for updating a single known file that has been modified.

*   **User Story 1:** As a user, when I significantly edit a file that has many tags, I want to update its record in Gooru and keep all its tags, so I don't have to manually re-apply them.
*   **User Story 2:** As a user, I want a fast, targeted way to update the database for a few specific files without running a full `relinkall` scan on a large directory.

## 3. Goals and Non-Goals

### Goals

*   Provide a new command, `rehash`, that accepts file paths, globs, or directories.
*   For each specified path that exists in the database, update its record (hash, size, modtime) to match the current on-disk file.
*   Atomically **transfer** all tags from the old content hash to the new content hash.
*   Clean up the old, now-obsolete content record from the database.
*   Provide clear, explicit feedback to the user about which files were successfully re-hashed and which were skipped.
*   All database modifications must be transactional for each file.

### Non-Goals

*   This command will **not** add new, previously unknown files to the database. It only operates on paths that are already tracked.
*   This command will **not** automatically discover moved or renamed files. Its scope is limited to updating content at known, existing paths. Discovering moved content remains the exclusive job of `relinkall` and the automatic detection in write-commands.

## 4. Proposed Solution & Technical Design

*   **User-Facing Changes:**
    *   A new command: `rehash <file/dir/glob...>`
    *   Optional flags for progress indication, e.g., `-p`/`--progress`.

*   **Internal Logic:**
    1.  Expand the input arguments into a list of file paths.
    2.  For each file path, perform the following steps:
        a. Check if the path exists in the `locations` table. If not, skip it and notify the user.
        b. If it exists, retrieve its database record (old hash, old size, old modtime).
        c. Check if the file exists on the filesystem. If not, report an error and skip.
        d. Compare the on-disk metadata with the database record. If they match, the file is unchanged. Skip it and notify the user.
        e. If the metadata differs, the file has been modified. Proceed with the rehash transaction:
            i.   Begin a transaction.
            ii.  Calculate the new content hash (`H_B`) of the on-disk file.
            iii. **Handle content collision:** Check if `H_B` already exists in the `contents` table.
                 *   If YES (the modified file is now a duplicate of other content), all tags from the old hash (`H_A`) will be merged (`INSERT OR IGNORE`) with the tags of `H_B`.
                 *   If NO, insert `H_B` into the `contents` table.
            iv.  Update the `locations` table entry for the file path to point to `H_B` and its new metadata.
            v.   If `H_B` did *not* previously exist, transfer the tags: `UPDATE content_tags SET content_hash = ? WHERE content_hash = ?` (new hash, old hash).
            vi.  Delete the old content record (`H_A`) from the `contents` table. This should cascade-delete its `content_tags` entries.
            vii. Commit the transaction.
            viii. Report success to the user.

### Example Walkthroughs

**Example 1: The "Happy Path" - Modifying a File**
```bash
# Initial State: 'report.docx' is in the DB with hash H_A and tags [project, draft].
$ gooru gettags report.docx
project
draft

# User edits and saves the file.

$ gooru rehash report.docx
# Expected Output:
Rehashed 'C:\path\to\report.docx' (updated content record)

# Check the tags again. They have been preserved.
$ gooru gettags report.docx
project
draft
```

**Example 2: Unchanged and Unknown Files**
```bash
# 'up-to-date.txt' is in the DB and unchanged. 'new-file.txt' is not in the DB.
$ gooru rehash up-to-date.txt new-file.txt
# Expected Output:
Skipped 'C:\path\to\up-to-date.txt': file is already up-to-date.
Skipped 'C:\path\to\new-file.txt': path not found in database.
```

## 5. Edge Cases & Unresolved Questions

*   **What if the file is unchanged?**
    *   **Decision:** The command will perform a fast metadata check, see that nothing has changed, and skip the file with a notification. No hashing will occur.
*   **What if the path is not in the database?**
    *   **Decision:** The command will skip the file with a notification. It will not add it.
*   **What if the file is in the DB but has been deleted from disk?**
    *   **Decision:** The command will report an error for that specific file (e.g., "file not found on disk").
*   **What if the rehashed file's new content is identical to another, different file already in the database?**
    *   **Decision:** The logic in 4.2.e.iii handles this. The tags from the old content will be merged with the existing content's tags. The file's path will be updated to point to the existing content hash, and the old, now-redundant content hash will be deleted. This correctly consolidates duplicate content.

## 6. Alternatives Considered

*   **Overloading `tag` with a `--rehash` flag:**
    *   **Rejected:** This confuses the purpose of the commands. `tag` is about *adding information*, while `rehash` is about *managing content identity*. A separate command is cleaner and more explicit.
*   **Automatically finding moves:**
    *   **Rejected:** This would make `rehash` a slow, scanning command, conflating its purpose with `relinkall`. Keeping `rehash` focused on updating known paths makes it a fast, surgical tool that complements the broader `relinkall` command.