package runtime

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"claw-code-go/internal/api"
)

// Compaction continuation constants (matching Rust compact.rs).
const (
	compactContinuationPreamble    = "This session is being continued from a previous conversation that ran out of context. The summary below covers the earlier portion of the conversation.\n\n"
	compactRecentMessagesNote      = "Recent messages are preserved verbatim."
	compactDirectResumeInstruction = "Continue the conversation from where it left off without asking the user any further questions. Resume directly — do not acknowledge the summary, do not recap what was happening, and do not preface with continuation text."
)

// analysisTagRe strips <analysis>...</analysis> tags from summaries.
var analysisTagRe = regexp.MustCompile(`(?s)<analysis>.*?</analysis>`)

// summaryTagRe extracts content from <summary>...</summary> tags.
var summaryTagRe = regexp.MustCompile(`(?s)<summary>(.*?)</summary>`)

const (
	// DefaultCompactionThreshold triggers compaction when input tokens reach
	// this fraction of MaxTokens (e.g., 0.75 = 75%).
	DefaultCompactionThreshold = 0.75

	// DefaultCompactionKeepRecent is the number of recent messages retained
	// verbatim after compaction.
	DefaultCompactionKeepRecent = 10

	// charsPerToken is the approximate character-to-token ratio used for estimation.
	charsPerToken = 4
)

// CompactionState tracks token usage and compaction history across turns.
type CompactionState struct {
	LastInputTokens   int // input tokens from the most recently completed turn
	TotalInputTokens  int // cumulative input tokens across all turns
	TotalOutputTokens int // cumulative output tokens across all turns
	CompactionCount   int // number of times the session has been compacted
}

// EstimateTokens roughly estimates the number of tokens in a slice of messages
// using a simple chars-per-token heuristic.
func EstimateTokens(messages []api.Message) int {
	var total int
	for _, msg := range messages {
		for _, cb := range msg.Content {
			total += len(cb.Text) / charsPerToken
			for _, inner := range cb.Content {
				total += len(inner.Text) / charsPerToken
			}
		}
	}
	return total
}

// ShouldCompact returns true when the session should be compacted.
// It uses the actual API-reported input token count when available (> 0),
// falling back to EstimateTokens.
func ShouldCompact(inputTokens int, messages []api.Message, cfg *Config) bool {
	if !cfg.CompactionEnabled {
		return false
	}
	if inputTokens <= 0 {
		inputTokens = EstimateTokens(messages)
	}
	threshold := int(float64(cfg.MaxTokens) * cfg.CompactionThreshold)
	return inputTokens >= threshold
}

// collectStreamText drains a StreamEvent channel and returns the concatenated
// text response, or an error if the stream encountered one.
func collectStreamText(ch <-chan api.StreamEvent) (string, error) {
	var sb strings.Builder
	for event := range ch {
		switch event.Type {
		case api.EventError:
			return "", fmt.Errorf("stream error: %s", event.ErrorMessage)
		case api.EventContentBlockDelta:
			if event.Delta.Type == "text_delta" {
				sb.WriteString(event.Delta.Text)
			}
		}
	}
	return sb.String(), nil
}

// buildTranscript constructs a plain-text transcript of the messages suitable
// for submission to the summarization model.
func buildTranscript(messages []api.Message) string {
	var sb strings.Builder
	sb.WriteString("Please provide a concise but thorough summary of the following conversation. ")
	sb.WriteString("Preserve all important technical details: file paths modified, commands run, ")
	sb.WriteString("decisions made, errors encountered, and the current state of any ongoing work. ")
	sb.WriteString("The summary will replace the conversation history and must be self-contained.\n\n")
	sb.WriteString("---CONVERSATION---\n")

	for _, msg := range messages {
		fmt.Fprintf(&sb, "\n[%s]:\n", strings.ToUpper(msg.Role))
		for _, cb := range msg.Content {
			switch cb.Type {
			case "text":
				if len(cb.Text) > 1000 {
					sb.WriteString(cb.Text[:1000])
					sb.WriteString("... [truncated]\n")
				} else {
					sb.WriteString(cb.Text)
					sb.WriteString("\n")
				}
			case "tool_use":
				fmt.Fprintf(&sb, "[Tool call: %s]\n", cb.Name)
			case "tool_result":
				for _, inner := range cb.Content {
					if len(inner.Text) > 300 {
						fmt.Fprintf(&sb, "[Tool result: %s... (truncated)]\n", inner.Text[:300])
					} else {
						fmt.Fprintf(&sb, "[Tool result: %s]\n", inner.Text)
					}
				}
			}
		}
	}
	sb.WriteString("\n---END CONVERSATION---\n")
	return sb.String()
}

