package database

import (
	"database/sql"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"

	"gooru.local/gooru/types"
	_ "github.com/mattn/go-sqlite3"
)

type Store struct {
	DB     *sql.DB
	logger *log.Logger
}

// Tx is a transaction wrapper that logs queries.
type Tx struct {
	*sql.Tx
	logger *log.Logger
}

// NewStore initializes the database connection and creates the schema if it doesn't exist.
func NewStore(dataSourceName string, verbose bool) (*Store, error) {
	db, err := sql.Open("sqlite3", dataSourceName)
	if err != nil {
		return nil, err
	}

	if err = db.Ping(); err != nil {
		return nil, err
	}

	if err = createTables(db); err != nil {
		return nil, err
	}

	var logOutput io.Writer
	if verbose {
		logOutput = os.Stderr
	} else {
		logOutput = io.Discard
	}
	logger := log.New(logOutput, "SQL: ", log.Ltime|log.Lmicroseconds)

	return &Store{DB: db, logger: logger}, nil
}

// Close closes the database connection.
func (s *Store) Close() error {
	return s.DB.Close()
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
	s.logger.Printf("QUERY: %s\n-- ARGS: %v", query, args)
	return s.DB.Exec(query, args...)
}
func (s *Store) Query(query string, args ...interface{}) (*sql.Rows, error) {
	s.logger.Printf("QUERY: %s\n-- ARGS: %v", query, args)
	return s.DB.Query(query, args...)
}
func (s *Store) QueryRow(query string, args ...interface{}) *sql.Row {
	s.logger.Printf("QUERY: %s\n-- ARGS: %v", query, args)
	return s.DB.QueryRow(query, args...)
}

// Querier implementations for logging on Tx
func (tx *Tx) Exec(query string, args ...interface{}) (sql.Result, error) {
	tx.logger.Printf("QUERY: %s\n-- ARGS: %v", query, args)
	return tx.Tx.Exec(query, args...)
}
func (tx *Tx) Query(query string, args ...interface{}) (*sql.Rows, error) {
	tx.logger.Printf("QUERY: %s\n-- ARGS: %v", query, args)
	return tx.Tx.Query(query, args...)
}
func (tx *Tx) QueryRow(query string, args ...interface{}) *sql.Row {
	tx.logger.Printf("QUERY: %s\n-- ARGS: %v", query, args)
	return tx.Tx.QueryRow(query, args...)
}

