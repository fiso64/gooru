package serve

import (
 "encoding/binary"
 "errors"
 "io"
)

var errJPEGFrameUnavailable = errors.New("JPEG frame header unavailable")

// jpegFrameIsProgressive inspects frame markers without decoding image pixels.
func jpegFrameIsProgressive(src io.Reader) (bool, error) {
 var buf [2]byte
 if _, err := io.ReadFull(src, buf[:]); err != nil { return false, err }
 if buf != [2]byte{0xff, 0xd8} { return false, errJPEGFrameUnavailable }
 for segment := 0; segment < 4096; segment++ {
  if _, err := io.ReadFull(src, buf[:1]); err != nil { return false, err }
  if buf[0] != 0xff { return false, errJPEGFrameUnavailable }
  for {
   if _, err := io.ReadFull(src, buf[:1]); err != nil { return false, err }
   if buf[0] != 0xff { break }
  }
  marker := buf[0]
  isFrame := marker >= 0xc0 && marker <= 0xcf && marker != 0xc4 && marker != 0xc8 && marker != 0xcc
  if marker == 0xda || marker == 0xd9 || marker == 0x00 || marker == 0x01 || (marker >= 0xd0 && marker <= 0xd8) {
   return false, errJPEGFrameUnavailable
  }
  if _, err := io.ReadFull(src, buf[:]); err != nil { return false, err }
  length := binary.BigEndian.Uint16(buf[:])
  if length < 2 || (isFrame && length < 8) { return false, errJPEGFrameUnavailable }
  if _, err := io.CopyN(io.Discard, src, int64(length-2)); err != nil { return false, err }
  if isFrame { return marker == 0xc2, nil }
 }
 return false, errJPEGFrameUnavailable
}
