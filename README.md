# Gooru

Gooru is a high-performance, content-centric command-line tool for tagging and organizing local files. It uses content hashing to identify files, making it resilient to renames and moves.

This project is currently under development.

## Todo (Important): Unusable for tagging large files.
The initial add is prohibitibely slow for 10GB+ files. This means that as of right now, gooru cannot be reasonably used for tagging large files (like movies, long videos).

Consider a general tagger. We want to optimize these two properties:

1. Performance
2. Reliablity (Content Change Detection)
3. Persistence (Surviving Moves/Renames) without user intervention.

An improvement in one will typically result in a decrease of the others. You can't have all three.

Examples:
1. Path-based tagger: Excellent performance, Terrible persistence, Terrible reliability. Tags do not persist on renames/moves. In addition, it will not detect when a file is overwriten with completely different content.
2. Inode-based tagger: Excellent performance, Mediocre persistence, Terrible reliability: Tags are preserved only when moving within the same drive. Like path-based taggers, it will not detect overwrites.
3. Hash-based tagger: Terrible performance, Excellent persistence, Excellent reliability: Hashing is slow, but files can be freely moved anywhere without losing tags. Changing a file will change its hash.

All three types of taggers can be combined with metadata heuristics (like modtime and file size) to improve performance or persistence or reliability. 

In this framework, gooru is a hash-based tagger that uses size and modtime heuristics to improve performance. Gooru's specific implementation of the heuristic leads to the following properties:
- Low performance: It is still terrible whenever it needs to hash, but is excellent whenever we trust the heuristic.
- Good reliability: Reliability is no longer excellent, as the two different files can have the same size+modtime. Sometimes we trust the heuristic that the file is unchanged.
- Excellent persistence: Same as the hash-based tagger, but faster. For the relinkall command, gooru only uses the filesize as a necessary condition for files to be the same, not a sufficient one. 

For common filesystems like ntfs and ext4, there might not be way to strictly improve it accross one or more of the three properties without hurting another. So, in terms of its fundamental approach to tagging (though perhaps not in concrete implementation), gooru may be a **local maximum**.

Arguably, the best high performance approach is an inode+metadata-based tagger, or:

### The "Incremental Assembler" Tagger

This is a high-performance tagger that minimizes initial work by building a file's identity progressively.

**Core Idea:** A file's unique ID is assembled from a series of hashes of small, non-overlapping data chunks (e.g., the first 4KB, the middle 4KB, etc.). The system only computes the minimum number of chunks needed to distinguish a file from all others in the database.

**How it Works:**
1.  **On `add`:** It computes only the first chunk's hash. If this hash is unique, it's done. If it collides with an existing file, it then computes the *next* chunk for both files and compares again, repeating this process only until the collision is resolved.
2.  **On `relinkall`:** To find a moved/renamed file, it incrementally computes each chunk's hash and queries the database, stopping as soon as a unique match is found.
3.  **Persistence:** Tags are linked to a stable, internal `content_id` that never changes, even as more hash chunks are added to resolve collisions.

Tradeoff: Reliability. Changes might not be detected due to the small initial partial hash. Maybe try:

1. For Speed on add: Use the fast, incremental hash to find a unique slot.
2. For Reliability on rehash: When modtime changes, use a full, deterministic, multi-part hash (e.g., hash of first 8KB + middle 8KB + last 8KB) to verify the content's integrity.

## TODO

- Test database performance on a large and a huge db.