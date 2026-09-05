# Encryption and protected mode

Gooru can protect the data it owns at rest. The central guarantee is deliberately narrow:

> **Gooru protects its database and the contents of media it manages under upload targets at rest; it does not make the whole filesystem or library opaque.**

This is server-side encryption at rest. It is not end-to-end encryption and it is not transport encryption. Use HTTPS whenever clients connect over an untrusted network. A running Gooru process has the configured key and necessarily serves decrypted data to authenticated clients.

## Enable protected mode

Generate one random 256-bit key and keep it as recovery-critical data. A typical owner-only key file can be created with:

```bash
umask 077
openssl rand -base64 32 > /srv/gooru/encryption.key
```

Set `encryption.enabled: true` and configure exactly one key source. The supported sources are mutually exclusive:

- `encryption.key_file` in YAML, containing the path to the base64 key file;
- `GOORU_ENCRYPTION_KEY`, containing the base64 key directly;
- `GOORU_ENCRYPTION_KEY_FILE`, containing the path to the base64 key file.

For example:

```yaml
encryption:
  enabled: true
  key_file: /run/secrets/gooru-encryption-key
```

On non-Windows systems key files must not be readable or writable by group or others. Secret-manager paths work well with declarative deployments; for example, NixOS/agenix can assign `services.gooru.settings.encryption.key_file` to an age-secret path without putting the key itself in the Nix store.

Back the key up separately from the encrypted data. Losing the key means losing access to the encrypted database and Gooru-managed encrypted media. A backup of only the encrypted data is not sufficient recovery material.

The configured key is a master recovery key. Gooru derives independent cryptographic subkeys for the database and managed media instead of using the same encryption key material for both domains. Existing protected installations created before this separation are migrated in place on startup while the original master key remains configured.

## What is encrypted

When protected mode is enabled, Gooru encrypts:

- the Gooru SQLite database, including SQLite sidecars and temporary storage handled by the encrypted VFS;
- contents of files Gooru manages under configured upload targets;
- new writes to Gooru-managed storage through the protected storage capability.

When an existing plaintext installation first enters protected mode, Gooru migrates the database and registered files under configured upload targets before serving requests. Plaintext derivative-cache entries from ordinary mode are removed on the transition. Protected media derivatives are generated without the ordinary persistent plaintext derivative cache and protected responses use private/no-store cache policy.

Arbitrary external or indexed library files are intentionally not rewritten or encrypted. They remain whatever they already are on disk.

## What is not hidden

The managed-file format protects file contents, not filesystem metadata. Protected mode does not hide filenames, directory layout, extensions, file existence/count, filesystem timestamps, or all size information. It also does not encrypt unrelated files elsewhere on the host.

Enabling encryption is not secure erasure. Previous plaintext copies can remain in old backups, filesystem snapshots/history, externally managed caches, browser traces, or other copies made before protected mode was enabled. Review those separately when the threat model requires it.

## Browser and request privacy

Viewed media necessarily exists as plaintext inside the browser while it is being displayed. Gooru minimizes durable client-side traces, but an uncleared browser profile is not an encrypted environment.

Protected mode defaults `encryption.opaque_url_state` to `true`. Library/search navigation state is represented in browser-visible routes by authenticated opaque tokens rather than readable query text. Tokens are bound to the authenticated user, expire after 30 days, and remain usable across server restarts while the same encryption key remains configured. Reload, back/forward navigation, and bookmarks work while a token remains valid. The plaintext mapping is not stored in browser persistence.

The WebUI also sends free-form protected-mode searches in POST request bodies rather than request URLs. This reduces leakage through browser history, copied URLs, reverse-proxy access logs, and URL-oriented tracing. HTTPS is still required because request bodies and decrypted responses otherwise remain visible in transit.

Disabling opaque URL state deliberately restores self-contained readable URLs and their corresponding history/log leakage tradeoff.

## Disable, key changes, and recovery

The current supported lifecycle is intentionally fail-safe:

- **Disabling encryption in place is not supported.** If an existing database is encrypted and `encryption.enabled` is turned off, configured commands fail with an explicit diagnostic rather than attempting to interpret encrypted data as plaintext.
- **Automatic administrator-driven re-key/key rotation is not yet supported.** Replacing the configured master key for an existing encrypted installation causes opening to fail. Restore the original key instead of repeatedly trying new keys against the live data. Internal format upgrades may migrate ciphertext between Gooru-owned subkeys while preserving the configured master key.
- **Intentional decrypt/migration back to plaintext is not yet provided as an administrative workflow.** Do not disable protected mode expecting an automatic reverse migration.
- **Recovery requires the original key and recoverable encrypted data.** If the key is lost and no backup exists, Gooru has no recovery key or escrow mechanism that can decrypt the data.

Before changing encryption configuration, keep a tested backup of both the encrypted data and the original key. Future explicit re-key/decrypt tooling should be used instead of manual file replacement once such a workflow exists.

## Operational boundary

Protected mode is aimed at at-rest exposure such as another local user or an offline copy of the storage, not at defending against a fully compromised running server. Anyone able to control the running Gooru process, read its process memory, or impersonate an authenticated client may be able to obtain decrypted content.

For configuration option defaults and the complete server reference, see [CONFIG.md](CONFIG.md).
