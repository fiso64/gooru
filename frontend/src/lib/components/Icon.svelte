<script lang="ts">
  let {
    name,
    size = 18,
    active = false
  } = $props<{
    name: string;
    size?: number;
    active?: boolean;
  }>();

  const paths: Record<string, string> = {
    search: '<circle cx="11" cy="11" r="7"/><path d="m20 20-3.5-3.5"/>',
    library: '<path d="M3 5h4v14H3z"/><path d="M9 5h4v14H9z"/><path d="m15 6 4.5 1L17 21l-4.5-1Z"/>',
    tag: '<path d="M21 12.6V4h-8.6L2.3 14.1a1.4 1.4 0 0 0 0 2L8 21.7a1.4 1.4 0 0 0 2 0L21 12.6Z"/><circle cx="16" cy="8" r="1.4"/>',
    tags: '<path d="M3.6 11.6V4h7.6L20 13a1.5 1.5 0 0 1 0 2.1l-5.1 5.1a1.5 1.5 0 0 1-2.1 0L3.6 11.6Z"/><path d="m8 4 9 9a1.5 1.5 0 0 1 0 2.1l-1 1"/><circle cx="8" cy="8" r="1.2"/>',
    tag_remove: '<path d="M21 12.6V4h-8.6L2.3 14.1a1.4 1.4 0 0 0 0 2L8 21.7a1.4 1.4 0 0 0 2 0L21 12.6Z"/><circle cx="16" cy="8" r="1.4"/><path d="M7.5 15h5"/>',
    photo: '<rect x="3" y="4.5" width="18" height="15" rx="2"/><circle cx="9" cy="10" r="1.6"/><path d="m3.5 17 5-5 4.5 4.5 3-3 4.5 4.5"/>',
    video: '<rect x="3" y="5" width="14" height="14" rx="2"/><path d="m21 7-4 3v4l4 3z"/>',
    audio: '<path d="M9 18V6l10-2v12"/><circle cx="6" cy="18" r="3"/><circle cx="16" cy="16" r="3"/>',
    gif: '<rect x="3" y="5" width="18" height="14" rx="2"/><path d="M7 10v4M10 10v4M13 10h2.5M13 10v4M13 12h1.5M17.5 10h2"/>',
    upload: '<path d="M12 16V4"/><path d="m6 10 6-6 6 6"/><path d="M4 20h16"/>',
    jobs: '<path d="M12 3v2"/><circle cx="12" cy="12" r="8"/><path d="M12 8v4l3 2"/>',
    settings: '<circle cx="12" cy="12" r="3"/><path d="M19.4 15a1.6 1.6 0 0 0 .3 1.8l.1.1a2 2 0 1 1-2.8 2.8l-.1-.1a1.6 1.6 0 0 0-1.8-.3 1.6 1.6 0 0 0-1 1.5V21a2 2 0 0 1-4 0v-.1a1.6 1.6 0 0 0-1-1.4 1.6 1.6 0 0 0-1.8.3l-.1.1a2 2 0 1 1-2.8-2.8l.1-.1a1.6 1.6 0 0 0 .3-1.8 1.6 1.6 0 0 0-1.5-1H3a2 2 0 0 1 0-4h.1A1.6 1.6 0 0 0 4.6 9a1.6 1.6 0 0 0-.3-1.8l-.1-.1a2 2 0 1 1 2.8-2.8l.1.1a1.6 1.6 0 0 0 1.8.3H9a1.6 1.6 0 0 0 1-1.5V3a2 2 0 0 1 4 0v.1a1.6 1.6 0 0 0 1 1.5 1.6 1.6 0 0 0 1.8-.3l.1-.1a2 2 0 1 1 2.8 2.8l-.1.1a1.6 1.6 0 0 0-.3 1.8V9a1.6 1.6 0 0 0 1.5 1H21a2 2 0 0 1 0 4h-.1a1.6 1.6 0 0 0-1.5 1Z"/>',
    keyboard: '<rect x="2" y="6" width="20" height="12" rx="2"/><path d="M6 10h.01M10 10h.01M14 10h.01M18 10h.01M6 14h.01M18 14h.01M10 14h4"/>',
    bookmark: '<path d="M6 4h12v17l-6-4-6 4z"/>',
    user: '<circle cx="12" cy="8" r="4"/><path d="M4 21a8 8 0 0 1 16 0"/>',
    logout: '<path d="M15 4h3a2 2 0 0 1 2 2v12a2 2 0 0 1-2 2h-3"/><path d="M10 17 5 12l5-5"/><path d="M5 12h11"/>',
    close: '<path d="m6 6 12 12M18 6 6 18"/>',
    check: '<path d="m5 12 5 5L20 7"/>',
    plus: '<path d="M12 5v14M5 12h14"/>',
    minus: '<path d="M5 12h14"/>',
    chev_left: '<path d="m15 6-6 6 6 6"/>',
    chev_right: '<path d="m9 6 6 6-6 6"/>',
    sliders: '<path d="M4 8h11M19 8h1"/><circle cx="17" cy="8" r="2"/><path d="M4 16h3M11 16h9"/><circle cx="9" cy="16" r="2"/>',
    sort: '<path d="M3 7h13M3 12h9M3 17h5"/><path d="m17 14 4 4 4-4" transform="translate(-3 -1)"/>',
    download: '<path d="M12 4v12"/><path d="m6 10 6 6 6-6"/><path d="M4 20h16"/>',
    external: '<path d="M14 4h6v6"/><path d="M20 4 10 14"/><path d="M19 14v5a1 1 0 0 1-1 1H5a1 1 0 0 1-1-1V6a1 1 0 0 1 1-1h5"/>',
    trash: '<path d="M4 7h16"/><path d="M9 7V4h6v3"/><path d="M6 7 7 21h10l1-14"/><path d="M10 11v6M14 11v6"/>',
    untrack: '<path d="M3.5 5h5v14h-5z"/><path d="M10.5 5h5v14h-5z"/><path d="M17.5 12h4"/><path d="m19.5 10 2 2-2 2"/>',
    play: '<path d="m6 4 14 8-14 8z"/>',
    pause: '<path d="M7 4v16M17 4v16"/>',
    folder: '<path d="M3 6.5A1.5 1.5 0 0 1 4.5 5h4.4l2 2h8.6A1.5 1.5 0 0 1 21 8.5V18a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2Z"/>',
    info: '<circle cx="12" cy="12" r="9"/><path d="M12 16v-5M12 8v.01"/>',
    menu: '<path d="M4 7h16M4 12h16M4 17h16"/>'
  };
</script>

<svg
  width={size}
  height={size}
  viewBox="0 0 24 24"
  fill="none"
  stroke="currentColor"
  stroke-width={active ? 2 : 1.6}
  stroke-linecap="round"
  stroke-linejoin="round"
  aria-hidden="true"
>
  {@html paths[name] ?? paths.info}
</svg>
