# Spec: Information Retrieval Operations

**Version:** 1.0
**Status:** Implemented

---

## 1. Abstract

This document specifies the behavior of read-only commands such as `gettags`, `list`, `table`, and `listtags`. The guiding principle for these commands is that they must be fast, predictable, and report on the current known state of the database. They do not perform filesystem synchronization or content hashing.

## 2. Problem Statement / Motivation

Users need to query the Gooru database to find files and view tags. These operations are expected to be instantaneous. If a user has reorganized their files, read-only commands might provide seemingly incorrect or incomplete information (e.g., not finding a moved file). This can be confusing. The system needs a clear policy for how to behave in these situations without compromising performance, while guiding the user toward a solution.

## 3. Goals and Non-Goals

### Goals

*   Ensure all information retrieval commands are fast by default.
*   Define behavior that reports solely on the database's known state.
*   Provide helpful, actionable feedback to the user when a query for a specific file path fails due to filesystem changes (moves or modifications).

### Non-Goals

*   These commands will not perform content hashing.
*   These commands will not automatically update the database's `locations` table. That is the exclusive responsibility of write commands (`tag`, `settags`, `untag`) and the `relinkall` command.

## 4. Proposed Solution & Technical Design

Read-only commands are separated into two categories: path-based and expression-based.

### 4.1. Path-Based Retrieval (`gettags`)

The `gettags` command is unique because it takes a direct file path as input, creating an expectation of filesystem awareness. Its behavior is defined by these cases:

*   **Case 1: Path is in DB and file is unchanged.**
    *   **Mechanism:** A simple `SELECT` from the database based on the path.
    *   **Result:** The tags are returned instantly. This is the happy path.

*   **Case 2: Path is in DB, but file has been modified.**
    *   **Mechanism:** The command performs a metadata check (`os.Stat`). If size/modtime do not match the database record, it withholds the stale tag information.
    *   **Result:** No tags are returned. A helpful message is printed to stderr.
    *   **Example Output:** `Warning: 'path/to/file.txt' has been modified. Tags for the previous version are not shown. Please re-tag the file to update it.`

*   **Case 3: Path is NOT in DB.**
    *   **Mechanism:** The database lookup fails. The command then performs a check to see if the file exists on the filesystem.
    *   **Result (if file exists on disk):** No tags are returned. A helpful message is printed to the user.
    *   **Example Output:** `No tags found for 'path/to/file.txt'. To track this file (especially if it was moved or renamed), use: 'gooru add "path/to/file.txt"'`
    *   **Result (if file does not exist on disk):** A standard "No matching files found" or similar message is shown.

This design preserves the performance of `gettags` while actively guiding the user toward the correct synchronization workflow when discrepancies are found.

### 4.2. Expression-Based Retrieval (`list`, `table`, `listtags`)

These commands operate on the abstract data within the database (tags, content hashes) and do not take specific filesystem paths as direct inputs.

*   **Mechanism:** They parse the user's expression, build a SQL query, and execute it against the database. They have no direct interaction with the filesystem.
*   **Result:** They return a list of paths or a table of information as recorded *in the database at that moment*. If the filesystem has changed, the paths returned by `list` or `table` may be stale, which is the expected behavior. The user must run `relinkall` to synchronize these paths.