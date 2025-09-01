package mount

import (
	"fmt"
	"gooru.local/gooru"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/winfsp/cgofuse/fuse"
	"gooru.local/gooru/types"
)

const (
	shortHashLen = 8
	// Use a character that is invalid in tags but valid in Windows filenames.
	tagSeparatorReplacement = "="
)

// GooruVFS implements the fuse.FileSystemInterface.
type GooruVFS struct {
	fuse.FileSystemBase
	svc            *gooru.Client
	baseQuery      string
	isLive         bool
	isHierarchical bool
	initialFiles   []types.FileInfo          // Used for root dir cache in hierarchical, and full cache in flat
	stateMu        sync.RWMutex              // Protects all query/file state

	// Flat Mode State
	flatVirtualFiles map[string]types.FileInfo

	// Hierarchical Mode State & Performance Cache
	dirCache  map[string]dirContents // A short-lived cache to optimize Getattr/Open calls that follow a Readdir.
	cacheMu   sync.Mutex             // Protects dirCache

	fileMode       uint32
	dirMode        uint32
	uid            uint32
	gid            uint32
	lastError      error
	openFiles      map[uint64]*os.File
	nextFileHandle uint64
	openFileMu     sync.Mutex
}

// dirContents holds the computed lists of subdirectories and files for a single virtual directory.
type dirContents struct {
	dirs  map[string]struct{}
	files map[string]types.FileInfo
}

// NewGooruVFS creates a new virtual filesystem.
func NewGooruVFS(svc *gooru.Client, initialQuery string, initialFiles []types.FileInfo, isLive, isHierarchical bool) *GooruVFS {
	vfs := &GooruVFS{
		svc:            svc,
		baseQuery:      initialQuery,
		isLive:         isLive,
		isHierarchical: isHierarchical,
		initialFiles:   initialFiles,
		fileMode:       0444, // Read-only for user
		dirMode:        0555, // Read/execute for user
		uid:            uint32(os.Getuid()),
		gid:            uint32(os.Getgid()),
		openFiles:      make(map[uint64]*os.File),
		nextFileHandle: 1,
	}

	if vfs.isHierarchical {
		vfs.dirCache = make(map[string]dirContents)
	} else {
		vfs.flatVirtualFiles = vfs.generateVirtualFileMap(initialFiles)
	}
	return vfs
}

// --- Path and Name Helpers ---

func sanitizeTagName(tag string) string {
	if runtime.GOOS == "windows" {
		// On Windows, colon is forbidden.
		return strings.ReplaceAll(tag, ":", tagSeparatorReplacement)
	}
	return tag
}

func unsanitizeTagName(name string) string {
	if runtime.GOOS == "windows" {
		return strings.ReplaceAll(name, tagSeparatorReplacement, ":")
	}
	return name
}

func (vfs *GooruVFS) generateVirtualFileMap(files []types.FileInfo) map[string]types.FileInfo {
	virtualFiles := make(map[string]types.FileInfo)
	basenameCounts := make(map[string]int)
	basenameFiles := make(map[string][]types.FileInfo)

	for _, file := range files {
		base := filepath.Base(file.Path)
		basenameCounts[base]++
		basenameFiles[base] = append(basenameFiles[base], file)
	}

	for base, count := range basenameCounts {
		if count == 1 {
			virtualFiles[base] = basenameFiles[base][0]
		} else {
			for _, file := range basenameFiles[base] {
				ext := filepath.Ext(base)
				name := strings.TrimSuffix(base, ext)
				shortHash := file.Hash
				if len(shortHash) > shortHashLen {
					shortHash = shortHash[:shortHashLen]
				}
				newName := fmt.Sprintf("%s-%s%s", name, shortHash, ext)
				virtualFiles[newName] = file
			}
		}
	}
	return virtualFiles
}

// --- State and Query Management ---

