<script lang="ts">
  import type { Snippet } from 'svelte';

  let {
    auth,
    sidebar,
    children
  } = $props<{
    auth: Snippet;
    sidebar: Snippet;
    children: Snippet;
  }>();
</script>

<main class="min-h-screen text-zinc-100">
  <div class="mx-auto flex min-h-screen w-full max-w-[1500px] flex-col px-4 py-4 sm:px-6 lg:px-8">
    <header class="flex flex-col gap-4 border-b border-white/10 pb-4 lg:flex-row lg:items-end lg:justify-between">
      <div class="min-w-0">
        <p class="text-xs font-semibold uppercase text-emerald-300/80">Gooru</p>
        <h1 class="mt-2 text-3xl font-semibold text-white sm:text-4xl">Library</h1>
      </div>
      {@render auth()}
    </header>

    <section class="grid flex-1 gap-4 py-4 lg:grid-cols-[18rem_minmax(0,1fr)]">
      <aside class="border-b border-white/10 pb-4 lg:border-b-0 lg:border-r lg:pr-4">
        {@render sidebar()}
      </aside>
      <section class="min-w-0">
        {@render children()}
      </section>
    </section>
  </div>
</main>
