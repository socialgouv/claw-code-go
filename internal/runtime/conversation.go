package runtime

import (
	"claw-code-go/hooks"
	"claw-code-go/internal/api"
	"claw-code-go/internal/apikit"
	clawctx "claw-code-go/internal/context"
	"claw-code-go/internal/mcp"
	"claw-code-go/internal/permissions"
	"claw-code-go/internal/tools"
	"claw-code-go/internal/usage"
	"claw-code-go/plugin"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync/atomic"
)

const systemPromptBase = `You are Claude Code, an AI assistant for software engineering tasks. You have access to tools for running bash commands, reading and writing files, searching with glob patterns, and grepping for patterns in code. Use these tools to help users with coding tasks.`

// ConversationLoop manages the agentic conversation loop with tool use.
type ConversationLoop struct {
	Client          api.APIClient // provider-agnostic client interface
	Session         *Session
	Tools           []api.Tool
	Permissions     *Permissions
	PermManager     *permissions.Manager // Phase 5 permission manager (may be nil)
	Config          *Config
	MCPRegistry     *mcp.Registry          // MCP server registry (may be nil)
	Compaction      CompactionState        // Phase 6 token tracking and compaction state
	CtxAssembler    *clawctx.Assembler     // Phase 12 context assembler (may be nil)
	Usage           *usage.Tracker         // Phase 13 per-session token usage tracker
	TelemetrySink   apikit.TelemetrySink   // Telemetry event sink (may be nil)
	Tracer          *apikit.SessionTracer  // Session telemetry tracer (may be nil)
	PluginRegistry  *plugin.PluginRegistry // Plugin registry (may be nil)
	HookRunner      *hooks.HookRunner      // Hook runner (may be nil; wired from config + plugin hooks)
	CommandRegistry interface{}            // Slash command registry (may be nil; *commands.Registry)

	// toolCallCount is the number of tool_use blocks executed in this session.
	// Incremented atomically in ExecuteTool for goroutine safety (Rust parity:
	// pending_tool_use_count is tracked per-turn; we track cumulative for the
	// session to feed ToolCallCount() on the LoopAdapter).
	toolCallCount atomic.Int64

	// Build-time info passed through to LoopAdapter. Set by the caller.
	BuildVersion string
	BuildCommit  string
}

// ToolCallCount returns the total number of tool calls executed in this
// conversation loop (goroutine-safe).
func (loop *ConversationLoop) ToolCallCount() int {
	return int(loop.toolCallCount.Load())
}

// NewConversationLoop creates a new conversation loop with the given client.
// Use NewProviderClient to create an appropriate client for the configured provider.
func NewConversationLoop(cfg *Config, client api.APIClient) *ConversationLoop {
	workDir, _ := os.Getwd()
	return &ConversationLoop{
		Client:  client,
		Session: NewSession(),
		Tools: []api.Tool{
			tools.BashTool(),
			tools.ReadFileTool(),
			tools.WriteFileTool(),
			tools.GlobTool(),
			tools.GrepTool(),
			tools.FileEditTool(),
			tools.WebFetchTool(),
			tools.WebSearchTool(),
			tools.AskUserQuestionTool(),
			tools.TodoWriteTool(),
		},
		Permissions:  DefaultPermissions(),
		Config:       cfg,
		CtxAssembler: clawctx.NewAssembler(workDir),
		Usage:        usage.NewTracker(cfg.Model),
	}
}

// currentPermMode returns the active permission mode for the conversation.
func (loop *ConversationLoop) currentPermMode() permissions.PermissionMode {
	if loop.PermManager != nil {
		return loop.PermManager.Mode
	}
	// Fall back to config string.
	mode, err := permissions.ParsePermissionMode(loop.Config.PermissionMode)
	if err != nil {
		return permissions.ModePrompt
	}
	return mode
}

// workspaceRoot returns the workspace root directory.
func (loop *ConversationLoop) workspaceRoot() string {
	if loop.CtxAssembler != nil {
		return loop.CtxAssembler.WorkDir
	}
	wd, _ := os.Getwd()
	return wd
}

// SystemPrompt returns the rendered system prompt for diagnostic use.
func (loop *ConversationLoop) SystemPrompt() string {
	return loop.systemPrompt()
}

// systemPrompt returns the system prompt, optionally injecting project context,
// compaction summary, and MCP tool context.
func (loop *ConversationLoop) systemPrompt() string {
	var parts []string
	parts = append(parts, systemPromptBase)

	// Inject project context (Phase 12): environment, git status, CLAUDE.md.
	if loop.CtxAssembler != nil {
		if ctx := loop.CtxAssembler.Assemble(); ctx != "" {
			parts = append(parts, ctx)
		}
	}

	// Inject compaction summary when the session has one (Phase 6).
	if loop.Session != nil && loop.Session.CompactionSummary != "" {
		parts = append(parts, FormatCompactSummary(loop.Session.CompactionSummary))
	}

	// Append MCP tool list if any servers are connected.
	if loop.MCPRegistry != nil {
		mcpTools := loop.MCPRegistry.AllTools()
		if len(mcpTools) > 0 {
			names := make([]string, len(mcpTools))
			for i, t := range mcpTools {
				names[i] = t.Name
			}
			parts = append(parts, "Additional tools available via MCP: "+strings.Join(names, ", ")+".")
		}
	}

	return strings.Join(parts, "\n\n")
}

