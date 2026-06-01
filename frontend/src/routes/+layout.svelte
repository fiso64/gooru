<script lang="ts">
  import { QueryClient, QueryClientProvider } from '@tanstack/svelte-query';
  import { onMount } from 'svelte';
  import { setUnauthorizedHandler } from '$lib/api/client';
  import { authState } from '$lib/stores/auth';
  import '../app.css';

  let { children } = $props();
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: {
        staleTime: 10_000,
        refetchOnWindowFocus: false
      }
    }
  });

  onMount(() => {
    setUnauthorizedHandler(() => {
      authState.set({ user: null, csrfToken: '', checked: true });
      queryClient.clear();
    });
    return () => setUnauthorizedHandler(undefined);
  });
</script>

<QueryClientProvider client={queryClient}>
  {@render children()}
</QueryClientProvider>
