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

Arguably, the best non-hash based approach is an inode+metadata-based tagger. Note that there is more than one way to implement it.

## TODO

- Test database performance on a large and a huge db.