// allTools returns built-in tools merged with any MCP tools.
func (loop *ConversationLoop) allTools() []api.Tool {
	if loop.MCPRegistry == nil {
		return loop.Tools
	}
	mcpAPITools := loop.MCPRegistry.AllAPITools()
	if len(mcpAPITools) == 0 {
		return loop.Tools
	}
	combined := make([]api.Tool, 0, len(loop.Tools)+len(mcpAPITools))
	combined = append(combined, loop.Tools...)
	combined = append(combined, mcpAPITools...)
	return combined
}

// SendMessage sends a user message and runs the full agentic loop.
func (loop *ConversationLoop) SendMessage(ctx context.Context, userText string) error {
	// Append user message
	loop.Session.Messages = append(loop.Session.Messages, api.Message{
		Role: "user",
		Content: []api.ContentBlock{
			{Type: "text", Text: userText},
		},
	})

	// Compact history if approaching the token budget (Phase 6).
	if ShouldCompact(loop.Compaction.LastInputTokens, loop.Session.Messages, loop.Config) {
		_, err := CompactSession(ctx, loop.Client, loop.Config, loop.Session)
		if err != nil {
			fmt.Fprintf(os.Stderr, "[compact] warning: %v\n", err)
		} else {
			loop.Compaction.CompactionCount++
		}
	}

	// Agentic loop: keep going until stop_reason is "end_turn"
	for {
		stopReason, err := loop.runOneTurn(ctx)
		if err != nil {
			return err
		}

		if stopReason != "tool_use" {
			break
		}
	}

	return nil
}

// runOneTurn sends the current session messages to the API and processes the response.
// Returns the stop_reason.
func (loop *ConversationLoop) runOneTurn(ctx context.Context) (string, error) {
	req := api.CreateMessageRequest{
		Model:     loop.Config.Model,
		MaxTokens: loop.Config.MaxTokens,
		System:    loop.systemPrompt(),
		Messages:  loop.Session.Messages,
		Tools:     loop.allTools(),
		Stream:    true,
	}

	ch, err := loop.Client.StreamResponse(ctx, req)
	if err != nil {
		return "", fmt.Errorf("stream response: %w", err)
	}

	// Accumulators for the current response
	type toolBlock struct {
		id          string
		name        string
		inputBuffer string
	}

	var (
		textBlocks   []api.ContentBlock
		toolBlocks   []toolBlock
		currentText  string
		currentTool  *toolBlock
		stopReason   string
		blockTypeMap = make(map[int]string) // index -> "text" or "tool_use"
	)

	for event := range ch {
		switch event.Type {
		case api.EventError:
			return "", fmt.Errorf("stream error: %s", event.ErrorMessage)

		case api.EventContentBlockStart:
			blockTypeMap[event.Index] = event.ContentBlock.Type
			if event.ContentBlock.Type == "tool_use" {
				tb := toolBlock{
					id:   event.ContentBlock.ID,
					name: event.ContentBlock.Name,
				}
				toolBlocks = append(toolBlocks, tb)
				currentTool = &toolBlocks[len(toolBlocks)-1]
			}

		case api.EventContentBlockDelta:
			switch event.Delta.Type {
			case "text_delta":
				currentText += event.Delta.Text
				fmt.Fprint(os.Stdout, event.Delta.Text)

			case "input_json_delta":
				if currentTool != nil {
					currentTool.inputBuffer += event.Delta.PartialJSON
				}
			}

		case api.EventContentBlockStop:
			bType, ok := blockTypeMap[event.Index]
			if ok && bType == "text" && currentText != "" {
				textBlocks = append(textBlocks, api.ContentBlock{
					Type: "text",
					Text: currentText,
				})
				currentText = ""
			}
			// Reset currentTool pointer (but keep toolBlocks slice)
			currentTool = nil

		case api.EventMessageDelta:
			stopReason = event.StopReason

		case api.EventMessageStop:
			// Stream complete
		}
	}

	// Ensure trailing newline after streaming text
	if len(textBlocks) > 0 || len(toolBlocks) > 0 {
		fmt.Fprintln(os.Stdout)
	}

	// Build the assistant message content
	var assistantContent []api.ContentBlock

	// Add text blocks first
	assistantContent = append(assistantContent, textBlocks...)

	// Add tool_use blocks
	for _, tb := range toolBlocks {
		var inputMap map[string]any
		if tb.inputBuffer != "" {
			if err := json.Unmarshal([]byte(tb.inputBuffer), &inputMap); err != nil {
				inputMap = map[string]any{"raw": tb.inputBuffer}
			}
		} else {
			inputMap = map[string]any{}
		}

		assistantContent = append(assistantContent, api.ContentBlock{
			Type:  "tool_use",
			ID:    tb.id,
			Name:  tb.name,
			Input: inputMap,
		})
	}

	// Append assistant message to session
	if len(assistantContent) > 0 {
		loop.Session.Messages = append(loop.Session.Messages, api.Message{
			Role:    "assistant",
			Content: assistantContent,
		})
	}

	// If stop_reason is tool_use, execute tools and append results
	if stopReason == "tool_use" {
		var toolResults []api.ContentBlock

		for _, tb := range toolBlocks {
			var inputMap map[string]any
			for _, cb := range assistantContent {
				if cb.Type == "tool_use" && cb.ID == tb.id {
					inputMap = cb.Input
					break
				}
			}
			if inputMap == nil {
				inputMap = map[string]any{}
			}

			fmt.Fprintf(os.Stdout, "\n[Tool: %s]\n", tb.name)
			result := loop.ExecuteTool(tb.name, inputMap)
			result.ToolUseID = tb.id
			toolResults = append(toolResults, result)
		}

		// Append tool results as a user message
		loop.Session.Messages = append(loop.Session.Messages, api.Message{
			Role:    "user",
			Content: toolResults,
		})
	}

	return stopReason, nil
}

