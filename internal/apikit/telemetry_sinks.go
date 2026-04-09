package apikit

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

// MemoryTelemetrySink collects events in memory for testing.
type MemoryTelemetrySink struct {
	mu     sync.Mutex
	events []TelemetryEvent
}

// Record appends an event to the in-memory list.
func (s *MemoryTelemetrySink) Record(event TelemetryEvent) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = append(s.events, event)
}

// Events returns a copy of all recorded events.
func (s *MemoryTelemetrySink) Events() []TelemetryEvent {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := make([]TelemetryEvent, len(s.events))
	copy(result, s.events)
	return result
}

// JsonlTelemetrySink writes events as newline-delimited JSON to a file.
type JsonlTelemetrySink struct {
	path string
	mu   sync.Mutex
	file *os.File
}

// NewJsonlTelemetrySink creates a JSONL sink, creating parent directories as needed.
func NewJsonlTelemetrySink(path string) (*JsonlTelemetrySink, error) {
	dir := filepath.Dir(path)
	if dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, err
		}
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, err
	}
	return &JsonlTelemetrySink{path: path, file: f}, nil
}

// Path returns the file path.
func (s *JsonlTelemetrySink) Path() string {
	return s.path
}

// Record writes one JSON line and flushes.
func (s *JsonlTelemetrySink) Record(event TelemetryEvent) {
	line, err := json.Marshal(event)
	if err != nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	_, _ = s.file.Write(append(line, '\n'))
	_ = s.file.Sync()
}

// Close closes the underlying file.
func (s *JsonlTelemetrySink) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.file.Close()
}
