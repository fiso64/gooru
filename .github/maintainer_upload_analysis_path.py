from pathlib import Path

path = Path('internal/serve/uploads.go')
text = path.read_text()
text = text.replace('''type StagedUpload struct {\n\tName     string\n\tPath     string\n\tSize     int64\n\tTargetID string\n\tStatus   string\n}''', '''type StagedUpload struct {\n\tName         string\n\tPath         string\n\tAnalysisPath string\n\tSize         int64\n\tTargetID     string\n\tStatus       string\n}''')

old = '''\t\tout = append(out, StagedUpload{Name: file.name, Path: path, Size: file.size, TargetID: file.targetID, Status: file.status})'''
new = '''\t\tout = append(out, StagedUpload{Name: file.name, Path: path, AnalysisPath: path, Size: file.size, TargetID: file.targetID, Status: file.status})'''
if old not in text:
    raise SystemExit('stagedUploads construction not found')
text = text.replace(old, new, 1)

old = '''\tresponse := UploadImportResponse{Files: make([]UploadedFileDTO, 0, len(files))}\n\thashes := make(map[string]string, len(files))\n\timportLocations := make([]types.LocationInfo, 0, len(files))\n\tresponseIndexByPath := make(map[string]int, len(files))\n\tfor _, file := range files {\n\t\tdto := UploadedFileDTO{Name: file.Name, Size: file.Size, TargetID: file.TargetID}\n\t\tif file.Status == "skipped" {\n\t\t\tdto.Status = "skipped"\n\t\t\tresponse.Files = append(response.Files, dto)\n\t\t\tcontinue\n\t\t}\n\t\tinfo, status, err := l.client.GetFileInfoForFile(file.Path, false)'''
new = '''\tresponse := UploadImportResponse{Files: make([]UploadedFileDTO, 0, len(files))}\n\thashes := make(map[string]string, len(files))\n\timportLocations := make([]types.LocationInfo, 0, len(files))\n\tresponseIndexByPath := make(map[string]int, len(files))\n\tanalysisPathByDestination := make(map[string]string, len(files))\n\tfor _, file := range files {\n\t\tdto := UploadedFileDTO{Name: file.Name, Size: file.Size, TargetID: file.TargetID}\n\t\tif file.Status == "skipped" {\n\t\t\tdto.Status = "skipped"\n\t\t\tresponse.Files = append(response.Files, dto)\n\t\t\tcontinue\n\t\t}\n\t\tanalysisPath := file.AnalysisPath\n\t\tif analysisPath == "" {\n\t\t\tanalysisPath = file.Path\n\t\t}\n\t\tinfo, status, err := l.client.GetFileInfoForFile(analysisPath, false)'''
if old not in text:
    raise SystemExit('import analysis block not found')
text = text.replace(old, new, 1)

text = text.replace('''\t\tif _, ok := hashes[info.Hash]; ok {\n\t\t\tdto.Status = "duplicate_in_batch"\n\t\t\t_ = os.Remove(file.Path)\n\t\t\tresponse.Files = append(response.Files, dto)\n\t\t\tcontinue\n\t\t}\n\t\thashes[info.Hash] = file.Path''', '''\t\tif _, ok := hashes[info.Hash]; ok {\n\t\t\tdto.Status = "duplicate_in_batch"\n\t\t\tremoveRejectedStagedUpload(file)\n\t\t\tresponse.Files = append(response.Files, dto)\n\t\t\tcontinue\n\t\t}\n\t\thashes[info.Hash] = file.Path''', 1)
text = text.replace('''\t\tif exists || status == types.StatusUntrackedContent || status == types.StatusOK {\n\t\t\tdto.Status = "duplicate_existing"\n\t\t\t_ = os.Remove(file.Path)\n\t\t\tresponse.Files = append(response.Files, dto)\n\t\t\tcontinue\n\t\t}''', '''\t\tif exists || status == types.StatusUntrackedContent || status == types.StatusOK {\n\t\t\tdto.Status = "duplicate_existing"\n\t\t\tremoveRejectedStagedUpload(file)\n\t\t\tresponse.Files = append(response.Files, dto)\n\t\t\tcontinue\n\t\t}''', 1)

old = '''\t\tresponseIndexByPath[file.Path] = len(response.Files) - 1\n\t\timportLocations = append(importLocations, types.LocationInfo{\n\t\t\tPath:      file.Path,\n\t\t\tHash:      info.Hash,\n\t\t\tSize:      info.Size,\n\t\t\tModTime:   info.ModTime,\n\t\t\tExtension: filepath.Ext(file.Path),\n\t\t})'''
new = '''\t\tresponseIndexByPath[file.Path] = len(response.Files) - 1\n\t\tanalysisPathByDestination[file.Path] = analysisPath\n\t\timportLocations = append(importLocations, types.LocationInfo{\n\t\t\tPath:      file.Path,\n\t\t\tHash:      info.Hash,\n\t\t\tSize:      info.Size,\n\t\t\tModTime:   info.ModTime,\n\t\t\tExtension: filepath.Ext(file.Path),\n\t\t})'''
if old not in text:
    raise SystemExit('import location block not found')
text = text.replace(old, new, 1)

