package types

// ParsedTag represents a tag split into its key and value.
// For simple tags, Key is an empty string.
type ParsedTag struct {
	Key   string
	Value string
}

// RelinkStats provides statistics about the relink operation.
type RelinkStats struct {
	FilesScanned     int
	LocationsAdded   int
	LocationsRemoved int
}

// LocationInfo holds metadata about a file's location.
type LocationInfo struct {
	Hash      string
	Size      int64
	ModTime   int64 // Unix time
	Extension string
	TagsCache string
}

// FileInfo holds all displayable information about a file.
type FileInfo struct {
	Path string
	Size int64
	Tags string // Comma-separated string of tags
}