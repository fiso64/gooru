<script lang="ts">
  let { size = 17 } = $props<{ size?: number }>();

  const params = {
    turns: 2.65,
    angle: 320,
    stroke: 1.7,
    startR: 1.2,
    endR: 9,
    oScale: 1.5
  };

  const baseCell = $derived(size * 0.62);
  const oSize = $derived(Math.round(baseCell * params.oScale));
  const sideAdjust = $derived(Math.round((params.oScale - 1) * baseCell * 0.18));
  const r = params.endR + 1;
  const viewBox = `${-r} ${-r} ${2 * r} ${2 * r}`;
  const valign = $derived(size * 0.25 - oSize / 2);

  function spiralPhase() {
    const endAngle = (params.angle * Math.PI) / 180;
    return endAngle - params.turns * 2 * Math.PI;
  }

  function spiralPath() {
    const points = 120;
    const out: string[] = [];
    const phase = spiralPhase();
    for (let i = 0; i <= points; i += 1) {
      const t = i / points;
      const theta = phase + params.turns * 2 * Math.PI * t;
      const radius = params.startR + (params.endR - params.startR) * t;
      const x = radius * Math.cos(theta);
      const y = radius * Math.sin(theta);
      out.push(`${i ? 'L' : 'M'} ${x.toFixed(2)} ${y.toFixed(2)}`);
    }
    return out.join(' ');
  }
</script>

<span class="gooru-logo" aria-label="gooru" style={`font-size: ${size}px;`}>
  <span style={sideAdjust ? `margin-right: ${sideAdjust}px;` : ''}>g</span>
  {#each [0, 1] as _}
    <svg
      width={oSize}
      height={oSize}
      {viewBox}
      aria-hidden="true"
      style={`vertical-align: ${valign.toFixed(2)}px;`}
    >
      <path d={spiralPath()} />
    </svg>
  {/each}
  <span style={sideAdjust ? `margin-left: ${sideAdjust}px;` : ''}>ru</span>
</span>

<style>
  .gooru-logo {
    display: inline-block;
    font-family: var(--font-mono);
    font-weight: 600;
    line-height: 1;
    letter-spacing: 0;
    color: var(--text);
    white-space: nowrap;
  }

  svg {
    display: inline-block;
    color: var(--accent);
  }

  path {
    fill: none;
    stroke: currentColor;
    stroke-width: 1.7;
    stroke-linecap: round;
    stroke-linejoin: round;
  }
</style>
