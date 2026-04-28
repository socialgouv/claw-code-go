package plugins

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// makeTarGz produces a gzipped tar archive containing the given files
// (path → content). Used to feed the installer in tests.
func makeTarGz(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for name, content := range files {
		hdr := &tar.Header{
			Name: name,
			Mode: 0o644,
			Size: int64(len(content)),
		}
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

func TestInstaller_DownloadVerifyExtract(t *testing.T) {
	tarball := makeTarGz(t, map[string]string{
		"manifest.json":    `{"name":"linter"}`,
		"bin/run.sh":       "#!/bin/sh\necho hi\n",
		"docs/readme.md":   "doc",
	})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(tarball)
	}))
	defer srv.Close()

	dir := t.TempDir()
	inst := &Installer{Dir: dir, HTTPClient: srv.Client()}

	entry := PluginEntry{
		Name:       "linter",
		TarballURL: srv.URL + "/linter.tgz",
		SHA256:     sha256Hex(tarball),
	}
	if err := inst.Install(context.Background(), entry); err != nil {
		t.Fatalf("Install: %v", err)
	}

	for _, want := range []string{"manifest.json", "bin/run.sh", "docs/readme.md"} {
		path := filepath.Join(dir, "linter", want)
		if _, err := os.Stat(path); err != nil {
			t.Errorf("expected %s to exist after install: %v", path, err)
		}
	}
}

func TestInstaller_RejectsBadChecksum(t *testing.T) {
	tarball := makeTarGz(t, map[string]string{"file": "x"})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(tarball)
	}))
	defer srv.Close()

	dir := t.TempDir()
	inst := &Installer{Dir: dir, HTTPClient: srv.Client()}
	entry := PluginEntry{
		Name:       "linter",
		TarballURL: srv.URL + "/x.tgz",
		SHA256:     "deadbeef",
	}
	err := inst.Install(context.Background(), entry)
	if err == nil || !strings.Contains(err.Error(), "checksum") {
		t.Fatalf("expected checksum mismatch error, got %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "linter")); !os.IsNotExist(err) {
		t.Errorf("expected target dir to be absent, got err=%v", err)
	}
}

func TestInstaller_RejectsPathTraversal(t *testing.T) {
	// Build a tarball with a malicious entry name.
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	hdr := &tar.Header{Name: "../escape.txt", Mode: 0o644, Size: 4}
	_ = tw.WriteHeader(hdr)
	_, _ = tw.Write([]byte("evil"))
	_ = tw.Close()
	_ = gz.Close()
	tarball := buf.Bytes()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(tarball)
	}))
	defer srv.Close()

	dir := t.TempDir()
	inst := &Installer{Dir: dir, HTTPClient: srv.Client()}
	entry := PluginEntry{
		Name:       "evil",
		TarballURL: srv.URL,
		SHA256:     sha256Hex(tarball),
	}
	err := inst.Install(context.Background(), entry)
	if err == nil || !strings.Contains(err.Error(), "escape") {
		t.Fatalf("expected path-escape error, got %v", err)
	}
}

func TestInstaller_Uninstall(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "linter")
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(target, "x"), []byte("y"), 0o644); err != nil {
		t.Fatal(err)
	}
	inst := &Installer{Dir: dir}
	if err := inst.Uninstall(context.Background(), "linter"); err != nil {
		t.Fatalf("Uninstall: %v", err)
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Errorf("expected target dir gone, got err=%v", err)
	}
	// Idempotent.
	if err := inst.Uninstall(context.Background(), "linter"); err != nil {
		t.Errorf("expected idempotent uninstall, got %v", err)
	}
}

func TestInstaller_RejectsHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()
	inst := &Installer{Dir: t.TempDir(), HTTPClient: srv.Client()}
	err := inst.Install(context.Background(), PluginEntry{
		Name:       "x",
		TarballURL: srv.URL,
		SHA256:     "deadbeef",
	})
	if err == nil || !strings.Contains(err.Error(), "503") {
		t.Fatalf("expected 503 error, got %v", err)
	}
}

func TestInstaller_RejectsEmptyEntry(t *testing.T) {
	inst := &Installer{Dir: t.TempDir()}
	if err := inst.Install(context.Background(), PluginEntry{}); err == nil {
		t.Error("expected error on empty entry")
	}
}
