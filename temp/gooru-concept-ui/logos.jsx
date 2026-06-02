import React from 'react';

// Gooru logo proposals.
//
// Each variant is rendered from live params so they can be tweaked in real time
// (turns, stroke width, start/end radius, eye separation). Paths are computed
// inside React.useMemo so re-renders are cheap.

// ── math helpers ─────────────────────────────────────────────────────────────

// Archimedean spiral: r grows linearly with θ. Smooth, even spacing between turns.
function archimedean(cx, cy, startR, endR, turns, points = 96, dir = 1, phase = 0) {
  const out = [];
  const totalTheta = turns * 2 * Math.PI;
  for (let i = 0; i <= points; i++) {
    const t = i / points;
    const theta = phase + dir * totalTheta * t;
    const r = startR + (endR - startR) * t;
    out.push([cx + r * Math.cos(theta), cy + r * Math.sin(theta)]);
  }
  return out;
}

// Logarithmic spiral. b is solved so spiral hits endR at the final turn.
function golden(cx, cy, startR, endR, turns, points = 96, dir = 1, phase = 0) {
  const out = [];
  const totalTheta = turns * 2 * Math.PI;
  const b = Math.log(endR / Math.max(0.0001, startR)) / totalTheta;
  for (let i = 0; i <= points; i++) {
    const t = i / points;
    const theta = phase + dir * totalTheta * t;
    const r = startR * Math.exp(b * (theta - phase) * dir);
    out.push([cx + r * Math.cos(theta), cy + r * Math.sin(theta)]);
  }
  return out;
}

function toPath(pts) {
  return pts.map((p, i) => (i ? 'L ' : 'M ') + p[0].toFixed(2) + ' ' + p[1].toFixed(2)).join(' ');
}

// Square spiral. Adapts the number of inward steps to a "turns" param so the
// labyrinth tweak still feels related to the curved variants.
function laidLabyrinth(turns) {
  // Each "turn" adds a pair of corners. Clamp so it stays inside the 24x24 box.
  const t = Math.max(0.5, Math.min(4, turns));
  const steps = Math.round(t * 2);            // ~2 corners per turn
  const pad = 3;                              // margin from edges
  const size = 24 - pad * 2;                  // usable side
  const inset = size / (steps * 2 + 1);       // how much each pass shrinks
  let x = pad, y = pad, w = size, h = size;
  const pts = [[x, y]];
  for (let i = 0; i < steps; i++) {
    // right, down, left (partial), up (partial — shrunk)
    pts.push([x + w, y]);
    pts.push([x + w, y + h]);
    pts.push([x + inset, y + h]);
    pts.push([x + inset, y + inset * 2]);
    x += inset * 2; y += inset * 2;
    w -= inset * 3; h -= inset * 3;
    if (w <= 0 || h <= 0) break;
  }
  return pts.map((p, i) => (i ? 'L ' : 'M ') + p[0].toFixed(2) + ' ' + p[1].toFixed(2)).join(' ');
}

// ── Logo variants ────────────────────────────────────────────────────────────

const LOGO_VARIANTS = [
  {key: 'swirl',    label: 'A. Swirl',    blurb: 'Clean Archimedean spiral. Quiet and modern.'},
  {key: 'target',   label: 'G. Target',   blurb: 'Hypnotic rings. Variable ring count via Turns.'},
  {key: 'disc',     label: 'H. Disc',     blurb: 'Current placeholder, for comparison.'},
  {key: 'wordmark', label: 'I. Wordmark', blurb: 'No standalone mark — the two O’s in “gooru” become spirals. Compact, signed.'},
];

// Variants where the Logo IS the full wordmark and the adjacent "gooru" label
// should be suppressed (in topbar, login, LogoCard…).
const LOGO_WORDMARK_VARIANTS = new Set(['wordmark']);
function logoIsWordmark(variant) {
  return LOGO_WORDMARK_VARIANTS.has(variant);
}

// Default params — match the original baked-in values so unchanged behavior is preserved.
const LOGO_PARAM_DEFAULTS = {
  turns: 1.85,
  stroke: 1.7,
  startR: 1.2,
  endR: 9.0,
  eyeGap: 5.2,
  oScale: 1.0,
  // Angle of the spiral's outer tail, in degrees. Chosen so the original
  // default (phase=0, turns=1.85) looks identical when angle is at its default.
  angle: 306,
};

// Sanitize numeric params (clamp + fallback to defaults).
function normalizeLogoParams(p = {}) {
  const d = LOGO_PARAM_DEFAULTS;
  const f = (v, fb) => (typeof v === 'number' && isFinite(v) ? v : fb);
  return {
    turns:  Math.max(0.3, Math.min(4.5, f(p.turns,  d.turns))),
    stroke: Math.max(0.3, Math.min(5.0, f(p.stroke, d.stroke))),
    startR: Math.max(0.1, Math.min(5.0, f(p.startR, d.startR))),
    endR:   Math.max(3.0, Math.min(11.5, f(p.endR,  d.endR))),
    eyeGap: Math.max(2.5, Math.min(7.5,  f(p.eyeGap,d.eyeGap))),
    oScale: Math.max(0.4, Math.min(2.6,  f(p.oScale,d.oScale))),
    angle:  ((f(p.angle, d.angle) % 360) + 360) % 360,
  };
}

