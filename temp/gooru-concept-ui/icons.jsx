// Gooru icons — three style variants so the user can pick.
// Style "line": thin (1.5) stroke, rounded — Lucide-ish
// Style "solid": filled glyphs
// Style "mixed": line by default, solid when `active`
//
// Usage: <Icon name="search" size={16} style="line" active />

const ICONS = {
  // -------- line (1.5 stroke) ----------
  line: {
    common: {strokeWidth: 1.6, strokeLinecap: 'round', strokeLinejoin: 'round', fill: 'none', stroke: 'currentColor'},
    paths: {
      search: <><circle cx="11" cy="11" r="7"/><path d="m20 20-3.5-3.5"/></>,
      grid: <><rect x="3" y="3" width="7" height="7" rx="1"/><rect x="14" y="3" width="7" height="7" rx="1"/><rect x="3" y="14" width="7" height="7" rx="1"/><rect x="14" y="14" width="7" height="7" rx="1"/></>,
      rows: <><path d="M3 6h18"/><path d="M3 12h18"/><path d="M3 18h18"/></>,
      tag: <><path d="M21 12.6V4h-8.6L2.3 14.1a1.4 1.4 0 0 0 0 2L8 21.7a1.4 1.4 0 0 0 2 0L21 12.6Z"/><circle cx="16" cy="8" r="1.4"/></>,
      tags: <><path d="M3.6 11.6V4h7.6L20 13a1.5 1.5 0 0 1 0 2.1l-5.1 5.1a1.5 1.5 0 0 1-2.1 0L3.6 11.6Z"/><path d="m8 4 9 9a1.5 1.5 0 0 1 0 2.1l-1 1"/><circle cx="8" cy="8" r="1.2"/></>,
      photo: <><rect x="3" y="4.5" width="18" height="15" rx="2"/><circle cx="9" cy="10" r="1.6"/><path d="m3.5 17 5-5 4.5 4.5 3-3 4.5 4.5"/></>,
      video: <><rect x="3" y="5" width="14" height="14" rx="2"/><path d="m21 7-4 3v4l4 3z"/></>,
      gif: <><rect x="3" y="5" width="18" height="14" rx="2"/><path d="M7 10v4M10 10v4M13 10h2.5M13 10v4M13 12h1.5M17.5 10h2"/></>,
      upload: <><path d="M12 16V4"/><path d="m6 10 6-6 6 6"/><path d="M4 20h16"/></>,
      jobs: <><path d="M12 3v2"/><circle cx="12" cy="12" r="8"/><path d="M12 8v4l3 2"/></>,
      settings: <><circle cx="12" cy="12" r="3"/><path d="M19.4 15a1.6 1.6 0 0 0 .3 1.8l.1.1a2 2 0 1 1-2.8 2.8l-.1-.1a1.6 1.6 0 0 0-1.8-.3 1.6 1.6 0 0 0-1 1.5V21a2 2 0 0 1-4 0v-.1a1.6 1.6 0 0 0-1-1.4 1.6 1.6 0 0 0-1.8.3l-.1.1a2 2 0 1 1-2.8-2.8l.1-.1a1.6 1.6 0 0 0 .3-1.8 1.6 1.6 0 0 0-1.5-1H3a2 2 0 0 1 0-4h.1A1.6 1.6 0 0 0 4.6 9a1.6 1.6 0 0 0-.3-1.8l-.1-.1a2 2 0 1 1 2.8-2.8l.1.1a1.6 1.6 0 0 0 1.8.3H9a1.6 1.6 0 0 0 1-1.5V3a2 2 0 0 1 4 0v.1a1.6 1.6 0 0 0 1 1.5 1.6 1.6 0 0 0 1.8-.3l.1-.1a2 2 0 1 1 2.8 2.8l-.1.1a1.6 1.6 0 0 0-.3 1.8V9a1.6 1.6 0 0 0 1.5 1H21a2 2 0 0 1 0 4h-.1a1.6 1.6 0 0 0-1.5 1Z"/></>,
      library: <><path d="M3 5h4v14H3z"/><path d="M9 5h4v14H9z"/><path d="m15 6 4.5 1L17 21l-4.5-1Z"/></>,
      keyboard: <><rect x="2" y="6" width="20" height="12" rx="2"/><path d="M6 10h.01M10 10h.01M14 10h.01M18 10h.01M6 14h.01M18 14h.01M10 14h4"/></>,
      close: <><path d="m6 6 12 12M18 6 6 18"/></>,
      check: <><path d="m5 12 5 5L20 7"/></>,
      plus: <><path d="M12 5v14M5 12h14"/></>,
      minus: <><path d="M5 12h14"/></>,
      chev_left: <><path d="m15 6-6 6 6 6"/></>,
      chev_right: <><path d="m9 6 6 6-6 6"/></>,
      chev_down: <><path d="m6 9 6 6 6-6"/></>,
      eye: <><path d="M2 12s3.5-7 10-7 10 7 10 7-3.5 7-10 7S2 12 2 12Z"/><circle cx="12" cy="12" r="3"/></>,
      info: <><circle cx="12" cy="12" r="9"/><path d="M12 16v-5M12 8v.01"/></>,
      download: <><path d="M12 4v12"/><path d="m6 10 6 6 6-6"/><path d="M4 20h16"/></>,
      trash: <><path d="M4 7h16"/><path d="M9 7V4h6v3"/><path d="M6 7 7 21h10l1-14"/><path d="M10 11v6M14 11v6"/></>,
      bookmark: <><path d="M6 4h12v17l-6-4-6 4z"/></>,
      star: <><path d="m12 4 2.6 5.6L20 10.4l-4 3.9 1 5.7L12 17.3 6.9 20l1-5.7-4-3.9 5.5-.8z"/></>,
      filter: <><path d="M4 6h16M7 12h10M10 18h4"/></>,
      sliders: <><path d="M4 8h11M19 8h1"/><circle cx="17" cy="8" r="2"/><path d="M4 16h3M11 16h9"/><circle cx="9" cy="16" r="2"/></>,
      sort: <><path d="M3 7h13M3 12h9M3 17h5"/><path d="m17 14 4 4 4-4" transform="translate(-3 -1)"/></>,
      folder: <><path d="M3 6.5A1.5 1.5 0 0 1 4.5 5h4.4l2 2h8.6A1.5 1.5 0 0 1 21 8.5V18a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2Z"/></>,
      key: <><circle cx="8" cy="14" r="4"/><path d="m10.8 11.2 9.2-9.2"/><path d="m17 5 3 3M14.5 7.5 17 10"/></>,
      logout: <><path d="M15 4h3a2 2 0 0 1 2 2v12a2 2 0 0 1-2 2h-3"/><path d="M10 17 5 12l5-5"/><path d="M5 12h11"/></>,
      user: <><circle cx="12" cy="8" r="4"/><path d="M4 21a8 8 0 0 1 16 0"/></>,
      gooru: <><circle cx="12" cy="12" r="9"/><circle cx="12" cy="12" r="4"/><circle cx="12" cy="12" r="1.2" fill="currentColor"/></>,
      play: <><path d="m6 4 14 8-14 8z"/></>,
      pause: <><path d="M7 4v16M17 4v16"/></>,
      x_circle: <><circle cx="12" cy="12" r="9"/><path d="m9 9 6 6M15 9l-6 6"/></>,
      help: <><circle cx="12" cy="12" r="9"/><path d="M9.5 9a2.5 2.5 0 0 1 5 0c0 1.5-2.5 2-2.5 3.5M12 17v.01"/></>,
      command: <><path d="M9 9h6v6H9z"/><path d="M9 9a3 3 0 1 1-3 3M9 15a3 3 0 1 0-3-3M15 9a3 3 0 1 0 3 3M15 15a3 3 0 1 1 3-3"/></>,
      external: <><path d="M14 4h6v6"/><path d="M20 4 10 14"/><path d="M19 14v5a1 1 0 0 1-1 1H5a1 1 0 0 1-1-1V6a1 1 0 0 1 1-1h5"/></>,
      arrow_up_right: <><path d="M7 17 17 7"/><path d="M9 7h8v8"/></>,
      circle: <><circle cx="12" cy="12" r="9"/></>,
      dot: <><circle cx="12" cy="12" r="2" fill="currentColor" stroke="none"/></>,
    },
  },

  solid: {
    common: {fill: 'currentColor', stroke: 'none'},
    paths: {
      search: <><circle cx="11" cy="11" r="7" fill="none" stroke="currentColor" strokeWidth="2.4"/><rect x="15.5" y="16" width="6.6" height="2.4" rx="1.2" transform="rotate(45 15.5 16)"/></>,
      grid: <><rect x="3" y="3" width="8" height="8" rx="1.5"/><rect x="13" y="3" width="8" height="8" rx="1.5"/><rect x="3" y="13" width="8" height="8" rx="1.5"/><rect x="13" y="13" width="8" height="8" rx="1.5"/></>,
      rows: <><rect x="3" y="5" width="18" height="2.5" rx="1"/><rect x="3" y="10.75" width="18" height="2.5" rx="1"/><rect x="3" y="16.5" width="18" height="2.5" rx="1"/></>,
      tag: <><path d="M21 12V4h-8L2.3 14.7a1.4 1.4 0 0 0 0 2L7.3 21.7a1.4 1.4 0 0 0 2 0L21 12Zm-5-2.5a1.4 1.4 0 1 1 0-2.8 1.4 1.4 0 0 1 0 2.8Z"/></>,
      tags: <><path d="M21 12V4h-8L2.3 14.7a1.4 1.4 0 0 0 0 2L7.3 21.7a1.4 1.4 0 0 0 2 0L21 12Z"/></>,
      photo: <><path d="M5 4.5h14a2 2 0 0 1 2 2v11a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-11a2 2 0 0 1 2-2Zm4 6.5a2 2 0 1 0 0-4 2 2 0 0 0 0 4Z"/></>,
      video: <><rect x="3" y="5" width="14" height="14" rx="2"/><path d="m21 7-4 3v4l4 3z"/></>,
      gif: <><rect x="3" y="5" width="18" height="14" rx="2" fill="currentColor"/><text x="12" y="15.5" textAnchor="middle" fontSize="7.5" fontFamily="monospace" fontWeight="700" fill="var(--bg)" stroke="none">GIF</text></>,
      upload: <><rect x="4" y="18" width="16" height="2.5" rx="1"/><path d="M12 4 6 10h4v6h4v-6h4z"/></>,
      jobs: <><circle cx="12" cy="12" r="9"/><path d="M12 7v5l3 2" stroke="var(--bg)" strokeWidth="1.8" strokeLinecap="round" fill="none"/></>,
      settings: <><path d="M19.4 15a1.6 1.6 0 0 0 .3 1.8l.1.1a2 2 0 1 1-2.8 2.8l-.1-.1a1.6 1.6 0 0 0-2.8 1.1V21a2 2 0 1 1-4 0v-.3a1.6 1.6 0 0 0-2.8-1l-.1.1a2 2 0 1 1-2.8-2.8l.1-.1a1.6 1.6 0 0 0-1-2.8H3a2 2 0 0 1 0-4h.3a1.6 1.6 0 0 0 1-2.8l-.1-.1a2 2 0 1 1 2.8-2.8l.1.1a1.6 1.6 0 0 0 2.8-1V3a2 2 0 0 1 4 0v.3a1.6 1.6 0 0 0 2.8 1l.1-.1a2 2 0 1 1 2.8 2.8l-.1.1a1.6 1.6 0 0 0 1 2.8H21a2 2 0 0 1 0 4h-.3a1.6 1.6 0 0 0-1.3 1.1ZM12 9.5a2.5 2.5 0 1 0 0 5 2.5 2.5 0 0 0 0-5Z"/></>,
      library: <><rect x="3" y="5" width="3.5" height="14" rx="0.5"/><rect x="7.5" y="5" width="3.5" height="14" rx="0.5"/><path d="m14.4 6.2 4 .8L16 21l-4-0.8z"/></>,
      keyboard: <><path d="M4 6h16a2 2 0 0 1 2 2v8a2 2 0 0 1-2 2H4a2 2 0 0 1-2-2V8a2 2 0 0 1 2-2Zm2 4v1.5h1.5V10H6Zm3 0v1.5h1.5V10H9Zm3 0v1.5h1.5V10H12Zm3 0v1.5h1.5V10H15Zm-9 3v1.5h1.5V13H6Zm3 0v1.5h6V13H9Zm7.5 0v1.5H18V13h-1.5Z"/></>,
      close: <><path d="m6 6 12 12M18 6 6 18" stroke="currentColor" strokeWidth="2.2" strokeLinecap="round" fill="none"/></>,
      check: <><path d="m5 12 5 5L20 7" stroke="currentColor" strokeWidth="2.2" strokeLinecap="round" strokeLinejoin="round" fill="none"/></>,
      plus: <><path d="M12 5v14M5 12h14" stroke="currentColor" strokeWidth="2.2" strokeLinecap="round" fill="none"/></>,
      minus: <><path d="M5 12h14" stroke="currentColor" strokeWidth="2.2" strokeLinecap="round" fill="none"/></>,
      chev_left: <><path d="m15 6-6 6 6 6" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" fill="none"/></>,
      chev_right: <><path d="m9 6 6 6-6 6" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" fill="none"/></>,
      chev_down: <><path d="m6 9 6 6 6-6" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" fill="none"/></>,
      eye: <><path d="M12 5C5 5 1.5 12 1.5 12S5 19 12 19s10.5-7 10.5-7S19 5 12 5Zm0 11a4 4 0 1 1 0-8 4 4 0 0 1 0 8Z"/></>,
      info: <><circle cx="12" cy="12" r="9"/><path d="M12 16v-5M12 8v.01" stroke="var(--bg)" strokeWidth="2.2" strokeLinecap="round" fill="none"/></>,
      download: <><rect x="4" y="18" width="16" height="2.5" rx="1"/><path d="M12 16 6 10h4V4h4v6h4z"/></>,
      trash: <><path d="M9 3h6v2h5v2H4V5h5z"/><path d="M5 8h14l-1 12a1 1 0 0 1-1 1H7a1 1 0 0 1-1-1Zm5 3v6h1.5v-6zm3.5 0v6H15v-6z"/></>,
      bookmark: <><path d="M6 4h12v17l-6-4-6 4z"/></>,
      star: <><path d="m12 4 2.6 5.6L20 10.4l-4 3.9 1 5.7L12 17.3 6.9 20l1-5.7-4-3.9 5.5-.8z"/></>,
      filter: <><path d="M3 5h18l-7 9v6l-4-2v-4z"/></>,
      sliders: <><rect x="3" y="7" width="18" height="2" rx="1"/><rect x="3" y="15" width="18" height="2" rx="1"/><circle cx="16" cy="8" r="3" fill="var(--bg)" stroke="currentColor" strokeWidth="2"/><circle cx="9" cy="16" r="3" fill="var(--bg)" stroke="currentColor" strokeWidth="2"/></>,
      sort: <><path d="M3 5h13v2H3zM3 11h9v2H3zM3 17h5v2H3z"/><path d="m17 13 3 4 3-4z" transform="translate(-3 1)"/></>,
      folder: <><path d="M4 5h4.5l2 2H20a1 1 0 0 1 1 1V18a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V6a1 1 0 0 1 1-1Z"/></>,
      key: <><circle cx="8" cy="14" r="4.5"/><path d="m10.8 11.2 9.2-9.2 1.5 1.5-2.5 2.5 2 2-1.5 1.5-2-2-1.5 1.5 2 2-1.5 1.5-2-2-3.7 3.7" stroke="currentColor" strokeWidth="1.5" fill="none" strokeLinecap="round" strokeLinejoin="round"/><circle cx="8" cy="14" r="1.3" fill="var(--bg)" stroke="none"/></>,
      logout: <><path d="M15 4h3a2 2 0 0 1 2 2v12a2 2 0 0 1-2 2h-3v-2h3V6h-3Z"/><path d="m10 17-1.5-1.5L11 13H3v-2h8L8.5 8.5 10 7l5 5z"/></>,
      user: <><circle cx="12" cy="8" r="4"/><path d="M4 21a8 8 0 0 1 16 0Z"/></>,
      gooru: <><circle cx="12" cy="12" r="9" fill="none" stroke="currentColor" strokeWidth="1.7"/><circle cx="12" cy="12" r="4.5" fill="currentColor"/></>,
      play: <><path d="m6 4 14 8-14 8z"/></>,
      pause: <><rect x="6" y="4" width="3.5" height="16" rx="0.5"/><rect x="14.5" y="4" width="3.5" height="16" rx="0.5"/></>,
      x_circle: <><circle cx="12" cy="12" r="9"/><path d="m9 9 6 6M15 9l-6 6" stroke="var(--bg)" strokeWidth="2.2" strokeLinecap="round" fill="none"/></>,
      help: <><circle cx="12" cy="12" r="9"/><path d="M9.5 9a2.5 2.5 0 0 1 5 0c0 1.5-2.5 2-2.5 3.5M12 17v.01" stroke="var(--bg)" strokeWidth="2" strokeLinecap="round" fill="none"/></>,
      command: <><path d="M9 9h6v6H9z"/><path d="M9 9a3 3 0 1 1-3 3M9 15a3 3 0 1 0-3-3M15 9a3 3 0 1 0 3 3M15 15a3 3 0 1 1 3-3" stroke="currentColor" strokeWidth="2" fill="none" strokeLinecap="round" strokeLinejoin="round"/></>,
      external: <><path d="M14 4h6v6h-2V7.4l-7.3 7.3-1.4-1.4L16.6 6H14Z"/><path d="M19 13v5a2 2 0 0 1-2 2H6a2 2 0 0 1-2-2V7a2 2 0 0 1 2-2h5v2H6v11h11v-5Z"/></>,
      arrow_up_right: <><path d="m7 17 10-10"/><path d="M8 6h10v10h-2V9.4l-7.3 7.3" stroke="currentColor" strokeWidth="2.2" fill="currentColor" strokeLinejoin="round"/></>,
      circle: <><circle cx="12" cy="12" r="9"/></>,
      dot: <><circle cx="12" cy="12" r="3"/></>,
    },
  },
};

// alias map so we don't repeat names
ICONS.mixed = ICONS.line; // base — `active` flag swaps to solid

function Icon({name, size = 18, style: iconStyle = 'line', active = false, color}) {
  const effective = (iconStyle === 'mixed' && active) ? 'solid' : iconStyle;
  const set = ICONS[effective] || ICONS.line;
  const def = (set.paths[name]) || ICONS.line.paths[name];
  if (!def) return null;
  return (
    <svg
      width={size}
      height={size}
      viewBox="0 0 24 24"
      {...set.common}
      style={{flexShrink: 0, ...(color ? {color} : {})}}
      aria-hidden="true"
    >
      {def}
    </svg>
  );
}

// export
Object.assign(window, {Icon, ICONS});
