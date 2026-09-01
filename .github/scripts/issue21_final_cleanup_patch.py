from pathlib import Path


def replace_once(path: str, old: str, new: str) -> None:
    file = Path(path)
    text = file.read_text()
    if text.count(old) != 1:
        raise SystemExit(f"expected exactly one match in {path}: {old!r}; found {text.count(old)}")
    file.write_text(text.replace(old, new, 1))


replace_once(
    'frontend/src/lib/components/AuthenticatedApp.svelte',
    "  import AccountView from '$lib/components/AccountView.svelte';\n",
    '',
)
replace_once(
    'frontend/src/lib/components/AuthenticatedApp.svelte',
    "    {:else if library.route === 'account'}\n      <AccountView username={$authState.user.username} onLogout={logout} />\n",
    '',
)
replace_once(
    'docs/issue-21-concept-discrepancies.md',
    '19. Account view separate from Settings: A. The concept has account controls in Settings; the existing Account route can remain as a compact reachable utility view, but Settings is the primary concept location for sign out.\n',
    '19. Separate Account route from Settings: C. The concept places account/session controls in Settings, and all current navigation routes there; remove the unreachable duplicate Account view instead of carrying #26 residue.\n',
)
replace_once(
    'frontend/src/lib/styles/components.css',
    '.upload-zone:hover,\n.upload-zone.is-drag,\n.upload-zone.drag-active {\n',
    '.upload-zone:hover,\n.upload-zone.is-drag {\n',
)
replace_once(
    'frontend/src/lib/styles/components.css',
    '.upload-list-card,\n.upload-list-wrap {\n  overflow: hidden;\n}\n',
    '.upload-list-card {\n  overflow: hidden;\n}\n',
)
