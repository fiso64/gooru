from pathlib import Path

def replace(path: str, old: str, new: str) -> None:
    p = Path(path)
    text = p.read_text()
    if text.count(old) != 1:
        raise SystemExit(f'unexpected state in {path}: {old[:80]!r}')
    p.write_text(text.replace(old, new, 1))

replace(
    'frontend/src/lib/components/AuthenticatedApp.svelte',
    "  const jobsQuery = createJobsQuery(() => Boolean($authState.user), () => authScope);\n",
    "  const jobsQuery = createJobsQuery(() => Boolean($authState.user), () => authScope, () => 50);\n"
)
replace(
    'frontend/src/lib/components/AuthenticatedApp.svelte',
    "    jobs={jobsQuery.data?.items ?? []}\n",
    "    jobs={(jobsQuery.data?.items ?? []).slice(0, 20)}\n"
)
replace(
    'frontend/tests/jobs-pagination.spec.ts',
    "  await expect.poll(() => requested(requests, 20, 0)).toBe(true);\n  expect(requests.every((request) => request.limit <= 50)).toBe(true);\n",
    "  await expect.poll(() => requested(requests, 50, 0)).toBe(true);\n  expect(requests.some((request) => request.limit === 20)).toBe(false);\n  expect(requests.every((request) => request.limit <= 50)).toBe(true);\n"
)
