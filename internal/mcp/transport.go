package mcp

import (
	"fmt"
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
	default:
		return nil, fmt.Errorf("mcp: unsupported transport type %q", cfg.Type)
	}
}
