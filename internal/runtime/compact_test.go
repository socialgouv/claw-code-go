package runtime

import (
	"strings"
	"testing"
)

func TestFormatCompactSummaryStripsAnalysisTags(t *testing.T) {
	input := "<analysis>Some internal thinking</analysis>\n<summary>The actual summary content</summary>"
	result := FormatCompactSummary(input)

	if strings.Contains(result, "internal thinking") {
		t.Error("analysis tags should be stripped")
	}
	if !strings.Contains(result, "The actual summary content") {
		t.Error("summary content should be preserved")
	}
	if strings.Contains(result, "<compacted_context>") {
		t.Error("should NOT be wrapped in compacted_context tags")
	}
	if !strings.Contains(result, "Summary:") {
		t.Error("should contain Summary: prefix")
	}
}

func TestFormatCompactSummaryMatchesRust(t *testing.T) {
	// Matches Rust test: formats_compact_summary_like_upstream
	summary := "<analysis>scratch</analysis>\n<summary>Kept work</summary>"
	result := FormatCompactSummary(summary)
	if result != "Summary:\nKept work" {
		t.Errorf("format mismatch: got %q, want %q", result, "Summary:\nKept work")
	}
}

func TestFormatCompactSummaryNoTags(t *testing.T) {
	input := "Plain summary without tags"
	result := FormatCompactSummary(input)
	if !strings.Contains(result, "Plain summary without tags") {
		t.Error("plain text should be preserved")
	}
}

func TestFormatCompactSummaryExtractsSummaryTag(t *testing.T) {
	input := "Some preamble\n<summary>\nExtracted content\n</summary>\nSome epilogue"
	result := FormatCompactSummary(input)
	if !strings.Contains(result, "Extracted content") {
		t.Error("should extract summary tag content")
	}
	if !strings.Contains(result, "Summary:") {
		t.Error("should contain Summary: prefix")
	}
	if strings.Contains(result, "<compacted_context>") {
		t.Error("should NOT be wrapped in compacted_context tags")
	}
}

func TestGetContinuationMessageSuppressFollowUp(t *testing.T) {
	msg := GetContinuationMessage("summary text", true, false)
	if msg.Role != "system" {
		t.Errorf("Role = %q, want 'system'", msg.Role)
	}
	text := msg.Content[0].Text
	if !strings.Contains(text, "do not acknowledge the summary") {
		t.Error("suppressFollowUp should include direct resume instruction")
	}
	if !strings.Contains(text, "summary text") {
		t.Error("should include summary")
	}
}

func TestGetContinuationMessageNoSuppress(t *testing.T) {
	msg := GetContinuationMessage("summary text", false, false)
	text := msg.Content[0].Text
	if strings.Contains(text, "do not acknowledge") {
		t.Error("should not include suppress instruction when not suppressed")
	}
}

func TestGetContinuationMessageRecentPreserved(t *testing.T) {
	msg := GetContinuationMessage("summary", true, true)
	text := msg.Content[0].Text
	if !strings.Contains(text, "Recent messages are preserved verbatim") {
		t.Error("should include recent messages note")
	}
}

func TestGetContinuationMessageFormatsInternally(t *testing.T) {
	// GetContinuationMessage should call FormatCompactSummary on its input,
	// matching Rust's get_compact_continuation_message behavior.
	summary := "<analysis>scratch</analysis>\n<summary>Real content</summary>"
	msg := GetContinuationMessage(summary, false, false)
	text := msg.Content[0].Text
	if strings.Contains(text, "<analysis>") {
		t.Error("analysis tags should be stripped by GetContinuationMessage")
	}
	if !strings.Contains(text, "Real content") {
		t.Error("summary content should be preserved")
	}
}

func TestMergeCompactSummaries(t *testing.T) {
	result := MergeCompactSummaries("first summary", "second summary")
	if !strings.Contains(result, "first summary") || !strings.Contains(result, "second summary") {
		t.Error("should contain both summaries")
	}
	if !strings.Contains(result, "Previously compacted context:") {
		t.Error("should contain 'Previously compacted context:' section")
	}
	if !strings.Contains(result, "Newly compacted context:") {
		t.Error("should contain 'Newly compacted context:' section")
	}
}

