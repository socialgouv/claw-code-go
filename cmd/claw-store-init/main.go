// Command claw-store-init scaffolds a local plugin marketplace
// directory the user can serve via static HTTP for team-internal
// plugin distribution without a public registry.
//
// Usage:
//
//	claw-store-init [--dir <path>]
//
// Default <path> is "$HOME/.claw/marketplace". The scaffolder writes
// a catalog.json template, a README, and a tiny serve.go that fronts
// the directory over HTTP. To actually serve:
//
//	go run serve.go --port 8080
//
// Then point the user's CLAW_MARKETPLACE_URL at
// http://localhost:8080 and `/store` slash commands resolve against
// the local catalog.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
)

const catalogTemplate = `{
  "version": 1,
  "plugins": [
    {
      "name": "example-plugin",
      "version": "0.1.0",
      "description": "Replace this entry with your real plugins.",
      "tarball_url": "http://localhost:8080/plugins/example-plugin-0.1.0.tar.gz",
      "sha256": "0000000000000000000000000000000000000000000000000000000000000000",
      "homepage": "https://example.com",
      "license": "MIT"
    }
  ]
}
`

const serveTemplate = `// serve.go fronts the local marketplace directory over HTTP. Run with:
//
//   go run serve.go --port 8080
package main

import (
	"flag"
	"log"
	"net/http"
)

func main() {
	port := flag.String("port", "8080", "HTTP port to listen on")
	dir := flag.String("dir", ".", "Directory to serve")
	flag.Parse()

	log.Printf("serving %s on http://localhost:%s/", *dir, *port)
	if err := http.ListenAndServe(":"+*port, http.FileServer(http.Dir(*dir))); err != nil {
		log.Fatal(err)
	}
}
`

const readmeTemplate = `# Local plugin marketplace

This directory holds a catalog.json template that ` + "`/store`" + ` slash commands
in claw-code-go (or iterion) can resolve against.

## Quickstart

1. Edit ` + "`catalog.json`" + ` to add your plugin entries.
2. Drop your plugin tarballs under ` + "`plugins/`" + `.
3. Compute SHA-256 for each tarball:
   ` + "`sha256sum plugins/your-plugin-1.0.0.tar.gz`" + `
   and update the catalog entry.
4. Serve:
   ` + "`go run serve.go --port 8080`" + `
5. Point your client:
   ` + "`export CLAW_MARKETPLACE_URL=http://localhost:8080`" + `

That's it. ` + "`/store search`" + `, ` + "`/store install`" + `, etc. now resolve
against this directory.

## Optional: signed plugins

See docs/plugin_signing.md in the claw-code-go repo and populate the
` + "`signature_url`" + `, ` + "`signature_bundle`" + `, ` + "`certificate_identity`" + `, and
` + "`certificate_oidc_issuer`" + ` fields. The installer verifies cosign
signatures via the ` + "`cosign`" + ` CLI on PATH; set
` + "`CLAW_REQUIRE_SIGNED=1`" + ` to reject unsigned plugins, or
` + "`CLAW_PLUGIN_PUBLIC_KEY=/path/to/cosign.pub`" + ` for key-based mode.

## Two-tier marketplace (` + "`/index.json`" + ` + per-plugin ` + "`manifest.json`" + `)

The ` + "`catalog.json`" + ` produced above feeds the legacy ` + "`/store`" + ` slash
command. To also serve the public ` + "`plugin.RemoteMarketplace`" + ` API
(used by ` + "`claw-code-go plugin install --marketplace <url> <name>`" + `),
publish the same content under the two-tier layout described in
` + "`docs/plugin_signing.md`" + `:

` + "```" + `
<base>/index.json                          # { plugins: [{ name, latest_version, url, ... }] }
<base>/<plugin>/manifest.json              # PluginManifest + tarball_url + sha256 + signature_*
<base>/<plugin>/<plugin>-<ver>.tar.gz
<base>/<plugin>/<plugin>-<ver>.tar.gz.sig  # optional
` + "```" + `
`

func main() {
	dir := flag.String("dir", "", "Directory to scaffold (default: $HOME/.claw/marketplace)")
	flag.Parse()

	target := *dir
	if target == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			fmt.Fprintf(os.Stderr, "claw-store-init: cannot resolve home dir: %v\n", err)
			os.Exit(1)
		}
		target = filepath.Join(home, ".claw", "marketplace")
	}

	if err := os.MkdirAll(filepath.Join(target, "plugins"), 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "claw-store-init: mkdir %s: %v\n", target, err)
		os.Exit(1)
	}

	files := []struct {
		path string
		body string
		mode os.FileMode
	}{
		{filepath.Join(target, "catalog.json"), catalogTemplate, 0o644},
		{filepath.Join(target, "serve.go"), serveTemplate, 0o644},
		{filepath.Join(target, "README.md"), readmeTemplate, 0o644},
	}
	for _, f := range files {
		if _, err := os.Stat(f.path); err == nil {
			fmt.Fprintf(os.Stderr, "claw-store-init: %s already exists, leaving untouched\n", f.path)
			continue
		}
		if err := os.WriteFile(f.path, []byte(f.body), f.mode); err != nil {
			fmt.Fprintf(os.Stderr, "claw-store-init: write %s: %v\n", f.path, err)
			os.Exit(1)
		}
		fmt.Printf("wrote %s\n", f.path)
	}

	fmt.Printf("\nLocal marketplace scaffolded at %s\n", target)
	fmt.Printf("Next: edit catalog.json, drop tarballs in %s/plugins/, then run:\n", target)
	fmt.Printf("  cd %s && go run serve.go --port 8080\n", target)
	fmt.Printf("  export CLAW_MARKETPLACE_URL=http://localhost:8080\n")
}
