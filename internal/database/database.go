package database

import (
	"database/sql"
	"strings"

	_ "github.com/mattn/go-sqlite3"
)

// InitDB initializes the database connection and creates the schema if it doesn't exist.
func InitDB(dataSourceName string) (*sql.DB, error) {
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

	return db, nil
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
			FOREIGN KEY (content_hash) REFERENCES contents(hash) ON DELETE CASCADE
		);`,
		`CREATE TABLE IF NOT EXISTS tags (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL UNIQUE
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
	}

	for _, stmt := range statements {
		if _, err := db.Exec(stmt); err != nil {
			return err
		}
	}

	return nil
}

// AddContent inserts a new content hash into the database.
func AddContent(db *sql.DB, hash string) error {
	_, err := db.Exec("INSERT OR IGNORE INTO contents (hash) VALUES (?)", hash)
	return err
}

// AddLocation adds a file path for a given content hash.
func AddLocation(db *sql.DB, hash, path string) error {
	_, err := db.Exec("INSERT OR IGNORE INTO locations (content_hash, path) VALUES (?, ?)", hash, path)
	return err
}

// FindContent by path
func FindContent(db *sql.DB, path string) (string, error) {
	var hash string
	err := db.QueryRow("SELECT content_hash FROM locations WHERE path = ?", path).Scan(&hash)
	if err != nil {
		return "", err
	}
	return hash, nil
}

// AddTag adds a new tag to the database and returns its ID.
func AddTag(db *sql.DB, name string) (int64, error) {
	id, err := GetTag(db, name)
	if err == nil {
		return id, nil
	}

	res, err := db.Exec("INSERT INTO tags (name) VALUES (?)", name)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// GetTag retrieves a tag by its name.
func GetTag(db *sql.DB, name string) (int64, error) {
	var id int64
	err := db.QueryRow("SELECT id FROM tags WHERE name = ?", name).Scan(&id)
	if err != nil {
		return 0, err
	}
	return id, nil
}

// AssociateTag links a tag with a content hash.
func AssociateTag(db *sql.DB, hash string, tagID int64) error {
	_, err := db.Exec("INSERT OR IGNORE INTO content_tags (content_hash, tag_id) VALUES (?, ?)", hash, tagID)
	return err
}

// DisassociateTag removes a link between a tag and a content hash.
func DisassociateTag(db *sql.DB, hash string, tagID int64) error {
	_, err := db.Exec("DELETE FROM content_tags WHERE content_hash = ? AND tag_id = ?", hash, tagID)
	return err
}

// GetTagsForContent retrieves all tags for a given content hash.
func GetTagsForContent(db *sql.DB, hash string) ([]string, error) {
	rows, err := db.Query(`
		SELECT t.name 
		FROM tags t 
		JOIN content_tags ct ON t.id = ct.tag_id 
		WHERE ct.content_hash = ?`, hash)
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
func ListAllFiles(db *sql.DB) ([]string, error) {
	rows, err := db.Query("SELECT path FROM locations")
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

// ListFilesByTag retrieves all file paths for a given tag.
func ListFilesByTag(db *sql.DB, tag string) ([]string, error) {
	rows, err := db.Query(`
		SELECT l.path
		FROM locations l
		JOIN content_tags ct ON l.content_hash = ct.content_hash
		JOIN tags t ON ct.tag_id = t.id
		WHERE t.name = ?`, tag)
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

// ListFilesByTagsAnd retrieves all file paths for a given set of tags (AND query).
func ListFilesByTagsAnd(db *sql.DB, tags []string) ([]string, error) {
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
	`

	args := make([]interface{}, len(tags)+1)
	for i, tag := range tags {
		args[i] = tag
	}
	args[len(tags)] = len(tags)

	rows, err := db.Query(query, args...)
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






