package display

import (
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"

	"gooru.local/types"
)

// PrintTable formats and prints a list of files as a table.
// It takes a list of FileInfo and a variadic list of headers to display.
// If no headers are provided, it defaults to ["PATH", "SIZE", "TAGS"].
// Valid headers are "PATH", "SIZE", "TAGS".
func PrintTable(files []types.FileInfo, headers ...string) {
	if len(files) == 0 {
		fmt.Println("No files found.")
		return
	}

	if len(headers) == 0 {
		headers = []string{"PATH", "SIZE", "TAGS"}
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	defer w.Flush()

	// Print Headers
	fmt.Fprintln(w, strings.Join(headers, "\t"))

	// Print Rows
	for _, file := range files {
		var row []string
		for _, h := range headers {
			switch strings.ToUpper(h) {
			case "PATH":
				row = append(row, file.Path)
			case "SIZE":
				row = append(row, HumanReadableSize(file.Size))
			case "TAGS":
				row = append(row, strings.ReplaceAll(file.Tags, ",", ", "))
			}
		}
		fmt.Fprintln(w, strings.Join(row, "\t"))
	}
}

// HumanReadableSize converts a size in bytes to a human-readable string.
func HumanReadableSize(size int64) string {
	const unit = 1024
	if size < unit {
		return fmt.Sprintf("%d B", size)
	}
	div, exp := int64(unit), 0
	for n := size / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(size)/float64(div), "KMGTPE"[exp])
}

// PrintPaths is a simpler printer for just paths, if needed elsewhere.
func PrintPaths(w io.Writer, paths []string) {
	for _, path := range paths {
		fmt.Fprintln(w, path)
	}
}