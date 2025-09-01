package mount

import (
	"fmt"
	"gooru.local/gooru"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"

	"github.com/winfsp/cgofuse/fuse"
	"gooru.local/gooru/types"
)

const (
	shortHashLen = 8
)

// GooruVFS implements the fuse.FileSystemInterface.
type GooruVFS struct {
	fuse.FileSystemBase
	svc            *gooru.Client
	currentQuery   string
	isLive         bool              // If true, re-query on every directory read.
	files          []types.FileInfo
	virtualFiles   map[string]string // map virtual filename to real filepath
	stateMu        sync.RWMutex      // Protects files, virtualFiles, currentQuery, isLive

	fileMode       uint32
	dirMode        uint32
	uid            uint32
	gid            uint32
	lastError      error
	openFiles      map[uint64]*os.File
	nextFileHandle uint64
	openFileMu     sync.Mutex // Protects openFiles, nextFileHandle
}

// NewGooruVFS creates a new virtual filesystem for the given files.
func NewGooruVFS(svc *gooru.Client, initialQuery string, initialFiles []types.FileInfo, isLive bool) *GooruVFS {
	vfs := &GooruVFS{
		svc:            svc,
		currentQuery:   initialQuery,
		isLive:         isLive,
		files:          initialFiles,
		virtualFiles:   make(map[string]string),
		fileMode:       0444, // Read-only for user
		dirMode:        0555, // Read/execute for user
		uid:            uint32(os.Getuid()),
		gid:            uint32(os.Getgid()),
		openFiles:      make(map[uint64]*os.File),
		nextFileHandle: 1,
	}
	// Initial population doesn't need a lock.
	vfs.populateVirtualFiles()
	return vfs
}

func (vfs *GooruVFS) populateVirtualFiles() {
	// This method should be called with the write lock held.
	vfs.virtualFiles = make(map[string]string)
	basenameCounts := make(map[string]int)
	basenameFiles := make(map[string][]types.FileInfo)

	for _, file := range vfs.files {
		base := filepath.Base(file.Path)
		basenameCounts[base]++
		basenameFiles[base] = append(basenameFiles[base], file)
	}

	for base, count := range basenameCounts {
		if count == 1 {
			vfs.virtualFiles[base] = basenameFiles[base][0].Path
		} else {
			for _, file := range basenameFiles[base] {
				ext := filepath.Ext(base)
				name := strings.TrimSuffix(base, ext)
				shortHash := file.Hash
				if len(shortHash) > shortHashLen {
					shortHash = shortHash[:shortHashLen]
				}
				newName := fmt.Sprintf("%s-%s%s", name, shortHash, ext)
				vfs.virtualFiles[newName] = file.Path
			}
		}
	}
}

// UpdateQuery re-runs a query and updates the filesystem view.
func (vfs *GooruVFS) UpdateQuery(expression string) error {
	var files []types.FileInfo
	var err error
	if expression == "" {
		files, err = vfs.svc.GetAllFilesInfo()
	} else {
		// The vfs doesn't know about the verbose flag, so pass false.
		files, err = vfs.svc.GetFilesInfoByQuery(expression, false)
	}
	if err != nil {
		return err
	}

	vfs.stateMu.Lock()
	defer vfs.stateMu.Unlock()
	vfs.files = files
	vfs.currentQuery = expression
	vfs.populateVirtualFiles()

	return nil
}

// refreshQuery re-runs the current query and updates the file list.
func (vfs *GooruVFS) refreshQuery() error {
	// Get the current query with a read lock to be thread-safe.
	vfs.stateMu.RLock()
	currentQuery := vfs.currentQuery
	vfs.stateMu.RUnlock()

	// Perform the potentially slow DB query without holding any locks.
	var files []types.FileInfo
	var err error
	if currentQuery == "" {
		files, err = vfs.svc.GetAllFilesInfo()
	} else {
		files, err = vfs.svc.GetFilesInfoByQuery(currentQuery, false)
	}
	if err != nil {
		return err
	}

	// Now, acquire a write lock to update the internal state.
	vfs.stateMu.Lock()
	defer vfs.stateMu.Unlock()
	vfs.files = files
	vfs.populateVirtualFiles()

	return nil
}

// ResolveVirtualPath finds the real path for a given virtual path.
func (vfs *GooruVFS) ResolveVirtualPath(vpath string) (string, bool) {
	vfs.stateMu.RLock()
	defer vfs.stateMu.RUnlock()
	// vpath will come in as "/<filename>", need to strip leading slash.
	realPath, ok := vfs.virtualFiles[strings.TrimPrefix(vpath, "/")]
	return realPath, ok
}

