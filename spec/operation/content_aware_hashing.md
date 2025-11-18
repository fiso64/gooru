# Spec: Content-Aware Hashing

**Version:** 2.0
**Status:** Proposed

---

## 1. Abstract

This document specifies a new, optional "content-aware" hashing capability for Gooru. This feature modifies the hashing process for supported media file formats (e.g., FLAC, MP3, JPEG) to intelligently ignore metadata sections, generating a hash based only on the file's core audio or visual content. This ensures a file's identity remains stable even when its internal metadata is modified. The feature is designed to be highly flexible and safe, allowing users to enable or disable it on-demand for specific file types via a new `gooru config` command. To prevent logical data corruption, the system enforces a strict consistency model, blocking certain operations and persistently warning the user until a required re-hashing migration is performed.

## 2. Problem Statement / Motivation

Gooru's primary strength is its content-centric identity model. However, for many common media files, "content" is ambiguous. When a user retags an MP3 file using an external editor, the file's bytes change, leading Gooru to generate a new hash. From Gooru's perspective, this is a completely new file, orphaning the old content record and all its associated organizational tags.

This behavior is technically correct but practically undesirable for media management. The user's intent is to modify information *about* the content, not the content itself.

*   **User Story:** As a music enthusiast, when I use a tool to add album art or fix the "Year" tag on my FLAC files, I want Gooru to recognize that the underlying audio has not changed, so that I don't lose the personal organizational tags (like `rating:5` or `playlist:workout`) that I've applied within Gooru.

## 3. Goals and Non-Goals

### Goals

*   Introduce a new, optional content-aware hashing system.
*   Initially support common, easily-parsable formats: FLAC, MP3, JPEG, Opus.
*   Provide a new, future-proof command, `gooru config content-aware`, for users to view and toggle settings for an existing database.
*   When enabling or disabling a format, **prompt** the user to perform the necessary re-hashing of existing files.
*   If the user declines the re-hash, put the database into a safe **"degraded mode"** for the affected formats, which includes persistent warnings and refusal of write operations to maintain consistency.
*   Provide a robust, versioned upgrade path (`gooru rehash --upgrade`) to apply new or improved hashing logic.
*   Store all configuration and versioning information transparently within the database's `meta` table.

### Non-Goals

*   This feature will **not** be enabled by default. Users must opt-in.
*   Initial implementation will **not** support complex container formats like MP4 or MKV.
*   The system will **not** automatically re-hash the database when the application is updated; upgrades are always user-initiated via `rehash --upgrade`.

## 4. Proposed Solution & Technical Design

The solution is a flexible, versioned system that enforces logical consistency at all times.

### 4.1. User-Facing Changes

#### 4.1.1. The `gooru init` Command

The initialization process is updated to introduce the feature:
1.  After the primary hashing strategy (`partial`/`full`) is chosen, the user will be asked:
    > Gooru can intelligently ignore metadata in supported media files (like FLAC, MP3, JPEG) to keep tags stable when metadata changes. This is highly recommended for media libraries.
    >
    > Would you like to enable this feature for supported file types? (Y/n)
2.  If 'yes', the relevant `_enabled` flags are set to `1` in the database `meta` table.

#### 4.1.2. The `gooru config` Command Group

A new, extensible command group for managing database configuration.

*   `gooru config content-aware`: Displays the current content-aware hashing settings.
    ```bash
    # Example Output:
    Content-Aware Hashing Settings:
    
    EXTENSION    ENABLED    LOGIC_VERSION
    .flac        true       1
    .mp3         true       1
    .jpg         true       1
    .opus        false      1
    ```

*   `gooru config content-aware enable <ext1> [ext2...]`: Enables content-aware hashing for the given extension(s).
*   `gooru config content-aware enable all`: Enables content-aware hashing for all supported extensions.
*   `gooru config content-aware disable <ext1> [ext2...]`: Disables the feature, reverting to the database's default hashing strategy.

For both `enable` and `disable`, the system will perform a quick database query to see how many existing files will be affected and then prompt the user for confirmation:

