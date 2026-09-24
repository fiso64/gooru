<script lang="ts">
  import { onMount, untrack } from 'svelte';
  import Icon from './Icon.svelte';
  import { readViewerSessionPreferences, updateViewerSessionPreferences, type ViewerRotation, type ViewerScaling } from '$lib/state/viewerSessionPreferences';
  import { mediaDuration } from '$lib/utils/format';
  import { hasCommandModifier, isEditableShortcutTarget, isInteractiveShortcutTarget } from '$lib/utils/keyboard';
  import { hasBlockingModal } from '$lib/utils/modal';
  import { isEmptyViewerTagShortcut } from '$lib/utils/viewerTagKeyRouting';
  import { preserveNativeViewerSize, type ViewerStageMedia } from '$lib/utils/media';
  import { recordViewerPresentation, recordViewerRequest } from '$lib/utils/viewerPerformance';
  import { normalizeViewerRotation, rotateViewer, viewerGeometry, viewerMediaStyle, type ViewerConfiguredFitMode, type ViewerFitMode } from '$lib/utils/viewer';

  let {
    file,
    imageSource,
    initialFitMode = 'fit_window',
    boundActualSizeToFit = true,
    initialScaling = 'smooth',
    onPrev,
    onNext,
    onPrimaryAction,
    keyboardNavigation = false,
    navigationUnit = 'file',
    closeOnFullscreenExit = false,
    onFullscreenExit,
    comicAvailable = false,
    comicEntered = false,
    comicLoading = false,
    comicPage = 0,
    comicPages = 0,
    comicError = '',
    navigationError = '',
    onToggleComic,
    onComicPageSelect,
    onPresented
  } = $props<{
    file: ViewerStageMedia;
    imageSource: string;
    initialFitMode?: ViewerConfiguredFitMode;
    boundActualSizeToFit?: boolean;
    initialScaling?: ViewerScaling;
    onPrev: () => void;
    onNext: () => void;
    onPrimaryAction?: () => void;
    keyboardNavigation?: boolean;
    navigationUnit?: string;
    closeOnFullscreenExit?: boolean;
    onFullscreenExit?: () => void;
    comicAvailable?: boolean;
    comicEntered?: boolean;
    comicLoading?: boolean;
    comicPage?: number;
    comicPages?: number;
    comicError?: string;
    navigationError?: string;
    onToggleComic?: () => void;
    onComicPageSelect?: (index: number) => void;
    onPresented?: (source: string) => void;
  }>();

  const initialViewerPreferences = readViewerSessionPreferences({
    preferOriginal: false,
    rotation: 0,
    fitMode: untrack(() => initialFitMode),
    scaling: untrack(() => initialScaling)
  });

  let stageElement = $state<HTMLDivElement | undefined>();
  let comicTransition = $state<'' | 'entering' | 'exiting'>('');
  let previousComicEntered = untrack(() => comicEntered);
  let panViewportElement = $state<HTMLDivElement | undefined>();
  let imageElement = $state<HTMLImageElement | undefined>();
  let freezeCanvasElement = $state<HTMLCanvasElement | undefined>();
  let freezeVisible = $state(false);
  let frozenStyle = $state('');
  let freezeGeneration = 0;
  let videoElement = $state<HTMLVideoElement | undefined>();
  let audioElement = $state<HTMLAudioElement | undefined>();
  let displayedFile = $state<ViewerStageMedia | undefined>();
  let displayedImageSource = $state('');
  let waitingForTarget = $state(false);
  let mediaError = $state('');
  let waitingTimer: ReturnType<typeof setTimeout> | undefined;
  let transitionGeneration = 0;
  let rotation = $state<number>(initialViewerPreferences.rotation);
  let fitMode = $state<ViewerFitMode>(initialViewerPreferences.fitMode);
  let scaling = $state<ViewerScaling>(initialViewerPreferences.scaling);
  let fitModeFeedback = $state('');
  let fitModeFeedbackTimer: ReturnType<typeof setTimeout> | undefined;
  let isFullscreen = $state(false);
  let keepViewerAfterFullscreenExit = false;
  let stageWidth = $state(0);
  let stageHeight = $state(0);
  let intrinsicWidth = $state(0);
  let intrinsicHeight = $state(0);
  let videoPaused = $state(true);
  let videoTime = $state(0);
  let videoLength = $state(0);
  let playbackControlsIdle = $state(false);
  let playbackControlsTimer: ReturnType<typeof setTimeout> | undefined;
  let videoSeekPointerId: number | undefined;
  let videoSeekPlaybackState: { video: HTMLVideoElement; paused: boolean; muted: boolean } | undefined;
  let cursorIdle = $state(false);
  let cursorIdleTimer: ReturnType<typeof setTimeout> | undefined;
  let zoom = $state(1);
  let panX = $state(0);
  let panY = $state(0);
  let panDragPointerId: number | undefined;
  let panDragStartClientX = 0;
  let panDragStartClientY = 0;
  let panDragStartX = 0;
  let panDragStartY = 0;
  let panDragging = $state(false);

  const renderedFile = $derived(displayedFile ?? file);
  const renderedImageSource = $derived(displayedFile ? displayedImageSource : imageSource);
  const videoProgress = $derived(videoLength > 0 ? Math.min(100, Math.max(0, (videoTime / videoLength) * 100)) : 0);
  const comicProgress = $derived(comicPages > 0 ? Math.min(100, Math.max(0, ((comicPage + 1) / comicPages) * 100)) : 0);
  const playbackControlsActive = $derived(renderedFile.media_kind === 'video' || comicEntered);
  const geometry = $derived(viewerGeometry({
    intrinsicWidth,
    intrinsicHeight,
    viewportWidth: stageWidth,
    viewportHeight: stageHeight,
    rotation,
    fitMode,
    boundActualSizeToFit,
    inset: isFullscreen ? 0 : 36,
    maxScale: preserveNativeViewerSize(renderedFile) ? 1 : Number.POSITIVE_INFINITY
  }));
  const comicVisualWidth = $derived(((((geometry.rotation % 360) + 360) % 360 === 90 || ((geometry.rotation % 360) + 360) % 360 === 270) ? geometry.height : geometry.width) * zoom);
  const comicVisualHeight = $derived(((((geometry.rotation % 360) + 360) % 360 === 90 || ((geometry.rotation % 360) + 360) % 360 === 270) ? geometry.width : geometry.height) * zoom);
  const comicReadStyle = $derived(comicVisualWidth > 0 && comicVisualHeight > 0
    ? `top:${Math.max(12, (stageHeight - comicVisualHeight) / 2 + 12)}px;right:${Math.max(12, (stageWidth - comicVisualWidth) / 2 + 12)}px`
    : '');
  const fitGeometry = $derived(viewerGeometry({
    intrinsicWidth,
    intrinsicHeight,
    viewportWidth: stageWidth,
    viewportHeight: stageHeight,
    rotation,
    fitMode: 'fit_window',
    inset: isFullscreen ? 0 : 36,
    maxScale: preserveNativeViewerSize(renderedFile) ? 1 : Number.POSITIVE_INFINITY
  }));
  const minimumZoom = $derived(fitMode === 'actual' && !boundActualSizeToFit ? Math.min(1, fitGeometry.scale) : 1);
  // Geometry carries the capped Actual-mode baseline. Scale the local ceiling inversely so
  // manual zoom preserves the same absolute 32x ceiling and can always pass back through 1:1.
  const maximumZoom = $derived(fitMode === 'actual' && boundActualSizeToFit && geometry.scale > 0 ? Math.max(32, 32 / geometry.scale) : 32);
  const panLimits = $derived(viewerPanLimits(zoom));
  const dragPanAvailable = $derived(
    renderedFile.media_kind !== 'video'
    && renderedFile.media_kind !== 'audio'
    && !renderedFile.media_type.startsWith('audio/')
    && (panLimits.x > 0.5 || panLimits.y > 0.5)
  );
  const panSurfaceStyle = $derived(`width:${stageWidth + panLimits.x * 2}px;height:${stageHeight + panLimits.y * 2}px`);
  const visualStyle = $derived(viewerMediaStyle(geometry, { zoom }));

  onMount(() => {
    const stage = stageElement;
    if (!stage) return;

    const resize = () => {
      const rect = stage.getBoundingClientRect();
      stageWidth = rect.width;
      stageHeight = rect.height;
      queueMicrotask(reconcileViewerTransform);
    };
    resize();
    const observer = new ResizeObserver(resize);
    observer.observe(stage);

    const syncFullscreen = () => {
      const wasFullscreen = isFullscreen;
      const nextFullscreen = document.fullscreenElement === stage;
      isFullscreen = nextFullscreen;
      resize();
      if (isFullscreen) scheduleCursorIdle();
      else {
        cursorIdle = false;
        if (cursorIdleTimer) clearTimeout(cursorIdleTimer);
        cursorIdleTimer = undefined;
      }
      if (wasFullscreen && !nextFullscreen && closeOnFullscreenExit) {
        if (keepViewerAfterFullscreenExit) keepViewerAfterFullscreenExit = false;
        else onFullscreenExit?.();
      }
    };
    document.addEventListener('fullscreenchange', syncFullscreen);
    return () => {
      observer.disconnect();
      document.removeEventListener('fullscreenchange', syncFullscreen);
      if (playbackControlsTimer) clearTimeout(playbackControlsTimer);
      if (cursorIdleTimer) clearTimeout(cursorIdleTimer);
      if (fitModeFeedbackTimer) clearTimeout(fitModeFeedbackTimer);
      if (waitingTimer) clearTimeout(waitingTimer);
    };
  });

  function viewerSupportsFile(target: ViewerStageMedia) {
    return target.viewer_support === 'supported';
  }

  function unsupportedViewerMessage(target: ViewerStageMedia) {
    return `No viewer is available for this file type (${target.media_type}).`;
  }

  function clearWaitingTimer() {
    if (waitingTimer) clearTimeout(waitingTimer);
    waitingTimer = undefined;
  }

  function armWaitingTimer(generation: number) {
    clearWaitingTimer();
    waitingTimer = setTimeout(() => {
      if (generation === transitionGeneration) waitingForTarget = true;
    }, 200);
  }

  $effect(() => {
    const targetFile = file;
    const targetImageSource = imageSource;
    // Only a new requested file/source starts a transition. Internal presentation
    // state must not rerun this effect and cancel its pending-media timer.
    return untrack(() => {
    const generation = ++transitionGeneration;
    recordViewerRequest(generation);
    clearWaitingTimer();
    waitingForTarget = false;
    mediaError = viewerSupportsFile(targetFile) ? '' : unsupportedViewerMessage(targetFile);

    if (!displayedFile) {
      displayedFile = targetFile;
      displayedImageSource = targetImageSource;
      if (!viewerSupportsFile(targetFile)) {
        freezeGeneration += 1;
        freezeVisible = false;
        intrinsicWidth = 0;
        intrinsicHeight = 0;
        return;
      }
      armWaitingTimer(generation);
      return () => { if (generation === transitionGeneration) clearWaitingTimer(); };
    }
    if (displayedFile.id === targetFile.id && displayedImageSource === targetImageSource) return;

    const rendersImage = viewerSupportsFile(targetFile) && targetFile.media_kind !== 'video' && targetFile.media_kind !== 'audio' && !targetFile.media_type.startsWith('audio/');
    if (rendersImage) {
      // Once rapid navigation has frozen a committed frame, keep that exact snapshot until the
      // latest requested target is presentable. Re-freezing from an in-flight <img> can capture
      // obsolete pixels using newer geometry and reintroduce the #170 stretch/overlap artifact.
      if (!freezeVisible) freezePresentedImage();
      const metadataWidth = targetFile.metadata?.image_width ?? 0;
      const metadataHeight = targetFile.metadata?.image_height ?? 0;
      if (metadataWidth > 0 && metadataHeight > 0) {
        intrinsicWidth = metadataWidth;
        intrinsicHeight = metadataHeight;
      }
    }

    // Speculative media preload must never gate navigation. The actual video/audio element owns
    // readiness and error reporting, so install every target immediately and let its native media
    // lifecycle clear the short loading state just as image load/error does below.
    displayedFile = targetFile;
    displayedImageSource = targetImageSource;
    if (!viewerSupportsFile(targetFile)) {
      clearWaitingTimer();
      waitingForTarget = false;
      freezeGeneration += 1;
      freezeVisible = false;
      intrinsicWidth = 0;
      intrinsicHeight = 0;
      return;
    }
    armWaitingTimer(generation);
    return () => { if (generation === transitionGeneration) clearWaitingTimer(); };
    });
  });

  $effect(() => {
    const nextFile = renderedFile;
    renderedImageSource;
    const rendersImage = viewerSupportsFile(nextFile) && nextFile.media_kind !== 'video' && nextFile.media_kind !== 'audio' && !nextFile.media_type.startsWith('audio/');
    // Image transitions install target metadata geometry while the presentation shield preserves old pixels separately.
    // Non-image media waits for its own metadata path and starts with no image geometry.
    if (!rendersImage) {
      intrinsicWidth = 0;
      intrinsicHeight = 0;
    }
    zoom = 1;
    panX = 0;
    panY = 0;
    videoPaused = true;
    videoTime = 0;
    videoLength = 0;
    if (nextFile.media_kind === 'video') showPlaybackControls();
    else if (!comicEntered && playbackControlsTimer) {
      clearTimeout(playbackControlsTimer);
      playbackControlsTimer = undefined;
      playbackControlsIdle = false;
    }
  });

  function freezePresentedImage() {
    const image = imageElement;
    const canvas = freezeCanvasElement;
    if (!image || !canvas || image.naturalWidth <= 0 || image.naturalHeight <= 0 || geometry.width <= 0 || geometry.height <= 0) return;

    const dpr = Math.max(1, Math.min(2, window.devicePixelRatio || 1));
    canvas.width = Math.max(1, Math.round(geometry.width * dpr));
    canvas.height = Math.max(1, Math.round(geometry.height * dpr));
    const context = canvas.getContext('2d');
    if (!context) return;
    try {
      context.clearRect(0, 0, canvas.width, canvas.height);
      context.imageSmoothingEnabled = scaling !== 'nearest';
      context.drawImage(image, 0, 0, canvas.width, canvas.height);
    } catch {
      return;
    }

    frozenStyle = visualStyle;
    freezeGeneration += 1;
    freezeVisible = true;
  }

  function imageMatchesCurrentSource(image: HTMLImageElement) {
    if (!renderedImageSource) return false;
    try {
      return image.currentSrc === new URL(renderedImageSource, document.baseURI).href;
    } catch {
      return image.currentSrc === renderedImageSource;
    }
  }

  function syncImage(event: Event) {
    const image = event.currentTarget;
    if (!(image instanceof HTMLImageElement) || !imageMatchesCurrentSource(image)) return;
    const generation = transitionGeneration;
    const source = renderedImageSource;
    intrinsicWidth = image.naturalWidth;
    intrinsicHeight = image.naturalHeight;
    clearWaitingTimer();
    waitingForTarget = false;
    mediaError = '';
    // Keep the frozen old pixels until the next paint after the *latest* target load. Stale
    // completions from sources superseded by rapid navigation must never release the freeze.
    requestAnimationFrame(() => {
      if (generation !== transitionGeneration || !imageMatchesCurrentSource(image)) return;
      freezeGeneration += 1;
      freezeVisible = false;
      recordViewerPresentation(generation);
      onPresented?.(source);
    });
  }

  function syncImageError(event: Event) {
    const image = event.currentTarget;
    if (!(image instanceof HTMLImageElement) || !imageMatchesCurrentSource(image)) return;
    // A failed/unsupported latest target has no paint event that can release the frozen frame.
    // End this handoff explicitly so stale pixels never stand in for the current file.
    clearWaitingTimer();
    waitingForTarget = false;
    mediaError = 'Media file could not be loaded. It may be missing from disk.';
    freezeGeneration += 1;
    freezeVisible = false;
    intrinsicWidth = 0;
    intrinsicHeight = 0;
  }

  function playableMatchesCurrentSource(media: HTMLMediaElement) {
    const source = renderedFile.media_urls.content;
    if (!source) return false;
    try {
      return media.currentSrc === new URL(source, document.baseURI).href;
    } catch {
      return media.currentSrc === source;
    }
  }

  function markPlayablePresented(media: HTMLMediaElement) {
    if (!playableMatchesCurrentSource(media)) return;
    const generation = transitionGeneration;
    clearWaitingTimer();
    waitingForTarget = false;
    mediaError = '';
    recordViewerPresentation(generation);
    onPresented?.(renderedImageSource);
  }

  function syncVideo() {
    const video = videoElement;
    if (!video) return;
    if (video.videoWidth > 0 && video.videoHeight > 0) {
      intrinsicWidth = video.videoWidth;
      intrinsicHeight = video.videoHeight;
    }
    videoPaused = video.paused;
    videoTime = video.currentTime;
    videoLength = Number.isFinite(video.duration) ? video.duration : 0;
  }

  function syncVideoPresented(event: Event) {
    syncVideo();
    const media = event.currentTarget;
    if (media instanceof HTMLVideoElement) markPlayablePresented(media);
  }

  function syncAudioPresented(event: Event) {
    const media = event.currentTarget;
    if (media instanceof HTMLAudioElement) markPlayablePresented(media);
  }

  function syncPlayableError(event: Event) {
    const media = event.currentTarget;
    if (!(media instanceof HTMLMediaElement) || !playableMatchesCurrentSource(media)) return;
    clearWaitingTimer();
    waitingForTarget = false;
    mediaError = 'Media file could not be loaded. It may be missing from disk.';
  }

  function revealPlaybackControls() {
    playbackControlsIdle = false;
    if (playbackControlsTimer) clearTimeout(playbackControlsTimer);
    playbackControlsTimer = setTimeout(() => {
      playbackControlsIdle = true;
      playbackControlsTimer = undefined;
    }, 2000);
  }

  function showPlaybackControls() {
    if (!playbackControlsActive) return;
    revealPlaybackControls();
  }

  $effect(() => {
    const nextComicEntered = comicEntered;
    if (nextComicEntered === previousComicEntered) return;
    previousComicEntered = nextComicEntered;
    comicTransition = nextComicEntered ? 'entering' : 'exiting';
    if (nextComicEntered) revealPlaybackControls();
    const timer = setTimeout(() => { comicTransition = ''; }, 520);
    return () => clearTimeout(timer);
  });

  function seekComicAt(clientX: number, control: HTMLElement) {
    if (!comicEntered || comicPages <= 0 || !onComicPageSelect) return;
    const rect = control.getBoundingClientRect();
    if (!rect.width) return;
    const ratio = Math.max(0, Math.min(0.999999, (clientX - rect.left) / rect.width));
    onComicPageSelect(Math.min(comicPages - 1, Math.floor(ratio * comicPages)));
    showPlaybackControls();
  }

  function seekComic(event: MouseEvent) {
    restoreStageFocusAfterPointer(event);
    const control = event.currentTarget;
    if (control instanceof HTMLElement) seekComicAt(event.clientX, control);
  }

  async function togglePlayback() {
    const media = videoElement ?? audioElement;
    if (!media) return;
    if (media.paused) await media.play();
    else media.pause();
  }

  function scheduleCursorIdle(event?: PointerEvent) {
    cursorIdle = false;
    if (cursorIdleTimer) clearTimeout(cursorIdleTimer);
    cursorIdleTimer = undefined;
    if (!isFullscreen) return;
    const target = event?.target;
    if (target instanceof Element && target.closest('.lightbox-video-controls')) return;
    cursorIdleTimer = setTimeout(() => {
      cursorIdle = true;
      cursorIdleTimer = undefined;
    }, 1000);
  }

  function handleStagePointerMove(event: PointerEvent) {
    scheduleCursorIdle(event);
    if (!playbackControlsActive) return;
    const rect = stageElement?.getBoundingClientRect();
    if (!rect) return;
    const revealDistance = Math.min(160, Math.max(80, rect.height * 0.22));
    if (event.clientY >= rect.bottom - revealDistance) showPlaybackControls();
  }

  function handleControlsPointerEnter(event: PointerEvent) {
    showPlaybackControls();
    scheduleCursorIdle(event);
  }

  function seekPlayback(deltaSeconds: number) {
    const media = videoElement ?? audioElement;
    if (!media || !Number.isFinite(media.duration) || media.duration <= 0) return false;
    media.currentTime = Math.max(0, Math.min(media.duration, media.currentTime + deltaSeconds));
    if (media === videoElement) syncVideo();
    return true;
  }

  function viewerPanLimits(nextZoom: number) {
    const normalizedRotation = ((rotation % 360) + 360) % 360;
    const quarterTurn = normalizedRotation === 90 || normalizedRotation === 270;
    const renderedWidth = (quarterTurn ? geometry.height : geometry.width) * nextZoom;
    const renderedHeight = (quarterTurn ? geometry.width : geometry.height) * nextZoom;
    const inset = isFullscreen ? 0 : 36;
    const viewportWidth = Math.max(0, stageWidth - inset * 2);
    const viewportHeight = Math.max(0, stageHeight - inset * 2);
    return {
      x: Math.max(0, (renderedWidth - viewportWidth) / 2),
      y: Math.max(0, (renderedHeight - viewportHeight) / 2)
    };
  }

  function clampViewerPan(nextX: number, nextY: number, nextZoom: number) {
    const limits = viewerPanLimits(nextZoom);
    return {
      x: Math.max(-limits.x, Math.min(limits.x, nextX)),
      y: Math.max(-limits.y, Math.min(limits.y, nextY))
    };
  }

  function syncNativePan() {
    const viewport = panViewportElement;
    if (!viewport) return;
    const limits = viewerPanLimits(zoom);
    viewport.scrollLeft = limits.x - panX;
    viewport.scrollTop = limits.y - panY;
  }

  function syncPanFromNativeScroll() {
    const viewport = panViewportElement;
    if (!viewport) return;
    const limits = viewerPanLimits(zoom);
    panX = limits.x - viewport.scrollLeft;
    panY = limits.y - viewport.scrollTop;
  }

  function beginPanDrag(event: PointerEvent) {
    const viewport = panViewportElement;
    if (event.button !== 0 || !dragPanAvailable || !viewport) return;
    event.preventDefault();
    stageElement?.focus({ preventScroll: true });
    panDragPointerId = event.pointerId;
    panDragStartClientX = event.clientX;
    panDragStartClientY = event.clientY;
    panDragStartX = panX;
    panDragStartY = panY;
    panDragging = true;
    viewport.setPointerCapture(event.pointerId);
  }

  function dragPan(event: PointerEvent) {
    if (panDragPointerId !== event.pointerId) return;
    event.preventDefault();
    const clamped = clampViewerPan(
      panDragStartX + event.clientX - panDragStartClientX,
      panDragStartY + event.clientY - panDragStartClientY,
      zoom
    );
    panX = clamped.x;
    panY = clamped.y;
    syncNativePan();
  }

  function endPanDrag(event: PointerEvent) {
    if (panDragPointerId !== event.pointerId) return;
    event.preventDefault();
    const viewport = panViewportElement;
    if (viewport?.hasPointerCapture(event.pointerId)) viewport.releasePointerCapture(event.pointerId);
    panDragPointerId = undefined;
    panDragging = false;
  }

  function losePanDragCapture(event: PointerEvent) {
    if (panDragPointerId !== event.pointerId) return;
    panDragPointerId = undefined;
    panDragging = false;
  }

  function reconcileViewerTransform() {
    const nextZoom = Math.max(minimumZoom, zoom);
    const clamped = clampViewerPan(panX, panY, nextZoom);
    zoom = nextZoom;
    panX = clamped.x;
    panY = clamped.y;
    queueMicrotask(syncNativePan);
  }

  function resetViewerTransform() {
    zoom = 1;
    panX = 0;
    panY = 0;
    queueMicrotask(syncNativePan);
  }

  const configuredFitModes: ViewerConfiguredFitMode[] = ['fit_window', 'fit_down_only', 'original_size_if_fit'];

  function fitModeLabel(mode: ViewerFitMode) {
    if (mode === 'fit_window') return 'Fit window';
    if (mode === 'fit_down_only') return 'Fit down only';
    if (mode === 'original_size_if_fit') return 'Original size if it fits';
    return 'Actual size';
  }

  function announceFitMode() {
    fitModeFeedback = fitModeLabel(fitMode);
    if (fitModeFeedbackTimer) clearTimeout(fitModeFeedbackTimer);
    fitModeFeedbackTimer = setTimeout(() => { fitModeFeedback = ''; fitModeFeedbackTimer = undefined; }, 1100);
  }

  function setFitMode(mode: ViewerFitMode, announce = false) {
    fitMode = mode;
    updateViewerSessionPreferences({ fitMode });
    resetViewerTransform();
    if (announce) announceFitMode();
  }

  function cycleFitMode() {
    const current = configuredFitModes.indexOf(fitMode as ViewerConfiguredFitMode);
    setFitMode(configuredFitModes[(current + 1 + configuredFitModes.length) % configuredFitModes.length], true);
  }

  function toggleScaling() {
    scaling = scaling === 'nearest' ? 'smooth' : 'nearest';
    updateViewerSessionPreferences({ scaling });
    fitModeFeedback = scaling === 'nearest' ? 'Nearest-neighbor scaling' : 'Smooth scaling';
    if (fitModeFeedbackTimer) clearTimeout(fitModeFeedbackTimer);
    fitModeFeedbackTimer = setTimeout(() => { fitModeFeedback = ''; fitModeFeedbackTimer = undefined; }, 1100);
  }

  function handleViewerWheel(event: WheelEvent) {
    if (renderedFile.media_kind === 'audio' || renderedFile.media_type.startsWith('audio/')) return;
    const stage = stageElement;
    const viewport = panViewportElement;
    if (!stage || !viewport) return;

    if (event.ctrlKey) {
      event.preventDefault();
      const nextZoom = Math.max(minimumZoom, Math.min(maximumZoom, zoom * Math.exp(-event.deltaY * 0.0075)));
      if (Math.abs(nextZoom - zoom) < 0.0001) return;
      const rect = stage.getBoundingClientRect();
      const pointerX = event.clientX - (rect.left + rect.width / 2);
      const pointerY = event.clientY - (rect.top + rect.height / 2);
      const ratio = nextZoom / zoom;
      const anchoredX = pointerX - (pointerX - panX) * ratio;
      const anchoredY = pointerY - (pointerY - panY) * ratio;
      const clamped = clampViewerPan(anchoredX, anchoredY, nextZoom);
      zoom = nextZoom;
      panX = clamped.x;
      panY = clamped.y;
      queueMicrotask(syncNativePan);
      return;
    }

    if (zoom <= minimumZoom + 0.0001) return;
    if (!event.altKey) return;
    event.preventDefault();
    viewport.scrollBy({ left: event.deltaY + event.deltaX, top: 0, behavior: 'auto' });
  }

  async function toggleFullscreen() {
    const stage = stageElement;
    if (!stage) return;
    const exiting = document.fullscreenElement === stage;
    if (exiting && closeOnFullscreenExit) keepViewerAfterFullscreenExit = true;
    try {
      if (exiting) await document.exitFullscreen();
      else await stage.requestFullscreen();
    } catch {
      if (exiting) keepViewerAfterFullscreenExit = false;
      // Browsers may deny fullscreen without a user gesture; keyboard/button use normally qualifies.
    }
  }

  function restoreStageFocusAfterPointer(event: MouseEvent) {
    if (event.detail <= 0) return;
    queueMicrotask(() => stageElement?.focus({ preventScroll: true }));
  }

  function rotateViewerAndReconcile(direction: 'left' | 'right') {
    rotation = rotateViewer(rotation, direction);
    updateViewerSessionPreferences({ rotation: normalizeViewerRotation(rotation) as ViewerRotation });
    queueMicrotask(reconcileViewerTransform);
  }

  function handleViewerKeydown(event: KeyboardEvent) {
    if (event.defaultPrevented || hasBlockingModal() || hasCommandModifier(event)) return;
    const target = event.target;
    const shiftedArrow = event.shiftKey && (event.key === 'ArrowLeft' || event.key === 'ArrowRight');
    const emptyViewerTagInput = target instanceof HTMLInputElement
      && target.id === 'tags-' + renderedFile.id
      && target.value === '';
    const delegatedTagKey = isEmptyViewerTagShortcut(event, renderedFile.id);
    if (isEditableShortcutTarget(target) && !(shiftedArrow && emptyViewerTagInput) && !delegatedTagKey) return;
    const targetInsideStage = target instanceof Node && Boolean(stageElement?.contains(target));
    if (targetInsideStage && isInteractiveShortcutTarget(target)) return;

    if (event.key === 'Escape' && isFullscreen) {
      event.preventDefault();
      event.stopPropagation();
      void document.exitFullscreen();
      return;
    }

    if (onPrimaryAction && (event.code === 'Space' || event.key === 'Enter')) {
      if (isInteractiveShortcutTarget(target) && !delegatedTagKey) return;
      event.preventDefault();
      event.stopPropagation();
      onPrimaryAction();
      return;
    }

    if (shiftedArrow) {
      const delta = event.key === 'ArrowLeft' ? -5 : 5;
      if (seekPlayback(delta)) {
        event.preventDefault();
        event.stopPropagation();
        return;
      }
    }

    if (keyboardNavigation && (event.key === 'ArrowLeft' || event.key === 'k')) {
      event.preventDefault();
      event.stopPropagation();
      onPrev();
      return;
    }
    if (keyboardNavigation && (event.key === 'ArrowRight' || event.key === 'j')) {
      event.preventDefault();
      event.stopPropagation();
      onNext();
      return;
    }

    const key = event.key.toLowerCase();
    if (key === 's') {
      event.preventDefault();
      event.stopPropagation();
      toggleScaling();
      return;
    }
    if (key === 'v') {
      event.preventDefault();
      event.stopPropagation();
      cycleFitMode();
      return;
    }
    if (key === 'f') {
      event.preventDefault();
      event.stopPropagation();
      void toggleFullscreen();
      return;
    }
    if (key === 'r') {
      event.preventDefault();
      event.stopPropagation();
      rotateViewerAndReconcile('right');
      return;
    }
    if (key === 'l') {
      event.preventDefault();
      event.stopPropagation();
      rotateViewerAndReconcile('left');
      return;
    }
    if (event.key === '1') {
      event.preventDefault();
      event.stopPropagation();
      setFitMode('fit_window', true);
      return;
    }
    if (event.key === '2') {
      event.preventDefault();
      event.stopPropagation();
      setFitMode('actual', true);
      return;
    }
    if (event.code === 'Space') {
      event.preventDefault();
      event.stopPropagation();
      if (videoElement || audioElement) void togglePlayback();
      else stageElement?.focus({ preventScroll: true });
    }
  }

  function seekVideoAt(clientX: number, control: HTMLElement) {
    const video = videoElement;
    if (!video || !videoLength) return;
    const rect = control.getBoundingClientRect();
    if (!rect.width) return;
    video.currentTime = Math.max(0, Math.min(videoLength, ((clientX - rect.left) / rect.width) * videoLength));
    syncVideo();
  }

  function seekVideo(event: MouseEvent) {
    restoreStageFocusAfterPointer(event);
    const control = event.currentTarget;
    if (control instanceof HTMLElement) seekVideoAt(event.clientX, control);
  }

  function beginVideoSeek(event: PointerEvent) {
    if (event.button !== 0) return;
    const control = event.currentTarget;
    const video = videoElement;
    if (!(control instanceof HTMLElement) || !video || !videoLength) return;
    event.preventDefault();
    restoreStageFocusAfterPointer(event);
    videoSeekPointerId = event.pointerId;
    videoSeekPlaybackState = { video, paused: video.paused, muted: video.muted };
    video.muted = true;
    video.pause();
    syncVideo();
    control.setPointerCapture(event.pointerId);
    seekVideoAt(event.clientX, control);
  }

  function dragVideoSeek(event: PointerEvent) {
    if (videoSeekPointerId !== event.pointerId) return;
    const control = event.currentTarget;
    if (control instanceof HTMLElement) seekVideoAt(event.clientX, control);
  }

  function restoreVideoSeekPlayback() {
    const playbackState = videoSeekPlaybackState;
    videoSeekPlaybackState = undefined;
    if (!playbackState) return;
    playbackState.video.muted = playbackState.muted;
    if (!playbackState.paused) void playbackState.video.play().catch(() => {});
    if (playbackState.video === videoElement) syncVideo();
  }

  function endVideoSeek(event: PointerEvent) {
    if (videoSeekPointerId !== event.pointerId) return;
    const control = event.currentTarget;
    videoSeekPointerId = undefined;
    if (control instanceof HTMLElement) {
      seekVideoAt(event.clientX, control);
      if (control.hasPointerCapture(event.pointerId)) control.releasePointerCapture(event.pointerId);
    }
    restoreVideoSeekPlayback();
  }

  function loseVideoSeekCapture(event: PointerEvent) {
    if (videoSeekPointerId !== event.pointerId) return;
    videoSeekPointerId = undefined;
    restoreVideoSeekPlayback();
  }

  function clock(seconds: number) {
    if (!Number.isFinite(seconds) || seconds < 0) return '0:00';
    const whole = Math.floor(seconds);
    return `${Math.floor(whole / 60)}:${String(whole % 60).padStart(2, '0')}`;
  }
