package database

import (
	"database/sql"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"

	"gooru.local/types"
	_ "gosqlite.org"
)

// splitTags is a helper to safely split the space-delimited tags_cache string.
func splitTags(cache string) []string {
	if cache == "" {
		return nil
	}
	return strings.Split(cache, " ")
}

// escapeLikeLiteral escapes SQLite LIKE metacharacters so filesystem paths are
// matched literally. The trailing wildcard is added separately by the caller.
func escapeLikeLiteral(value string) string {
	replacer := strings.NewReplacer("~", "~~", "%", "~%", "_", "~_")
	return replacer.Replace(value)
}

type Store struct {
	DB             *sql.DB
	dataSourceName string
	logger         *log.Logger
	backendClose   func() error
}

// Tx is a transaction wrapper that logs queries.
type Tx struct {
	*sql.Tx
	logger *log.Logger
}

// CreateEmptyDB ensures a database file exists at the given path.
// It creates an empty file but does not initialize any schema.
func CreateEmptyDB(dataSourceName string) error {
	// Opening and immediately closing is a standard way to create the file if it doesn't exist.
	db, err := sql.Open("sqlite3", dataSourceName)
	if err != nil {
		return err
	}
	if err := db.Close(); err != nil {
		return err
	}
	return SecureDBFiles(dataSourceName)
}

// NewStore opens an existing database connection. It does not perform initialization.
func NewStore(dataSourceName string, verbose bool) (*Store, error) {
	// Add `_journal=WAL` for better concurrency.
	// Add `_busy_timeout=5000` so transient writer contention waits instead of failing immediately.
	// Add `_txlock=immediate` so mutation transactions reserve SQLite's writer slot
	// before establishing a read snapshot. This prevents read-then-write callers
	// from failing with SQLITE_BUSY_SNAPSHOT after another writer commits.
	// Add `_foreign_keys=on` to enforce foreign key constraints.
	db, err := sql.Open("sqlite3", fmt.Sprintf("%s?_foreign_keys=on&_journal=WAL&_busy_timeout=5000&_txlock=immediate", dataSourceName))
	if err != nil {
		return nil, err
	}

	if err = db.Ping(); err != nil {
		db.Close()
		return nil, err
	}
	if err := SecureDBFiles(dataSourceName); err != nil {
		db.Close()
		return nil, err
	}

	var logOutput io.Writer
	if verbose {
		logOutput = os.Stderr
	} else {
		logOutput = io.Discard
	}
	logger := log.New(logOutput, "SQL: ", log.Ltime|log.Lmicroseconds)

	return &Store{DB: db, dataSourceName: dataSourceName, logger: logger}, nil
}

// SecureDBFiles constrains the SQLite database and sidecar files to owner-only access.
func SecureDBFiles(dataSourceName string) error {
	for _, path := range []string{dataSourceName, dataSourceName + "-wal", dataSourceName + "-shm", dataSourceName + "-journal"} {
		if err := os.Chmod(path, 0600); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("secure database file %s: %w", path, err)
		}
	}
	return nil
}

// Close closes the database connection.
func (s *Store) Close() error {
	var err error
	if s.backendClose != nil {
		err = s.backendClose()
	} else {
		err = s.DB.Close()
	}
	if secureErr := SecureDBFiles(s.dataSourceName); err == nil {
		err = secureErr
	}
	return err
}

func logQuery(logger *log.Logger, query string, args []interface{}) {
	logger.Printf("QUERY: %s\n-- ARGS: %d bound values redacted", query, len(args))
}

// Begin starts a new transaction.
func (s *Store) Begin() (*Tx, error) {
	tx, err := s.DB.Begin()
	if err != nil {
		return nil, err
	}
	return &Tx{Tx: tx, logger: s.logger}, nil
}

// Querier implementations for logging on Store
func (s *Store) Exec(query string, args ...interface{}) (sql.Result, error) {
	logQuery(s.logger, query, args)
	return s.DB.Exec(query, args...)
}
func (s *Store) Query(query string, args ...interface{}) (*sql.Rows, error) {
	logQuery(s.logger, query, args)
	return s.DB.Query(query, args...)
}
func (s *Store) QueryRow(query string, args ...interface{}) *sql.Row {
	logQuery(s.logger, query, args)
	return s.DB.QueryRow(query, args...)
}

// Querier implementations for logging on Tx
func (tx *Tx) Exec(query string, args ...interface{}) (sql.Result, error) {
	logQuery(tx.logger, query, args)
	return tx.Tx.Exec(query, args...)
}
func (tx *Tx) Query(query string, args ...interface{}) (*sql.Rows, error) {
	logQuery(tx.logger, query, args)
	return tx.Tx.Query(query, args...)
}
func (tx *Tx) QueryRow(query string, args ...interface{}) *sql.Row {
	logQuery(tx.logger, query, args)
	return tx.Tx.QueryRow(query, args...)
}

// GetHashingStrategy reads the configured hashing strategy from the meta table.
func (s *Store) GetHashingStrategy() (types.HashingStrategy, error) {
	var strategy string
	err := s.QueryRow("SELECT value FROM meta WHERE key = 'hashing_strategy'").Scan(&strategy)
	if err != nil {
		return "", err // Propagates sql.ErrNoRows if not found
	}
	return types.HashingStrategy(strategy), nil
}

// SetHashingStrategy saves the chosen hashing strategy to the meta table.
// This is typically only done once during database initialization.
func (s *Store) SetHashingStrategy(strategy types.HashingStrategy) error {
	_, err := s.Exec("INSERT INTO meta (key, value) VALUES (?, ?)", "hashing_strategy", strategy)
	return err
}

// GetDBVersion reads the application-level database version from the meta table.
func (s *Store) GetDBVersion() (int, error) {
	var version int
	err := s.QueryRow("SELECT value FROM meta WHERE key = 'db_version'").Scan(&version)
	if err != nil {
		if err == sql.ErrNoRows {
			// If the key doesn't exist, it's an un-versioned or corrupt DB.
			return 0, fmt.Errorf("db_version key not found in meta table")
		}
		return 0, err
	}
	return version, nil
}

// IsInitialized checks if the database schema appears to be initialized.
func (s *Store) IsInitialized() (bool, error) {
	var name string
	// The schema_migrations table is the source of truth for whether migrations have run.
	query := "SELECT name FROM sqlite_master WHERE type='table' AND name='schema_migrations'"
	err := s.QueryRow(query).Scan(&name)
	if err != nil {
		if err == sql.ErrNoRows {
			return false, nil // Table not found, so not initialized.
		}
		return false, err // A real database error occurred.
	}
	return true, nil
}

type Querier interface {
	Exec(query string, args ...interface{}) (sql.Result, error)
	Query(query string, args ...interface{}) (*sql.Rows, error)
	QueryRow(query string, args ...interface{}) *sql.Row
}

// GetOrCreateContent inserts a content hash if it doesn't exist.
// It returns true if a new row was inserted, indicating brand new content.
func (s *Store) GetOrCreateContent(q Querier, hash string) (bool, error) {
	res, err := q.Exec("INSERT OR IGNORE INTO contents (hash) VALUES (?)", hash)
	if err != nil {
		return false, err
	}
	rowsAffected, err := res.RowsAffected()
	if err != nil {
		// This path is highly unlikely with go-sqlite3 but handle it for robustness.
		return false, err
	}
	return rowsAffected > 0, nil
}

