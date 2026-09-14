from pathlib import Path


def replace(path: str, old: str, new: str, count: int = 1) -> None:
    p = Path(path)
    text = p.read_text()
    actual = text.count(old)
    if actual < count:
        raise SystemExit(f"{path}: expected at least {count} copies, found {actual}: {old[:120]!r}")
    p.write_text(text.replace(old, new, count))


# Durable batch format. Keep v2 decoding for already queued work and use v3 only
# for the new per-file mixed delete/untrack semantics.
path = "internal/serve/background_file_removals.go"
replace(
    path,
    'backgroundFileRemovalBatchVersion  = 2',
    'backgroundFileRemovalBatchVersion      = 2\n\tbackgroundFileRemovalMixedBatchVersion = 3',
)
replace(
    path,
    'input := backgroundFileRemovalBatchInput{\n\t\tVersion: backgroundFileRemovalBatchVersion,\n\t\tMode:    mode,',
    'version := backgroundFileRemovalBatchVersion\n\tif mode == "delete_or_untrack" {\n\t\tversion = backgroundFileRemovalMixedBatchVersion\n\t}\n\tinput := backgroundFileRemovalBatchInput{\n\t\tVersion: version,\n\t\tMode:    mode,',
)
replace(
    path,
    'stagingToken := ""\n\tif mode == "delete" {',
    'stagingToken := ""\n\tphysicalDelete := mode == "delete" || mode == "delete_or_untrack"\n\tif physicalDelete {',
)
replace(
    path,
    '''\t\tif mode == "delete" {\n\t\t\tmanagedPath, ok := s.managedDeleteCandidatePath(fileStoragePath(file))\n\t\t\tif !ok {\n\t\t\t\treturn core.BackgroundTaskRequest{}, ErrFileNotManaged\n\t\t\t}\n\t\t\titem.OriginalPath = managedPath\n\t\t\titem.StagingPath = filepath.Join(\n\t\t\t\tfilepath.Dir(managedPath),\n\t\t\t\tfmt.Sprintf(".gooru-delete-%s-%06d", stagingToken, index),\n\t\t\t)\n\t\t}''',
    '''\t\tif physicalDelete {\n\t\t\tmanagedPath, ok := s.managedDeleteCandidatePath(fileStoragePath(file))\n\t\t\tif !ok && mode == "delete" {\n\t\t\t\treturn core.BackgroundTaskRequest{}, ErrFileNotManaged\n\t\t\t}\n\t\t\tif ok {\n\t\t\t\titem.OriginalPath = managedPath\n\t\t\t\titem.StagingPath = filepath.Join(\n\t\t\t\t\tfilepath.Dir(managedPath),\n\t\t\t\t\tfmt.Sprintf(".gooru-delete-%s-%06d", stagingToken, index),\n\t\t\t\t)\n\t\t\t}\n\t\t}''',
)
replace(
    path,
    'if mode != "delete" && mode != "untrack" {',
    'if mode != "delete" && mode != "untrack" && mode != "delete_or_untrack" {',
)
replace(
    path,
    'if envelope.Version == backgroundFileRemovalBatchVersion {',
    'if envelope.Version == backgroundFileRemovalBatchVersion || envelope.Version == backgroundFileRemovalMixedBatchVersion {',
)
replace(
    path,
    '''\tcase "delete":\n\t\treturn s.resumeManagedFileDeletionBatch(ctx, input.Files, publicIDs)\n\tdefault:''',
    '''\tcase "delete":\n\t\treturn s.resumeManagedFileDeletionBatch(ctx, input.Files, publicIDs)\n\tcase "delete_or_untrack":\n\t\tif input.Version != backgroundFileRemovalMixedBatchVersion {\n\t\t\treturn errors.New("mixed file removal batch has invalid version")\n\t\t}\n\t\tmanagedFiles := make([]backgroundFileRemovalBatchFile, 0, len(input.Files))\n\t\tfor _, file := range input.Files {\n\t\t\tif file.OriginalPath == "" && file.StagingPath == "" {\n\t\t\t\tcontinue\n\t\t\t}\n\t\t\tif file.OriginalPath == "" || file.StagingPath == "" {\n\t\t\t\treturn errors.New("mixed file removal batch has incomplete managed deletion paths")\n\t\t\t}\n\t\t\tmanagedFiles = append(managedFiles, file)\n\t\t}\n\t\treturn s.resumeManagedFileDeletionBatch(ctx, managedFiles, publicIDs)\n\tdefault:''',
)