// createTables creates the necessary tables and indexes for the application.
func createTables(db *sql.DB) error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS contents (
			hash TEXT PRIMARY KEY,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		);`,
		`CREATE TABLE IF NOT EXISTS locations (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			content_hash TEXT NOT NULL,
			path TEXT NOT NULL UNIQUE,
			size_bytes INTEGER NOT NULL,
			mod_time INTEGER NOT NULL,
			extension TEXT NOT NULL,
			tags_cache TEXT NOT NULL DEFAULT '',
			FOREIGN KEY (content_hash) REFERENCES contents(hash) ON DELETE CASCADE
		);`,
		`CREATE TABLE IF NOT EXISTS tags (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			key TEXT NOT NULL COLLATE NOCASE,
			value TEXT NOT NULL COLLATE NOCASE,
			UNIQUE(key, value)
		);`,
		`CREATE TABLE IF NOT EXISTS content_tags (
			content_hash TEXT NOT NULL,
			tag_id INTEGER NOT NULL,
			PRIMARY KEY (content_hash, tag_id),
			FOREIGN KEY (content_hash) REFERENCES contents(hash) ON DELETE CASCADE,
			FOREIGN KEY (tag_id) REFERENCES tags(id) ON DELETE CASCADE
		);`,
		`CREATE INDEX IF NOT EXISTS idx_locations_path ON locations(path);`,
		`CREATE INDEX IF NOT EXISTS idx_locations_content_hash ON locations(content_hash);`,
		`CREATE INDEX IF NOT EXISTS idx_content_tags_tag_id ON content_tags(tag_id);`,
		`CREATE INDEX IF NOT EXISTS idx_locations_extension_lower ON locations(lower(extension));`,

		/* TRIGGERS FOR MAINTAINING tags_cache */
		`CREATE TRIGGER IF NOT EXISTS populate_tags_cache_on_location_insert
		AFTER INSERT ON locations
		BEGIN
			UPDATE locations
			SET tags_cache = (
				SELECT IFNULL(GROUP_CONCAT(tag_str), '')
				FROM (
					SELECT CASE WHEN t.value = '' THEN t.key ELSE t.key || ':' || t.value END AS tag_str
					FROM tags t
					JOIN content_tags ct ON t.id = ct.tag_id
					WHERE ct.content_hash = NEW.content_hash
					ORDER BY t.key, t.value
				)
			)
			WHERE id = NEW.id;
		END;`,

		`CREATE TRIGGER IF NOT EXISTS update_tags_cache_on_insert
		AFTER INSERT ON content_tags
		BEGIN
			UPDATE locations
			SET tags_cache = (
				SELECT IFNULL(GROUP_CONCAT(tag_str), '')
				FROM (
					SELECT CASE WHEN t.value = '' THEN t.key ELSE t.key || ':' || t.value END AS tag_str
					FROM tags t
					JOIN content_tags ct ON t.id = ct.tag_id
					WHERE ct.content_hash = NEW.content_hash
					ORDER BY t.key, t.value
				)
			)
			WHERE content_hash = NEW.content_hash;
		END;`,

		`CREATE TRIGGER IF NOT EXISTS update_tags_cache_on_delete
		AFTER DELETE ON content_tags
		BEGIN
			UPDATE locations
			SET tags_cache = (
				SELECT IFNULL(GROUP_CONCAT(tag_str), '')
				FROM (
					SELECT CASE WHEN t.value = '' THEN t.key ELSE t.key || ':' || t.value END AS tag_str
					FROM tags t
					JOIN content_tags ct ON t.id = ct.tag_id
					WHERE ct.content_hash = OLD.content_hash
					ORDER BY t.key, t.value
				)
			)
			WHERE content_hash = OLD.content_hash;
		END;`,

		/* TRIGGER FOR CLEANING UP ORPHANED TAGS */
		`CREATE TRIGGER IF NOT EXISTS cleanup_orphan_tags_on_delete
		AFTER DELETE ON content_tags
		BEGIN
			DELETE FROM tags
			WHERE id = OLD.tag_id
			AND NOT EXISTS (
				SELECT 1 FROM content_tags WHERE tag_id = OLD.tag_id
			);
		END;`,
	}

	for _, stmt := range statements {
		if _, err := db.Exec(stmt); err != nil {
			return err
		}
	}

	return nil
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

// GetOrCreateLocation ensures a file path for a given content hash exists.
// The tags_cache will be populated by a database trigger.
func (s *Store) GetOrCreateLocation(q Querier, hash, path string, size int64, modTime int64, extension string) error {
	_, err := q.Exec("INSERT OR IGNORE INTO locations (content_hash, path, size_bytes, mod_time, extension) VALUES (?, ?, ?, ?, ?)", hash, path, size, modTime, extension)
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
	_, err := q.Exec("INSERT INTO locations (content_hash, path, size_bytes, mod_time, extension) VALUES (?, ?, ?, ?, ?)", hash, path, size, modTime, extension)
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

	var tags []string
	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			return nil, err
		}
		if value == "" {
			tags = append(tags, key)
		} else {
			tags = append(tags, key+":"+value)
		}
	}
	return tags, nil
}

// ListAllFiles retrieves all file paths from the database.
func (s *Store) ListAllFiles() ([]string, error) {
	rows, err := s.Query("SELECT path FROM locations ORDER BY path")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var paths []string
	for rows.Next() {
		var path string
		if err := rows.Scan(&path); err != nil {
			return nil, err
		}
		paths = append(paths, path)
	}
	return paths, nil
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
	return cacheMap, nil
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

	var paths []string
	for rows.Next() {
		var path string
		if err := rows.Scan(&path); err != nil {
			return nil, err
		}
		paths = append(paths, path)
	}
	return paths, nil
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
	return hashes, nil
}

// GetSizeToHashesMap retrieves a map of file sizes to a list of hashes of files with that size.
func (s *Store) GetSizeToHashesMap() (map[int64][]string, error) {
	rows, err := s.Query("SELECT size_bytes, content_hash FROM locations")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

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

	// Convert to the final structure
	finalMap := make(map[int64][]string)
	for size, hashes := range tempMap {
		finalMap[size] = make([]string, 0, len(hashes))
		for hash := range hashes {
			finalMap[size] = append(finalMap[size], hash)
		}
	}
	return finalMap, nil
}

// GetLocationsForDirs retrieves a map of all known paths to their content hashes for the given directories.
func (s *Store) GetLocationsForDirs(dirs []string) (map[string]types.LocationInfo, error) {
	locations := make(map[string]types.LocationInfo)
	for _, dir := range dirs {
		rows, err := s.Query("SELECT path, content_hash, size_bytes, mod_time, extension, tags_cache FROM locations WHERE path LIKE ?", dir+string(filepath.Separator)+"%")
		if err != nil {
			return nil, err
		}
		defer rows.Close()

		for rows.Next() {
			var path string
			var info types.LocationInfo
			if err := rows.Scan(&path, &info.Hash, &info.Size, &info.ModTime, &info.Extension, &info.TagsCache); err != nil {
				return nil, err
			}
			locations[path] = info
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
	const batchSize = 250
	var args []interface{}
	var queryBuilder strings.Builder
	// The tags_cache is now populated by triggers, so we don't insert it here.
	queryBuilder.WriteString("INSERT OR IGNORE INTO locations (content_hash, path, size_bytes, mod_time, extension) VALUES ")

	itemsInBatch := 0
	for path, info := range toAdd {
		if itemsInBatch > 0 {
			queryBuilder.WriteString(", ")
		}
		queryBuilder.WriteString("(?, ?, ?, ?, ?)")
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
			queryBuilder.WriteString("INSERT OR IGNORE INTO locations (content_hash, path, size_bytes, mod_time, extension) VALUES ")
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
	const columns = 1
	batchSize := maxVars / columns

	var totalRemoved int
	for i := 0; i < len(paths); i += batchSize {
		end := i + batchSize
		if end > len(paths) {
			end = len(paths)
		}
		batch := paths[i:end]

		placeholders := strings.Repeat("?,", len(batch)-1) + "?"
		query := "DELETE FROM locations WHERE path IN (" + placeholders + ")"
		args := make([]interface{}, len(batch))
		for j, v := range batch {
			args[j] = v
		}
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

// ListFilesByTagsAnd retrieves all file paths for a given set of tags (AND query).
func (s *Store) ListFilesByTagsAnd(tags []types.ParsedTag) ([]string, error) {
	if len(tags) == 0 {
		return []string{}, nil
	}

	var whereClauses []string
	var args []interface{}
	for _, tag := range tags {
		whereClauses = append(whereClauses, "(t.key = ? AND t.value = ?)")
		args = append(args, tag.Key, tag.Value)
	}
	whereCondition := strings.Join(whereClauses, " OR ")

	query := `
		SELECT l.path
		FROM locations l
		WHERE l.content_hash IN (
			SELECT ct.content_hash
			FROM content_tags ct
			JOIN tags t ON ct.tag_id = t.id
			WHERE ` + whereCondition + `
			GROUP BY ct.content_hash
			HAVING COUNT(t.id) = ?
		)
		ORDER BY l.path
	`
	args = append(args, len(tags))

	rows, err := s.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var paths []string
	for rows.Next() {
		var path string
		if err := rows.Scan(&path); err != nil {
			return nil, err
		}
		paths = append(paths, path)
	}
	return paths, nil
}

// GetAllFilesInfo retrieves detailed info for all files from the database using the cache.
func (s *Store) GetAllFilesInfo() ([]types.FileInfo, error) {
	query := `SELECT path, size_bytes, tags_cache FROM locations ORDER BY path`

	rows, err := s.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var files []types.FileInfo
	for rows.Next() {
		var file types.FileInfo
		if err := rows.Scan(&file.Path, &file.Size, &file.Tags); err != nil {
			return nil, err
		}
		files = append(files, file)
	}
	return files, nil
}

// GetFilesInfoByTag retrieves info for all files for a given tag using the cache.
func (s *Store) GetFilesInfoByTag(key, value string) ([]types.FileInfo, error) {
	query := `
		SELECT l.path, l.size_bytes, l.tags_cache
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

	var files []types.FileInfo
	for rows.Next() {
		var file types.FileInfo
		if err := rows.Scan(&file.Path, &file.Size, &file.Tags); err != nil {
			return nil, err
		}
		files = append(files, file)
	}
	return files, nil
}

