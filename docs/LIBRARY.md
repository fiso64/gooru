
# Gooru Library Usage

This document outlines how to use the `gooru` package as a library in your own Go projects.

## Setup

To use the Gooru library, import it into your Go project:

```go
import "gooru.local/gooru"
```

## Initialization

Using the Gooru library is a two-step process: initializing a database and then creating a client to connect to it.

### Step 1: Initializing the Database

Before you can use Gooru, you must initialize a database file. This is a one-time operation that creates the necessary tables and, crucially, sets the **hashing strategy** for the database. This choice is permanent.

Use the `gooru.Init()` function for this:
```go
import "gooru.local/gooru/types"

// dbPath is the path where the SQLite database file will be created.
dbPath := "/path/to/gooru.db"

// Choose a hashing strategy. This determines the trade-off between
// performance and reliability for large files.
// - types.StrategyPartial: (Recommended) Very fast. Identifies files by their size
//   plus hashes of a few small data chunks. Ideal for large media libraries.
// - types.StrategyFull: Slower for large files but provides maximum reliability by
//   hashing the entire file content. Best for critical documents.
strategy := types.StrategyPartial

// The verbose flag logs SQL queries to stderr.
err := gooru.Init(dbPath, strategy, false)
if err != nil {
    // handle error (e.g., if the file already exists)
}
```

### Step 2: Creating a Client

Once the database is initialized, you can create a `gooru.Client` to interact with it. The client will automatically detect and use the hashing strategy that was set during initialization.

```go
// dbPath points to your *initialized* database file.
client, err := gooru.New(dbPath, false)
if err != nil {
    // This will return `gooru.ErrDBUninitialized` if `gooru.Init()` was not run.
    // Handle other potential errors.
}
defer client.Close() // IMPORTANT: Always close the client when done.
```

## Core Operations

All operations are methods on the `gooru.Client` struct.

### Tag Validation

The library enforces a strict validation policy for tags.

#### General Syntax Rules (for all operations)

The following rules apply to tags whether you are creating them or using them in a query:
-   A tag must only contain printable ASCII characters (characters 33-126). **Spaces are not allowed.**
-   The key part of a `key:value` tag cannot be empty.
-   A tag (or the key/value parts of a `key:value` tag) cannot start or end with the characters `-`, `!`, or `:`.

**Valid syntax examples:** `photo`, `project:alpha`, `version-1.0`, `needs_review`
**Invalid syntax examples:** `"my tag"` (contains space), `!important` (starts with `!`), `final-` (ends with `-`), `:work` (empty key), `project:v1:` (value ends with `:`)

#### Reserved Keywords (for creating/setting tags)

When you are applying tags to a file (e.g., using `TagFiles`, `SetTagsForFiles`), an additional rule applies:
-   The tag key cannot be `ext` or `type`, as these are reserved for special query syntax.

This means you can search for files using `ext:jpg`, but you cannot create a tag with the key `ext`. Methods that create tags will return an error if you attempt to use a reserved keyword.

**Invalid tags to apply:** `ext:backup`, `type:document`.

**Note on Simple Tags vs. Key-Only Queries:**

- When **tagging**, `tag` and `tag:` are treated identically. Both create a simple tag in the database with `key='tag'` and an empty value.
- When **querying**, a simple tag like `photo` in an expression (`gooru list photo`) acts as a "key-only" search. It will match all content that has *any* tag with the key `photo`, including the simple tag `photo` as well as key-value tags like `photo:album1` and `photo:vacation`.

### Tagging Files

The library provides high-performance, transactional methods for tagging files.

- **`TagFiles(filePaths []string, tags []string, progressCb func(filePath string, err error)) (int, error)`**: Adds one or more tags to multiple files. It's additive and won't remove existing tags. Returns the number of *new* tag associations created.

- **`SetTagsForFiles(filePaths []string, tags []string, progressCb func(filePath string, err error)) (int, error)`**: Sets the tags for multiple files, replacing all existing tags. If `tags` is empty, it removes all tags. Returns the total number of tag associations changed (removed + added).

- **`UntagFiles(filePaths []string, tags []string, progressCb func(filePath string, err error)) (int, error)`**: Removes specific tags from multiple files. If `tags` is empty, it removes all tags from the files. Returns the number of tag associations that were actually removed.

The `progressCb` is an optional callback that, if provided, is invoked for each file processed, reporting either success (`err == nil`) or failure.

**Example:**
```go
files := []string{"/path/to/image.jpg", "/path/to/doc.pdf"}
tags := []string{"project:alpha", "important"}

err := client.TagFiles(files, tags, func(filePath string, err error) {
    if err != nil {
        fmt.Printf("Failed to tag %s: %v\n", filePath, err)
    }
})
// handle potential database error
```

**File Modification and Move Handling**

All three path-based tagging functions (`TagFiles`, `SetTagsForFiles`, and `UntagFiles`) intelligently handle cases where a file has been modified or moved since it was last seen.

- If a file path points to content that has been **modified** (different size or modtime), the system automatically re-hashes the file, updates the database to point the path to the new content, and proceeds with the tagging operation on the new content. The old content's tags are left orphaned in the database.
- If a file path is new, but its content hash matches an existing file that is now missing from its old path, the system treats this as a **move/rename**. It updates the path in the database and applies the operation.