func (vfs *GooruVFS) getEffectiveQueryForPath(path string) string {
	vfs.stateMu.RLock()
	base := vfs.baseQuery
	vfs.stateMu.RUnlock()

	// Normalize path by treating it as a URL path, which uses forward slashes.
	// This avoids platform-specific separator issues.
	path = strings.Trim(filepath.ToSlash(path), "/")
	if path == "" || path == "." {
		return base
	}

	parts := strings.Split(path, "/")
	var tags []string
	for _, part := range parts {
		tags = append(tags, fmt.Sprintf(`"%s"`, unsanitizeTagName(part)))
	}

	tagQuery := strings.Join(tags, " & ")
	if base == "" {
		return tagQuery
	}
	return fmt.Sprintf("(%s) & %s", base, tagQuery)
}

func (vfs *GooruVFS) getFilesForPath(path string) ([]types.FileInfo, error) {
	// Root dir in hierarchical mode is special: its contents are based on the initial query.
	// In non-live mode, we can use the cache.
	if path == "/" && vfs.isHierarchical && !vfs.isLive {
		vfs.stateMu.RLock()
		defer vfs.stateMu.RUnlock()
		return vfs.initialFiles, nil
	}

	effectiveQuery := vfs.getEffectiveQueryForPath(path)
	if effectiveQuery == "" {
		return vfs.svc.GetAllFilesInfo()
	}
	// The vfs doesn't know about the verbose flag, so pass false.
	return vfs.svc.GetFilesInfoByQuery(effectiveQuery, false)
}

// getDirEntries is the caching front-end for computeDirEntries.
// It ensures that for a single directory view operation, the expensive computation
// of directory contents is only performed once.
func (vfs *GooruVFS) getDirEntries(path string) (dirs map[string]struct{}, files map[string]types.FileInfo, err error) {
	vfs.cacheMu.Lock()
	if cached, ok := vfs.dirCache[path]; ok {
		vfs.cacheMu.Unlock()
		return cached.dirs, cached.files, nil
	}
	vfs.cacheMu.Unlock()

	computedDirs, computedFiles, err := vfs.computeDirEntries(path)
	if err != nil {
		return nil, nil, err
	}

	vfs.cacheMu.Lock()
	vfs.dirCache[path] = dirContents{dirs: computedDirs, files: computedFiles}
	vfs.cacheMu.Unlock()

	return computedDirs, computedFiles, nil
}

// computeDirEntries performs the actual work of calculating the contents of a virtual directory.
func (vfs *GooruVFS) computeDirEntries(path string) (dirs map[string]struct{}, files map[string]types.FileInfo, err error) {
	fileInfos, err := vfs.getFilesForPath(path)
	if err != nil {
		return nil, nil, err
	}

	dirs = make(map[string]struct{})
	uniqueTags := make(map[string]struct{})

	for _, file := range fileInfos {
		tags := strings.Split(file.Tags, ",")
		for _, tag := range tags {
			if tag != "" {
				uniqueTags[tag] = struct{}{}
			}
		}
	}

	pathTags := make(map[string]struct{})
	normalizedPath := strings.Trim(filepath.ToSlash(path), "/")
	if normalizedPath != "" {
		parts := strings.Split(normalizedPath, "/")
		for _, part := range parts {
			pathTags[unsanitizeTagName(part)] = struct{}{}
		}
	}

	for tag := range uniqueTags {
		if _, exists := pathTags[tag]; !exists {
			dirs[sanitizeTagName(tag)] = struct{}{}
		}
	}

	files = vfs.generateVirtualFileMap(fileInfos)
	return dirs, files, nil
}

// UpdateQuery re-runs a query and updates the filesystem view.
func (vfs *GooruVFS) UpdateQuery(expression string) error {
	var files []types.FileInfo
	var err error
	if expression == "" {
		files, err = vfs.svc.GetAllFilesInfo()
	} else {
		files, err = vfs.svc.GetFilesInfoByQuery(expression, false)
	}
	if err != nil {
		return err
	}

	vfs.stateMu.Lock()
	vfs.baseQuery = expression
	vfs.initialFiles = files
	if vfs.isHierarchical {
		vfs.cacheMu.Lock()
		vfs.dirCache = make(map[string]dirContents) // Invalidate cache
		vfs.cacheMu.Unlock()
	} else {
		vfs.flatVirtualFiles = vfs.generateVirtualFileMap(files)
	}
	vfs.stateMu.Unlock()

	return nil
}

