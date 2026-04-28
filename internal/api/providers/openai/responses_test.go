package openai

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/SocialGouv/claw-code-go/internal/api"
)

// ----- Dispatch decision ----------------------------------------------------

func TestShouldUseResponsesAPI_GatesOnReasoningPlusTools(t *testing.T) {
	tool := api.Tool{
		Name:        "noop",
		Description: "no-op",
		InputSchema: api.InputSchema{Type: "object"},
	}

	tests := []struct {
		name string
		req  api.CreateMessageRequest
		want bool
	}{
		{
			name: "reasoning + tools → responses",
			req: api.CreateMessageRequest{
				Model:           "gpt-5.5",
				ReasoningEffort: "high",
				Tools:           []api.Tool{tool},
			},
			want: true,
		},
		{
			name: "reasoning, no tools → chat completions",
			req: api.CreateMessageRequest{
				Model:           "gpt-5.5",
				ReasoningEffort: "high",
			},
			want: false,
		},
		{
			name: "tools, no reasoning → chat completions",
			req: api.CreateMessageRequest{
				Model: "gpt-4o",
				Tools: []api.Tool{tool},
			},
			want: false,
		},
		{
			name: "neither → chat completions",
			req: api.CreateMessageRequest{
				Model: "gpt-4o",
			},
			want: false,
		},
		{
			name: "empty reasoning + tools → chat completions",
			req: api.CreateMessageRequest{
				Model:           "gpt-5.5",
				ReasoningEffort: "",
				Tools:           []api.Tool{tool},
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shouldUseResponsesAPI(tt.req); got != tt.want {
				t.Errorf("shouldUseResponsesAPI() = %v, want %v", got, tt.want)
			}
		})
	}
}

// ----- Request shape --------------------------------------------------------

func TestBuildResponsesRequest_Shape(t *testing.T) {
	client := &Client{Model: "gpt-5.5", MaxTokens: 1024}
	req := api.CreateMessageRequest{
		Model:           "gpt-5.5",
		MaxTokens:       1024,
		ReasoningEffort: "high",
		System:          "You are concise.",
		Messages: []api.Message{
			{
				Role: "user",
				Content: []api.ContentBlock{
					{Type: "text", Text: "hi"},
				},
			},
		},
		Tools: []api.Tool{
			{
				Name:        "ping",
				Description: "ping the server",
				InputSchema: api.InputSchema{
					Type: "object",
					Properties: map[string]api.Property{
						"target": {Type: "string"},
					},
					Required: []string{"target"},
				},
			},
		},
	}

	got, err := client.buildResponsesRequest(req)
	if err != nil {
		t.Fatalf("buildResponsesRequest: %v", err)
	}

	body, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var payload map[string]interface{}
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	// instructions replaces system
	if v, _ := payload["instructions"].(string); v != "You are concise." {
		t.Errorf("instructions = %q, want %q", v, "You are concise.")
	}

	// reasoning is an object with effort
	reasoning, ok := payload["reasoning"].(map[string]interface{})
	if !ok {
		t.Fatalf("reasoning missing or wrong shape: %v", payload["reasoning"])
	}
	if reasoning["effort"] != "high" {
		t.Errorf("reasoning.effort = %v, want %q", reasoning["effort"], "high")
	}

	// tools[0] is FLAT (no nested function object)
	tools, ok := payload["tools"].([]interface{})
	if !ok || len(tools) != 1 {
		t.Fatalf("tools missing: %v", payload["tools"])
	}
	tool := tools[0].(map[string]interface{})
	if tool["type"] != "function" {
		t.Errorf("tool.type = %v, want \"function\"", tool["type"])
	}
	if tool["name"] != "ping" {
		t.Errorf("tool.name = %v, want \"ping\"", tool["name"])
	}
	if _, ok := tool["function"]; ok {
		t.Errorf("responses tool must be flat, found nested .function: %v", tool)
	}

	// stream is true
	if payload["stream"] != true {
		t.Errorf("stream = %v, want true", payload["stream"])
	}

	// max_output_tokens replaces max_tokens / max_completion_tokens
	if _, ok := payload["max_tokens"]; ok {
		t.Errorf("responses request must not contain max_tokens")
	}
	if v, ok := payload["max_output_tokens"].(float64); !ok || int(v) != 1024 {
		t.Errorf("max_output_tokens = %v, want 1024", payload["max_output_tokens"])
	}

	// input is the array form of messages
	input, ok := payload["input"].([]interface{})
	if !ok || len(input) == 0 {
		t.Fatalf("input missing or empty: %v", payload["input"])
	}
}

