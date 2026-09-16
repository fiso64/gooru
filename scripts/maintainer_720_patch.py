#!/usr/bin/env python3
from pathlib import Path
import re

ROOT = Path(__file__).resolve().parents[1]


def load(path):
    return (ROOT / path).read_text()


def save(path, text):
    (ROOT / path).write_text(text)


def replace(path, old, new, count=1):
    text = load(path)
    found = text.count(old)
    if found < count:
        raise SystemExit(f"{path}: expected at least {count} occurrence(s), found {found}: {old[:100]!r}")
    text = text.replace(old, new, count)
    save(path, text)


def replace_all(path, old, new):
    text = load(path)
    if old not in text:
        raise SystemExit(f"{path}: missing expected text: {old[:100]!r}")
    save(path, text.replace(old, new))


def replace_func(path, name, body):
    text = load(path)
    start_match = re.search(rf"(?m)^func {re.escape(name)}\b", text)
    if not start_match:
        raise SystemExit(f"{path}: function {name} not found")
    start = start_match.start()
    next_match = re.search(r"(?m)^(?:func|type|var|const)\b", text[start_match.end():])
    end = len(text) if not next_match else start_match.end() + next_match.start()
    save(path, text[:start] + body.rstrip() + "\n\n" + text[end:])


def remove_decl(path, kind, name):
    text = load(path)
    start_match = re.search(rf"(?m)^{kind} {re.escape(name)}\b", text)
    if not start_match:
        raise SystemExit(f"{path}: {kind} {name} not found")
    start = start_match.start()
    next_match = re.search(r"(?m)^(?:func|type|var|const)\b", text[start_match.end():])
    end = len(text) if not next_match else start_match.end() + next_match.start()
    save(path, text[:start] + text[end:])


def remove_tests_matching(path, needle):
    text = load(path)
    names = re.findall(rf"(?m)^func (Test\w*{re.escape(needle)}\w*)\(", text)
    for name in names:
        remove_decl(path, "func", name)


def delete(path):
    p = ROOT / path
    if not p.exists():
        raise SystemExit(f"{path}: expected file to delete")
    p.unlink()

# Config: conflict policy is request-only; server config no longer controls filename semantics.
replace("internal/serve/config.go", '\tConflictPolicy   string         `yaml:"conflict_policy"`\n', "")
replace("internal/serve/config.go", 'Uploads: UploadsConfig{Enabled: false, ConflictPolicy: "rename", PreserveModTime: true}', 'Uploads: UploadsConfig{Enabled: false, PreserveModTime: true}')
replace("internal/serve/config.go", '''\tif cfg.Uploads.ConflictPolicy == "" {\n\t\tcfg.Uploads.ConflictPolicy = "rename"\n\t}\n\tswitch cfg.Uploads.ConflictPolicy {\n\tcase "skip", "rename", "replace", "error":\n\tdefault:\n\t\terrs = append(errs, errors.New("uploads.conflict_policy must be one of: skip, rename, replace, error"))\n\t}\n''', "")
replace("internal/serve/config_test.go", '''\tif cfg.Uploads.ConflictPolicy != "rename" {\n\t\tt.Fatalf("unexpected upload conflict policy default %q", cfg.Uploads.ConflictPolicy)\n\t}\n''', "")
remove_decl("internal/serve/config_test.go", "func", "TestLoadConfigRejectsInvalidUploadConflictPolicy")
delete("internal/serve/config_upload_policy_test.go")

