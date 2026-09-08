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

The configured key is a master recovery key. Gooru derives independent cryptographic subkeys for the database, managed media, and derivative cache instead of using the same encryption key material for those domains. Existing protected installations created before this separation are migrated in place on startup while the original master key remains configured.

## What is encrypted

When protected mode is enabled, Gooru encrypts:

- the Gooru SQLite database, including SQLite sidecars and temporary storage handled by the encrypted VFS;
- contents of files Gooru manages under configured upload targets;
- persistent Gooru-generated media derivatives in a separate protected cache namespace;
- new writes to Gooru-managed storage through the protected storage capability.

When an existing plaintext installation first enters protected mode, Gooru migrates the database and registered files under configured upload targets before serving requests. Plaintext derivative-cache entries from ordinary mode are removed on the transition. Protected derivatives persist only as authenticated encrypted cache entries; cache hits are decrypted in memory and protected responses use private/no-store cache policy.

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

Protected mode can be disabled in place, but the original recovery key must remain available for the one-time restoration startup:

1. Keep the existing `encryption.key_file`, `GOORU_ENCRYPTION_KEY`, or `GOORU_ENCRYPTION_KEY_FILE` source unchanged.
2. Set `encryption.enabled: false` and start a Gooru command that opens the configured database (normally the server).
3. Gooru opens the encrypted database with the recovery key, restores registered media under managed upload targets to plaintext at their ordinary canonical paths, removes the disposable encrypted derivative-cache namespace, and converts the database to ordinary plaintext **last**.
4. After that startup completes successfully, the database and managed media are ordinary plaintext storage again. The recovery-key source is no longer required and may be removed from the runtime configuration.

The ordering is deliberate and restart-safe. Managed files are replaced atomically one at a time, and an already-restored plaintext file is accepted on retry. Until all managed media and protected-cache cleanup succeed, the database remains encrypted so a later startup can still enumerate and continue the remaining work. The database itself is copied to a plaintext sibling, integrity-checked, atomically swapped, reopened, and verified before its encrypted rollback copy is removed.

If the process is interrupted during the disable transition, restart it with `encryption.enabled: false` and the **same original recovery key** still configured. Do not remove the key source until a startup succeeds. A missing or wrong key fails before Gooru begins restoring managed media, and an encrypted database is never opened as an ordinary plaintext database.

Other lifecycle constraints still apply:

- **Automatic administrator-driven re-key/key rotation is not yet supported.** Replacing the configured master key for an existing encrypted installation causes opening to fail. Restore the original key instead of repeatedly trying new keys against the live data. Internal format upgrades may migrate ciphertext between Gooru-owned subkeys while preserving the configured master key.
- **Recovery requires the original key and recoverable encrypted data until the disable transition completes.** If the key is lost and no backup exists, Gooru has no recovery key or escrow mechanism that can decrypt the data.
- Disabling protected mode is not secure erasure of old ciphertext or prior plaintext copies. Backups, snapshots, or other external copies remain whatever they were when created.

Before changing encryption configuration, keep a tested backup of both the data and the original key. Keep the key until the reverse migration has completed successfully and ordinary-mode startup has been verified.

## Operational boundary

Protected mode is aimed at at-rest exposure such as another local user or an offline copy of the storage, not at defending against a fully compromised running server. Anyone able to control the running Gooru process, read its process memory, or impersonate an authenticated client may be able to obtain decrypted content.

For configuration option defaults and the complete server reference, see [CONFIG.md](CONFIG.md).
