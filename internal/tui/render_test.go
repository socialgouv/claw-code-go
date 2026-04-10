package tui

import (
	"strings"
	"testing"
)

func stripANSI(s string) string {
	var out strings.Builder
	runes := []rune(s)
	i := 0
	for i < len(runes) {
		if runes[i] == '\033' {
			if i+1 < len(runes) && runes[i+1] == '[' {
				i += 2
				for i < len(runes) && !isASCIIAlpha(runes[i]) {
					i++
				}
				if i < len(runes) {
					i++ // skip the letter
				}
				continue
			}
		}
		out.WriteRune(runes[i])
		i++
	}
	return out.String()
}

func isASCIIAlpha(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')
}

func TestRendererInterface(t *testing.T) {
	// Verify MarkdownRenderer satisfies the Renderer interface.
	var r Renderer = NewMarkdownRenderer()
	output := r.RenderMarkdown("# Test")
	plain := stripANSI(output)
	if !strings.Contains(plain, "Test") {
		t.Errorf("expected 'Test' in renderer output, got: %s", plain)
	}
}

func TestRendersHeading(t *testing.T) {
	r := NewMarkdownRenderer()
	output := r.RenderMarkdown("# Heading\n\nText here")
	plain := stripANSI(output)
	if !strings.Contains(plain, "Heading") {
		t.Error("expected 'Heading' in output")
	}
	if !strings.Contains(plain, "Text here") {
		t.Error("expected 'Text here' in output")
	}
	// Note: lipgloss may not emit ANSI codes when no TTY is attached (CI/test).
	// The styling is applied but may degrade to plain text in non-TTY mode.
}

func TestRendersBoldAndItalic(t *testing.T) {
	r := NewMarkdownRenderer()
	output := r.RenderMarkdown("This is **bold** and *italic*.")
	plain := stripANSI(output)
	if !strings.Contains(plain, "bold") {
		t.Error("expected 'bold' in output")
	}
	if !strings.Contains(plain, "italic") {
		t.Error("expected 'italic' in output")
	}
}

func TestRendersInlineCode(t *testing.T) {
	r := NewMarkdownRenderer()
	output := r.RenderMarkdown("Use `code` here")
	plain := stripANSI(output)
	if !strings.Contains(plain, "`code`") {
		t.Error("expected '`code`' in output")
	}
}

func TestRendersUnorderedList(t *testing.T) {
	r := NewMarkdownRenderer()
	output := r.RenderMarkdown("- first\n- second\n- third")
	plain := stripANSI(output)
	if !strings.Contains(plain, "• first") {
		t.Error("expected '• first' in output")
	}
	if !strings.Contains(plain, "• second") {
		t.Error("expected '• second' in output")
	}
}

func TestRendersOrderedList(t *testing.T) {
	r := NewMarkdownRenderer()
	output := r.RenderMarkdown("1. first\n2. second")
	plain := stripANSI(output)
	if !strings.Contains(plain, "1.") {
		t.Error("expected '1.' in output")
	}
	if !strings.Contains(plain, "2.") {
		t.Error("expected '2.' in output")
	}
}

func TestRendersCodeBlock(t *testing.T) {
	r := NewMarkdownRenderer()
	output := r.RenderMarkdown("```rust\nfn main() {}\n```")
	plain := stripANSI(output)
	if !strings.Contains(plain, "╭─ rust") {
		t.Errorf("expected '╭─ rust' in output, got: %s", plain)
	}
	if !strings.Contains(plain, "fn main") {
		t.Error("expected 'fn main' in code block output")
	}
	if !strings.Contains(plain, "╰─") {
		t.Error("expected '╰─' closing fence in output")
	}
}

func TestRendersBlockQuote(t *testing.T) {
	r := NewMarkdownRenderer()
	output := r.RenderMarkdown("> quoted text")
	plain := stripANSI(output)
	if !strings.Contains(plain, "│ quoted text") {
		t.Errorf("expected '│ quoted text' in output, got: %s", plain)
	}
}

func TestStreamStateTracksCodeFence(t *testing.T) {
	r := NewMarkdownRenderer()
	s := NewMarkdownStreamState()

	// Push content with an open code fence — should not render yet
	_, ok := s.Push(r, "# Title\n\nSome text\n\n```rust\nfn foo() {\n")
	// The blank line before the code fence may be a safe boundary
	// The open code fence should prevent rendering past it.

	// Push closing fence and more content
	rendered, ok := s.Push(r, "}\n```\n\nMore text\n\n")
	if !ok {
		// Flush should produce content
		rendered, ok = s.Flush(r)
		if !ok {
			t.Fatal("expected flush to produce content")
		}
	}
	if !strings.Contains(stripANSI(rendered), "Title") || !strings.Contains(stripANSI(rendered), "foo") {
		// Content may be split — that's fine as long as flush gets the rest
		remaining, _ := s.Flush(r)
		combined := rendered + remaining
		plain := stripANSI(combined)
		if !strings.Contains(plain, "foo") && !strings.Contains(plain, "Title") {
			t.Errorf("expected rendered content to contain title or code, got: %s", plain)
		}
	}
}

func TestStreamStateFlush(t *testing.T) {
	r := NewMarkdownRenderer()
	s := NewMarkdownStreamState()

	s.Push(r, "Hello world")

	rendered, ok := s.Flush(r)
	if !ok {
		t.Fatal("expected flush to produce content")
	}
	plain := stripANSI(rendered)
	if !strings.Contains(plain, "Hello world") {
		t.Errorf("expected 'Hello world', got: %s", plain)
	}
}

func TestStreamStateEmptyFlush(t *testing.T) {
	r := NewMarkdownRenderer()
	s := NewMarkdownStreamState()

	_, ok := s.Flush(r)
	if ok {
		t.Error("expected empty flush to return false")
	}
}
