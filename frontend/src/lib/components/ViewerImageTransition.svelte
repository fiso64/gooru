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

  // These intentionally capture the initial props. Subsequent prop changes are
  // reconciled by the effect below so source transitions can snapshot the last
  // presented image before installing the next target.
  let currentSource = $state(source);
  let currentStyle = $state(style);
  let currentAlt = $state(alt);
  let outgoing = $state<{ source: string; style: string } | null>(null);
  let releaseGeneration = 0;

  $effect(() => {
    const nextSource = source;
    const nextStyle = style;
    const nextAlt = alt;

    if (currentSource === nextSource) {
      currentStyle = nextStyle;
      currentAlt = nextAlt;
      return;
    }

    // Invalidate a pending outgoing-layer release from an older target. Rapid
    // navigation should keep the last fully presented pixels until the newest
    // incoming image has itself reached a painted frame.
    releaseGeneration += 1;
    if (!outgoing && currentSource) outgoing = { source: currentSource, style: currentStyle };
    currentSource = nextSource;
    currentStyle = nextStyle;
    currentAlt = nextAlt;
  });

  function handleLoad(event: Event) {
    const image = event.currentTarget;
    const loadedSource = image instanceof HTMLImageElement ? image.getAttribute('src') : null;
    onload?.(event);

    // `load` means the image data is available, but it can fire before the browser
    // has presented those pixels. Removing the backing layer synchronously can
    // therefore expose a blank frame. Keep it through one paint and release it on
    // the following frame; stale callbacks are ignored if navigation moved again.
    const generation = ++releaseGeneration;
    requestAnimationFrame(() => {
      if (generation !== releaseGeneration || loadedSource !== currentSource) return;
      requestAnimationFrame(() => {
        if (generation !== releaseGeneration || loadedSource !== currentSource) return;
        outgoing = null;
      });
    });
  }
</script>

{#if outgoing}
  <img
    class="viewer-outgoing-media"
    style={outgoing.style}
    src={outgoing.source}
    alt=""
    aria-hidden="true"
  />
{/if}

{#key currentSource}
  <img
    class="viewer-visual-media viewer-incoming-media"
    style={currentStyle}
    src={currentSource}
    alt={currentAlt}
    onload={handleLoad}
  />
{/key}

<style>
  :global(.viewer-stage .viewer-outgoing-media) {
    z-index: 0;
    object-fit: contain;
    pointer-events: none;
    transition: transform 120ms ease, filter 120ms ease, opacity 120ms ease;
  }

  :global(.viewer-stage .viewer-incoming-media) {
    z-index: 1;
  }
</style>
