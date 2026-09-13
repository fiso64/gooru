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
if '        - name: offset\n' not in section:
    if section.count(old_limit) != 1:
        raise SystemExit('unexpected /operations limit contract state')
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
if '  /operations/events:\n' not in text:
    if text.count(anchor) != 1:
        raise SystemExit('unexpected operations/cancel-all contract state')
    text = text.replace(anchor, events + anchor, 1)
openapi.write_text(text)
PY

cd frontend
npm ci --prefer-offline --no-audit
npm run generate:api
cd ..

git diff --check
git status --short

if git diff --quiet -- docs/openapi.yaml frontend/src/lib/api/openapi.ts; then
  echo 'OpenAPI contract already synchronized.'
  exit 0
fi

git config user.name 'github-actions[bot]'
git config user.email '41898282+github-actions[bot]@users.noreply.github.com'
git add docs/openapi.yaml frontend/src/lib/api/openapi.ts
git diff --cached --check
git diff --cached --name-only
git commit -m 'Sync jobs API contract'
git push origin HEAD:maintainer/624-jobs-api-ui
