<script lang="ts">
  import '@fontsource/comic-neue/400.css';
  import '@fontsource/comic-neue/700.css';
  import { QueryClient, QueryClientProvider } from '@tanstack/svelte-query';
  import { onMount } from 'svelte';
  import { setUnauthorizedHandler } from '$lib/api/client';
  import { subscribeJobsEvents } from '$lib/jobsEvents';
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
    let stopJobsEvents: (() => void) | undefined;
    const unsubscribeAuth = authState.subscribe((state) => {
      stopJobsEvents?.();
      stopJobsEvents = undefined;
      if (state.user?.role !== 'admin') return;
      stopJobsEvents = subscribeJobsEvents(() => queryClient.invalidateQueries({ queryKey: ['jobs'] }));
    });
    setUnauthorizedHandler(() => {
      authState.set({ user: null, csrfToken: '', checked: true });
      queryClient.clear();
    });
    return () => {
      stopJobsEvents?.();
      unsubscribeAuth();
      setUnauthorizedHandler(undefined);
    };
  });
</script>

<QueryClientProvider client={queryClient}>
  {@render children()}
</QueryClientProvider>
