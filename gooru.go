package gooru

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gooru.local/gooru/internal/database"
	"gooru.local/gooru/internal/hashing"
	"gooru.local/gooru/types"
)

var (
	// ErrDBUninitialized is returned when the database has not been set up.
	ErrDBUninitialized = errors.New("database not initialized")
)

// Init creates and initializes a new Gooru database with a chosen hashing strategy.
// It will overwrite an existing file, so the caller is responsible for any checks.
func Init(dbPath string, strategy types.HashingStrategy, verbose bool) error {
	// The `init` command is responsible for checking if a valid database already exists.
	// This function proceeds with initialization, overwriting if necessary.
	if err := database.InitStore(dbPath, strategy, verbose); err != nil {
		// Clean up the partially created db file on failure
		_ = os.Remove(dbPath)
		return fmt.Errorf("failed to initialize database: %w", err)
	}
	return nil
}

// Client encapsulates the core business logic.
type Client struct {
	store  *database.Store
	hasher *hashing.Hasher
}

// New creates a new Client and initializes the database connection.
// It will return ErrDBUninitialized if the database has not been created with `gooru init`.
// The caller is responsible for calling Close() on the returned client.
func New(dbPath string, verbose bool) (*Client, error) {
	store, err := database.NewStore(dbPath, verbose)
	if err != nil {
		// This can happen if the file doesn't exist.
		if os.IsNotExist(err) {
			return nil, ErrDBUninitialized
		}
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	strategy, err := store.GetHashingStrategy()
	if err != nil {
		// If we can't get the strategy, the DB is likely uninitialized or corrupt.
		store.Close()
		// A more robust check for an uninitialized DB.
		// sql.ErrNoRows happens if the meta table exists but is empty.
		// "no such table" happens if the schema was never created.
		// Both indicate an uninitialized state.
		if errors.Is(err, sql.ErrNoRows) || strings.Contains(err.Error(), "no such table") {
			return nil, ErrDBUninitialized
		}
		return nil, fmt.Errorf("could not read hashing strategy: %w", err)
	}

	hasher, err := hashing.NewHasher(strategy)
	if err != nil {
		store.Close()
		return nil, fmt.Errorf("failed to initialize hasher: %w", err)
	}

	return &Client{store: store, hasher: hasher}, nil
}

// Close closes the underlying database connection.
func (c *Client) Close() error {
	if c.store != nil {
		return c.store.Close()
	}
	return nil
}

// resolvePath canonicalizes a path. If it's a virtual path, it resolves it to a real one.
// If it doesn't exist, it returns the absolute path of the input to allow for clean
// "file not found" errors later.
func resolvePath(filePath string) (string, error) {
	// First, get the absolute path. This can fail if the working directory is invalid.
	absPath, err := filepath.Abs(filePath)
	if err != nil {
		return "", err
	}

	// NEW: Check if this is a virtual path and resolve it to a real path.
	realPath, wasVirtual, err := resolveIfVirtual(absPath)
	if err != nil {
		// Propagate specific errors from the virtual resolution, like os.ErrNotExist.
		if os.IsNotExist(err) {
			return "", err
		}
		return "", fmt.Errorf("virtual path resolution failed for '%s': %w", filePath, err)
	}
	if wasVirtual {
		// The path was virtual and has been resolved. The new "real" path is now absPath.
		// We trust the mount process to have returned a valid, absolute path.
		absPath = realPath
	}

	// Now, check for filesystem errors other than NotExist (e.g., permission denied) on the real path.
	// We ignore NotExist because the service layer is equipped to handle it.
	if _, err := os.Lstat(absPath); err != nil && !os.IsNotExist(err) {
		return "", err // Return the actual filesystem error.
	}

	return absPath, nil
}

func toAbsolutePaths(paths []string) ([]string, error) {
	absPaths := make([]string, len(paths))
	for i, p := range paths {
		abs, err := filepath.Abs(p)
		if err != nil {
			return nil, err
		}
		absPaths[i] = abs
	}
	return absPaths, nil
}