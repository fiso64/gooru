<script lang="ts">
  import { onMount } from 'svelte';
  import Icon from './Icon.svelte';
  import { mediaDuration } from '$lib/utils/format';
  import { hasCommandModifier, isEditableShortcutTarget, isInteractiveShortcutTarget } from '$lib/utils/keyboard';
  import { preserveNativeViewerSize } from '$lib/utils/media';
  import { rotateViewer, viewerGeometry, viewerMediaStyle, type ViewerFitMode } from '$lib/utils/viewer';
  import { preloadViewerMediaSource, viewerPreloadSource } from '$lib/utils/viewerPreload';
  import type { FileItem } from '$lib/api/types';

  let {
    file,
    imageSource,
    onPrev,
    onNext
  } = $props<{
    file: FileItem;
    imageSource: string;
    onPrev: () => void;
    onNext: () => void;
  }>();

  let stageElement = $state<HTMLDivElement | undefined>();
  let videoElement = $state<HTMLVideoElement | undefined>();
  let audioElement = $state<HTMLAudioElement | undefined>();
  let displayedFile = $state<FileItem>(file);
  let displayedImageSource = $state(imageSource);
  let waitingForTarget = $state(false);
  let transitionGeneration = 0;
  let rotation = $state(0);
  let fitMode = $state<ViewerFitMode>('screen');
  let isFullscreen = $state(false);
  let stageWidth = $state(0);
  let stageHeight = $state(0);
  let intrinsicWidth = $state(0);
  let intrinsicHeight = $state(0);
  let videoPaused = $state(true);
  let videoTime = $state(0);
  let videoLength = $state(0);

  const videoProgress = $derived(videoLength > 0 ? Math.min(100, Math.max(0, (videoTime / videoLength) * 100)) : 0);
  const geometry = $derived(viewerGeometry({
    intrinsicWidth,
    intrinsicHeight,
    viewportWidth: stageWidth,
    viewportHeight: stageHeight,
    rotation,
    fitMode,
    inset: isFullscreen ? 0 : 36,
    maxScale: preserveNativeViewerSize(displayedFile) ? 1 : Number.POSITIVE_INFINITY
  }));
  const visualStyle = $derived(viewerMediaStyle(geometry));

  onMount(() => {
    const stage = stageElement;
    if (!stage) return;

    const resize = () => {
      const rect = stage.getBoundingClientRect();
      stageWidth = rect.width;
      stageHeight = rect.height;
    };
    resize();
    const observer = new ResizeObserver(resize);
    observer.observe(stage);

    const syncFullscreen = () => {
      isFullscreen = document.fullscreenElement === stage;
      resize();
    };
    document.addEventListener('fullscreenchange', syncFullscreen);
    return () => {
      observer.disconnect();
      document.removeEventListener('fullscreenchange', syncFullscreen);
    };
  });

  $effect(() => {
    const targetFile = file;
    const targetImageSource = imageSource;
    if (displayedFile.id === targetFile.id && displayedImageSource === targetImageSource) return;

    const generation = ++transitionGeneration;
    waitingForTarget = false;
    const waitingTimer = setTimeout(() => {
      if (generation === transitionGeneration) waitingForTarget = true;
    }, 200);
    const preloadSource = targetFile.media_kind === 'image' ? targetImageSource : viewerPreloadSource(targetFile);

    void preloadViewerMediaSource(targetFile, preloadSource)
      .catch(() => undefined)
      .then(() => {
        if (generation !== transitionGeneration) return;
        clearTimeout(waitingTimer);
        displayedFile = targetFile;
        displayedImageSource = targetImageSource;
        waitingForTarget = false;
      });

    return () => clearTimeout(waitingTimer);
  });

  $effect(() => {
    displayedFile.id;
    intrinsicWidth = 0;
    intrinsicHeight = 0;
    videoPaused = true;
    videoTime = 0;
    videoLength = 0;
  });

  function syncImage(event: Event) {
    const image = event.currentTarget;
    if (!(image instanceof HTMLImageElement)) return;
    intrinsicWidth = image.naturalWidth;
    intrinsicHeight = image.naturalHeight;
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

  async function togglePlayback() {
    const media = videoElement ?? audioElement;
    if (!media) return;
    if (media.paused) await media.play();
    else media.pause();
  }

  async function toggleFullscreen() {
    const stage = stageElement;
    if (!stage) return;
    try {
      if (document.fullscreenElement === stage) await document.exitFullscreen();
      else await stage.requestFullscreen();
    } catch {
      // Browsers may deny fullscreen without a user gesture; keyboard/button use normally qualifies.
    }
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
      rotation = rotateViewer(rotation, 'right');
      return;
    }
    if (key === 'l') {
      event.preventDefault();
      event.stopPropagation();
      rotation = rotateViewer(rotation, 'left');
      return;
    }
    if (event.key === '1') {
      event.preventDefault();
      event.stopPropagation();
      fitMode = 'screen';
      return;
    }
    if (event.key === '2') {
      event.preventDefault();
      event.stopPropagation();
      fitMode = 'actual';
      return;
    }
    if (event.code === 'Space') {
      event.preventDefault();
      event.stopPropagation();
      if (videoElement || audioElement) void togglePlayback();
      else stageElement?.focus({ preventScroll: true });
    }
  }

  function seekVideo(event: MouseEvent) {
    const video = videoElement;
    if (!video || !videoLength) return;
    const button = event.currentTarget;
    if (!(button instanceof HTMLElement)) return;
    const rect = button.getBoundingClientRect();
    if (!rect.width) return;
    video.currentTime = Math.max(0, Math.min(videoLength, ((event.clientX - rect.left) / rect.width) * videoLength));
    syncVideo();
  }

  function clock(seconds: number) {
    if (!Number.isFinite(seconds) || seconds < 0) return '0:00';
    const whole = Math.floor(seconds);
    return `${Math.floor(whole / 60)}:${String(whole % 60).padStart(2, '0')}`;
  }
