<script lang="ts">
  import { paginationWindow } from '$lib/utils/pagination';
  import { isEditableShortcutTarget } from '$lib/utils/keyboard';

  let {
    page,
    pageCount,
    onPage,
    ariaLabel = 'Pages',
    disabled = false,
    embedded = false,
    testId
  } = $props<{
    page: number;
    pageCount: number;
    onPage: (page: number) => void;
    ariaLabel?: string;
    disabled?: boolean;
    embedded?: boolean;
    testId?: string;
  }>();

  let navElement: HTMLElement | undefined;

  function selectPage(nextPage: number) {
    if (disabled || nextPage < 1 || nextPage > pageCount || nextPage === page) return;
    onPage(nextPage);
  }

  function shortcutOwner(target: EventTarget | null) {
    if (target instanceof Element) {
      const targetNav = target.closest<HTMLElement>('.page-nav');
      if (targetNav) return targetNav;
    }

    const viewportCenter = window.innerHeight / 2;
    let owner: HTMLElement | undefined;
    let ownerVisible = false;
    let ownerDistance = Number.POSITIVE_INFINITY;

    for (const nav of document.querySelectorAll<HTMLElement>('.page-nav')) {
      const rect = nav.getBoundingClientRect();
      if (rect.width <= 0 || rect.height <= 0) continue;
      const visible = rect.bottom > 0 && rect.top < window.innerHeight;
      const distance = Math.abs((rect.top + rect.bottom) / 2 - viewportCenter);
      if (!owner || (visible && !ownerVisible) || (visible === ownerVisible && distance < ownerDistance)) {
        owner = nav;
        ownerVisible = visible;
        ownerDistance = distance;
      }
    }
    return owner;
  }

  function handlePageShortcut(event: KeyboardEvent) {
    if (event.defaultPrevented || !event.shiftKey || event.altKey || event.ctrlKey || event.metaKey) return;
    if (event.key !== 'PageUp' && event.key !== 'PageDown') return;
    if (isEditableShortcutTarget(event.target)) return;
    if (document.querySelector('[role="dialog"][aria-modal="true"]')) return;
    if (!navElement || shortcutOwner(event.target) !== navElement) return;

    event.preventDefault();
    event.stopPropagation();
    selectPage(page + (event.key === 'PageDown' ? 1 : -1));
  }
</script>

<svelte:window onkeydown={handlePageShortcut} />

<nav bind:this={navElement} class:embedded class="page-nav" aria-label={ariaLabel} data-testid={testId}>
  <button
    class="g-btn g-btn-sm page-nav-step"
    type="button"
    aria-label="Previous page"
    title="Previous page"
    disabled={disabled || page <= 1}
    onclick={() => selectPage(page - 1)}
  >‹</button>
  <div class="page-nav-pages">
    {#each paginationWindow(page, pageCount) as control, index (`${control}-${index}`)}
      {#if control === 'ellipsis'}
        <span class="page-nav-ellipsis" aria-hidden="true">…</span>
      {:else}
        <button
          class="g-btn g-btn-sm page-nav-button"
          type="button"
          aria-label={`Page ${control}`}
          aria-current={control === page ? 'page' : undefined}
          {disabled}
          onclick={() => selectPage(control)}
        >{control.toLocaleString()}</button>
      {/if}
    {/each}
  </div>
  <button
    class="g-btn g-btn-sm page-nav-step"
    type="button"
    aria-label="Next page"
    title="Next page"
    disabled={disabled || page >= pageCount}
    onclick={() => selectPage(page + 1)}
  >›</button>
</nav>

<style>
  .page-nav {
    flex: 0 0 auto;
    width: 100%;
    display: flex;
    justify-content: center;
    align-items: center;
    gap: 8px;
    box-sizing: border-box;
    margin-top: auto;
    padding: 18px 24px 28px;
  }

  .page-nav.embedded {
    margin-top: 0;
    padding: 10px 12px;
    border-top: 1px solid var(--border);
  }

  .page-nav-pages {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    gap: 4px;
    min-width: 0;
  }

  .page-nav-step {
    flex: 0 0 32px;
    width: 32px;
    min-width: 32px;
    justify-content: center;
    padding-inline: 0;
    font-size: 18px;
    line-height: 1;
  }

  .page-nav-button {
    min-width: 32px;
    justify-content: center;
    padding-inline: 9px;
    font-variant-numeric: tabular-nums;
  }

  .page-nav-button[aria-current='page'] {
    background: var(--accent-soft);
    border-color: var(--accent-line);
    color: var(--accent);
  }

  .page-nav-ellipsis {
    min-width: 22px;
    text-align: center;
    color: var(--text-4);
    font-family: var(--font-mono);
  }

  @media (max-width: 720px) {
    .page-nav {
      gap: 4px;
      padding-inline: 8px;
    }

    .page-nav-pages {
      gap: 2px;
    }

    .page-nav-step,
    .page-nav-button {
      min-width: 28px;
      width: 28px;
      padding-inline: 0;
    }

    .page-nav-step {
      flex-basis: 28px;
    }

    .page-nav-ellipsis {
      min-width: 16px;
    }
  }
</style>
