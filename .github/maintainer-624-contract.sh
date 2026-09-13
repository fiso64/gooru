#!/usr/bin/env bash
set -euo pipefail

python3 - <<'PY'
from pathlib import Path

openapi = Path('docs/openapi.yaml')
text = openapi.read_text()
start = text.index('  /operations:\n')
end = text.index('  /operations/cancel-all:\n', start)
section = text[start:end]
old_limit = '''        - name: limit
          in: query
          required: false
          schema:
            type: integer
            minimum: 1
            maximum: 1000
            default: 100
'''
new_limit = old_limit + '''        - name: offset
          in: query
          required: false
          description: Zero-based number of visible operations to skip.
          schema:
            type: integer
            minimum: 0
            default: 0
'''
if section.count(old_limit) != 1 or '        - name: offset\n' in section:
    raise SystemExit('unexpected /operations limit/offset contract state')
section = section.replace(old_limit, new_limit, 1)
text = text[:start] + section + text[end:]

events = '''  /operations/events:
    get:
      summary: Stream durable background operation change signals.
      description: Sends payload-free `operations` server-sent events after committed Jobs-visible operation or task state changes, plus an initial reconciliation signal and periodic keepalive comments. Requires an authenticated administrator session.
      security:
        - sessionAuth: []
      responses:
        "200":
          description: Long-lived server-sent event stream of Jobs invalidation signals.
          content:
            text/event-stream:
              schema:
                type: string
        "401":
          $ref: "#/components/responses/Unauthorized"
        "403":
          $ref: "#/components/responses/Forbidden"
        "503":
          $ref: "#/components/responses/ServiceUnavailable"
'''
anchor = '  /operations/cancel-all:\n'
if '  /operations/events:\n' in text or text.count(anchor) != 1:
    raise SystemExit('unexpected operations/events contract state')
text = text.replace(anchor, events + anchor, 1)
openapi.write_text(text)

ci = Path('.github/workflows/ci.yml')
ci_text = ci.read_text()
old = 'jobs-layout.spec.ts jobs-pagination.spec.ts jobs-history-controls.spec.ts'
new = 'jobs-layout.spec.ts jobs-pagination.spec.ts jobs-polling.spec.ts jobs-history-controls.spec.ts'
if ci_text.count(old) != 1 or 'jobs-pagination.spec.ts jobs-polling.spec.ts' in ci_text:
    raise SystemExit('unexpected browser regression command state')
ci.write_text(ci_text.replace(old, new, 1))
PY

cd frontend
npm ci --prefer-offline --no-audit
npm run generate:api
cd ..

git diff --check
git status --short

rm .github/workflows/maintainer-624-contract.yml .github/maintainer-624-contract.sh
git config user.name 'github-actions[bot]'
git config user.email '41898282+github-actions[bot]@users.noreply.github.com'
git add .github/workflows/ci.yml docs/openapi.yaml frontend/src/lib/api/openapi.ts .github/workflows/maintainer-624-contract.yml .github/maintainer-624-contract.sh
git diff --cached --check
git diff --cached --name-only
git commit -m 'Sync jobs API contract and browser coverage'
git push origin HEAD:maintainer/624-jobs-api-ui
