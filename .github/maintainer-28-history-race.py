from pathlib import Path

workflow = Path('frontend/src/lib/state/libraryWorkflow.svelte.ts')
text = workflow.read_text()
text = text.replace('  let historyGeneration = 0;\n', '  let historyGeneration = 0;\n  let restoreGeneration = 0;\n', 1)
old = '''    const restoreRoute = async () => {\n      const nextRoute = appRouteFromPath(window.location.pathname);\n      route = nextRoute;\n      if (nextRoute !== 'library') {\n        applyRestoredLibraryState(defaultLibraryURLState);\n        return;\n      }\n      const token = opaqueURLState ? (new URLSearchParams(window.location.search).get('state')?.trim() ?? '') : '';\n      try {\n        const nextLibraryState = token ? await resolveOpaqueState(token) : libraryURLStateFromSearch(window.location.search);\n        applyRestoredLibraryState(nextLibraryState);\n      } catch (error) {\n        console.warn('Unable to restore protected library history state', error);\n        applyRestoredLibraryState(defaultLibraryURLState);\n        window.history.replaceState(null, '', pathForAppRoute('library'));\n      }\n    };'''
new = '''    const restoreRoute = async () => {\n      const generation = ++restoreGeneration;\n      const nextRoute = appRouteFromPath(window.location.pathname);\n      route = nextRoute;\n      if (nextRoute !== 'library') {\n        applyRestoredLibraryState(defaultLibraryURLState);\n        return;\n      }\n      const token = opaqueURLState ? (new URLSearchParams(window.location.search).get('state')?.trim() ?? '') : '';\n      try {\n        const nextLibraryState = token ? await resolveOpaqueState(token) : libraryURLStateFromSearch(window.location.search);\n        if (generation !== restoreGeneration) return;\n        applyRestoredLibraryState(nextLibraryState);\n      } catch (error) {\n        if (generation !== restoreGeneration) return;\n        console.warn('Unable to restore protected library history state', error);\n        applyRestoredLibraryState(defaultLibraryURLState);\n        window.history.replaceState(null, '', pathForAppRoute('library'));\n      }\n    };'''
if old not in text:
    raise SystemExit('restoreRoute pattern missing')
workflow.write_text(text.replace(old, new, 1))

test = Path('frontend/tests/protected-browser-privacy.spec.ts')
t = test.read_text()
old_route = '''    if (path.startsWith('/api/v1/ui-state/') && request.method() === 'GET') {\n      const token = decodeURIComponent(path.slice('/api/v1/ui-state/'.length));\n      const state = states.get(token);\n      return state ? json(route, state) : json(route, { error: { code: 'not_found', message: 'not found' } }, 404);\n    }'''
new_route = '''    if (path.startsWith('/api/v1/ui-state/') && request.method() === 'GET') {\n      const token = decodeURIComponent(path.slice('/api/v1/ui-state/'.length));\n      const state = states.get(token);\n      if (token === 'opaque-1') await new Promise((resolve) => setTimeout(resolve, 120));\n      return state ? json(route, state) : json(route, { error: { code: 'not_found', message: 'not found' } }, 404);\n    }'''
if old_route not in t:
    raise SystemExit('ui-state route pattern missing')
t = t.replace(old_route, new_route, 1)
old_tail = '''  await page.goForward();\n  await expectCommittedTokens(page, [firstToken, secondToken]);\n  expect(page.url()).toBe(secondURL);\n});'''
new_tail = '''  await page.goForward();\n  await expectCommittedTokens(page, [firstToken, secondToken]);\n  expect(page.url()).toBe(secondURL);\n\n  // A slow restore for the older history entry must not overwrite a newer one.\n  await page.goBack({ waitUntil: 'commit' });\n  await page.goForward({ waitUntil: 'commit' });\n  await expectCommittedTokens(page, [firstToken, secondToken]);\n  await page.waitForTimeout(180);\n  await expectCommittedTokens(page, [firstToken, secondToken]);\n  expect(page.url()).toBe(secondURL);\n});'''
if old_tail not in t:
    raise SystemExit('history test tail pattern missing')
test.write_text(t.replace(old_tail, new_tail, 1))
