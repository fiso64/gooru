import { onMount } from 'svelte';

export function createViewportState() {
  let height = $state(900);
  let width = $state(1200);
  let scrollY = $state(0);

  onMount(() => {
    let frame = 0;
    const update = () => {
      height = window.innerHeight;
      width = window.innerWidth;
      scrollY = window.scrollY;
    };
    const schedule = () => {
      if (frame) return;
      frame = requestAnimationFrame(() => {
        frame = 0;
        update();
      });
    };
    update();
    window.addEventListener('resize', schedule);
    window.addEventListener('scroll', schedule, { passive: true });
    return () => {
      if (frame) cancelAnimationFrame(frame);
      window.removeEventListener('resize', schedule);
      window.removeEventListener('scroll', schedule);
    };
  });

  return {
    get height() { return height; },
    get width() { return width; },
    get scrollY() { return scrollY; }
  };
}
