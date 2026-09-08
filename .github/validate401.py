from pathlib import Path


def replace(path: str, old: str, new: str) -> None:
    p = Path(path)
    text = p.read_text()
    if old not in text:
        raise SystemExit(f"expected text not found in {path}: {old[:80]!r}")
    p.write_text(text.replace(old, new, 1))


# Core: generic atomic registration + follow-up task boundary.
p = Path("gooru/tagging.go")
text = p.read_text()
old = '''// TagKnownFiles imports files whose content hash and filesystem metadata have
// already been computed by the caller, avoiding a second hashing pass.
func (c *Client) TagKnownFiles(files []types.LocationInfo, tags []string, progressCb func(filePath string, err error)) (types.TagOperationResult, error) {
\tresult := types.TagOperationResult{}
'''
new = '''// TagKnownFiles imports files whose content hash and filesystem metadata have
// already been computed by the caller, avoiding a second hashing pass.
func (c *Client) TagKnownFiles(files []types.LocationInfo, tags []string, progressCb func(filePath string, err error)) (types.TagOperationResult, error) {
\treturn c.TagKnownFilesWithBackgroundTasks(files, tags, nil, progressCb)
}

// TagKnownFilesWithBackgroundTasks atomically registers known files and enqueues
// durable follow-up work. If any task cannot be persisted, file registration and
// tag mutations roll back with it.
func (c *Client) TagKnownFilesWithBackgroundTasks(files []types.LocationInfo, tags []string, tasks []BackgroundTaskRequest, progressCb func(filePath string, err error)) (types.TagOperationResult, error) {
\tresult := types.TagOperationResult{}
'''
if old not in text:
    raise SystemExit("TagKnownFiles signature block not found")
text = text.replace(old, new, 1)
text = text.replace(
    "affectedCount, _, err := c.executeTaggingTransaction(analysis, tags, opTag)",
    "affectedCount, _, err := c.executeTaggingTransaction(analysis, tags, opTag, tasks)",
    1,
)
text = text.replace(
    "func (c *Client) executeTaggingTransaction(analysis *fileStateAnalysis, tags []string, kind opKind) (int64, map[string]string, error) {",
    "func (c *Client) executeTaggingTransaction(analysis *fileStateAnalysis, tags []string, kind opKind, tasks []BackgroundTaskRequest) (int64, map[string]string, error) {",
    1,
)
needle = '''\t// 3. Perform the specific tagging operation.
\taffectedCount, err := c.applyTaggingOperationInTx(tx, analysis.allHashes, tags, kind)
\tif err != nil {
\t\treturn 0, nil, err
\t}

\treturn affectedCount, movesHandled, tx.Commit()
'''
replacement = '''\t// 3. Perform the specific tagging operation.
\taffectedCount, err := c.applyTaggingOperationInTx(tx, analysis.allHashes, tags, kind)
\tif err != nil {
\t\treturn 0, nil, err
\t}

\t// 4. Persist durable follow-up work in the same transaction as content registration.
\tfor _, task := range tasks {
\t\tif _, _, err := c.enqueueBackgroundTask(tx, task); err != nil {
\t\t\treturn 0, nil, fmt.Errorf("failed to enqueue background task: %w", err)
\t\t}
\t}

\treturn affectedCount, movesHandled, tx.Commit()
'''
if needle not in text:
    raise SystemExit("transaction tail not found")
text = text.replace(needle, replacement, 1)
text = text.replace(
    "c.executeTaggingTransaction(analysis, tags, kind)",
    "c.executeTaggingTransaction(analysis, tags, kind, nil)",
    1,
)
p.write_text(text)

Path("gooru/content_lookup.go").write_text('''package gooru

import (
\t"fmt"

\t"gooru.local/types"
)

// GetFileInfoByContentHash resolves a currently tracked location for immutable
// content identity. The store's content query orders paths deterministically, so
// background work survives moves without persisting a stale filesystem path.
func (c *Client) GetFileInfoByContentHash(hash string) (types.FileInfo, error) {
\tfiles, err := c.store.GetFilesInfoByContentQuery("SELECT ?", []interface{}{hash})
\tif err != nil {
\t\treturn types.FileInfo{}, err
\t}
\tif len(files) == 0 {
\t\treturn types.FileInfo{}, fmt.Errorf("content %q has no tracked location", hash)
\t}
\treturn files[0], nil
}
''')