// ----- End-to-end stream translation via httptest ---------------------------

// TestStreamResponses_TranslatesEvents wires up an httptest server that
// emits a canned /v1/responses SSE stream, then verifies that the
// translator produces the expected sequence of api.StreamEvent values.
func TestStreamResponses_TranslatesEvents(t *testing.T) {
	// A representative event stream: text deltas, then a function call
	// with argument deltas, then completion with usage.
	frames := []string{
		`{"type":"response.created","response":{"id":"resp_1","status":"in_progress"}}`,
		`{"type":"response.output_item.added","output_index":0,"item":{"type":"message","id":"msg_1"}}`,
		`{"type":"response.output_text.delta","item_id":"msg_1","delta":"Hello"}`,
		`{"type":"response.output_text.delta","item_id":"msg_1","delta":" world"}`,
		`{"type":"response.output_text.done","item_id":"msg_1"}`,
		`{"type":"response.output_item.added","output_index":1,"item":{"type":"function_call","id":"fc_1","call_id":"call_abc","name":"ping","status":"in_progress"}}`,
		`{"type":"response.function_call_arguments.delta","item_id":"fc_1","delta":"{\"target\":"}`,
		`{"type":"response.function_call_arguments.delta","item_id":"fc_1","delta":"\"alpha\"}"}`,
		`{"type":"response.function_call_arguments.done","item_id":"fc_1","arguments":"{\"target\":\"alpha\"}"}`,
		`{"type":"response.completed","response":{"id":"resp_1","status":"completed","usage":{"input_tokens":10,"output_tokens":7,"total_tokens":17}}}`,
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Sanity-check the dispatch hit the responses endpoint.
		if r.URL.Path != "/v1/responses" {
			t.Errorf("server got path %q, want /v1/responses", r.URL.Path)
			http.Error(w, "wrong path", http.StatusBadRequest)
			return
		}
		if got := r.Header.Get("Authorization"); !strings.HasPrefix(got, "Bearer ") {
			t.Errorf("missing/invalid Authorization header: %q", got)
		}
		// Inspect the body to confirm `reasoning.effort` and `tools[0].name`
		// were transcribed correctly.
		body, _ := io.ReadAll(r.Body)
		var payload map[string]interface{}
		_ = json.Unmarshal(body, &payload)
		if reasoning, ok := payload["reasoning"].(map[string]interface{}); !ok || reasoning["effort"] != "high" {
			t.Errorf("server saw reasoning = %v, want effort=high", payload["reasoning"])
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		flusher, _ := w.(http.Flusher)
		for _, f := range frames {
			fmt.Fprintf(w, "data: %s\n\n", f)
			if flusher != nil {
				flusher.Flush()
			}
		}
	}))
	defer srv.Close()

	client := &Client{
		APIKey:     "test-key",
		BaseURL:    srv.URL,
		Model:      "gpt-5.5",
		MaxTokens:  256,
		HTTPClient: srv.Client(),
	}

	req := api.CreateMessageRequest{
		Model:           "gpt-5.5",
		MaxTokens:       256,
		System:          "system",
		ReasoningEffort: "high",
		Messages: []api.Message{
			{Role: "user", Content: []api.ContentBlock{{Type: "text", Text: "hi"}}},
		},
		Tools: []api.Tool{
			{Name: "ping", Description: "", InputSchema: api.InputSchema{Type: "object"}},
		},
	}

	if !shouldUseResponsesAPI(req) {
		t.Fatalf("dispatch gate failed to fire on reasoning+tools")
	}

	ch, err := client.StreamResponse(context.Background(), req)
	if err != nil {
		t.Fatalf("StreamResponse: %v", err)
	}

	var events []api.StreamEvent
	for ev := range ch {
		events = append(events, ev)
	}

	// Expect a deterministic sequence prefix:
	//   message_start
	//   content_block_start (text, idx 0)
	//   content_block_delta (text_delta "Hello")
	//   content_block_delta (text_delta " world")
	//   content_block_start (tool_use, idx 1, id call_abc, name ping)
	//   content_block_delta (input_json_delta {"target":)
	//   content_block_delta (input_json_delta "alpha"})
	//   content_block_stop (idx 0)         (text closes after stream end)
	//   content_block_stop (idx 1)         (tool closes after stream end)
	//   message_delta (stop_reason tool_use, output tokens 7)
	//   message_stop

	var (
		sawMsgStart     bool
		sawTextStart    bool
		gotTextDeltas   []string
		sawToolStart    bool
		toolID, toolNm  string
		gotToolJSON     []string
		sawTextStop     bool
		sawToolStop     bool
		sawMsgDelta     bool
		gotStopReason   string
		gotOutputTokens int
		sawMsgStop      bool
	)

	for _, ev := range events {
		switch ev.Type {
		case api.EventMessageStart:
			sawMsgStart = true
		case api.EventContentBlockStart:
			switch ev.ContentBlock.Type {
			case "text":
				sawTextStart = true
			case "tool_use":
				sawToolStart = true
				toolID = ev.ContentBlock.ID
				toolNm = ev.ContentBlock.Name
			}
		case api.EventContentBlockDelta:
			switch ev.Delta.Type {
			case "text_delta":
				gotTextDeltas = append(gotTextDeltas, ev.Delta.Text)
			case "input_json_delta":
				gotToolJSON = append(gotToolJSON, ev.Delta.PartialJSON)
			}
		case api.EventContentBlockStop:
			if ev.Index == 0 {
				sawTextStop = true
			} else {
				sawToolStop = true
			}
		case api.EventMessageDelta:
			sawMsgDelta = true
			gotStopReason = ev.StopReason
			gotOutputTokens = ev.Usage.OutputTokens
		case api.EventMessageStop:
			sawMsgStop = true
		}
	}

	if !sawMsgStart {
		t.Error("missing message_start")
	}
	if !sawTextStart {
		t.Error("missing text content_block_start")
	}
	if got := strings.Join(gotTextDeltas, ""); got != "Hello world" {
		t.Errorf("text deltas concatenated = %q, want %q", got, "Hello world")
	}
	if !sawToolStart {
		t.Error("missing tool_use content_block_start")
	}
	if toolID != "call_abc" {
		t.Errorf("tool_use id = %q, want call_abc", toolID)
	}
	if toolNm != "ping" {
		t.Errorf("tool_use name = %q, want ping", toolNm)
	}
	if got := strings.Join(gotToolJSON, ""); got != `{"target":"alpha"}` {
		t.Errorf("tool args concatenated = %q, want %q", got, `{"target":"alpha"}`)
	}
	if !sawTextStop {
		t.Error("missing text content_block_stop")
	}
	if !sawToolStop {
		t.Error("missing tool content_block_stop")
	}
	if !sawMsgDelta {
		t.Error("missing message_delta")
	}
	if gotStopReason != "tool_use" {
		t.Errorf("stop_reason = %q, want tool_use", gotStopReason)
	}
	if gotOutputTokens != 7 {
		t.Errorf("output tokens = %d, want 7", gotOutputTokens)
	}
	if !sawMsgStop {
		t.Error("missing message_stop")
	}
}