# Admission/API. Strict delete still rejects mixed selections before durable work;
# only the explicit second-confirmation mode permits untracking external paths.
path = "internal/serve/file_removals.go"
replace(
    path,
    'if request.Mode == "delete" {\n\t\ttask, err = backgroundFileRemovalApplyDeleteRetryPolicy(task)',
    'if request.Mode == "delete" || request.Mode == "delete_or_untrack" {\n\t\ttask, err = backgroundFileRemovalApplyDeleteRetryPolicy(task)',
)
replace(
    path,
    '''\toperation, _, err := removalLibrary.CreateBackgroundOperationWithTasks(core.BackgroundOperationRequest{\n\t\tKind:          "files." + request.Mode,''',
    '''\toperationKind := "files." + request.Mode\n\tif request.Mode == "delete_or_untrack" {\n\t\toperationKind = "files.delete"\n\t}\n\toperation, _, err := removalLibrary.CreateBackgroundOperationWithTasks(core.BackgroundOperationRequest{\n\t\tKind:          operationKind,''',
)
replace(
    path,
    '''\tif request.Mode != "untrack" && request.Mode != "delete" {\n\t\treturn errors.New("mode must be untrack or delete")\n\t}''',
    '''\tif request.Mode != "untrack" && request.Mode != "delete" && request.Mode != "delete_or_untrack" {\n\t\treturn errors.New("mode must be untrack, delete, or delete_or_untrack")\n\t}''',
)

# OpenAPI contract: the mixed mode exists only on the bulk selector endpoint.
path = "docs/openapi.yaml"
replace(
    path,
    'description: Accepts the same explicit-ID or query-with-exclusions selector used by bulk UI actions. Physical deletion is accepted only when every selected file is inside configured upload targets.',
    'description: Accepts the same explicit-ID or query-with-exclusions selector used by bulk UI actions. Strict physical deletion is accepted only when every selected file is inside configured upload targets. After explicit confirmation, delete_or_untrack deletes managed files while only untracking files outside configured upload targets.',
)
p = Path(path)
text = p.read_text()
marker = "    FileRemovalRequest:"
before, sep, after = text.partition(marker)
if not sep:
    raise SystemExit("FileRemovalRequest schema marker not found")
old_enum = "enum: [untrack, delete]"
if old_enum not in after:
    raise SystemExit("FileRemovalRequest mode enum not found")
after = after.replace(old_enum, "enum: [untrack, delete, delete_or_untrack]", 1)
p.write_text(before + sep + after)

# UI: a strict-delete 409 becomes a second confirmation. The confirmation sends
# one mixed durable request rather than two independently durable mutations.
path = "frontend/src/lib/components/AuthenticatedApp.svelte"
replace(
    path,
    "'bulk-untrack-selected' | 'bulk-delete-selected' | 'untrack-file' | 'delete-file';",
    "'bulk-untrack-selected' | 'bulk-delete-selected' | 'bulk-delete-or-untrack-selected' | 'untrack-file' | 'delete-file';",
)
replace(
    path,
    '''      } else if (actionDialog.kind === 'bulk-untrack-selected' || actionDialog.kind === 'bulk-delete-selected') {\n        const selector = await ensureSelectionReady();\n        await filesRemovalMutation.mutateAsync({\n          ...selector,\n          mode: actionDialog.kind === 'bulk-delete-selected' ? 'delete' : 'untrack'\n        });\n        library.clearSelection();''',
    '''      } else if (actionDialog.kind === 'bulk-untrack-selected' || actionDialog.kind === 'bulk-delete-selected' || actionDialog.kind === 'bulk-delete-or-untrack-selected') {\n        const selector = await ensureSelectionReady();\n        await filesRemovalMutation.mutateAsync({\n          ...selector,\n          mode: actionDialog.kind === 'bulk-delete-selected'\n            ? 'delete'\n            : actionDialog.kind === 'bulk-delete-or-untrack-selected'\n              ? 'delete_or_untrack'\n              : 'untrack'\n        });\n        library.clearSelection();''',
)
replace(
    path,
    '''    } catch (error) {\n      actionDialog = { ...actionDialog, busy: false, error: errorMessage(error) };\n    }''',
    '''    } catch (error) {\n      if (error instanceof ApiError && error.code === 'file_not_managed' && actionDialog.kind === 'bulk-delete-selected') {\n        actionDialog = { ...actionDialog, kind: 'bulk-delete-or-untrack-selected', busy: false, error: '' };\n        return;\n      }\n      actionDialog = { ...actionDialog, busy: false, error: errorMessage(error) };\n    }''',
)
replace(
    path,
    "      case 'bulk-delete-selected': return 'Delete selected files';",
    "      case 'bulk-delete-selected': return 'Delete selected files';\n      case 'bulk-delete-or-untrack-selected': return 'Delete managed files and untrack external files?';",
)
replace(
    path,
    "      case 'bulk-delete-selected': return `Permanently delete ${selectedCount} selected file${selectedCount === 1 ? '' : 's'} from disk and remove them from the library. Only files in managed upload targets can be deleted.`;",
    "      case 'bulk-delete-selected': return `Permanently delete ${selectedCount} selected file${selectedCount === 1 ? '' : 's'} from disk and remove them from the library. Only files in managed upload targets can be deleted.`;\n      case 'bulk-delete-or-untrack-selected': return 'Some selected files are outside configured upload targets. Delete managed files and untrack the external files? External files will remain on disk.';",
)
replace(
    path,
    "      case 'bulk-delete-selected': return 'Delete files';",
    "      case 'bulk-delete-selected': return 'Delete files';\n      case 'bulk-delete-or-untrack-selected': return 'Delete and untrack';",
)
replace(
    path,
    "destructive={actionDialog.kind === 'save-delete' || actionDialog.kind === 'bulk-untrack-selected' || actionDialog.kind === 'bulk-delete-selected' || actionDialog.kind === 'untrack-file' || actionDialog.kind === 'delete-file'}",
    "destructive={actionDialog.kind === 'save-delete' || actionDialog.kind === 'bulk-untrack-selected' || actionDialog.kind === 'bulk-delete-selected' || actionDialog.kind === 'bulk-delete-or-untrack-selected' || actionDialog.kind === 'untrack-file' || actionDialog.kind === 'delete-file'}",
)
replace(
    path,
    "input={actionDialog.kind !== 'save-delete' && actionDialog.kind !== 'bulk-untrack-selected' && actionDialog.kind !== 'bulk-delete-selected' && actionDialog.kind !== 'untrack-file' && actionDialog.kind !== 'delete-file'}",
    "input={actionDialog.kind !== 'save-delete' && actionDialog.kind !== 'bulk-untrack-selected' && actionDialog.kind !== 'bulk-delete-selected' && actionDialog.kind !== 'bulk-delete-or-untrack-selected' && actionDialog.kind !== 'untrack-file' && actionDialog.kind !== 'delete-file'}",
)