# Core synchronous upload path.
replace("internal/serve/uploads.go", '''\tactivated, err := activateSavedReplacements(saved)\n\tif err != nil {\n\t\tremoveSavedUploads(saved)\n\t\twriteError(w, http.StatusInternalServerError, "internal_error", "failed to import uploaded files", nil)\n\t\treturn\n\t}\n\tresponse, err := importer.ImportUploadedFiles(r.Context(), stagedUploads(saved), tags)\n\tif err != nil {\n\t\trollbackErr := rollbackSavedReplacements(activated)\n\t\tremoveSavedUploads(saved)\n\t\tif rollbackErr != nil {\n\t\t\terr = fmt.Errorf("%w; replacement rollback failed: %v", err, rollbackErr)\n\t\t}\n\t\tif errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {\n\t\t\twriteError(w, http.StatusRequestTimeout, "request_canceled", "request was canceled", nil)\n\t\t\treturn\n\t\t}\n\t\twriteError(w, http.StatusInternalServerError, "internal_error", "failed to import uploaded files", nil)\n\t\treturn\n\t}\n\tif err := settleSavedReplacements(activated, response); err != nil {\n\t\twriteError(w, http.StatusInternalServerError, "internal_error", "failed to finalize uploaded files", nil)\n\t\treturn\n\t}\n''', '''\tresponse, err := importer.ImportUploadedFiles(r.Context(), stagedUploads(saved), tags)\n\tif err != nil {\n\t\tremoveSavedUploads(saved)\n\t\tif errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {\n\t\t\twriteError(w, http.StatusRequestTimeout, "request_canceled", "request was canceled", nil)\n\t\t\treturn\n\t\t}\n\t\twriteError(w, http.StatusInternalServerError, "internal_error", "failed to import uploaded files", nil)\n\t\treturn\n\t}\n''')
replace("internal/serve/uploads.go", "\treplace         bool\n", "")
replace_func("internal/serve/uploads.go", "uploadConflictPolicy", '''func uploadConflictPolicy(requested string) (string, error) {
\tswitch strings.TrimSpace(requested) {
\tcase "", "rename":
\t\treturn "rename", nil
\tcase "error":
\t\treturn "error", nil
\tdefault:
\t\treturn "", errors.New("conflict_policy must be one of: rename, error")
\t}
}''')
replace("internal/serve/uploads.go", "dst, path, tmpPath, skipped, err := createUploadDestination(target.Path, name, conflictPolicy)", "dst, path, tmpPath, err := createUploadDestination(target.Path, name, conflictPolicy)")
replace("internal/serve/uploads.go", '''\t\tif skipped {\n\t\t\t_ = src.Close()\n\t\t\tsaved = append(saved, savedUpload{name: name, path: path, destinationPath: path, size: header.Size, targetID: target.ID, status: "skipped"})\n\t\t\tcontinue\n\t\t}\n''', "")
replace("internal/serve/uploads.go", '''\t\tif conflictPolicy == "replace" {\n\t\t\tsaved = append(saved, savedUpload{\n\t\t\t\tname:            filepath.Base(path),\n\t\t\t\tpath:            tmpPath,\n\t\t\t\tdestinationPath: path,\n\t\t\t\tsize:            size,\n\t\t\t\ttargetID:        target.ID,\n\t\t\t\treplace:         true,\n\t\t\t})\n\t\t\tcontinue\n\t\t}\n''', "")
replace("internal/serve/uploads.go", '''\t\tif file.replace {\n\t\t\t_ = os.Remove(file.path)\n\t\t\tcontinue\n\t\t}\n''', "")
for name in ["activateSavedReplacements", "settleSavedReplacements", "rollbackSavedReplacements", "activateReplacement", "resumeReplacementActivation", "createReplacementStateMarker", "replacementPathExists", "rollbackReplacement", "commitReplacement"]:
    remove_decl("internal/serve/uploads.go", "func", name)
for name in ["activatedReplacement", "activatedSavedReplacement"]:
    remove_decl("internal/serve/uploads.go", "type", name)
replace_func("internal/serve/uploads.go", "createUploadDestination", '''func createUploadDestination(dir string, name string, conflictPolicy string) (*os.File, string, string, error) {
\text := filepath.Ext(name)
\tbase := strings.TrimSuffix(filepath.Base(name), ext)
\tif base == "" {
\t\tbase = "upload"
\t}
\tfor i := 0; i < 10_000; i++ {
\t\tcandidate := name
\t\tif i > 0 {
\t\t\tcandidate = fmt.Sprintf("%s-%d%s", base, i, ext)
\t\t}
\t\tpath := filepath.Join(dir, candidate)
\t\tif _, err := os.Stat(path); err == nil {
\t\t\tif conflictPolicy == "error" {
\t\t\t\treturn nil, "", "", errUploadConflict
\t\t\t}
\t\t\tcontinue
\t\t} else if !errors.Is(err, os.ErrNotExist) {
\t\t\treturn nil, "", "", fmt.Errorf("failed to create uploaded file")
\t\t}
\t\tfile, err := os.CreateTemp(dir, "."+candidate+".tmp-*")
\t\tif err != nil {
\t\t\treturn nil, "", "", fmt.Errorf("failed to create uploaded file")
\t\t}
\t\treturn file, path, file.Name(), nil
\t}
\treturn nil, "", "", errors.New("could not choose a non-conflicting upload filename")
}''')
replace("internal/serve/uploads.go", '''\t\tpath := file.path\n\t\tif file.replace {\n\t\t\tpath = file.destinationPath\n\t\t}\n''', "\t\tpath := file.path\n")
replace("internal/serve/uploads.go", "return l.importUploadedFiles(ctx, files, tags, backgroundUploadImportState{}, nil)", "return l.importUploadedFiles(ctx, files, tags, backgroundUploadImportState{})")
replace("internal/serve/uploads.go", "func (l *GooruLibrary) importUploadedFiles(ctx context.Context, files []StagedUpload, tags []string, state backgroundUploadImportState, activated []activatedSavedReplacement) (UploadImportResponse, error)", "func (l *GooruLibrary) importUploadedFiles(ctx context.Context, files []StagedUpload, tags []string, state backgroundUploadImportState) (UploadImportResponse, error)")
replace_all("internal/serve/uploads.go", "backgroundUploadActivatedCheckpoint(activated, len(files), completed, completedPrefix)", "backgroundUploadActivatedCheckpoint(len(files), completed, completedPrefix)")
replace_all("internal/serve/uploads.go", "backgroundUploadImportedCheckpoint(activated, response)", "backgroundUploadImportedCheckpoint(response)")