// GetFilesInfoByTagsAnd retrieves info for all files for a given set of tags (AND query) using the cache.
func (s *Store) GetFilesInfoByTagsAnd(tags []types.ParsedTag) ([]types.FileInfo, error) {
	if len(tags) == 0 {
		return []types.FileInfo{}, nil
	}

	var whereClauses []string
	var args []interface{}
	for _, tag := range tags {
		whereClauses = append(whereClauses, "(t.key = ? AND t.value = ?)")
		args = append(args, tag.Key, tag.Value)
	}
	whereCondition := strings.Join(whereClauses, " OR ")

	query := `
		SELECT l.path, l.size_bytes, l.tags_cache
		FROM locations l
		WHERE l.content_hash IN (
			SELECT ct.content_hash
			FROM content_tags ct
			JOIN tags t ON ct.tag_id = t.id
			WHERE ` + whereCondition + `
			GROUP BY ct.content_hash
			HAVING COUNT(t.id) = ?
		)
		ORDER BY l.path
	`
	args = append(args, len(tags))

	rows, err := s.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var files []types.FileInfo
	for rows.Next() {
		var file types.FileInfo
		if err := rows.Scan(&file.Path, &file.Size, &file.Tags); err != nil {
			return nil, err
		}
		files = append(files, file)
	}
	return files, nil
}

