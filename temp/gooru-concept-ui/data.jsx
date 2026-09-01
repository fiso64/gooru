import React from 'react';

// Gooru — mock data for the design.
// Real thumbnails come from Unsplash (source URLs). Each file has rich tags
// across namespaces so the UI exposes real metadata structure.

const UNSPLASH = (id, w = 600, h = 600) =>
  `https://images.unsplash.com/photo-${id}?auto=format&fit=crop&w=${w}&h=${h}&q=75`;

// curated list of stable Unsplash photo IDs by theme
const PHOTOS = [
  // architecture
  {id: '1499856871958-5b9627545d1a', subject: 'architecture', location: 'paris', color: 'monochrome', year: 2024, name: 'gare-du-nord-mezzanine.jpg'},
  {id: '1487958449943-2429e8be8625', subject: 'architecture', location: 'london', color: 'monochrome', year: 2023, name: 'barbican-walkway.jpg'},
  {id: '1518005020951-eccb494ad742', subject: 'architecture', location: 'nyc',   color: 'monochrome', year: 2024, name: 'lower-manhattan-fog.jpg'},
  {id: '1486325212027-8081e485255e', subject: 'architecture', location: 'tokyo', color: 'monochrome', year: 2024, name: 'ginza-facade-grid.jpg'},
  {id: '1495107334309-fcf20504a5ab', subject: 'architecture', location: 'berlin', color: 'vibrant',   year: 2024, name: 'spree-bridge-evening.jpg'},
  {id: '1480714378408-67cf0d13bc1b', subject: 'architecture', location: 'nyc',   color: 'monochrome', year: 2022, name: 'flatiron-corner.jpg'},

  // landscape / nature
  {id: '1501785888041-af3ef285b470', subject: 'landscape', location: 'iceland', color: 'cool',     year: 2023, name: 'reykjavik-fjord.jpg'},
  {id: '1469474968028-56623f02e42e', subject: 'landscape', location: 'usa',     color: 'warm',     year: 2022, name: 'sierra-light-rays.jpg'},
  {id: '1470770841072-f978cf4d019e', subject: 'landscape', location: 'norway',  color: 'cool',     year: 2024, name: 'lofoten-low-tide.jpg'},
  {id: '1418065460487-3e41a6c84dc5', subject: 'landscape', location: 'usa',     color: 'cool',     year: 2023, name: 'tahoe-mist.jpg'},
  {id: '1500382017468-9049fed747ef', subject: 'landscape', location: 'usa',     color: 'warm',     year: 2024, name: 'colorado-overlook.jpg'},
  {id: '1418489098061-ce87b5dc3aee', subject: 'landscape', location: 'iceland', color: 'monochrome', year: 2022, name: 'highlands-storm.jpg'},

  // portrait
  {id: '1494790108377-be9c29b29330', subject: 'portrait', location: 'studio', color: 'warm',     year: 2024, name: 'studio-soft-light-a.jpg', people: 'a-okafor'},
  {id: '1463453091185-61582044d556', subject: 'portrait', location: 'studio', color: 'warm',     year: 2023, name: 'window-light-portrait.jpg', people: 'm-tanaka'},
  {id: '1531746020798-e6953c6e8e04', subject: 'portrait', location: 'tokyo',  color: 'cool',     year: 2024, name: 'tokyo-arcade-portrait.jpg', people: 'r-haines'},
  {id: '1488426862026-3ee34a7d66df', subject: 'portrait', location: 'studio', color: 'warm',     year: 2024, name: 'studio-amber-portrait.jpg', people: 'a-okafor'},
  {id: '1535713875002-d1d0cf377fde', subject: 'portrait', location: 'studio', color: 'cool',     year: 2023, name: 'overcast-portrait.jpg', people: 'l-meier'},

  // still life
  {id: '1493612276216-ee3925520721', subject: 'still-life', location: 'studio', color: 'warm', year: 2024, name: 'film-shelf.jpg'},
  {id: '1452860606245-08befc0ff44b', subject: 'still-life', location: 'studio', color: 'warm', year: 2023, name: 'leica-on-desk.jpg'},
  {id: '1495121553079-4c61bcce1894', subject: 'still-life', location: 'studio', color: 'warm', year: 2024, name: 'enamel-cup.jpg'},
  {id: '1474181487882-5abf3f0ba6c2', subject: 'still-life', location: 'studio', color: 'monochrome', year: 2024, name: 'ceramic-set-bw.jpg'},
  {id: '1485827404703-89b55fcc595e', subject: 'still-life', location: 'studio', color: 'cool',  year: 2024, name: 'tape-deck.jpg'},

  // streets
  {id: '1517732306149-e8f829eb588a', subject: 'street', location: 'tokyo',  color: 'vibrant', year: 2024, name: 'shibuya-crossing-1.jpg'},
  {id: '1542051841857-5f90071e7989', subject: 'street', location: 'tokyo',  color: 'cool',    year: 2024, name: 'akihabara-rain.jpg'},
  {id: '1480714378408-67cf0d13bc1b', subject: 'street', location: 'nyc',    color: 'monochrome', year: 2023, name: 'soho-fire-escape.jpg'},
  {id: '1493217465235-c7795c1f3dfb', subject: 'street', location: 'paris',  color: 'warm',    year: 2024, name: 'le-marais-cafe.jpg'},
  {id: '1502602898657-3e91760cbb34', subject: 'street', location: 'paris',  color: 'warm',    year: 2023, name: 'paris-evening.jpg'},

  // documents / scans
  {id: '1457369804613-52c61a468e7d', subject: 'document', location: 'archive', color: 'monochrome', year: 2024, name: 'archive-book.jpg'},
  {id: '1519682337058-a94d519337bc', subject: 'document', location: 'archive', color: 'monochrome', year: 2024, name: 'notebook-spread.jpg'},
  {id: '1481627834876-b7833e8f5570', subject: 'document', location: 'archive', color: 'warm',     year: 2024, name: 'old-map.jpg'},
];