# Streamed synchronous staging: rename by default, error only when explicitly requested.
replace_all("internal/serve/upload_stream.go", "uploadConflictPolicy(conflictRequested, s.cfg.Uploads.ConflictPolicy)", "uploadConflictPolicy(conflictRequested)")
replace_func("internal/serve/upload_stream.go", "finalizeStreamedUpload", '''func finalizeStreamedUpload(target UploadTarget, file streamedUpload, conflictPolicy string, preserveModTime bool) (savedUpload, error) {
\tpath, err := chooseUploadDestination(target.Path, file.name, conflictPolicy)
\tif err != nil {
\t\treturn savedUpload{}, uploadFileError{name: file.name, err: err}
\t}
\tstagedPath, err := moveStreamedUploadIntoDir(file.path, target.Path, file.name)
\tif err != nil {
\t\treturn savedUpload{}, uploadFileError{name: file.name, err: err}
\t}
\tif err := applyUploadedSourceModTime(stagedPath, file.sourceModTime, preserveModTime); err != nil {
\t\t_ = os.Remove(stagedPath)
\t\treturn savedUpload{}, uploadFileError{name: file.name, err: err}
\t}
\tif err := commitUploadDestinationWithOwnership(stagedPath, path); err != nil {
\t\t_ = os.Remove(stagedPath)
\t\treturn savedUpload{}, uploadFileError{name: file.name, err: err}
\t}
\treturn savedUpload{name: filepath.Base(path), path: path, destinationPath: path, size: file.size, targetID: target.ID, sourceModTime: file.sourceModTime, ownershipPath: stagedPath}, nil
}''')
replace_func("internal/serve/upload_stream.go", "chooseUploadDestination", '''func chooseUploadDestination(dir, name, conflictPolicy string) (string, error) {
\text := filepath.Ext(name)
\tbase := name[:len(name)-len(ext)]
\tif base == "" {
\t\tbase = "upload"
\t}
\tfor i := 0; i < 10_000; i++ {
\t\tcandidate := name
\t\tif i > 0 {
\t\t\tcandidate = fmt.Sprintf("%s-%d%s", base, i, ext)
\t\t}
\t\tpath := filepath.Join(dir, candidate)
\t\t_, statErr := os.Stat(path)
\t\tif statErr == nil {
\t\t\tif conflictPolicy == "error" {
\t\t\t\treturn "", errUploadConflict
\t\t\t}
\t\t\tcontinue
\t\t}
\t\tif errors.Is(statErr, os.ErrNotExist) {
\t\t\treturn path, nil
\t\t}
\t\treturn "", errors.New("failed to inspect upload destination")
\t}
\treturn "", errors.New("could not choose a non-conflicting upload filename")
}''')

