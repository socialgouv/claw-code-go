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
