package display

import (
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"

	"gooru.local/gooru/types"
)

// ListHeaders defines the column headers for the list command output.
// This is the "easily modifiable location" for list properties.
var ListHeaders = []string{"PATH", "SIZE", "TAGS"}

// PrintTable formats and prints a list of files as a table.
func PrintTable(files []types.FileInfo) {
	if len(files) == 0 {
		fmt.Println("No files found.")
		return
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	defer w.Flush()

	// Print Headers
	fmt.Fprintln(w, strings.Join(ListHeaders, "\t"))

	// Print Rows
	for _, file := range files {
		tags := strings.ReplaceAll(file.Tags, ",", ", ")
		fmt.Fprintf(w, "%s\t%s\t%s\n", file.Path, HumanReadableSize(file.Size), tags)
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