// CompactSession summarizes the session's message history by calling the model,
// stores the summary in the session, and trims the message list to the most
// recent cfg.CompactionKeepRecent messages. Returns the summary text.
func CompactSession(ctx context.Context, client api.APIClient, cfg *Config, session *Session) (string, error) {
	if len(session.Messages) == 0 {
		return "", nil
	}

	transcript := buildTranscript(session.Messages)

	req := api.CreateMessageRequest{
		Model:     cfg.Model,
		MaxTokens: 2048,
		Messages: []api.Message{
			{
				Role: "user",
				Content: []api.ContentBlock{
					{Type: "text", Text: transcript},
				},
			},
		},
		Stream: true,
	}

	ch, err := client.StreamResponse(ctx, req)
	if err != nil {
		return "", fmt.Errorf("compact: stream response: %w", err)
	}

	summary, err := collectStreamText(ch)
	if err != nil {
		return "", fmt.Errorf("compact: collect stream: %w", err)
	}

	// Merge with previous summary if exists (must happen BEFORE building
	// the continuation message so the injected context includes all prior
	// compacted material — matching Rust's compact_session).
	mergedSummary := summary
	if session.CompactionSummary != "" {
		mergedSummary = MergeCompactSummaries(session.CompactionSummary, summary)
	}

	// Retain the most recent N messages verbatim.
	keepCount := cfg.CompactionKeepRecent
	if keepCount > len(session.Messages) {
		keepCount = len(session.Messages)
	}
	recent := make([]api.Message, keepCount)
	copy(recent, session.Messages[len(session.Messages)-keepCount:])

	// Use the merged summary for the continuation message so that prior
	// compacted context is preserved (GetContinuationMessage formats internally).
	continuationMsg := GetContinuationMessage(mergedSummary, true, keepCount > 0)

	// Build new message list: continuation + recent.
	newMessages := make([]api.Message, 0, 1+len(recent))
	newMessages = append(newMessages, continuationMsg)
	newMessages = append(newMessages, recent...)

	session.CompactionSummary = mergedSummary
	session.CompactionCount++
	session.Messages = newMessages

	return summary, nil
}

// FormatCompactSummary cleans and formats a compaction summary for injection
// into the system prompt. Strips <analysis> tags and extracts <summary> content.
func FormatCompactSummary(summary string) string {
	// Strip <analysis>...</analysis> tags.
	cleaned := analysisTagRe.ReplaceAllString(summary, "")

	// Replace <summary>...</summary> tags with "Summary:\n" prefix.
	if matches := summaryTagRe.FindStringSubmatch(cleaned); len(matches) > 1 {
		cleaned = summaryTagRe.ReplaceAllString(cleaned, "Summary:\n"+strings.TrimSpace(matches[1]))
	}

	// Collapse multiple blank lines.
	cleaned = collapseBlankLines(cleaned)

	return strings.TrimSpace(cleaned)
}

// collapseBlankLines replaces runs of multiple blank lines with a single blank line.
var multipleBlankLinesRe = regexp.MustCompile(`\n{3,}`)

func collapseBlankLines(s string) string {
	return multipleBlankLinesRe.ReplaceAllString(s, "\n\n")
}

