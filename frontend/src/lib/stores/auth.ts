import { writable } from 'svelte/store';
import type { AuthUser } from '$lib/api/types';

export interface AuthState {
  user: AuthUser | null;
  csrfToken: string;
  checked: boolean;
}

export const authState = writable<AuthState>({
  user: null,
  csrfToken: '',
  checked: false
});
