# CLI guide

This guide covers the everyday `gooru` command-line workflows. Run `gooru <command> --help` for the exact flags supported by your build.

## Database selection

By default, Gooru uses its normal database path, typically `~/.config/gooru/gooru.db`.

Use `--database` to operate on another library:

```bash
gooru --database /absolute/path/library.db init
gooru --database /absolute/path/library.db list
```

## Initialize a library

```bash
gooru init
```

Initialization is a one-time operation. It creates the database and records the hashing strategy. See [HASHING.md](HASHING.md) for the differences between partial and full hashing and how to choose between them.

For non-interactive first-boot automation, choose the strategy explicitly and make creation idempotent:

```bash
gooru init --if-missing --hashing-strategy partial
# or
gooru init --if-missing --hashing-strategy full
```

`--hashing-strategy` skips the prompt and accepts only `partial` or `full`. When `--if-missing` finds an existing database file, it leaves that database completely unchanged; it does not reconcile the stored hashing strategy. When the database is missing, `--if-missing` requires an explicit `--hashing-strategy` so automation never silently chooses a permanent database invariant.

## Manage local database users

User-management commands operate only on an already initialized database; they never create or initialize one.

Create an administrator interactively:

```bash
gooru user create-admin --username alice
```

Add `--if-missing` when provisioning should succeed without changing an existing user. For non-interactive automation, `GOORU_ADMIN_PASSWORD` supplies the password instead of prompting; its value is used exactly rather than trimmed.

Rotate an existing user's password with:

```bash
gooru user set-password --username alice
```

`set-password` preserves the user's Gooru identity and revokes the user's active sessions after the password is replaced. It also accepts `GOORU_ADMIN_PASSWORD` for non-interactive local automation.

The NixOS module uses the machine-oriented reconciliation command:

```bash
GOORU_ADMIN_PASSWORD='desired password' \
  gooru user reconcile-admin --username alice
```

The command prints the stable Gooru user ID. Persist that ID and pass it back on later runs with `--user-id`: the same user can then be renamed without losing per-user data. With an unchanged password, reconciliation performs one normal password verification and does not create a new hash. A changed password is re-hashed and active sessions for that user are revoked. A persisted `--user-id` that no longer exists is an error rather than permission to adopt a different account.

## Track and tag files

`tag` both tracks files and adds tags:

```bash
gooru tag ~/Pictures/*.jpg photo
gooru tag ~/Pictures/IMG_001.jpg event:wedding favorite
```

`add` tracks files without assigning tags:

```bash
gooru add ~/Pictures ~/Videos/intro.mp4
```

`settags` replaces a file's existing tags:

```bash
gooru settags ~/Pictures/IMG_001.jpg photo event:wedding selected
```

`untag` removes specific tags, or all tags when no tag is supplied:

```bash
gooru untag ~/Pictures/IMG_001.jpg selected
gooru untag ~/Pictures/IMG_001.jpg
```

For multiple input paths, commands that support `--multi` use `--` to separate file arguments from tags:

```bash
gooru tag --multi a.jpg b.jpg c.jpg -- photo reviewed
```

## Query the library

```bash
gooru list 'photo event:wedding'
gooru table 'photo -rejected'
gooru count 'type:img'
gooru exists 'favorite'
gooru listtags --count
```

`exists` prints `true` and exits 0 when a match exists; it prints `false` and exits 1 otherwise, which makes it useful in shell scripts.

The full query language is documented in [QUERY.md](QUERY.md).

## Modify by query

Tagging commands can operate on all files matching an expression with `-e`:

```bash
gooru tag -e 'photo -reviewed' reviewed
gooru untag -e 'event:wedding' temporary
gooru settags -e 'archive:2024' archived
```

Treat query-based mutations carefully: they may affect many records at once.

## Filesystem changes

Gooru associates tags with content identity rather than with a single pathname, so it can recover from many renames and moves.

### Repair moves, duplicate locations, and missing paths

```bash
gooru relinkall ~/Pictures ~/Videos
```

`relinkall` performs a pre-check, scans when needed, shows proposed moves/additions/deletions, and asks before applying changes. Use `--yes` for non-interactive operation after you understand the proposal behavior.

Use `--always-verify-hash` when you want the pre-scan to hash content instead of relying on metadata.

### A file changed intentionally

```bash
gooru rehash path/to/file
```

`rehash` updates the content identity while preserving the file's tags.

### A path changed and you already know both names

```bash
gooru editpath old/path new/path
```

This manually updates the recorded path without a scan.

## Remove tracked records

```bash
gooru delete path/to/file
gooru delete -e 'temporary'
```

The CLI `delete` command removes Gooru's location records; it does **not** delete the underlying filesystem file. If the removed location was the last location for that content, the associated content record and tags are also removed from the database.

The web API has a separate physical-delete mode for files inside managed upload roots.

## Rename a tag

```bash
gooru renametag project:alpha project:beta
```

The rename is applied across the database.

## Saved searches

Server users can own saved searches. The CLI exposes management under:

```bash
gooru saved-search --help
```

Saved searches are scoped to the stable DB-backed user identity, so changing a username does not change ownership.

## Virtual filesystem

`gooru mount` is only functional in binaries built with FUSE support:

```bash
go build -tags fuse -o gooru ./cmd/gooru
gooru mount /mnt/gooru 'photo favorite'
```

A normal build still contains the command, but reports that FUSE support is unavailable. Platform FUSE headers/runtime support are required.

## Correctness versus speed

Path-based file operations normally hash content to determine the current identity. Commands exposing `--use-metadata` can instead use size + modification time as a fast pre-check. That can substantially reduce work on large batches, but it may miss a content change when metadata has not changed.
