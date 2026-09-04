# Gooru frontend

Static SvelteKit application served by `gooru serve` in production.

## Development

```bash
npm ci
npm run check
npm run test:unit
npm run dev
```

The dev server is for frontend work. When testing against real library/API behavior, run the Go server separately and use an appropriate local proxy/same-origin setup.

## Production build

```bash
npm run build
```

Static output is written to `frontend/build`, which the Go server serves through `server.frontend_dir`.

## End-to-end tests

```bash
npm run test:e2e
```

## API types

`../docs/openapi.yaml` is the public API contract. Regenerate TypeScript definitions after endpoint/schema changes:

```bash
npm run generate:api
```

`src/lib/api/openapi.ts` is generated. `src/lib/api/client.ts` is the thin `openapi-fetch` facade used by the frontend query modules.

See [../docs/DEVELOPMENT.md](../docs/DEVELOPMENT.md) for the repository-wide workflow and [../docs/SERVE.md](../docs/SERVE.md) for runtime deployment.
