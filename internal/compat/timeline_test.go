package compat

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/SocialGouv/claw-code-go/internal/api"
	"github.com/SocialGouv/claw-code-go/internal/runtime"
)

func writeFixtureSession(t *testing.T, dir string) *runtime.Session {
	t.Helper()
	created := time.Date(2026, 4, 29, 12, 0, 0, 0, time.UTC)
	updated := created.Add(2 * time.Minute)
	s := &runtime.Session{
		ID:        "test-session",
		CreatedAt: created,
		UpdatedAt: updated,
		PromptHistory: []runtime.PromptHistoryEntry{
			{TimestampMs: created.Add(10 * time.Second).UnixMilli(), Text: "first prompt"},
			{TimestampMs: created.Add(60 * time.Second).UnixMilli(), Text: "second prompt"},
		},
		Messages: []api.Message{
			{Role: "user", Content: []api.ContentBlock{{Type: "text", Text: "first prompt"}}},
			{Role: "assistant", Content: []api.ContentBlock{
				{Type: "text", Text: "thinking"},
				{Type: "tool_use", Name: "bash", Input: map[string]any{"cmd": "ls"}},
			}},
			{Role: "user", Content: []api.ContentBlock{
				{Type: "tool_result", Content: []api.ContentBlock{{Type: "text", Text: "ok"}}},
			}},
			{Role: "assistant", Content: []api.ContentBlock{{Type: "text", Text: "done"}}},
		},
	}
	if err := runtime.SaveSessionJSONL(dir, s); err != nil {
		t.Fatalf("save session: %v", err)
	}
	return s
}

func runCmd(t *testing.T, args ...string) (string, string, error) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	err := runTimeline(args, &stdout, &stderr)
	return stdout.String(), stderr.String(), err
}

func TestTimelineCmdReadsSession(t *testing.T) {
	dir := t.TempDir()
	writeFixtureSession(t, dir)

	stdout, _, err := runCmd(t, "--session", "test-session", "--store", dir, "--format", "json")
	if err != nil {
		t.Fatalf("runTimeline: %v", err)
	}

	lines := splitLines(stdout)
	if len(lines) == 0 {
		t.Fatalf("empty output")
	}

	// First line must be the session_meta record.
	var first map[string]any
	if err := json.Unmarshal([]byte(lines[0]), &first); err != nil {
		t.Fatalf("first line not JSON: %v\nline=%q", err, lines[0])
	}
	if first["type"] != "session_meta" {
		t.Errorf("first line type = %v, want session_meta", first["type"])
	}
	if first["session_id"] != "test-session" {
		t.Errorf("session_id = %v, want test-session", first["session_id"])
	}

	// Count types: 1 meta + 2 prompt_history + 4 messages == 7 lines.
	counts := map[string]int{}
	for _, l := range lines {
		var rec struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal([]byte(l), &rec); err != nil {
			t.Fatalf("malformed JSONL line %q: %v", l, err)
		}
		counts[rec.Type]++
	}
	if counts["session_meta"] != 1 {
		t.Errorf("session_meta count = %d, want 1", counts["session_meta"])
	}
	if counts["prompt_history"] != 2 {
		t.Errorf("prompt_history count = %d, want 2", counts["prompt_history"])
	}
	if counts["message"] != 4 {
		t.Errorf("message count = %d, want 4", counts["message"])
	}
}

func TestTimelineCmdMarkdown(t *testing.T) {
	dir := t.TempDir()
	writeFixtureSession(t, dir)

	stdout, _, err := runCmd(t, "--session", "test-session", "--store", dir, "--format", "md")
	if err != nil {
		t.Fatalf("runTimeline: %v", err)
	}

	if !strings.HasPrefix(stdout, "# Session test-session") {
		t.Errorf("output missing top-level header; got prefix %q", firstN(stdout, 60))
	}
	if !strings.Contains(stdout, "first prompt") {
		t.Errorf("markdown missing first prompt body")
	}
	if !strings.Contains(stdout, "## ") {
		t.Errorf("markdown has no event sections (## headers)")
	}
	if !strings.Contains(stdout, "tool_call: bash") {
		t.Errorf("tool_call event not present in markdown output:\n%s", stdout)
	}
}

