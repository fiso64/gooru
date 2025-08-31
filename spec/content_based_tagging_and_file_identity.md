# Spec: Content-Based Tagging and File Identity

**Version:** 1.0
**Status:** Implemented

---

## 1. Abstract

This document specifies the core principle of Gooru: a file's identity is defined by the hash of its content, not its name or location. Tags are associated with this content hash, making the system resilient to file renames, moves, and duplicates.

## 2. Problem Statement / Motivation

Traditional file systems and tagging tools tie metadata to a file's path. If a user renames `report_v1.docx` to `final_report.docx` or moves it to another directory, any associated tags or metadata are lost. This is fragile and forces users to re-apply information.

*   **User Story:** As a user, when I rename or move a tagged file, I want the system to recognize that it is the same file so that I don't lose its tags.

## 3. Goals and Non-Goals

### Goals

*   Associate tags with content, not paths.
*   Use a cryptographic hash of the file's content as its unique, stable identifier.
*   The system must be able to track the same content across multiple paths (duplicates).
*   The system must be able to update the path for a piece of content if it is moved or renamed.

### Non-Goals

*   This system will not automatically monitor the filesystem for changes in real-time. Synchronization is a user-initiated action.
*   This system will not store file content, only its hash and metadata.

## 4. Proposed Solution & Technical Design

*   **User-Facing Changes:** The core tagging commands will operate on file paths. Internally, these paths will be resolved to content hashes. The user experience should feel seamless; they provide a path, and the system handles the content mapping.

*   **Internal Logic:**
    1.  When a file is tagged for the first time, the system will compute its BLAKE3 hash.
    2.  This hash is stored in a `contents` table.
    3.  The file's path, size, and modification time are stored in a `locations` table, linked to the content hash.
    4.  Tags are stored in a `tags` table and linked to the content hash in a `content_tags` junction table.

### Example Walkthroughs

**Example 1: Tagging and Renaming**
```bash
# 1. Initial State: photo.jpg exists, is not in the DB.
$ gooru tag photo.jpg vacation summer
# System computes hash H_A for photo.jpg and applies tags.

# 2. User renames the file on the filesystem.
$ mv photo.jpg holiday.jpg

# 3. User tags the "new" file. Gooru recognizes the content.
$ gooru tag holiday.jpg trip
# Expected Output:
Detected move for known content: '/path/to/photo.jpg' -> '/path/to/holiday.jpg'

# 4. Internally, Gooru has updated its locations table to point to the new
#    path for hash H_A and then added the new tag.

# 5. Retrieving tags for the new path now shows all associated tags.
$ gooru gettags holiday.jpg
# Expected Output:
summer
trip
vacation
```

## 5. Edge Cases & Unresolved Questions

*   **What happens if a tagged file is modified?**
    *   **Decision:** The file on disk is now new content with a new hash. The old hash and its tags are "orphaned" but remain in the DB. For information on how specific operations handle modified files, see operations/core_tagging_operations.md.
*   **What happens if a file is an exact duplicate of another already-tagged file?**
    *   **Decision:** When the new file is tagged, the system will compute its hash. It will find that the hash already exists. The system will simply add a new entry to the `locations` table for the new path, pointing to the existing content hash. Both files will now share the same tags.
*   **What happens if a file is deleted?**
    *   **Decision:** The record in the `locations` table becomes stale. See operations/filesystem_synchronization.md.