// SendMessageStreaming sends a user message and runs the full agentic loop, emitting
// TurnEvents to the provided channel. The channel is NOT closed by this function;
// callers should close it after this returns.
func (loop *ConversationLoop) SendMessageStreaming(ctx context.Context, userText string, events chan<- TurnEvent) error {
	loop.Session.Messages = append(loop.Session.Messages, api.Message{
		Role: "user",
		Content: []api.ContentBlock{
			{Type: "text", Text: userText},
		},
	})

	// Compact history if approaching the token budget (Phase 6).
	if ShouldCompact(loop.Compaction.LastInputTokens, loop.Session.Messages, loop.Config) {
		_, err := CompactSession(ctx, loop.Client, loop.Config, loop.Session)
		if err != nil {
			fmt.Fprintf(os.Stderr, "[compact] warning: %v\n", err)
		} else {
			loop.Compaction.CompactionCount++
		}
	}

	var totalInput, totalOutput int

	for {
		stopReason, inTok, outTok, err := loop.runOneTurnStreaming(ctx, events)
		if err != nil {
			events <- TurnEvent{Type: TurnEventError, Err: err}
			return err
		}
		totalInput += inTok
		totalOutput += outTok

		if stopReason != "tool_use" {
			break
		}
	}

	// Update compaction state with the latest token counts (Phase 6).
	loop.Compaction.LastInputTokens = totalInput
	loop.Compaction.TotalInputTokens += totalInput
	loop.Compaction.TotalOutputTokens += totalOutput

	// Update the usage tracker (Phase 13).
	if loop.Usage != nil {
		loop.Usage.Add(totalInput, totalOutput, 0, 0)
	}

	events <- TurnEvent{
		Type:         TurnEventUsage,
		InputTokens:  totalInput,
		OutputTokens: totalOutput,
	}
	events <- TurnEvent{Type: TurnEventDone}
	return nil
}

