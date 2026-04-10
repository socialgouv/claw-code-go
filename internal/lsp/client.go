package lsp

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"sync"
)

// Registry is a thread-safe registry for LSP language servers.
type Registry struct {
	mu      sync.Mutex
	servers map[string]*LspServerState
}

// NewRegistry creates a new empty LSP registry.
func NewRegistry() *Registry {
	return &Registry{
		servers: make(map[string]*LspServerState),
	}
}

// Register registers a language server with the given configuration.
func (r *Registry) Register(language string, status LspServerStatus, rootPath *string, capabilities []string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	caps := make([]string, len(capabilities))
	copy(caps, capabilities)

	r.servers[language] = &LspServerState{
		Language:     language,
		Status:       status,
		RootPath:     rootPath,
		Capabilities: caps,
		Diagnostics:  []LspDiagnostic{},
	}
}

// Get retrieves a server by language name.
func (r *Registry) Get(language string) (LspServerState, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()

	s, ok := r.servers[language]
	if !ok {
		return LspServerState{}, false
	}
	return s.clone(), true
}

// FindServerForPath finds the appropriate server for a file path based on extension.
func (r *Registry) FindServerForPath(path string) (LspServerState, bool) {
	ext := filepath.Ext(path)
	if ext != "" {
		ext = ext[1:] // strip leading dot
	}

	language := extensionToLanguage(ext)
	if language == "" {
		return LspServerState{}, false
	}

	return r.Get(language)
}

// ListServers returns all registered servers.
func (r *Registry) ListServers() []LspServerState {
	r.mu.Lock()
	defer r.mu.Unlock()

	result := make([]LspServerState, 0, len(r.servers))
	for _, s := range r.servers {
		result = append(result, s.clone())
	}
	return result
}

// AddDiagnostics adds diagnostics to a language server.
func (r *Registry) AddDiagnostics(language string, diagnostics []LspDiagnostic) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	s, ok := r.servers[language]
	if !ok {
		return fmt.Errorf("LSP server not found for language: %s", language)
	}
	s.Diagnostics = append(s.Diagnostics, diagnostics...)
	return nil
}

// GetDiagnostics returns diagnostics for a specific file path across all servers.
func (r *Registry) GetDiagnostics(path string) []LspDiagnostic {
	r.mu.Lock()
	defer r.mu.Unlock()

	var result []LspDiagnostic
	for _, s := range r.servers {
		for _, d := range s.Diagnostics {
			if d.Path == path {
				result = append(result, d)
			}
		}
	}
	return result
}

// ClearDiagnostics clears all diagnostics for a language server.
func (r *Registry) ClearDiagnostics(language string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	s, ok := r.servers[language]
	if !ok {
		return fmt.Errorf("LSP server not found for language: %s", language)
	}
	s.Diagnostics = s.Diagnostics[:0]
	return nil
}

// Disconnect removes a server from the registry.
func (r *Registry) Disconnect(language string) *LspServerState {
	r.mu.Lock()
	defer r.mu.Unlock()

	s, ok := r.servers[language]
	if !ok {
		return nil
	}
	c := s.clone()
	delete(r.servers, language)
	return &c
}

// Len returns the number of registered servers.
func (r *Registry) Len() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.servers)
}

// IsEmpty returns true if the registry has no servers.
func (r *Registry) IsEmpty() bool {
	return r.Len() == 0
}

// Dispatch dispatches an LSP action and returns a structured result.
func (r *Registry) Dispatch(action string, path *string, line *uint32, character *uint32, query *string) (json.RawMessage, error) {
	lspAction, ok := ParseAction(action)
	if !ok {
		return nil, fmt.Errorf("unknown LSP action: %s", action)
	}

	// For diagnostics, we can check existing cached diagnostics.
	if lspAction == ActionDiagnostics {
		if path != nil {
			diags := r.GetDiagnostics(*path)
			result, _ := json.Marshal(map[string]any{
				"action":      "diagnostics",
				"path":        *path,
				"diagnostics": diags,
				"count":       len(diags),
			})
			return result, nil
		}

		// All diagnostics across all servers.
		r.mu.Lock()
		var allDiags []LspDiagnostic
		for _, s := range r.servers {
			allDiags = append(allDiags, s.Diagnostics...)
		}
		r.mu.Unlock()

		if allDiags == nil {
			allDiags = []LspDiagnostic{}
		}
		result, _ := json.Marshal(map[string]any{
			"action":      "diagnostics",
			"diagnostics": allDiags,
			"count":       len(allDiags),
		})
		return result, nil
	}

	// For other actions, we need a connected server for the given file.
	if path == nil {
		return nil, fmt.Errorf("path is required for this LSP action")
	}

	server, ok := r.FindServerForPath(*path)
	if !ok {
		return nil, fmt.Errorf("no LSP server available for path: %s", *path)
	}

	if server.Status != StatusConnected {
		return nil, fmt.Errorf("LSP server for '%s' is not connected (status: %s)", server.Language, server.Status)
	}

	// Return structured placeholder — actual LSP JSON-RPC calls would
	// go through the real LSP process here.
	result, _ := json.Marshal(map[string]any{
		"action":    action,
		"path":      *path,
		"line":      line,
		"character": character,
		"language":  server.Language,
		"status":    "dispatched",
		"message":   fmt.Sprintf("LSP %s dispatched to %s server", action, server.Language),
	})
	return result, nil
}

// extensionToLanguage maps file extensions to language identifiers.
func extensionToLanguage(ext string) string {
	switch ext {
	case "rs":
		return "rust"
	case "ts", "tsx":
		return "typescript"
	case "js", "jsx":
		return "javascript"
	case "py":
		return "python"
	case "go":
		return "go"
	case "java":
		return "java"
	case "c", "h":
		return "c"
	case "cpp", "hpp", "cc":
		return "cpp"
	case "rb":
		return "ruby"
	case "lua":
		return "lua"
	default:
		return ""
	}
}
