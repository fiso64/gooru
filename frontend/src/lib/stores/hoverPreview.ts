import { writable } from 'svelte/store';

// Grid cards share one active preview so hovering a new eligible card immediately
// tears down playback in the previous card. Keeping this ephemeral avoids browser
// persistence and remains independent of protected-mode storage details.
export const activeHoverPreviewID = writable('');
