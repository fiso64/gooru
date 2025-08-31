package display

import (
	"fmt"
	"os"
)

const (
	ColorRed    = "\033[31m"
	ColorYellow = "\033[33m"
	ColorReset  = "\033[0m"
)

// Warnf prints a formatted warning message to stderr in yellow.
func Warnf(format string, a ...any) {
	message := fmt.Sprintf(format, a...)
	fmt.Fprintf(os.Stderr, "%sWarning: %s%s\n", ColorYellow, message, ColorReset)
}

// Errorf prints a formatted error message to stderr in red.
func Errorf(format string, a ...any) {
	message := fmt.Sprintf(format, a...)
	fmt.Fprintf(os.Stderr, "%sError: %s%s\n", ColorRed, message, ColorReset)
}