# Durable staging retains ordinary ownership/replay machinery but no replacement policy.
replace_all("internal/serve/upload_durable_staging.go", "uploadConflictPolicy(conflictRequested, s.cfg.Uploads.ConflictPolicy)", "uploadConflictPolicy(conflictRequested)")
replace("internal/serve/upload_durable_staging.go", "path, skipped, replace, destinationErr := chooseDurableUploadDestination(target.Path, file.name, conflictPolicy, reserved)", "path, destinationErr := chooseDurableUploadDestination(target.Path, file.name, conflictPolicy, reserved)")
replace("internal/serve/upload_durable_staging.go", '''\t\tif skipped {\n\t\t\t_ = os.Remove(file.path)\n\t\t\tfile.path = ""\n\t\t\tsaved = append(saved, savedUpload{name: file.name, path: path, destinationPath: path, size: file.size, targetID: target.ID, status: "skipped", sourceModTime: file.sourceModTime})\n\t\t\tcontinue\n\t\t}\n''', "")
replace("internal/serve/upload_durable_staging.go", "replace: replace, ", "")
replace_func("internal/serve/upload_durable_staging.go", "chooseDurableUploadDestination", '''func chooseDurableUploadDestination(dir, name, conflictPolicy string, reserved map[string]struct{}) (string, error) {
\text := filepath.Ext(name)
\tbase := name[:len(name)-len(ext)]
\tif base == "" {
\t\tbase = "upload"
\t}
\tfor i := 0; i < 10_000; i++ {
\t\tcandidate := name
\t\tif i > 0 {
\t\t\tcandidate = fmt.Sprintf("%s-%d%s", base, i, ext)
\t\t}
\t\tpath := filepath.Join(dir, candidate)
\t\t_, planned := reserved[path]
\t\t_, statErr := os.Stat(path)
\t\tif statErr == nil || planned {
\t\t\tif conflictPolicy == "error" {
\t\t\t\treturn "", errUploadConflict
\t\t\t}
\t\t\tcontinue
\t\t}
\t\tif errors.Is(statErr, os.ErrNotExist) {
\t\t\treturn path, nil
\t\t}
\t\treturn "", errors.New("failed to inspect upload destination")
\t}
\treturn "", errors.New("could not choose a non-conflicting upload filename")
}''')

# Durable checkpoint format and worker no longer carry replacement transactions.
replace("internal/serve/background_uploads.go", '\tReplacements           []backgroundUploadReplacementCheckpoint `json:"replacements,omitempty"`\n', "")
remove_decl("internal/serve/background_uploads.go", "type", "backgroundUploadReplacementCheckpoint")
replace("internal/serve/background_uploads.go", '\tReplace         bool      `json:"replace,omitempty"`\n', "")
replace_func("internal/serve/background_uploads.go", "backgroundUploadActivatedCheckpoint", '''func backgroundUploadActivatedCheckpoint(progress ...int) backgroundUploadCheckpoint {
\tcheckpoint := backgroundUploadCheckpoint{Phase: backgroundUploadPhaseActivated}
\tif len(progress) > 0 {
\t\tcheckpoint.FileTotal = progress[0]
\t}
\tif len(progress) > 1 {
\t\tcheckpoint.FilesCompleted = progress[1]
\t}
\tif len(progress) > 2 {
\t\tcheckpoint.FilesCompletedPrefix = progress[2]
\t}
\treturn checkpoint
}''')
replace_func("internal/serve/background_uploads.go", "backgroundUploadImportedCheckpoint", '''func backgroundUploadImportedCheckpoint(response UploadImportResponse) backgroundUploadCheckpoint {
\ttotal := len(response.Files)
\treturn backgroundUploadCheckpoint{
\t\tPhase:                backgroundUploadPhaseImported,
\t\tResponse:             &response,
\t\tFileTotal:            total,
\t\tFilesCompleted:       total,
\t\tFilesCompletedPrefix: total,
\t}
}''')
for name in ["backgroundUploadReplacementCheckpoints", "activatedSavedReplacementsFromCheckpoint"]:
    remove_decl("internal/serve/background_uploads.go", "func", name)
replace_all("internal/serve/background_uploads.go", "\n\t\t\tReplace:         file.replace,", "")
replace_all("internal/serve/background_uploads.go", "\n\t\t\treplace:         file.Replace,", "")
replace("internal/serve/background_uploads.go", '''\t\tif file.Replace && file.DestinationPath == "" {\n\t\t\treturn fmt.Errorf("upload background task file %d is missing replacement destination", index)\n\t\t}\n''', "")