# GooruLibrary keeps media-owned enqueue policy injected by server composition.
replace(
    "internal/serve/browse.go",
    '''type GooruLibrary struct {
\tclient       *core.Client
\tverbose      bool
\tmetadata     MediaMetadataProvider
\tencryption   EncryptionConfig
\tmanagedRoots []string
}''',
    '''type GooruLibrary struct {
\tclient          *core.Client
\tverbose         bool
\tmetadata        MediaMetadataProvider
\tencryption      EncryptionConfig
\tmanagedRoots    []string
\tbackgroundTasks func(types.LocationInfo) []core.BackgroundTaskRequest
}''',
)
constructor = '''func NewGooruLibrary(client *core.Client, verbose bool) *GooruLibrary {
\treturn &GooruLibrary{client: client, verbose: verbose, metadata: BasicMediaMetadataProvider{}}
}
'''
replace(
    "internal/serve/browse.go",
    constructor,
    constructor
    + '''
func (l *GooruLibrary) GetFileByContentHash(ctx context.Context, hash string) (types.FileInfo, error) {
\tif err := ctx.Err(); err != nil {
\t\treturn types.FileInfo{}, err
\t}
\treturn l.client.GetFileInfoByContentHash(hash)
}
''',
)

# Upload registration computes follow-up tasks before entering the atomic core commit.
replace(
    "internal/serve/uploads.go",
    '''\t"gooru.local/internal/query"
\t"gooru.local/types"''',
    '''\tcore "gooru.local/gooru"
\t"gooru.local/internal/query"
\t"gooru.local/types"''',
)
replace(
    "internal/serve/uploads.go",
    '''\tfailures := make(map[string]string)
\tresult, err := l.client.TagKnownFiles(importLocations, tags, func(filePath string, err error) {''',
    '''\tbackgroundTasks := make([]core.BackgroundTaskRequest, 0, len(importLocations))
\tif l.backgroundTasks != nil {
\t\tfor _, location := range importLocations {
\t\t\tbackgroundTasks = append(backgroundTasks, l.backgroundTasks(location)...)
\t\t}
\t}
\tfailures := make(map[string]string)
\tresult, err := l.client.TagKnownFilesWithBackgroundTasks(importLocations, tags, backgroundTasks, func(filePath string, err error) {''',
)

# Compose media once and preserve the concrete content lookup behind public wrappers.
replace(
    "internal/serve/server.go",
    '''\tmanagedFiles   *managedfile.Writer
}''',
    '''\tmanagedFiles      *managedfile.Writer
\tbackgroundContent contentHashLibrary
}''',
)
replace(
    "internal/serve/server.go",
    '''func NewServerWithLibrary(cfg Config, library Library) *Server {
\tmetadata := NewMediaMetadataProvider(cfg)
\tif gooruLibrary, ok := library.(*GooruLibrary); ok {''',
    '''func NewServerWithLibrary(cfg Config, library Library) *Server {
\tmetadata := NewMediaMetadataProvider(cfg)
\tmedia := newComposedMediaServiceFromConfig(cfg)
\tvar backgroundContent contentHashLibrary
\tif gooruLibrary, ok := library.(*GooruLibrary); ok {
\t\tbackgroundContent = gooruLibrary
\t\tgooruLibrary.backgroundTasks = media.backgroundTaskRequests''',
)
replace(
    "internal/serve/server.go",
    '''\t\tmedia:          newComposedMediaServiceFromConfig(cfg),''',
    '''\t\tmedia:          media,''',
)
replace(
    "internal/serve/server.go",
    '''\t\tmanagedFiles:   managedFiles,
\t}''',
    '''\t\tmanagedFiles:      managedFiles,
\t\tbackgroundContent: backgroundContent,
\t}''',
)