// refreshFlatQuery re-runs the current query for flat mode.
func (vfs *GooruVFS) refreshFlatQuery() error {
	vfs.stateMu.RLock()
	currentQuery := vfs.baseQuery
	vfs.stateMu.RUnlock()

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

	vfs.stateMu.Lock()
	defer vfs.stateMu.Unlock()
	vfs.initialFiles = files
	vfs.flatVirtualFiles = vfs.generateVirtualFileMap(files)

	return nil
}

// ResolveVirtualPath finds the real path for a given virtual path.
func (vfs *GooruVFS) ResolveVirtualPath(vpath string) (string, bool) {
	// Normalize path separators for consistency.
	vpath = filepath.ToSlash(vpath)

	if !vfs.isHierarchical {
		vfs.stateMu.RLock()
		defer vfs.stateMu.RUnlock()
		if fileInfo, ok := vfs.flatVirtualFiles[strings.TrimPrefix(vpath, "/")]; ok {
			return fileInfo.Path, true
		}
		return "", false
	}

	// Hierarchical resolution
	dir := filepath.Dir(vpath)
	if dir == "." {
		dir = "/"
	}
	base := filepath.Base(vpath)

	_, files, err := vfs.getDirEntries(dir)
	if err != nil {
		return "", false
	}

	if fileInfo, ok := files[base]; ok {
		return fileInfo.Path, true
	}
	return "", false
}

// GetCurrentQuery returns the current query expression.
func (vfs *GooruVFS) GetCurrentQuery() string {
	vfs.stateMu.RLock()
	defer vfs.stateMu.RUnlock()
	return vfs.baseQuery
}

// --- FUSE Operations ---

func (vfs *GooruVFS) Getattr(path string, stat *fuse.Stat_t, fh uint64) (errc int) {
	if path == "/" {
		stat.Mode = fuse.S_IFDIR | vfs.dirMode
		stat.Nlink = 1
		return 0
	}

	// For flat mode, we can do a quick lookup.
	if !vfs.isHierarchical {
		vfs.stateMu.RLock()
		fileInfo, ok := vfs.flatVirtualFiles[path[1:]]
		vfs.stateMu.RUnlock()

		if !ok {
			return -fuse.ENOENT
		}

		// We stat the real file to ensure it still exists, but use DB values for consistency.
		if _, err := os.Stat(fileInfo.Path); err != nil {
			return -fuse.ENOENT
		}

		stat.Mode = fuse.S_IFREG | vfs.fileMode
		stat.Size = fileInfo.Size
		stat.Mtim = fuse.NewTimespec(time.Unix(fileInfo.ModTime, 0))
		stat.Nlink = 1
		stat.Uid = vfs.uid
		stat.Gid = vfs.gid
		return 0
	}

	// Hierarchical getattr needs to determine if the path is a file or a directory.
	dir := filepath.Dir(path)
	if dir == "." {
		dir = "/"
	}
	base := filepath.Base(path)
	dirs, files, err := vfs.getDirEntries(dir)
	if err != nil {
		return -fuse.EIO
	}

	if _, isDir := dirs[base]; isDir {
		stat.Mode = fuse.S_IFDIR | vfs.dirMode
		stat.Nlink = 1
		return 0
	}

	if fileInfo, isFile := files[base]; isFile {
		// We stat the real file to ensure it still exists, but use DB values for consistency.
		if _, err := os.Stat(fileInfo.Path); err != nil {
			return -fuse.ENOENT
		}
		stat.Mode = fuse.S_IFREG | vfs.fileMode
		stat.Size = fileInfo.Size
		stat.Mtim = fuse.NewTimespec(time.Unix(fileInfo.ModTime, 0))
		stat.Nlink = 1
		stat.Uid = vfs.uid
		stat.Gid = vfs.gid
		return 0
	}

	return -fuse.ENOENT
}

