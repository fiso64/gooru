
# Spec: File Identity and Change Detection

**Version:** 2.0
**Status:** Implemented

---

## 1. Abstract

This document specifies Gooru's core philosophy for identifying files and detecting changes, establishing a unified and robust policy for all **path-based operations**. To guarantee data integrity, the default behavior for any command that accepts a direct filesystem path (e.g., `tag`, `gettags`) will now be to **always perform a content hash** to definitively verify a file's identity. This ensures user actions and queries always pertain to the file currently present on disk.

Recognizing the performance implications, this policy introduces an explicit, user-controlled override. Affected commands will accept a `--use-metadata` flag, and corresponding library methods will accept a parameter, to revert to the previous, faster heuristic of trusting file `size + modification time`.

Furthermore, this spec acknowledges that the `relinkall` command's pre-check is also affected. For performance reasons, it will **retain the metadata heuristic as its default behavior**, but will gain a new `--always-verify-hash` flag to enable a slower, 100% correct pre-scan when absolute certainty is required.

## 2. Revision History & Motivation (From v1 to v2)

This section details the evolution of Gooru's change detection mechanism and the critical reasoning that led to this revised specification.

### 2.1. The Initial Design (v1 Heuristics)

The original design prioritized raw performance, using a fast but unreliable metadata heuristic (`file size + modification time`) to detect file changes.

### 2.2. The Flaw Discovered

The initial design accepted the theoretical imperfections of the `size + mod_time` heuristic as a pragmatic trade-off for performance. It was believed to be "good enough" for typical interactive command-line use.

However, testing revealed that this assumption was dangerously incorrect. The heuristic proved to be unpredictable in practice, even on modern filesystems like NTFS with high-resolution timestamps. The failing test case (`tagging_a_modified_file_updates_record_and_orphans_old_tags`) demonstrated that it was trivial to create a scenario where a file's content changed but its metadata did not update in a way the heuristic could detect, especially when file modifications were performed programmatically or in rapid succession.

*   **The Problem:** The heuristic's unreliability exposed two critical failure modes:
    1.  **Write Corruption:** A `tag` operation could silently apply tags to the wrong version of a file's content.
    2.  **Misleading Reads:** A `gettags` call could receive stale tag data, leading to incorrect downstream logic.

The conclusion was stark: a heuristic that is only safe for casual, manual use is not safe at all for a tool designed to be a reliable building block.

### 2.3. Re-evaluation and the New Philosophy (v2)

The discovery of this flaw forced a re-evaluation. The core user expectation for any command like `gooru [verb] /path/to/file.txt` is that the operation pertains to the file *currently at that path*. The metadata heuristic is an insufficient proxy for this reality.

However, completely discarding the heuristic removes a powerful performance optimization that is acceptable in many contexts. The new philosophy is therefore:

**Correctness by Default, Performance by Choice.**

This strikes a balance: the default behavior is safe and infallible, but users who understand the trade-offs can explicitly opt in to the higher-performance, lower-guarantee mode.

## 3. Goals and Non-Goals

### Goals

*   **Guarantee Correctness by Default:** Make "always hash" the default behavior for all standard path-based commands to eliminate data corruption and misleading reads.
*   **Provide an Explicit Performance Opt-In:** Introduce a `--use-metadata` flag for path-based commands to allow users to revert to the faster, heuristic-based logic.
*   **Maintain `relinkall` Performance:** Keep the metadata heuristic as the default for `relinkall`'s pre-check to ensure it remains fast for large directories.
*   **Provide a High-Integrity `relinkall` Option:** Add an `--always-verify-hash` flag to `relinkall` for users who require a 100% correct (but slower) synchronization scan.

### Non-Goals

*   This policy does **not** apply to expression-based queries (`gooru list "tag:X"`). Those commands correctly query the abstract database state.
*   The `gettags` command is no longer guaranteed to be "instantaneous" by default, as it now involves file I/O. Correctness is prioritized over maximum possible speed in the default case.

## 4. Proposed Solution & Technical Design

The new policy is unified across all commands that take a direct filesystem path as input, with a special case for `relinkall`.

### 4.1. The Unified Path-Based Operation Policy

*   **Affected Commands:** `tag`, `settags`, `untag`, `add`, `rehash`, `gettags`, and any future commands that accept a direct file path.
*   **Default Mechanism ("Always Hash"):**
    1.  When a command is invoked with a file path, the system will **always** perform a content hash on the file at that path to get its definitive, current content hash (`H_CURRENT`).
    2.  The system will look up the given path in the `locations` table to retrieve its last known content hash (`H_DB`), if one exists.
    3.  The system then compares the hashes:
        *   **If `H_CURRENT` == `H_DB`:** The file is unchanged. The operation proceeds normally.
        *   **If `H_CURRENT` != `H_DB`:** The file has been modified. The system performs the **Safe Update Path**:
            *   **For Write Operations (`tag`, `add`, etc.):** The `locations` table record for the path is updated to point to `H_CURRENT`. The user is notified of the update and warned about any orphaned tags from `H_DB`. The write operation then proceeds on `H_CURRENT`.
            *   **For Read Operations (`gettags`):** The system returns a `StatusModified` state. The CLI uses this to inform the user that the file has changed and its current content has no tags.
        *   **If Path is Not in DB:** The file is new. Write operations create a new record. Read operations report the file as not tracked.

*   **Performance Opt-In (`--use-metadata`):**
    *   **CLI:** All affected commands will gain a `--use-metadata` flag. When present, the command will revert to the v1 behavior: check `size + mod_time` first, and only perform the "Always Hash" logic if the metadata differs.
    *   **Library:** Corresponding public methods in the `gooru` library will accept a boolean parameter (e.g., `useMetadataHeuristic`) to control this behavior.

### 4.2. The `relinkall` Command Policy

The `relinkall` command is a special case due to its performance-sensitive nature.

*   **Default Behavior (Metadata Heuristic):**
    *   The initial `NeedsRelink` check will continue to use the `size + mod_time` heuristic by default. If no metadata mismatches are found for known files, it will exit early, reporting that the database is synchronized. This preserves its speed for typical use.

*   **High-Integrity Opt-In (`--always-verify-hash`):**
    *   **CLI:** The `relinkall` command will gain a new `--always-verify-hash` flag.
    *   **Mechanism:** When this flag is present, the initial `NeedsRelink` check is modified. Instead of just checking metadata, it will perform a content hash for every file in the database that still exists on disk to compare its current hash against the database record. This is significantly slower but guarantees detection of content changes that did not alter file metadata. The full `Relink` scan proceeds only if a discrepancy is found.

This hybrid approach makes Gooru safe by default, while providing the necessary levers for users to tune performance based on their specific needs and risk tolerance.

## 5. Impact on Existing Specs

This v2 specification supersedes previous documents regarding file change detection:

*   **`spec/operation/file_change_heuristics.md`**: This document is now **obsolete** and should be deprecated or removed. Its contents are replaced by this spec.
*   **`spec/operation/core_tagging_operations.md`**: Section 4.2 ("Behavior on File Modification") must be updated to reflect the new "always hash" default and the existence of the `--use-metadata` flag.
*   **`spec/operation/information_retrieval.md`**: The behavior of `gettags` must be updated to reflect that it now performs hashing by default and has a `--use-metadata` flag.
*   **`spec/operation/filesystem_synchronization.md`**: The `relinkall` section must be updated to describe the new `--always-verify-hash` flag and clarify that the default remains the metadata heuristic.