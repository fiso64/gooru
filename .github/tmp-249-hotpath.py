from pathlib import Path


def replace_once(path: str, old: str, new: str) -> None:
    p = Path(path)
    text = p.read_text()
    count = text.count(old)
    if count != 1:
        raise SystemExit(f"{path}: expected one match, found {count}")
    p.write_text(text.replace(old, new, 1))


replace_once(
    "frontend/src/lib/components/UploadViewerDialog.svelte",
    "  function move(delta: number) {\n",
    "  function move(delta: -1 | 1) {\n",
)
replace_once(
    "frontend/src/lib/components/AuthenticatedApp.svelte",
    "      const currentIndex = upload.items.indexOf(item);\n",
    "      const currentIndex = upload.items[index] === item ? index : upload.items.indexOf(item);\n",
)
replace_once(
    "frontend/src/lib/components/AuthenticatedApp.svelte",
    "        const appliedIndex = upload.items.indexOf(item);\n",
    "        const appliedIndex = upload.items[index] === item ? index : upload.items.indexOf(item);\n",
)
replace_once(
    "frontend/src/lib/components/AuthenticatedApp.svelte",
    "        const failedIndex = upload.items.indexOf(item);\n",
    "        const failedIndex = upload.items[index] === item ? index : upload.items.indexOf(item);\n",
)
