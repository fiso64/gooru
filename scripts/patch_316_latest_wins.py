from pathlib import Path


def replace_once(path: str, old: str, new: str) -> None:
    file = Path(path)
    text = file.read_text()
    if old not in text:
        raise SystemExit(f"missing patch anchor in {path}: {old[:80]!r}")
    file.write_text(text.replace(old, new, 1))


# Library navigation owns cancellation: every logical step invalidates speculative work
# before the foreground file changes. PreviewDialog will re-prime only after presentation.
replace_once(
    "frontend/src/lib/state/libraryWorkflow.svelte.ts",
    "import { clearViewerPreloadCache, preloadViewerMedia } from '$lib/utils/viewerPreload';",
    "import { clearViewerPreloadCache } from '$lib/utils/viewerPreload';",
)
replace_once(
    "frontend/src/lib/state/libraryWorkflow.svelte.ts",
    '''  function primePreviewNeighbor(file: FileItem, files: FileItem[]) {\n    const next = previewNeighbor(file, files, 1);\n    if (next) void preloadViewerMedia(next).catch(() => undefined);\n  }\n\n  function openPreview(file: FileItem, files: FileItem[] = []) {\n    activeFile = file;\n    pendingPreviewID = file.id;\n    route = 'library';\n    primePreviewNeighbor(file, files);\n  }\n''',
    '''  function openPreview(file: FileItem, files: FileItem[] = []) {\n    void files;\n    clearViewerPreloadCache();\n    activeFile = file;\n    pendingPreviewID = file.id;\n    route = 'library';\n  }\n''',
)
replace_once(
    "frontend/src/lib/state/libraryWorkflow.svelte.ts",
    '''  function movePreview(delta: number, files: FileItem[]) {\n    const next = previewNeighbor(activeFile, files, delta);\n    if (!next) return;\n    void preloadViewerMedia(next).catch(() => undefined);\n    activeFile = next;\n    pendingPreviewID = next.id;\n    primePreviewNeighbor(next, files);\n  }\n''',
    '''  function movePreview(delta: number, files: FileItem[]) {\n    const next = previewNeighbor(activeFile, files, delta);\n    if (!next) return;\n    // Rapid navigation is latest-wins: stale speculative decodes must not remain queued\n    // ahead of the browser's foreground request for the newly requested file.\n    clearViewerPreloadCache();\n    activeFile = next;\n    pendingPreviewID = next.id;\n  }\n''',
)

# Parent supplies the immediate neighbors; presentation-mode-aware preloading lives in the dialog.
replace_once(
    "frontend/src/lib/components/AuthenticatedApp.svelte",
    "  import { appendSidebarKind, queryWithoutSidebarKind } from '$lib/utils/sidebarKinds';",
    "  import { appendSidebarKind, queryWithoutSidebarKind } from '$lib/utils/sidebarKinds';\n  import { previewNeighbor } from '$lib/utils/viewerNavigation';",
)
replace_once(
    "frontend/src/lib/components/AuthenticatedApp.svelte",
    '''      file={library.activeFile}\n      tagDraft={tagWorkflow.drafts[library.activeFile.id] ?? ''}\n''',
    '''      file={library.activeFile}\n      preloadPrev={previewNeighbor(library.activeFile, files, -1)}\n      preloadNext={previewNeighbor(library.activeFile, files, 1)}\n      tagDraft={tagWorkflow.drafts[library.activeFile.id] ?? ''}\n''',
)

