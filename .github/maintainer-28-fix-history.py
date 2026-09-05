from pathlib import Path

path = Path('frontend/src/lib/state/libraryWorkflow.svelte.ts')
text = path.read_text()
text = text.replace(
    "  let initialOpaqueRestorePending = Boolean(initialOpaqueToken);",
    "  let initialOpaqueRestorePending = $state(Boolean(initialOpaqueToken));",
    1,
)
old = '''    initialOpaqueRestorePending = false;\n    const controller = new AbortController();\n    resolveOpaqueState(initialOpaqueToken, controller.signal).then(applyRestoredLibraryState).catch((error) => {\n      if (controller.signal.aborted) return;\n      console.warn('Unable to restore protected library URL state', error);\n      restoredStateKey = stateKey(defaultLibraryURLState);\n      window.history.replaceState(null, '', pathForAppRoute('library'));\n    });'''
new = '''    const controller = new AbortController();\n    resolveOpaqueState(initialOpaqueToken, controller.signal).then((state) => {\n      if (controller.signal.aborted) return;\n      applyRestoredLibraryState(state);\n      initialOpaqueRestorePending = false;\n    }).catch((error) => {\n      if (controller.signal.aborted) return;\n      console.warn('Unable to restore protected library URL state', error);\n      restoredStateKey = stateKey(defaultLibraryURLState);\n      window.history.replaceState(null, '', pathForAppRoute('library'));\n      initialOpaqueRestorePending = false;\n    });'''
if old not in text:
    raise SystemExit('initial opaque restore pattern not found')
path.write_text(text.replace(old, new, 1))
