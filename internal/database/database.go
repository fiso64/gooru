package database

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"strings"

	"gooru.local/gooru/internal/types"
	_ "github.com/mattn/go-sqlite3"
)

type Store struct {
	DB *sql.DB
}

// NewStore initializes the database connection and creates the schema if it doesn't exist.
func NewStore(dataSourceName string) (*Store, error) {
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

	return &Store{DB: db}, nil
}

// Close closes the database connection.
func (s *Store) Close() error {
	return s.DB.Close()
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
		`CREATE INDEX IF NOT EXISTS idx_content_tags_tag_id ON content_tags(tag_id);`,

		/* TRIGGERS FOR MAINTAINING tags_cache */
		`CREATE TRIGGER IF NOT EXISTS populate_tags_cache_on_location_insert
		AFTER INSERT ON locations
		BEGIN
			UPDATE locations
			SET tags_cache = (
				SELECT IFNULL(GROUP_CONCAT(tag_str), '')
				FROM (
					SELECT CASE WHEN t.key = '' THEN t.value ELSE t.key || ':' || t.value END AS tag_str
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
					SELECT CASE WHEN t.key = '' THEN t.value ELSE t.key || ':' || t.value END AS tag_str
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
					SELECT CASE WHEN t.key = '' THEN t.value ELSE t.key || ':' || t.value END AS tag_str
					FROM tags t
					JOIN content_tags ct ON t.id = ct.tag_id
					WHERE ct.content_hash = OLD.content_hash
					ORDER BY t.key, t.value
				)
			)
			WHERE content_hash = OLD.content_hash;
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
func (s *Store) GetOrCreateContent(q Querier, hash string) error {
	_, err := q.Exec("INSERT OR IGNORE INTO contents (hash) VALUES (?)", hash)
	return err
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
	err := s.DB.QueryRow("SELECT content_hash FROM locations WHERE path = ?", path).Scan(&hash)
	if err != nil {
		if err == sql.ErrNoRows {
			return "", nil // Return empty string and no error if not found
		}
		return "", err
	}
	return hash, nil
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
	err := s.DB.QueryRow("SELECT id FROM tags WHERE key = ? AND value = ?", key, value).Scan(&id)
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
	rows, err := s.DB.Query(query, hash)
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
		if key == "" {
			tags = append(tags, value)
		} else {
			tags = append(tags, key+":"+value)
		}
	}
	return tags, nil
}

// ListAllFiles retrieves all file paths from the database.
func (s *Store) ListAllFiles() ([]string, error) {
	rows, err := s.DB.Query("SELECT path FROM locations ORDER BY path")
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
	rows, err := s.DB.Query("SELECT content_hash, tags_cache FROM locations GROUP BY content_hash")
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
	rows, err := s.DB.Query(`
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
	rows, err := s.DB.Query("SELECT hash FROM contents")
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
	rows, err := s.DB.Query("SELECT size_bytes, content_hash FROM locations")
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
		rows, err := s.DB.Query("SELECT path, content_hash, size_bytes, mod_time, extension, tags_cache FROM locations WHERE path LIKE ?", dir+string(filepath.Separator)+"%")
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

	tx, err := s.DB.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

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
			res, err := tx.Exec(queryBuilder.String(), args...)
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
		res, err := tx.Exec(queryBuilder.String(), args...)
		if err != nil {
			return 0, err
		}
		added, _ := res.RowsAffected()
		locationsAdded += int(added)
	}

	return locationsAdded, tx.Commit()
}

// RemoveLocationsByPath transactionally removes locations by their paths.
func (s *Store) RemoveLocationsByPath(paths []string) (int, error) {
	if len(paths) == 0 {
		return 0, nil
	}

	tx, err := s.DB.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	query := "DELETE FROM locations WHERE path IN (?" + strings.Repeat(",?", len(paths)-1) + ")"
	args := make([]interface{}, len(paths))
	for i, v := range paths {
		args[i] = v
	}
	res, err := tx.Exec(query, args...)
	if err != nil {
		return 0, err
	}
	removed, _ := res.RowsAffected()

	return int(removed), tx.Commit()
}

// UpdatePath updates a location's path, with checks for existence.
func (s *Store) UpdatePath(absOldPath, absNewPath, displayOldPath, displayNewPath string) error {
	tx, err := s.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// 1. Check if new path already exists
	var dummy int
	err = tx.QueryRow("SELECT 1 FROM locations WHERE path = ?", absNewPath).Scan(&dummy)
	if err != sql.ErrNoRows {
		if err == nil { // A row was found
			return fmt.Errorf("new path already exists in database: %s", displayNewPath)
		}
		return fmt.Errorf("db check for new path failed: %w", err) // Other DB error
	}

	// 2. Perform the update
	res, err := tx.Exec("UPDATE locations SET path = ? WHERE path = ?", absNewPath, absOldPath)
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

	rows, err := s.DB.Query(query, args...)
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

	rows, err := s.DB.Query(query)
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

	rows, err := s.DB.Query(query, key, value)
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

	rows, err := s.DB.Query(query, args...)
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
	rows, err := s.DB.Query(query)
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
		if key == "" {
			tags = append(tags, value)
		} else {
			tags = append(tags, key+":"+value)
		}
	}
	return tags, nil
}