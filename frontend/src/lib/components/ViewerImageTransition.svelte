<script lang="ts">
  let {
    source,
    style,
    alt,
    onload
  } = $props<{
    source: string;
    style: string;
    alt: string;
    onload?: (event: Event) => void;
  }>();

  // Keep one real image element mounted across navigation. Browsers can retain the
  // already-painted bitmap while a replacement src loads; replacing that element
  // with a newly-created "outgoing" clone loses that guarantee and can itself flash
  // while the clone is decoded/presented.
  let currentSource = $state(source);
  let currentStyle = $state(style);
  let currentAlt = $state(alt);
  let pendingStyle = style;
  let pendingAlt = alt;
  let loadingTarget = false;

  $effect(() => {
    const nextSource = source;
    const nextStyle = style;
    const nextAlt = alt;

    if (currentSource !== nextSource) {
      // Start the next request immediately, but freeze the currently presented
      // geometry until that same DOM node reports the replacement source loaded.
      // This keeps old pixels at old dimensions without cloning the painted node.
      currentSource = nextSource;
      pendingStyle = nextStyle;
      pendingAlt = nextAlt;
      loadingTarget = true;
      return;
    }

    if (loadingTarget) {
      pendingStyle = nextStyle;
      pendingAlt = nextAlt;
    } else {
      currentStyle = nextStyle;
      currentAlt = nextAlt;
    }
  });

  function handleLoad(event: Event) {
    const image = event.currentTarget;
    if (!(image instanceof HTMLImageElement)) return;
    const loadedSource = image.getAttribute('src');
    if (loadedSource !== currentSource) return;

    onload?.(event);

    // Parent geometry may reconcile from naturalWidth/naturalHeight in `onload`.
    // Commit the latest target style in a microtask so those reactive updates can
    // feed `pendingStyle` before the browser paints the newly loaded pixels.
    queueMicrotask(() => {
      if (image.getAttribute('src') !== currentSource) return;
      loadingTarget = false;
      currentStyle = pendingStyle;
      currentAlt = pendingAlt;
    });
  }
</script>

<img
  class="viewer-visual-media viewer-incoming-media"
  style={currentStyle}
  src={currentSource}
  alt={currentAlt}
  onload={handleLoad}
/>