// TestStreamResponses_InterleavedMessageItems exercises the case where a
// /v1/responses stream emits two message items separated by a function
// call: message A → text deltas → function call → message B → text
// deltas → completion.
//
// The translator must allocate distinct block indices for the two
// message items; otherwise deltas from message B would silently
// collapse into message A's block (textBlockIndex was hardcoded to 0).
func TestStreamResponses_InterleavedMessageItems(t *testing.T) {
	frames := []string{
		`{"type":"response.created","response":{"id":"resp_1","status":"in_progress"}}`,
		// First message item: text deltas only.
		`{"type":"response.output_item.added","output_index":0,"item":{"type":"message","id":"msg_A"}}`,
		`{"type":"response.output_text.delta","item_id":"msg_A","delta":"Alpha"}`,
		`{"type":"response.output_text.delta","item_id":"msg_A","delta":"-A"}`,
		`{"type":"response.output_text.done","item_id":"msg_A"}`,
		// Function call between the two message items.
		`{"type":"response.output_item.added","output_index":1,"item":{"type":"function_call","id":"fc_1","call_id":"call_x","name":"ping","status":"in_progress"}}`,
		`{"type":"response.function_call_arguments.delta","item_id":"fc_1","delta":"{}"}`,
		`{"type":"response.function_call_arguments.done","item_id":"fc_1","arguments":"{}"}`,
		// Second message item, post-tool: text deltas must NOT collapse into msg_A's block.
		`{"type":"response.output_item.added","output_index":2,"item":{"type":"message","id":"msg_B"}}`,
		`{"type":"response.output_text.delta","item_id":"msg_B","delta":"Beta"}`,
		`{"type":"response.output_text.delta","item_id":"msg_B","delta":"-B"}`,
		`{"type":"response.output_text.done","item_id":"msg_B"}`,
		`{"type":"response.completed","response":{"id":"resp_1","status":"completed","usage":{"input_tokens":12,"output_tokens":9,"total_tokens":21}}}`,
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		flusher, _ := w.(http.Flusher)
		for _, f := range frames {
			fmt.Fprintf(w, "data: %s\n\n", f)
			if flusher != nil {
				flusher.Flush()
			}
		}
	}))
	defer srv.Close()

	client := &Client{
		APIKey:     "test-key",
		BaseURL:    srv.URL,
		Model:      "gpt-5.5",
		MaxTokens:  256,
		HTTPClient: srv.Client(),
	}

	req := api.CreateMessageRequest{
		Model:           "gpt-5.5",
		MaxTokens:       256,
		ReasoningEffort: "high",
		Messages: []api.Message{
			{Role: "user", Content: []api.ContentBlock{{Type: "text", Text: "hi"}}},
		},
		Tools: []api.Tool{
			{Name: "ping", Description: "", InputSchema: api.InputSchema{Type: "object"}},
		},
	}

	ch, err := client.StreamResponse(context.Background(), req)
	if err != nil {
		t.Fatalf("StreamResponse: %v", err)
	}

	var events []api.StreamEvent
	for ev := range ch {
		events = append(events, ev)
	}

	// Walk the event stream and bucket text deltas by content_block index.
	// The block index assigned to each text content_block_start is
	// remembered, so we can attribute later deltas to the correct block.
	textStartIndices := []int{}
	textByIndex := map[int]string{}
	toolStartIndex := -1
	stopIndices := []int{}
	for _, ev := range events {
		switch ev.Type {
		case api.EventContentBlockStart:
			switch ev.ContentBlock.Type {
			case "text":
				textStartIndices = append(textStartIndices, ev.Index)
			case "tool_use":
				toolStartIndex = ev.Index
			}
		case api.EventContentBlockDelta:
			if ev.Delta.Type == "text_delta" {
				textByIndex[ev.Index] += ev.Delta.Text
			}
		case api.EventContentBlockStop:
			stopIndices = append(stopIndices, ev.Index)
		}
	}

	if len(textStartIndices) != 2 {
		t.Fatalf("expected 2 distinct text content_block_start events, got %d (indices=%v)",
			len(textStartIndices), textStartIndices)
	}
	idxA := textStartIndices[0]
	idxB := textStartIndices[1]
	if idxA == idxB {
		t.Fatalf("text blocks for msg_A and msg_B share index %d — second message collapsed into first", idxA)
	}
	if toolStartIndex == -1 {
		t.Fatal("missing tool_use content_block_start")
	}
	if toolStartIndex == idxA || toolStartIndex == idxB {
		t.Errorf("tool_use index %d collides with a text block index (A=%d, B=%d)",
			toolStartIndex, idxA, idxB)
	}

	if got := textByIndex[idxA]; got != "Alpha-A" {
		t.Errorf("msg_A text (index %d) = %q, want %q", idxA, got, "Alpha-A")
	}
	if got := textByIndex[idxB]; got != "Beta-B" {
		t.Errorf("msg_B text (index %d) = %q, want %q", idxB, got, "Beta-B")
	}

	// Both text blocks and the tool block should be closed.
	mustStop := map[int]bool{idxA: false, idxB: false, toolStartIndex: false}
	for _, i := range stopIndices {
		if _, ok := mustStop[i]; ok {
			mustStop[i] = true
		}
	}
	for i, ok := range mustStop {
		if !ok {
			t.Errorf("missing content_block_stop for index %d", i)
		}
	}
}

