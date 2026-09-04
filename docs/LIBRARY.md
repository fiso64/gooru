# Go package

Gooru's core library lives in `gooru.local/gooru` and is used by the CLI and server.

> **Module-path note:** the repository currently declares `module gooru.local`. That is suitable for this source tree, but it is not a normal public Go module path. External consumers will need a local module replacement/fork until the project adopts a publicly resolvable module path. Treat the package API as evolving unless a release states otherwise.

## Create a database

```go
package main

import (
    "log"

    "gooru.local/gooru"
    "gooru.local/types"
)

func main() {
    err := gooru.Init("/path/to/gooru.db", types.StrategyPartial, false)
    if err != nil {
        log.Fatal(err)
    }
}
```

`StrategyPartial` is optimized for large files. `StrategyFull` hashes the entire file. The chosen strategy is persisted with the database. See [HASHING.md](HASHING.md) for the exact behavior and trade-offs.

## Open a client

```go
client, err := gooru.New("/path/to/gooru.db", false)
if err != nil {
    // gooru.ErrDBUninitialized means Init has not been run for this database.
    return err
}
defer client.Close()
```

All normal operations are methods on `*gooru.Client`.

## Tag files

```go
result, err := client.TagFiles(
    []string{"/data/image.jpg"},
    []string{"photo", "project:alpha"},
    nil,
    false,
)
```

The last argument controls the metadata heuristic:

- `false` — hash content for correctness;
- `true` — use size + modification time as a fast pre-check, with the risk of missing changes whose metadata did not change.

Related methods include `SetTagsForFiles` and `UntagFiles`.

## Tag by query

```go
count, err := client.TagFilesByQuery("photo -reviewed", []string{"reviewed"})
```

Query-based mutations operate directly against matching database records and avoid per-path callbacks.

See [QUERY.md](QUERY.md) for expression syntax.

## Read data

Common methods include:

```go
paths, err := client.ListFilesByQuery("photo favorite", false)
files, err := client.GetFilesInfoByQuery("photo favorite", false)
tags, err := client.GetAllTags()
tagCounts, err := client.GetAllTagsWithCounts()
count, err := client.CountFilesByQuery("photo", false)
found, err := client.ExistsFilesByQuery("photo", false)
```

`GetTagsForFile` and `GetFileInfoForFile` also return a `types.FileStatus` indicating whether the path is known and whether its current content differs from the recorded identity.

## Filesystem synchronization

Use the relink flow when paths may have moved:

```go
needsScan, err := client.NeedsRelink([]string{"/data"}, false)
if err != nil {
    return err
}
if needsScan {
    proposed, err := client.Relink([]string{"/data"})
    if err != nil {
        return err
    }

    // Inspect proposed before applying it.
    _, err = client.ApplyRelinkChanges(proposed)
    if err != nil {
        return err
    }
}
```

The proposal separates moved/renamed paths, additional duplicate locations, and obsolete locations.

For a known path rename, use `EditPath`. To remove location records, use `PruneLocations` or `DeleteFilesByQuery`.

## Intentionally modified content

`RehashFiles` updates a file's content identity while transferring its tags to the new content record. Use it for files that are expected to change in place.

## Rename a tag

```go
err := client.RenameTag("project:alpha", "project:beta")
```

The operation updates the tag across the database.

## Tag validation

Tags use printable, non-space ASCII syntax. A tag can be a simple key (`photo`) or key/value pair (`project:alpha`). Keys and values cannot begin or end with `-`, `!`, or `:` and the key cannot be empty.

The query-only keys `ext` and `type` are reserved and cannot be created as normal tag keys.