// derivative kinds so we can show video/gif badges
function deriveKind(i) {
  // every 7th is a video, every 11th is a gif
  if (i % 11 === 5) return 'gif';
  if (i % 7 === 4) return 'video';
  return 'photo';
}
function durationFor(i) {
  // pseudo-random duration string
  const s = (i * 13) % 240 + 7;
  const m = Math.floor(s / 60);
  const ss = (s % 60).toString().padStart(2, '0');
  return `${m}:${ss}`;
}
function sizeFor(i, kind) {
  if (kind === 'photo') return 1.2 + (i % 7) * 0.85;       // MB
  if (kind === 'gif')   return 4.3 + (i % 5) * 1.4;
  return 28 + (i % 9) * 12;                                 // video MB
}

function hashHex(i) {
  // pseudo content hash — mix in an offset so index 0 doesn't yield all zeros
  const s = (((i + 7) * 2654435761) >>> 0).toString(16).padStart(8, '0');
  const t = (((i + 91) * 1779033703) >>> 0).toString(16).padStart(8, '0');
  return ('sha256:' + (s + t + s + t)).slice(0, 71);
}

const FILES = PHOTOS.map((p, i) => {
  const kind = deriveKind(i);
  const tags = [
    'rating:safe',
    `subject:${p.subject}`,
    `location:${p.location}`,
    `color:${p.color}`,
    `year:${p.year}`,
  ];
  if (p.people) tags.push(`people:${p.people}`);
  if (i % 3 === 0) tags.push('collection:portfolio');
  if (i % 5 === 0) tags.push('collection:archive-2024');
  if (i % 4 === 0) tags.push('film:portra-400');
  if (i % 9 === 0) tags.push('camera:leica-q2');
  if (i % 6 === 0) tags.push('camera:fujifilm-x100v');
  if (i % 8 === 0) tags.push('hero');
  if (i % 13 === 2) tags.push('review');
  if (i % 6 === 1) tags.push('processed');

  const w = 1600 + (i % 5) * 200;
  const h = kind === 'video' ? 900 + (i % 3) * 120 : 1066 + (i % 3) * 160;

  const ext = kind === 'video' ? 'mp4' : (kind === 'gif' ? 'gif' : (i % 4 === 0 ? 'jpg' : (i % 4 === 1 ? 'jpeg' : 'png')));
  const baseName = p.name.replace(/\.[^.]+$/, '');
  const name = `${baseName}.${ext}`;

  return {
    id: `f_${('000' + i).slice(-4)}`,
    contentHash: hashHex(i),
    name,
    path: `/library/${p.subject}/${name}`,
    kind,
    mime: kind === 'video' ? 'video/mp4' : (kind === 'gif' ? 'image/gif' : `image/${ext === 'jpg' ? 'jpeg' : ext}`),
    width: w, height: h,
    duration: kind !== 'photo' ? durationFor(i) : null,
    sizeMB: sizeFor(i, kind),
    modified: new Date(2024, (i % 12), (i % 27) + 1, (i % 23), (i % 59)).toISOString(),
    tags,
    thumb: UNSPLASH(p.id, 600, 600),
    preview: UNSPLASH(p.id, 1600, 1600),
  };
});