```bash
This change requires 1,234 '.flac' files to be re-hashed. This may take some time.
If you decline, Gooru will operate in a degraded mode for .flac files until they are re-hashed.
Proceed with re-hashing now? [Y/n]:
```
*   If **'Y'**: The command proceeds with the re-hash operation for the affected files and then exits.
*   If **'n'**: The command updates the configuration in the database but does not re-hash. The database is now in an inconsistent state for that format, triggering the "Degraded Mode" (see below).

#### 4.1.3. The `gooru rehash --upgrade` Command

This command is the unified tool for applying hashing logic updates, whether from a version change or a manual config change.
*   It compares the application's internal logic versions against the database's stored versions.
*   It also checks for any formats that have been enabled/disabled but not yet migrated.
*   It then builds a list of all files that need updating, re-hashes them, and updates their content records and the `meta` table versions.

### 4.2. The Consistency Model: "Degraded Mode"

An inconsistent or "mixed" state—where some files of a given type use one hashing logic and others use another—is fundamentally broken and must be prevented. "Degraded Mode" is the mechanism to enforce this.

*   **Trigger:** On every command invocation, Gooru checks its configuration. Degraded mode is triggered for a format (e.g., `.flac`) if:
    1.  The application's logic version for `.flac` is newer than the database's stored version.
    2.  The user has run `config content-aware enable/disable .flac` but declined the immediate re-hash.

*   **User-Facing Consequences:** As long as a format is in a degraded state:
    1.  **Persistent Warning:** A prominent warning will be displayed *before* the output of any Gooru command.
        > `Warning: Hashing logic for '.flac' is inconsistent. Duplicate detection and queries may be unreliable. Run 'gooru rehash --upgrade' to fix.`
    2.  **Write Refusal:** All write operations (`tag`, `settags`, `untag`, `add`, `rehash`) that target a file of the inconsistent format will be **blocked**.
        > `Error: Cannot process 'song.flac'. The database is in an inconsistent state for this file type. Please run 'gooru rehash --upgrade' to apply pending configuration changes.`

This model makes it impossible for a user to unknowingly corrupt the logical integrity of their database.

### 4.3. Internal Logic & DB Schema

*   **Hasher Dispatcher:** The `Hasher` struct will act as a dispatcher. When asked to hash a file, it will consult the database configuration to see if content-aware hashing is enabled for that extension. If so, it uses the format-specific logic; otherwise, it falls back to the database's default `partial` or `full` strategy.

*   **Database Schema:** The `meta` table will be used to store the configuration. This structure is extensible for future configuration needs.

| key | value | Description |
| :--- | :--- | :--- |
| `hashing_strategy` | `partial` | The default hashing strategy for the database. |
| `content_aware_version` | `1` | Global version, incremented on any logic change. |
| `ca_ext_flac_enabled` | `1` or `0` | Per-format toggle. |
| `ca_ext_flac_version` | `1` | The logic version for `.flac` active in this DB. |
| `ca_ext_flac_consistent`| `1` or `0`| Flag indicating if a migration is pending. |
| ... | ... | ... |

## 5. Edge Cases & Unresolved Questions

*   **What if a user enables CAH, says "no" to the rehash, then tries to add a *new* `.flac` file?**
    *   **Decision:** The "Write Refusal" aspect of Degraded Mode solves this perfectly. The `gooru add` command will be blocked with an error message, preventing the database from entering a mixed state.
*   **What if a re-hash operation is interrupted (e.g., Ctrl+C)?**
    *   **Decision:** Database operations on individual files are transactional. The `meta` table version number for the format will not have been updated. The next time Gooru is run, it will re-detect the inconsistency and re-enter degraded mode, prompting the user to run the upgrade again. This is a safe failure mode.

## 6. Alternatives Considered

*   **"Mixed State" Operation:** The idea of allowing the database to contain hashes from different logic versions simultaneously.
    *   **Rejected:** As discussed, this is fundamentally broken. It breaks duplicate detection, fragments query results, and violates the user's mental model of content identity. The "Degraded Mode" with write refusal is a far safer and more responsible design.
*   **Auto-Rehash on `config` command:** Instead of prompting, just start the re-hash immediately.
    *   **Rejected:** Violates the principle of user control over long-running, potentially destructive operations. A re-hash could take hours on a large library, and the user must explicitly consent to it.