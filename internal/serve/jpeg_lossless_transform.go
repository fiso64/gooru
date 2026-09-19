package serve

import (
 "context"
 "fmt"
 "io"
 "os/exec"
)

// transcodeProgressiveJPEG invokes a coefficient-domain JPEG transform.
// Without -progressive, jpegtran emits sequential JPEG; -copy all retains
// APP/COM metadata, including EXIF and ICC data.
func transcodeProgressiveJPEG(ctx context.Context, src io.Reader, dst io.Writer, binary string) error {
 if binary == "" { binary = "jpegtran" }
 process := exec.CommandContext(ctx, binary, "-copy", "all")
 process.Stdin = src
 process.Stdout = dst
 process.Stderr = io.Discard
 if err := process.Run(); err != nil { return fmt.Errorf("lossless JPEG conversion: %w", err) }
 return nil
}
