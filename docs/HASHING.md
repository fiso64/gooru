# Hashing strategies

Gooru identifies files by a BLAKE3-based content hash. Each database uses one hashing strategy: **full** or **partial**. The strategy is selected when the database is initialized and is stored with that database.

```bash
gooru init
```

The choice affects how much file data Gooru must read when determining content identity. It does not change how tags or paths are stored.

## Full hashing

Full hashing reads the entire file and computes a BLAKE3 hash over its contents.

This provides cryptographically strong content identity: accidentally different files are overwhelmingly unlikely to receive the same hash. The trade-off is I/O cost, because every byte must be read whenever Gooru needs to calculate the file's identity.

Full hashing is a good choice when exact byte-for-byte identity is more important than hashing speed, or when the library mainly contains small files.

## Partial hashing

Partial hashing reduces the amount of data read from large files.

Files smaller than **640 KiB** are still hashed in full. For files at or above that size, Gooru hashes:

- the file size; and
- ten **64 KiB** samples distributed across the file, including the beginning and end.

This means hashing a very large file requires reading only a small, fixed amount of its contents instead of the entire file.

Partial hashing is useful for media libraries containing large files such as videos. It is designed for content identity in normal library use, not as an adversarial integrity check. Two different files with the same size and identical sampled regions will have the same partial hash even if they differ elsewhere.

## Choosing a strategy

| Strategy | Reads | Best suited for |
| --- | --- | --- |
| **Partial** | Full contents below 640 KiB; size + ten 64 KiB samples for larger files | Large media libraries where hashing speed and disk I/O matter |
| **Full** | Entire file | Libraries where the strongest content-identity assurance is preferred |

The strategy applies to the whole database. Gooru does not mix full and partial identities within one database.

If you are unsure, partial hashing is generally suitable for a personal media library. Choose full hashing when you specifically want every byte read when content identity is calculated.

## Hashing strategy vs. metadata shortcuts

The database hashing strategy is separate from options such as `--use-metadata`.

The hashing strategy controls **how a content hash is calculated** when Gooru hashes a file. Metadata shortcuts can instead avoid hashing in some operations by using properties such as file size and modification time as a fast pre-check.

See [CLI.md](CLI.md#correctness-versus-speed) for the metadata shortcut behavior.
