<script lang="ts">
  let { size = 68 } = $props<{ size?: number }>();
</script>

<svg class="gooru-logo" width={size} height={size / 3.2} viewBox="0 0 150 46" aria-label="gooru">
  <text x="0" y="34" class="word">g</text>
  <g transform="translate(39 23)">
    <path d="M 1.20 0.00 L -2.83 -1.90 L 1.20 -4.97 L 5.02 1.68 L -3.86 6.19 L -7.08 -4.90 L 6.94 -8.66 L 9.15 7.96 L -9.94 10.00 L -11.15 -11.12 L 12.70 -11.14 L 12.88 14.95 L -15.87 11.77 L -12.93 -18.95 L 19.62 -11.86" />
  </g>
  <g transform="translate(71 23)">
    <path d="M 1.20 0.00 L -2.83 -1.90 L 1.20 -4.97 L 5.02 1.68 L -3.86 6.19 L -7.08 -4.90 L 6.94 -8.66 L 9.15 7.96 L -9.94 10.00 L -11.15 -11.12 L 12.70 -11.14 L 12.88 14.95 L -15.87 11.77 L -12.93 -18.95 L 19.62 -11.86" />
  </g>
  <text x="91" y="34" class="word">ru</text>
</svg>

<style>
  .gooru-logo {
    display: block;
    color: var(--accent);
    overflow: visible;
  }

  .word {
    fill: currentColor;
    font-family: var(--font-display);
    font-size: 38px;
    font-style: italic;
  }

  path {
    fill: none;
    stroke: currentColor;
    stroke-width: 1.7;
    stroke-linecap: round;
    stroke-linejoin: round;
  }
</style>
