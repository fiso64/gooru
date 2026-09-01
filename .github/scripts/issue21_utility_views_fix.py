from pathlib import Path


def replace_once(path: str, old: str, new: str, label: str) -> None:
    file = Path(path)
    text = file.read_text()
    if old not in text:
        raise SystemExit(f'{label} not found in {path}')
    file.write_text(text.replace(old, new, 1))


replace_once(
    'frontend/src/lib/components/JobsDrawer.svelte',
    '<JobRow {job} compact onCancel={onCancel} />',
    '<JobRow {job} onCancel={onCancel} />',
    'compact Jobs drawer row',
)

replace_once(
    'frontend/src/lib/components/JobRow.svelte',
    "    compact = false,\n",
    '',
    'compact prop default',
)
replace_once(
    'frontend/src/lib/components/JobRow.svelte',
    "    compact?: boolean;\n",
    '',
    'compact prop type',
)
replace_once(
    'frontend/src/lib/components/JobRow.svelte',
    '<div class:compact class="job-row">',
    '<div class="job-row">',
    'compact class binding',
)
replace_once(
    'frontend/src/lib/components/JobRow.svelte',
    "\n  .compact {\n    padding-block: 10px;\n  }\n",
    '\n',
    'compact CSS override',
)