// Phase for the archimedean spiral so that its outer tail (t=1) lands at the
// requested angle regardless of turn count. Decouples "how many loops" from
// "which way does the tail point".
function spiralPhase(params, dir = 1) {
  const endAngle = params.angle * Math.PI / 180;
  return endAngle - dir * params.turns * 2 * Math.PI;
}

function LogoInner({variant, params}) {
  const p = params;
  // Common path props
  const strokeProps = {
    fill: 'none',
    stroke: 'currentColor',
    strokeWidth: p.stroke,
    strokeLinecap: 'round',
    strokeLinejoin: 'round',
  };

  switch (variant) {
    case 'swirl':
      return <path d={toPath(archimedean(12, 12, p.startR, p.endR, p.turns, 120, 1, spiralPhase(p)))} {...strokeProps}/>;

    case 'swirl-bold':
      // Same shape, heavier stroke — multiplier so the user's stroke slider still has effect.
      return <path d={toPath(archimedean(12, 12, p.startR, p.endR, p.turns, 120))}
                   {...strokeProps} strokeWidth={p.stroke * 1.65}/>;

    case 'nautilus':
      return <path d={toPath(golden(12, 12, Math.max(0.5, p.startR), p.endR, p.turns, 120, 1, -Math.PI / 2))}
                   {...strokeProps}/>;

    case 'labyrinth':
      return <path d={laidLabyrinth(p.turns)} fill="none" stroke="currentColor"
                   strokeWidth={p.stroke * 1.1} strokeLinecap="square" strokeLinejoin="miter"/>;

    case 'eyes': {
      const lx = 12 - p.eyeGap, rx = 12 + p.eyeGap;
      // Scale eye size from end radius (full spiral) — clamp so it doesn't overflow the gap.
      const eyeEnd = Math.min(p.endR * 0.48, p.eyeGap - 0.3);
      return (
        <>
          <path d={toPath(archimedean(lx, 12, 0.4, eyeEnd, p.turns, 96, 1))} {...strokeProps}/>
          <path d={toPath(archimedean(rx, 12, 0.4, eyeEnd, p.turns, 96, 1))} {...strokeProps}/>
        </>);
    }
    case 'eyes-mirror': {
      const lx = 12 - p.eyeGap, rx = 12 + p.eyeGap;
      const eyeEnd = Math.min(p.endR * 0.48, p.eyeGap - 0.3);
      return (
        <>
          <path d={toPath(archimedean(lx, 12, 0.4, eyeEnd, p.turns, 96, 1))} {...strokeProps}/>
          <path d={toPath(archimedean(rx, 12, 0.4, eyeEnd, p.turns, 96, -1))} {...strokeProps}/>
        </>);
    }
    case 'eyes-asym': {
      const lx = 12 - p.eyeGap * 0.82, rx = 12 + p.eyeGap * 1.02;
      const lEnd = Math.min(p.endR * 0.30, p.eyeGap - 0.8);
      const rEnd = Math.min(p.endR * 0.52, p.eyeGap - 0.1);
      return (
        <>
          <path d={toPath(archimedean(lx, 12, 0.5, lEnd, p.turns * 0.75, 96, 1))} {...strokeProps}/>
          <path d={toPath(archimedean(rx, 12, 0.4, rEnd, p.turns,        96, 1))} {...strokeProps}/>
        </>);
    }
    case 'eyes-wink': {
      const lx = 12 - p.eyeGap, rx = 12 + p.eyeGap;
      const eyeEnd = Math.min(p.endR * 0.48, p.eyeGap - 0.3);
      // closed-eye arc: a smile-curve at the same height with width scaled by eyeGap
      const w = eyeEnd * 1.25;
      return (
        <>
          <path d={toPath(archimedean(lx, 12, 0.4, eyeEnd, p.turns, 96, 1))} {...strokeProps}/>
          <path d={`M ${rx - w} 12 Q ${rx} ${12 + w * 0.55} ${rx + w} 12`} {...strokeProps} strokeLinejoin="round"/>
        </>);
    }
    case 'glasses': {
      const lx = 12 - p.eyeGap, rx = 12 + p.eyeGap;
      const lensR = Math.min(p.eyeGap - 0.3, p.endR * 0.58);
      const bridgeStart = lx + lensR;
      const bridgeEnd = rx - lensR;
      return (
        <>
          <circle cx={lx} cy="12" r={lensR} fill="none" stroke="currentColor" strokeWidth={p.stroke * 0.85}/>
          <circle cx={rx} cy="12" r={lensR} fill="none" stroke="currentColor" strokeWidth={p.stroke * 0.85}/>
          {bridgeEnd > bridgeStart && (
            <path d={`M ${bridgeStart} 12 L ${bridgeEnd} 12`} stroke="currentColor"
                  strokeWidth={p.stroke * 0.85} strokeLinecap="round"/>
          )}
          <path d={toPath(archimedean(lx, 12, 0.4, lensR * 0.78, p.turns, 96, 1))}
                {...strokeProps} strokeWidth={p.stroke * 0.7} opacity="0.85"/>
          <path d={toPath(archimedean(rx, 12, 0.4, lensR * 0.78, p.turns, 96, 1))}
                {...strokeProps} strokeWidth={p.stroke * 0.7} opacity="0.85"/>
        </>);
    }
    case 'target': {
      // Number of rings tracks the turns slider.
      const rings = Math.max(1, Math.round(p.turns * 1.5));
      const outerR = Math.min(p.endR + 0.5, 10.5);
      const innerR = Math.max(0.6, p.startR);
      const r = (i) => innerR + (outerR - innerR) * (i / Math.max(1, rings - 1));
      return (
        <>
          {Array.from({length: rings}).map((_, i) => (
            <circle key={i} cx="12" cy="12" r={r(rings - 1 - i)}
                    fill="none" stroke="currentColor" strokeWidth={p.stroke * 0.9}/>
          ))}
          <circle cx="12" cy="12" r={Math.max(0.4, p.stroke * 0.55)} fill="currentColor"/>
        </>);
    }
    case 'disc':
    default:
      return (
        <>
          <circle cx="12" cy="12" r={p.endR} fill="none" stroke="currentColor" strokeWidth={p.stroke}/>
          <circle cx="12" cy="12" r={Math.max(1, p.endR * 0.5)} fill="currentColor"/>
        </>);
  }
}