// runOneTurnStreaming streams one API turn and sends TurnEvents.
// Returns stop_reason, inputTokens, outputTokens, error.
func (loop *ConversationLoop) runOneTurnStreaming(ctx context.Context, events chan<- TurnEvent) (string, int, int, error) {
	req := api.CreateMessageRequest{
		Model:     loop.Config.Model,
		MaxTokens: loop.Config.MaxTokens,
		System:    loop.systemPrompt(),
		Messages:  loop.Session.Messages,
		Tools:     loop.allTools(),
		Stream:    true,
	}

	ch, err := loop.Client.StreamResponse(ctx, req)
	if err != nil {
		return "", 0, 0, fmt.Errorf("stream response: %w", err)
	}

	type toolBlock struct {
		id          string
		name        string
		inputBuffer string
	}

	var (
		textBlocks   []api.ContentBlock
		toolBlocks   []toolBlock
		currentText  string
		currentTool  *toolBlock
		stopReason   string
		blockTypeMap = make(map[int]string)
		inputTokens  int
		outputTokens int
	)

	for event := range ch {
		switch event.Type {
		case api.EventError:
			return "", 0, 0, fmt.Errorf("stream error: %s", event.ErrorMessage)

		case api.EventMessageStart:
			inputTokens = event.InputTokens

		case api.EventContentBlockStart:
			blockTypeMap[event.Index] = event.ContentBlock.Type
			if event.ContentBlock.Type == "tool_use" {
				tb := toolBlock{
					id:   event.ContentBlock.ID,
					name: event.ContentBlock.Name,
				}
				toolBlocks = append(toolBlocks, tb)
				currentTool = &toolBlocks[len(toolBlocks)-1]
			}

		case api.EventContentBlockDelta:
			switch event.Delta.Type {
			case "text_delta":
				currentText += event.Delta.Text
				select {
				case events <- TurnEvent{Type: TurnEventTextDelta, Text: event.Delta.Text}:
				case <-ctx.Done():
					return "", 0, 0, ctx.Err()
				}
			case "input_json_delta":
				if currentTool != nil {
					currentTool.inputBuffer += event.Delta.PartialJSON
				}
			}

		case api.EventContentBlockStop:
			if bType, ok := blockTypeMap[event.Index]; ok && bType == "text" && currentText != "" {
				textBlocks = append(textBlocks, api.ContentBlock{Type: "text", Text: currentText})
				currentText = ""
			}
			currentTool = nil

		case api.EventMessageDelta:
			stopReason = event.StopReason
			outputTokens = event.Usage.OutputTokens

		case api.EventMessageStop:
			// stream complete
		}
	}

	// Build assistant message content
	var assistantContent []api.ContentBlock
	assistantContent = append(assistantContent, textBlocks...)

	for _, tb := range toolBlocks {
		var inputMap map[string]any
		if tb.inputBuffer != "" {
			if err := json.Unmarshal([]byte(tb.inputBuffer), &inputMap); err != nil {
				inputMap = map[string]any{"raw": tb.inputBuffer}
			}
		} else {
			inputMap = map[string]any{}
		}
		assistantContent = append(assistantContent, api.ContentBlock{
			Type:  "tool_use",
			ID:    tb.id,
			Name:  tb.name,
			Input: inputMap,
		})
	}

	if len(assistantContent) > 0 {
		loop.Session.Messages = append(loop.Session.Messages, api.Message{
			Role:    "assistant",
			Content: assistantContent,
		})
	}

	// Execute tools if needed
	if stopReason == "tool_use" {
		var toolResults []api.ContentBlock

		for _, tb := range toolBlocks {
			var inputMap map[string]any
			for _, cb := range assistantContent {
				if cb.Type == "tool_use" && cb.ID == tb.id {
					inputMap = cb.Input
					break
				}
			}
			if inputMap == nil {
				inputMap = map[string]any{}
			}

			summary := summarizeToolInput(inputMap)

			// --- Permission check (Phase 5) ---
			if loop.PermManager != nil {
				decision := loop.PermManager.Check(tb.name, summary)

				// Plan mode: describe without executing.
				if loop.PermManager.Mode == permissions.ModePlan {
					planResult := api.ContentBlock{
						Type:      "tool_result",
						ToolUseID: tb.id,
						Content:   []api.ContentBlock{{Type: "text", Text: fmt.Sprintf("[Plan: %s %s]", tb.name, summary)}},
					}
					toolResults = append(toolResults, planResult)
					continue
				}

				switch decision {
				case permissions.DecisionDeny:
					denied := api.ContentBlock{
						Type:      "tool_result",
						ToolUseID: tb.id,
						Content:   []api.ContentBlock{{Type: "text", Text: fmt.Sprintf("Permission denied for tool: %s", tb.name)}},
						IsError:   true,
					}
					toolResults = append(toolResults, denied)
					continue

				case permissions.DecisionAsk:
					replyCh := make(chan PermDecision, 1)
					select {
					case events <- TurnEvent{
						Type:      TurnEventPermissionAsk,
						ToolName:  tb.name,
						ToolInput: summary,
						PermReply: replyCh,
					}:
					case <-ctx.Done():
						return "", 0, 0, ctx.Err()
					}

					var userDecision PermDecision
					select {
					case userDecision = <-replyCh:
					case <-ctx.Done():
						return "", 0, 0, ctx.Err()
					}

					switch userDecision {
					case PermDecisionDeny:
						denied := api.ContentBlock{
							Type:      "tool_result",
							ToolUseID: tb.id,
							Content:   []api.ContentBlock{{Type: "text", Text: fmt.Sprintf("Permission denied for tool: %s", tb.name)}},
							IsError:   true,
						}
						toolResults = append(toolResults, denied)
						continue
					case PermDecisionAllowAlways:
						loop.PermManager.Remember(tb.name, summary, permissions.DecisionAllow, permissions.ScopeAlways)
					}
					// PermDecisionAllowOnce falls through to execution
				}
				// DecisionAllow falls through to execution
			}

			// ask_user: surface the question to the caller and wait for a reply.
			if tb.name == "ask_user" {
				question, _ := tools.AskUserInput(inputMap)
				if question == "" {
					question = "?"
				}
				replyCh := make(chan string, 1)
				select {
				case events <- TurnEvent{
					Type:         TurnEventAskUser,
					ToolName:     tb.name,
					ToolInput:    question,
					AskUserReply: replyCh,
				}:
				case <-ctx.Done():
					return "", 0, 0, ctx.Err()
				}
				var answer string
				select {
				case answer = <-replyCh:
				case <-ctx.Done():
					return "", 0, 0, ctx.Err()
				}
				toolResults = append(toolResults, api.ContentBlock{
					Type:      "tool_result",
					ToolUseID: tb.id,
					Content:   []api.ContentBlock{{Type: "text", Text: answer}},
				})
				continue
			}

			select {
			case events <- TurnEvent{Type: TurnEventToolStart, ToolName: tb.name, ToolInput: summary}:
			case <-ctx.Done():
				return "", 0, 0, ctx.Err()
			}

			result := loop.ExecuteTool(tb.name, inputMap)
			result.ToolUseID = tb.id
			toolResults = append(toolResults, result)

			resultText := ""
			if len(result.Content) > 0 {
				resultText = result.Content[0].Text
			}
			select {
			case events <- TurnEvent{Type: TurnEventToolDone, ToolName: tb.name, ToolResult: resultText}:
			case <-ctx.Done():
				return "", 0, 0, ctx.Err()
			}
		}

		loop.Session.Messages = append(loop.Session.Messages, api.Message{
			Role:    "user",
			Content: toolResults,
		})
	}

	return stopReason, inputTokens, outputTokens, nil
}

// Deprecated: ExecuteToolQuiet is kept for backward compatibility. New code
// should use ExecuteTool which includes hooks, plugin dispatch, and telemetry.
func (loop *ConversationLoop) ExecuteToolQuiet(name string, input map[string]any) api.ContentBlock {
	if !CheckPermission(loop.Permissions, name) {
		return api.ContentBlock{
			Type:    "tool_result",
			Content: []api.ContentBlock{{Type: "text", Text: fmt.Sprintf("Permission denied for tool: %s", name)}},
			IsError: true,
		}
	}

	var result string
	var err error

	switch name {
	case "bash":
		result, err = tools.ExecuteBash(input, loop.currentPermMode(), loop.workspaceRoot())
	case "read_file":
		result, err = tools.ExecuteReadFile(input)
	case "write_file":
		result, err = tools.ExecuteWriteFile(input)
	case "glob":
		result, err = tools.ExecuteGlob(input)
	case "grep":
		result, err = tools.ExecuteGrep(input)
	case "file_edit":
		result, err = tools.ExecuteFileEdit(input)
	case "web_fetch":
		result, err = tools.ExecuteWebFetch(input)
	case "web_search":
		result, err = tools.ExecuteWebSearch(input)
	case "ask_user":
		q, ok := tools.AskUserInput(input)
		if !ok {
			err = fmt.Errorf("ask_user: 'question' is required")
		} else {
			return tools.AskUserFallback(q)
		}
	case "todo_write":
		result, err = tools.ExecuteTodoWrite(input)
	default:
		// Fall back to MCP registry.
		if loop.MCPRegistry != nil {
			if client, _, ok := loop.MCPRegistry.FindTool(name); ok {
				mcpResult, mcpErr := client.CallTool(context.Background(), name, input)
				if mcpErr != nil {
					return api.ContentBlock{
						Type:    "tool_result",
						Content: []api.ContentBlock{{Type: "text", Text: fmt.Sprintf("Error: %v", mcpErr)}},
						IsError: true,
					}
				}
				text := mcpResultText(mcpResult)
				return api.ContentBlock{
					Type:    "tool_result",
					Content: []api.ContentBlock{{Type: "text", Text: text}},
					IsError: mcpResult.IsError,
				}
			}
		}
		err = fmt.Errorf("unknown tool: %s", name)
	}

	isError := err != nil
	text := result
	if err != nil {
		text = fmt.Sprintf("Error: %v", err)
	}

	return api.ContentBlock{
		Type:    "tool_result",
		Content: []api.ContentBlock{{Type: "text", Text: text}},
		IsError: isError,
	}
}