preview = Path("frontend/src/lib/components/PreviewDialog.svelte")
text = preview.read_text()
text = text.replace(
    "  import { preloadViewerMediaSource } from '$lib/utils/viewerPreload';",
    "  import { clearViewerPreloadCache, preloadViewerMediaSource } from '$lib/utils/viewerPreload';",
    1,
)
text = text.replace(
    '''    file,\n    tagDraft,\n''',
    '''    file,\n    preloadPrev,\n    preloadNext,\n    tagDraft,\n''',
    1,
)
text = text.replace(
    '''    file: FileItem;\n    tagDraft: string;\n''',
    '''    file: FileItem;\n    preloadPrev?: FileItem;\n    preloadNext?: FileItem;\n    tagDraft: string;\n''',
    1,
)
text = text.replace(
    "  let comicController: AbortController | undefined;\n",
    "  let comicController: AbortController | undefined;\n  let navigationDirection: -1 | 1 = 1;\n",
    1,
)
old_effect = '''  $effect(() => {\n    if (!comicEntered || !comicManifest) return;\n    const targetFile = file;\n    for (const page of adjacentComicPages(comicManifest, comicPageIndex)) {\n      void preloadViewerMediaSource(targetFile, page.url).catch(() => undefined);\n    }\n  });\n'''
new_effect = '''  $effect(() => {\n    // Any requested source change invalidates speculative work immediately. The next neighbor\n    // is not primed until ViewerStage confirms that this exact target has been presented.\n    file.id;\n    imageSource;\n    comicEntered;\n    comicPageIndex;\n    clearViewerPreloadCache();\n  });\n\n  function primeAfterPresentation(source: string) {\n    if (source !== imageSource) return;\n    clearViewerPreloadCache();\n\n    if (comicEntered && comicManifest) {\n      const targetIndex = comicPageIndex + navigationDirection;\n      const page = comicPageAt(comicManifest, targetIndex);\n      if (page) void preloadViewerMediaSource(file, page.url).catch(() => undefined);\n      return;\n    }\n\n    const neighbor = navigationDirection < 0 ? preloadPrev : preloadNext;\n    if (!neighbor) return;\n    void preloadViewerMediaSource(neighbor, viewerImageSource(neighbor, preferOriginal)).catch(() => undefined);\n  }\n'''
if old_effect not in text:
    raise SystemExit("missing PreviewDialog preload effect")
text = text.replace(old_effect, new_effect, 1)
text = text.replace(
    '''  function movePage(delta: number) {\n    const next = moveComicPage(comicPageIndex, delta, comicManifest?.pages.length ?? 0);\n    if (next !== comicPageIndex) comicPageIndex = next;\n  }\n\n  function stagePrev() {\n    if (comicEntered) movePage(-1);\n    else onPrev();\n  }\n\n  function stageNext() {\n    if (comicEntered) movePage(1);\n    else onNext();\n  }\n''',
    '''  function movePage(delta: number) {\n    const next = moveComicPage(comicPageIndex, delta, comicManifest?.pages.length ?? 0);\n    if (next !== comicPageIndex) {\n      navigationDirection = delta < 0 ? -1 : 1;\n      comicPageIndex = next;\n    }\n  }\n\n  function stagePrev() {\n    navigationDirection = -1;\n    if (comicEntered) movePage(-1);\n    else onPrev();\n  }\n\n  function stageNext() {\n    navigationDirection = 1;\n    if (comicEntered) movePage(1);\n    else onNext();\n  }\n''',
    1,
)
text = text.replace(
    '''    onComicPageSelect={(index) => { comicPageIndex = index; }}\n  />\n''',
    '''    onComicPageSelect={(index) => {\n      navigationDirection = index < comicPageIndex ? -1 : 1;\n      comicPageIndex = index;\n    }}\n    onPresented={primeAfterPresentation}\n  />\n''',
    1,
)
# adjacentComicPages is no longer needed after moving speculative work to the presentation callback.
text = text.replace(
    "  import { adjacentComicPages, comicPageAt, isComicFile, moveComicPage } from '$lib/utils/comic';",
    "  import { comicPageAt, isComicFile, moveComicPage } from '$lib/utils/comic';",
    1,
)
preview.write_text(text)

