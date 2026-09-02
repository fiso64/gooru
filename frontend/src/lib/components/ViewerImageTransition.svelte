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
  let presentedSource = $state('');
  let presentedStyle = $state('');

  $effect(() => {
    const nextSource = source;
    const nextStyle = style;

    // Before the first successful load there are no old pixels to preserve, so let
    // geometry follow the parent normally. After a load, freeze that presented
    // geometry whenever a different source is in flight on the same DOM node.
    if (!presentedSource || presentedSource === nextSource) {
      presentedStyle = nextStyle;
    }
  });

  function handleLoad(event: Event) {
    const image = event.currentTarget;
    if (!(image instanceof HTMLImageElement)) return;
    const loadedSource = image.getAttribute('src') ?? '';
    if (loadedSource !== source) return;

    onload?.(event);
    presentedSource = loadedSource;
  }
</script>

<img
  class="viewer-visual-media viewer-incoming-media"
  style={presentedStyle}
  src={source}
  {alt}
  onload={handleLoad}
/>
