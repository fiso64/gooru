package database

import (
	"iter"
	"strings"

	"gooru.local/types"
)

func stringBatchArgs(values []string) (string, []any) {
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(values)), ",")
	args := make([]any, len(values))
	for i, value := range values {
		args[i] = value
	}
	return placeholders, args
}

// bindBatches yields SQLite-sized placeholder and argument batches. It reuses
// one argument buffer and one placeholder string across yields, so callers must
// consume each args slice synchronously.
func bindBatches[T any](values []T) iter.Seq2[string, []any] {
	return func(yield func(string, []any) bool) {
		if len(values) == 0 {
			return
		}

		batchSize := min(len(values), maxVars)
		placeholders := strings.TrimSuffix(strings.Repeat("?,", batchSize), ",")
		args := make([]any, batchSize)

		for start := 0; start < len(values); start += batchSize {
			batch := values[start:min(start+batchSize, len(values))]
			for i, value := range batch {
				args[i] = value
			}
			if !yield(placeholders[:2*len(batch)-1], args[:len(batch)]) {
				return
			}
		}
	}
}

func parsedTagBindBatches(tags []types.ParsedTag) iter.Seq2[string, []any] {
	return func(yield func(string, []any) bool) {
		if len(tags) == 0 {
			return
		}

		const (
			columns         = 2
			rowPlaceholders = "(?, ?)"
		)
		batchSize := min(len(tags), maxVars/columns)
		placeholders := strings.TrimSuffix(strings.Repeat(rowPlaceholders+",", batchSize), ",")
		args := make([]any, batchSize*columns)

		for start := 0; start < len(tags); start += batchSize {
			batch := tags[start:min(start+batchSize, len(tags))]
			for i, tag := range batch {
				args[i*columns] = tag.Key
				args[i*columns+1] = tag.Value
			}
			placeholderLen := len(batch)*(len(rowPlaceholders)+1) - 1
			if !yield(placeholders[:placeholderLen], args[:len(batch)*columns]) {
				return
			}
		}
	}
}

func stringBindBatches(values []string) iter.Seq2[string, []any] {
	return bindBatches(values)
}