func (vfs *GooruVFS) Readdir(path string,
	fill func(name string, stat *fuse.Stat_t, ofst int64) bool,
	ofst int64,
	fh uint64) (errc int) {

	if vfs.isLive {
		if vfs.isHierarchical {
			// Clear the cache to ensure the directory listing is fresh.
			// Subsequent Getattr/Open calls for this listing will be fast due to re-population.
			vfs.cacheMu.Lock()
			vfs.dirCache = make(map[string]dirContents)
			vfs.cacheMu.Unlock()
		} else {
			if err := vfs.refreshFlatQuery(); err != nil {
				fmt.Fprintf(os.Stderr, "vfs: live refresh failed: %v\n", err)
				return -fuse.EIO
			}
		}
	}

	fill(".", nil, 0)
	fill("..", nil, 0)

	// Flat mode is simple: just list the cached virtual files.
	if !vfs.isHierarchical {
		vfs.stateMu.RLock()
		defer vfs.stateMu.RUnlock()
		for name := range vfs.flatVirtualFiles {
			// Passing nil for stat is okay; the OS will call Getattr subsequently.
			fill(name, nil, 0)
		}
		return 0
	}

	// Hierarchical readdir is more complex. It must provide stat info directly
	// to be robust, avoiding fragile subsequent Getattr calls.
	dirs, files, err := vfs.getDirEntries(path)
	if err != nil {
		return -fuse.EIO
	}

	dirStat := fuse.Stat_t{Mode: fuse.S_IFDIR | vfs.dirMode}
	for name := range dirs {
		fill(name, &dirStat, 0)
	}

	for name, file := range files {
		fileStat := fuse.Stat_t{
			Mode:  fuse.S_IFREG | vfs.fileMode,
			Size:  file.Size,
			Mtim:  fuse.NewTimespec(time.Unix(file.ModTime, 0)),
			Nlink: 1,
			Uid:   vfs.uid,
			Gid:   vfs.gid,
		}
		fill(name, &fileStat, 0)
	}
	return 0
}

func (vfs *GooruVFS) Open(path string, flags int) (errc int, fh uint64) {
	// Enforce read-only
	if flags&os.O_RDWR != 0 || flags&os.O_WRONLY != 0 {
		return -fuse.EACCES, 0
	}

	realPath, ok := vfs.ResolveVirtualPath(path)
	if !ok {
		return -fuse.ENOENT, 0
	}

	file, err := os.Open(realPath)
	if err != nil {
		return -fuse.ENOENT, 0
	}

	vfs.openFileMu.Lock()
	defer vfs.openFileMu.Unlock()
	fh = vfs.nextFileHandle
	vfs.openFiles[fh] = file
	vfs.nextFileHandle++
	return 0, fh
}

func (vfs *GooruVFS) Read(path string, buff []byte, ofst int64, fh uint64) (n int) {
	vfs.openFileMu.Lock()
	file, ok := vfs.openFiles[fh]
	vfs.openFileMu.Unlock()

	if !ok {
		return -fuse.EBADF
	}

	n, err := file.ReadAt(buff, ofst)
	if err != nil && err != io.EOF {
		vfs.lastError = err
		return -fuse.EIO
	}
	return n
}

func (vfs *GooruVFS) Release(path string, fh uint64) (errc int) {
	vfs.openFileMu.Lock()
	defer vfs.openFileMu.Unlock()

	file, ok := vfs.openFiles[fh]
	if !ok {
		return -fuse.EBADF
	}
	delete(vfs.openFiles, fh)
	file.Close()
	return 0
}

func (vfs *GooruVFS) Getxattr(path string, name string) (errc int, value []byte) {
	return -fuse.ENOSYS, nil
}

func (vfs *GooruVFS) Listxattr(path string, fill func(name string) bool) (errc int) {
	return -fuse.ENOSYS
}

func (vfs *GooruVFS) OnError(op string, errc int) {
	if vfs.lastError != nil {
		fmt.Fprintf(os.Stderr, "Error in %s: %v (underlying error: %v)\n", op, syscall.Errno(errc), vfs.lastError)
		vfs.lastError = nil
	} else {
		fmt.Fprintf(os.Stderr, "Error in %s: %v\n", op, syscall.Errno(errc))
	}
}