function Logo({variant = 'swirl', size = 22, color,
               turns, stroke, startR, endR, eyeGap, oScale, angle}) {
  const params = React.useMemo(
    () => normalizeLogoParams({turns, stroke, startR, endR, eyeGap, oScale, angle}),
    [turns, stroke, startR, endR, eyeGap, oScale, angle]
  );
  if (variant === 'wordmark') {
    return <WordmarkLogo size={size} color={color} params={params}/>;
  }
  return (
    <svg
      width={size}
      height={size}
      viewBox="0 0 24 24"
      aria-hidden="true"
      style={color ? {color} : undefined}
    >
      <LogoInner variant={variant} params={params}/>
    </svg>
  );
}

// Wordmark: "g·◯·◯·r·u" where the two O's are spirals matching variant E1.
// `size` is the cap-height of the letters in px; the spirals scale to match.
function WordmarkLogo({size = 22, color, params}) {
  // Each spiral SVG nominally occupies one monospace character cell (~0.62em wide).
  // params.oScale > 1 grows the spirals beyond that cell; inline flow pushes the
  // surrounding "ru" outward and the spiral centers stay locked to x-height/2.
  const baseCell = size * 0.62;
  const oSize = Math.round(baseCell * params.oScale);
  // When the spirals are scaled away from default size, lend a sliver of negative
  // margin so "g" and "ru" snug in (smaller spirals) or breathe out (larger ones)
  // around the spiral pair, instead of leaving a hard butt-joint.
  const sideAdjust = Math.round((params.oScale - 1) * baseCell * 0.18);
  const pad = 1.0;
  const r = params.endR + pad;
  const viewBox = `${-r} ${-r} ${2 * r} ${2 * r}`;
  const spiralPath = React.useMemo(
    () => toPath(archimedean(0, 0, params.startR, params.endR, params.turns, 120, 1, spiralPhase(params))),
    [params.startR, params.endR, params.turns, params.angle]
  );
  // Drop the SVG so its center lands at half-x-height above the text baseline
  // (x-height ≈ 0.5em ⇒ center at +0.25em above baseline). Works at any oSize.
  const valign = (size * 0.25) - (oSize / 2);
  const spiral = (
    <svg
      width={oSize}
      height={oSize}
      viewBox={viewBox}
      aria-hidden="true"
      style={{
        display: 'inline-block',
        verticalAlign: `${valign.toFixed(2)}px`,
        color: color || 'var(--accent)',
      }}
    >
      <path d={spiralPath} fill="none" stroke="currentColor" strokeWidth={params.stroke}
            strokeLinecap="round" strokeLinejoin="round"/>
    </svg>
  );
  // "g" gets right-margin, "ru" gets left-margin so the pair stays optically
  // centered between them as the spirals scale.
  const gStyle = sideAdjust ? {marginRight: `${sideAdjust}px`} : undefined;
  const ruStyle = sideAdjust ? {marginLeft: `${sideAdjust}px`} : undefined;
  return (
    <span
      aria-label="gooru"
      style={{
        display: 'inline-block',
        fontFamily: 'var(--font-mono, ui-monospace, monospace)',
        fontWeight: 600,
        fontSize: size,
        lineHeight: 1,
        letterSpacing: '0',
        color: 'var(--text)',
        whiteSpace: 'nowrap',
      }}
    ><span style={gStyle}>g</span>{spiral}{spiral}<span style={ruStyle}>ru</span></span>
  );
}

Object.assign(window, {Logo, LOGO_VARIANTS, LogoInner, LOGO_PARAM_DEFAULTS, logoIsWordmark});
