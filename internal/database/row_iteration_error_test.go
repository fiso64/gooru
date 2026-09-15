package database

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"io"
	"log"
	"strings"
	"sync/atomic"
	"testing"

	"gooru.local/types"
)

var terminalRowsDriverSequence uint64

type terminalRowsDriver struct {
	terminalErr error
}

func (d terminalRowsDriver) Open(string) (driver.Conn, error) {
	return terminalRowsConn{terminalErr: d.terminalErr}, nil
}

type terminalRowsConn struct {
	terminalErr error
}

func (c terminalRowsConn) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("prepare is not supported")
}

func (c terminalRowsConn) Close() error {
	return nil
}

func (c terminalRowsConn) Begin() (driver.Tx, error) {
	return nil, errors.New("transactions are not supported")
}

func (c terminalRowsConn) QueryContext(_ context.Context, query string, _ []driver.NamedValue) (driver.Rows, error) {
	columns, values := terminalRowForQuery(query)
	return &terminalRows{
		columns:     columns,
		values:      values,
		terminalErr: c.terminalErr,
	}, nil
}

type terminalRows struct {
	columns     []string
	values      []driver.Value
	emitted     bool
	terminalErr error
}

func (r *terminalRows) Columns() []string {
	return r.columns
}

func (r *terminalRows) Close() error {
	return nil
}

func (r *terminalRows) Next(dest []driver.Value) error {
	if !r.emitted {
		copy(dest, r.values)
		r.emitted = true
		return nil
	}
	return r.terminalErr
}

func terminalRowForQuery(query string) ([]string, []driver.Value) {
	switch {
	case strings.Contains(query, "SELECT t.key, t.value"):
		return []string{"key", "value"}, []driver.Value{"animal", "cat"}
	case strings.Contains(query, "SELECT content_hash, tags_cache"):
		return []string{"content_hash", "tags_cache"}, []driver.Value{"hash-one", "animal:cat"}
	case strings.Contains(query, "SELECT l.id, l.path, l.content_hash"):
		return []string{"id", "path", "content_hash", "size_bytes", "mod_time", "added_at", "tags_cache"}, []driver.Value{
			int64(1), "/library/photo.jpg", "hash-one", int64(123), int64(456), int64(789), "animal:cat",
		}
	case strings.Contains(query, "SELECT key AS tag_str, files_count AS final_count"):
		return []string{"tag_str", "final_count"}, []driver.Value{"animal", int64(1)}
	case strings.Contains(query, "SELECT key, value FROM tags"):
		return []string{"key", "value"}, []driver.Value{"animal", "cat"}
	case strings.Contains(query, "SELECT path, content_hash, size_bytes, mod_time"):
		return []string{"path", "content_hash", "size_bytes", "mod_time"}, []driver.Value{
			"/library/photo.jpg", "hash-one", int64(123), int64(456),
		}
	case strings.Contains(query, "SELECT path, content_hash FROM locations"):
		return []string{"path", "content_hash"}, []driver.Value{"/library/photo.jpg", "hash-one"}
	case strings.Contains(query, "SELECT hash FROM contents"):
		return []string{"hash"}, []driver.Value{"hash-one"}
	default:
		return []string{"path"}, []driver.Value{"/library/photo.jpg"}
	}
}

func newTerminalRowsStore(t *testing.T, terminalErr error) *Store {
	t.Helper()
	driverName := fmt.Sprintf("terminal-rows-%d", atomic.AddUint64(&terminalRowsDriverSequence, 1))
	sql.Register(driverName, terminalRowsDriver{terminalErr: terminalErr})
	db, err := sql.Open(driverName, "")
	if err != nil {
		t.Fatalf("open terminal rows database: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return &Store{DB: db, logger: log.New(io.Discard, "", 0)}
}

func TestCollectionReadersPropagateTerminalRowErrors(t *testing.T) {
	terminalErr := errors.New("terminal row iteration failure")
	store := newTerminalRowsStore(t, terminalErr)
	tag := types.ParsedTag{Key: "animal", Value: "cat"}

	tests := []struct {
		name string
		run  func() error
	}{
		{
			name: "GetTagsForContent",
			run: func() error {
				_, err := store.GetTagsForContent("hash-one")
				return err
			},
		},
		{
			name: "ListAllFiles",
			run: func() error {
				_, err := store.ListAllFiles()
				return err
			},
		},
		{
			name: "GetHashToTagsCacheMap",
			run: func() error {
				_, err := store.GetHashToTagsCacheMap()
				return err
			},
		},
		{
			name: "ListFilesByTag",
			run: func() error {
				_, err := store.ListFilesByTag(tag.Key, tag.Value)
				return err
			},
		},
		{
			name: "GetAllContentHashes",
			run: func() error {
				_, err := store.GetAllContentHashes()
				return err
			},
		},
		{
			name: "ListFilesByTagsAnd",
			run: func() error {
				_, err := store.ListFilesByTagsAnd([]types.ParsedTag{tag}, nil)
				return err
			},
		},
		{
			name: "GetFilesInfoByTag",
			run: func() error {
				_, err := store.GetFilesInfoByTag(tag.Key, tag.Value)
				return err
			},
		},
		{
			name: "GetFilesInfoByTagsAnd",
			run: func() error {
				_, err := store.GetFilesInfoByTagsAnd([]types.ParsedTag{tag}, nil)
				return err
			},
		},
		{
			name: "GetTags",
			run: func() error {
				_, err := store.GetTags(20)
				return err
			},
		},
		{
			name: "GetTagsWithCounts",
			run: func() error {
				_, err := store.GetTagsWithCounts(20)
				return err
			},
		},
		{
			name: "BatchFindContentHashesByPaths",
			run: func() error {
				_, err := store.BatchFindContentHashesByPaths([]string{"/library/photo.jpg"})
				return err
			},
		},
		{
			name: "BatchGetLocationsByPaths",
			run: func() error {
				_, err := store.BatchGetLocationsByPaths([]string{"/library/photo.jpg"})
				return err
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := test.run()
			if !errors.Is(err, terminalErr) {
				t.Fatalf("error = %v, want terminal iterator error", err)
			}
		})
	}
}
