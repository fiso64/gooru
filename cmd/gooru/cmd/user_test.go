package cmd

import (
	"bufio"
	"bytes"
	"testing"

	"github.com/spf13/cobra"
)

func TestReadLineSecretUsesSharedReader(t *testing.T) {
	input := bufio.NewReader(bytes.NewBufferString("correct horse\ncorrect horse\n"))
	cmd := &cobra.Command{}
	var stderr bytes.Buffer
	cmd.SetErr(&stderr)

	first, err := readLineSecret(cmd, input, "Password: ")
	if err != nil {
		t.Fatalf("read first secret: %v", err)
	}
	second, err := readLineSecret(cmd, input, "Confirm password: ")
	if err != nil {
		t.Fatalf("read second secret: %v", err)
	}
	if first != "correct horse" || second != "correct horse" {
		t.Fatalf("unexpected secrets %q %q", first, second)
	}
	if got := stderr.String(); got != "Password: Confirm password: " {
		t.Fatalf("unexpected prompts %q", got)
	}
}
