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

// MoveInfo describes a file that appears to have been moved.
type MoveInfo struct {
	OldPath string
	NewPath string
	Size    int64
	Tags    string
}

// RelinkResult holds the outcome of a relink scan before any destructive actions.
type RelinkResult struct {
	Stats           RelinkStats
	ProposedMoves   []MoveInfo     // Files that appear to have been moved/renamed.
	ProposedAdds    []LocationInfo // New locations for existing content (duplicates).
	ProposedDeletes []FileInfo     // Files in DB that are no longer on disk.
}

// LocationInfo holds metadata about a file's location.
type LocationInfo struct {
	Path      string // The absolute path of the file
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

// TagWithCount holds a tag and its usage count.
type TagWithCount struct {
	Tag   string
	Count int
}
