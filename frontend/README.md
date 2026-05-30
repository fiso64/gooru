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