// summarizeToolInput returns a short human-readable summary of tool inputs.
func summarizeToolInput(input map[string]any) string {
	for _, key := range []string{"command", "path", "file_path", "pattern", "url", "query", "question"} {
		if v, ok := input[key].(string); ok {
			if len(v) > 60 {
				return v[:60] + "..."
			}
			return v
		}
	}
	return ""
}

// ClearSession resets the conversation history in the current session.
func (loop *ConversationLoop) ClearSession() {
	loop.Session.Messages = []api.Message{}
}

// ListSessions returns all session IDs saved in the configured session directory.
func (loop *ConversationLoop) ListSessions() ([]string, error) {
	return ListSessions(loop.Config.SessionDir)
}

// SaveCurrentSession persists the active session to disk, including usage data.
func (loop *ConversationLoop) SaveCurrentSession() error {
	if loop.Usage != nil {
		loop.Session.TotalInputTokens = loop.Usage.TotalInput
		loop.Session.TotalOutputTokens = loop.Usage.TotalOutput
		loop.Session.TotalTurns = loop.Usage.Turns
	}
	return SaveSession(loop.Config.SessionDir, loop.Session)
}

// LoadNamedSession replaces the active session with one loaded from disk by ID.
// Usage tracker state is restored from persisted session data.
func (loop *ConversationLoop) LoadNamedSession(id string) error {
	sess, err := LoadSession(loop.Config.SessionDir, id)
	if err != nil {
		return err
	}
	loop.Session = sess
	if loop.Usage != nil && sess.TotalTurns > 0 {
		loop.Usage.TotalInput = sess.TotalInputTokens
		loop.Usage.TotalOutput = sess.TotalOutputTokens
		loop.Usage.Turns = sess.TotalTurns
	}
	return nil
}

// ListSessionsWithMeta returns metadata for all saved sessions, sorted newest first.
func (loop *ConversationLoop) ListSessionsWithMeta() ([]SessionMeta, error) {
	return ListSessionsWithMeta(loop.Config.SessionDir)
}

// MessageCount returns the number of messages in the active session.
func (loop *ConversationLoop) MessageCount() int {
	if loop.Session == nil {
		return 0
	}
	return len(loop.Session.Messages)
}

