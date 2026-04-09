package apikit

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestMemorySinkRecordsAndReturnsEvents(t *testing.T) {
	sink := &MemoryTelemetrySink{}
	sink.Record(TelemetryEvent{Type: EventTypeAnalytics, Analytics: &AnalyticsEvent{Namespace: "cli", Action: "start"}})
	sink.Record(TelemetryEvent{Type: EventTypeHTTPRequestStarted, SessionID: "s1", Attempt: 1, Method: "POST", Path: "/v1/messages"})

	events := sink.Events()
	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}
	if events[0].Type != EventTypeAnalytics {
		t.Errorf("expected analytics event, got %s", events[0].Type)
	}
	if events[1].Type != EventTypeHTTPRequestStarted {
		t.Errorf("expected http_request_started, got %s", events[1].Type)
	}
}

func TestMemorySinkReturnsCopy(t *testing.T) {
	sink := &MemoryTelemetrySink{}
	sink.Record(TelemetryEvent{Type: EventTypeAnalytics})
	events := sink.Events()
	events[0].Type = "mutated"
	original := sink.Events()
	if original[0].Type != EventTypeAnalytics {
		t.Error("Events() should return a copy, not a reference")
	}
}

func TestJsonlSinkWritesValidJSONL(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "telemetry.jsonl")

	sink, err := NewJsonlTelemetrySink(path)
	if err != nil {
		t.Fatalf("failed to create sink: %v", err)
	}
	defer sink.Close()

	sink.Record(TelemetryEvent{
		Type:      EventTypeAnalytics,
		Analytics: &AnalyticsEvent{Namespace: "cli", Action: "turn_completed"},
	})

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read file: %v", err)
	}

	content := string(data)
	if len(content) == 0 {
		t.Fatal("empty file")
	}

	var parsed TelemetryEvent
	if err := json.Unmarshal([]byte(content), &parsed); err != nil {
		t.Fatalf("invalid JSON line: %v", err)
	}
	if parsed.Type != EventTypeAnalytics {
		t.Errorf("expected analytics type, got %s", parsed.Type)
	}
}

func TestJsonlSinkCreatesParentDirs(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nested", "deep", "telemetry.jsonl")

	sink, err := NewJsonlTelemetrySink(path)
	if err != nil {
		t.Fatalf("failed to create sink with nested dirs: %v", err)
	}
	defer sink.Close()

	if _, err := os.Stat(filepath.Dir(path)); os.IsNotExist(err) {
		t.Error("parent directories should have been created")
	}
}

func TestSessionTracerAutoIncrementsSequence(t *testing.T) {
	sink := &MemoryTelemetrySink{}
	tracer := NewSessionTracer("session-42", sink)

	tracer.Record("first", nil)
	tracer.Record("second", nil)

	events := sink.Events()
	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}
	if events[0].SessionTrace.Sequence != 0 {
		t.Errorf("first sequence should be 0, got %d", events[0].SessionTrace.Sequence)
	}
	if events[1].SessionTrace.Sequence != 1 {
		t.Errorf("second sequence should be 1, got %d", events[1].SessionTrace.Sequence)
	}
}

func TestSessionTracerHTTPLifecycle(t *testing.T) {
	sink := &MemoryTelemetrySink{}
	tracer := NewSessionTracer("session-123", sink)

	tracer.RecordHTTPRequestStarted(1, "POST", "/v1/messages", nil)
	tracer.RecordHTTPRequestSucceeded(1, "POST", "/v1/messages", 200, "req-abc", nil)
	tracer.RecordHTTPRequestFailed(2, "POST", "/v1/messages", "timeout", true, nil)

	events := sink.Events()
	// Each HTTP method emits 2 events: the primary + a trace record
	if len(events) != 6 {
		t.Fatalf("expected 6 events (3 primary + 3 traces), got %d", len(events))
	}

	// First: HTTPRequestStarted
	if events[0].Type != EventTypeHTTPRequestStarted {
		t.Errorf("expected http_request_started, got %s", events[0].Type)
	}
	if events[0].SessionID != "session-123" {
		t.Errorf("wrong session ID: %s", events[0].SessionID)
	}
	if events[0].Method != "POST" || events[0].Path != "/v1/messages" {
		t.Error("method/path mismatch")
	}

	// Second: trace for started
	if events[1].Type != EventTypeSessionTrace {
		t.Errorf("expected session_trace, got %s", events[1].Type)
	}
	if events[1].SessionTrace.Name != "http_request_started" {
		t.Errorf("expected trace name 'http_request_started', got '%s'", events[1].SessionTrace.Name)
	}

	// Third: HTTPRequestSucceeded
	if events[2].Type != EventTypeHTTPRequestSucceeded {
		t.Errorf("expected http_request_succeeded, got %s", events[2].Type)
	}
	if events[2].Status != 200 {
		t.Errorf("expected status 200, got %d", events[2].Status)
	}

	// Fifth: HTTPRequestFailed
	if events[4].Type != EventTypeHTTPRequestFailed {
		t.Errorf("expected http_request_failed, got %s", events[4].Type)
	}
	if events[4].Error != "timeout" {
		t.Errorf("expected error 'timeout', got '%s'", events[4].Error)
	}
	if !events[4].Retryable {
		t.Error("expected retryable to be true")
	}
}

