package types

// RelinkStats provides statistics about the relink operation.
type RelinkStats struct {
	FilesScanned     int
	LocationsAdded   int
	LocationsRemoved int
}

// LocationInfo holds metadata about a file's location.
type LocationInfo struct {
	Hash    string
	Size    int64
	ModTime int64 // Unix time
}