old = '''\tresponse.AffectedCount = result.AffectedCount\n\tresponse.Notifications = notificationDTOs(result.Notifications)\n\tl.cacheImportedMediaMetadata(ctx, importLocations)\n\treturn response, nil\n}\n\nfunc (l *GooruLibrary) cacheImportedMediaMetadata(ctx context.Context, files []types.LocationInfo) {'''
new = '''\tresponse.AffectedCount = result.AffectedCount\n\tresponse.Notifications = notificationDTOs(result.Notifications)\n\tl.cacheImportedMediaMetadata(ctx, importLocations, analysisPathByDestination)\n\treturn response, nil\n}\n\nfunc removeRejectedStagedUpload(file StagedUpload) {\n\t_ = os.Remove(file.Path)\n\tif file.AnalysisPath != "" && file.AnalysisPath != file.Path {\n\t\t_ = os.Remove(file.AnalysisPath)\n\t}\n}\n\nfunc (l *GooruLibrary) cacheImportedMediaMetadata(ctx context.Context, files []types.LocationInfo, analysisPaths map[string]string) {'''
if old not in text:
    raise SystemExit('metadata cache signature block not found')
text = text.replace(old, new, 1)

old = '''\t\tmediaType := mediaTypeForPath(file.Path)\n\t\tmediaKind := mediaKindForType(mediaType)\n\t\tmetadata, err := provider.Metadata(ctx, file, mediaType, mediaKind)'''
new = '''\t\tmediaType := mediaTypeForPath(file.Path)\n\t\tmediaKind := mediaKindForType(mediaType)\n\t\tanalysisFile := file\n\t\tif analysisPath := analysisPaths[file.Path]; analysisPath != "" {\n\t\t\tanalysisFile.Path = analysisPath\n\t\t}\n\t\tmetadata, err := provider.Metadata(ctx, analysisFile, mediaType, mediaKind)'''
if old not in text:
    raise SystemExit('metadata provider call block not found')
text = text.replace(old, new, 1)
path.write_text(text)

new_test = r'''package serve

import (
    "context"
    "os"
    "path/filepath"
    "testing"

    core "gooru.local/gooru"
    "gooru.local/types"
)

type recordingImportMetadataProvider struct {
    path      string
    mediaType string
    mediaKind string
}

func (p *recordingImportMetadataProvider) Metadata(_ context.Context, file types.FileInfo, mediaType string, mediaKind string) (MediaMetadata, error) {
    p.path = file.Path
    p.mediaType = mediaType
    p.mediaKind = mediaKind
    width, height := 7, 5
    return MediaMetadata{ImageWidth: &width, ImageHeight: &height}, nil
}

func TestGooruUploadImportSeparatesAnalysisSourceFromRegisteredDestination(t *testing.T) {
    dir := t.TempDir()
    dbPath := filepath.Join(dir, "gooru.db")
    if err := core.Init(dbPath, types.StrategyFull, false); err != nil {
        t.Fatalf("init db: %v", err)
    }
    client, err := core.New(dbPath, false)
    if err != nil {
        t.Fatalf("open client: %v", err)
    }
    defer client.Close()

    source := filepath.Join(dir, "plaintext-staging.tmp")
    if err := os.WriteFile(source, mustReadFile(t, writePNGImage(t)), 0600); err != nil {
        t.Fatalf("write analysis source: %v", err)
    }
    destination := filepath.Join(dir, "library", "image.png")
    provider := &recordingImportMetadataProvider{}
    library := NewGooruLibrary(client, false)
    library.metadata = provider

    response, err := library.ImportUploadedFiles(context.Background(), []StagedUpload{{
        Name:         "image.png",
        Path:         destination,
        AnalysisPath: source,
        Size:         int64(len(mustReadFile(t, source))),
        TargetID:     "default",
    }}, []string{"uploaded"})
    if err != nil {
        t.Fatalf("import staged upload: %v", err)
    }
    if len(response.Files) != 1 || response.Files[0].Status != "imported" {
        t.Fatalf("unexpected response: %+v", response.Files)
    }

    registered, err := client.GetFileInfoByPath(destination)
    if err != nil {
        t.Fatalf("registered destination: %v", err)
    }
    if registered.Path != destination {
        t.Fatalf("registered logical destination = %q, want %q", registered.Path, destination)
    }
    if provider.path != source {
        t.Fatalf("metadata provider path = %q, want analysis source %q", provider.path, source)
    }
    if provider.mediaType != "image/png" || provider.mediaKind != "photo" {
        t.Fatalf("metadata classification = %q/%q, want image/png/photo", provider.mediaType, provider.mediaKind)
    }
    metadata, err := client.GetMediaMetadata(registered.ID)
    if err != nil {
        t.Fatalf("cached metadata: %v", err)
    }
    if metadata.ImageWidth == nil || *metadata.ImageWidth != 7 || metadata.ImageHeight == nil || *metadata.ImageHeight != 5 {
        t.Fatalf("cached metadata = %+v", metadata)
    }
}

func TestRejectedStagedUploadRemovesDistinctAnalysisSource(t *testing.T) {
    dir := t.TempDir()
    source := filepath.Join(dir, "analysis.tmp")
    destination := filepath.Join(dir, "stored.enc")
    if err := os.WriteFile(source, []byte("source"), 0600); err != nil {
        t.Fatal(err)
    }
    if err := os.WriteFile(destination, []byte("destination"), 0600); err != nil {
        t.Fatal(err)
    }

    removeRejectedStagedUpload(StagedUpload{Path: destination, AnalysisPath: source})
    for _, path := range []string{source, destination} {
        if _, err := os.Stat(path); !os.IsNotExist(err) {
            t.Fatalf("rejected staged upload left %s, err=%v", path, err)
        }
    }
}
'''
Path('internal/serve/upload_analysis_path_test.go').write_text(new_test)
