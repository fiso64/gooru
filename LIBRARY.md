
# Gooru Library Usage

This document outlines how to use the `gooru` package as a library in your own Go projects.

## Setup

To use the Gooru library, import it into your Go project:

```go
import "gooru.local/gooru"
```

## Initialization

The primary entrypoint to the library is the `gooru.Client`. To create a new client, use `gooru.New()`:

```go
// dbPath is the path to the SQLite database file (e.g., "/path/to/gooru.db").
// The directory will be created if it doesn't exist.
// Set verbose to true to log SQL queries to stderr.
client, err := gooru.New(dbPath, false)
if err != nil {
    // handle error
}
defer client.Close() // IMPORTANT: Always close the client when done.
```

## Core Operations

All operations are methods on the `gooru.Client` struct.

### Tagging Files

The library provides high-performance, transactional methods for tagging files.

- **`TagFiles(filePaths []string, tags []string, progressCb func(filePath string, err error)) error`**: Adds one or more tags to multiple files. It's additive and won't remove existing tags.

- **`SetTagsForFiles(filePaths []string, tags []string, progressCb func(filePath string, err error)) error`**: Sets the tags for multiple files, replacing all existing tags. If `tags` is empty, it removes all tags.

- **`UntagFiles(filePaths []string, tags []string, progressCb func(filePath string, err error)) error`**: Removes specific tags from multiple files.

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

### Retrieving Files and Tags

- **`GetTagsForFile(filePath string) ([]string, error)`**: Retrieves all tags for a single file. Performs a safety check against the file's metadata to ensure it hasn't changed.

- **`GetAllTags() ([]string, error)`**: Returns a sorted list of all unique tags in the database.

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