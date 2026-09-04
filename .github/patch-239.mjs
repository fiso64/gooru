import fs from 'node:fs';

function edit(path, before, after) {
  const source = fs.readFileSync(path, 'utf8');
  if (!source.includes(before)) throw new Error(`missing expected snippet in ${path}`);
  fs.writeFileSync(path, source.replace(before, after));
}

edit(
  'frontend/src/lib/components/ViewerStage.svelte',
  '  <div class="transition-arrow" aria-hidden="true"></div>\n\n',
  ''
);

edit(
  'frontend/src/lib/styles/viewer.css',
`/* Directional transition accent copied structurally and numerically from the
   supplied concept. The arrowhead is the element's own ::after child so it
   inherits the exact parent transform/keyframes. */
.transition-arrow {
  position: absolute;
  z-index: 7;
  left: 50%;
  top: 50%;
  width: 138px;
  height: 1px;
  background: linear-gradient(90deg, transparent, rgba(246,222,119,.82), transparent);
  transform: translate(-50%,-50%) scaleX(.05);
  opacity: 0;
  pointer-events: none;
}
.transition-arrow::after {
  content: "";
  position: absolute;
  right: 18px;
  top: 50%;
  width: 8px;
  height: 8px;
  border-top: 1.5px solid var(--accent);
  border-right: 1.5px solid var(--accent);
  transform: translateY(-50%) rotate(45deg);
}

/* Gooru derives the real image geometry rather than imposing the concept's
   synthetic fixed frame, but preserves its 61/52 growth and 420ms easing. */
.viewer-stage.entering .viewer-pan-surface {
  animation: comic-frame-enter 420ms cubic-bezier(.2,.78,.22,1) both;
}
.viewer-stage.exiting .viewer-pan-surface {
  animation: comic-frame-exit 420ms cubic-bezier(.2,.78,.22,1) both;
}
.viewer-stage.comic-reading:not(.entering) .viewer-pan-surface {
  transform: scale(1.1730769231);
}

.viewer-stage.entering .transition-arrow {
  animation: arrow-through 460ms cubic-bezier(.2,.78,.22,1);
}
@keyframes arrow-through {
  0% { opacity: 0; transform: translate(-50%,-50%) scaleX(.05); }
  24% { opacity: .95; }
  65% { opacity: .8; transform: translate(-50%,-50%) scaleX(1); }
  100% { opacity: 0; transform: translate(18%,-50%) scaleX(.45); }
}

.viewer-stage.exiting .transition-arrow {
  transform: translate(-50%,-50%) rotate(180deg);
  animation: arrow-back 400ms cubic-bezier(.2,.78,.22,1);
}
@keyframes arrow-back {
  0% { opacity: 0; transform: translate(-50%,-50%) rotate(180deg) scaleX(.08); }
  30% { opacity: .9; }
  72% { opacity: .65; transform: translate(-50%,-50%) rotate(180deg) scaleX(.92); }
  100% { opacity: 0; transform: translate(-115%,-50%) rotate(180deg) scaleX(.38); }
}

@keyframes comic-frame-enter {
  from { transform: scale(1); }
  to { transform: scale(1.1730769231); }
}
@keyframes comic-frame-exit {
  from { transform: scale(1.1730769231); }
  to { transform: scale(1); }
}
`,
`/* Comic mode is communicated with a short settle animation only. The media
   always returns to its normal viewer geometry; reader mode must not leave a
   persistent zoom or directional decoration behind. */
.viewer-stage.entering .viewer-pan-surface {
  animation: comic-reader-enter 480ms cubic-bezier(.16,.84,.24,1) both;
}
.viewer-stage.exiting .viewer-pan-surface {
  animation: comic-reader-exit 420ms cubic-bezier(.32,.02,.24,1) both;
}

@keyframes comic-reader-enter {
  0% {
    transform: scale(.965);
    filter: brightness(.84) saturate(.82);
    opacity: .82;
  }
  58% {
    transform: scale(1.018);
    filter: brightness(1.06) saturate(1.04);
    opacity: 1;
  }
  100% {
    transform: scale(1);
    filter: none;
    opacity: 1;
  }
}

@keyframes comic-reader-exit {
  0% {
    transform: scale(1);
    filter: none;
    opacity: 1;
  }
  44% {
    transform: scale(.982);
    filter: brightness(.9) saturate(.88);
    opacity: .88;
  }
  100% {
    transform: scale(1);
    filter: none;
    opacity: 1;
  }
}
`
);

edit(
  'frontend/src/lib/styles/viewer.css',
`  .viewer-stage .viewer-pan-surface,
  .transition-arrow {
`,
`  .viewer-stage .viewer-pan-surface {
`
);

edit(
  'frontend/tests/comic-ui.spec.ts',
`  const transitionArrow = stage.locator('.transition-arrow');
  await expect(stage).toHaveClass(/entering/);
  await expect(transitionArrow).toHaveCount(1);
  await expect.poll(() => transitionArrow.evaluate((element) => getComputedStyle(element).animationName)).toBe('arrow-through');
  await expect.poll(() => transitionArrow.evaluate((element) => getComputedStyle(element).animationDuration)).toBe('0.46s');
  await expect.poll(() => transitionArrow.evaluate((element) => getComputedStyle(element, '::after').content)).toBe('""');
  await expect.poll(() => surface.evaluate((element) => getComputedStyle(element).animationName)).toBe('comic-frame-enter');
  await expect.poll(() => surface.evaluate((element) => getComputedStyle(element).animationDuration)).toBe('0.42s');
`,
`  await expect(stage).toHaveClass(/entering/);
  await expect(stage.locator('.transition-arrow')).toHaveCount(0);
  await expect.poll(() => surface.evaluate((element) => getComputedStyle(element).animationName)).toBe('comic-reader-enter');
  await expect.poll(() => surface.evaluate((element) => getComputedStyle(element).animationDuration)).toBe('0.48s');
  await expect.poll(() => stage.evaluate((element) => !element.classList.contains('entering'))).toBe(true);
  await expect(surface).toHaveCSS('transform', 'none');
`
);

edit(
  'frontend/tests/comic-ui.spec.ts',
`  await expect(stage).toHaveClass(/exiting/);
  await expect.poll(() => transitionArrow.evaluate((element) => getComputedStyle(element).animationName)).toBe('arrow-back');
  await expect.poll(() => transitionArrow.evaluate((element) => getComputedStyle(element).animationDuration)).toBe('0.4s');
  await expect.poll(() => surface.evaluate((element) => getComputedStyle(element).animationName)).toBe('comic-frame-exit');
`,
`  await expect(stage).toHaveClass(/exiting/);
  await expect.poll(() => surface.evaluate((element) => getComputedStyle(element).animationName)).toBe('comic-reader-exit');
  await expect.poll(() => surface.evaluate((element) => getComputedStyle(element).animationDuration)).toBe('0.42s');
  await expect.poll(() => stage.evaluate((element) => !element.classList.contains('exiting'))).toBe(true);
  await expect(surface).toHaveCSS('transform', 'none');
`
);

edit(
  'frontend/tests/comic-ui.spec.ts',
`  await expect(stage.locator('.viewer-pan-surface')).toHaveCSS('animation-duration', '0.001s');
  await expect(stage.locator('.transition-arrow')).toHaveCSS('animation-duration', '0.001s');
`,
`  await expect(stage.locator('.viewer-pan-surface')).toHaveCSS('animation-duration', '0.001s');
  await expect(stage.locator('.transition-arrow')).toHaveCount(0);
`
);