// GetAllTags retrieves all unique tags from the database.
func (s *Store) GetAllTags() ([]string, error) {
	query := `SELECT key, value FROM tags ORDER BY key, value`
	rows, err := s.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tags []string
	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			return nil, err
		}
		if value == "" {
			tags = append(tags, key)
		} else {
			tags = append(tags, key+":"+value)
		}
	}
	return tags, nil
}

// GetAllTagsWithCounts retrieves all tags and their usage counts.
func (s *Store) GetAllTagsWithCounts() ([]types.TagWithCount, error) {
	query := `
		SELECT
			CASE WHEN t.value = '' THEN t.key ELSE t.key || ':' || t.value END AS tag_str,
			COUNT(ct.content_hash) as usage_count
		FROM
			tags t
		JOIN
			content_tags ct ON t.id = ct.tag_id
		GROUP BY
			t.id
		ORDER BY
			usage_count DESC, tag_str ASC`
	rows, err := s.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tags []types.TagWithCount
	for rows.Next() {
		var item types.TagWithCount
		if err := rows.Scan(&item.Tag, &item.Count); err != nil {
			return nil, err
		}
		tags = append(tags, item)
	}
	return tags, nil
}

// Batch Helpers

const (
	// maxVars is a safe limit for SQLite's SQLITE_MAX_VARIABLE_NUMBER, which defaults to 999
	maxVars = 900
)

func (s *Store) BatchGetTags(q Querier, parsedTags []types.ParsedTag) (map[string]int64, error) {
	tagIDMap := make(map[string]int64)
	if len(parsedTags) == 0 {
		return tagIDMap, nil
	}

	var placeholders []string
	var args []interface{}
	for _, t := range parsedTags {
		placeholders = append(placeholders, "(?, ?)")
		args = append(args, t.Key, t.Value)
	}
	// Note: Using row value constructor `(key, value) IN ((?,?), ...)`
	query := `SELECT id, key, value FROM tags WHERE (key, value) IN (` + strings.Join(placeholders, ",") + `)`

	rows, err := q.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var id int64
		var key, value string
		if err := rows.Scan(&id, &key, &value); err != nil {
			return nil, err
		}
		var tagStr string
		if value == "" {
			tagStr = key
		} else {
			tagStr = key + ":" + value
		}
		tagIDMap[tagStr] = id
	}
	return tagIDMap, rows.Err()
}