# Backend regression for classification and durable payload versioning.
p = Path("internal/serve/file_removals_managed_storage_test.go")
text = p.read_text()
if "TestMixedRemovalTaskDeletesManagedAndOnlyUntracksExternal" not in text:
    text = text.rstrip() + r'''

func TestMixedRemovalTaskDeletesManagedAndOnlyUntracksExternal(t *testing.T) {
	managedRoot := t.TempDir()
	managedPath := filepath.Join(managedRoot, "managed.jpg")
	externalPath := filepath.Join(t.TempDir(), "external.jpg")
	for _, path := range []string{managedPath, externalPath} {
		if err := os.WriteFile(path, []byte("data"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	cfg := DefaultConfig(filepath.Join(t.TempDir(), "gooru.db"))
	cfg.Uploads.Targets = []UploadTarget{{ID: "managed", Name: "Managed", Path: managedRoot}}
	server := NewServerWithLibrary(cfg, emptyLibrary{})

	task, err := server.backgroundFileRemovalBatchTask("delete_or_untrack", []types.FileInfo{
		{PublicID: "managed", Path: managedPath},
		{PublicID: "external", Path: externalPath},
	})
	if err != nil {
		t.Fatal(err)
	}
	var input backgroundFileRemovalBatchInput
	if err := json.Unmarshal([]byte(task.InputKey), &input); err != nil {
		t.Fatal(err)
	}
	if input.Version != backgroundFileRemovalMixedBatchVersion || input.Mode != "delete_or_untrack" || len(input.Files) != 2 {
		t.Fatalf("unexpected mixed removal payload: %+v", input)
	}
	if input.Files[0].OriginalPath != managedPath || input.Files[0].StagingPath == "" {
		t.Fatalf("managed file was not staged for deletion: %+v", input.Files[0])
	}
	if input.Files[1].OriginalPath != "" || input.Files[1].StagingPath != "" {
		t.Fatalf("external file must only be untracked: %+v", input.Files[1])
	}
}
''' + "\n"
    p.write_text(text)

# Browser regression for the exact two-step confirmation and single confirmed mutation.
p = Path("frontend/tests/selection-ui-modal.spec.ts")
text = p.read_text()
if "mixed delete asks to untrack external files" not in text:
    text = text.rstrip() + r'''


test('mixed delete asks to untrack external files before one confirmed removal request', async ({ page }) => {
  await mockApp(page);
  const removals: Array<Record<string, unknown>> = [];
  await page.route('**/api/v1/files', async (route) => {
    if (route.request().method() !== 'DELETE') return route.fallback();
    const removal = route.request().postDataJSON() as Record<string, unknown>;
    removals.push(removal);
    if (removal.mode === 'delete') {
      await route.fulfill({
        status: 409,
        contentType: 'application/json',
        body: JSON.stringify({ error: { code: 'file_not_managed', message: 'one or more selected files are outside configured upload targets; untrack them instead' } })
      });
      return;
    }
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({ mode: removal.mode, removed_locations: 2, selector: {} })
    });
  });

  await selectFirstFile(page);
  await page.keyboard.press('Shift+Delete');
  let dialog = page.getByRole('dialog', { name: 'Delete selected files' });
  await dialog.getByRole('button', { name: 'Delete files' }).click();

  dialog = page.getByRole('dialog', { name: 'Delete managed files and untrack external files?' });
  await expect(dialog).toBeVisible();
  await expect(dialog).toContainText('External files will remain on disk.');
  expect(removals).toEqual([{ mode: 'delete', selection_id: 'selection-one', exclude_file_ids: ['one'] }]);

  await dialog.getByRole('button', { name: 'Delete and untrack' }).click();
  await expect.poll(() => removals.length).toBe(2);
  expect(removals[1]).toEqual({ mode: 'delete_or_untrack', selection_id: 'selection-one', exclude_file_ids: ['one'] });
  await expect(dialog).toHaveCount(0);
});
''' + "\n"
    p.write_text(text)