In both cases, the operation succeeds on the current state of the file, ensuring data integrity and resilience to filesystem changes. Note that these automatic updates print notifications to standard output, which may not be desirable in all library contexts.

### Tagging by Query

For maximum performance when tagging large sets of files that match a query, use the `ByQuery` variants. These methods operate directly on the database using the query expression, avoiding the overhead of fetching and iterating through file paths in your application. They do not support progress callbacks but return a count of affected records.

- **`TagFilesByQuery(expression string, tags []string) (int, error)`**: Adds tags to all files matching the query expression. Returns the number of new tag associations created.

- **`SetTagsForFilesByQuery(expression string, tags []string) (int, error)`**: Sets the tags for all files matching the query, replacing existing ones. Returns the number of files affected.

- **`UntagFilesByQuery(expression string, tags []string) (int, error)`**: Removes specific tags from all files matching the query. If `tags` is empty, it removes *all* tags from the matching files. Returns the number of tag associations removed.

**Example:**
```go
// Add the 'archived' tag to all files that are not tagged 'important'
count, err := client.TagFilesByQuery("-important", []string{"archived"})
if err != nil {
    // handle error
}
fmt.Printf("Archived %d items.\n", count)
```

### Retrieving Files and Tags

- **`GetTagsForFile(filePath string) ([]string, error)`**: Retrieves all tags for a single file. Performs a safety check against the file's metadata to ensure it hasn't changed.

- **`GetAllTags() ([]string, error)`**: Returns a sorted list of all unique tags in the database.

- **`GetAllTagsWithCounts() ([]types.TagWithCount, error)`**: Returns a list of all tags and their usage counts, sorted by count (descending). The `TagWithCount` struct has `Tag` (string) and `Count` (int) fields.

- **`ListFilesByQuery(expression string, verbose bool) ([]string, error)`**: The most powerful query method. Parses a complex query expression and returns a list of matching file paths.
    - **Expression Syntax**: `tag1`, `"tag1 tag2"` (AND), `"tag1 | tag2"` (OR), `tag1 -tag2` (NOT), `(tag1 | tag2) -tag3` (grouping), `ext:jpg`, `type:img`.

- **`ListAllFiles() ([]string, error)`**: Lists all file paths known to the database.

- **`ListFilesByTag(tag string) ([]string, error)`**: Lists files matching a single tag.

- **`ListFilesByTagsAnd(tags []string) ([]string, error)`**: Lists files matching all of the given tags.

**Example:**
```go
// Find all files tagged with 'photo' but not 'work'
paths, err := client.ListFilesByQuery("photo -work", false)
if err != nil {
    // handle error
}
for _, p := range paths {
    fmt.Println(p)
}
```

### Retrieving Detailed File Information

The library also provides methods to retrieve `types.FileInfo` structs, which include the path, size, and a cached, comma-separated string of tags. These are more efficient than getting paths and then getting tags for each file.

- **`GetFilesInfoByQuery(expression string, verbose bool) ([]types.FileInfo, error)`**
- **`GetAllFilesInfo() ([]types.FileInfo, error)`**
- **`GetFilesInfoByTag(tag string) ([]types.FileInfo, error)`**
- **`GetFilesInfoByTagsAnd(tags []string) ([]types.FileInfo, error)`**


### Filesystem Synchronization

Gooru is resilient to file moves and renames. The `relink` operation scans the filesystem to find these changes.

- **`NeedsRelink(dirs []string) (bool, error)`**: Performs a fast metadata check to see if a full scan is necessary. Returns `true` if any file in the database is missing from disk or has been modified.

- **`Relink(dirs []string) (types.RelinkResult, error)`**: Performs a "dry run" scan of the specified directories. It returns a `RelinkResult` struct detailing proposed changes:
    - `ProposedMoves`: Files that have been moved or renamed.
    - `ProposedAdds`: New locations (duplicates) for content already in the database.
    - `ProposedDeletes`: Database records for files no longer found on disk.

- **`ApplyRelinkChanges(changes types.RelinkResult) (types.RelinkStats, error)`**: Executes the changes proposed by `Relink`. This is the only method in the relink process that modifies the database.

**Example Workflow:**
```go
dirs := []string{"/home/user/documents"}
needsScan, err := client.NeedsRelink(dirs)
if err == nil && needsScan {
    proposedChanges, err := client.Relink(dirs)
    if err == nil {
        // Optionally, inspect proposedChanges and ask for user confirmation
        stats, err := client.ApplyRelinkChanges(proposedChanges)
        // handle error and check stats
    }
}
```

### Manual Path Management

For cases where you know a file has moved and want to update the database without a full scan.

- **`EditPath(oldPath, newPath string) error`**: Manually updates a file's path in the database.

- **`PruneLocations(paths []string) (int, error)`**: Removes a list of file paths from the database. Returns the number of records removed.

### Manual Content Management

- **`RehashFiles(filePaths []string, progressCb func(path string, status types.RehashStatus, err error))`**: Explicitly updates the content record for files that have been modified on disk. For each file, it calculates the new content hash and transactionally transfers all existing tags from the old content record to the new one, preserving the file's tagged identity. This is the primary library function for managing the lifecycle of a file that is expected to change over time. The `progressCb` is invoked for each file, reporting its final status (e.g., `StatusRehashed`, `StatusSkippedUnchanged`).