//go:build linux

package serve

import (
	"fmt"
	"io"
	"os"

	"golang.org/x/sys/unix"
)

func protectedVideoSeekablePath(src io.ReadSeeker) (string, func(), bool, error) {
	if _, err := src.Seek(0, io.SeekStart); err != nil {
		return "", nil, false, err
	}
	fd, err := unix.MemfdCreate("gooru-protected-video", unix.MFD_CLOEXEC)
	if err != nil {
		return "", nil, false, nil
	}
	file := os.NewFile(uintptr(fd), "gooru-protected-video")
	if file == nil {
		_ = unix.Close(fd)
		return "", nil, false, fmt.Errorf("create protected video memory file")
	}
	cleanup := func() { _ = file.Close() }
	if _, err := io.Copy(file, src); err != nil {
		cleanup()
		return "", nil, false, err
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		cleanup()
		return "", nil, false, err
	}
	// The child does not need to inherit the descriptor: procfs lets ffmpeg and
	// ffprobe open the still-live descriptor owned by this process as a regular,
	// seekable file. No plaintext pathname is ever created on disk.
	return fmt.Sprintf("/proc/%d/fd/%d", os.Getpid(), fd), cleanup, true, nil
}
