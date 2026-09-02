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

  $effect(() => {
    const nextSource = source;
    const nextStyle = style;
    const nextAlt = alt;

    if (currentSource === nextSource) {
      currentStyle = nextStyle;
      currentAlt = nextAlt;
      return;
    }

    // Keep the last fully presented image at its own frozen geometry while a
    // new image enters the browser loading/progressive-decode pipeline. Rapid
    // navigation keeps that stable backing layer instead of chaining partially
    // loaded targets as new outgoing frames.
    if (!outgoing && currentSource) outgoing = { source: currentSource, style: currentStyle };
    currentSource = nextSource;
    currentStyle = nextStyle;
    currentAlt = nextAlt;
  });

  function handleLoad(event: Event) {
    outgoing = null;
    onload?.(event);
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
