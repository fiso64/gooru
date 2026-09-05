from pathlib import Path


def replace_once(path: str, old: str, new: str) -> None:
    p = Path(path)
    text = p.read_text()
    count = text.count(old)
    if count != 1:
        raise SystemExit(f"{path}: expected one marker, found {count}")
    p.write_text(text.replace(old, new, 1))


replace_once(
    "frontend/src/lib/components/AuthenticatedApp.svelte",
    "    jobsActiveCount={activeJobs.length}\n",
    "    jobsActiveCount={jobsQuery.data?.active_count ?? activeJobs.length}\n",
)
replace_once(
    "frontend/src/lib/components/AuthenticatedApp.svelte",
    "<JobsView jobs={jobsQuery.data?.items ?? []} onCancel={cancelJob} onClearCompleted={clearCompletedJobs} />",
    "<JobsView jobs={jobsQuery.data?.items ?? []} {authScope} onCancel={cancelJob} onClearCompleted={clearCompletedJobs} />",
)
replace_once(
    "frontend/src/lib/components/JobsView.svelte",
    "    jobs,\n    onCancel,",
    "    jobs,\n    authScope,\n    onCancel,",
)
replace_once(
    "frontend/src/lib/components/JobsView.svelte",
    "    jobs: Job[];\n    onCancel:",
    "    jobs: Job[];\n    authScope: number;\n    onCancel:",
)
replace_once(
    "frontend/src/lib/components/JobsView.svelte",
    "    () => 0,\n    () => 50,",
    "    () => authScope,\n    () => 50,",
)

spec = Path("frontend/tests/jobs-pagination.spec.ts")
text = spec.read_text()
old = "        active_count: 0,"
if text.count(old) < 1:
    raise SystemExit("jobs-pagination.spec.ts: missing active_count fixture")
text = text.replace(old, "        active_count: 7,", 1)
marker = "  await expect(drawer.locator('.job-row')).toHaveCount(20);\n"
if text.count(marker) != 1:
    raise SystemExit("jobs-pagination.spec.ts: drawer marker not unique")
text = text.replace(marker, marker + "  await expect(topbarJobs).toContainText('7');\n", 1)
spec.write_text(text)

replace_once(
    "docs/openapi.yaml",
    """        - name: status
          in: query
          required: false
          schema:
            type: string
            enum: [pending, running, completed, failed, canceled]
      responses:""",
    """        - name: status
          in: query
          required: false
          schema:
            type: string
            enum: [pending, running, completed, failed, canceled]
        - name: limit
          in: query
          required: false
          schema:
            type: integer
            minimum: 1
            maximum: 200
            default: 50
        - name: page_token
          in: query
          required: false
          schema:
            type: string
      responses:""",
)
replace_once(
    "docs/openapi.yaml",
    """    JobListResponse:
      type: object
      required: [items]
      properties:
        items:""",
    """    JobListResponse:
      type: object
      required: [items, active_count]
      properties:
        active_count:
          type: integer
          minimum: 0
          description: Total retained jobs currently pending or running, independent of the visible page.
        next_page_token:
          type: string
        items:""",
)
