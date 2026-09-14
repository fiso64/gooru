<script lang="ts">
  let { size = 17 } = $props<{ size?: number }>();
</script>

<span
  class="gooru-logo"
  role="img"
  aria-label="gooru"
  style={`height: ${size}px; --gooru-logo-size: ${size}px;`}
>
  <img class="gooru-logo-default" src="/gooru-logo.svg" alt="" aria-hidden="true" />
  <svg class="gooru-logo-comic" viewBox="7.57 -187.22 944.03 247.49" aria-hidden="true">
    <text class="gooru-logo-comic-letter gooru-logo-comic-g" x="8" y="9">g</text>
    <text class="gooru-logo-comic-letter gooru-logo-comic-r" x="660" y="9">r</text>
    <text class="gooru-logo-comic-letter gooru-logo-comic-u" x="805" y="9">u</text>
  </svg>
  <svg class="gooru-logo-accent" viewBox="7.57 -187.22 944.03 247.49" aria-hidden="true">
    <defs>
      <mask id="gooru-logo-spiral-mask" style="mask-type: alpha;">
        <image href="/favicon.svg" x="173.55" y="-185.975" width="223.125" height="226.1" />
        <image href="/favicon.svg" x="411.55" y="-185.975" width="223.125" height="226.1" />
      </mask>
    </defs>
    <rect
      class="gooru-logo-accent-fill"
      x="7.57"
      y="-187.22"
      width="944.03"
      height="247.49"
      mask="url(#gooru-logo-spiral-mask)"
    />
  </svg>
</span>

<style>
  .gooru-logo {
    position: relative;
    display: block;
    width: auto;
    aspect-ratio: 944.03 / 247.49;
  }

  .gooru-logo img,
  .gooru-logo-comic,
  .gooru-logo-accent {
    position: absolute;
    inset: 0;
    width: 100%;
    height: 100%;
  }

  .gooru-logo-default,
  .gooru-logo-accent {
    display: block;
  }

  .gooru-logo img {
    object-fit: contain;
  }

  .gooru-logo-comic {
    display: none;
    overflow: visible;
  }

  .gooru-logo-comic-letter {
    fill: var(--text);
    font-family: var(--font-display);
    font-size: 280px;
    font-weight: var(--font-display-weight);
  }

  .gooru-logo-accent-fill {
    fill: var(--brand-accent, var(--accent));
  }

  :global(.gooru-type-comic) .gooru-logo-default {
    display: none;
  }

  :global(.gooru-type-comic) .gooru-logo-comic {
    display: block;
  }
</style>
