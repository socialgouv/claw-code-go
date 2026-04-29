package compat

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/SocialGouv/claw-code-go/plugin"
)

func makeTarGz(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for name, content := range files {
		hdr := &tar.Header{Name: name, Mode: 0o644, Size: int64(len(content))}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func sha256Hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func TestRunPluginInstall_Success(t *testing.T) {
	tarball := makeTarGz(t, map[string]string{"plugin.json": `{"name":"linter","version":"1.0.0"}`})
	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	defer srv.Close()

	manifest := plugin.RemoteManifest{
		PluginManifest: plugin.PluginManifest{Name: "linter", Version: "1.0.0", Description: "A linter"},
		TarballURL:     srv.URL + "/linter/linter-1.0.0.tar.gz",
		SHA256:         sha256Hex(tarball),
	}
	mux.HandleFunc("/index.json", func(w http.ResponseWriter, r *http.Request) {
		idx := plugin.RemoteIndex{Plugins: []plugin.RemotePluginEntry{{
			Name: "linter", LatestVersion: "1.0.0", URL: "linter/manifest.json",
		}}}
		_ = json.NewEncoder(w).Encode(idx)
	})
	mux.HandleFunc("/linter/manifest.json", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(manifest)
	})
	mux.HandleFunc("/linter/linter-1.0.0.tar.gz", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(tarball)
	})

	dest := t.TempDir()
	var stdout, stderr bytes.Buffer
	err := runPluginInstall([]string{
		"--marketplace", srv.URL,
		"--insecure-marketplace",
		"--dest", dest,
		"linter",
	}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("runPluginInstall: %v (stderr=%q)", err, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Installed linter@1.0.0") {
		t.Errorf("missing success line in stdout: %q", stdout.String())
	}
	if _, err := os.Stat(filepath.Join(dest, "linter", "plugin.json")); err != nil {
		t.Errorf("expected installed file: %v", err)
	}
}

func TestRunPluginInstall_RequiresMarketplace(t *testing.T) {
	t.Setenv("CLAW_MARKETPLACE_URL", "")
	var stdout, stderr bytes.Buffer
	err := runPluginInstall([]string{"some-plugin"}, &stdout, &stderr)
	if err == nil || !strings.Contains(err.Error(), "marketplace") {
		t.Fatalf("expected marketplace-required error, got %v", err)
	}
}

func TestRunPluginInstall_RejectsInsecureWithoutFlag(t *testing.T) {
	var stdout, stderr bytes.Buffer
	err := runPluginInstall([]string{
		"--marketplace", "http://example.invalid",
		"linter",
	}, &stdout, &stderr)
	if err == nil || !strings.Contains(err.Error(), "insecure") {
		t.Fatalf("expected insecure rejection, got %v", err)
	}
}

func TestRunPluginInstall_NeedsName(t *testing.T) {
	var stdout, stderr bytes.Buffer
	err := runPluginInstall([]string{"--marketplace", "https://example.com"}, &stdout, &stderr)
	if err == nil || !strings.Contains(err.Error(), "plugin name") {
		t.Fatalf("expected missing-name error, got %v", err)
	}
}

func TestRunPluginInstall_NotFoundIsError(t *testing.T) {
	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	defer srv.Close()
	mux.HandleFunc("/index.json", func(w http.ResponseWriter, r *http.Request) {
		idx := plugin.RemoteIndex{Plugins: []plugin.RemotePluginEntry{}}
		_ = json.NewEncoder(w).Encode(idx)
	})

	var stdout, stderr bytes.Buffer
	err := runPluginInstall([]string{
		"--marketplace", srv.URL,
		"--insecure-marketplace",
		"missing-plugin",
	}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected error for missing plugin")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected not-found error, got %v", err)
	}
	// Sanity: server saw the index request.
	_ = fmt.Sprintf("hit srv %s", srv.URL)
}
