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

The `gettags` command takes a direct file path, creating an expectation of filesystem awareness. Its behavior is defined by the new "correctness by default" policy.

*   **Default Behavior ("Always Hash"):**
    *   **Mechanism:** The command will hash the content of the file at the given path and compare it to the hash stored in the database for that path.
    *   **Result (Hashes Match):** The tags are returned.
    *   **Result (Hashes Differ):** The file has been modified. The command returns no tags and prints a warning that the file has changed.
    *   **Result (Path Not in DB):** The command returns no tags and provides a helpful message guiding the user to use `gooru add`.

*   **Performance Opt-In (`--use-metadata`):**
    *   The `gettags` command has a `--use-metadata` flag.
    *   When present, it reverts to the faster, heuristic-based logic of checking `size + modification time` instead of hashing the file to determine if it has been modified.

### 4.2. Expression-Based Retrieval (`list`, `table`, `listtags`)

These commands operate on the abstract data within the database (tags, content hashes) and do not take specific filesystem paths as direct inputs.

*   **Mechanism:** They parse the user's expression, build a SQL query, and execute it against the database. They have no direct interaction with the filesystem.
*   **Result:** They return a list of paths or a table of information as recorded *in the database at that moment*. If the filesystem has changed, the paths returned by `list` or `table` may be stale, which is the expected behavior. The user must run `relinkall` to synchronize these paths.