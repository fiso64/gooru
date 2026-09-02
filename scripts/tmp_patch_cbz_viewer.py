from pathlib import Path

path = Path("frontend/src/lib/components/AuthenticatedApp.svelte")
text = path.read_text()

replacements = [
    (
        "  let jobsDrawerOpen = $state(false);\n",
        "  let jobsDrawerOpen = $state(false);\n  let nestedPreviewNavigation = $state(false);\n",
    ),
    (
        "  $effect(() => {\n    if (uploadJobQuery.isError) upload.applyJobError(uploadJobQuery.error);\n  });\n",
        "  $effect(() => {\n    if (uploadJobQuery.isError) upload.applyJobError(uploadJobQuery.error);\n  });\n\n  $effect(() => {\n    if (!library.activeFile) nestedPreviewNavigation = false;\n  });\n",
    ),
    (
        "\n    library.handleKeydown(event, loadedFiles);\n",
        "\n    if (nestedPreviewNavigation && library.activeFile && (event.key === 'ArrowLeft' || event.key === 'ArrowRight' || event.key === 'j' || event.key === 'k')) return;\n    library.handleKeydown(event, loadedFiles);\n",
    ),
    (
        "      onNext={() => library.movePreview(1, files)}\n",
        "      onNext={() => library.movePreview(1, files)}\n      onNestedNavigationChange={(active) => (nestedPreviewNavigation = active)}\n",
    ),
]

for old, new in replacements:
    if old not in text:
        raise SystemExit(f"missing patch anchor: {old!r}")
    text = text.replace(old, new, 1)

path.write_text(text)
