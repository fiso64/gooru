# Spec: File Identity and Change Detection

**Version:** 2.0
**Status:** Proposed

---

## 1. Abstract

This document specifies Gooru's core philosophy for identifying files and detecting changes, establishing a unified and robust policy for all **path-based operations**. To guarantee data integrity and eliminate user confusion from misleading information, any command that accepts a direct filesystem path (e.g., `tag`, `gettags`, `add`) will now **always perform a content hash** to definitively verify the file's identity. This ensures that user actions and queries always pertain to the file currently present on disk. This v2 spec revises a previous, flawed heuristic-based approach to solve a critical data integrity and trust issue that affected both read and write commands.

## 2. Revision History & Motivation (From v1 to v2)

This section details the evolution of Gooru's change detection mechanism and the critical reasoning that led to this revised specification.

### 2.1. The Initial Design (v1 Heuristics)

The original design prioritized raw performance above all, using a fast but unreliable metadata heuristic (`file size + modification time`) to detect file changes. The system's reaction to a mismatch was inconsistent across commands, creating a complex and unpredictable user experience.

### 2.2. The Flaw Discovered

The initial design accepted the theoretical imperfections of the `size + mod_time` heuristic as a pragmatic trade-off for performance. It was believed to be "good enough" for typical interactive command-line use.

However, testing revealed that this assumption was dangerously incorrect. The heuristic proved to be completely unreliable in practice, even on modern filesystems like NTFS with high-resolution timestamps. The failing test case (`tagging_a_modified_file_updates_record_and_orphans_old_tags`) demonstrated that it was trivial to create a scenario where a file's content changed but its metadata did not update in a way the heuristic could detect, especially when file modifications were performed programmatically or in rapid succession.

*   **The Problem:** The key discovery was not merely that the heuristic *could* fail, but that it *would* fail frequently and unpredictably in automated contexts. Its reliability was far lower than initially assumed, making it unsuitable for any use case beyond casual, manual interaction.
*   **The Consequence:** This unpredictability exposed two critical failure modes:
    1.  **Write Corruption:** A `tag` operation within a script could silently apply tags to the wrong version of a file's content.
    2.  **Misleading Reads:** A `gettags` call from a library or script could receive stale data, leading to incorrect downstream logic.

The conclusion was stark: a heuristic that is only safe for manual use is not safe at all for a tool designed to be a reliable building block. For Gooru to be trustworthy as both a CLI tool and a library, its core mechanism for identifying content at a given path had to be infallible.

### 2.3. Re-evaluation and the New Philosophy (v2)

The discovery of this flaw forced a re-evaluation. The core user expectation for any command like `gooru [verb] /path/to/file.txt` is that the operation pertains to the file *currently at that path*. The metadata heuristic is an insufficient proxy for this reality.

The new philosophy is therefore simple and absolute:

**To know the content, you must read the content.**

The introduction of fast partial hashing makes this philosophy practical. The small performance cost of hashing is an acceptable price for complete data integrity and user trust.

## 3. Goals and Non-Goals

### Goals

*   **Guarantee Correctness for All Path-Based Operations:** Ensure that any command operating on a direct file path is acting upon or reporting about the file's current content, never a stale record.
*   **Eliminate Misleading Information:** A user must be able to trust the output of `gettags` as the ground truth for the file on their disk.
*   **Establish a Simple, Predictable System:** The file change detection mechanism should be unified and consistent across all relevant commands.

### Non-Goals

*   This policy does **not** apply to expression-based queries (`gooru list "tag:X"`). Those commands correctly query the abstract database state, and the paths they return are understood to be the last known locations.
*   The `gettags` command is no longer guaranteed to be "instantaneous" as it now involves file I/O. We are explicitly choosing correctness over maximum possible speed.

## 4. Proposed Solution & Technical Design

The new policy is unified across all commands that take a direct filesystem path as input.

### The Unified Path-Based Operation Policy

*   **Affected Commands:** `tag`, `settags`, `untag`, `add`, `rehash`, `gettags`, and any future commands that accept a direct file path.
*   **Mechanism:**
    1.  When one of these commands is invoked with a file path, the system will **always** perform a content hash on the file at that path to get its definitive, current content hash (`H_CURRENT`).
    2.  The system will look up the given path in the `locations` table to retrieve its last known content hash (`H_DB`), if one exists.
    3.  The system then compares the hashes:
        *   **If `H_CURRENT` == `H_DB`:** The file is unchanged. The operation proceeds normally. `gettags` will return the known tags.
        *   **If `H_CURRENT` != `H_DB`:** The file has been modified. The system performs the **Safe Update Path**:
            a. **For Write Operations (`tag`, `add`, etc.):** The `locations` table record for the path is updated to point to `H_CURRENT`. The user is notified of the update and warned about any orphaned tags from `H_DB`. The write operation then proceeds on `H_CURRENT`.
            b. **For Read Operations (`gettags`):** The system now knows the tags associated with `H_DB` are for a different file. It returns a `StatusModified` state. The CLI will use this to inform the user that the file has changed and has no tags associated with its *new* content.
                *   **Example Output:** `Warning: 'path/to/file.txt' has been modified. It has no tags in its current state. The previous version had tags [...]. Run 'gooru rehash' to transfer them.`
        *   **If Path is Not in DB:** The file is new to Gooru. Write operations will create a new record for it. `gettags` will report that the file is not tracked and has no tags.

This unified approach ensures that every interaction related to a specific file path is grounded in the reality of that file's current content, completely eliminating the risk of data corruption or user deception.

## 5. Impact on Existing Specs

This v2 specification supersedes previous documents regarding file change detection:

*   **`spec/operation/file_change_heuristics.md`**: This document is now **obsolete** and should be deprecated or removed. Its contents are replaced by this spec.
*   **`spec/operation/core_tagging_operations.md`**: Section 4.2 ("Behavior on File Modification") must be updated to reflect the new "always hash" policy.
*   **`spec/operation/information_retrieval.md`**: The behavior described for `gettags` must be fundamentally updated to reflect that it now performs hashing and can definitively report on a file's modified status.