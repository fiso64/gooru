<script lang="ts">
  import { paginationWindow } from '$lib/utils/pagination';

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

  function selectPage(nextPage: number) {
    if (disabled || nextPage < 1 || nextPage > pageCount || nextPage === page) return;
    onPage(nextPage);
  }
</script>

<nav class:embedded class="page-nav" aria-label={ariaLabel} data-testid={testId}>
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
    margin-top: 18px;
    padding: 0 24px 28px;
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
