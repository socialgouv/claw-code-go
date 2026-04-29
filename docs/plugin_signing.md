# Plugin signature verification (status: schema only)

The marketplace catalog (`PluginEntry`) carries optional sigstore
fields:

| Field | Purpose |
|---|---|
| `signature_url` | URL pointing at a detached cosign signature blob |
| `signature_bundle` | Inline cosign bundle (when no separate URL) |
| `certificate_identity` | Pin the expected signer identity (e.g. email, GitHub Actions URI) |
| `certificate_oidc_issuer` | Pin the expected OIDC issuer URL |

The installer **does not yet verify these fields cryptographically**.
SHA-256 of the tarball is the only enforced integrity check today.

## Why deferred

Adding `github.com/sigstore/sigstore-go` is a substantive dependency
choice (~30 transitive modules, public-key infra, certificate trust
roots). Landing it requires:

- Pinning a sigstore-go release that is module-clean (no replace
  directives that conflict with iterion's vendoring).
- Deciding the trust policy: keyless via OIDC issuer + identity
  matchers, key-based via a host-pinned public key, or both.
- Surfacing a CLI flag (`--require-signed`, `--allow-unsigned`) so
  operators can opt into strict mode incrementally.

These are ecosystem-level choices, not mechanical edits, so the
current iteration ships the schema and signing flow documentation
ahead of the verification engine.

## Signing flow (for catalog authors)

Use `cosign` with keyless signing (Sigstore Public Good Instance):

```bash
# Build the tarball
tar czf my-plugin-1.0.0.tar.gz -C plugin-dir .

# Sign keylessly (opens browser for OIDC)
cosign sign-blob --bundle my-plugin-1.0.0.bundle.json my-plugin-1.0.0.tar.gz
```

Then in your catalog entry:

```json
{
  "name": "my-plugin",
  "version": "1.0.0",
  "tarball_url": "https://example.com/my-plugin-1.0.0.tar.gz",
  "sha256": "abc123...",
  "signature_bundle": "<inline contents of my-plugin-1.0.0.bundle.json>",
  "certificate_identity": "ci@example.com",
  "certificate_oidc_issuer": "https://accounts.google.com"
}
```

When the verification engine lands, the installer will reject any
plugin whose signature doesn't match the pinned identity / issuer.

## Tracking issue

Track the verification implementation under
`docs/roadmap_progress.md` Track 7.