// ExecuteTool dispatches to the appropriate tool implementation.
// When HookRunner is set, pre-tool and post-tool hooks wrap the execution.
func (loop *ConversationLoop) ExecuteTool(name string, input map[string]any) api.ContentBlock {
	if !CheckPermission(loop.Permissions, name) {
		return api.ContentBlock{
			Type:    "tool_result",
			Content: []api.ContentBlock{{Type: "text", Text: fmt.Sprintf("Permission denied for tool: %s", name)}},
			IsError: true,
		}
	}

	// Serialize input for hook payload.
	inputJSON, _ := json.Marshal(input)
	inputStr := string(inputJSON)

	// --- PreToolUse hooks ---
	var preHookMessages []string
	// Extract permission override from pre-hooks (Phase 3: wired when
	// PermissionContext support is added in Phase 4).
	var preHookPermOverride *hooks.PermissionDecision
	var preHookPermReason string
	if loop.HookRunner != nil {
		hookResult := loop.HookRunner.RunPreToolUse(name, inputStr)
		if hookResult.IsDenied() || hookResult.IsFailed() || hookResult.IsCancelled() {
			msg := strings.Join(hookResult.Messages, "\n")
			if msg == "" {
				msg = fmt.Sprintf("Hook denied tool %s", name)
			}
			return api.ContentBlock{
				Type:    "tool_result",
				Content: []api.ContentBlock{{Type: "text", Text: msg}},
				IsError: true,
			}
		}
		// Store pre-hook messages for merging after tool execution.
		preHookMessages = hookResult.Messages
		preHookPermOverride = hookResult.PermissionOverride
		preHookPermReason = hookResult.PermissionReason
		// Apply input mutation from hooks.
		if hookResult.UpdatedInput != "" {
			var updatedMap map[string]any
			if err := json.Unmarshal([]byte(hookResult.UpdatedInput), &updatedMap); err == nil {
				input = updatedMap
				inputStr = hookResult.UpdatedInput
			}
		}
	}
	// Wire hook permission override: if a pre-hook set a permission decision,
	// apply it. Deny is enforced immediately; allow is remembered in the
	// permission manager so later checks skip the ask flow.
	if preHookPermOverride != nil {
		switch *preHookPermOverride {
		case hooks.PermissionAllow:
			if loop.PermManager != nil {
				loop.PermManager.Remember(name, summarizeToolInput(input), permissions.DecisionAllow, permissions.ScopeAlways)
			}
		case hooks.PermissionDeny:
			msg := "Hook denied tool execution"
			if preHookPermReason != "" {
				msg = preHookPermReason
			}
			return api.ContentBlock{
				Type:    "tool_result",
				Content: []api.ContentBlock{{Type: "text", Text: msg}},
				IsError: true,
			}
		// hooks.PermissionAsk — fall through to normal permission flow
		default:
		}
	}

	// Increment the atomic tool-call counter (Rust parity: tracks total tool
	// executions for the session; used by LoopAdapter.ToolCallCount()).
	loop.toolCallCount.Add(1)

	// --- Telemetry: record tool execution start ---
	if loop.Tracer != nil {
		loop.Tracer.Record("tool_execute_start", map[string]any{
			"tool_name": name,
		})
	}

	var result string
	var err error

	switch name {
	case "bash":
		result, err = tools.ExecuteBash(input, loop.currentPermMode(), loop.workspaceRoot())
	case "read_file":
		result, err = tools.ExecuteReadFile(input)
	case "write_file":
		result, err = tools.ExecuteWriteFile(input)
	case "glob":
		result, err = tools.ExecuteGlob(input)
	case "grep":
		result, err = tools.ExecuteGrep(input)
	case "file_edit":
		result, err = tools.ExecuteFileEdit(input)
	case "web_fetch":
		result, err = tools.ExecuteWebFetch(input)
	case "web_search":
		result, err = tools.ExecuteWebSearch(input)
	case "ask_user":
		q, ok := tools.AskUserInput(input)
		if !ok {
			err = fmt.Errorf("ask_user: 'question' is required")
		} else {
			cb := tools.AskUserFallback(q)
			askText := cb.Content[0].Text
			fmt.Fprintf(os.Stdout, "%s\n", askText)
			// Run post-hooks and telemetry for ask_user (matching Rust behavior).
			askText = hooks.MergeHookFeedback(preHookMessages, askText, false)
			postResult := loop.runPostToolHooks(name, inputStr, askText, false)
			if postResult.IsDenied() || postResult.IsFailed() || postResult.IsCancelled() {
				askText = hooks.MergeHookFeedback(postResult.Messages, askText, true)
				cb.Content[0].Text = askText
				cb.IsError = true
			} else {
				askText = hooks.MergeHookFeedback(postResult.Messages, askText, false)
				cb.Content[0].Text = askText
			}
			if loop.Tracer != nil {
				loop.Tracer.Record("tool_execute_end", map[string]any{
					"tool_name": name,
					"is_error":  cb.IsError,
				})
			}
			return cb
		}
	case "todo_write":
		result, err = tools.ExecuteTodoWrite(input)
	default:
		// Try plugin tools first.
		if loop.PluginRegistry != nil {
			pluginTools, ptErr := loop.PluginRegistry.AggregatedTools()
			if ptErr == nil {
				for i := range pluginTools {
					pt := &pluginTools[i]
					if pt.Definition.Name == name {
						ptResult, ptExecErr := pt.Execute(json.RawMessage(inputJSON))
						ptIsError := ptExecErr != nil
						ptText := ptResult
						if ptExecErr != nil {
							ptText = fmt.Sprintf("Error: %v", ptExecErr)
							fmt.Fprintf(os.Stderr, "[Plugin tool %s error]: %v\n", name, ptExecErr)
						} else {
							fmt.Fprintf(os.Stdout, "%s\n", ptResult)
						}
						ptText = hooks.MergeHookFeedback(preHookMessages, ptText, ptIsError)
						postResult := loop.runPostToolHooks(name, inputStr, ptText, ptIsError)
						if postResult.IsDenied() || postResult.IsFailed() || postResult.IsCancelled() {
							ptIsError = true
						}
						ptText = hooks.MergeHookFeedback(postResult.Messages, ptText,
							postResult.IsDenied() || postResult.IsFailed() || postResult.IsCancelled())
						if loop.Tracer != nil {
							loop.Tracer.Record("tool_execute_end", map[string]any{
								"tool_name": name,
								"is_error":  ptIsError,
							})
						}
						return api.ContentBlock{
							Type:    "tool_result",
							Content: []api.ContentBlock{{Type: "text", Text: ptText}},
							IsError: ptIsError,
						}
					}
				}
			}
		}

		// Fall back to MCP registry.
		if loop.MCPRegistry != nil {
			if client, _, ok := loop.MCPRegistry.FindTool(name); ok {
				mcpResult, mcpErr := client.CallTool(context.Background(), name, input)
				if mcpErr != nil {
					fmt.Fprintf(os.Stderr, "[MCP tool %s error]: %v\n", name, mcpErr)
					mcpErrText := fmt.Sprintf("Error: %v", mcpErr)
					mcpErrText = hooks.MergeHookFeedback(preHookMessages, mcpErrText, true)
					postResult := loop.runPostToolHooks(name, inputStr, mcpErrText, true)
					mcpErrText = hooks.MergeHookFeedback(postResult.Messages, mcpErrText, true)
					return api.ContentBlock{
						Type:    "tool_result",
						Content: []api.ContentBlock{{Type: "text", Text: mcpErrText}},
						IsError: true,
					}
				}
				mcpText := mcpResultText(mcpResult)
				fmt.Fprintf(os.Stdout, "%s\n", mcpText)
				mcpIsError := mcpResult.IsError
				mcpText = hooks.MergeHookFeedback(preHookMessages, mcpText, false)
				postResult := loop.runPostToolHooks(name, inputStr, mcpText, mcpIsError)
				if postResult.IsDenied() || postResult.IsFailed() || postResult.IsCancelled() {
					mcpIsError = true
				}
				mcpText = hooks.MergeHookFeedback(postResult.Messages, mcpText,
					postResult.IsDenied() || postResult.IsFailed() || postResult.IsCancelled())
				return api.ContentBlock{
					Type:    "tool_result",
					Content: []api.ContentBlock{{Type: "text", Text: mcpText}},
					IsError: mcpIsError,
				}
			}
		}
		err = fmt.Errorf("unknown tool: %s", name)
	}

	isError := err != nil
	text := result
	if err != nil {
		text = fmt.Sprintf("Error: %v", err)
		fmt.Fprintf(os.Stderr, "[Tool %s error]: %v\n", name, err)
	} else {
		fmt.Fprintf(os.Stdout, "%s\n", result)
	}

	// --- Pre-hook message merging (Rust: merge_hook_feedback(pre_hook_result.messages(), output, false)) ---
	text = hooks.MergeHookFeedback(preHookMessages, text, false)

	// --- PostToolUse / PostToolUseFailure hooks ---
	postResult := loop.runPostToolHooks(name, inputStr, text, isError)
	if postResult.IsDenied() || postResult.IsFailed() || postResult.IsCancelled() {
		isError = true
	}
	text = hooks.MergeHookFeedback(postResult.Messages, text,
		postResult.IsDenied() || postResult.IsFailed() || postResult.IsCancelled())

	// --- Telemetry: record tool execution end ---
	if loop.Tracer != nil {
		loop.Tracer.Record("tool_execute_end", map[string]any{
			"tool_name": name,
			"is_error":  isError,
		})
	}

	return api.ContentBlock{
		Type: "tool_result",
		Content: []api.ContentBlock{
			{Type: "text", Text: text},
		},
		IsError: isError,
	}
}