replace("internal/serve/background_upload_worker.go", "importUploadedFiles(context.Context, []StagedUpload, []string, backgroundUploadImportState, []activatedSavedReplacement) (UploadImportResponse, error)", "importUploadedFiles(context.Context, []StagedUpload, []string, backgroundUploadImportState) (UploadImportResponse, error)")
replace_func("internal/serve/background_upload_worker.go", "runBackgroundUploadTask", '''func runBackgroundUploadTask(ctx context.Context, importer backgroundUploadImporter, store backgroundUploadWorkerStore, task core.BackgroundTask) error {
\tif importer == nil {
\t\treturn errors.New("durable upload import service is not configured")
\t}
\tif store == nil {
\t\treturn errors.New("upload background operation store is not configured")
\t}
\tfiles, tags, err := decodeBackgroundUploadTask(task)
\tif err != nil {
\t\treturn err
\t}
\trecovery, checkpoint, err := loadBackgroundUploadRecoveryState(store, task)
\tif err != nil {
\t\treturn err
\t}
\tswitch checkpoint.Phase {
\tcase backgroundUploadPhaseStaged:
\t\tif err := activateSavedDurableUploads(files); err != nil {
\t\t\treturn err
\t\t}
\t\tcheckpoint = backgroundUploadActivatedCheckpoint(len(files), 0)
\t\tif err := recovery.setCheckpoint(checkpoint); err != nil {
\t\t\tcanceled, stateErr := backgroundUploadOperationCanceled(store, task.OperationID)
\t\t\tif stateErr != nil {
\t\t\t\treturn errors.Join(fmt.Errorf("persist activated upload checkpoint: %w", err), fmt.Errorf("inspect upload cancellation: %w", stateErr))
\t\t\t}
\t\t\tif canceled {
\t\t\t\tif cleanupErr := cleanupCanceledClaimedUpload(files); cleanupErr != nil {
\t\t\t\t\treturn cleanupErr
\t\t\t\t}
\t\t\t\treturn nil
\t\t\t}
\t\t\treturn fmt.Errorf("persist activated upload checkpoint: %w", err)
\t\t}
\tcase backgroundUploadPhaseActivated:
\t\tif err := restoreDurableNonreplacementDestinations(files); err != nil {
\t\t\treturn fmt.Errorf("restore activated durable uploads: %w", err)
\t\t}
\tcase backgroundUploadPhaseImported:
\t\tif checkpoint.Response == nil {
\t\t\treturn errors.New("imported upload checkpoint is missing response")
\t\t}
\t\tresponse := *checkpoint.Response
\t\tresponse.Files = append([]UploadedFileDTO(nil), response.Files...)
\t\tif enricher, ok := importer.(backgroundUploadResultIdentityEnricher); ok {
\t\t\tif err := enricher.populateUploadFileIDs(&response, durableStagedUploads(files)); err != nil {
\t\t\t\treturn fmt.Errorf("restore upload result identities: %w", err)
\t\t\t}
\t\t}
\t\tif err := settleDurableNonreplacementActivations(files); err != nil {
\t\t\treturn fmt.Errorf("settle imported durable uploads: %w", err)
\t\t}
\t\tif err := recovery.setResult(response); err != nil {
\t\t\tcanceled, stateErr := backgroundUploadOperationCanceled(store, task.OperationID)
\t\t\tif stateErr != nil {
\t\t\t\treturn errors.Join(fmt.Errorf("publish upload background result: %w", err), fmt.Errorf("inspect upload cancellation: %w", stateErr))
\t\t\t}
\t\t\tif canceled {
\t\t\t\treturn nil
\t\t\t}
\t\t\treturn fmt.Errorf("publish upload background result: %w", err)
\t\t}
\t\treturn nil
\tdefault:
\t\treturn fmt.Errorf("upload background checkpoint has invalid phase %q", checkpoint.Phase)
\t}
\n\tresponse, err := importer.importUploadedFiles(withDeferredUploadMediaMetadata(ctx), durableStagedUploads(files), tags, recovery.importState())
\tif err != nil {
\t\tcanceled, stateErr := backgroundUploadOperationCanceled(store, task.OperationID)
\t\tif stateErr != nil {
\t\t\treturn errors.Join(err, fmt.Errorf("inspect upload cancellation: %w", stateErr))
\t\t}
\t\tif canceled {
\t\t\tif cleanupErr := cleanupCanceledClaimedUpload(files); cleanupErr != nil {
\t\t\t\treturn cleanupErr
\t\t\t}
\t\t\treturn nil
\t\t}
\t\treturn err
\t}
\n\tcheckpoint = backgroundUploadImportedCheckpoint(response)
\tif err := recovery.setCheckpoint(checkpoint); err != nil {
\t\tcanceled, stateErr := backgroundUploadOperationCanceled(store, task.OperationID)
\t\tif stateErr != nil {
\t\t\treturn errors.Join(fmt.Errorf("persist imported upload checkpoint: %w", err), fmt.Errorf("inspect upload cancellation: %w", stateErr))
\t\t}
\t\tif canceled {
\t\t\tif settleErr := settleDurableNonreplacementActivations(files); settleErr != nil {
\t\t\t\treturn fmt.Errorf("settle canceled durable uploads: %w", settleErr)
\t\t\t}
\t\t\treturn nil
\t\t}
\t\treturn fmt.Errorf("persist imported upload checkpoint: %w", err)
\t}
\tif err := settleDurableNonreplacementActivations(files); err != nil {
\t\treturn fmt.Errorf("settle imported durable uploads: %w", err)
\t}
\tif err := recovery.setResult(response); err != nil {
\t\tcanceled, stateErr := backgroundUploadOperationCanceled(store, task.OperationID)
\t\tif stateErr != nil {
\t\t\treturn errors.Join(fmt.Errorf("publish upload background result: %w", err), fmt.Errorf("inspect upload cancellation: %w", stateErr))
\t\t}
\t\tif canceled {
\t\t\treturn nil
\t\t}
\t\treturn fmt.Errorf("publish upload background result: %w", err)
\t}
\treturn nil
}''')
replace_func("internal/serve/background_upload_worker.go", "cleanupCanceledClaimedUpload", '''func cleanupCanceledClaimedUpload(files []savedUpload) error {
\tif err := rollbackDurableNonreplacementActivations(files); err != nil {
\t\treturn fmt.Errorf("rollback canceled durable uploads: %w", err)
\t}
\tif err := removeCanceledSavedUploads(files); err != nil {
\t\treturn fmt.Errorf("remove canceled staged uploads: %w", err)
\t}
\treturn nil
}''')

