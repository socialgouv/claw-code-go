package compat

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/SocialGouv/claw-code-go/internal/plugins"
	"github.com/SocialGouv/claw-code-go/plugin"
)

// RunPluginInstall implements the `plugin install --marketplace <url> <name>`
// subcommand. It resolves the plugin in a remote marketplace, verifies the
// SHA-256 (and cosign signature when present), and extracts the tarball
// into the user's plugin directory.
func RunPluginInstall(args []string) {
	if err := runPluginInstall(args, os.Stdout, os.Stderr); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func runPluginInstall(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("plugin install", flag.ContinueOnError)
	fs.SetOutput(stderr)
	marketplaceFlag := fs.String("marketplace", "", "Remote marketplace base URL (default: $CLAW_MARKETPLACE_URL)")
	destFlag := fs.String("dest", "", "Install destination root (default: $XDG_DATA_HOME/claw/plugins or platform default)")
	requireSignedFlag := fs.Bool("require-signed", false, "Reject plugins without cosign signature material (also via CLAW_REQUIRE_SIGNED=1)")
	insecureFlag := fs.Bool("insecure-marketplace", false, "Allow http:// marketplace URLs (development only)")
	timeoutFlag := fs.Duration("timeout", 60*time.Second, "Total install timeout")
	fs.Usage = func() {
		fmt.Fprintf(stderr, "Usage: claw-code-go plugin install --marketplace <url> [--dest <dir>] [--require-signed] [--insecure-marketplace] <plugin-name>\n\n")
		fs.PrintDefaults()
	}

	if err := fs.Parse(args); err != nil {
		return err
	}
	rest := fs.Args()
	if len(rest) != 1 {
		fs.Usage()
		return fmt.Errorf("plugin install: exactly one plugin name is required")
	}
	name := rest[0]

	mktURL := strings.TrimSpace(*marketplaceFlag)
	if mktURL == "" {
		mktURL = strings.TrimSpace(os.Getenv("CLAW_MARKETPLACE_URL"))
	}
	if mktURL == "" {
		return fmt.Errorf("plugin install: --marketplace or $CLAW_MARKETPLACE_URL is required")
	}

	dest := strings.TrimSpace(*destFlag)
	if dest == "" {
		def, err := plugins.DefaultPluginDir()
		if err != nil {
			return fmt.Errorf("plugin install: resolve default plugin dir: %w", err)
		}
		dest = def
	}

	requireSigned := *requireSignedFlag || os.Getenv("CLAW_REQUIRE_SIGNED") == "1"

	opts := []plugin.MarketplaceOption{
		plugin.WithMarketplaceRequireSigned(requireSigned),
		plugin.WithMarketplaceAllowInsecure(*insecureFlag),
	}
	if pubKey := strings.TrimSpace(os.Getenv("CLAW_PLUGIN_PUBLIC_KEY")); pubKey != "" {
		opts = append(opts, plugin.WithMarketplaceVerifier(&plugins.CosignVerifier{PublicKeyFile: pubKey}))
	}

	mkt, err := plugin.NewRemoteMarketplace(mktURL, opts...)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), *timeoutFlag)
	defer cancel()
	// Translate Ctrl+C into context cancel so the in-flight download aborts
	// promptly rather than hanging until the timeout fires.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(sigCh)
	go func() {
		select {
		case <-sigCh:
			cancel()
		case <-ctx.Done():
		}
	}()

	manifest, err := mkt.Install(ctx, name, dest)
	if err != nil {
		return err
	}

	fmt.Fprintf(stdout, "Installed %s@%s into %s\n", manifest.Name, manifest.Version, dest)
	if manifest.Description != "" {
		fmt.Fprintf(stdout, "  description: %s\n", manifest.Description)
	}
	fmt.Fprintf(stdout, "  sha256: %s\n", manifest.SHA256)
	if manifest.SignatureURL != "" || manifest.SignatureBundle != "" {
		fmt.Fprintln(stdout, "  signature: verified (cosign)")
	}
	return nil
}