// GetCurrentQuery returns the current query expression.
func (vfs *GooruVFS) GetCurrentQuery() string {
	vfs.stateMu.RLock()
	defer vfs.stateMu.RUnlock()
	return vfs.currentQuery
}

// Getattr gets file attributes.
func (vfs *GooruVFS) Getattr(path string, stat *fuse.Stat_t, fh uint64) (errc int) {
	vfs.stateMu.RLock()
	defer vfs.stateMu.RUnlock()

	if path == "/" {
		stat.Mode = fuse.S_IFDIR | vfs.dirMode
		stat.Nlink = 1
		return 0
	}

	realPath, ok := vfs.virtualFiles[path[1:]]
	if !ok {
		return 0 - fuse.ENOENT
	}

	info, err := os.Stat(realPath)
	if err != nil {
		return 0 - fuse.ENOENT
	}

	stat.Mode = fuse.S_IFREG | vfs.fileMode
	stat.Size = info.Size()
	stat.Mtim = fuse.NewTimespec(info.ModTime())
	stat.Nlink = 1
	stat.Uid = vfs.uid
	stat.Gid = vfs.gid
	return 0
}

// Readdir reads a directory.
func (vfs *GooruVFS) Readdir(path string,
	fill func(name string, stat *fuse.Stat_t, ofst int64) bool,
	ofst int64,
	fh uint64) (errc int) {

	if vfs.isLive {
		// Re-run the query to get fresh data.
		if err := vfs.refreshQuery(); err != nil {
			fmt.Fprintf(os.Stderr, "vfs: live refresh failed: %v\n", err)
			return 0 - fuse.EIO // Input/output error
		}
	}

	vfs.stateMu.RLock()
	defer vfs.stateMu.RUnlock()

	if path != "/" {
		return 0 - fuse.ENOENT
	}

	fill(".", nil, 0)
	fill("..", nil, 0)

	for name := range vfs.virtualFiles {
		fill(name, nil, 0)
	}
	return 0
}

// Open opens a file.
func (vfs *GooruVFS) Open(path string, flags int) (errc int, fh uint64) {
	vfs.stateMu.RLock()
	realPath, ok := vfs.virtualFiles[path[1:]]
	vfs.stateMu.RUnlock()

	if !ok {
		return 0 - fuse.ENOENT, 0
	}

	// Enforce read-only
	if flags&os.O_RDWR != 0 || flags&os.O_WRONLY != 0 {
		return 0 - fuse.EACCES, 0
	}

	file, err := os.Open(realPath)
	if err != nil {
		// Can happen if the real file was deleted after the mount started.
		return 0 - fuse.ENOENT, 0
	}

	vfs.openFileMu.Lock()
	defer vfs.openFileMu.Unlock()
	fh = vfs.nextFileHandle
	vfs.openFiles[fh] = file
	vfs.nextFileHandle++
	return 0, fh
}

// Read reads from an open file.
func (vfs *GooruVFS) Read(path string, buff []byte, ofst int64, fh uint64) (n int) {
	vfs.openFileMu.Lock()
	file, ok := vfs.openFiles[fh]
	vfs.openFileMu.Unlock()

	if !ok {
		return 0 - fuse.EBADF
	}

	n, err := file.ReadAt(buff, ofst)
	if err != nil && err != io.EOF {
		vfs.lastError = err
		return 0 - fuse.EIO
	}
	return n
}

// Release closes an open file.
func (vfs *GooruVFS) Release(path string, fh uint64) (errc int) {
	vfs.openFileMu.Lock()
	defer vfs.openFileMu.Unlock()

	file, ok := vfs.openFiles[fh]
	if !ok {
		return 0 - fuse.EBADF
	}
	delete(vfs.openFiles, fh)
	file.Close()
	return 0
}

// Getxattr is required by the FileSystemInterface.
func (vfs *GooruVFS) Getxattr(path string, name string) (errc int, value []byte) {
	return 0 - fuse.ENOSYS, nil
}

// Listxattr is required by the FileSystemInterface.
func (vfs *GooruVFS) Listxattr(path string, fill func(name string) bool) (errc int) {
	return 0 - fuse.ENOSYS
}

// OnError is called for errors, useful for debugging.
func (vfs *GooruVFS) OnError(op string, errc int) {
	if vfs.lastError != nil {
		fmt.Fprintf(os.Stderr, "Error in %s: %v (underlying error: %v)\n", op, syscall.Errno(errc), vfs.lastError)
		vfs.lastError = nil
	} else {
		fmt.Fprintf(os.Stderr, "Error in %s: %v\n", op, syscall.Errno(errc))
	}
}