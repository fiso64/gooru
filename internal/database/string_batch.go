package database

import (
	"iter"
	"strings"

	"gooru.local/types"
)

// bindValueBatches yields SQLite-sized one-column placeholder and argument
// batches. The placeholder form is caller-supplied so the same chunking can
// serve both IN lists ("?") and VALUES rows ("(?)"). It reuses one argument
// buffer and one placeholder string across yields, so callers must consume each
// args slice synchronously.
func bindValueBatches[T any](values []T, valuePlaceholder string) iter.Seq2[string, []any] {
	return bindValueBatchesReserved(values, valuePlaceholder, 0)
}

func bindValueBatchesReserved[T any](
	values []T,
	valuePlaceholder string,
	reservedArgs int,
) iter.Seq2[string, []any] {
	return func(yield func(string, []any) bool) {
		if len(values) == 0 {
			return
		}

		batchSize := min(len(values), maxVars-reservedArgs)
		placeholders := strings.TrimSuffix(strings.Repeat(valuePlaceholder+",", batchSize), ",")
		args := make([]any, batchSize, batchSize+reservedArgs)

		for start := 0; start < len(values); start += batchSize {
			batch := values[start:min(start+batchSize, len(values))]
			for i, value := range batch {
				args[i] = value
			}
			placeholderLen := len(batch)*(len(valuePlaceholder)+1) - 1
			if !yield(placeholders[:placeholderLen], args[:len(batch)]) {
				return
			}
		}
	}
}

func bindBatches[T any](values []T) iter.Seq2[string, []any] {
	return bindValueBatches(values, "?")
}

// bindPairBatches yields SQLite-sized two-column row batches. The row
// placeholder string is supplied by the caller so existing SQL formatting can
// remain unchanged while the chunking and argument-buffer logic is shared.
func bindRowBatchesReserved[T any](
	values []T,
	rowPlaceholders string,
	columns int,
	reservedArgs int,
	fill func([]any, T, int),
) iter.Seq2[string, []any] {
	return func(yield func(string, []any) bool) {
		if len(values) == 0 {
			return
		}

		batchSize := min(len(values), (maxVars-reservedArgs)/columns)
		placeholders := strings.TrimSuffix(strings.Repeat(rowPlaceholders+",", batchSize), ",")
		args := make([]any, batchSize*columns, batchSize*columns+reservedArgs)

		for start := 0; start < len(values); start += batchSize {
			batch := values[start:min(start+batchSize, len(values))]
			for i, value := range batch {
				fill(args[i*columns:(i+1)*columns], value, start+i)
			}
			placeholderLen := len(batch)*(len(rowPlaceholders)+1) - 1
			if !yield(placeholders[:placeholderLen], args[:len(batch)*columns]) {
				return
			}
		}
	}
}

func bindRowBatches[T any](
	values []T,
	rowPlaceholders string,
	columns int,
	fill func([]any, T, int),
) iter.Seq2[string, []any] {
	return bindRowBatchesReserved(values, rowPlaceholders, columns, 0, fill)
}

func bindPairBatches[T any](
	values []T,
	rowPlaceholders string,
	fields func(T) (any, any),
) iter.Seq2[string, []any] {
	return bindPairBatchesReserved(values, rowPlaceholders, 0, fields)
}

func bindPairBatchesReserved[T any](
	values []T,
	rowPlaceholders string,
	reservedArgs int,
	fields func(T) (any, any),
) iter.Seq2[string, []any] {
	return bindRowBatchesReserved(values, rowPlaceholders, 2, reservedArgs, func(args []any, value T, _ int) {
		args[0], args[1] = fields(value)
	})
}

func parsedTagBindBatches(tags []types.ParsedTag) iter.Seq2[string, []any] {
	return bindPairBatches(tags, "(?, ?)", func(tag types.ParsedTag) (any, any) {
		return tag.Key, tag.Value
	})
}

func contentTagPairBindBatches(pairs []ContentTagPair) iter.Seq2[string, []any] {
	return bindPairBatches(pairs, "(?,?)", func(pair ContentTagPair) (any, any) {
		return pair.ContentHash, pair.TagID
	})
}

func stringBindBatches(values []string) iter.Seq2[string, []any] {
	return bindBatches(values)
}
