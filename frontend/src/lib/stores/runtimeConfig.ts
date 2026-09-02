import { writable } from 'svelte/store';

export type RuntimeConfig = {
  accentColor: string;
  loadFullMediaByDefault: boolean;
};

export const runtimeConfig = writable<RuntimeConfig>({
  accentColor: '',
  loadFullMediaByDefault: false
});