// Tag index with counts
const TAG_INDEX = (() => {
  const counts = new Map();
  FILES.forEach(f => f.tags.forEach(t => counts.set(t, (counts.get(t) || 0) + 1)));
  const arr = [...counts.entries()].map(([tag, count]) => ({tag, count}));
  arr.sort((a, b) => b.count - a.count);
  return arr;
})();

const NAMESPACES = ['rating', 'subject', 'location', 'color', 'year', 'collection', 'film', 'camera', 'people'];

function parseTag(t) {
  const i = t.indexOf(':');
  if (i < 0) return {ns: '', value: t}; // bare e.g. @favorite
  return {ns: t.slice(0, i), value: t.slice(i + 1)};
}
function groupTagsByNamespace(tags) {
  const map = new Map();
  for (const t of tags) {
    const {ns} = parseTag(t);
    const key = ns || '';
    if (!map.has(key)) map.set(key, []);
    map.get(key).push(t);
  }
  // Valueless tags come first (under empty key), then namespaced groups in order
  const order = ['', 'rating', 'subject', 'location', 'people', 'color', 'year', 'collection', 'film', 'camera'];
  return order.filter(k => map.has(k)).map(k => ({ns: k, tags: map.get(k)}));
}

// Recent saved searches
const SAVED_SEARCHES = [
  {name: 'Portraits this year', query: 'subject:portrait year:2024'},
  {name: 'Tokyo street', query: 'subject:street location:tokyo'},
  {name: 'Hero shots', query: 'hero'},
  {name: 'Portra rolls', query: 'film:portra-400'},
  {name: 'Needs review', query: 'review'},
];

const KINDS = [
  {key: 'photo', label: 'Photos', count: FILES.filter(f => f.kind === 'photo').length},
  {key: 'video', label: 'Videos', count: FILES.filter(f => f.kind === 'video').length},
  {key: 'gif',   label: 'GIFs',   count: FILES.filter(f => f.kind === 'gif').length},
];

const JOBS = [
  {id: 'j_001', kind: 'thumbnail', name: 'Building thumbnails', status: 'running', total: 248, done: 173, started: '2 min ago'},
  {id: 'j_002', kind: 'import',    name: 'Importing /Volumes/work/feb-rolls', status: 'running', total: 124, done: 47,  started: '4 min ago'},
  {id: 'j_003', kind: 'tag',       name: 'Bulk tag — added subject:street to 36', status: 'done',  total: 36,  done: 36,  started: '6 min ago'},
  {id: 'j_004', kind: 'import',    name: 'Imported /Volumes/work/jan-rolls', status: 'done',  total: 318, done: 318, started: '22 min ago'},
  {id: 'j_005', kind: 'thumbnail', name: 'Thumbnail backfill', status: 'error', total: 84, done: 71, started: '1 h ago', error: 'ffmpeg exit 254 on 13 files'},
];

const SHORTCUTS = [
  {group: 'Navigation', items: [
    {keys: ['/'], desc: 'Focus search'},
    {keys: ['g', 'l'], desc: 'Go to library'},
    {keys: ['g', 't'], desc: 'Go to tags'},
    {keys: ['g', 's'], desc: 'Go to settings'},
    {keys: ['?'], desc: 'Show this cheatsheet'},
  ]},
  {group: 'Browsing', items: [
    {keys: ['j'], desc: 'Next file'},
    {keys: ['k'], desc: 'Previous file'},
    {keys: ['↵'], desc: 'Open lightbox'},
    {keys: ['Esc'], desc: 'Close lightbox / clear selection'},
    {keys: ['Space'], desc: 'Quick preview'},
  ]},
  {group: 'Selection', items: [
    {keys: ['x'], desc: 'Toggle select'},
    {keys: ['⇧', 'click'], desc: 'Range select'},
    {keys: ['⌘', 'a'], desc: 'Select all'},
    {keys: ['Esc'], desc: 'Clear selection'},
  ]},
  {group: 'Tagging', items: [
    {keys: ['t'], desc: 'Edit tags on focused file'},
    {keys: ['T'], desc: 'Bulk tag selection'},
    {keys: ['f'], desc: 'Toggle @favorite'},
    {keys: ['r'], desc: 'Toggle @review'},
  ]},
];

// expose
Object.assign(window, {FILES, TAG_INDEX, NAMESPACES, SAVED_SEARCHES, KINDS, JOBS, SHORTCUTS, parseTag, groupTagsByNamespace});
