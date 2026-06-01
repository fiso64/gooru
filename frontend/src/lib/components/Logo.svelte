<script lang="ts">
  let { size = 86 } = $props<{ size?: number }>();

  function spiral(cx: number, cy: number, dir = 1) {
    const turns = 2.65;
    const startR = 1.2;
    const endR = 10.5;
    const points = 120;
    const angle = 320 * Math.PI / 180;
    const phase = angle - dir * turns * 2 * Math.PI;
    const out: string[] = [];
    for (let i = 0; i <= points; i += 1) {
      const t = i / points;
      const theta = phase + dir * turns * 2 * Math.PI * t;
      const r = startR + (endR - startR) * t;
      const x = cx + r * Math.cos(theta);
      const y = cy + r * Math.sin(theta);
      out.push(`${i ? 'L' : 'M'} ${x.toFixed(2)} ${y.toFixed(2)}`);
    }
    return out.join(' ');
  }
</script>

<svg class="gooru-logo" width={size} height={Math.round(size * 0.34)} viewBox="0 0 152 48" aria-label="gooru">
  <text x="0" y="36" class="word">g</text>
  <path class="spiral" d={spiral(42, 24, 1)} />
  <path class="spiral" d={spiral(70, 24, 1)} />
  <text x="88" y="36" class="word">ru</text>
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
    font-size: 40px;
    font-style: italic;
  }

  .spiral {
    fill: none;
    stroke: currentColor;
    stroke-width: 1.7;
    stroke-linecap: round;
    stroke-linejoin: round;
  }
</style>
