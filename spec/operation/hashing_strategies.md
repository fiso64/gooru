# Spec: Hashing Strategies (Partial vs. Full)

**Version:** 1.0
**Status:** Implemented

---

## 1. Abstract

Gooru's core principle is identifying files by their content. This is achieved by generating a unique hash for each file. To accommodate different user needs regarding performance and data integrity, Gooru offers two distinct hashing strategies: `partial` and `full`. This choice, made once during database initialization, fundamentally determines the trade-off between the speed of processing large files and the absolute guarantee of content uniqueness. This document specifies the technical implementation, trade-offs, and use cases for each strategy.

## 2. Problem Statement / Motivation

A single hashing strategy cannot efficiently serve all use cases. The time required to read and hash a file is directly proportional to its size. While this is negligible for small documents, it becomes a significant bottleneck for large media files.

*   **User Story (Media Library):** As a user with terabytes of large video and photo files, I need to add thousands of items to Gooru quickly. Waiting for the system to read every byte of every file would make the initial import and subsequent additions unacceptably slow. I am willing to accept a theoretical, infinitesimally small risk of a hash collision in exchange for a massive performance gain.

*   **User Story (Archivist):** As a user managing a critical archive of legal documents, source code, or research data, I need an absolute, cryptographic guarantee that two files are identical. A hash collision, however unlikely, would represent a catastrophic failure of data integrity. I am willing to accept slower performance on large files to achieve this certainty.

Gooru must serve both users effectively. The solution is to empower the user to make an informed, one-time decision that aligns the tool's behavior with their specific needs.

## 3. Goals and Non-Goals

### Goals

*   Provide a very fast hashing option (`partial`) optimized for large files.
*   Provide a maximally reliable hashing option (`full`) that guarantees content uniqueness.
*   Present this choice clearly to the user during the `gooru init` command.
*   The chosen strategy must be stored permanently in the database metadata.
*   The rest of the Gooru system must operate seamlessly with either strategy.

### Non-Goals

*   The system will not allow mixing hashing strategies within a single database.
*   The system will not allow the hashing strategy to be changed after the database has been initialized.
*   The system will not attempt to automatically guess the best strategy for the user.

## 4. Proposed Solution & Technical Design

The user selects their preferred hashing strategy via an interactive prompt during the `gooru init` command. This choice is saved to the `meta` table in the database and is used to configure the hasher instance for all subsequent operations.

### 4.1. Strategy 1: Full Hashing (`full`)

This strategy prioritizes correctness and data integrity above all else.

*   **Algorithm:** The system reads the entire file from the first byte to the last byte and feeds the complete data stream into the BLAKE3 hash function.
*   **Identifier:** The resulting BLAKE3 hash, represented as a hex-encoded string.
*   **Pros:**
    *   **Maximum Reliability:** Provides a cryptographic guarantee of uniqueness. The probability of two different files producing the same BLAKE3 hash is negligible to the point of being considered impossible for practical purposes.
    *   **Simplicity:** The logic is straightforward and unambiguous.
*   **Cons:**
    *   **Performance:** I/O bound. The time to hash a file is directly proportional to its size. This can be very slow for multi-gigabyte files.
*   **Primary Use Case:** Archives of critical documents, source code repositories, legal records, or any collection where absolute data integrity is non-negotiable.

### 4.2. Strategy 2: Partial Hashing (`partial`)

This strategy prioritizes performance for large files, making a pragmatic trade-off for reliability.

*   **Algorithm:** The hashing process is size-dependent.
    1.  **Small File Fallback:** If a file is smaller than a predefined threshold (`640KB`), it is **fully hashed** using the same logic as the `full` strategy. This ensures maximum reliability for documents and other small files where the performance cost is negligible.
    2.  **Large File Sampling:** For files larger than the threshold, the hash is generated from a composite of metadata and sampled content chunks:
        *   **Component 1: File Size:** The exact size of the file in bytes is fed into the hasher first. This is the most critical part of the partial hash, as it immediately differentiates files of different sizes.
        *   **Component 2: Content Chunks:** The system reads 10 fixed-size chunks of `64KB` each from strategic locations in the file: the first chunk from the beginning, the last chunk from the very end, and the remaining eight chunks distributed evenly in between. These chunks are then fed into the hasher.
*   **Identifier:** The resulting BLAKE3 hash of the combined `filesize + chunk1 + chunk2 + ... + chunk10` data, represented as a hex-encoded string.
*   **Pros:**
    *   **Extreme Performance:** The time to hash a large file is constant and very low, as it only ever reads a small, fixed amount of data (approx. 640KB) regardless of whether the file is 1MB or 100GB.
*   **Cons:**
    *   **Theoretical Collision Risk:** It is theoretically possible, though exceedingly rare, for two different files of the exact same size to have identical content in all 10 sampled chunks, resulting in a hash collision.
*   **Primary Use Case:** Large media libraries (videos, photos, music), disk images, virtual machine files, and any other collection dominated by large files where near-instantaneous processing is the primary concern.

## 5. Edge Cases & Unresolved Questions

*   **What is the real-world collision risk of partial hashing?**
    *   **Decision:** While difficult to quantify, the risk is astronomically low for non-malicious, real-world data. The combination of the file size pre-filter, 10 separate data chunks from across the file, and the strength of the BLAKE3 algorithm makes an accidental collision a purely theoretical concern.
*   **Could a file be maliciously crafted to cause a partial hash collision?**
    *   **Decision:** Yes. An attacker with knowledge of the sampling algorithm could construct a file to match another file's partial hash. For this reason, Gooru's partial hashing should not be used for security-sensitive applications (e.g., verifying file integrity against a malicious actor). It is designed for organization, not for cryptographic non-repudiation.
*   **How does the chosen strategy affect file modification detection?**
    *   **Decision:** The logic for detecting changes remains the same (as defined in `spec/operation/file_change_heuristics.v2.md`). However, the performance of commands like `rehash` is dramatically affected. Rehashing a 50GB video file will be nearly instant in `partial` mode but will take minutes in `full` mode. This is an expected and accepted consequence of the chosen strategy.

## 6. Performance Considerations

The performance characteristics are the primary differentiator between the two strategies.

| Feature / Operation        | Full Hashing                               | Partial Hashing                            |
| -------------------------- | ------------------------------------------ | ------------------------------------------ |
| **Speed (Large Files >1GB)** | Slow (minutes)                             | Very Fast (sub-second)                     |
| **Speed (Small Files <1MB)** | Fast (sub-second)                          | Fast (sub-second, falls back to full hash) |
| **Reliability**            | Maximum (Cryptographically Secure)         | Extremely High (Theoretical Risk)          |
| **I/O Usage (Large Files)**  | Reads the entire file from disk            | Reads a small, fixed amount (~640KB)       |