package runtime

import (
	"claw-code-go/internal/api"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRenderJSONLSnapshotRoundtrip(t *testing.T) {
	s := NewSession()
	s.Messages = []api.Message{
		{Role: "user", Content: []api.ContentBlock{{Type: "text", Text: "Hello"}}},
		{Role: "assistant", Content: []api.ContentBlock{{Type: "text", Text: "Hi there!"}}},
	}
	s.CompactionSummary = "previous context summary"
	s.CompactionCount = 2

	snapshot, err := RenderJSONLSnapshot(s)
	if err != nil {
		t.Fatal(err)
	}

	// Each line should be valid JSON.
	lines := strings.Split(strings.TrimSpace(snapshot), "\n")
	if len(lines) < 3 { // meta + compaction + 2 messages = 4 lines minimum
		t.Fatalf("expected at least 4 lines, got %d", len(lines))
	}
	for i, line := range lines {
		var raw json.RawMessage
		if err := json.Unmarshal([]byte(line), &raw); err != nil {
			t.Errorf("line %d is not valid JSON: %v", i, err)
		}
	}

	// Parse back.
	parsed, err := ParseJSONL([]byte(snapshot))
	if err != nil {
		t.Fatal(err)
	}

	if parsed.ID != s.ID {
		t.Errorf("ID: got %q, want %q", parsed.ID, s.ID)
	}
	if len(parsed.Messages) != 2 {
		t.Fatalf("Messages: got %d, want 2", len(parsed.Messages))
	}
	if parsed.Messages[0].Content[0].Text != "Hello" {
		t.Errorf("Message[0] text: got %q", parsed.Messages[0].Content[0].Text)
	}
	if parsed.CompactionSummary != "previous context summary" {
		t.Errorf("CompactionSummary: got %q", parsed.CompactionSummary)
	}
	if parsed.CompactionCount != 2 {
		t.Errorf("CompactionCount: got %d", parsed.CompactionCount)
	}
}

func TestParseJSONLLegacyJSON(t *testing.T) {
	// Legacy JSON format (single JSON object, not JSONL).
	s := &Session{
		ID:        "session-legacy",
		Messages:  []api.Message{{Role: "user", Content: []api.ContentBlock{{Type: "text", Text: "old format"}}}},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	data, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}

	parsed, err := ParseJSONL(data)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.ID != "session-legacy" {
		t.Errorf("ID: got %q", parsed.ID)
	}
	if len(parsed.Messages) != 1 {
		t.Fatalf("Messages: got %d", len(parsed.Messages))
	}
}

func TestParseJSONLCorruptLineRecovery(t *testing.T) {
	// JSONL with one corrupt line — should skip it gracefully.
	lines := []string{
		`{"type":"session_meta","version":1,"session_id":"s1","created_at_ms":1000,"updated_at_ms":2000}`,
		`this is not json`,
		`{"type":"message","message":{"role":"user","content":[{"type":"text","text":"hello"}]}}`,
	}
	data := []byte(strings.Join(lines, "\n"))

	parsed, err := ParseJSONL(data)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.ID != "s1" {
		t.Errorf("ID: got %q", parsed.ID)
	}
	if len(parsed.Messages) != 1 {
		t.Fatalf("Messages: got %d, want 1 (corrupt line skipped)", len(parsed.Messages))
	}
}

func TestParseJSONLEmpty(t *testing.T) {
	parsed, err := ParseJSONL([]byte(""))
	if err != nil {
		t.Fatal(err)
	}
	if len(parsed.Messages) != 0 {
		t.Errorf("expected no messages for empty input")
	}
}

func TestAppendMessageRecord(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.jsonl")

	msg := api.Message{Role: "user", Content: []api.ContentBlock{{Type: "text", Text: "appended"}}}
	if err := AppendMessageRecord(path, msg); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 1 {
		t.Fatalf("expected 1 line, got %d", len(lines))
	}

	// Append another.
	msg2 := api.Message{Role: "assistant", Content: []api.ContentBlock{{Type: "text", Text: "response"}}}
	if err := AppendMessageRecord(path, msg2); err != nil {
		t.Fatal(err)
	}

	data, _ = os.ReadFile(path)
	lines = strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines after second append, got %d", len(lines))
	}
}

func TestRotateSessionFileIfNeeded(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "session.jsonl")

	// Small file — no rotation.
	if err := os.WriteFile(path, make([]byte, 100*1024), 0o644); err != nil {
		t.Fatal(err)
	}
	rotated, err := RotateSessionFileIfNeeded(path)
	if err != nil {
		t.Fatal(err)
	}
	if rotated {
		t.Error("100KB file should not trigger rotation")
	}

	// Large file — should rotate.
	if err := os.WriteFile(path, make([]byte, 300*1024), 0o644); err != nil {
		t.Fatal(err)
	}
	rotated, err = RotateSessionFileIfNeeded(path)
	if err != nil {
		t.Fatal(err)
	}
	if !rotated {
		t.Error("300KB file should trigger rotation")
	}

	// Original file should be gone.
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("original file should be removed after rotation")
	}
}

func TestRotateSessionFileNonexistent(t *testing.T) {
	rotated, err := RotateSessionFileIfNeeded("/nonexistent/path.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	if rotated {
		t.Error("nonexistent file should not trigger rotation")
	}
}

func TestCleanupRotatedLogs(t *testing.T) {
	dir := t.TempDir()
	basePath := filepath.Join(dir, "session.jsonl")

	// Create 5 rotated files.
	for i := 0; i < 5; i++ {
		name := filepath.Join(dir, "session.rot-"+strings.Repeat("0", i+1)+".jsonl")
		if err := os.WriteFile(name, []byte("data"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	if err := CleanupRotatedLogs(basePath, MaxRotatedFiles); err != nil {
		t.Fatal(err)
	}

	entries, _ := os.ReadDir(dir)
	var rotatedCount int
	for _, e := range entries {
		if strings.Contains(e.Name(), ".rot-") {
			rotatedCount++
		}
	}
	if rotatedCount != MaxRotatedFiles {
		t.Errorf("expected %d rotated files, got %d", MaxRotatedFiles, rotatedCount)
	}
}

func TestSaveAndLoadSessionJSONL(t *testing.T) {
	dir := t.TempDir()
	s := NewSession()
	s.Messages = []api.Message{
		{Role: "user", Content: []api.ContentBlock{{Type: "text", Text: "test"}}},
	}

	if err := SaveSessionJSONL(dir, s); err != nil {
		t.Fatal(err)
	}

	loaded, err := LoadSessionAuto(dir, s.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.ID != s.ID {
		t.Errorf("ID: got %q, want %q", loaded.ID, s.ID)
	}
	if len(loaded.Messages) != 1 {
		t.Fatalf("Messages: got %d, want 1", len(loaded.Messages))
	}
}

func TestLoadSessionAutoFallbackJSON(t *testing.T) {
	dir := t.TempDir()
	s := &Session{
		ID:        "session-old",
		Messages:  []api.Message{{Role: "user", Content: []api.ContentBlock{{Type: "text", Text: "legacy"}}}},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	// Save as legacy JSON.
	data, _ := json.MarshalIndent(s, "", "  ")
	os.WriteFile(filepath.Join(dir, "session-old.json"), data, 0o644)

	loaded, err := LoadSessionAuto(dir, "session-old")
	if err != nil {
		t.Fatal(err)
	}
	if loaded.ID != "session-old" {
		t.Errorf("ID: got %q", loaded.ID)
	}
}