# Durable activation itself remains; only replacement composition is removed.
replace("internal/serve/upload_durable_activation.go", "return !file.replace && file.status != \"error\" && file.status != \"skipped\"", "return file.status != \"error\" && file.status != \"skipped\"")
replace_func("internal/serve/upload_durable_activation.go", "activateSavedDurableUploads", '''func activateSavedDurableUploads(files []savedUpload) error {
\tfor _, file := range files {
\t\tif !durableUploadNeedsActivation(file) {
\t\t\tcontinue
\t\t}
\t\tif err := activateDurableUploadDestination(file.path, file.destinationPath); err != nil {
\t\t\trollbackErr := restoreDurableNonreplacementActivations(files)
\t\t\tif rollbackErr != nil {
\t\t\t\treturn uploadFileError{name: file.name, err: fmt.Errorf("%w; durable activation rollback failed: %v", err, rollbackErr)}
\t\t\t}
\t\t\treturn uploadFileError{name: file.name, err: err}
\t\t}
\t}
\tif err := finalizeDurableNonreplacementActivations(files); err != nil {
\t\trollbackErr := restoreDurableNonreplacementActivations(files)
\t\tif rollbackErr != nil {
\t\t\treturn errors.Join(fmt.Errorf("finalize durable upload activation: %w", err), fmt.Errorf("durable activation rollback failed: %w", rollbackErr))
\t\t}
\t\treturn fmt.Errorf("finalize durable upload activation: %w", err)
\t}
\treturn nil
}''')
remove_decl("internal/serve/upload_durable_activation.go", "func", "wrapOptionalError")
replace("internal/serve/upload_durable_activation.go", "\treturn errors.Join(result, settleDurableReplacementRecoveryMarkers(files))\n", "\treturn result\n")

replace_func("internal/serve/uploads_durable.go", "cleanupCanceledDurableUpload", '''func cleanupCanceledDurableUpload(store durableUploadCleanupStore, operationID string, task core.BackgroundTask) error {
\tfiles, _, err := decodeBackgroundUploadTask(task)
\tif err != nil {
\t\treturn fmt.Errorf("decode canceled background upload task: %w", err)
\t}
\tvar checkpoint backgroundUploadCheckpoint
\tfound, err := store.GetBackgroundOperationCheckpoint(operationID, &checkpoint)
\tif err != nil {
\t\treturn fmt.Errorf("load canceled upload checkpoint: %w", err)
\t}
\tif !found {
\t\treturn errors.New("canceled upload checkpoint is missing")
\t}
\tswitch checkpoint.Phase {
\tcase backgroundUploadPhaseStaged:
\t\treturn removeCanceledSavedUploads(files)
\tcase backgroundUploadPhaseActivated:
\t\treturn cleanupCanceledClaimedUpload(files)
\tcase backgroundUploadPhaseImported:
\t\tif checkpoint.Response == nil {
\t\t\treturn errors.New("canceled imported upload checkpoint is missing response")
\t\t}
\t\tif err := settleDurableNonreplacementActivations(files); err != nil {
\t\t\treturn fmt.Errorf("settle canceled imported durable uploads: %w", err)
\t\t}
\t\treturn nil
\tdefault:
\t\treturn fmt.Errorf("canceled upload checkpoint has invalid phase %q", checkpoint.Phase)
\t}
}''')

