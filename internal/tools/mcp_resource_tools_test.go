package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/SocialGouv/claw-code-go/internal/mcp"
	"strings"
	"sync"
	"testing"
)

// mockTransport is a test-only MCP Transport that returns canned responses.
type mockTransport struct {
	mu        sync.Mutex
	responses map[string]any
}

func newMockTransport() *mockTransport {
	return &mockTransport{
		responses: map[string]any{
			"initialize": mcp.InitializeResult{
				ProtocolVersion: "2024-11-05",
				Capabilities: mcp.ServerCapabilities{
					Tools:     &mcp.ToolsCapability{},
					Resources: &mcp.ResourcesCapability{},
				},
				ServerInfo: mcp.ServerInfo{Name: "mock-server", Version: "1.0.0"},
			},
			"tools/list": map[string]any{
				"tools": []mcp.MCPTool{},
			},
			"resources/list": map[string]any{
				"resources": []mcp.McpResourceInfo{
					{URI: "test://doc", Name: "doc", Description: "A test doc", MimeType: "text/plain"},
					{URI: "test://config", Name: "config", Description: "Config file", MimeType: "application/json"},
				},
			},
			"resources/read": map[string]any{
				"contents": []map[string]any{
					{
						"uri":         "test://doc",
						"name":        "doc",
						"description": "A test doc",
						"mimeType":    "text/plain",
						"text":        "Hello, world!",
					},
				},
			},
		},
	}
}

func (m *mockTransport) Send(_ context.Context, req mcp.Request) (mcp.Response, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	result, ok := m.responses[req.Method]
	if !ok {
		return mcp.Response{
			JSONRPC: "2.0", ID: req.ID,
			Error: &mcp.RPCError{Code: -32601, Message: fmt.Sprintf("method %q not found", req.Method)},
		}, nil
	}
	// Round-trip through JSON to match real transport behavior.
	data, err := json.Marshal(result)
	if err != nil {
		return mcp.Response{}, fmt.Errorf("mock: marshal: %w", err)
	}
	var unmarshalled any
	if err := json.Unmarshal(data, &unmarshalled); err != nil {
		return mcp.Response{}, fmt.Errorf("mock: unmarshal: %w", err)
	}
	return mcp.Response{JSONRPC: "2.0", ID: req.ID, Result: unmarshalled}, nil
}

func (m *mockTransport) Notify(_ mcp.Notification) error { return nil }
func (m *mockTransport) Close() error                    { return nil }

// setupMockRegistry creates a Registry with a mock server registered under the given name.
func setupMockRegistry(t *testing.T, serverName string) *mcp.Registry {
	t.Helper()
	reg := mcp.NewRegistry()
	transport := newMockTransport()
	if err := reg.AddServer(context.Background(), serverName, transport); err != nil {
		t.Fatalf("failed to add mock server: %v", err)
	}
	return reg
}

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

func TestReadMcpResource_Success(t *testing.T) {
	reg := setupMockRegistry(t, "test-server")

	result, err := ExecuteReadMcpResource(map[string]any{
		"uri":    "test://doc",
		"server": "test-server",
	}, reg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal([]byte(result), &parsed); err != nil {
		t.Fatalf("failed to parse result: %v", err)
	}

	// Assert exactly 5 keys matching Rust response shape.
	expectedKeys := []string{"server", "uri", "name", "description", "mime_type"}
	if len(parsed) != len(expectedKeys) {
		t.Errorf("expected %d keys, got %d: %v", len(expectedKeys), len(parsed), parsed)
	}
	for _, key := range expectedKeys {
		if _, ok := parsed[key]; !ok {
			t.Errorf("missing key %q in response", key)
		}
	}

	// Assert 'content' key is absent (Rust parity).
	if _, ok := parsed["content"]; ok {
		t.Error("response should not contain 'content' key (Rust parity)")
	}

	// Assert values.
	if parsed["server"] != "test-server" {
		t.Errorf("expected server 'test-server', got %v", parsed["server"])
	}
	if parsed["uri"] != "test://doc" {
		t.Errorf("expected uri 'test://doc', got %v", parsed["uri"])
	}
	if parsed["name"] != "doc" {
		t.Errorf("expected name 'doc', got %v", parsed["name"])
	}
}

func TestListMcpResources_Success(t *testing.T) {
	reg := setupMockRegistry(t, "test-server")

	result, err := ExecuteListMcpResources(map[string]any{
		"server": "test-server",
	}, reg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal([]byte(result), &parsed); err != nil {
		t.Fatalf("failed to parse result: %v", err)
	}

	// Assert response has server, resources, count.
	if parsed["server"] != "test-server" {
		t.Errorf("expected server 'test-server', got %v", parsed["server"])
	}

	resources, ok := parsed["resources"].([]any)
	if !ok {
		t.Fatalf("expected resources to be an array, got %T", parsed["resources"])
	}

	count, ok := parsed["count"].(float64)
	if !ok {
		t.Fatalf("expected count to be a number, got %T", parsed["count"])
	}

	// count must match array length.
	if int(count) != len(resources) {
		t.Errorf("expected count=%d to match resources length=%d", int(count), len(resources))
	}

	// Our mock has 2 resources.
	if len(resources) != 2 {
		t.Errorf("expected 2 resources, got %d", len(resources))
	}

	// No error key on success.
	if _, ok := parsed["error"]; ok {
		t.Error("response should not contain 'error' key on success")
	}
}