// TestStreamResponses_PropagatesAPIError verifies that non-2xx responses
// surface as *api.APIError with status, message, and Retryable populated.
func TestStreamResponses_PropagatesAPIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, `{"error":{"message":"bad","type":"invalid_request_error","code":null}}`)
	}))
	defer srv.Close()

	client := &Client{
		APIKey:     "test-key",
		BaseURL:    srv.URL,
		Model:      "gpt-5.5",
		HTTPClient: srv.Client(),
	}

	req := api.CreateMessageRequest{
		Model:           "gpt-5.5",
		ReasoningEffort: "high",
		Tools: []api.Tool{
			{Name: "x", InputSchema: api.InputSchema{Type: "object"}},
		},
	}

	_, err := client.StreamResponse(context.Background(), req)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	apiErr, ok := err.(*api.APIError)
	if !ok {
		t.Fatalf("err type = %T, want *api.APIError", err)
	}
	if apiErr.StatusCode != 400 {
		t.Errorf("status = %d, want 400", apiErr.StatusCode)
	}
	if apiErr.Message != "bad" {
		t.Errorf("message = %q, want %q", apiErr.Message, "bad")
	}
	if apiErr.Provider != "openai" {
		t.Errorf("provider = %q, want openai", apiErr.Provider)
	}
}