// ContentExists reports whether a content hash is already tracked.
func (s *Store) ContentExists(hash string) (bool, error) {
	var exists int
	err := s.QueryRow("SELECT 1 FROM contents WHERE hash = ? LIMIT 1", hash).Scan(&exists)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

func (s *Store) UpsertMediaMetadata(meta types.MediaMetadata) error {
	_, err := s.Exec(`
        INSERT INTO media_metadata (
  content_hash, media_kind, mime_type, image_width, image_height,
  video_width, video_height, duration_seconds, frame_count, page_count, updated_at
        )
        SELECT content_hash, ?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP
        FROM locations
        WHERE id = ?
        ON CONFLICT(content_hash) DO UPDATE SET
  media_kind=excluded.media_kind,
  mime_type=excluded.mime_type,
  image_width=excluded.image_width,
  image_height=excluded.image_height,
  video_width=excluded.video_width,
  video_height=excluded.video_height,
  duration_seconds=excluded.duration_seconds,
  frame_count=excluded.frame_count,
  page_count=excluded.page_count,
  updated_at=CURRENT_TIMESTAMP
    `, meta.MediaKind, meta.MimeType, meta.ImageWidth, meta.ImageHeight, meta.VideoWidth, meta.VideoHeight, meta.DurationSeconds, meta.FrameCount, meta.PageCount, meta.LocationID)
	return err
}

func (s *Store) GetMediaMetadata(locationID int64) (types.MediaMetadata, error) {
	var meta types.MediaMetadata
	meta.LocationID = locationID
	err := s.QueryRow(`
        SELECT mm.media_kind, mm.mime_type, mm.image_width, mm.image_height, mm.video_width, mm.video_height, mm.duration_seconds, mm.frame_count, mm.page_count
        FROM locations l
        JOIN media_metadata mm ON mm.content_hash = l.content_hash
        WHERE l.id = ?
    `, locationID).Scan(&meta.MediaKind, &meta.MimeType, &meta.ImageWidth, &meta.ImageHeight, &meta.VideoWidth, &meta.VideoHeight, &meta.DurationSeconds, &meta.FrameCount, &meta.PageCount)
	return meta, err
}

// GetOrCreateLocation ensures a file path for a given content hash exists.
// The tags_cache will be populated by a database trigger.
func (s *Store) GetOrCreateLocation(q Querier, hash, path string, size int64, modTime int64, extension string) error {
	_, err := q.Exec("INSERT OR IGNORE INTO locations (public_id, content_hash, path, size_bytes, mod_time, extension) VALUES ('file_' || lower(hex(randomblob(16))), ?, ?, ?, ?, ?)", hash, path, size, modTime, extension)
	return err
}

// UpdateContentLocation atomically replaces the location for a given content hash.
// This handles file moves/renames cleanly.
func (s *Store) UpdateContentLocation(q Querier, hash, path string, size int64, modTime int64, extension string) error {
	// Remove any old locations for this content.
	if _, err := q.Exec("DELETE FROM locations WHERE content_hash = ?", hash); err != nil {
		return err
	}
	// Insert the new, current location. The new trigger will populate the tags_cache automatically.
	_, err := q.Exec("INSERT INTO locations (public_id, content_hash, path, size_bytes, mod_time, extension) VALUES ('file_' || lower(hex(randomblob(16))), ?, ?, ?, ?, ?)", hash, path, size, modTime, extension)
	return err
}

// FindContentHashByPath finds a content hash by its file path.
func (s *Store) FindContentHashByPath(path string) (string, error) {
	var hash string
	err := s.QueryRow("SELECT content_hash FROM locations WHERE path = ?", path).Scan(&hash)
	if err != nil {
		if err == sql.ErrNoRows {
			return "", nil // Return empty string and no error if not found
		}
		return "", err
	}
	return hash, nil
}

// GetLocationByPath finds a location's metadata by its file path.
func (s *Store) GetLocationByPath(path string) (types.LocationInfo, error) {
	var loc types.LocationInfo
	err := s.QueryRow("SELECT content_hash, size_bytes, mod_time, tags_cache FROM locations WHERE path = ?", path).Scan(&loc.Hash, &loc.Size, &loc.ModTime, &loc.TagsCache)
	if err != nil {
		// This will correctly propagate sql.ErrNoRows
		return types.LocationInfo{}, err
	}
	loc.Path = path
	return loc, nil
}

// GetOrCreateTag finds a tag by key/value or creates it, returning its ID.
func (s *Store) GetOrCreateTag(q Querier, key, value string) (int64, error) {
	var id int64
	err := q.QueryRow("SELECT id FROM tags WHERE key = ? AND value = ?", key, value).Scan(&id)
	if err == nil {
		return id, nil // Found it
	}
	if err != sql.ErrNoRows {
		return 0, err // A real error occurred
	}

	// Not found, so create it
	res, err := q.Exec("INSERT INTO tags (key, value) VALUES (?, ?)", key, value)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// GetTagID retrieves a tag's ID by its key/value.
func (s *Store) GetTagID(key, value string) (int64, error) {
	var id int64
	err := s.QueryRow("SELECT id FROM tags WHERE key = ? AND value = ?", key, value).Scan(&id)
	return id, err
}

// AssociateTag links a tag with a content hash. The database trigger will handle updating the cache.
func (s *Store) AssociateTag(q Querier, hash string, tagID int64) error {
	_, err := q.Exec("INSERT OR IGNORE INTO content_tags (content_hash, tag_id) VALUES (?, ?)", hash, tagID)
	return err
}

// DisassociateTag removes a link between a tag and a content hash. The database trigger will handle updating the cache.
func (s *Store) DisassociateTag(q Querier, hash string, tagID int64) error {
	_, err := q.Exec("DELETE FROM content_tags WHERE content_hash = ? AND tag_id = ?", hash, tagID)
	return err
}

// ClearTagsForContent removes all tag associations for a given content hash. The database trigger will handle updating the cache.
func (s *Store) ClearTagsForContent(q Querier, hash string) error {
	_, err := q.Exec("DELETE FROM content_tags WHERE content_hash = ?", hash)
	return err
}

// GetTagsForContent retrieves all tags for a given content hash.
func (s *Store) GetTagsForContent(hash string) ([]string, error) {
	query := `
		SELECT t.key, t.value
		FROM tags t 
		JOIN content_tags ct ON t.id = ct.tag_id 
		WHERE ct.content_hash = ? ORDER BY t.key, t.value`
	rows, err := s.Query(query, hash)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanTagStrings(rows)
}

func scanTagStrings(rows *sql.Rows) ([]string, error) {
	var tags []string
	for rows.Next() {
		var tag types.ParsedTag
		if err := rows.Scan(&tag.Key, &tag.Value); err != nil {
			return nil, err
		}
		tags = append(tags, parsedTagString(tag))
	}
	return tags, rows.Err()
}

func scanStringsInto(rows *sql.Rows, out []string) ([]string, error) {
	for rows.Next() {
		var value string
		if err := rows.Scan(&value); err != nil {
			return nil, err
		}
		out = append(out, value)
	}
	return out, rows.Err()
}

func scanStrings(rows *sql.Rows) ([]string, error) {
	return scanStringsInto(rows, nil)
}

// ListAllFiles retrieves all file paths from the database.
func (s *Store) ListAllFiles() ([]string, error) {
	rows, err := s.Query("SELECT path FROM locations ORDER BY path")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanStrings(rows)
}

// CountAllFiles counts all location records in the database.
func (s *Store) CountAllFiles() (int, error) {
	var count int
	err := s.QueryRow("SELECT COALESCE(SUM(files_count), 0) FROM kind_counts").Scan(&count)
	return count, err
}

// GetHashToTagsCacheMap retrieves a map of content hashes to their cached tag strings.
func (s *Store) GetHashToTagsCacheMap() (map[string]string, error) {
	// We only need one entry per hash, so GROUP BY is appropriate.
	rows, err := s.Query("SELECT content_hash, tags_cache FROM locations GROUP BY content_hash")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	cacheMap := make(map[string]string)
	for rows.Next() {
		var hash, cache string
		if err := rows.Scan(&hash, &cache); err != nil {
			return nil, err
		}
		cacheMap[hash] = cache
	}
	return cacheMap, rows.Err()
}

// ListFilesByTag retrieves all file paths for a given tag.
func (s *Store) ListFilesByTag(key, value string) ([]string, error) {
	rows, err := s.Query(`
		SELECT l.path
		FROM locations l
		JOIN content_tags ct ON l.content_hash = ct.content_hash
		JOIN tags t ON ct.tag_id = t.id
		WHERE t.key = ? AND t.value = ? ORDER BY l.path`, key, value)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanStrings(rows)
}

// GetAllContentHashes retrieves a set of all known content hashes for fast lookups.
func (s *Store) GetAllContentHashes() (map[string]struct{}, error) {
	rows, err := s.Query("SELECT hash FROM contents")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	hashes := make(map[string]struct{})
	for rows.Next() {
		var hash string
		if err := rows.Scan(&hash); err != nil {
			return nil, err
		}
		hashes[hash] = struct{}{}
	}
	return hashes, rows.Err()
}

type sizeToHashesRows interface {
	Next() bool
	Scan(dest ...any) error
	Err() error
}

func sizeToHashesMapFromRows(rows sizeToHashesRows) (map[int64][]string, error) {
	// Using a map to a map to easily handle unique hashes per size
	tempMap := make(map[int64]map[string]struct{})

	for rows.Next() {
		var size int64
		var hash string
		if err := rows.Scan(&size, &hash); err != nil {
			return nil, err
		}
		if _, ok := tempMap[size]; !ok {
			tempMap[size] = make(map[string]struct{})
		}
		tempMap[size][hash] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Convert to the final structure only after the full authoritative query succeeded.
	finalMap := make(map[int64][]string)
	for size, hashes := range tempMap {
		finalMap[size] = make([]string, 0, len(hashes))
		for hash := range hashes {
			finalMap[size] = append(finalMap[size], hash)
		}
	}
	return finalMap, nil
}

// GetSizeToHashesMap retrieves a map of file sizes to a list of hashes of files with that size.
func (s *Store) GetSizeToHashesMap() (map[int64][]string, error) {
	rows, err := s.Query("SELECT size_bytes, content_hash FROM locations")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return sizeToHashesMapFromRows(rows)
}

// GetLocationsForDirs retrieves a map of all known paths to their content hashes for the given directories.
func (s *Store) GetLocationsForDirs(dirs []string) (map[string]types.LocationInfo, error) {
	locations := make(map[string]types.LocationInfo)
	for i := 0; i < len(dirs); i += maxVars {
		end := i + maxVars
		if end > len(dirs) {
			end = len(dirs)
		}
		batch := dirs[i:end]
		placeholders := make([]string, len(batch))
		args := make([]interface{}, len(batch))
		for j, dir := range batch {
			placeholders[j] = "(?)"
			args[j] = escapeLikeLiteral(dir+string(filepath.Separator)) + "%"
		}

		rows, err := s.Query(`
			WITH requested(pattern) AS (VALUES `+strings.Join(placeholders, ",")+`)
			SELECT l.path, l.content_hash, l.size_bytes, l.mod_time, l.extension, l.tags_cache
			FROM locations l
			WHERE EXISTS (
				SELECT 1 FROM requested
				WHERE l.path LIKE requested.pattern ESCAPE '~'
			)
		`, args...)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var path string
			var info types.LocationInfo
			if err := rows.Scan(&path, &info.Hash, &info.Size, &info.ModTime, &info.Extension, &info.TagsCache); err != nil {
				rows.Close()
				return nil, err
			}
			locations[path] = info
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return nil, err
		}
		if err := rows.Close(); err != nil {
			return nil, err
		}
	}
	return locations, nil
}

// ApplyRelinkAdditions transactionally adds new locations.
func (s *Store) ApplyRelinkAdditions(toAdd map[string]types.LocationInfo) (int, error) {
	if len(toAdd) == 0 {
		return 0, nil
	}

	tx, err := s.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	added, err := s.ApplyRelinkAdditionsTx(tx, toAdd)
	if err != nil {
		return 0, err
	}

	return added, tx.Commit()
}

// ApplyRelinkAdditionsTx adds new locations within an existing transaction.
func (s *Store) ApplyRelinkAdditionsTx(q Querier, toAdd map[string]types.LocationInfo) (int, error) {
	if len(toAdd) == 0 {
		return 0, nil
	}

	var locationsAdded int
	const columns = 5 // content_hash, path, size_bytes, mod_time, extension
	batchSize := maxVars / columns
	var args []interface{}
	var queryBuilder strings.Builder
	// The tags_cache is now populated by triggers, so we don't insert it here.
	queryBuilder.WriteString("INSERT OR IGNORE INTO locations (public_id, content_hash, path, size_bytes, mod_time, extension) VALUES ")

	itemsInBatch := 0
	for path, info := range toAdd {
		if itemsInBatch > 0 {
			queryBuilder.WriteString(", ")
		}
		queryBuilder.WriteString("('file_' || lower(hex(randomblob(16))), ?, ?, ?, ?, ?)")
		args = append(args, info.Hash, path, info.Size, info.ModTime, info.Extension)
		itemsInBatch++

		if itemsInBatch >= batchSize {
			res, err := q.Exec(queryBuilder.String(), args...)
			if err != nil {
				return 0, err
			}
			added, _ := res.RowsAffected()
			locationsAdded += int(added)
			itemsInBatch = 0
			args = nil
			queryBuilder.Reset()
			queryBuilder.WriteString("INSERT OR IGNORE INTO locations (public_id, content_hash, path, size_bytes, mod_time, extension) VALUES ")
		}
	}

	if itemsInBatch > 0 {
		res, err := q.Exec(queryBuilder.String(), args...)
		if err != nil {
			return 0, err
		}
		added, _ := res.RowsAffected()
		locationsAdded += int(added)
	}

	return locationsAdded, nil
}

// RemoveLocationsByPath transactionally removes locations by their paths.
func (s *Store) RemoveLocationsByPath(paths []string) (int, error) {
	if len(paths) == 0 {
		return 0, nil
	}

	tx, err := s.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	removed, err := s.RemoveLocationsByPathTx(tx, paths)
	if err != nil {
		return 0, err
	}

	return removed, tx.Commit()
}

// RemoveLocationsByPathTx removes locations by their paths within an existing transaction.
func (s *Store) RemoveLocationsByPathTx(q Querier, paths []string) (int, error) {
	if len(paths) == 0 {
		return 0, nil
	}

	var totalRemoved int
	for start := 0; start < len(paths); start += maxVars {
		end := min(start+maxVars, len(paths))
		placeholders, args := stringBatchArgs(paths[start:end])
		query := "DELETE FROM locations WHERE path IN (" + placeholders + ")"
		res, err := q.Exec(query, args...)
		if err != nil {
			return 0, err
		}
		removed, _ := res.RowsAffected()
		totalRemoved += int(removed)
	}

	return totalRemoved, nil
}

// UpdatePath updates a location's path and metadata, with checks for existence.
func (s *Store) UpdatePath(absOldPath string, newInfo types.LocationInfo, displayOldPath, displayNewPath string) error {
	tx, err := s.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// 1. Check if new path already exists
	var dummy int
	err = tx.QueryRow("SELECT 1 FROM locations WHERE path = ?", newInfo.Path).Scan(&dummy)
	if err != sql.ErrNoRows {
		if err == nil { // A row was found
			return fmt.Errorf("new path already exists in database: %s", displayNewPath)
		}
		return fmt.Errorf("db check for new path failed: %w", err) // Other DB error
	}

	// 2. Perform the update
	res, err := tx.Exec(`
		UPDATE locations SET path = ?, size_bytes = ?, mod_time = ?, extension = ?
		WHERE path = ?`,
		newInfo.Path, newInfo.Size, newInfo.ModTime, newInfo.Extension, absOldPath,
	)
	if err != nil {
		return err
	}

	// 3. Check if the update was successful
	rowsAffected, err := res.RowsAffected()
	if err != nil {
		return err
	}

	if rowsAffected == 0 {
		return fmt.Errorf("old path not found in database: %s", displayOldPath)
	}

	return tx.Commit()
}

func tagMatchCondition(tags []types.ParsedTag) (string, []interface{}) {
	clauses := make([]string, len(tags))
	args := make([]interface{}, 0, len(tags)*2)
	for i, tag := range tags {
		clauses[i] = "(t.key = ? AND t.value = ?)"
		args = append(args, tag.Key, tag.Value)
	}
	return strings.Join(clauses, " OR "), args
}

func tagFilteredLocationsQuery(projection string, tags, notTags []types.ParsedTag) (string, []interface{}) {
	positiveWhere, args := tagMatchCondition(tags)
	var queryBuilder strings.Builder
	queryBuilder.WriteString(fmt.Sprintf(`
		WITH positive_hashes AS (
			SELECT ct.content_hash
			FROM content_tags ct
			JOIN tags t ON ct.tag_id = t.id
			WHERE %s
			GROUP BY ct.content_hash
			HAVING COUNT(t.id) = ?
		)`, positiveWhere))
	args = append(args, len(tags))

	if len(notTags) > 0 {
		negativeWhere, negativeArgs := tagMatchCondition(notTags)
		args = append(args, negativeArgs...)
		queryBuilder.WriteString(fmt.Sprintf(`,
		negative_hashes AS (
			SELECT DISTINCT ct.content_hash
			FROM content_tags ct
			JOIN tags t ON ct.tag_id = t.id
			WHERE %s
		)`, negativeWhere))
	}

	queryBuilder.WriteString(`
		SELECT ` + projection + `
		FROM locations l
		JOIN positive_hashes ph ON l.content_hash = ph.content_hash`)
	if len(notTags) > 0 {
		queryBuilder.WriteString(`
		LEFT JOIN negative_hashes nh ON l.content_hash = nh.content_hash
		WHERE nh.content_hash IS NULL`)
	}
	queryBuilder.WriteString(` ORDER BY l.path`)
	return queryBuilder.String(), args
}

// ListFilesByTagsAnd retrieves all file paths for files matching all `tags` but none of the `notTags`.
func (s *Store) ListFilesByTagsAnd(tags []types.ParsedTag, notTags []types.ParsedTag) ([]string, error) {
	if len(tags) == 0 {
		return []string{}, nil
	}

	query, args := tagFilteredLocationsQuery("l.path", tags, notTags)
	rows, err := s.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanStrings(rows)
}

func scanBasicFileInfos(rows *sql.Rows) ([]types.FileInfo, error) {
	var files []types.FileInfo
	for rows.Next() {
		var file types.FileInfo
		var tagsCache string
		if err := rows.Scan(&file.ID, &file.Path, &file.Hash, &file.Size, &file.ModTime, &file.AddedAt, &tagsCache); err != nil {
			return nil, err
		}
		file.Tags = splitTags(tagsCache)
		files = append(files, file)
	}
	return files, rows.Err()
}

// GetAllFilesInfo retrieves detailed info for all files from the database using the cache.
func (s *Store) GetAllFilesInfo() ([]types.FileInfo, error) {
	query := `SELECT id, path, content_hash, size_bytes, mod_time, added_at, tags_cache FROM locations ORDER BY path`

	rows, err := s.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanBasicFileInfos(rows)
}

// GetAllFilesInfoPage retrieves one bounded page of file info from the database.
func (s *Store) GetAllFilesInfoPage(limit int, offset int) ([]types.FileInfo, error) {
	query := `SELECT id, path, content_hash, size_bytes, mod_time, added_at, tags_cache FROM locations ORDER BY path LIMIT ? OFFSET ?`

	rows, err := s.Query(query, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanBasicFileInfos(rows)
}

// GetFileInfoByLocationID retrieves detailed info for one tracked file location.
func (s *Store) GetFileInfoByLocationID(id int64) (types.FileInfo, error) {
	query := `SELECT ` + fileInfoColumns() + ` FROM locations l LEFT JOIN media_metadata mm ON mm.content_hash = l.content_hash WHERE l.id = ?`
	files, err := s.scanFileInfos(query, id)
	if err != nil {
		return types.FileInfo{}, err
	}
	if len(files) == 0 {
		return types.FileInfo{}, sql.ErrNoRows
	}
	return files[0], nil
}

func (s *Store) GetLocationPublicID(id int64) (string, error) {
	var publicID string
	err := s.QueryRow(`SELECT public_id FROM locations WHERE id = ?`, id).Scan(&publicID)
	return publicID, err
}

func (s *Store) GetLocationIDByPublicID(publicID string) (int64, error) {
	var id int64
	err := s.QueryRow(`SELECT id FROM locations WHERE public_id = ?`, publicID).Scan(&id)
	return id, err
}

// GetFileInfoByPath retrieves detailed info for one tracked file path without hashing the file.
func (s *Store) GetFileInfoByPath(path string) (types.FileInfo, error) {
	query := `SELECT ` + fileInfoColumns() + ` FROM locations l LEFT JOIN media_metadata mm ON mm.content_hash = l.content_hash WHERE l.path = ?`
	files, err := s.scanFileInfos(query, path)
	if err != nil {
		return types.FileInfo{}, err
	}
	if len(files) == 0 {
		return types.FileInfo{}, sql.ErrNoRows
	}
	return files[0], nil
}

// GetFilesInfoByTag retrieves info for all files for a given tag using the cache.
func (s *Store) GetFilesInfoByTag(key, value string) ([]types.FileInfo, error) {
	query := `
		SELECT l.id, l.path, l.content_hash, l.size_bytes, l.mod_time, l.added_at, l.tags_cache
		FROM locations l
		JOIN content_tags ct ON l.content_hash = ct.content_hash
		JOIN tags t ON ct.tag_id = t.id
		WHERE t.key = ? AND t.value = ?
		ORDER BY l.path`

	rows, err := s.Query(query, key, value)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanBasicFileInfos(rows)
}

// GetFilesInfoByTagsAnd retrieves info for all files matching all `tags` but none of the `notTags`.
func (s *Store) GetFilesInfoByTagsAnd(tags []types.ParsedTag, notTags []types.ParsedTag) ([]types.FileInfo, error) {
	if len(tags) == 0 {
		return []types.FileInfo{}, nil
	}

	query, args := tagFilteredLocationsQuery(
		"l.id, l.path, l.content_hash, l.size_bytes, l.mod_time, l.added_at, l.tags_cache",
		tags,
		notTags,
	)
	rows, err := s.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanBasicFileInfos(rows)
}

// GetAllTags retrieves all unique tags from the database.
// This includes synthesized simple tags for keys that only have key-value pairs.
func (s *Store) GetAllTags() ([]string, error) {
	return s.GetTags(0)
}

// GetTags retrieves unique tags from the database, optionally bounded by limit.
// This includes synthesized simple tags for keys that only have key-value pairs.
func (s *Store) GetTags(limit int) ([]string, error) {
	limitSQL := ""
	args := []any{}
	if limit > 0 {
		limitSQL = " LIMIT ?"
		args = append(args, limit)
	}
	query := `
		SELECT key, value FROM tags
		UNION
		SELECT DISTINCT key, '' AS value FROM tags
		ORDER BY key, value` + limitSQL
	rows, err := s.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanTagStrings(rows)
}

// GetAllTagsWithCounts retrieves all tags and their usage counts, sorted by count descending.
// It always includes a row for each unique tag key (e.g. 'photo') with its aggregate count,
// as well as rows for specific key-value tags (e.g. 'photo:album1').
func (s *Store) GetAllTagsWithCounts() ([]types.TagWithCount, error) {
	return s.GetTagsWithCounts(0)
}

func scanTagWithCounts(rows *sql.Rows) ([]types.TagWithCount, error) {
	var out []types.TagWithCount
	for rows.Next() {
		var item types.TagWithCount
		if err := rows.Scan(&item.Tag, &item.Count); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

// GetTagsWithCounts retrieves tags and their usage counts, sorted by count descending.
// A positive limit constrains the number of returned rows for bounded tag-index UIs.
func (s *Store) GetTagsWithCounts(limit int) ([]types.TagWithCount, error) {
	limitSQL := ""
	args := []any{}
	if limit > 0 {
		limitSQL = " LIMIT ?"
		args = append(args, limit)
	}
	query := `
		SELECT key AS tag_str, files_count AS final_count
		FROM tag_key_counts
		WHERE files_count > 0
		UNION ALL
		SELECT key || ':' || value AS tag_str, files_count AS final_count
		FROM tags
		WHERE value != '' AND files_count > 0
		ORDER BY final_count DESC, tag_str ASC
	` + limitSQL
	rows, err := s.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanTagWithCounts(rows)
}

func (s *Store) ListTagSuggestions(prefix string, limit int) ([]types.TagWithCount, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	prefix = strings.TrimSpace(prefix)
	like := prefix + "%"

	var rows *sql.Rows
	var err error
	switch {
	case strings.ContainsAny(prefix, "%_"):
		// Preserve historical raw-LIKE wildcard behavior. Key aggregates use
		// the maintained distinct-per-content summary, while direct plain-tag
		// rows remain a compatibility fallback for callers with synthetic data.
		rows, err = s.Query(`
			SELECT tag_str, files_count FROM (
				SELECT key AS tag_str, files_count FROM tag_key_counts
				WHERE files_count > 0 AND key LIKE ?
				UNION ALL
				SELECT t.key AS tag_str, t.files_count FROM tags t
				WHERE t.value = '' AND t.files_count > 0 AND t.key LIKE ?
				  AND NOT EXISTS (SELECT 1 FROM tag_key_counts k WHERE k.key = t.key AND k.files_count > 0)
				UNION ALL
				SELECT key || ':' || value AS tag_str, files_count FROM tags
				WHERE value != '' AND key || ':' || value LIKE ?
			)
			ORDER BY files_count DESC, tag_str ASC
			LIMIT ?
		`, like, like, like, limit)
	case strings.Contains(prefix, ":"):
		namespace, valuePrefix, _ := strings.Cut(prefix, ":")
		rows, err = s.Query(`
			SELECT key || ':' || value AS tag_str, files_count
			FROM tags
			WHERE value != '' AND key = ? AND value LIKE ?
			ORDER BY files_count DESC, tag_str ASC
			LIMIT ?
		`, strings.TrimSpace(namespace), strings.TrimSpace(valuePrefix)+"%", limit)
	default:
		rows, err = s.Query(`
			SELECT tag_str, files_count FROM (
				SELECT key AS tag_str, files_count FROM tag_key_counts
				WHERE files_count > 0 AND key LIKE ?
				UNION ALL
				SELECT t.key AS tag_str, t.files_count FROM tags t
				WHERE t.value = '' AND t.files_count > 0 AND t.key LIKE ?
				  AND NOT EXISTS (SELECT 1 FROM tag_key_counts k WHERE k.key = t.key AND k.files_count > 0)
				UNION ALL
				SELECT key || ':' || value AS tag_str, files_count FROM tags
				WHERE value != '' AND key LIKE ?
			)
			ORDER BY files_count DESC, tag_str ASC
			LIMIT ?
		`, like, like, like, limit)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanTagWithCounts(rows)
}

func (s *Store) ListNamespaceSuggestions(prefix string, limit int) ([]types.TagWithCount, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	like := strings.TrimSpace(prefix) + "%"
	rows, err := s.Query(`
		SELECT key || ':' AS tag_str, files_count
		FROM tag_namespace_counts
		WHERE files_count > 0 AND key LIKE ?
		ORDER BY files_count DESC, tag_str ASC
		LIMIT ?
	`, like, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanTagWithCounts(rows)
}

func (s *Store) ListTagValueSuggestions(namespace string, valuePrefix string, limit int) ([]types.TagWithCount, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	like := strings.TrimSpace(valuePrefix) + "%"
	rows, err := s.Query(`
		SELECT key || ':' || value AS tag_str, files_count
		FROM tags
		WHERE value != '' AND key = ? AND value LIKE ?
		ORDER BY files_count DESC, tag_str ASC
		LIMIT ?
	`, strings.TrimSpace(namespace), like, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanTagWithCounts(rows)
}

func (s *Store) ListTagNamespaces() ([]string, error) {
	rows, err := s.Query(`
		SELECT tk.key
		FROM tag_key_counts tk
		WHERE EXISTS (
			SELECT 1
			FROM tags t
			WHERE t.key = tk.key AND t.value != ''
		)
		ORDER BY tk.key
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanStrings(rows)
}

func (s *Store) KindFacets() ([]types.TagWithCount, error) {
	rows, err := s.Query(`
		SELECT kind, files_count
		FROM kind_counts
		WHERE files_count > 0
		ORDER BY files_count DESC, kind ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanTagWithCounts(rows)
}

func (s *Store) KindFacetsByLocationQuery(query string, args []interface{}) ([]types.TagWithCount, error) {
	finalQuery := fmt.Sprintf(`
		WITH result_locations(id) AS (%s)
		SELECT %s AS kind, COUNT(*)
		FROM locations l
		JOIN result_locations rl ON l.id = rl.id
		LEFT JOIN media_metadata mm ON mm.content_hash = l.content_hash
		GROUP BY kind
		ORDER BY COUNT(*) DESC, kind ASC
	`, query, fileKindExpression())
	rows, err := s.Query(finalQuery, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanTagWithCounts(rows)
}

func (s *Store) DeleteLocationByID(id int64) (bool, error) {
	res, err := s.Exec("DELETE FROM locations WHERE id = ?", id)
	if err != nil {
		return false, err
	}
	affected, err := res.RowsAffected()
	return affected > 0, err
}

// GetCountForTag gets the pre-calculated usage count for a specific tag.
func (s *Store) GetCountForTag(key, value string) (int, error) {
	var count int
	err := s.QueryRow("SELECT files_count FROM tags WHERE key = ? AND value = ?", key, value).Scan(&count)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	return count, err
}

// GetCountForKey gets the maintained distinct-file count for a tag key.
func (s *Store) GetCountForKey(key string) (int, error) {
	var count int
	err := s.QueryRow("SELECT files_count FROM tag_key_counts WHERE key = ?", key).Scan(&count)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	return count, err
}

// ExistsForKey checks if any file exists for a given tag key.
func (s *Store) ExistsForKey(key string) (bool, error) {
	var dummy int
	query := `
		SELECT 1
		FROM content_tags ct
		JOIN tags t ON ct.tag_id = t.id
		WHERE t.key = ? LIMIT 1
	`
	err := s.QueryRow(query, key).Scan(&dummy)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// Batch Helpers

const (
	// maxVars is a safe limit for SQLite's SQLITE_MAX_VARIABLE_NUMBER, which defaults to 999
	maxVars = 900
)

func parsedTagString(tag types.ParsedTag) string {
	if tag.Value == "" {
		return tag.Key
	}
	return tag.Key + ":" + tag.Value
}

func (s *Store) BatchGetTags(q Querier, parsedTags []types.ParsedTag) (map[string]int64, error) {
	tagIDMap := make(map[string]int64)
	if len(parsedTags) == 0 {
		return tagIDMap, nil
	}

	const columns = 2 // key, value
	batchSize := maxVars / columns
	for i := 0; i < len(parsedTags); i += batchSize {
		end := i + batchSize
		if end > len(parsedTags) {
			end = len(parsedTags)
		}
		batch := parsedTags[i:end]

		placeholders := make([]string, len(batch))
		args := make([]interface{}, 0, len(batch)*columns)
		for j, tag := range batch {
			placeholders[j] = "(?, ?)"
			args = append(args, tag.Key, tag.Value)
		}
		query := `WITH requested(key, value) AS (VALUES ` + strings.Join(placeholders, ",") + `)
			SELECT t.id, requested.key, requested.value
			FROM requested
			JOIN tags t ON t.key = requested.key AND t.value = requested.value`

		rows, err := q.Query(query, args...)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var id int64
			var key, value string
			if err := rows.Scan(&id, &key, &value); err != nil {
				rows.Close()
				return nil, err
			}
			tagIDMap[parsedTagString(types.ParsedTag{Key: key, Value: value})] = id
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return nil, err
		}
		if err := rows.Close(); err != nil {
			return nil, err
		}
	}
	return tagIDMap, nil
}

func (s *Store) BatchGetOrCreateTags(q Querier, parsedTags []types.ParsedTag) (map[string]int64, error) {
	tagIDMap, err := s.BatchGetTags(q, parsedTags)
	if err != nil {
		return nil, err
	}

	missing := make([]types.ParsedTag, 0, len(parsedTags)-len(tagIDMap))
	seenMissing := make(map[string]struct{})
	for _, tag := range parsedTags {
		tagStr := parsedTagString(tag)
		if _, exists := tagIDMap[tagStr]; exists {
			continue
		}
		if _, exists := seenMissing[tagStr]; exists {
			continue
		}
		seenMissing[tagStr] = struct{}{}
		missing = append(missing, tag)
	}
	if len(missing) == 0 {
		return tagIDMap, nil
	}

	const columns = 2 // key, value
	batchSize := maxVars / columns
	for i := 0; i < len(missing); i += batchSize {
		end := i + batchSize
		if end > len(missing) {
			end = len(missing)
		}
		batch := missing[i:end]
		placeholders := make([]string, len(batch))
		args := make([]interface{}, 0, len(batch)*columns)
		for j, tag := range batch {
			placeholders[j] = "(?, ?)"
			args = append(args, tag.Key, tag.Value)
		}
		query := "INSERT OR IGNORE INTO tags (key, value) VALUES " + strings.Join(placeholders, ",")
		if _, err := q.Exec(query, args...); err != nil {
			return nil, err
		}
	}

	created, err := s.BatchGetTags(q, missing)
	if err != nil {
		return nil, err
	}
	for tagStr, id := range created {
		tagIDMap[tagStr] = id
	}
	return tagIDMap, nil
}

func (s *Store) BatchInsertContents(q Querier, hashes []string) error {
	if len(hashes) == 0 {
		return nil
	}
	const columns = 1 // hash
	batchSize := maxVars / columns

	for i := 0; i < len(hashes); i += batchSize {
		end := i + batchSize
		if end > len(hashes) {
			end = len(hashes)
		}
		batch := hashes[i:end]

		placeholders := strings.Repeat("(?),", len(batch)-1) + "(?)"
		query := "INSERT OR IGNORE INTO contents (hash) VALUES " + placeholders
		args := make([]interface{}, len(batch))
		for j, h := range batch {
			args[j] = h
		}

		if _, err := q.Exec(query, args...); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) BatchUpsertLocations(q Querier, locations map[string]types.LocationInfo) error {
	if len(locations) == 0 {
		return nil
	}
	const columns = 6 // content_hash, path, size_bytes, mod_time, added_at, extension
	batchSize := maxVars / columns

	locs := make([]types.LocationInfo, 0, len(locations))
	for _, loc := range locations {
		locs = append(locs, loc)
	}

	for i := 0; i < len(locs); i += batchSize {
		end := i + batchSize
		if end > len(locs) {
			end = len(locs)
		}
		batch := locs[i:end]

		var placeholders []string
		var args []interface{}
		for _, loc := range batch {
			placeholders = append(placeholders, "('file_' || lower(hex(randomblob(16))), ?, ?, ?, ?, COALESCE(NULLIF(?, 0), CAST(strftime('%s','now') AS INTEGER)), ?)")
			args = append(args, loc.Hash, loc.Path, loc.Size, loc.ModTime, loc.AddedAt, loc.Extension)
		}
		query := `INSERT INTO locations (public_id, content_hash, path, size_bytes, mod_time, added_at, extension) VALUES ` +
			strings.Join(placeholders, ",") +
			` ON CONFLICT(path) DO UPDATE SET
				content_hash=excluded.content_hash,
				size_bytes=excluded.size_bytes,
				mod_time=excluded.mod_time,
				extension=excluded.extension`

		if _, err := q.Exec(query, args...); err != nil {
			return err
		}
		managedPlaceholders := make([]string, 0, len(batch))
		managedArgs := make([]interface{}, 0, len(batch)*2)
		for _, loc := range batch {
			if strings.TrimSpace(loc.StoragePath) == "" {
				continue
			}
			managedPlaceholders = append(managedPlaceholders, "(?, ?)")
			managedArgs = append(managedArgs, loc.StoragePath, loc.Path)
		}
		if len(managedPlaceholders) > 0 {
			managedQuery := `WITH managed(physical_path, path) AS (VALUES ` +
				strings.Join(managedPlaceholders, ",") +
				`)
				INSERT INTO managed_storage_locations (location_id, physical_path)
				SELECT l.id, managed.physical_path
				FROM managed
				JOIN locations l ON l.path = managed.path
				WHERE true
				ON CONFLICT(location_id) DO UPDATE SET physical_path = excluded.physical_path`
			if _, err := q.Exec(managedQuery, managedArgs...); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *Store) BatchClearTagsForContent(q Querier, hashes []string) (int64, error) {
	if len(hashes) == 0 {
		return 0, nil
	}

	var totalAffected int64
	for start := 0; start < len(hashes); start += maxVars {
		end := min(start+maxVars, len(hashes))
		placeholders, args := stringBatchArgs(hashes[start:end])
		query := "DELETE FROM content_tags WHERE content_hash IN (" + placeholders + ")"

		res, err := q.Exec(query, args...)
		if err != nil {
			return 0, err
		}
		affected, _ := res.RowsAffected()
		totalAffected += affected
	}
	return totalAffected, nil
}

type ContentTagPair struct {
	ContentHash string
	TagID       int64
}

func (s *Store) BatchAssociateTags(q Querier, pairs []ContentTagPair) (int64, error) {
	if len(pairs) == 0 {
		return 0, nil
	}
	const columns = 2
	batchSize := maxVars / columns

	var totalAffected int64
	for i := 0; i < len(pairs); i += batchSize {
		end := i + batchSize
		if end > len(pairs) {
			end = len(pairs)
		}
		batch := pairs[i:end]

		placeholders := strings.Repeat("(?,?),", len(batch)-1) + "(?,?)"
		query := "INSERT OR IGNORE INTO content_tags (content_hash, tag_id) VALUES " + placeholders
		args := make([]interface{}, 0, len(batch)*2)
		for _, p := range batch {
			args = append(args, p.ContentHash, p.TagID)
		}

		res, err := q.Exec(query, args...)
		if err != nil {
			return 0, err
		}
		affected, _ := res.RowsAffected()
		totalAffected += affected
	}
	return totalAffected, nil
}

func (s *Store) BatchFindContentHashesByPaths(paths []string) (map[string]string, error) {
	if len(paths) == 0 {
		return make(map[string]string), nil
	}
	pathMap := make(map[string]string)

	for start := 0; start < len(paths); start += maxVars {
		end := min(start+maxVars, len(paths))
		placeholders, args := stringBatchArgs(paths[start:end])
		query := "SELECT path, content_hash FROM locations WHERE path IN (" + placeholders + ")"

		rows, err := s.Query(query, args...)
		if err != nil {
			return nil, err
		}

		for rows.Next() {
			var path, hash string
			if err := rows.Scan(&path, &hash); err != nil {
				rows.Close()
				return nil, err
			}
			pathMap[path] = hash
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return nil, err
		}
	}
	return pathMap, nil
}

func (s *Store) BatchGetLocationsByPaths(paths []string) (map[string]types.LocationInfo, error) {
	if len(paths) == 0 {
		return make(map[string]types.LocationInfo), nil
	}
	locationMap := make(map[string]types.LocationInfo)

	for start := 0; start < len(paths); start += maxVars {
		end := min(start+maxVars, len(paths))
		placeholders, args := stringBatchArgs(paths[start:end])
		query := "SELECT path, content_hash, size_bytes, mod_time FROM locations WHERE path IN (" + placeholders + ")"

		rows, err := s.Query(query, args...)
		if err != nil {
			return nil, err
		}

		for rows.Next() {
			var path string
			var loc types.LocationInfo
			if err := rows.Scan(&path, &loc.Hash, &loc.Size, &loc.ModTime); err != nil {
				rows.Close()
				return nil, err
			}
			loc.Path = path
			locationMap[path] = loc
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return nil, err
		}
	}
	return locationMap, nil
}

func (s *Store) ListSavedSearches(userID string) ([]types.SavedSearch, error) {
	rows, err := s.Query(`SELECT id, user_id, name, query, sort, "order", strftime('%s', created_at), strftime('%s', updated_at) FROM saved_searches WHERE user_id = ? ORDER BY updated_at DESC, name ASC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanSavedSearches(rows)
}

func (s *Store) GetSavedSearch(userID string, id string) (types.SavedSearch, error) {
	rows, err := s.Query(`SELECT id, user_id, name, query, sort, "order", strftime('%s', created_at), strftime('%s', updated_at) FROM saved_searches WHERE user_id = ? AND id = ?`, userID, id)
	if err != nil {
		return types.SavedSearch{}, err
	}
	defer rows.Close()
	items, err := scanSavedSearches(rows)
	if err != nil {
		return types.SavedSearch{}, err
	}
	if len(items) == 0 {
		return types.SavedSearch{}, sql.ErrNoRows
	}
	return items[0], nil
}

func (s *Store) CreateSavedSearch(item types.SavedSearch) (types.SavedSearch, error) {
	_, err := s.Exec(`
		INSERT INTO saved_searches (id, user_id, name, query, sort, "order")
		VALUES (?, ?, ?, ?, ?, ?)
	`, item.ID, item.UserID, item.Name, item.Query, item.Sort, item.Order)
	if err != nil {
		return types.SavedSearch{}, err
	}
	return s.GetSavedSearch(item.UserID, item.ID)
}

func (s *Store) UpdateSavedSearch(item types.SavedSearch) (types.SavedSearch, error) {
	res, err := s.Exec(`
		UPDATE saved_searches
		SET name = ?, query = ?, sort = ?, "order" = ?, updated_at = CURRENT_TIMESTAMP
		WHERE user_id = ? AND id = ?
	`, item.Name, item.Query, item.Sort, item.Order, item.UserID, item.ID)
	if err != nil {
		return types.SavedSearch{}, err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return types.SavedSearch{}, err
	}
	if affected == 0 {
		return types.SavedSearch{}, sql.ErrNoRows
	}
	return s.GetSavedSearch(item.UserID, item.ID)
}

func (s *Store) DeleteSavedSearch(userID string, id string) (bool, error) {
	res, err := s.Exec("DELETE FROM saved_searches WHERE user_id = ? AND id = ?", userID, id)
	if err != nil {
		return false, err
	}
	affected, err := res.RowsAffected()
	return affected > 0, err
}

func scanSavedSearches(rows *sql.Rows) ([]types.SavedSearch, error) {
	var out []types.SavedSearch
	for rows.Next() {
		var item types.SavedSearch
		if err := rows.Scan(&item.ID, &item.UserID, &item.Name, &item.Query, &item.Sort, &item.Order, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Store) BatchDisassociateTags(q Querier, pairs []ContentTagPair) (int64, error) {
	if len(pairs) == 0 {
		return 0, nil
	}
	const columns = 2
	batchSize := maxVars / columns

	var totalAffected int64
	for i := 0; i < len(pairs); i += batchSize {
		end := i + batchSize
		if end > len(pairs) {
			end = len(pairs)
		}
		batch := pairs[i:end]

		// Using row values `(content_hash, tag_id) IN (...)` is highly efficient.
		placeholders := strings.Repeat("(?,?),", len(batch)-1) + "(?,?)"
		query := "DELETE FROM content_tags WHERE (content_hash, tag_id) IN (VALUES " + placeholders + ")"

		args := make([]interface{}, 0, len(batch)*2)
		for _, p := range batch {
			args = append(args, p.ContentHash, p.TagID)
		}

		res, err := q.Exec(query, args...)
		if err != nil {
			return 0, err
		}
		affected, _ := res.RowsAffected()
		totalAffected += affected
	}
	return totalAffected, nil
}

// UpdateLocationPath updates a location's path using the provided querier (e.g., a transaction).
// It performs a simple update without any pre-checks.
func (s *Store) UpdateLocationPath(q Querier, oldPath, newPath string) error {
	_, err := q.Exec("UPDATE locations SET path = ? WHERE path = ?", newPath, oldPath)
	return err
}

// UpdateMovedLocation updates a location's path and metadata. Used for applying relink changes for moved files.
func (s *Store) UpdateMovedLocation(q Querier, oldPath string, newInfo types.LocationInfo) error {
	_, err := q.Exec(`
		UPDATE locations
		SET path = ?, size_bytes = ?, mod_time = ?, extension = ?
		WHERE path = ?`,
		newInfo.Path, newInfo.Size, newInfo.ModTime, newInfo.Extension, oldPath)
	return err
}

// BatchGetPathsForHashes finds all known paths for a given batch of content hashes.
func (s *Store) BatchGetPathsForHashes(q Querier, hashes []string) (map[string][]string, error) {
	if len(hashes) == 0 {
		return make(map[string][]string), nil
	}
	pathMap := make(map[string][]string)

	for start := 0; start < len(hashes); start += maxVars {
		end := min(start+maxVars, len(hashes))
		placeholders, args := stringBatchArgs(hashes[start:end])
		query := "SELECT content_hash, path FROM locations WHERE content_hash IN (" + placeholders + ")"

		rows, err := q.Query(query, args...)
		if err != nil {
			return nil, err
		}

		for rows.Next() {
			var hash, path string
			if err := rows.Scan(&hash, &path); err != nil {
				rows.Close()
				return nil, err
			}
			pathMap[hash] = append(pathMap[hash], path)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return nil, err
		}
	}
	return pathMap, nil
}

// GetPathsByContentQuery executes a complex query for content hashes and returns their paths.
func (s *Store) GetPathsByContentQuery(query string, args []interface{}) ([]string, error) {
	finalQuery := fmt.Sprintf(`
		WITH result_hashes(hash) AS (%s)
		SELECT l.path
		FROM locations l JOIN result_hashes rh ON l.content_hash = rh.hash
		ORDER BY l.path
	`, query)

	rows, err := s.Query(finalQuery, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanStrings(rows)
}

// GetFilesInfoByContentQuery executes a complex query for content hashes and returns their full info.
func (s *Store) GetFilesInfoByContentQuery(query string, args []interface{}) ([]types.FileInfo, error) {
	finalQuery := fmt.Sprintf(`
		WITH result_hashes(hash) AS (%s)
		SELECT l.id, l.path, l.content_hash, l.size_bytes, l.mod_time, l.added_at, l.tags_cache
		FROM locations l JOIN result_hashes rh ON l.content_hash = rh.hash
		ORDER BY l.path
	`, query)

	rows, err := s.Query(finalQuery, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanBasicFileInfos(rows)
}

// GetFilesInfoByContentQueryPage executes a complex query and returns one bounded page.
func (s *Store) GetFilesInfoByContentQueryPage(query string, args []interface{}, limit int, offset int) ([]types.FileInfo, error) {
	finalQuery := fmt.Sprintf(`
		WITH result_hashes(hash) AS (%s)
		SELECT l.id, l.path, l.content_hash, l.size_bytes, l.mod_time, l.added_at, l.tags_cache
		FROM locations l JOIN result_hashes rh ON l.content_hash = rh.hash
		ORDER BY l.path
		LIMIT ? OFFSET ?
	`, query)
	pagedArgs := append(append([]interface{}{}, args...), limit, offset)

	rows, err := s.Query(finalQuery, pagedArgs...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanBasicFileInfos(rows)
}

// GetFilesInfoByLocationQuerySorted executes a location-ID query and returns all matching rows in a validated order.
func (s *Store) GetFilesInfoByLocationQuerySorted(query string, args []interface{}, sort string, order string) ([]types.FileInfo, error) {
	finalQuery := fmt.Sprintf(`
		WITH result_locations(id) AS (%s)
		SELECT %s
		FROM locations l
		JOIN result_locations rl ON l.id = rl.id
		LEFT JOIN media_metadata mm ON mm.content_hash = l.content_hash
		ORDER BY %s
	`, query, fileInfoColumns(), fileSortClause(sort, order))
	return s.scanFileInfos(finalQuery, args...)
}

// GetFilesInfoByLocationQueryPageSorted executes a location-ID query and returns one bounded keyset page.
func (s *Store) GetFilesInfoByLocationQueryPageSorted(query string, args []interface{}, limit int, cursor *types.PageCursor, sort string, order string) ([]types.FileInfo, error) {
	cursorClause, cursorArgs, err := s.fileCursorClause(cursor, sort, order)
	if err != nil {
		return nil, err
	}
	finalQuery := fmt.Sprintf(`
		WITH result_locations(id) AS (%s)
		SELECT %s
		FROM locations l
		JOIN result_locations rl ON l.id = rl.id
		LEFT JOIN media_metadata mm ON mm.content_hash = l.content_hash
		WHERE 1=1 %s
		ORDER BY %s
		LIMIT ?
	`, query, fileInfoColumns(), cursorClause, fileSortClause(sort, order))
	pagedArgs := append(append([]interface{}{}, args...), cursorArgs...)
	pagedArgs = append(pagedArgs, limit)
	return s.scanFileInfos(finalQuery, pagedArgs...)
}

// GetFilesInfoByLocationQueryPageSortedOffset executes a location-ID query and returns one bounded offset page.
func (s *Store) GetFilesInfoByLocationQueryPageSortedOffset(query string, args []interface{}, limit int, offset int, sort string, order string) ([]types.FileInfo, error) {
	finalQuery := fmt.Sprintf(`
		WITH result_locations(id) AS (%s)
		SELECT %s
		FROM locations l
		JOIN result_locations rl ON l.id = rl.id
		LEFT JOIN media_metadata mm ON mm.content_hash = l.content_hash
		ORDER BY %s
		LIMIT ? OFFSET ?
	`, query, fileInfoColumns(), fileSortClause(sort, order))
	pagedArgs := append(append([]interface{}{}, args...), limit, offset)
	return s.scanFileInfos(finalQuery, pagedArgs...)
}

// GetAllFilesInfoSorted retrieves all tracked files in a validated order.
func (s *Store) GetAllFilesInfoSorted(sort string, order string) ([]types.FileInfo, error) {
	query := fmt.Sprintf(`
		SELECT %s
		FROM locations l
		LEFT JOIN media_metadata mm ON mm.content_hash = l.content_hash
		ORDER BY %s
	`, fileInfoColumns(), fileSortClause(sort, order))
	return s.scanFileInfos(query)
}

// GetAllFilesInfoPageSorted retrieves one bounded keyset page with a validated sort.
func (s *Store) GetAllFilesInfoPageSorted(limit int, cursor *types.PageCursor, sort string, order string) ([]types.FileInfo, error) {
	cursorClause, cursorArgs, err := s.fileCursorClause(cursor, sort, order)
	if err != nil {
		return nil, err
	}
	query := fmt.Sprintf(`
		SELECT %s
		FROM locations l
		LEFT JOIN media_metadata mm ON mm.content_hash = l.content_hash
		WHERE 1=1 %s
		ORDER BY %s
		LIMIT ?
	`, fileInfoColumns(), cursorClause, fileSortClause(sort, order))
	args := append(cursorArgs, limit)
	return s.scanFileInfos(query, args...)
}

// GetAllFilesInfoPageSortedOffset retrieves one bounded offset page with a validated sort.
func (s *Store) GetAllFilesInfoPageSortedOffset(limit int, offset int, sort string, order string) ([]types.FileInfo, error) {
	query := fmt.Sprintf(`
		SELECT %s
		FROM locations l
		LEFT JOIN media_metadata mm ON mm.content_hash = l.content_hash
		ORDER BY %s
		LIMIT ? OFFSET ?
	`, fileInfoColumns(), fileSortClause(sort, order))
	return s.scanFileInfos(query, limit, offset)
}

type fileInfoRow struct {
	file                                                    types.FileInfo
	tagsCache                                               string
	mediaKind, mimeType                                     sql.NullString
	imageWidth, imageHeight, videoWidth, videoHeight         sql.NullInt64
	frameCount, pageCount                                   sql.NullInt64
	duration                                                sql.NullFloat64
}

func (row *fileInfoRow) scanTargets(extra ...interface{}) []interface{} {
	targets := []interface{}{
		&row.file.ID, &row.file.PublicID, &row.file.Path, &row.file.Hash,
		&row.file.Size, &row.file.ModTime, &row.file.AddedAt, &row.tagsCache,
		&row.mediaKind, &row.mimeType, &row.imageWidth, &row.imageHeight,
		&row.videoWidth, &row.videoHeight, &row.duration, &row.frameCount, &row.pageCount,
	}
	return append(targets, extra...)
}

func (row *fileInfoRow) value() types.FileInfo {
	file := row.file
	file.Tags = splitTags(row.tagsCache)
	if row.mediaKind.Valid || row.mimeType.Valid {
		file.Metadata = &types.MediaMetadata{
			LocationID:      file.ID,
			MediaKind:       row.mediaKind.String,
			MimeType:        row.mimeType.String,
			ImageWidth:      nullIntPtr(row.imageWidth),
			ImageHeight:     nullIntPtr(row.imageHeight),
			VideoWidth:      nullIntPtr(row.videoWidth),
			VideoHeight:     nullIntPtr(row.videoHeight),
			DurationSeconds: nullFloatPtr(row.duration),
			FrameCount:      nullIntPtr(row.frameCount),
			PageCount:       nullIntPtr(row.pageCount),
		}
	}
	return file
}

func (s *Store) scanFileInfos(query string, args ...interface{}) ([]types.FileInfo, error) {
	rows, err := s.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var files []types.FileInfo
	for rows.Next() {
		var row fileInfoRow
		if err := rows.Scan(row.scanTargets()...); err != nil {
			return nil, err
		}
		files = append(files, row.value())
	}
	return files, rows.Err()
}

func fileInfoColumns() string {
	return `l.id, l.public_id, l.path, l.content_hash, l.size_bytes, l.mod_time, l.added_at, l.tags_cache,
		mm.media_kind, mm.mime_type, mm.image_width, mm.image_height,
		mm.video_width, mm.video_height, mm.duration_seconds, mm.frame_count, mm.page_count`
}

func fileSortExpression(sort string) string {
	switch sort {
	case "added":
		return "l.added_at"
	case "modified":
		return "l.mod_time"
	case "name":
		return "lower(l.path)"
	case "size":
		return "l.size_bytes"
	case "kind":
		return "lower(" + fileKindExpression() + ")"
	default:
		return "lower(l.path)"
	}
}

func fileSortClause(sort string, order string) string {
	if sort == "added" {
		return addedOrderSortClause(order)
	}
	return fmt.Sprintf("%s %s, l.id ASC", fileSortExpression(sort), sortOrder(order))
}

func fileKindExpression() string {
	return `CASE
		WHEN lower(l.extension) = '.cbz' THEN 'comic'
		ELSE coalesce(mm.media_kind, CASE
			WHEN lower(l.extension) = '.gif' THEN 'gif'
			WHEN lower(l.extension) IN ('.jpg', '.jpeg', '.png', '.webp', '.bmp', '.tif', '.tiff', '.heic', '.heif') THEN 'photo'
			WHEN lower(l.extension) IN ('.mp4', '.mov', '.avi', '.mkv', '.webm', '.flv', '.wmv', '.mpeg', '.mpg') THEN 'video'
			ELSE 'other'
		END)
	END`
}

func (s *Store) fileCursorClause(cursor *types.PageCursor, sort string, order string) (string, []interface{}, error) {
	if cursor == nil {
		return "", nil, nil
	}
	if sort == "added" {
		return s.addedOrderCursorClause(cursor, order)
	}
	expr := fileSortExpression(sort)
	key, err := s.cursorKeyForLocation(cursor.ID, sort)
	if err != nil {
		return "", nil, err
	}
	comparison := ">"
	if strings.EqualFold(order, "desc") {
		comparison = "<"
	}
	return fmt.Sprintf("AND (%s %s ? OR (%s = ? AND l.id > ?))", expr, comparison, expr), []interface{}{key, key, cursor.ID}, nil
}

func (s *Store) cursorKeyForLocation(locationID int64, sort string) (interface{}, error) {
	query := fmt.Sprintf(`
		SELECT %s
		FROM locations l
		LEFT JOIN media_metadata mm ON mm.content_hash = l.content_hash
		WHERE l.id = ?
	`, fileSortExpression(sort))
	switch sort {
	case "added", "modified", "size":
		var value int64
		if err := s.QueryRow(query, locationID).Scan(&value); err != nil {
			return nil, err
		}
		return value, nil
	default:
		var value string
		if err := s.QueryRow(query, locationID).Scan(&value); err != nil {
			return nil, err
		}
		return strings.ToLower(value), nil
	}
}

func nullIntPtr(value sql.NullInt64) *int {
	if !value.Valid {
		return nil
	}
	out := int(value.Int64)
	return &out
}

func nullFloatPtr(value sql.NullFloat64) *float64 {
	if !value.Valid {
		return nil
	}
	out := value.Float64
	return &out
}

func sortOrder(order string) string {
	if strings.EqualFold(order, "desc") {
		return "DESC"
	}
	return "ASC"
}

// GetCountByContentQuery executes a complex query for content hashes and returns their count.
func (s *Store) GetCountByContentQuery(query string, args []interface{}) (int, error) {
	// The subquery returns a list of unique content hashes. We just need to count them.
	finalQuery := fmt.Sprintf(`SELECT COUNT(*) FROM (%s)`, query)

	var count int
	err := s.QueryRow(finalQuery, args...).Scan(&count)
	return count, err
}

// GetLocationCountByContentQuery counts tracked file locations matching a content-hash query.
func (s *Store) GetLocationCountByContentQuery(query string, args []interface{}) (int, error) {
	finalQuery := fmt.Sprintf(`
		WITH result_hashes(hash) AS (%s)
		SELECT COUNT(*)
		FROM locations l
		JOIN result_hashes rh ON l.content_hash = rh.hash
	`, query)
	var count int
	err := s.QueryRow(finalQuery, args...).Scan(&count)
	return count, err
}

// GetCountByLocationQuery counts rows produced by a location-ID query.
func (s *Store) GetCountByLocationQuery(query string, args []interface{}) (int, error) {
	finalQuery := fmt.Sprintf(`SELECT COUNT(*) FROM (%s)`, query)
	var count int
	err := s.QueryRow(finalQuery, args...).Scan(&count)
	return count, err
}

// ExistsByContentQuery executes a complex query and checks for the existence of any match.
func (s *Store) ExistsByContentQuery(query string, args []interface{}) (bool, error) {
	finalQuery := fmt.Sprintf(`SELECT EXISTS (%s)`, query)

	var found bool
	err := s.QueryRow(finalQuery, args...).Scan(&found)
	return found, err
}

// GetHashesByContentQueryTx executes a complex query for content hashes and returns them within a transaction.
func (s *Store) GetHashesByContentQueryTx(q Querier, subQuery string, args []interface{}) ([]string, error) {
	rows, err := q.Query(subQuery, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanStrings(rows)
}

// BatchClearTagsByContentQueryTx removes all tag associations for content matching a subquery.
func (s *Store) BatchClearTagsByContentQueryTx(q Querier, subQuery string, args []interface{}) (int64, error) {
	query := fmt.Sprintf("DELETE FROM content_tags WHERE content_hash IN (%s)", subQuery)
	res, err := q.Exec(query, args...)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// BatchAssociateTagsByContentQueryTx associates tags with content matching a subquery.
func (s *Store) BatchAssociateTagsByContentQueryTx(q Querier, subQuery string, args []interface{}, tagIDs []int64) (int64, error) {
	if len(tagIDs) == 0 {
		return 0, nil
	}

	batchSize := maxVars - len(args)
	if batchSize <= 0 {
		return 0, fmt.Errorf("content query uses %d bind variables, leaving no room for tag association", len(args))
	}

	var totalAffected int64
	for start := 0; start < len(tagIDs); start += batchSize {
		end := start + batchSize
		if end > len(tagIDs) {
			end = len(tagIDs)
		}
		batch := tagIDs[start:end]

		placeholders := strings.Repeat("(?),", len(batch)-1) + "(?)"
		query := fmt.Sprintf(`WITH tag_ids(tag_id) AS (VALUES %s)
			INSERT OR IGNORE INTO content_tags (content_hash, tag_id)
			SELECT selected_hashes.hash, tag_ids.tag_id
			FROM (%s) AS selected_hashes
			CROSS JOIN tag_ids`, placeholders, subQuery)

		finalArgs := make([]interface{}, 0, len(args)+len(batch))
		for _, tagID := range batch {
			finalArgs = append(finalArgs, tagID)
		}
		finalArgs = append(finalArgs, args...)

		res, err := q.Exec(query, finalArgs...)
		if err != nil {
			return 0, err
		}
		affected, _ := res.RowsAffected()
		totalAffected += affected
	}

	return totalAffected, nil
}

// BatchDisassociateTagsByContentQueryTx disassociates tags from content matching a subquery.
func (s *Store) BatchDisassociateTagsByContentQueryTx(q Querier, subQuery string, args []interface{}, tagIDs []int64) (int64, error) {
	if len(tagIDs) == 0 {
		return 0, nil
	}

	batchSize := maxVars - len(args)
	if batchSize <= 0 {
		return 0, fmt.Errorf("content query uses %d bind variables, leaving no room for tag disassociation", len(args))
	}

	var totalAffected int64
	for start := 0; start < len(tagIDs); start += batchSize {
		end := start + batchSize
		if end > len(tagIDs) {
			end = len(tagIDs)
		}
		batch := tagIDs[start:end]

		placeholders := strings.Repeat("?,", len(batch)-1) + "?"
		query := fmt.Sprintf(
			"DELETE FROM content_tags WHERE tag_id IN (%s) AND content_hash IN (%s)",
			placeholders,
			subQuery,
		)

		finalArgs := make([]interface{}, 0, len(args)+len(batch))
		for _, id := range batch {
			finalArgs = append(finalArgs, id)
		}
		finalArgs = append(finalArgs, args...)

		res, err := q.Exec(query, finalArgs...)
		if err != nil {
			return 0, err
		}
		affected, _ := res.RowsAffected()
		totalAffected += affected
	}

	return totalAffected, nil
}

// RemoveLocationsByContentQueryTx removes locations for content matching a subquery.
// The cleanup_orphan_content_on_delete trigger will handle cleaning up content and tags.
func (s *Store) RemoveLocationsByContentQueryTx(q Querier, subQuery string, args []interface{}) (int64, error) {
	query := fmt.Sprintf("DELETE FROM locations WHERE content_hash IN (%s)", subQuery)
	res, err := q.Exec(query, args...)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// UpdateLocationMetadata updates the size and modtime for a given path.
func (s *Store) UpdateLocationMetadata(path string, size int64, modTime int64) error {
	_, err := s.Exec("UPDATE locations SET size_bytes = ?, mod_time = ? WHERE path = ?", size, modTime, path)
	return err
}

// BatchGetTagCounts retrieves the files_count for a batch of tags.
// Returns a map of the original tag string to its count.
func (s *Store) BatchGetTagCounts(parsedTags []types.ParsedTag) (map[string]int, error) {
	counts := make(map[string]int)
	if len(parsedTags) == 0 {
		return counts, nil
	}

	var placeholders []string
	var args []interface{}
	for _, t := range parsedTags {
		placeholders = append(placeholders, "(?, ?)")
		args = append(args, t.Key, t.Value)
	}
	// Note: Using row value constructor `(key, value) IN ((?,?), ...)`
	query := `SELECT key, value, files_count FROM tags WHERE (key, value) IN (` + strings.Join(placeholders, ",") + `)`

	rows, err := s.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var key, value string
		var count int
		if err := rows.Scan(&key, &value, &count); err != nil {
			return nil, err
		}
		counts[parsedTagString(types.ParsedTag{Key: key, Value: value})] = count
	}
	return counts, rows.Err()
}

// TransferTagsAndRehashLocation transactionally updates a location to a new content hash,
// moving all tags from the old hash to the new one.
func (s *Store) TransferTagsAndRehashLocation(oldHash, newHash string, newLoc types.LocationInfo) error {
	tx, err := s.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// 1. Check if the new hash represents existing content.
	var dummy int
	err = tx.QueryRow("SELECT 1 FROM contents WHERE hash = ? LIMIT 1", newHash).Scan(&dummy)
	newContentExists := err == nil
	if err != nil && err != sql.ErrNoRows {
		return fmt.Errorf("failed to check for new content existence: %w", err)
	}

	if newContentExists {
		// The new content is a duplicate of something else. Merge tags from the old content into it.
		_, err := tx.Exec(`
            INSERT OR IGNORE INTO content_tags (content_hash, tag_id)
            SELECT ?, tag_id FROM content_tags WHERE content_hash = ?`, newHash, oldHash)
		if err != nil {
			return fmt.Errorf("failed to merge tags to existing content: %w", err)
		}
	} else {
		// This is brand new content. Insert it and re-assign all tags from the old content.
		if _, err := tx.Exec("INSERT INTO contents (hash) VALUES (?)", newHash); err != nil {
			return fmt.Errorf("failed to insert new content: %w", err)
		}
		if _, err := tx.Exec("UPDATE content_tags SET content_hash = ? WHERE content_hash = ?", newHash, oldHash); err != nil {
			return fmt.Errorf("failed to reassign tags: %w", err)
		}
	}

	// 2. Update the location record to point to the new hash and metadata.
	// The triggers will handle updating the tags_cache.
	_, err = tx.Exec(`
        UPDATE locations SET content_hash = ?, size_bytes = ?, mod_time = ?, extension = ?
        WHERE path = ?`, newHash, newLoc.Size, newLoc.ModTime, newLoc.Extension, newLoc.Path)
	if err != nil {
		return fmt.Errorf("failed to update location record: %w", err)
	}

	// 3. Delete the old, now-obsolete content record.
	// This will cascade-delete its (now empty or merged) tag associations from content_tags.
	if _, err := tx.Exec("DELETE FROM contents WHERE hash = ?", oldHash); err != nil {
		return fmt.Errorf("failed to delete old content record: %w", err)
	}

	return tx.Commit()
}

// formatTag is a helper to format a ParsedTag back to a string for display in errors.
func formatTag(tag types.ParsedTag) string {
	if tag.Value == "" {
		return tag.Key
	}
	return tag.Key + ":" + tag.Value
}

// RenameTag atomically renames a tag. It will return an error if the new
// tag name already exists or if the old tag name does not exist.
// It also handles updating the tags_cache for all affected files.
func (s *Store) RenameTag(oldTag, newTag types.ParsedTag) error {
	tx, err := s.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// 1. Check if new tag already exists.
	var newTagExists int
	err = tx.QueryRow("SELECT 1 FROM tags WHERE key = ? AND value = ? LIMIT 1", newTag.Key, newTag.Value).Scan(&newTagExists)
	if err != nil && err != sql.ErrNoRows {
		return fmt.Errorf("failed to check for new tag existence: %w", err)
	}
	if err == nil { // No error means a row was found
		return fmt.Errorf("new tag '%s' already exists", formatTag(newTag))
	}

	// 2. Find old tag ID.
	var oldTagID int64
	err = tx.QueryRow("SELECT id FROM tags WHERE key = ? AND value = ?", oldTag.Key, oldTag.Value).Scan(&oldTagID)
	if err == sql.ErrNoRows {
		return fmt.Errorf("tag '%s' not found", formatTag(oldTag))
	}
	if err != nil {
		return fmt.Errorf("failed to find old tag: %w", err)
	}

	// 3. Update the tag.
	_, err = tx.Exec("UPDATE tags SET key = ?, value = ? WHERE id = ?", newTag.Key, newTag.Value, oldTagID)
	if err != nil {
		// This could be a UNIQUE constraint violation if there's a race condition,
		// though the initial check should prevent it.
		return fmt.Errorf("failed to update tag: %w", err)
	}

	// 4. Invalidate and rebuild the tags_cache for all affected content.
	// We update the cache on all `locations` records whose `content_hash`
	// is associated with the renamed tag.
	rebuildCacheQuery := `
        UPDATE locations
        SET tags_cache = (
            SELECT IFNULL(GROUP_CONCAT(tag_str, ' '), '')
            FROM (
                     SELECT CASE WHEN t.value = '' THEN t.key ELSE t.key || ':' || t.value END AS tag_str
                     FROM tags t
                              JOIN content_tags ct ON t.id = ct.tag_id
                     WHERE ct.content_hash = locations.content_hash
                     ORDER BY t.key, t.value
                 )
        )
        WHERE content_hash IN (SELECT content_hash FROM content_tags WHERE tag_id = ?)`

	_, err = tx.Exec(rebuildCacheQuery, oldTagID)
	if err != nil {
		return fmt.Errorf("failed to update tags cache for affected files: %w", err)
	}

	return tx.Commit()
}

// ListAllPublicFileIDs returns stable public API identifiers without materializing full file metadata.
func (s *Store) ListAllPublicFileIDs() ([]string, error) {
	rows, err := s.Query("SELECT public_id FROM locations WHERE public_id IS NOT NULL AND public_id <> '' ORDER BY public_id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanStringsInto(rows, []string{})
}

// GetPublicFileIDsByLocationQuery executes a location-ID subquery and projects only public IDs.
func (s *Store) GetPublicFileIDsByLocationQuery(query string, args []interface{}) ([]string, error) {
	finalQuery := fmt.Sprintf(`
		WITH result_locations(id) AS (%s)
		SELECT l.public_id
		FROM locations l
		JOIN result_locations rl ON l.id = rl.id
		WHERE l.public_id IS NOT NULL AND l.public_id <> ''
		ORDER BY l.public_id`, query)
	rows, err := s.Query(finalQuery, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanStringsInto(rows, []string{})
}
