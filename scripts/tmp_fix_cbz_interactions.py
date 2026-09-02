from pathlib import Path

preview = Path("frontend/src/lib/components/PreviewDialog.svelte")
text = preview.read_text()
text = text.replace("  import { onMount } from 'svelte';\n", "  import { onMount, untrack } from 'svelte';\n", 1)
old = '''  $effect(() => {
    file.id;
    comicController?.abort();
    comicController = undefined;
    comicManifest = null;
    comicPageIndex = 0;
    comicEntered = false;
    comicLoading = false;
    comicError = '';
    onNestedNavigationChange(false);
  });
'''
new = '''  $effect(() => {
    file.id;
    untrack(() => {
      comicController?.abort();
      comicController = undefined;
      comicManifest = null;
      comicPageIndex = 0;
      comicEntered = false;
      comicLoading = false;
      comicError = '';
      onNestedNavigationChange(false);
    });
  });
'''
if old not in text:
    raise SystemExit("PreviewDialog reset effect anchor missing")
preview.write_text(text.replace(old, new, 1))

stage = Path("frontend/src/lib/components/ViewerStage.svelte")
text = stage.read_text()
old = '''    if (onPrimaryAction && (event.code === 'Space' || event.key === 'Enter')) {
      event.preventDefault();
'''
new = '''    if (onPrimaryAction && (event.code === 'Space' || event.key === 'Enter')) {
      if (isInteractiveShortcutTarget(target)) return;
      event.preventDefault();
'''
if old not in text:
    raise SystemExit("ViewerStage primary action anchor missing")
stage.write_text(text.replace(old, new, 1))
