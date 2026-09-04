<script lang="ts">
  import { onMount } from 'svelte';
  import Icon from './Icon.svelte';
  import { readViewerSessionPreferences, updateViewerSessionPreferences, type ViewerRotation } from '$lib/state/viewerSessionPreferences';
  import { mediaDuration } from '$lib/utils/format';
  import { hasCommandModifier, isEditableShortcutTarget, isInteractiveShortcutTarget } from '$lib/utils/keyboard';
  import { preserveNativeViewerSize } from '$lib/utils/media';
  import { normalizeViewerRotation, rotateViewer, viewerGeometry, viewerMediaStyle, type ViewerFitMode } from '$lib/utils/viewer';
  import { preloadViewerMediaSource, viewerPreloadSource } from '$lib/utils/viewerPreload';
  import type { FileItem } from '$lib/api/types';

  let {
    file,
    imageSource,
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
    onToggleComic,
    onComicPageSelect
  } = $props<{
    file: FileItem;
    imageSource: string;
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
    onToggleComic?: () => void;
    onComicPageSelect?: (index: number) => void;
  }>();

  const initialViewerPreferences = readViewerSessionPreferences({
    preferOriginal: false,
    rotation: 0,
    fitMode: 'screen'
  });

  let stageElement = $state<HTMLDivElement | undefined>();
  let comicTransition = $state<'' | 'entering' | 'exiting'>('');
  let previousComicEntered = comicEntered;
  let panViewportElement = $state<HTMLDivElement | undefined>();
  let imageElement = $state<HTMLImageElement | undefined>();
  let freezeCanvasElement = $state<HTMLCanvasElement | undefined>();
  let freezeVisible = $state(false);
  let frozenStyle = $state('');
  let freezeGeneration = 0;
  let videoElement = $state<HTMLVideoElement | undefined>();
  let audioElement = $state<HTMLAudioElement | undefined>();
  let displayedFile = $state<FileItem | undefined>();
  let displayedImageSource = $state('');
  let waitingForTarget = $state(false);
  let transitionGeneration = 0;
  let rotation = $state<number>(initialViewerPreferences.rotation);
  let fitMode = $state<ViewerFitMode>(initialViewerPreferences.fitMode);
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
  let cursorIdle = $state(false);
  let cursorIdleTimer: ReturnType<typeof setTimeout> | undefined;
  let zoom = $state(1);
  let panX = $state(0);
  let panY = $state(0);

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
    fitMode: 'screen',
    inset: isFullscreen ? 0 : 36,
    maxScale: preserveNativeViewerSize(renderedFile) ? 1 : Number.POSITIVE_INFINITY
  }));
  const minimumZoom = $derived(fitMode === 'screen' ? 1 : Math.min(1, fitGeometry.scale));
  const panLimits = $derived(viewerPanLimits(zoom));
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
    };
  });

  $effect(() => {
    const targetFile = file;
    const targetImageSource = imageSource;
    const generation = ++transitionGeneration;
    waitingForTarget = false;

    if (!displayedFile) {
      displayedFile = targetFile;
      displayedImageSource = targetImageSource;
      return;
    }
    if (displayedFile.id === targetFile.id && displayedImageSource === targetImageSource) return;

    const rendersImage = targetFile.media_kind !== 'video' && targetFile.media_kind !== 'audio' && !targetFile.media_type.startsWith('audio/');
    if (rendersImage) {
      // A single <img> cannot preserve both sides of an aspect-ratio handoff: keeping old geometry
      // makes the first target pixels letterbox inside the old box, while applying target geometry
      // can resize pixels the browser is still retaining from the old source. Freeze the already
      // painted frame into a canvas before changing either property, then let the real foreground
      // image enter the browser's native loading/presentation path immediately with target geometry.
      freezePresentedImage();
      const metadataWidth = targetFile.metadata?.image_width ?? 0;
      const metadataHeight = targetFile.metadata?.image_height ?? 0;
      if (metadataWidth > 0 && metadataHeight > 0) {
        intrinsicWidth = metadataWidth;
        intrinsicHeight = metadataHeight;
      }
      displayedFile = targetFile;
      displayedImageSource = targetImageSource;
      return;
    }

    const waitingTimer = setTimeout(() => {
      if (generation === transitionGeneration) waitingForTarget = true;
    }, 200);
    const preloadSource = viewerPreloadSource(targetFile);

    void preloadViewerMediaSource(targetFile, preloadSource)
      .catch(() => undefined)
      .then(() => {
        if (generation !== transitionGeneration) return;
        clearTimeout(waitingTimer);
        displayedFile = targetFile;
        displayedImageSource = targetImageSource;
        waitingForTarget = false;
      });

    return () => {
      clearTimeout(waitingTimer);
      if (generation === transitionGeneration) transitionGeneration += 1;
    };
  });

  $effect(() => {
    const nextFile = renderedFile;
    renderedImageSource;
    const rendersImage = nextFile.media_kind !== 'video' && nextFile.media_kind !== 'audio' && !nextFile.media_type.startsWith('audio/');
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
      context.drawImage(image, 0, 0, canvas.width, canvas.height);
    } catch {
      return;
    }

    frozenStyle = visualStyle;
    freezeGeneration += 1;
    freezeVisible = true;
  }

  function syncImage(event: Event) {
    const image = event.currentTarget;
    if (!(image instanceof HTMLImageElement)) return;
    intrinsicWidth = image.naturalWidth;
    intrinsicHeight = image.naturalHeight;
    // Keep the frozen old pixels until the next paint after target load. The target is already
    // laid out at final geometry underneath, so changing both visibility states in the same
    // animation-frame callback presents only one image while avoiding an extra frame of latency.
    const generation = freezeGeneration;
    requestAnimationFrame(() => {
      if (generation === freezeGeneration) freezeVisible = false;
    });
  }

  function syncImageError() {
    // A failed/unsupported target has no paint event that can release the frozen frame.
    // End this handoff explicitly so stale pixels never stand in for the current file.
    freezeGeneration += 1;
    freezeVisible = false;
    intrinsicWidth = 0;
    intrinsicHeight = 0;
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

  function setFitMode(mode: ViewerFitMode) {
    fitMode = mode;
    updateViewerSessionPreferences({ fitMode });
    resetViewerTransform();
  }

  function handleViewerWheel(event: WheelEvent) {
    if (renderedFile.media_kind === 'audio' || renderedFile.media_type.startsWith('audio/')) return;
    const stage = stageElement;
    const viewport = panViewportElement;
    if (!stage || !viewport) return;

    if (event.ctrlKey) {
      event.preventDefault();
      const nextZoom = Math.max(minimumZoom, Math.min(32, zoom * Math.exp(-event.deltaY * 0.0075)));
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
    if (event.defaultPrevented || hasCommandModifier(event) || isEditableShortcutTarget(event.target)) return;
    const target = event.target;
    const targetInsideStage = target instanceof Node && Boolean(stageElement?.contains(target));
    if (targetInsideStage && isInteractiveShortcutTarget(target)) return;

    if (event.key === 'Escape' && isFullscreen) {
      event.preventDefault();
      event.stopPropagation();
      void document.exitFullscreen();
      return;
    }

    if (onPrimaryAction && (event.code === 'Space' || event.key === 'Enter')) {
      if (isInteractiveShortcutTarget(target)) return;
      event.preventDefault();
      event.stopPropagation();
      onPrimaryAction();
      return;
    }

    if (event.shiftKey && (event.key === 'ArrowLeft' || event.key === 'ArrowRight')) {
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
      setFitMode('screen');
      return;
    }
    if (event.key === '2') {
      event.preventDefault();
      event.stopPropagation();
      setFitMode('actual');
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
    if (!(control instanceof HTMLElement)) return;
    event.preventDefault();
    restoreStageFocusAfterPointer(event);
    videoSeekPointerId = event.pointerId;
    control.setPointerCapture(event.pointerId);
    seekVideoAt(event.clientX, control);
  }

  function dragVideoSeek(event: PointerEvent) {
    if (videoSeekPointerId !== event.pointerId) return;
    const control = event.currentTarget;
    if (control instanceof HTMLElement) seekVideoAt(event.clientX, control);
  }

  function endVideoSeek(event: PointerEvent) {
    if (videoSeekPointerId !== event.pointerId) return;
    const control = event.currentTarget;
    if (control instanceof HTMLElement) {
      seekVideoAt(event.clientX, control);
      if (control.hasPointerCapture(event.pointerId)) control.releasePointerCapture(event.pointerId);
    }
    videoSeekPointerId = undefined;
  }

  function clock(seconds: number) {
    if (!Number.isFinite(seconds) || seconds < 0) return '0:00';
    const whole = Math.floor(seconds);
    return `${Math.floor(whole / 60)}:${String(whole % 60).padStart(2, '0')}`;
  }
</script>

<svelte:window onkeydown={handleViewerKeydown} />

<div bind:this={stageElement} class:fullscreen={isFullscreen} class:waiting={waitingForTarget} class:cursor-idle={isFullscreen && cursorIdle} class:comic-reading={comicEntered} class:entering={comicTransition === 'entering'} class:exiting={comicTransition === 'exiting'} class="lightbox-stage viewer-stage" tabindex="-1" aria-busy={waitingForTarget} onpointermove={handleStagePointerMove}>
  <div bind:this={panViewportElement} class="viewer-pan-viewport" onwheel={handleViewerWheel} onscroll={syncPanFromNativeScroll}>
    <div class="viewer-pan-surface" style={panSurfaceStyle}>
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
          onloadedmetadata={syncVideo}
          ontimeupdate={syncVideo}
          onplay={syncVideo}
          onpause={syncVideo}
          onended={syncVideo}
        ></video>
      {:else if renderedFile.media_kind === 'audio' || renderedFile.media_type.startsWith('audio/')}
        <div class="audio-stage viewer-audio-stage" style={`transform: rotate(${rotation}deg);`}>
          <div class="audio-art"><Icon name="audio" size={42} /></div>
          <audio bind:this={audioElement} src={renderedFile.media_urls.content} controls preload="auto"></audio>
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
          onload={syncImage}
          onerror={syncImageError}
        />
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
        <button class="video-progress" type="button" aria-label="Seek video" onclick={seekVideo} onpointerdown={beginVideoSeek} onpointermove={dragVideoSeek} onpointerup={endVideoSeek} onpointercancel={endVideoSeek}>
          <span style={`width: ${videoProgress}%`}></span>
        </button>
        <span class="video-time">{videoLength ? clock(videoLength) : (mediaDuration(renderedFile) || '0:00')}</span>
      {/if}
    </div>
  {/if}

  <div class="viewer-mode-controls" aria-label="Viewer display controls">
    <button type="button" class="viewer-mode-button" class:active={fitMode === 'screen'} aria-label="Fit to screen" title="Fit to screen (1)" onclick={(event) => { setFitMode('screen'); restoreStageFocusAfterPointer(event); }}>1</button>
    <button type="button" class="viewer-mode-button" class:active={fitMode === 'actual'} aria-label="Actual size" title="Actual size (2)" onclick={(event) => { setFitMode('actual'); restoreStageFocusAfterPointer(event); }}>2</button>
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