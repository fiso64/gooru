package serve

import (
 "bytes"
 "errors"
 "image"
 "image/color"
 "image/jpeg"
 "io"
 "testing"
)

func TestJPEGFrameIsProgressive(t *testing.T) {
 imageData := image.NewRGBA(image.Rect(0, 0, 2, 2))
 imageData.Set(0, 0, color.RGBA{R: 200, A: 255})
 var baseline bytes.Buffer
 if err := jpeg.Encode(&baseline, imageData, nil); err != nil { t.Fatal(err) }
 cases := []struct {
  name string
  source []byte
  progressive bool
  bad bool
 }{
  {"real baseline JPEG", baseline.Bytes(), false, false},
  {"progressive SOF2 after APP marker data", []byte{0xff,0xd8,0xff,0xe1,0x00,0x06,0xff,0xc0,0xff,0xc2,0xff,0xc2}, true, false},
  {"extended sequential SOF1", []byte{0xff,0xd8,0xff,0xc1}, false, false},
  {"not JPEG", []byte("not jpeg"), false, true},
  {"truncated APP segment", []byte{0xff,0xd8,0xff,0xe1,0x00,0x05,0x01}, false, true},
  {"invalid segment size", []byte{0xff,0xd8,0xff,0xe1,0x00,0x01}, false, true},
  {"SOS before frame", []byte{0xff,0xd8,0xff,0xda}, false, true},
 }
 for _, test := range cases {
  t.Run(test.name, func(t *testing.T) {
   progressive, err := jpegFrameIsProgressive(bytes.NewReader(test.source))
   if test.bad && err == nil { t.Fatal("expected invalid header error") }
   if !test.bad && err != nil { t.Fatal(err) }
   if progressive != test.progressive { t.Fatalf("progressive = %v, want %v", progressive, test.progressive) }
  })
 }
 if _, err := jpegFrameIsProgressive(io.LimitReader(bytes.NewReader(baseline.Bytes()), 1)); !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrUnexpectedEOF) {
  t.Fatalf("truncated source error: %v", err)
 }
}