// GetContinuationMessage creates a synthetic system message that announces the
// compaction event, suitable for prepending to the retained recent messages.
//
// The summary is normalized via FormatCompactSummary before injection (matching
// Rust's get_compact_continuation_message which calls format_compact_summary
// internally).
//
// When suppressFollowUp is true, the message includes an instruction to not
// acknowledge the summary or ask follow-up questions.
// When recentPreserved is true, a note about preserved recent messages is added.
func GetContinuationMessage(summary string, suppressFollowUp, recentPreserved bool) api.Message {
	var sb strings.Builder
	sb.WriteString(compactContinuationPreamble)
	sb.WriteString(FormatCompactSummary(summary))

	if recentPreserved {
		sb.WriteString("\n\n")
		sb.WriteString(compactRecentMessagesNote)
	}

	if suppressFollowUp {
		sb.WriteString("\n")
		sb.WriteString(compactDirectResumeInstruction)
	}

	return api.Message{
		Role: "system",
		Content: []api.ContentBlock{
			{Type: "text", Text: sb.String()},
		},
	}
}

// MergeCompactSummaries merges two compaction summaries into one with structured
// sections. The current summary is formatted via FormatCompactSummary before
// extracting highlights and timeline, preventing raw <analysis> tags from
// leaking into the merged output (matching Rust merge_compact_summaries).
func MergeCompactSummaries(previous, current string) string {
	if previous == "" {
		return current
	}
	if current == "" {
		return previous
	}

	prevHighlights := extractSummaryHighlights(previous)
	newFormatted := FormatCompactSummary(current)
	newHighlights := extractSummaryHighlights(newFormatted)
	newTimeline := extractSummaryTimeline(newFormatted)

	var sb strings.Builder
	sb.WriteString("<summary>\n")
	sb.WriteString("Conversation summary:\n")

	if len(prevHighlights) > 0 {
		sb.WriteString("- Previously compacted context:\n")
		for _, h := range prevHighlights {
			sb.WriteString("  ")
			sb.WriteString(h)
			sb.WriteString("\n")
		}
	}

	if len(newHighlights) > 0 {
		sb.WriteString("- Newly compacted context:\n")
		for _, h := range newHighlights {
			sb.WriteString("  ")
			sb.WriteString(h)
			sb.WriteString("\n")
		}
	}

	if len(newTimeline) > 0 {
		sb.WriteString("- Key timeline:\n")
		for _, h := range newTimeline {
			sb.WriteString("  ")
			sb.WriteString(h)
			sb.WriteString("\n")
		}
	}

	sb.WriteString("</summary>")
	return sb.String()
}

// extractSummaryHighlights extracts non-timeline content lines from a summary.
// The summary is formatted via FormatCompactSummary first (matching Rust which
// calls format_compact_summary inside extract_summary_highlights).
func extractSummaryHighlights(summary string) []string {
	var highlights []string
	inTimeline := false
	formatted := FormatCompactSummary(summary)
	for _, line := range strings.Split(formatted, "\n") {
		trimmed := strings.TrimRight(line, " \t\r")
		if trimmed == "" || trimmed == "Summary:" || trimmed == "Conversation summary:" {
			continue
		}
		if trimmed == "- Key timeline:" {
			inTimeline = true
			continue
		}
		if inTimeline {
			continue
		}
		highlights = append(highlights, trimmed)
	}
	return highlights
}

// extractSummaryTimeline extracts timeline lines from a summary.
// The summary is formatted via FormatCompactSummary first (matching Rust which
// calls format_compact_summary inside extract_summary_timeline).
func extractSummaryTimeline(summary string) []string {
	var timeline []string
	inTimeline := false
	formatted := FormatCompactSummary(summary)
	for _, line := range strings.Split(formatted, "\n") {
		trimmed := strings.TrimRight(line, " \t\r")
		if trimmed == "- Key timeline:" {
			inTimeline = true
			continue
		}
		if !inTimeline {
			continue
		}
		if trimmed == "" {
			break
		}
		timeline = append(timeline, trimmed)
	}
	return timeline
}
