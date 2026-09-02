<script lang="ts">
  let { size = 17 } = $props<{ size?: number }>();
</script>

<span
  class="gooru-logo"
  role="img"
  aria-label="gooru"
  style={`height: ${size}px;`}
>
  <img src="/gooru-logo.svg" alt="" aria-hidden="true" />
  <svg class="gooru-logo-accent" viewBox="7.57 -187.22 944.03 247.49" aria-hidden="true">
    <defs>
      <mask id="gooru-logo-spiral-mask" mask-type="alpha">
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
  .gooru-logo-accent {
    position: absolute;
    inset: 0;
    display: block;
    width: 100%;
    height: 100%;
  }

  .gooru-logo img {
    object-fit: contain;
  }

  .gooru-logo-accent-fill {
    fill: var(--accent);
  }
</style>