// runPostToolHooks runs PostToolUse or PostToolUseFailure hooks if a HookRunner is configured.
// Returns the HookRunResult so callers can check for denial/failure/cancellation
// and merge hook messages into tool output (matching Rust behavior).
func (loop *ConversationLoop) runPostToolHooks(toolName, toolInput, toolOutput string, isError bool) hooks.HookRunResult {
	if loop.HookRunner == nil {
		return hooks.Allow(nil)
	}
	if isError {
		return loop.HookRunner.RunPostToolUseFailure(toolName, toolInput, toolOutput)
	}
	return loop.HookRunner.RunPostToolUse(toolName, toolInput, toolOutput, false)
}

// mcpResultText extracts the concatenated text from an MCP tool result.
func mcpResultText(r mcp.MCPToolResult) string {
	var parts []string
	for _, c := range r.Content {
		if c.Text != "" {
			parts = append(parts, c.Text)
		}
	}
	return strings.Join(parts, "\n")
}

// InitMCPFromConfig connects to all MCP servers defined in the config.
// Errors are printed but do not abort startup.
func (loop *ConversationLoop) InitMCPFromConfig(ctx context.Context) {
	if len(loop.Config.MCPServers) == 0 {
		return
	}
	if loop.MCPRegistry == nil {
		loop.MCPRegistry = mcp.NewRegistry()
	}
	for _, srv := range loop.Config.MCPServers {
		var transport mcp.Transport
		var err error

		switch strings.ToLower(srv.Transport) {
		case "stdio":
			var envPairs []string
			for k, v := range srv.Env {
				envPairs = append(envPairs, k+"="+v)
			}
			transport, err = mcp.NewStdioTransport(srv.Command, srv.Args, envPairs)
			if err != nil {
				fmt.Fprintf(os.Stderr, "[MCP] failed to start stdio server %q: %v\n", srv.Name, err)
				continue
			}
		case "sse", "http":
			auth := ""
			if tok, ok := srv.Env["AUTHORIZATION"]; ok {
				auth = tok
			}
			transport = mcp.NewSSETransport(srv.URL, auth)
		default:
			fmt.Fprintf(os.Stderr, "[MCP] unknown transport %q for server %q\n", srv.Transport, srv.Name)
			continue
		}

		if err := loop.MCPRegistry.AddServer(ctx, srv.Name, transport); err != nil {
			fmt.Fprintf(os.Stderr, "[MCP] failed to connect to server %q: %v\n", srv.Name, err)
		} else {
			toolCount := len(loop.MCPRegistry.ServerTools(srv.Name))
			fmt.Fprintf(os.Stdout, "[MCP] connected to %q (%d tools)\n", srv.Name, toolCount)
		}
	}
}

// MCPConnect connects to a named MCP server defined in config.
func (loop *ConversationLoop) MCPConnect(ctx context.Context, name string) error {
	if loop.MCPRegistry == nil {
		loop.MCPRegistry = mcp.NewRegistry()
	}
	for _, srv := range loop.Config.MCPServers {
		if srv.Name != name {
			continue
		}
		var transport mcp.Transport
		var err error
		switch strings.ToLower(srv.Transport) {
		case "stdio":
			var envPairs []string
			for k, v := range srv.Env {
				envPairs = append(envPairs, k+"="+v)
			}
			transport, err = mcp.NewStdioTransport(srv.Command, srv.Args, envPairs)
			if err != nil {
				return err
			}
		case "sse", "http":
			auth := ""
			if tok, ok := srv.Env["AUTHORIZATION"]; ok {
				auth = tok
			}
			transport = mcp.NewSSETransport(srv.URL, auth)
		default:
			return fmt.Errorf("unknown transport %q", srv.Transport)
		}
		return loop.MCPRegistry.AddServer(ctx, name, transport)
	}
	return fmt.Errorf("MCP server %q not found in config", name)
}

