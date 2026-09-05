from pathlib import Path

path = Path('frontend/src/lib/state/libraryWorkflow.svelte.ts')
text = path.read_text()
text = text.replace(
    "  let initialOpaqueRestorePending = Boolean(initialOpaqueToken);",
    "  let initialOpaqueRestorePending = $state(Boolean(initialOpaqueToken));",
    1,
)
old_restore = '''    initialOpaqueRestorePending = false;\n    const controller = new AbortController();\n    resolveOpaqueState(initialOpaqueToken, controller.signal).then(applyRestoredLibraryState).catch((error) => {\n      if (controller.signal.aborted) return;\n      console.warn('Unable to restore protected library URL state', error);\n      restoredStateKey = stateKey(defaultLibraryURLState);\n      window.history.replaceState(null, '', pathForAppRoute('library'));\n    });'''
new_restore = '''    const controller = new AbortController();\n    resolveOpaqueState(initialOpaqueToken, controller.signal).then((state) => {\n      if (controller.signal.aborted) return;\n      applyRestoredLibraryState(state);\n      initialOpaqueRestorePending = false;\n    }).catch((error) => {\n      if (controller.signal.aborted) return;\n      console.warn('Unable to restore protected library URL state', error);\n      restoredStateKey = stateKey(defaultLibraryURLState);\n      window.history.replaceState(null, '', pathForAppRoute('library'));\n      initialOpaqueRestorePending = false;\n    });'''
if old_restore in text:
    text = text.replace(old_restore, new_restore, 1)
elif new_restore not in text:
    raise SystemExit('initial opaque restore pattern not found')

old_generation = '''    const pathname = pathForAppRoute(route);\n    if (route !== 'library') {'''
new_generation = '''    const pathname = pathForAppRoute(route);\n    const generation = ++historyGeneration;\n    if (route !== 'library') {'''
if old_generation in text:
    text = text.replace(old_generation, new_generation, 1)
elif new_generation not in text:
    raise SystemExit('history generation insertion point not found')
text = text.replace('''    const generation = ++historyGeneration;\n    const replaceLegacy =''', '''    const replaceLegacy =''', 1)
path.write_text(text)