Path("internal/serve/background_thumbnails.go").write_text('''package serve

import (
\t"context"
\t"fmt"
\t"io"
\t"path/filepath"
\t"strings"

\tcore "gooru.local/gooru"
\t"gooru.local/types"
)

const (
\tbackgroundThumbnailTaskKind      = "media.thumbnail"
\tbackgroundThumbnailResourceClass = "media"
)

type contentHashLibrary interface {
\tGetFileByContentHash(context.Context, string) (types.FileInfo, error)
}

func (m *MediaService) browsingThumbnailSpec(file types.FileInfo) (size int, format string, relativePath string, ok bool) {
\tif m == nil || len(m.cfg.Media.ThumbnailSizes) == 0 || m.thumbnailer == nil {
\t\treturn 0, "", "", false
\t}
\tmediaKind := mediaKindForType(mediaTypeForPath(file.Path))
\tif mediaKind != "photo" && mediaKind != "gif" && mediaKind != "video" && !strings.EqualFold(filepath.Ext(file.Path), ".cbz") {
\t\treturn 0, "", "", false
\t}
\tsize = m.cfg.Media.ThumbnailSizes[0]
\tformat = strings.ToLower(strings.TrimSpace(m.cfg.Media.ThumbnailFormat))
\tif format == "" {
\t\tformat = "jpeg"
\t}
\treturn size, format, m.derivativeRelativePath(file, "thumbnail", size, format), true
}

func (m *MediaService) backgroundTaskRequests(location types.LocationInfo) []core.BackgroundTaskRequest {
\tfile := types.FileInfo{Path: location.Path, Hash: location.Hash}
\t_, _, inputKey, ok := m.browsingThumbnailSpec(file)
\tif !ok {
\t\treturn nil
\t}
\treturn []core.BackgroundTaskRequest{{
\t\tDedupeKey:     "thumbnail:" + inputKey,
\t\tKind:          backgroundThumbnailTaskKind,
\t\tSubjectKind:   "content",
\t\tSubjectID:     location.Hash,
\t\tInputKey:      inputKey,
\t\tResourceClass: backgroundThumbnailResourceClass,
\t\tMaxAttempts:   5,
\t}}
}

func (m *MediaService) ensureBrowsingThumbnail(file types.FileInfo) error {
\tsize, format, relativePath, ok := m.browsingThumbnailSpec(file)
\tif !ok {
\t\treturn nil
\t}
\tif m.derivativeStoreErr != nil {
\t\treturn m.derivativeStoreErr
\t}
\tif m.derivatives == nil {
\t\treturn fmt.Errorf("media derivative store is not configured")
\t}
\tartifact, err := m.derivatives.GetOrGenerate(relativePath, func(dst io.Writer) error {
\t\treturn m.generateDerivative(file, dst, size, format, "thumbnail")
\t})
\tif err != nil {
\t\treturn err
\t}
\treturn artifact.Close()
}

func (s *Server) backgroundThumbnailHandler(ctx context.Context, task core.BackgroundTask) error {
\tif s.backgroundContent == nil {
\t\treturn fmt.Errorf("content hash lookup is not configured")
\t}
\tif task.SubjectKind != "content" || strings.TrimSpace(task.SubjectID) == "" {
\t\treturn fmt.Errorf("thumbnail task has invalid content identity")
\t}
\tfile, err := s.backgroundContent.GetFileByContentHash(ctx, task.SubjectID)
\tif err != nil {
\t\treturn err
\t}
\treturn s.media.ensureBrowsingThumbnail(file)
}

// NewBackgroundRuntime composes the serve process's durable media worker. The
// HTTP server and worker share one MediaService, so eager and lazy generation use
// identical keys, encryption policy, locking, and fallback behavior.
func (s *Server) NewBackgroundRuntime(client *core.Client, workerID string) (BackgroundRuntime, error) {
\tif client == nil {
\t\treturn nil, fmt.Errorf("background client is required")
\t}
\treturn client.NewBackgroundRuntime(core.BackgroundWorkerConfig{
\t\tResourceClass: backgroundThumbnailResourceClass,
\t\tWorkerID:      workerID,
\t\tHandlers: map[string]core.BackgroundTaskHandler{
\t\t\tbackgroundThumbnailTaskKind: s.backgroundThumbnailHandler,
\t\t},
\t})
}
''')

