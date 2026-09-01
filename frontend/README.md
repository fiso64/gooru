# Gooru Frontend

Static SvelteKit app served by `gooru serve`.

## Development

```bash
npm install
npm run dev
```

The dev server expects the Go API at the same origin in production. During local frontend-only work, run the API separately or configure a Vite proxy in a follow-up slice.

## Production Build

```bash
npm run check
npm run test:unit
npm run build
```

The static output is written to `frontend/build`. `gooru serve` serves that directory by default through `server.frontend_dir`.

See [../docs/SERVE.md](../docs/SERVE.md) for the full runtime configuration and
deployment notes.

## API client

`../docs/openapi.yaml` is the API source of truth. Regenerate the frontend API
types before changing endpoint shapes:

```bash
npm run generate:api
```

`src/lib/api/openapi.ts` is generated and `src/lib/api/client.ts` is a thin
`openapi-fetch` facade used by the Svelte query modules.
