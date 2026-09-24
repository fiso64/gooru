# CI layout

The default pull-request gate is [CI](workflows/ci.yml). Go tests/build and
frontend type checks, unit tests, a production build, CSP validation and ordinary
Playwright browser regressions run on same-repository PRs and pushes to
`develop`. The root-selection browser integration test also runs against a
freshly initialized Go server. The community workflow uses GitHub-hosted runners
and executes the same automatically discovered frontend regression suite for
external PRs without granting repository write credentials or using self-hosted
runners.

## Browser test discovery

`frontend/playwright.config.ts` discovers all `frontend/tests/*.spec.ts`.
The credentialed, destructive `smoke-upload-library.spec.ts` is excluded by
`testIgnore` and runs **only** via `playwright.smoke.config.ts` and the
manual [E2E Smoke](workflows/e2e-smoke.yml) workflow. New ordinary browser tests
run in CI automatically: do not add filenames to workflow commands.

CI builds the frontend once, checks its CSP, and passes
`GOORU_E2E_PREBUILT=1` to Playwright to use that built output. A standalone
local `npm run test:e2e` still builds before starting the preview server. If
testing an existing build locally, run `npm run build` first and explicitly
set `GOORU_E2E_PREBUILT=1`. CI chooses a free preview port via
`GOORU_E2E_PORT` because the self-hosted runners share loopback ports.

To run the ordinary browser suite:

```sh
cd frontend
npm ci
npx playwright install chromium
npm run test:e2e
```

To inspect only one regression, pass the test filename to `npm run test:e2e --`.
A genuine backend integration run sets `GOORU_E2E_BASE_URL` to the isolated
server instead of starting Playwright's mocked preview web server. The normal
CI gate uses `--max-failures=3` to limit wasted time when a shared fixture
breaks; a clean run still executes **every** ordinary test.

## Package / release validation

The ordinary CI gate builds the production frontend on each application PR.
Expensive distribution-specific workflows are reserved for their packaging
inputs and explicit manual dispatch:

- [Debian packages](workflows/validate-deb.yml): Debian scripts, service
  definitions, Go dependency metadata, or production frontend build/dependency
  configuration.
- [Portable release artifacts](workflows/validate-portable.yml): portable build
  scripts, Go dependency metadata, or production frontend build/dependency
  configuration.
- [Nix package](workflows/nix-package.yml): Nix and build identity inputs, Go
  dependency metadata, or production frontend build/dependency configuration.
  Publication on `main` is unchanged.
- [Release](workflows/release.yml): a `VERSION`-changing PR runs the complete
  package matrix. Version tags also run release validation and publishing.
- [Debian service package](workflows/deb-service-package.yml): changes to the
  service or instance-management packaging and pushes to `main`.
- [E2E Smoke](workflows/e2e-smoke.yml): manually started, isolated upload/delete
  benchmarking.

Ordinary `frontend/src/**` changes no longer rebuild Nix, Debian and portable
artifacts in *separate* PR checks. The normal CI production build is still
required, and the release/version validation remains comprehensive. Packaging
workflows can always be run explicitly when an application change needs
distribution-specific coverage.

**Branch protection:** Treat the always-triggered `CI / Go` and
`CI / Frontend` jobs as PR gates. Path-filtered or event-specific packaging
workflows can legitimately be absent on unrelated PRs; do not add their job
names as unconditionally required checks without an always-running aggregate
gate.
