package serve

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"strings"

	sqlite "gosqlite.org"
)

const maxJobErrorTypeDepth = 8

// jobFailureLogAttrs turns an arbitrary job error into operator-facing fields
// that are safe to emit from the shared job manager. Raw error strings are
// intentionally excluded because jobs routinely wrap filenames, paths, tags,
// and other user-controlled library metadata into their errors.
func jobFailureLogAttrs(err error) []any {
	if err == nil {
		return nil
	}

	attrs := []any{"error_class", jobFailureClass(err)}

	var sqliteErr *sqlite.Error
	if errors.As(err, &sqliteErr) {
		attrs = append(attrs,
			"sqlite_code", sqliteErr.Code(),
			"sqlite_extended_code", sqliteErr.ExtendedCode(),
		)
	}

	if types := jobErrorTypeChain(err); types != "" {
		attrs = append(attrs, "error_types", types)
	}
	return attrs
}

func jobFailureClass(err error) string {
	var sqliteErr *sqlite.Error
	if errors.As(err, &sqliteErr) {
		switch sqliteErr.Code() {
		case sqlite.SQLITE_BUSY:
			return "sqlite_busy"
		case sqlite.SQLITE_LOCKED:
			return "sqlite_locked"
		case sqlite.SQLITE_CONSTRAINT:
			return "sqlite_constraint"
		case sqlite.SQLITE_FULL:
			return "sqlite_full"
		default:
			return "sqlite_error"
		}
	}

	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return "deadline_exceeded"
	case errors.Is(err, context.Canceled):
		return "canceled"
	case errors.Is(err, fs.ErrPermission):
		return "permission_denied"
	case errors.Is(err, fs.ErrNotExist):
		return "not_found"
	default:
		return "internal_error"
	}
}

func jobErrorTypeChain(err error) string {
	var types []string
	appendJobErrorTypes(&types, err, maxJobErrorTypeDepth)
	return strings.Join(types, " -> ")
}

func appendJobErrorTypes(types *[]string, err error, remaining int) {
	if err == nil || remaining <= 0 {
		return
	}
	*types = append(*types, fmt.Sprintf("%T", err))
	remaining--
	if remaining == 0 {
		return
	}

	if joined, ok := err.(interface{ Unwrap() []error }); ok {
		for _, child := range joined.Unwrap() {
			if remaining <= 0 {
				return
			}
			before := len(*types)
			appendJobErrorTypes(types, child, remaining)
			remaining -= len(*types) - before
		}
		return
	}
	if wrapped, ok := err.(interface{ Unwrap() error }); ok {
		appendJobErrorTypes(types, wrapped.Unwrap(), remaining)
	}
}