// MCPDisconnect disconnects from a named MCP server.
func (loop *ConversationLoop) MCPDisconnect(name string) error {
	if loop.MCPRegistry == nil {
		return fmt.Errorf("no MCP servers connected")
	}
	return loop.MCPRegistry.Disconnect(name)
}

// MCPList returns a human-readable summary of connected MCP servers and their tools.
func (loop *ConversationLoop) MCPList() string {
	if loop.MCPRegistry == nil {
		return "No MCP servers connected.\n"
	}
	names := loop.MCPRegistry.ServerNames()
	if len(names) == 0 {
		return "No MCP servers connected.\n"
	}
	var sb strings.Builder
	for _, name := range names {
		tools := loop.MCPRegistry.ServerTools(name)
		fmt.Fprintf(&sb, "Server: %s (%d tools)\n", name, len(tools))
		for _, t := range tools {
			desc := t.Description
			if len(desc) > 60 {
				desc = desc[:60] + "..."
			}
			fmt.Fprintf(&sb, "  - %s: %s\n", t.Name, desc)
		}
	}
	return sb.String()
}

// InitHooksFromConfig creates a HookRunner from the config's hook commands
// merged with any plugin-provided hooks. This wires the hooks system into
// the execution loop so that PreToolUse/PostToolUse/PostToolUseFailure hooks
// run around every tool dispatch.
func (loop *ConversationLoop) InitHooksFromConfig(configHooks hooks.HookConfig) {
	merged := configHooks

	// Merge plugin hooks if a PluginRegistry is available.
	if loop.PluginRegistry != nil {
		pluginHooks, err := loop.PluginRegistry.AggregatedHooks()
		if err != nil {
			fmt.Fprintf(os.Stderr, "[hooks] warning: failed to aggregate plugin hooks: %v\n", err)
		} else {
			merged = hooks.MergeConfigs(merged, hooks.HookConfig{
				PreToolUse:         pluginHooks.PreToolUse,
				PostToolUse:        pluginHooks.PostToolUse,
				PostToolUseFailure: pluginHooks.PostToolUseFailure,
			})
		}
	}

	// Only create a runner if there are hooks to run.
	if len(merged.PreToolUse) > 0 || len(merged.PostToolUse) > 0 || len(merged.PostToolUseFailure) > 0 {
		loop.HookRunner = hooks.NewHookRunner(merged)
	}
}

// InitTelemetry sets up the telemetry sink and session tracer.
// If path is empty, telemetry is disabled (NopTelemetrySink).
func (loop *ConversationLoop) InitTelemetry(path string) {
	if path == "" {
		loop.TelemetrySink = apikit.NopTelemetrySink{}
		return
	}

	sink, err := apikit.NewJsonlTelemetrySink(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[telemetry] warning: failed to open sink at %s: %v\n", path, err)
		loop.TelemetrySink = apikit.NopTelemetrySink{}
		return
	}

	loop.TelemetrySink = sink
	if loop.Session != nil {
		loop.Tracer = apikit.NewSessionTracer(loop.Session.ID, sink)
	}

	// Wire the tracer to the API client if it supports it.
	if loop.Tracer != nil {
		if c, ok := loop.Client.(*api.Client); ok {
			c.Tracer = loop.Tracer
		}
	}
}

// InitPlugins discovers and initializes plugins, wiring their tools and hooks.
func (loop *ConversationLoop) InitPlugins(registry *plugin.PluginRegistry) {
	if registry == nil {
		return
	}
	loop.PluginRegistry = registry

	// Register plugin tools as API tools.
	pluginTools, err := registry.AggregatedTools()
	if err != nil {
		fmt.Fprintf(os.Stderr, "[plugins] warning: failed to aggregate tools: %v\n", err)
		return
	}
	for _, pt := range pluginTools {
		var schema api.InputSchema
		if pt.Definition.InputSchema != nil {
			// Best-effort parse of the raw JSON schema into InputSchema.
			_ = json.Unmarshal(pt.Definition.InputSchema, &schema)
		}
		if schema.Type == "" {
			schema.Type = "object"
		}
		loop.Tools = append(loop.Tools, api.Tool{
			Name:        pt.Definition.Name,
			Description: pt.Definition.Description,
			InputSchema: schema,
		})
	}
}

// HandleSlashCommand dispatches a slash command through the LoopAdapter and
// CommandRegistry. Returns (handled, error). If the command is not a slash
// command (doesn't start with /), returns (false, nil).
func (loop *ConversationLoop) HandleSlashCommand(input string) (bool, error) {
	input = strings.TrimSpace(input)
	if !strings.HasPrefix(input, "/") {
		return false, nil
	}
	if loop.CommandRegistry == nil {
		return false, nil
	}

	// Create a LoopAdapter to satisfy all command interface contracts.
	adapter := NewLoopAdapter(loop)
	adapter.SetBuildInfo(loop.BuildVersion, loop.BuildCommit)

	// Type-assert to *commands.Registry interface.
	type commandExecutor interface {
		Execute(input string, loop interface{}) (bool, error)
	}
	reg, ok := loop.CommandRegistry.(commandExecutor)
	if !ok {
		return false, nil
	}

	return reg.Execute(input, adapter)
}
