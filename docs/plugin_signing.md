# Plugin signature verification

The marketplace catalog (`PluginEntry`) carries optional cosign
signature fields:

| Field | Purpose |
|---|---|
| `signature_url` | URL pointing at a detached cosign signature blob (raw `.sig` or `.bundle.json`) |
| `signature_bundle` | Inline cosign bundle (no separate URL fetch) |
| `certificate_identity` | Pin the expected signer identity (e.g. email, GitHub Actions URI) — selects keyless mode |
| `certificate_oidc_issuer` | Pin the expected OIDC issuer URL — selects keyless mode |

When any of these fields are populated **OR** when the operator sets
`CLAW_REQUIRE_SIGNED=1`, the installer invokes
`plugins.SignatureVerifier` after the SHA-256 hash check. Failure
aborts the install before extraction.

## How verification works

The default verifier (`plugins.CosignVerifier`) shells out to the
`cosign` CLI. Subprocess was chosen over an in-process `sigstore-go`
binding to keep the dependency footprint minimal: operators who care
about signing already have `cosign` installed.

### Auto-detect keyless vs key-based

The verifier picks a mode at call time based on what the entry and
environment declare:

- **Keyless** (Fulcio + Rekor) — used when the entry has at least one
  of `certificate_identity` or `certificate_oidc_issuer`. The
  transparency log entry is checked by `cosign verify-blob`
  automatically. Missing matcher fields are wildcarded
  (`--certificate-identity-regexp .*`).
- **Key-based** — used otherwise, when a public key is configured via
  `CLAW_PLUGIN_PUBLIC_KEY` (path to PEM) or programmatically through
  `CosignVerifier.PublicKeyPEM` / `PublicKeyFile`.
- **No material** — install fails with a clear error so misconfigured
  catalogs fail loudly.

### Operator controls

| Control | Effect |
|---|---|
| `CLAW_REQUIRE_SIGNED=1` | Every install must carry signature material. Entries without it are rejected up-front. |
| `CLAW_PLUGIN_PUBLIC_KEY=/path/to/cosign.pub` | PEM file used for key-based verification. |
| (no env vars set) | Default. Signed entries are verified; unsigned entries install with hash-only enforcement. |

Strict-by-default is available by setting `CLAW_REQUIRE_SIGNED=1` in
the operator's shell or service unit.

## Signing flow (for catalog authors)

### Keyless

```bash
tar czf my-plugin-1.0.0.tar.gz -C plugin-dir .
cosign sign-blob --bundle my-plugin-1.0.0.bundle.json my-plugin-1.0.0.tar.gz
```

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

### Key-based

```bash
cosign generate-key-pair                   # produces cosign.key + cosign.pub
cosign sign-blob --key cosign.key \
                 --bundle my-plugin-1.0.0.bundle.json \
                 my-plugin-1.0.0.tar.gz
```

Operators consuming this catalog set
`CLAW_PLUGIN_PUBLIC_KEY=/path/to/cosign.pub` and the install verifies
the signature with the pinned key. The `signature_bundle` field is
sufficient — no certificate fields are required for key-based mode.

## Programmatic configuration

```go
inst := plugins.NewInstaller("/var/lib/claw/plugins")
inst.RequireSigned = true
inst.Verifier = &plugins.CosignVerifier{
    PublicKeyFile: "/etc/claw/cosign.pub", // optional, only for key-based mode
    Timeout:       90 * time.Second,        // optional, defaults to 60 s
}
```

A custom verifier (e.g. one using `sigstore-go` in-process) can be
swapped in by implementing the `SignatureVerifier` interface; the
installer talks to that interface only.

## Limitations

- Subprocess approach assumes `cosign` is on PATH. Install via
  Homebrew, Go (`go install github.com/sigstore/cosign/v2/cmd/cosign@latest`),
  or distribution packages.
- The transparency log lookup runs over the public Sigstore Rekor
  instance by default. Air-gapped deployments need a private Rekor
  and `cosign --rekor-url` — not currently configurable through
  `CosignVerifier` (PRs welcome).

## Remote marketplace layout

`plugin.RemoteMarketplace` (in `plugin/remote_marketplace.go`) consumes a
two-tier static layout that any HTTPS server can host. The `claw-store-init`
scaffolder generates this structure; the `internal/plugins.Marketplace`
client and the `claw-code-go plugin install` command both speak the same
schema, so a single catalog serves both surfaces.

```
<base-url>/
  index.json
  <plugin-name>/
    manifest.json
    <plugin-name>-<version>.tar.gz
    <plugin-name>-<version>.tar.gz.sig         # optional, raw cosign sig
    <plugin-name>-<version>.bundle.json        # optional, sigstore bundle
```

### `index.json`

A list of pointers — minimal so a marketplace can carry hundreds of
plugins without shipping every full manifest in one document.

```json
{
  "version": 1,
  "plugins": [
    {
      "name": "linter",
      "latest_version": "1.0.0",
      "url": "linter/manifest.json",
      "description": "A linter plugin"
    }
  ]
}
```

`url` may be relative to the marketplace base URL (recommended) or an
absolute HTTPS URL. Relative paths resolve against `<base-url>/`.

### Per-plugin `manifest.json`

The full plugin manifest: extends `plugin.PluginManifest` (the
plugin.json schema documented elsewhere) with the install coordinates.

```json
{
  "name": "linter",
  "version": "1.0.0",
  "description": "A linter plugin",
  "permissions": [],
  "defaultEnabled": true,
  "hooks": {"PreToolUse": [], "PostToolUse": [], "PostToolUseFailure": []},
  "lifecycle": {"Init": [], "Shutdown": []},
  "tools": [],
  "commands": [],

  "tarball_url": "https://example.com/linter/linter-1.0.0.tar.gz",
  "sha256": "abc123…",
  "signature_url": "https://example.com/linter/linter-1.0.0.bundle.json",
  "certificate_identity": "ci@example.com",
  "certificate_oidc_issuer": "https://accounts.google.com"
}
```

The signature fields (`signature_url` / `signature_bundle` /
`certificate_identity` / `certificate_oidc_issuer`) are the same ones
`PluginEntry` already accepts and trigger the cosign verification
chain documented above.

## Installing from a remote marketplace

CLI:

```bash
export CLAW_MARKETPLACE_URL=https://plugins.example.com
claw-code-go plugin install linter
```

Flags:

| Flag | Effect |
|---|---|
| `--marketplace <url>` | Override `$CLAW_MARKETPLACE_URL`. |
| `--dest <dir>` | Install root (default: `plugins.DefaultPluginDir()`). |
| `--require-signed` | Reject plugins without signature material. Equivalent to `CLAW_REQUIRE_SIGNED=1`. |
| `--insecure-marketplace` | Permit `http://` URLs. Development only — production must run over HTTPS so an on-path attacker cannot tamper with tarballs before SHA-256 + cosign verification. |
| `--timeout <duration>` | Total install timeout (default: 60s). |

Programmatic:

```go
mkt, err := plugin.NewRemoteMarketplace("https://plugins.example.com",
    plugin.WithMarketplaceRequireSigned(true),
)
if err != nil { return err }
manifest, err := mkt.Install(ctx, "linter", "/var/lib/claw/plugins")
```

The install path: `index.json` → `<plugin>/manifest.json` →
`<plugin>/<plugin>-<version>.tar.gz` (size-capped) → SHA-256 verify →
optional cosign verify-blob → atomic extract under `<dest>/<name>`.
