package apikit

import (
	"encoding/json"
	"strings"
	"testing"
	"testing/quick"
)

func TestSSEParserSingleFrame(t *testing.T) {
	parser := NewSSEParser()
	chunk := []byte("event: content_block_start\ndata: {\"type\":\"content_block_start\",\"index\":0}\n\n")

	events, err := parser.Push(chunk)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Event != "content_block_start" {
		t.Errorf("event = %q, want 'content_block_start'", events[0].Event)
	}

	var parsed map[string]any
	if err := json.Unmarshal(events[0].Data, &parsed); err != nil {
		t.Fatalf("invalid JSON data: %v", err)
	}
	if parsed["type"] != "content_block_start" {
		t.Errorf("data.type = %v, want 'content_block_start'", parsed["type"])
	}
}

func TestSSEParserChunkedStream(t *testing.T) {
	// Matches Rust test: parses_chunked_stream
	parser := NewSSEParser()

	first := []byte("event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"Hel")
	second := []byte("lo\"}}\n\n")

	events, err := parser.Push(first)
	if err != nil {
		t.Fatalf("first push error: %v", err)
	}
	if len(events) != 0 {
		t.Fatalf("first push should yield 0 events, got %d", len(events))
	}

	events, err = parser.Push(second)
	if err != nil {
		t.Fatalf("second push error: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("second push should yield 1 event, got %d", len(events))
	}

	var parsed map[string]any
	json.Unmarshal(events[0].Data, &parsed)
	delta := parsed["delta"].(map[string]any)
	if delta["text"] != "Hello" {
		t.Errorf("expected text='Hello', got %v", delta["text"])
	}
}

func TestSSEParserIgnoresPingAndDone(t *testing.T) {
	// Matches Rust test: ignores_ping_and_done
	parser := NewSSEParser()

	payload := strings.Join([]string{
		": keepalive",
		"event: ping",
		"data: {\"type\":\"ping\"}",
		"",
		"event: message_delta",
		"data: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\"}}",
		"",
		"event: message_stop",
		"data: {\"type\":\"message_stop\"}",
		"",
		"data: [DONE]",
		"",
	}, "\n")

	events, err := parser.Push([]byte(payload))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(events) != 2 {
		t.Fatalf("expected 2 events (message_delta + message_stop), got %d", len(events))
	}
	if events[0].Event != "message_delta" {
		t.Errorf("events[0].Event = %q, want 'message_delta'", events[0].Event)
	}
	if events[1].Event != "message_stop" {
		t.Errorf("events[1].Event = %q, want 'message_stop'", events[1].Event)
	}
}

func TestSSEParserDataLessEventFrame(t *testing.T) {
	// Matches Rust test: ignores_data_less_event_frames
	parser := NewSSEParser()
	events, err := parser.Push([]byte("event: ping\n\n"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(events) != 0 {
		t.Errorf("expected 0 events for data-less frame, got %d", len(events))
	}
}

func TestSSEParserSplitJSONAcrossDataLines(t *testing.T) {
	// Matches Rust test: parses_split_json_across_data_lines
	parser := NewSSEParser()
	frame := strings.Join([]string{
		"event: content_block_delta",
		"data: {\"type\":\"content_block_delta\",\"index\":0,",
		"data: \"delta\":{\"type\":\"text_delta\",\"text\":\"Hello\"}}",
		"",
		"",
	}, "\n")

	events, err := parser.Push([]byte(frame))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}

	var parsed map[string]any
	json.Unmarshal(events[0].Data, &parsed)
	delta := parsed["delta"].(map[string]any)
	if delta["text"] != "Hello" {
		t.Errorf("expected text='Hello', got %v", delta["text"])
	}
}

func TestSSEParserMultipleEvents(t *testing.T) {
	parser := NewSSEParser()

	payload := strings.Join([]string{
		"event: content_block_start",
		"data: {\"type\":\"content_block_start\",\"index\":0}",
		"",
		"event: content_block_delta",
		"data: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"Hi\"}}",
		"",
		"event: content_block_stop",
		"data: {\"type\":\"content_block_stop\",\"index\":0}",
		"",
		"",
	}, "\n")

	events, err := parser.Push([]byte(payload))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(events) != 3 {
		t.Fatalf("expected 3 events, got %d", len(events))
	}
}

func TestSSEParserWindowsLineEndings(t *testing.T) {
	parser := NewSSEParser()
	frame := "event: message_stop\r\ndata: {\"type\":\"message_stop\"}\r\n\r\n"

	events, err := parser.Push([]byte(frame))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event with CRLF, got %d", len(events))
	}
}

func TestSSEParserFinishTrailingData(t *testing.T) {
	parser := NewSSEParser()

	// Push data without a terminating \n\n
	_, err := parser.Push([]byte("event: message_stop\ndata: {\"type\":\"message_stop\"}"))
	if err != nil {
		t.Fatalf("push error: %v", err)
	}

	events, err := parser.Finish()
	if err != nil {
		t.Fatalf("finish error: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event from Finish, got %d", len(events))
	}
}

func TestSSEParserFinishEmpty(t *testing.T) {
	parser := NewSSEParser()
	events, err := parser.Finish()
	if err != nil {
		t.Fatalf("finish error: %v", err)
	}
	if len(events) != 0 {
		t.Errorf("expected 0 events from empty Finish, got %d", len(events))
	}
}

func TestSSEParserMalformedJSON(t *testing.T) {
	parser := NewSSEParser().WithContext("anthropic", "claude-sonnet")
	_, err := parser.Push([]byte("event: test\ndata: {invalid json}\n\n"))
	if err == nil {
		t.Error("expected error for malformed JSON")
	}
	if !strings.Contains(err.Error(), "anthropic") {
		t.Error("error should include provider context")
	}
}

func TestSSEParserCommentOnlyFrame(t *testing.T) {
	parser := NewSSEParser()
	events, err := parser.Push([]byte(": this is a comment\n\n"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(events) != 0 {
		t.Errorf("expected 0 events for comment-only frame, got %d", len(events))
	}
}

func TestSSEParserFuzz(t *testing.T) {
	// Fuzz test: the parser should never panic on arbitrary input.
	f := func(data []byte) bool {
		parser := NewSSEParser()
		_, _ = parser.Push(data)
		_, _ = parser.Finish()
		return true
	}
	if err := quick.Check(f, &quick.Config{MaxCount: 1000}); err != nil {
		t.Errorf("fuzz test failed: %v", err)
	}
}
