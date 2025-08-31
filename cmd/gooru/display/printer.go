package display

import (
	"fmt"
	"os"
)

// Warnf prints a formatted warning message to stderr.
// This provides a single point for adding color in the future.
func Warnf(format string, a ...any) {
	fmt.Fprintf(os.Stderr, "Warning: "+format+"\n", a...)
}

// Errorf prints a formatted error message to stderr.
// This provides a single point for adding color in the future.
func Errorf(format string, a ...any) {
	fmt.Fprintf(os.Stderr, "Error: "+format+"\n", a...)
}