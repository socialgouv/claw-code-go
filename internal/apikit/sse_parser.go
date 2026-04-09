package apikit

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
)

// SSEEvent represents a parsed Server-Sent Event with event type and JSON data.
type SSEEvent struct {
	Event string          // e.g., "content_block_delta", "message_delta"
	Data  json.RawMessage // the raw JSON payload
}

// SSEParser is a stateful SSE frame parser that buffers incoming bytes and
// yields complete SSE events. It handles chunked delivery, multi-line data
// fields, ping/[DONE] filtering, and both \n\n and \r\n\r\n separators.
//
// This matches Rust's SseParser in crates/api/src/sse.rs — a byte-buffered
// state machine that accumulates partial frames across multiple Push calls.
type SSEParser struct {
	buffer   []byte
	provider string
	model    string
}

// NewSSEParser creates an empty parser.
func NewSSEParser() *SSEParser {
	return &SSEParser{}
}

// WithContext attaches provider and model context for error reporting.
func (p *SSEParser) WithContext(provider, model string) *SSEParser {
	p.provider = provider
	p.model = model
	return p
}

// Push accumulates bytes and returns any complete SSE events found.
// Partial frames remain in the internal buffer until more data arrives.
func (p *SSEParser) Push(chunk []byte) ([]SSEEvent, error) {
	p.buffer = append(p.buffer, chunk...)
	var events []SSEEvent

	for {
		frame := p.nextFrame()
		if frame == "" {
			break
		}
		event, err := p.parseFrame(frame)
		if err != nil {
			return events, err
		}
		if event != nil {
			events = append(events, *event)
		}
	}

	return events, nil
}

// Finish processes any remaining buffered data as a final frame.
func (p *SSEParser) Finish() ([]SSEEvent, error) {
	if len(p.buffer) == 0 {
		return nil, nil
	}
	trailing := string(p.buffer)
	p.buffer = nil

	event, err := p.parseFrame(trailing)
	if err != nil {
		return nil, err
	}
	if event != nil {
		return []SSEEvent{*event}, nil
	}
	return nil, nil
}

// nextFrame extracts the next complete SSE frame delimited by \n\n or \r\n\r\n.
// Returns empty string if no complete frame is available.
func (p *SSEParser) nextFrame() string {
	// Try \n\n first (most common)
	pos := -1
	sepLen := 0
	if idx := bytes.Index(p.buffer, []byte("\n\n")); idx >= 0 {
		pos = idx
		sepLen = 2
	}
	// Try \r\n\r\n (Windows line endings)
	if idx := bytes.Index(p.buffer, []byte("\r\n\r\n")); idx >= 0 {
		if pos < 0 || idx < pos {
			pos = idx
			sepLen = 4
		}
	}

	if pos < 0 {
		return ""
	}

	frame := string(p.buffer[:pos])
	p.buffer = p.buffer[pos+sepLen:]
	return frame
}

// parseFrame parses a single SSE frame string into an event.
// Returns nil for ping events, [DONE] sentinels, comment-only frames, etc.
func (p *SSEParser) parseFrame(frame string) (*SSEEvent, error) {
	trimmed := strings.TrimSpace(frame)
	if trimmed == "" {
		return nil, nil
	}

	var dataLines []string
	var eventName string

	for _, line := range strings.Split(trimmed, "\n") {
		line = strings.TrimRight(line, "\r")

		// Comments (lines starting with ':')
		if strings.HasPrefix(line, ":") {
			continue
		}

		// Event type
		if after, found := strings.CutPrefix(line, "event:"); found {
			eventName = strings.TrimSpace(after)
			continue
		}

		// Data line
		if after, found := strings.CutPrefix(line, "data:"); found {
			dataLines = append(dataLines, strings.TrimLeft(after, " "))
		}
	}

	// Ping events are silently ignored.
	if eventName == "ping" {
		return nil, nil
	}

	// No data → nothing to parse.
	if len(dataLines) == 0 {
		return nil, nil
	}

	// Join multi-line data (handles split JSON across multiple data: fields).
	payload := strings.Join(dataLines, "\n")

	// [DONE] sentinel
	if payload == "[DONE]" {
		return nil, nil
	}

	// Validate it's at least valid JSON.
	if !json.Valid([]byte(payload)) {
		provider := p.provider
		if provider == "" {
			provider = "unknown"
		}
		model := p.model
		if model == "" {
			model = "unknown"
		}
		return nil, fmt.Errorf("SSE JSON parse error (provider=%s, model=%s): invalid JSON in frame: %.200s", provider, model, payload)
	}

	return &SSEEvent{
		Event: eventName,
		Data:  json.RawMessage(payload),
	}, nil
}
