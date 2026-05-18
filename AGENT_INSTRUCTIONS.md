# Project-specific Codex instructions

## Validation

Before reporting candidate completion, run:

```bash
go build ./...
go test ./...
```

If a narrower validation command is appropriate for an intermediate checkpoint, document why. Final candidate completion should use the full build and test commands above unless blocked.
