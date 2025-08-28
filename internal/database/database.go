package database

import (
	"database/sql"
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
			name TEXT NOT NULL UNIQUE COLLATE NOCASE
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
		`CREATE TRIGGER IF NOT EXISTS update_tags_cache_on_insert
		AFTER INSERT ON content_tags
		BEGIN
			UPDATE locations
			SET tags_cache = (
				SELECT IFNULL(GROUP_CONCAT(t.name ORDER BY t.name), '')
				FROM tags t
				JOIN content_tags ct ON t.id = ct.tag_id
				WHERE ct.content_hash = NEW.content_hash
			)
			WHERE content_hash = NEW.content_hash;
		END;`,

		`CREATE TRIGGER IF NOT EXISTS update_tags_cache_on_delete
		AFTER DELETE ON content_tags
		BEGIN
			UPDATE locations
			SET tags_cache = (
				SELECT IFNULL(GROUP_CONCAT(t.name ORDER BY t.name), '')
				FROM tags t
				JOIN content_tags ct ON t.id = ct.tag_id
				WHERE ct.content_hash = OLD.content_hash
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
// It initializes the tags_cache to an empty string.
func (s *Store) GetOrCreateLocation(q Querier, hash, path string, size int64, modTime int64, extension string) error {
	_, err := q.Exec("INSERT OR IGNORE INTO locations (content_hash, path, size_bytes, mod_time, extension, tags_cache) VALUES (?, ?, ?, ?, ?, '')", hash, path, size, modTime, extension)
	return err
}

// UpdateContentLocation atomically replaces the location for a given content hash.
// This handles file moves/renames cleanly.
func (s *Store) UpdateContentLocation(q Querier, hash, path string, size int64, modTime int64, extension string) error {
	// Remove any old locations for this content.
	if _, err := q.Exec("DELETE FROM locations WHERE content_hash = ?", hash); err != nil {
		return err
	}
	// Insert the new, current location. The tags_cache will be updated by AssociateTag.
	_, err := q.Exec("INSERT INTO locations (content_hash, path, size_bytes, mod_time, extension, tags_cache) VALUES (?, ?, ?, ?, ?, '')", hash, path, size, modTime, extension)
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

// GetOrCreateTag finds a tag by name or creates it, returning its ID.
func (s *Store) GetOrCreateTag(q Querier, name string) (int64, error) {
	var id int64
	err := q.QueryRow("SELECT id FROM tags WHERE name = ?", name).Scan(&id)
	if err == nil {
		return id, nil // Found it
	}
	if err != sql.ErrNoRows {
		return 0, err // A real error occurred
	}

	// Not found, so create it
	res, err := q.Exec("INSERT INTO tags (name) VALUES (?)", name)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// GetTagID retrieves a tag's ID by its name.
func (s *Store) GetTagID(name string) (int64, error) {
	var id int64
	err := s.DB.QueryRow("SELECT id FROM tags WHERE name = ?", name).Scan(&id)
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

// GetTagsForContent retrieves all tags for a given content hash.
func (s *Store) GetTagsForContent(hash string) ([]string, error) {
	rows, err := s.DB.Query(`
		SELECT t.name 
		FROM tags t 
		JOIN content_tags ct ON t.id = ct.tag_id 
		WHERE ct.content_hash = ? ORDER BY t.name`, hash)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tags []string
	for rows.Next() {
		var tag string
		if err := rows.Scan(&tag); err != nil {
			return nil, err
		}
		tags = append(tags, tag)
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
func (s *Store) ListFilesByTag(tag string) ([]string, error) {
	rows, err := s.DB.Query(`
		SELECT l.path
		FROM locations l
		JOIN content_tags ct ON l.content_hash = ct.content_hash
		JOIN tags t ON ct.tag_id = t.id
		WHERE t.name = ? ORDER BY l.path`, tag)
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
		rows, err := s.DB.Query("SELECT path, content_hash, size_bytes, mod_time, extension FROM locations WHERE path LIKE ?", dir+string(filepath.Separator)+"%")
		if err != nil {
			return nil, err
		}
		defer rows.Close()

		for rows.Next() {
			var path string
			var info types.LocationInfo
			if err := rows.Scan(&path, &info.Hash, &info.Size, &info.ModTime, &info.Extension); err != nil {
				return nil, err
			}
			locations[path] = info
		}
	}
	return locations, nil
}

// ApplyRelinkChanges transactionally removes old paths and adds new ones.
func (s *Store) ApplyRelinkChanges(toAdd map[string]types.LocationInfo, toRemove []string) (types.RelinkStats, error) {
	stats := types.RelinkStats{}
	if len(toAdd) == 0 && len(toRemove) == 0 {
		return stats, nil // Nothing to do
	}

	tx, err := s.DB.Begin()
	if err != nil {
		return stats, err
	}
	defer tx.Rollback()

	// 1. Batch remove obsolete locations
	if len(toRemove) > 0 {
		query := "DELETE FROM locations WHERE path IN (?" + strings.Repeat(",?", len(toRemove)-1) + ")"
		args := make([]interface{}, len(toRemove))
		for i, v := range toRemove {
			args[i] = v
		}
		res, err := tx.Exec(query, args...)
		if err != nil {
			return stats, err
		}
		removed, _ := res.RowsAffected()
		stats.LocationsRemoved = int(removed)
	}

	// 2. Batch insert new locations
	if len(toAdd) > 0 {
		const batchSize = 250
		var args []interface{}
		var queryBuilder strings.Builder
		queryBuilder.WriteString("INSERT OR IGNORE INTO locations (content_hash, path, size_bytes, mod_time, extension, tags_cache) VALUES ")

		itemsInBatch := 0
		for path, info := range toAdd {
			if itemsInBatch > 0 {
				queryBuilder.WriteString(", ")
			}
			queryBuilder.WriteString("(?, ?, ?, ?, ?, ?)")
			args = append(args, info.Hash, path, info.Size, info.ModTime, info.Extension, info.TagsCache)
			itemsInBatch++

			if itemsInBatch >= batchSize {
				res, err := tx.Exec(queryBuilder.String(), args...)
				if err != nil {
					return stats, err
				}
				added, _ := res.RowsAffected()
				stats.LocationsAdded += int(added)
				itemsInBatch = 0
				args = nil
				queryBuilder.Reset()
				queryBuilder.WriteString("INSERT OR IGNORE INTO locations (content_hash, path, size_bytes, mod_time, extension, tags_cache) VALUES ")
			}
		}

		if itemsInBatch > 0 {
			res, err := tx.Exec(queryBuilder.String(), args...)
			if err != nil {
				return stats, err
			}
			added, _ := res.RowsAffected()
			stats.LocationsAdded += int(added)
		}
	}

	return stats, tx.Commit()
}

// ListFilesByTagsAnd retrieves all file paths for a given set of tags (AND query).
func (s *Store) ListFilesByTagsAnd(tags []string) ([]string, error) {
	if len(tags) == 0 {
		return []string{}, nil
	}

	query := `
		SELECT l.path
		FROM locations l
		WHERE l.content_hash IN (
			SELECT ct.content_hash
			FROM content_tags ct
			JOIN tags t ON ct.tag_id = t.id
			WHERE t.name IN (?` + strings.Repeat(",?", len(tags)-1) + `)
			GROUP BY ct.content_hash
			HAVING COUNT(DISTINCT t.name) = ?
		)
		ORDER BY l.path
	`

	args := make([]interface{}, len(tags)+1)
	for i, tag := range tags {
		args[i] = tag
	}
	args[len(tags)] = len(tags)

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
func (s *Store) GetFilesInfoByTag(tag string) ([]types.FileInfo, error) {
	query := `
		SELECT l.path, l.size_bytes, l.tags_cache
		FROM locations l
		JOIN content_tags ct ON l.content_hash = ct.content_hash
		JOIN tags t ON ct.tag_id = t.id
		WHERE t.name = ?
		ORDER BY l.path`

	rows, err := s.DB.Query(query, tag)
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
func (s *Store) GetFilesInfoByTagsAnd(tags []string) ([]types.FileInfo, error) {
	if len(tags) == 0 {
		return []types.FileInfo{}, nil
	}

	query := `
		SELECT l.path, l.size_bytes, l.tags_cache
		FROM locations l
		WHERE l.content_hash IN (
			SELECT ct.content_hash
			FROM content_tags ct
			JOIN tags t ON ct.tag_id = t.id
			WHERE t.name IN (?` + strings.Repeat(",?", len(tags)-1) + `)
			GROUP BY ct.content_hash
			HAVING COUNT(DISTINCT t.name) = ?
		)
		ORDER BY l.path
	`

	args := make([]interface{}, len(tags)+1)
	for i, tag := range tags {
		args[i] = tag
	}
	args[len(tags)] = len(tags)

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