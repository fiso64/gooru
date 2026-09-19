package serve

import (
 "bytes"
 "context"
 "errors"
 "os"
 "path/filepath"
 "runtime"
 "strings"
 "testing"
 "time"
)

func writeJPEGTranscodeStub(t *testing.T, script string) string {
 t.Helper()
 if runtime.GOOS == "windows" { t.Skip("shell fixture is Unix-specific") }
 path := filepath.Join(t.TempDir(), "jpegtran-stub")
 if err := os.WriteFile(path, []byte("#!/bin/sh\n"+script+"\n"), 0700); err != nil { t.Fatal(err) }
 return path
}

func TestLosslessJPEGTransformUsesStreamingCoefficientTool(t *testing.T) {
 binary := writeJPEGTranscodeStub(t, "[ \"$1\" = '-copy' ] && [ \"$2\" = 'all' ] || exit 20\ncat")
 source := []byte("input bytes remain available to the caller")
 var output bytes.Buffer
 if err := transcodeProgressiveJPEG(context.Background(), bytes.NewReader(source), &output, binary); err != nil { t.Fatal(err) }
 if !bytes.Equal(output.Bytes(), source) { t.Fatalf("tool output was not streamed: %q", output.Bytes()) }
 if !bytes.Equal(source, []byte("input bytes remain available to the caller")) { t.Fatal("source changed") }
}

func TestLosslessJPEGTransformFailureAndCancel(t *testing.T) {
 binary := writeJPEGTranscodeStub(t, "exit 23")
 if err := transcodeProgressiveJPEG(context.Background(), strings.NewReader("jpeg"), &bytes.Buffer{}, binary); err == nil {
  t.Fatal("missing command failure")
 }
 canceled := writeJPEGTranscodeStub(t, "exec sleep 5")
 ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
 defer cancel()
 err := transcodeProgressiveJPEG(ctx, strings.NewReader("jpeg"), &bytes.Buffer{}, canceled)
 if !errors.Is(ctx.Err(), context.DeadlineExceeded) || err == nil { t.Fatalf("cancellation not propagated: %v (%v)", err, ctx.Err()) }
}
