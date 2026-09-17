package types

// HashingStrategy defines the method used to hash files.
type HashingStrategy string

const (
	// StrategyPartial uses file size and hashes of strategic chunks. Fast for large files.
	StrategyPartial HashingStrategy = "partial"
	// StrategyFull hashes the entire file content. Most reliable, but slow for large files.
	StrategyFull HashingStrategy = "full"
)

// FileStatus indicates the state of a file relative to the database.
type FileStatus int

const (
	// StatusOK means the file is in the DB and its metadata matches.
	StatusOK FileStatus = iota
	// StatusModified means the file is in the DB but its metadata differs.
	StatusModified
	// StatusNotInDB means the file path is not in the database, though the file may exist on disk.
	StatusNotInDB
	// StatusUntrackedContent means the path is not in the DB, but the content is known.
	StatusUntrackedContent
)

// RehashStatus indicates the result of a rehash operation for a single file.
type RehashStatus int

const (
	// StatusRehashed means the file was modified and its content record was updated.
	StatusRehashed RehashStatus = iota
	// StatusSkippedUnchanged means the file was not modified and was skipped.
	StatusSkippedUnchanged
	// StatusSkippedNotInDB means the file was not in the database and was skipped.
	StatusSkippedNotInDB
	// StatusMetadataUpdated means the file content was identical but metadata was updated.
	StatusMetadataUpdated
)

// ParsedTag represents a tag split into its key and value.
// For simple tags, Value is an empty string.
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
	OldPath     string
	NewLocation LocationInfo
}

// RelinkResult holds the outcome of a relink scan before any destructive actions.
type RelinkResult struct {
	Stats           RelinkStats
	ProposedMoves   []MoveInfo     // Files that appear to have been moved/renamed.
	ProposedAdds    []LocationInfo // New locations for existing content (duplicates).
	ProposedDeletes []FileInfo     // Files in DB that are no longer on disk.
}

// NotificationKind describes the type of a notification generated during an operation.
type NotificationKind int

const (
	// NotificationKindMoveDetected indicates a file was moved/renamed.
	NotificationKindMoveDetected NotificationKind = iota
	// NotificationKindModified indicates a file's content was updated in the database.
	NotificationKindModified
)

// Notification provides structured information about events that occurred during an operation.
type Notification struct {
	Kind         NotificationKind
	OriginalPath string   // The path the user provided.
	OldPath      string   // For moves, the path stored in the DB.
	NewPath      string   // For moves, the new path on disk.
	OrphanedTags []string // For modifications, tags of the old content.
}

// TagOperationResult holds the results of a path-based tagging operation.
type TagOperationResult struct {
	AffectedCount int
	Notifications []Notification
}

// LocationInfo holds metadata about a file's location.
type LocationInfo struct {
	Path        string // The canonical logical path of the file
	StoragePath string // Optional managed physical path; empty means Path
	Hash        string
	Size        int64
	ModTime     int64 // Unix time
	AddedAt     int64 // Unix milliseconds; zero lets the database assign insertion time
	Extension   string
	TagsCache   string
}

// FileInfo holds all displayable information about a file.
type FileInfo struct {
	ID          int64
	PublicID    string
	Path        string // Canonical logical path
	StoragePath string // Optional managed physical path; empty means Path
	Hash        string
	Size        int64
	ModTime     int64 // Unix time
	AddedAt     int64 // Unix milliseconds
	Tags        []string
	Metadata    *MediaMetadata
}

// PageCursor identifies the last row from a keyset-paginated file page.
type PageCursor struct {
	Sort   string `json:"sort"`
	Order  string `json:"order"`
	ID     int64  `json:"id"`
	Offset int    `json:"offset,omitempty"`
}

// MediaMetadata holds cached media properties for a tracked file.
type MediaMetadata struct {
	LocationID      int64
	MediaKind       string
	MimeType        string
	ImageWidth      *int
	ImageHeight     *int
	VideoWidth      *int
	VideoHeight     *int
	DurationSeconds *float64
	FrameCount      *int
	PageCount       *int
}

// SavedSearch is a per-user persisted browse query.
type SavedSearch struct {
	ID        string
	UserID    string
	Name      string
	Query     string
	Sort      string
	Order     string
	CreatedAt int64
	UpdatedAt int64
}

// TagWithCount holds a tag and its usage count.
type TagWithCount struct {
	Tag   string
	Count int
}

// MountInfo holds metadata about a running mount process.
// This is serialized to JSON in the runtime directory.
type MountInfo struct {
	PID        int    `json:"pid"`
	MountPoint string `json:"mount_point"`
	Port       int    `json:"port"`
	ID         string `json:"id"`
}
