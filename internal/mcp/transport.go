package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
)

// TransportType identifies the kind of MCP transport.
type TransportType string

const (
	TransportStdio        TransportType = "stdio"
	TransportSSE          TransportType = "sse"
	TransportHTTP         TransportType = "http"
	TransportWebSocket    TransportType = "websocket"
	TransportManagedProxy TransportType = "managed_proxy"
	TransportSDK          TransportType = "sdk"
)

// TransportConfig holds common configuration for creating transports.
type TransportConfig struct {
	Type    TransportType
	URL     string            // For SSE, HTTP, WebSocket, ManagedProxy
	Headers map[string]string // For SSE, HTTP, WebSocket
	Command string            // For Stdio
	Args    []string          // For Stdio
	Env     map[string]string // For Stdio
	Auth    string            // Auth header for SSE/HTTP
	ID      string            // For ManagedProxy
	Name    string            // For SDK
}

// NewTransport creates a Transport from the given configuration.
// Returns an error for unsupported or misconfigured transport types.
func NewTransport(cfg TransportConfig) (Transport, error) {
	switch cfg.Type {
	case TransportStdio:
		env := make([]string, 0, len(cfg.Env))
		for k, v := range cfg.Env {
			env = append(env, k+"="+v)
		}
		return NewStdioTransport(cfg.Command, cfg.Args, env)
	case TransportSSE, TransportHTTP:
		return NewSSETransport(cfg.URL, cfg.Auth), nil
	case TransportWebSocket:
		return NewWebSocketTransport(websocketURL(cfg.URL), cfg.Headers)
	case TransportManagedProxy:
		return NewManagedProxyTransport(cfg.URL, cfg.ID)
	case TransportSDK:
		return NewSDKTransport(cfg.Name, cfg.Command, cfg.Args, cfg.Env)
	default:
		return nil, fmt.Errorf("mcp: unsupported transport type %q", cfg.Type)
	}
}

// SDKTransport wraps an MCP SDK server subprocess, communicating via
// Content-Length framed JSON-RPC over stdin/stdout.
type SDKTransport struct {
	name   string
	cmd    *exec.Cmd
	server *McpSdkServer
	cancel context.CancelFunc
}

// NewSDKTransport creates a transport that spawns a subprocess with an MCP SDK
// server. The command/args specify the subprocess to run; the server reads from
// the subprocess stdout and writes to its stdin.
func NewSDKTransport(name, command string, args []string, env map[string]string) (*SDKTransport, error) {
	ctx, cancel := context.WithCancel(context.Background())

	cmd := exec.CommandContext(ctx, command, args...)
	for k, v := range env {
		cmd.Env = append(cmd.Env, k+"="+v)
	}

	return &SDKTransport{
		name:   name,
		cmd:    cmd,
		cancel: cancel,
	}, nil
}

func (t *SDKTransport) Send(req Request) (Response, error) {
	// The SDK transport is currently a placeholder for subprocess-based
	// MCP SDK servers. Real implementation would pipe JSON-RPC frames
	// to/from the subprocess. For now, return an unsupported error.
	_ = json.RawMessage{}
	return Response{
		JSONRPC: "2.0",
		ID:      req.ID,
		Error: &RPCError{
			Code:    -32601,
			Message: fmt.Sprintf("SDK transport %q: subprocess communication not yet wired", t.name),
		},
	}, nil
}

func (t *SDKTransport) Notify(n Notification) error {
	return nil
}

func (t *SDKTransport) Close() error {
	if t.cancel != nil {
		t.cancel()
	}
	return nil
}
