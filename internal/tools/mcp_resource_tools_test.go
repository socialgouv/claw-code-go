package tools

import (
	"claw-code-go/internal/mcp"
	"encoding/json"
	"strings"
	"testing"
)

func TestListMcpResources_NilRegistry(t *testing.T) {
	_, err := ExecuteListMcpResources(map[string]any{}, nil)
	if err == nil {
		t.Fatal("expected error for nil registry")
	}
	if !strings.Contains(err.Error(), "not available") {
		t.Errorf("expected 'not available' error, got: %v", err)
	}
}

func TestListMcpResources_ServerNotFound(t *testing.T) {
	reg := mcp.NewRegistry()
	result, err := ExecuteListMcpResources(map[string]any{
		"server": "nonexistent",
	}, reg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(result), &parsed); err != nil {
		t.Fatalf("failed to parse result: %v", err)
	}
	if parsed["error"] == nil {
		t.Error("expected error field in response")
	}
	if parsed["server"] != "nonexistent" {
		t.Errorf("expected server 'nonexistent', got %v", parsed["server"])
	}
}

func TestReadMcpResource_NilRegistry(t *testing.T) {
	_, err := ExecuteReadMcpResource(map[string]any{"uri": "test://resource"}, nil)
	if err == nil {
		t.Fatal("expected error for nil registry")
	}
}

func TestReadMcpResource_MissingUri(t *testing.T) {
	reg := mcp.NewRegistry()
	_, err := ExecuteReadMcpResource(map[string]any{}, reg)
	if err == nil {
		t.Fatal("expected error for missing uri")
	}
	if !strings.Contains(err.Error(), "uri") {
		t.Errorf("expected error about 'uri', got: %v", err)
	}
}

func TestReadMcpResource_ServerNotFound(t *testing.T) {
	reg := mcp.NewRegistry()
	result, err := ExecuteReadMcpResource(map[string]any{
		"uri":    "test://resource",
		"server": "nonexistent",
	}, reg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(result), &parsed); err != nil {
		t.Fatalf("failed to parse result: %v", err)
	}
	if parsed["error"] == nil {
		t.Error("expected error field in response")
	}
}

func TestMcpAuth_NilRegistry(t *testing.T) {
	_, err := ExecuteMcpAuth(map[string]any{"server": "test"}, nil, nil)
	if err == nil {
		t.Fatal("expected error for nil registry")
	}
}

func TestMcpAuth_MissingServer(t *testing.T) {
	reg := mcp.NewRegistry()
	_, err := ExecuteMcpAuth(map[string]any{}, reg, nil)
	if err == nil {
		t.Fatal("expected error for missing server")
	}
	if !strings.Contains(err.Error(), "server") {
		t.Errorf("expected error about 'server', got: %v", err)
	}
}

func TestMcpAuth_ServerNotRegistered(t *testing.T) {
	reg := mcp.NewRegistry()
	authState := mcp.NewAuthState()

	result, err := ExecuteMcpAuth(map[string]any{"server": "unknown"}, reg, authState)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(result), &parsed); err != nil {
		t.Fatalf("failed to parse result: %v", err)
	}
	if parsed["status"] != "disconnected" {
		t.Errorf("expected status 'disconnected', got %v", parsed["status"])
	}
	if parsed["message"] == nil {
		t.Error("expected message for unregistered server")
	}
}