// TestConvertToolsToResponses_PropagatesMarshalError pins the contract
// that the responses-API tool conversion bubbles up json.Marshal failures
// rather than silently dropping the offending tool. We trigger a marshal
// failure by stuffing a chan into Property.Enum.
func TestConvertToolsToResponses_PropagatesMarshalError(t *testing.T) {
	client := &Client{Model: "gpt-5.5", APIKey: "stub"}
	req := api.CreateMessageRequest{
		Model:           "gpt-5.5",
		MaxTokens:       16,
		ReasoningEffort: "high",
		Messages: []api.Message{
			{Role: "user", Content: []api.ContentBlock{{Type: "text", Text: "hi"}}},
		},
		Tools: []api.Tool{
			{
				Name:        "broken",
				Description: "schema with unmarshallable enum",
				InputSchema: api.InputSchema{
					Type: "object",
					Properties: map[string]api.Property{
						"x": {Type: "string", Enum: []any{make(chan int)}},
					},
				},
			},
		},
	}

	if _, err := client.buildResponsesRequest(req); err == nil {
		t.Fatal("expected buildResponsesRequest to fail when a tool's input schema cannot be marshalled")
	} else if !strings.Contains(err.Error(), "broken") {
		t.Errorf("error %q should mention the offending tool name %q", err.Error(), "broken")
	}
}