func (s *Store) BatchGetOrCreateTags(q Querier, parsedTags []types.ParsedTag) (map[string]int64, error) {
	tagIDMap := make(map[string]int64)

	// 1. First, try to fetch all existing tags in one query
	if len(parsedTags) > 0 {
		var placeholders []string
		var args []interface{}
		for _, t := range parsedTags {
			placeholders = append(placeholders, "(?, ?)")
			args = append(args, t.Key, t.Value)
		}
		query := `SELECT id, key, value FROM tags WHERE (key, value) IN (` + strings.Join(placeholders, ",") + `)`

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
			var tagStr string
			if value == "" {
				tagStr = key
			} else {
				tagStr = key + ":" + value
			}
			tagIDMap[tagStr] = id
		}
		rows.Close()
	}

	// 2. Insert any tags that weren't found
	for _, t := range parsedTags {
		var tagStr string
		if t.Value == "" {
			tagStr = t.Key
		} else {
			tagStr = t.Key + ":" + t.Value
		}

		if _, exists := tagIDMap[tagStr]; !exists {
			res, err := q.Exec("INSERT OR IGNORE INTO tags (key, value) VALUES (?, ?)", t.Key, t.Value)
			if err != nil {
				return nil, err
			}
			id, err := res.LastInsertId()
			if err != nil {
				return nil, err
			}
			// If LastInsertId is 0, another concurrent transaction might have inserted it. Re-query.
			if id == 0 {
				err := q.QueryRow("SELECT id FROM tags WHERE key = ? AND value = ?", t.Key, t.Value).Scan(&id)
				if err != nil {
					return nil, err
				}
			}
			tagIDMap[tagStr] = id
		}
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
	const columns = 5 // content_hash, path, size_bytes, mod_time, extension
	batchSize := maxVars / columns

	locs := make([]types.LocationInfo, 0, len(locations))
	paths := make([]string, 0, len(locations))
	for path, loc := range locations {
		locs = append(locs, loc)
		paths = append(paths, path)
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
			placeholders = append(placeholders, "(?, ?, ?, ?, ?)")
			args = append(args, loc.Hash, loc.Path, loc.Size, loc.ModTime, loc.Extension)
		}
		query := `INSERT INTO locations (content_hash, path, size_bytes, mod_time, extension) VALUES ` +
			strings.Join(placeholders, ",") +
			` ON CONFLICT(path) DO UPDATE SET
				content_hash=excluded.content_hash,
				size_bytes=excluded.size_bytes,
				mod_time=excluded.mod_time,
				extension=excluded.extension`

		if _, err := q.Exec(query, args...); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) BatchClearTagsForContent(q Querier, hashes []string) error {
	if len(hashes) == 0 {
		return nil
	}
	const columns = 1
	batchSize := maxVars / columns

	for i := 0; i < len(hashes); i += batchSize {
		end := i + batchSize
		if end > len(hashes) {
			end = len(hashes)
		}
		batch := hashes[i:end]

		placeholders := strings.Repeat("?,", len(batch)-1) + "?"
		query := "DELETE FROM content_tags WHERE content_hash IN (" + placeholders + ")"
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

type ContentTagPair struct {
	ContentHash string
	TagID       int64
}

func (s *Store) BatchAssociateTags(q Querier, pairs []ContentTagPair) error {
	if len(pairs) == 0 {
		return nil
	}
	const columns = 2
	batchSize := maxVars / columns

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

		if _, err := q.Exec(query, args...); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) BatchFindContentHashesByPaths(paths []string) (map[string]string, error) {
	if len(paths) == 0 {
		return make(map[string]string), nil
	}
	pathMap := make(map[string]string)
	const columns = 1
	batchSize := maxVars / columns

	for i := 0; i < len(paths); i += batchSize {
		end := i + batchSize
		if end > len(paths) {
			end = len(paths)
		}
		batch := paths[i:end]

		placeholders := strings.Repeat("?,", len(batch)-1) + "?"
		query := "SELECT path, content_hash FROM locations WHERE path IN (" + placeholders + ")"
		args := make([]interface{}, len(batch))
		for j, p := range batch {
			args[j] = p
		}

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
		rows.Close()
	}
	return pathMap, nil
}

func (s *Store) BatchGetLocationsByPaths(paths []string) (map[string]types.LocationInfo, error) {
	if len(paths) == 0 {
		return make(map[string]types.LocationInfo), nil
	}
	locationMap := make(map[string]types.LocationInfo)
	const columns = 1
	batchSize := maxVars / columns

	for i := 0; i < len(paths); i += batchSize {
		end := i + batchSize
		if end > len(paths) {
			end = len(paths)
		}
		batch := paths[i:end]

		placeholders := strings.Repeat("?,", len(batch)-1) + "?"
		query := "SELECT path, content_hash, size_bytes, mod_time FROM locations WHERE path IN (" + placeholders + ")"
		args := make([]interface{}, len(batch))
		for j, p := range batch {
			args[j] = p
		}

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
		rows.Close()
	}
	return locationMap, nil
}

func (s *Store) BatchDisassociateTags(q Querier, pairs []ContentTagPair) error {
	if len(pairs) == 0 {
		return nil
	}
	const columns = 2
	batchSize := maxVars / columns

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

		if _, err := q.Exec(query, args...); err != nil {
			return err
		}
	}
	return nil
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
	const columns = 1
	batchSize := maxVars / columns

	for i := 0; i < len(hashes); i += batchSize {
		end := i + batchSize
		if end > len(hashes) {
			end = len(hashes)
		}
		batch := hashes[i:end]

		placeholders := strings.Repeat("?,", len(batch)-1) + "?"
		query := "SELECT content_hash, path FROM locations WHERE content_hash IN (" + placeholders + ")"
		args := make([]interface{}, len(batch))
		for j, h := range batch {
			args[j] = h
		}

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

	var paths []string
	for rows.Next() {
		var path string
		if err := rows.Scan(&path); err != nil {
			return nil, err
		}
		paths = append(paths, path)
	}
	return paths, rows.Err()
}

// GetFilesInfoByContentQuery executes a complex query for content hashes and returns their full info.
func (s *Store) GetFilesInfoByContentQuery(query string, args []interface{}) ([]types.FileInfo, error) {
	finalQuery := fmt.Sprintf(`
		WITH result_hashes(hash) AS (%s)
		SELECT l.path, l.size_bytes, l.tags_cache
		FROM locations l JOIN result_hashes rh ON l.content_hash = rh.hash
		ORDER BY l.path
	`, query)

	rows, err := s.Query(finalQuery, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var files []types.FileInfo
	for rows.Next() {
		var file types.FileInfo
		if err := rows.Scan(&file.Path, &file.Size, &file.Tags); err != nil {
			return nil, err
		}
		files = append(files, file)
	}
	return files, rows.Err()
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

	var totalAffected int64
	for _, tagID := range tagIDs {
		// We use a subquery to select the hashes and a constant for the tag_id.
		query := fmt.Sprintf(
			"INSERT OR IGNORE INTO content_tags (content_hash, tag_id) SELECT hash, ? FROM (%s)",
			subQuery,
		)

		finalArgs := make([]interface{}, 0, len(args)+1)
		finalArgs = append(finalArgs, tagID)
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

	placeholders := strings.Repeat("?,", len(tagIDs)-1) + "?"
	query := fmt.Sprintf(
		"DELETE FROM content_tags WHERE tag_id IN (%s) AND content_hash IN (%s)",
		placeholders,
		subQuery,
	)

	finalArgs := make([]interface{}, 0, len(args)+len(tagIDs))
	for _, id := range tagIDs {
		finalArgs = append(finalArgs, id)
	}
	finalArgs = append(finalArgs, args...)

	res, err := q.Exec(query, finalArgs...)
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