replace(
    "cmd/gooru/cmd/serve.go",
    '''\t\terr = server.ListenAndServe(ctx)''',
    '''\t\truntime, err := server.NewBackgroundRuntime(client, fmt.Sprintf("serve-%d", os.Getpid()))
\t\tif err != nil {
\t\t\treturn fmt.Errorf("failed to initialize background runtime: %w", err)
\t\t}
\t\terr = server.ListenAndServeWithBackgroundRuntime(ctx, runtime)''',
)

Path("gooru/tag_known_background_test.go").write_text('''package gooru

import (
\t"testing"

\t"gooru.local/types"
)

func TestTagKnownFilesWithBackgroundTasksRollsBackRegistrationWhenEnqueueFails(t *testing.T) {
\tclient := newBackgroundEnqueueTestClient(t)
\tlocation := types.LocationInfo{Path: "/library/item.jpg", Hash: "hash-atomic-import", Size: 12, ModTime: 34, Extension: ".jpg"}
\t_, err := client.TagKnownFilesWithBackgroundTasks([]types.LocationInfo{location}, nil, []BackgroundTaskRequest{{
\t\tDedupeKey: "invalid-without-kind",
\t}}, nil)
\tif err == nil {
\t\tt.Fatal("expected invalid background task to fail")
\t}
\texists, lookupErr := client.ContentExists(location.Hash)
\tif lookupErr != nil {
\t\tt.Fatalf("ContentExists: %v", lookupErr)
\t}
\tif exists {
\t\tt.Fatal("content registration committed despite background enqueue failure")
\t}
}
''')

Path("internal/serve/background_thumbnails_test.go").write_text('''package serve

import (
\t"context"
\t"testing"

\tcore "gooru.local/gooru"
\t"gooru.local/types"
)

func TestBackgroundThumbnailTaskUsesBrowsingDerivativeIdentity(t *testing.T) {
\tcfg := DefaultConfig("")
\tcfg.Media.ThumbnailSizes = []int{320, 640}
\tcfg.Media.ThumbnailFormat = "jpeg"
\tmedia := NewMediaService(cfg)
\tlocation := types.LocationInfo{Path: "/library/photo.jpg", Hash: "content-hash"}

\ttasks := media.backgroundTaskRequests(location)
\tif len(tasks) != 1 {
\t\tt.Fatalf("tasks = %d, want 1", len(tasks))
\t}
\t_, _, expected, ok := media.browsingThumbnailSpec(types.FileInfo{Path: location.Path, Hash: location.Hash})
\tif !ok {
\t\tt.Fatal("browsing thumbnail unexpectedly unsupported")
\t}
\ttask := tasks[0]
\tif task.InputKey != expected || task.DedupeKey != "thumbnail:"+expected || task.SubjectID != location.Hash || task.ResourceClass != backgroundThumbnailResourceClass {
\t\tt.Fatalf("unexpected task: %+v", task)
\t}
}

func TestBackgroundThumbnailHandlerUsesCurrentContentLocation(t *testing.T) {
\tcfg := DefaultConfig("")
\tcfg.Media.ThumbnailSizes = nil
\tserver := NewServer(cfg)
\tserver.backgroundContent = fakeContentHashLibrary{file: types.FileInfo{Path: "/moved/photo.jpg", Hash: "hash"}}
\ttask := core.BackgroundTask{Kind: backgroundThumbnailTaskKind, SubjectKind: "content", SubjectID: "hash"}
\tif err := server.backgroundThumbnailHandler(context.Background(), task); err != nil {
\t\tt.Fatalf("handler: %v", err)
\t}
}

type fakeContentHashLibrary struct{ file types.FileInfo }

func (f fakeContentHashLibrary) GetFileByContentHash(context.Context, string) (types.FileInfo, error) {
\treturn f.file, nil
}
''')