func TestSessionTracerRecordAnalytics(t *testing.T) {
	sink := &MemoryTelemetrySink{}
	tracer := NewSessionTracer("session-123", sink)

	evt := NewAnalyticsEvent("cli", "prompt_sent").WithProperty("model", "claude-opus")
	tracer.RecordAnalytics(evt)

	events := sink.Events()
	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}
	if events[0].Type != EventTypeAnalytics {
		t.Errorf("expected analytics, got %s", events[0].Type)
	}
	if events[0].Analytics.Namespace != "cli" || events[0].Analytics.Action != "prompt_sent" {
		t.Error("analytics event fields mismatch")
	}
	if events[1].Type != EventTypeSessionTrace {
		t.Errorf("expected session_trace, got %s", events[1].Type)
	}
}

func TestConcurrentRecordIsSafe(t *testing.T) {
	sink := &MemoryTelemetrySink{}
	tracer := NewSessionTracer("concurrent", sink)

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			tracer.Record("event", map[string]any{"n": n})
		}(i)
	}
	wg.Wait()

	events := sink.Events()
	if len(events) != 100 {
		t.Errorf("expected 100 events from concurrent writes, got %d", len(events))
	}

	// Verify all sequence numbers are unique
	seqs := make(map[uint64]bool)
	for _, e := range events {
		if e.SessionTrace == nil {
			t.Fatal("expected session trace event")
		}
		if seqs[e.SessionTrace.Sequence] {
			t.Errorf("duplicate sequence number: %d", e.SessionTrace.Sequence)
		}
		seqs[e.SessionTrace.Sequence] = true
	}
}

func TestAnthropicRequestProfileHeaders(t *testing.T) {
	profile := NewAnthropicRequestProfile(
		NewClientIdentity("claude-code", "1.2.3").WithRuntime("go-cli"),
	).WithBeta("tools-2026-04-01").
		WithExtraBody("metadata", map[string]any{"source": "test"})

	headers := profile.HeaderPairs()
	if len(headers) != 3 {
		t.Fatalf("expected 3 header pairs, got %d", len(headers))
	}
	if headers[0][0] != "anthropic-version" || headers[0][1] != DefaultAnthropicVersion {
		t.Errorf("wrong anthropic-version header: %v", headers[0])
	}
	if headers[1][0] != "user-agent" || headers[1][1] != "claude-code/1.2.3" {
		t.Errorf("wrong user-agent header: %v", headers[1])
	}
	expected := "claude-code-20250219,prompt-caching-scope-2026-01-05,tools-2026-04-01"
	if headers[2][0] != "anthropic-beta" || headers[2][1] != expected {
		t.Errorf("wrong anthropic-beta header: %v", headers[2])
	}
}

func TestAnthropicRequestProfileRenderJSONBody(t *testing.T) {
	profile := NewAnthropicRequestProfile(NewClientIdentity("claude-code", "1.2.3")).
		WithExtraBody("metadata", map[string]any{"source": "test"})

	body, err := profile.RenderJSONBody(map[string]any{"model": "claude-sonnet"})
	if err != nil {
		t.Fatal(err)
	}
	metadata, ok := body["metadata"].(map[string]any)
	if !ok {
		t.Fatal("metadata field not found or wrong type")
	}
	if metadata["source"] != "test" {
		t.Errorf("expected source=test, got %v", metadata["source"])
	}
	if body["betas"] == nil {
		t.Error("betas should be in body")
	}
}

func TestWithBetaDeduplication(t *testing.T) {
	profile := NewAnthropicRequestProfile(NewClientIdentity("test", "1.0")).
		WithBeta(DefaultAgenticBeta) // Already present by default
	if len(profile.Betas) != 2 {
		t.Errorf("expected 2 betas (no duplicate), got %d", len(profile.Betas))
	}
}

func TestClientIdentityUserAgent(t *testing.T) {
	ci := NewClientIdentity("my-app", "2.0.0")
	if ci.UserAgent() != "my-app/2.0.0" {
		t.Errorf("unexpected user agent: %s", ci.UserAgent())
	}
}