func TestTimelineCmdHonorsLimit(t *testing.T) {
	dir := t.TempDir()
	writeFixtureSession(t, dir)

	// JSON limit 2 → 1 meta + 2 prompt_history + 2 messages == 5 lines.
	stdout, _, err := runCmd(t, "--session", "test-session", "--store", dir, "--format", "json", "--limit", "2")
	if err != nil {
		t.Fatalf("json+limit: %v", err)
	}
	lines := splitLines(stdout)
	counts := map[string]int{}
	for _, l := range lines {
		var rec struct {
			Type string `json:"type"`
		}
		_ = json.Unmarshal([]byte(l), &rec)
		counts[rec.Type]++
	}
	if counts["message"] != 2 {
		t.Errorf("limited message count = %d, want 2", counts["message"])
	}
	if counts["session_meta"] != 1 {
		t.Errorf("session_meta still required, got %d", counts["session_meta"])
	}

	// MD limit 1 → only one event section header (last event).
	stdout, _, err = runCmd(t, "--session", "test-session", "--store", dir, "--format", "md", "--limit", "1")
	if err != nil {
		t.Fatalf("md+limit: %v", err)
	}
	if got := strings.Count(stdout, "\n## "); got != 1 {
		t.Errorf("md --limit 1: section count = %d, want 1\n%s", got, stdout)
	}
}

func TestTimelineCmdMissingSessionErrors(t *testing.T) {
	dir := t.TempDir()
	writeFixtureSession(t, dir)

	// No --session at all.
	if _, _, err := runCmd(t, "--store", dir); err == nil {
		t.Errorf("expected error when --session omitted")
	}

	// Unknown session id.
	_, _, err := runCmd(t, "--session", "no-such-session", "--store", dir, "--format", "json")
	if err == nil {
		t.Errorf("expected error for unknown session id")
	} else if !strings.Contains(err.Error(), "no-such-session") {
		t.Errorf("error should mention session id; got: %v", err)
	}

	// Unknown format.
	_, _, err = runCmd(t, "--session", "test-session", "--store", dir, "--format", "yaml")
	if err == nil {
		t.Errorf("expected error for unknown --format value")
	}
}

func TestTimelineCmdPrettyHasHeader(t *testing.T) {
	dir := t.TempDir()
	writeFixtureSession(t, dir)

	stdout, _, err := runCmd(t, "--session", "test-session", "--store", dir, "--format", "pretty")
	if err != nil {
		t.Fatalf("pretty: %v", err)
	}
	if !strings.Contains(stdout, "Session test-session") {
		t.Errorf("pretty output missing session header:\n%s", firstN(stdout, 200))
	}
	if !strings.Contains(stdout, "tool_call") {
		t.Errorf("pretty output missing tool_call line:\n%s", stdout)
	}
}

func TestTimelineCmdLegacyJSONStore(t *testing.T) {
	// Confirms LoadSessionAuto fallback: a legacy <id>.json file is rendered
	// just like a JSONL session.
	dir := t.TempDir()
	created := time.Date(2026, 4, 28, 9, 0, 0, 0, time.UTC)
	s := &runtime.Session{
		ID:        "legacy",
		CreatedAt: created,
		UpdatedAt: created.Add(time.Minute),
		Messages: []api.Message{
			{Role: "user", Content: []api.ContentBlock{{Type: "text", Text: "hi"}}},
			{Role: "assistant", Content: []api.ContentBlock{{Type: "text", Text: "hello"}}},
		},
	}
	if err := runtime.SaveSession(dir, s); err != nil {
		t.Fatalf("save legacy session: %v", err)
	}
	// Sanity check: only the legacy file exists.
	if _, err := os.Stat(filepath.Join(dir, "legacy.json")); err != nil {
		t.Fatalf("legacy file missing: %v", err)
	}

	stdout, _, err := runCmd(t, "--session", "legacy", "--store", dir, "--format", "md")
	if err != nil {
		t.Fatalf("runTimeline legacy: %v", err)
	}
	if !strings.Contains(stdout, "# Session legacy") {
		t.Errorf("markdown missing legacy header")
	}
	if !strings.Contains(stdout, "hello") {
		t.Errorf("markdown missing assistant body")
	}
}

func splitLines(s string) []string {
	s = strings.TrimRight(s, "\n")
	if s == "" {
		return nil
	}
	return strings.Split(s, "\n")
}

func firstN(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
