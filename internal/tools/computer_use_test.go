package tools

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/SocialGouv/claw-code-go/internal/api"
)

// minimal 1x1 transparent PNG used as fixture content.
var pngFixture = []byte{
	0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A,
	0x00, 0x00, 0x00, 0x0D, 0x49, 0x48, 0x44, 0x52,
	0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
	0x08, 0x06, 0x00, 0x00, 0x00, 0x1F, 0x15, 0xC4,
	0x89, 0x00, 0x00, 0x00, 0x0D, 0x49, 0x44, 0x41,
	0x54, 0x78, 0x9C, 0x63, 0x00, 0x01, 0x00, 0x00,
	0x05, 0x00, 0x01, 0x0D, 0x0A, 0x2D, 0xB4, 0x00,
	0x00, 0x00, 0x00, 0x49, 0x45, 0x4E, 0x44, 0xAE,
	0x42, 0x60, 0x82,
}

func TestReadImage_Base64FromFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tiny.png")
	if err := os.WriteFile(path, pngFixture, 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	res, err := ExecuteReadImage(context.Background(), map[string]any{"path": path})
	if err != nil {
		t.Fatalf("ExecuteReadImage: %v", err)
	}
	if len(res.Blocks) != 1 {
		t.Fatalf("expected 1 block, got %d", len(res.Blocks))
	}
	block := res.Blocks[0]
	if block.Type != "image" || block.Source == nil {
		t.Fatalf("expected image block with Source, got %+v", block)
	}
	if block.Source.Type != "base64" {
		t.Errorf("expected Source.Type=base64, got %q", block.Source.Type)
	}
	if block.Source.MediaType != "image/png" {
		t.Errorf("expected MediaType=image/png, got %q", block.Source.MediaType)
	}
	if block.Source.Data == "" {
		t.Errorf("expected non-empty base64 data")
	}
	if res.Description == "" {
		t.Errorf("expected description to default to filename")
	}
}

func TestReadImage_RejectsLargeFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "big.png")
	// 6 MB of zeros — past the 5 MB cap.
	if err := os.WriteFile(path, make([]byte, 6*1024*1024), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	_, err := ExecuteReadImage(context.Background(), map[string]any{"path": path})
	if err == nil {
		t.Fatal("expected error on oversized file")
	}
	if !strings.Contains(err.Error(), "5 MB") {
		t.Errorf("expected size-limit error, got %v", err)
	}
}

func TestReadImage_RejectsHTTPURL(t *testing.T) {
	_, err := ExecuteReadImage(context.Background(), map[string]any{"url": "http://example.com/x.png"})
	if err == nil {
		t.Fatal("expected error on plain http url")
	}
	if !strings.Contains(err.Error(), "https") {
		t.Errorf("expected https error, got %v", err)
	}
}

func TestReadImage_RejectsFileURL(t *testing.T) {
	_, err := ExecuteReadImage(context.Background(), map[string]any{"url": "file:///etc/passwd"})
	if err == nil {
		t.Fatal("expected error on file scheme")
	}
	if !strings.Contains(err.Error(), "https") {
		t.Errorf("expected https error, got %v", err)
	}
}

func TestReadImage_HTTPSFetch(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(pngFixture)
	}))
	t.Cleanup(srv.Close)

	prev := http.DefaultClient
	http.DefaultClient = srv.Client()
	t.Cleanup(func() { http.DefaultClient = prev })

	res, err := ExecuteReadImage(context.Background(), map[string]any{"url": srv.URL + "/img.png"})
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if len(res.Blocks) != 1 || res.Blocks[0].Source == nil {
		t.Fatalf("expected one image block, got %+v", res)
	}
	if res.Blocks[0].Source.MediaType != "image/png" {
		t.Errorf("expected image/png, got %q", res.Blocks[0].Source.MediaType)
	}
}

func TestReadImage_RejectsHTTPSToHTTPRedirect(t *testing.T) {
	// Plain HTTP target — the "evil" host the redirect would land on.
	plain := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("plain http server should never be hit; redirect must be blocked")
		_, _ = w.Write(pngFixture)
	}))
	defer plain.Close()

	// HTTPS origin that 302s to the plain http target. The starting URL
	// passes the scheme check, so without CheckRedirect we'd silently
	// follow into plaintext.
	tls := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, plain.URL+"/img.png", http.StatusFound)
	}))
	defer tls.Close()

	prev := http.DefaultClient
	http.DefaultClient = tls.Client()
	t.Cleanup(func() { http.DefaultClient = prev })

	_, err := ExecuteReadImage(context.Background(), map[string]any{"url": tls.URL + "/start"})
	if err == nil || !strings.Contains(err.Error(), "non-https") {
		t.Fatalf("expected redirect-to-non-https error, got %v", err)
	}
}

func TestReadImage_PathAndURL_Mutex(t *testing.T) {
	_, err := ExecuteReadImage(context.Background(), map[string]any{
		"path": "/tmp/x.png",
		"url":  "https://example.com/y.png",
	})
	if err == nil || !strings.Contains(err.Error(), "mutually exclusive") {
		t.Fatalf("expected mutual-exclusion error, got %v", err)
	}
}

func TestScreenshot_NotImplemented(t *testing.T) {
	_, err := ExecuteScreenshot(context.Background(), map[string]any{})
	if err == nil {
		t.Fatal("expected error from screenshot stub")
	}
	var apiErr *api.APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected *api.APIError, got %T: %v", err, err)
	}
	if apiErr.StatusCode != 501 {
		t.Errorf("expected status 501, got %d", apiErr.StatusCode)
	}
}
