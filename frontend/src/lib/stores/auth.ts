import { browser } from '$app/environment';
import { writable } from 'svelte/store';

const storageKey = 'gooru.authToken';
const mediaCookieName = 'gooru_auth';
const initialToken = browser ? window.localStorage.getItem(storageKey) ?? '' : '';

export const authToken = writable(initialToken);

if (browser) {
  authToken.subscribe((value) => {
    if (value) {
      window.localStorage.setItem(storageKey, value);
      document.cookie = `${mediaCookieName}=${encodeURIComponent(value)}; Path=/api/v1/files/; SameSite=Strict${window.location.protocol === 'https:' ? '; Secure' : ''}`;
    } else {
      window.localStorage.removeItem(storageKey);
      document.cookie = `${mediaCookieName}=; Path=/api/v1/files/; Max-Age=0; SameSite=Strict`;
    }
  });
}