</script>

<svelte:window onkeydown={handleViewerKeydown} />

<!-- Pointer motion only tracks fullscreen cursor idleness; keyboard viewer controls are handled globally. -->
<!-- svelte-ignore a11y_no_static_element_interactions -->
<div bind:this={stageElement} class:fullscreen={isFullscreen} class:waiting={waitingForTarget} class:cursor-idle={isFullscreen && cursorIdle} class:comic-reading={comicEntered} class:entering={comicTransition === 'entering'} class:exiting={comicTransition === 'exiting'} class:nearest-scaling={scaling === 'nearest'} class="lightbox-stage viewer-stage" tabindex="-1" aria-busy={waitingForTarget} onpointermove={handleStagePointerMove}>
  <!-- Drag panning supplements native viewport scrolling; an interactive ARIA role would misdescribe this surface. -->
  <!-- svelte-ignore a11y_no_static_element_interactions -->
  <div
    bind:this={panViewportElement}
    class="viewer-pan-viewport"
    class:pannable={dragPanAvailable}
    class:dragging={panDragging}
    onwheel={handleViewerWheel}
    onscroll={syncPanFromNativeScroll}
    onpointerdown={beginPanDrag}
    onpointermove={dragPan}
    onpointerup={endPanDrag}
    onpointercancel={endPanDrag}
    onlostpointercapture={losePanDragCapture}
  >
    <div class="viewer-pan-surface" style={panSurfaceStyle}>
      {#if renderedFile.viewer_support !== 'unsupported_media_type'}
      {#if renderedFile.media_kind === 'video'}
        <!-- svelte-ignore a11y_media_has_caption -->
        <video
          bind:this={videoElement}
          class="viewer-visual-media"
          style={visualStyle}
          src={renderedFile.media_urls.content}
          poster={renderedFile.media_urls.preview}
          preload="auto"
          autoplay
          loop
          onclick={(event) => { void togglePlayback(); restoreStageFocusAfterPointer(event); }}
          onloadedmetadata={syncVideoPresented}
          onerror={syncPlayableError}
          ontimeupdate={syncVideo}
          onplay={syncVideo}
          onpause={syncVideo}
          onended={syncVideo}
        ></video>
      {:else if renderedFile.media_kind === 'audio' || renderedFile.media_type.startsWith('audio/')}
        <div class="audio-stage viewer-audio-stage" style={`transform: rotate(${rotation}deg);`}>
          <div class="audio-art"><Icon name="audio" size={42} /></div>
          <audio bind:this={audioElement} src={renderedFile.media_urls.content} controls preload="auto" onloadedmetadata={syncAudioPresented} onerror={syncPlayableError}></audio>
        </div>
      {:else}
        <canvas
          bind:this={freezeCanvasElement}
          class="viewer-image-freeze"
          class:visible={freezeVisible}
          style={frozenStyle}
          aria-hidden="true"
        ></canvas>
        <img
          bind:this={imageElement}
          class="viewer-visual-media"
          class:viewer-image-concealed={freezeVisible}
          style={visualStyle}
          src={renderedImageSource}
          alt={renderedFile.name}
          draggable={false}
          onload={syncImage}
          onerror={syncImageError}
        />
      {/if}
      {/if}
    </div>
  </div>

  {#if comicAvailable && !comicEntered}
    <button class="comic-read-button" type="button" style={comicReadStyle} disabled={comicLoading} aria-label="Read comic" onclick={(event) => { onToggleComic?.(); restoreStageFocusAfterPointer(event); }}>
      <svg class="icon" viewBox="0 0 20 20" fill="none" stroke="currentColor" stroke-width="1.55" aria-hidden="true">
        <path d="M4.2 3.8h7.1a2 2 0 0 1 2 2v10.4H6.2a2 2 0 0 1-2-2V3.8Z"/>
        <path d="M13.3 5.8h2.5v10.4h-7a2.6 2.6 0 0 0-2.6 0"/>
      </svg>
      <span>{comicLoading ? 'Loading comic…' : 'Read comic'}</span>
      <svg class="icon enter-arrow" viewBox="0 0 20 20" fill="none" stroke="currentColor" stroke-width="1.55" aria-hidden="true">
        <path d="M4 10h11M11 6l4 4-4 4"/>
      </svg>
    </button>
    {#if comicError}<div class="comic-error-overlay" role="alert">{comicError}</div>{/if}
  {/if}

  {#if renderedFile.media_kind === 'video' || comicEntered}
    <!-- Pointer entry keeps transient controls visible; onfocusin is the keyboard/focus equivalent. -->
    <!-- svelte-ignore a11y_no_static_element_interactions -->
    <div class="lightbox-video-controls" class:comic-controls={comicEntered} class:is-idle={playbackControlsIdle} onpointerenter={handleControlsPointerEnter} onfocusin={showPlaybackControls}>
      {#if comicEntered}
        <button class="video-play comic-exit" type="button" aria-label="Exit comic (Space)" onclick={(event) => { onToggleComic?.(); restoreStageFocusAfterPointer(event); }}>
          <Icon name="close" size={13} /><span>Space</span>
        </button>
        <span class="video-time">{comicPage + 1}</span>
        <button class="video-progress" type="button" aria-label="Seek comic page" onclick={seekComic}>
          <span style={`width: ${comicProgress}%`}></span>
        </button>
        <span class="video-time">{comicPages}</span>
      {:else}
        <button class="video-play" type="button" aria-label={videoPaused ? 'Play video' : 'Pause video'} onclick={(event) => { void togglePlayback(); restoreStageFocusAfterPointer(event); }}>
          <Icon name={videoPaused ? 'play' : 'pause'} size={14} />
        </button>
        <span class="video-time">{clock(videoTime)}</span>
        <button class="video-progress" type="button" aria-label="Seek video" onclick={seekVideo} onpointerdown={beginVideoSeek} onpointermove={dragVideoSeek} onpointerup={endVideoSeek} onpointercancel={endVideoSeek} onlostpointercapture={loseVideoSeekCapture}>
          <span style={`width: ${videoProgress}%`}></span>
        </button>
        <span class="video-time">{videoLength ? clock(videoLength) : (mediaDuration(renderedFile) || '0:00')}</span>
      {/if}
    </div>
  {/if}

  {#if waitingForTarget}<div class="viewer-loading-indicator" role="status" aria-live="polite">Loading media…</div>{/if}
  {#if mediaError}<div class="viewer-media-error" role="alert">{mediaError}</div>{/if}
  {#if navigationError}<div class="viewer-media-error" role="alert">Unable to navigate: {navigationError}</div>{/if}
  {#if fitModeFeedback}<div class="viewer-mode-feedback" role="status" aria-live="polite">{fitModeFeedback}</div>{/if}

  <div class="viewer-mode-controls" aria-label="Viewer display controls">
    <button type="button" class="viewer-mode-button" class:active={fitMode === 'fit_window'} aria-label="Fit to window" title="Fit to window (1)" onclick={(event) => { setFitMode('fit_window', true); restoreStageFocusAfterPointer(event); }}>1</button>
    <button type="button" class="viewer-mode-button" aria-label="Cycle fit mode" title="Cycle fit mode (V)" onclick={(event) => { cycleFitMode(); restoreStageFocusAfterPointer(event); }}>V</button>
    <button type="button" class="viewer-mode-button" class:active={fitMode === 'actual'} aria-label="Actual size" title="Actual size (2)" onclick={(event) => { setFitMode('actual', true); restoreStageFocusAfterPointer(event); }}>2</button>
    <button type="button" class="viewer-mode-button" aria-label="Rotate left" title="Rotate left (L)" onclick={(event) => { rotateViewerAndReconcile('left'); restoreStageFocusAfterPointer(event); }}>↺</button>
    <button type="button" class="viewer-mode-button" aria-label="Rotate right" title="Rotate right (R)" onclick={(event) => { rotateViewerAndReconcile('right'); restoreStageFocusAfterPointer(event); }}>↻</button>
    <button type="button" class="viewer-mode-button" class:active={isFullscreen} aria-label="Toggle fullscreen" title="Fullscreen (F)" onclick={(event) => { void toggleFullscreen(); restoreStageFocusAfterPointer(event); }}>F</button>
  </div>

  <button class="lightbox-nav-arrow prev" type="button" title={`Previous ${navigationUnit} (←)`} aria-label={`Previous ${navigationUnit}`} onclick={(event) => { onPrev(); restoreStageFocusAfterPointer(event); }}><Icon name="chev_left" size={20} /></button>
  <button class="lightbox-nav-arrow next" type="button" title={`Next ${navigationUnit} (→)`} aria-label={`Next ${navigationUnit}`} onclick={(event) => { onNext(); restoreStageFocusAfterPointer(event); }}><Icon name="chev_right" size={20} /></button>
</div>

<style>
  :global(.viewer-stage) {
    position: relative;
    overflow: hidden;
  }

  .viewer-pan-viewport {
    position: absolute;
    inset: 0;
    overflow: auto;
    scrollbar-width: none;
    overscroll-behavior: contain;
  }

  .viewer-pan-viewport::-webkit-scrollbar { display: none; }

  .viewer-pan-viewport.pannable {
    cursor: grab;
    touch-action: none;
  }

  .viewer-pan-viewport.pannable.dragging {
    cursor: grabbing;
    user-select: none;
  }

  :global(.viewer-stage:fullscreen.cursor-idle) .viewer-pan-viewport {
    cursor: none;
  }

  .viewer-pan-surface {
    position: relative;
    min-width: 100%;
    min-height: 100%;
  }

  :global(.viewer-stage:fullscreen) {
    width: 100vw;
    height: 100vh;
    background: #000;
  }

  :global(.viewer-stage:fullscreen.cursor-idle) {
    cursor: none;
  }

  :global(.viewer-stage .viewer-visual-media) {
    object-fit: contain;
    transition: filter 120ms ease, opacity 120ms ease;
  }

  :global(.viewer-stage.nearest-scaling img.viewer-visual-media),
  :global(.viewer-stage.nearest-scaling .viewer-image-freeze) {
    image-rendering: pixelated;
  }

  .viewer-image-freeze {
    display: none;
    pointer-events: none;
    z-index: 2;
  }

  .viewer-image-freeze.visible {
    display: block;
  }

  :global(.viewer-stage .viewer-visual-media.viewer-image-concealed) {
    visibility: hidden;
  }

  :global(.viewer-stage .viewer-audio-stage) {
    transform-origin: center;
    transition: transform 120ms ease, filter 120ms ease, opacity 120ms ease;
  }

  :global(.viewer-stage.waiting .viewer-visual-media),
  :global(.viewer-stage.waiting .viewer-audio-stage) {
    filter: grayscale(1) brightness(0.65);
    opacity: 0.58;
  }

  .viewer-loading-indicator,
  .viewer-media-error {
    position: absolute;
    z-index: 5;
    left: 50%;
    top: 50%;
    transform: translate(-50%, -50%);
    padding: 7px 10px;
    border-radius: 5px;
    background: rgba(0, 0, 0, 0.76);
    color: #fff;
    font: 600 11px/1.2 var(--font-mono);
    pointer-events: none;
  }

  .viewer-media-error {
    max-width: min(420px, calc(100% - 40px));
    text-align: center;
  }

  .viewer-mode-feedback {
    position: absolute;
    z-index: 5;
    left: 50%;
    bottom: 54px;
    transform: translateX(-50%);
    padding: 7px 10px;
    border: 1px solid rgba(255, 255, 255, 0.16);
    border-radius: 5px;
    background: rgba(0, 0, 0, 0.76);
    color: #fff;
    font: 600 11px/1.2 var(--font-mono);
    pointer-events: none;
  }

  .viewer-mode-controls {
    position: absolute;
    z-index: 4;
    top: 12px;
    left: 50%;
    transform: translateX(-50%);
    display: flex;
    gap: 4px;
    padding: 4px;
    border: 1px solid rgba(255, 255, 255, 0.15);
    border-radius: 6px;
    background: rgba(0, 0, 0, 0.68);
    backdrop-filter: blur(8px);
  }

  .viewer-mode-button {
    min-width: 28px;
    height: 28px;
    padding: 0 7px;
    border: 0;
    border-radius: 4px;
    background: transparent;
    color: rgba(255, 255, 255, 0.72);
    font: 600 12px/1 var(--font-mono);
    cursor: pointer;
  }

  .viewer-mode-button:hover,
  .viewer-mode-button.active {
    color: #fff;
    background: rgba(255, 255, 255, 0.15);
  }

  :global(.viewer-stage:fullscreen .viewer-mode-controls) {
    top: 8px;
  }
</style>