func TestMergeCompactSummariesRustFormat(t *testing.T) {
	// Verify the merged format matches Rust: Conversation summary header,
	// "- " prefix on sections, "  " indentation on content.
	result := MergeCompactSummaries("first", "second")
	if !strings.Contains(result, "Conversation summary:") {
		t.Error("should contain 'Conversation summary:' header")
	}
	if !strings.Contains(result, "- Previously compacted context:") {
		t.Error("should have '- ' prefix on Previously compacted")
	}
	if !strings.Contains(result, "- Newly compacted context:") {
		t.Error("should have '- ' prefix on Newly compacted")
	}
	if !strings.Contains(result, "  first") {
		t.Error("content should be indented with two spaces")
	}
	if !strings.Contains(result, "  second") {
		t.Error("content should be indented with two spaces")
	}
}

func TestMergeCompactSummariesStripsAnalysisTags(t *testing.T) {
	// BLOCKER FIX: MergeCompactSummaries must format current before extraction
	// to prevent <analysis> tag leaks (matching Rust behavior).
	current := "<analysis>internal thinking</analysis>\n<summary>Actual summary\n- Key timeline:\n  - user: did something</summary>"
	result := MergeCompactSummaries("previous context", current)

	if strings.Contains(result, "internal thinking") {
		t.Error("analysis tags should be stripped from current before merging")
	}
	if strings.Contains(result, "<analysis>") {
		t.Error("analysis tags should not appear in merged output")
	}
	if !strings.Contains(result, "Actual summary") {
		t.Error("should preserve summary content after stripping analysis")
	}
}

func TestMergeCompactSummariesPreservesTimeline(t *testing.T) {
	current := "<summary>Conversation summary:\n- Scope: 5 messages.\n- Key timeline:\n  - user: asked question\n  - assistant: answered</summary>"
	result := MergeCompactSummaries("prior context", current)

	if !strings.Contains(result, "- Key timeline:") {
		t.Error("should preserve timeline section")
	}
	if !strings.Contains(result, "user: asked question") {
		t.Error("should preserve timeline entries")
	}
}

func TestCollapseBlankLines(t *testing.T) {
	input := "line1\n\n\n\nline2\n\n\n\n\nline3"
	result := collapseBlankLines(input)
	expected := "line1\n\nline2\n\nline3"
	if result != expected {
		t.Errorf("collapseBlankLines = %q, want %q", result, expected)
	}
}

func TestMergeCompactSummariesEmpty(t *testing.T) {
	if MergeCompactSummaries("", "current") != "current" {
		t.Error("empty previous should return current")
	}
	if MergeCompactSummaries("previous", "") != "previous" {
		t.Error("empty current should return previous")
	}
	if MergeCompactSummaries("", "") != "" {
		t.Error("both empty should return empty")
	}
}

func TestRepeatedCompactionPreservesContext(t *testing.T) {
	// Simulate two rounds of compaction to verify the merged summary used
	// in the continuation message includes all prior compacted context.
	firstSummary := "<summary>Conversation summary:\n- Scope: 3 messages.\n- Key timeline:\n  - user: initial request</summary>"
	secondSummary := "<summary>Conversation summary:\n- Scope: 2 messages.\n- Key timeline:\n  - user: follow-up</summary>"

	// First merge
	merged1 := MergeCompactSummaries("", firstSummary)
	if merged1 != firstSummary {
		t.Error("first merge with empty previous should return current")
	}

	// Second merge uses first result as previous
	merged2 := MergeCompactSummaries(merged1, secondSummary)

	if !strings.Contains(merged2, "Previously compacted context:") {
		t.Error("second merge should contain previously compacted context")
	}
	if !strings.Contains(merged2, "Newly compacted context:") {
		t.Error("second merge should contain newly compacted context")
	}
	// Verify first summary content is in the "previously compacted" section
	if !strings.Contains(merged2, "Scope: 3 messages") {
		t.Error("first summary scope should be preserved in previously compacted context")
	}
	// Verify second summary content is in the "newly compacted" section
	if !strings.Contains(merged2, "Scope: 2 messages") {
		t.Error("second summary scope should appear in newly compacted context")
	}
}