viewer = Path("frontend/src/lib/components/ViewerStage.svelte")
text = viewer.read_text()
text = text.replace(
    '''    onComicPageSelect\n  } = $props<{\n''',
    '''    onComicPageSelect,\n    onPresented\n  } = $props<{\n''',
    1,
)
text = text.replace(
    '''    onComicPageSelect?: (index: number) => void;\n  }>();\n''',
    '''    onComicPageSelect?: (index: number) => void;\n    onPresented?: (source: string) => void;\n  }>();\n''',
    1,
)
text = text.replace(
    "  let waitingForTarget = $state(false);\n  let transitionGeneration = 0;\n",
    "  let waitingForTarget = $state(false);\n  let waitingTimer: ReturnType<typeof setTimeout> | undefined;\n  let transitionGeneration = 0;\n",
    1,
)
text = text.replace(
    '''      if (fitModeFeedbackTimer) clearTimeout(fitModeFeedbackTimer);\n    };\n  });\n''',
    '''      if (fitModeFeedbackTimer) clearTimeout(fitModeFeedbackTimer);\n      if (waitingTimer) clearTimeout(waitingTimer);\n    };\n  });\n''',
    1,
)
# Replace the transition effect wholesale.
start = text.index("  $effect(() => {\n    const targetFile = file;\n    const targetImageSource = imageSource;")
end = text.index("\n  $effect(() => {\n    const nextFile = renderedFile;", start)
new_transition = '''  function clearWaitingTimer() {\n    if (waitingTimer) clearTimeout(waitingTimer);\n    waitingTimer = undefined;\n  }\n\n  function armWaitingTimer(generation: number) {\n    clearWaitingTimer();\n    waitingTimer = setTimeout(() => {\n      if (generation === transitionGeneration) waitingForTarget = true;\n    }, 200);\n  }\n\n  $effect(() => {\n    const targetFile = file;\n    const targetImageSource = imageSource;\n    const generation = ++transitionGeneration;\n    clearWaitingTimer();\n    waitingForTarget = false;\n\n    if (!displayedFile) {\n      displayedFile = targetFile;\n      displayedImageSource = targetImageSource;\n      armWaitingTimer(generation);\n      return () => { if (generation === transitionGeneration) clearWaitingTimer(); };\n    }\n    if (displayedFile.id === targetFile.id && displayedImageSource === targetImageSource) return;\n\n    const rendersImage = targetFile.media_kind !== 'video' && targetFile.media_kind !== 'audio' && !targetFile.media_type.startsWith('audio/');\n    if (rendersImage) {\n      // Once rapid navigation has frozen a committed frame, keep that exact snapshot until the\n      // latest requested target is presentable. Re-freezing from an in-flight <img> can capture\n      // obsolete pixels using newer geometry and reintroduce the #170 stretch/overlap artifact.\n      if (!freezeVisible) freezePresentedImage();\n      const metadataWidth = targetFile.metadata?.image_width ?? 0;\n      const metadataHeight = targetFile.metadata?.image_height ?? 0;\n      if (metadataWidth > 0 && metadataHeight > 0) {\n        intrinsicWidth = metadataWidth;\n        intrinsicHeight = metadataHeight;\n      }\n      displayedFile = targetFile;\n      displayedImageSource = targetImageSource;\n      armWaitingTimer(generation);\n      return () => { if (generation === transitionGeneration) clearWaitingTimer(); };\n    }\n\n    armWaitingTimer(generation);\n    const preloadSource = viewerPreloadSource(targetFile);\n\n    void preloadViewerMediaSource(targetFile, preloadSource)\n      .catch(() => undefined)\n      .then(() => {\n        if (generation !== transitionGeneration) return;\n        clearWaitingTimer();\n        displayedFile = targetFile;\n        displayedImageSource = targetImageSource;\n        waitingForTarget = false;\n      });\n\n    return () => {\n      if (generation === transitionGeneration) {\n        clearWaitingTimer();\n        transitionGeneration += 1;\n      }\n    };\n  });\n'''
text = text[:start] + new_transition + text[end:]
# Replace image synchronization with source/generation guards.
old_sync = '''  function syncImage(event: Event) {\n    const image = event.currentTarget;\n    if (!(image instanceof HTMLImageElement)) return;\n    intrinsicWidth = image.naturalWidth;\n    intrinsicHeight = image.naturalHeight;\n    // Keep the frozen old pixels until the next paint after target load. The target is already\n    // laid out at final geometry underneath, so changing both visibility states in the same\n    // animation-frame callback presents only one image while avoiding an extra frame of latency.\n    const generation = freezeGeneration;\n    requestAnimationFrame(() => {\n      if (generation === freezeGeneration) freezeVisible = false;\n    });\n  }\n\n  function syncImageError() {\n    // A failed/unsupported target has no paint event that can release the frozen frame.\n    // End this handoff explicitly so stale pixels never stand in for the current file.\n    freezeGeneration += 1;\n    freezeVisible = false;\n    intrinsicWidth = 0;\n    intrinsicHeight = 0;\n  }\n'''
new_sync = '''  function imageMatchesCurrentSource(image: HTMLImageElement) {\n    if (!renderedImageSource) return false;\n    try {\n      return image.currentSrc === new URL(renderedImageSource, document.baseURI).href;\n    } catch {\n      return image.currentSrc === renderedImageSource;\n    }\n  }\n\n  function syncImage(event: Event) {\n    const image = event.currentTarget;\n    if (!(image instanceof HTMLImageElement) || !imageMatchesCurrentSource(image)) return;\n    const generation = transitionGeneration;\n    const source = renderedImageSource;\n    intrinsicWidth = image.naturalWidth;\n    intrinsicHeight = image.naturalHeight;\n    clearWaitingTimer();\n    waitingForTarget = false;\n    // Keep the frozen old pixels until the next paint after the *latest* target load. Stale\n    // completions from sources superseded by rapid navigation must never release the freeze.\n    requestAnimationFrame(() => {\n      if (generation !== transitionGeneration || !imageMatchesCurrentSource(image)) return;\n      freezeGeneration += 1;\n      freezeVisible = false;\n      onPresented?.(source);\n    });\n  }\n\n  function syncImageError(event: Event) {\n    const image = event.currentTarget;\n    if (!(image instanceof HTMLImageElement) || !imageMatchesCurrentSource(image)) return;\n    // A failed/unsupported latest target has no paint event that can release the frozen frame.\n    // End this handoff explicitly so stale pixels never stand in for the current file.\n    clearWaitingTimer();\n    waitingForTarget = false;\n    freezeGeneration += 1;\n    freezeVisible = false;\n    intrinsicWidth = 0;\n    intrinsicHeight = 0;\n  }\n'''
if old_sync not in text:
    raise SystemExit("missing ViewerStage sync anchor")
text = text.replace(old_sync, new_sync, 1)
# Add a visible delayed-loading signal; the committed frame remains dimmed by the existing waiting class.
text = text.replace(
    '''  {#if fitModeFeedback}<div class="viewer-mode-feedback" role="status" aria-live="polite">{fitModeFeedback}</div>{/if}\n''',
    '''  {#if waitingForTarget}<div class="viewer-loading-indicator" role="status" aria-live="polite">Loading latest…</div>{/if}\n  {#if fitModeFeedback}<div class="viewer-mode-feedback" role="status" aria-live="polite">{fitModeFeedback}</div>{/if}\n''',
    1,
)
text = text.replace(
    '''  .viewer-mode-feedback {\n''',
    '''  .viewer-loading-indicator {\n    position: absolute;\n    z-index: 5;\n    left: 50%;\n    top: 50%;\n    transform: translate(-50%, -50%);\n    padding: 7px 10px;\n    border-radius: 5px;\n    background: rgba(0, 0, 0, 0.76);\n    color: #fff;\n    font: 600 11px/1.2 var(--font-mono);\n    pointer-events: none;\n  }\n\n  .viewer-mode-feedback {\n''',
    1,
)
viewer.write_text(text)
