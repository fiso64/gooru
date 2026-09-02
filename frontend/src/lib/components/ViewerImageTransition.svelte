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

  type Slot = 0 | 1;

  // Keep two stable image elements and alternate them. The currently presented
  // element is never mutated while its replacement loads; the inactive slot loads
  // and paints behind it, then becomes active on the next animation frame.
  let initialized = $state(false);
  let activeSlot = $state<Slot>(0);
  let pendingSlot = $state<Slot | null>(null);
  let sourceA = $state('');
  let sourceB = $state('');
  let styleA = $state('');
  let styleB = $state('');
  let altA = $state('');
  let altB = $state('');
  let promotionGeneration = 0;

  function slotSource(slot: Slot) {
    return slot === 0 ? sourceA : sourceB;
  }

  function setSlot(slot: Slot, nextSource: string, nextStyle: string, nextAlt: string) {
    if (slot === 0) {
      sourceA = nextSource;
      styleA = nextStyle;
      altA = nextAlt;
    } else {
      sourceB = nextSource;
      styleB = nextStyle;
      altB = nextAlt;
    }
  }

  function updateSlotPresentation(slot: Slot, nextStyle: string, nextAlt: string) {
    if (slot === 0) {
      styleA = nextStyle;
      altA = nextAlt;
    } else {
      styleB = nextStyle;
      altB = nextAlt;
    }
  }

  $effect(() => {
    const nextSource = source;
    const nextStyle = style;
    const nextAlt = alt;

    if (!initialized) {
      setSlot(0, nextSource, nextStyle, nextAlt);
      initialized = true;
      return;
    }

    const activeSource = slotSource(activeSlot);
    if (nextSource === activeSource && pendingSlot == null) {
      updateSlotPresentation(activeSlot, nextStyle, nextAlt);
      return;
    }

    if (pendingSlot != null && nextSource === slotSource(pendingSlot)) {
      updateSlotPresentation(pendingSlot, nextStyle, nextAlt);
      return;
    }

    // Rapid navigation reuses the inactive slot for the newest target while the
    // active, already-painted slot stays untouched.
    promotionGeneration += 1;
    const targetSlot: Slot = activeSlot === 0 ? 1 : 0;
    setSlot(targetSlot, nextSource, nextStyle, nextAlt);
    pendingSlot = targetSlot;
  });

  function handleLoad(slot: Slot, event: Event) {
    const image = event.currentTarget;
    if (!(image instanceof HTMLImageElement)) return;
    const loadedSource = image.getAttribute('src') ?? '';

    // The first slot is already the presented image; its load only synchronizes
    // intrinsic geometry. Later loads promote only the newest requested source.
    if (pendingSlot == null) {
      if (slot === activeSlot && loadedSource === source) onload?.(event);
      return;
    }
    if (slot !== pendingSlot || loadedSource !== source || loadedSource !== slotSource(slot)) return;

    onload?.(event);
    const generation = ++promotionGeneration;
    requestAnimationFrame(() => {
      if (generation !== promotionGeneration || pendingSlot !== slot || source !== loadedSource) return;
      activeSlot = slot;
      pendingSlot = null;
    });
  }
</script>

{#if sourceA}
  <img
    class="viewer-image-slot"
    class:viewer-visual-media={activeSlot === 0}
    class:viewer-incoming-media={activeSlot === 0}
    class:viewer-buffer-media={activeSlot !== 0}
    style={`${styleA};z-index:${activeSlot === 0 ? 1 : 0}`}
    src={sourceA}
    alt={activeSlot === 0 ? altA : ''}
    aria-hidden={activeSlot === 0 ? undefined : 'true'}
    onload={(event) => handleLoad(0, event)}
  />
{/if}

{#if sourceB}
  <img
    class="viewer-image-slot"
    class:viewer-visual-media={activeSlot === 1}
    class:viewer-incoming-media={activeSlot === 1}
    class:viewer-buffer-media={activeSlot !== 1}
    style={`${styleB};z-index:${activeSlot === 1 ? 1 : 0}`}
    src={sourceB}
    alt={activeSlot === 1 ? altB : ''}
    aria-hidden={activeSlot === 1 ? undefined : 'true'}
    onload={(event) => handleLoad(1, event)}
  />
{/if}

<style>
  :global(.viewer-stage .viewer-image-slot) {
    object-fit: contain;
    pointer-events: none;
  }
</style>