delete("internal/serve/upload_replacement_recovery.go")
for path in [
    "internal/serve/upload_replace_replay_test.go",
    "internal/serve/upload_replace_status_test.go",
    "internal/serve/upload_replace_test.go",
    "internal/serve/upload_replacement_recovery_test.go",
    "internal/serve/upload_durable_replacement_rollback_test.go",
    "internal/serve/background_upload_recovery_test.go",
    "internal/serve/background_upload_terminal_marker_cleanup_test.go",
]:
    delete(path)

# Tests: keep path-collision error and hash-dedup/rename coverage; reject removed values.
for name in ["TestUploadConflictPolicyErrorRejectsExistingName", "TestUploadConflictPolicyErrorIsolatesExistingNameWithinBatch", "TestUploadRequestedConflictPolicySkipReturnsSkippedFile", "TestUploadRequestedConflictPolicyReplaceOverwritesExistingName"]:
    remove_decl("internal/serve/uploads_test.go", "func", name)
conflict_test = ROOT / "internal/serve/upload_conflict_policy_test.go"
conflict_test.write_text('''package serve\n\nimport (\n\t"net/http"\n\t"net/http/httptest"\n\t"os"\n\t"path/filepath"\n\t"testing"\n)\n\nfunc TestUploadConflictPolicyErrorRejectsExistingName(t *testing.T) {\n\tdir := t.TempDir()\n\tif err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("existing"), 0600); err != nil {\n\t\tt.Fatalf("write existing file: %v", err)\n\t}\n\tserver := newUploadTestServer(t, dir, true, &recordingUploadLibrary{})\n\trec := httptest.NewRecorder()\n\tserver.Handler().ServeHTTP(rec, uploadRequestWithConflict(t, map[string]string{"a.txt": "uploaded"}, nil, "", "error"))\n\tassertAPIError(t, rec, http.StatusBadRequest, "invalid_request")\n\tif got := string(mustReadFile(t, filepath.Join(dir, "a.txt"))); got != "existing" {\n\t\tt.Fatalf("existing file was overwritten: %q", got)\n\t}\n}\n\nfunc TestUploadRejectsRemovedConflictPolicies(t *testing.T) {\n\tfor _, policy := range []string{"skip", "replace"} {\n\t\tt.Run(policy, func(t *testing.T) {\n\t\t\tserver := newUploadTestServer(t, t.TempDir(), true, &recordingUploadLibrary{})\n\t\t\trec := httptest.NewRecorder()\n\t\t\tserver.Handler().ServeHTTP(rec, uploadRequestWithConflict(t, map[string]string{"a.txt": "uploaded"}, nil, "", policy))\n\t\t\tassertAPIError(t, rec, http.StatusBadRequest, "invalid_request")\n\t\t})\n\t}\n}\n''')

# Durable chooser tests now have no skip/replace result bits.
replace("internal/serve/upload_durable_staging_test.go", 'first, skipped, replace, err := chooseDurableUploadDestination(root, "photo.jpg", "rename", reserved)', 'first, err := chooseDurableUploadDestination(root, "photo.jpg", "rename", reserved)')
replace("internal/serve/upload_durable_staging_test.go", '''\tif err != nil || skipped || replace {\n\t\tt.Fatalf("first destination = %q skip=%v replace=%v err=%v", first, skipped, replace, err)\n\t}\n''', '''\tif err != nil {\n\t\tt.Fatalf("first destination = %q err=%v", first, err)\n\t}\n''')
replace("internal/serve/upload_durable_staging_test.go", 'second, skipped, replace, err := chooseDurableUploadDestination(root, "photo.jpg", "rename", reserved)', 'second, err := chooseDurableUploadDestination(root, "photo.jpg", "rename", reserved)')
replace("internal/serve/upload_durable_staging_test.go", '''\tif err != nil || skipped || replace {\n\t\tt.Fatalf("second destination = %q skip=%v replace=%v err=%v", second, skipped, replace, err)\n\t}\n''', '''\tif err != nil {\n\t\tt.Fatalf("second destination = %q err=%v", second, err)\n\t}\n''')
replace("internal/serve/uploads_segment_reservations_test.go", 'path, skipped, replace, err := chooseDurableUploadDestination(targetDir, "photo.jpg", "rename", reserved)', 'path, err := chooseDurableUploadDestination(targetDir, "photo.jpg", "rename", reserved)')
replace("internal/serve/uploads_segment_reservations_test.go", '''\tif path != want || skipped || replace {\n\t\tt.Fatalf("later destination = (%q, skipped %v, replace %v), want (%q, false, false)", path, skipped, replace, want)\n\t}\n''', '''\tif path != want {\n\t\tt.Fatalf("later destination = %q, want %q", path, want)\n\t}\n''')

