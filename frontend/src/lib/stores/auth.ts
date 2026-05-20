import { browser } from '$app/environment';
import { writable } from 'svelte/store';

const storageKey = 'gooru.authToken';
const initialToken = browser ? window.localStorage.getItem(storageKey) ?? '' : '';

export const authToken = writable(initialToken);

if (browser) {
  authToken.subscribe((value) => {
    if (value) {
      window.localStorage.setItem(storageKey, value);
    } else {
      window.localStorage.removeItem(storageKey);
    }
  });
}