</script>

<svelte:window onkeydown={handleViewerKeydown} />

<div bind:this={stageElement} class:fullscreen={isFullscreen} class:waiting={waitingForTarget} class="lightbox-stage viewer-stage" tabindex="-1" aria-busy={waitingForTarget}>
  {#if displayedFile.media_kind === 'video'}
    <!-- svelte-ignore a11y_media_has_caption -->
    <video
      bind:this={videoElement}
      class="viewer-visual-media"
      style={visualStyle}
      src={displayedFile.media_urls.content}
      poster={displayedFile.media_urls.preview}
      preload="auto"
      autoplay
      loop
      onclick={() => void togglePlayback()}
      onloadedmetadata={syncVideo}
      ontimeupdate={syncVideo}
      onplay={syncVideo}
      onpause={syncVideo}
      onended={syncVideo}
    ></video>

    <div class="lightbox-video-controls">
      <button class="video-play" type="button" aria-label={videoPaused ? 'Play video' : 'Pause video'} onclick={() => void togglePlayback()}>
        <Icon name={videoPaused ? 'play' : 'pause'} size={14} />
      </button>
      <span class="video-time">{clock(videoTime)}</span>
      <button class="video-progress" type="button" aria-label="Seek video" onclick={seekVideo}>
        <span style={`width: ${videoProgress}%`}></span>
      </button>
      <span class="video-time">{videoLength ? clock(videoLength) : (mediaDuration(displayedFile) || '0:00')}</span>
    </div>
  {:else if displayedFile.media_kind === 'audio' || displayedFile.media_type.startsWith('audio/')}
    <div class="audio-stage viewer-audio-stage" style={`transform: rotate(${rotation}deg);`}>
      <div class="audio-art"><Icon name="audio" size={42} /></div>
      <audio bind:this={audioElement} src={displayedFile.media_urls.content} controls preload="auto"></audio>
    </div>
  {:else}
    <img class="viewer-visual-media" style={visualStyle} src={displayedImageSource} alt={displayedFile.name} onload={syncImage} />
  {/if}

  <div class="viewer-mode-controls" aria-label="Viewer display controls">
    <button type="button" class="viewer-mode-button" class:active={fitMode === 'screen'} aria-label="Fit to screen" title="Fit to screen (1)" onclick={() => { fitMode = 'screen'; }}>1</button>
    <button type="button" class="viewer-mode-button" class:active={fitMode === 'actual'} aria-label="Actual size" title="Actual size (2)" onclick={() => { fitMode = 'actual'; }}>2</button>
    <button type="button" class="viewer-mode-button" aria-label="Rotate left" title="Rotate left (L)" onclick={() => { rotation = rotateViewer(rotation, 'left'); }}>↺</button>
    <button type="button" class="viewer-mode-button" aria-label="Rotate right" title="Rotate right (R)" onclick={() => { rotation = rotateViewer(rotation, 'right'); }}>↻</button>
    <button type="button" class="viewer-mode-button" class:active={isFullscreen} aria-label="Toggle fullscreen" title="Fullscreen (F)" onclick={() => void toggleFullscreen()}>F</button>
  </div>

  <button class="lightbox-nav-arrow prev" type="button" title="Previous (←)" aria-label="Previous file" onclick={onPrev}><Icon name="chev_left" size={20} /></button>
  <button class="lightbox-nav-arrow next" type="button" title="Next (→)" aria-label="Next file" onclick={onNext}><Icon name="chev_right" size={20} /></button>
</div>

<style>
  :global(.viewer-stage) {
    position: relative;
    overflow: hidden;
  }

  :global(.viewer-stage:fullscreen) {
    width: 100vw;
    height: 100vh;
    background: #000;
  }

  :global(.viewer-stage .viewer-visual-media) {
    object-fit: contain;
    transition: width 120ms ease, height 120ms ease, transform 120ms ease, filter 120ms ease, opacity 120ms ease;
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
