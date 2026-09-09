//go:build !linux

package serve

import "io"

func protectedVideoSeekablePath(io.ReadSeeker) (string, func(), bool, error) {
	return "", nil, false, nil
}