# Worker tests lose replacement-only scenarios and the obsolete activated argument.
remove_tests_matching("internal/serve/background_upload_worker_test.go", "Replacement")
replace("internal/serve/background_upload_worker_test.go", "\tactivated int\n", "")
replace("internal/serve/background_upload_worker_test.go", "func (i *recordingUploadImporter) importUploadedFiles(_ context.Context, _ []StagedUpload, _ []string, state backgroundUploadImportState, activated []activatedSavedReplacement) (UploadImportResponse, error)", "func (i *recordingUploadImporter) importUploadedFiles(_ context.Context, _ []StagedUpload, _ []string, state backgroundUploadImportState) (UploadImportResponse, error)")
replace("internal/serve/background_upload_worker_test.go", "\ti.activated = len(activated)\n", "")

# Background task serialization test data no longer has replacement fields.
text = load("internal/serve/background_uploads_test.go")
text = re.sub(r'(?m)^\s*replace:\s*true,\n', '', text)
text = re.sub(r'(?m)^\s*Replace:\s*true,\n', '', text)
text = text.replace('conflictPolicy:  "replace"', 'conflictPolicy:  "rename"')
text = text.replace('ConflictPolicy:  "replace"', 'ConflictPolicy:  "rename"')
save("internal/serve/background_uploads_test.go", text)

# Architecture guard no longer needs the replacement marker exception.
text = load("internal/serve/architecture_test.go")
text = re.sub(r'(?m)^\s*"createReplacementStateMarker": true,\n', '', text)
save("internal/serve/architecture_test.go", text)

# OpenAPI: request may explicitly ask for rename/error; omitted means rename.
replace("docs/openapi.yaml", "enum: [skip, rename, replace]", "enum: [rename, error]")
text = load("docs/openapi.yaml")
text = text.replace("How to handle uploaded filenames that already exist in the target.", "How to handle a requested filename that already exists: rename (default) or error.")
save("docs/openapi.yaml", text)

# Remove config exposure and document API-only strictness.
text = load("docs/CONFIG.md")
text = re.sub(r'(?m)^\| `uploads\.conflict_policy` \|.*\n', '', text)
text = re.sub(r'(?m)^\s*conflict_policy:\s*(?:skip|rename|replace|error)\s*\n', '', text)
text = text.replace("mutation/replacement/protected-storage transitions", "mutation/protected-storage transitions")
text = text.replace("upload into, replace within, or delete from", "upload into or delete from")
save("docs/CONFIG.md", text)
text = load("docs/SERVE.md")
text = re.sub(r'(?m)^\s*conflict_policy:\s*(?:skip|rename|replace|error)\s*\n', '', text)
text = re.sub(r'The default `skip` policy leaves an existing same-name file untouched; API clients may explicitly request `rename`, `replace`, or `error`\.', 'Same-name uploads are renamed by default; API clients may explicitly request `conflict_policy=error` to reject a path collision.', text)
save("docs/SERVE.md", text)
text = load("spec/operation/serve_and_api.md")
text = text.replace("same-name conflicts follow the server-side `uploads.conflict_policy`.", "same-name conflicts are renamed by default, and API callers may request `conflict_policy=error` to reject a path collision.")
save("spec/operation/serve_and_api.md", text)
for path in ["scripts/devprofile/main.go", "scripts/e2e/run_smoke_stage.sh"]:
    text = load(path)
    text = re.sub(r'(?m)^\s*conflict_policy:\s*["\']?(?:skip|rename|replace|error)["\']?\s*\n', '', text)
    save(path, text)

# Final source-level invariants before formatter/compiler checks.
for path in ["internal/serve/config.go", "internal/serve/uploads.go", "internal/serve/upload_stream.go", "internal/serve/upload_durable_staging.go", "internal/serve/background_uploads.go", "internal/serve/background_upload_worker.go", "internal/serve/upload_durable_activation.go", "internal/serve/uploads_durable.go"]:
    text = load(path)
    if 'case "skip"' in text or 'case "replace"' in text:
        raise SystemExit(f"{path}: removed conflict policy branch remains")
if "Uploads.ConflictPolicy" in "\n".join(load(p) for p in ["internal/serve/config.go", "internal/serve/upload_stream.go", "internal/serve/upload_durable_staging.go"]):
    raise SystemExit("configured conflict policy reference remains")
