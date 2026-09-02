import { writable } from 'svelte/store';

export type RuntimeConfig = {
  loadFullMediaByDefault: boolean;
};

export const runtimeConfig = writable<RuntimeConfig>({
  loadFullMediaByDefault: false
});
