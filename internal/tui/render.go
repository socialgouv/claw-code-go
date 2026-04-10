package tui

import (
	"fmt"
	"strings"
	"unicode"

	"github.com/charmbracelet/lipgloss"
)

// ColorTheme holds colors for terminal markdown rendering.
type ColorTheme struct {
	Heading         lipgloss.AdaptiveColor
	Emphasis        lipgloss.AdaptiveColor
	Strong          lipgloss.AdaptiveColor
	InlineCode      lipgloss.AdaptiveColor
	Link            lipgloss.AdaptiveColor
	Quote           lipgloss.AdaptiveColor
	TableBorder     lipgloss.AdaptiveColor
	CodeBlockBorder lipgloss.AdaptiveColor
	SpinnerActive   lipgloss.AdaptiveColor
	SpinnerDone     lipgloss.AdaptiveColor
	SpinnerFailed   lipgloss.AdaptiveColor
}

// DefaultColorTheme returns the default markdown color theme.
func DefaultColorTheme() ColorTheme {
	return ColorTheme{
		Heading:         lipgloss.AdaptiveColor{Light: "6", Dark: "6"},   // cyan
		Emphasis:        lipgloss.AdaptiveColor{Light: "5", Dark: "5"},   // magenta
		Strong:          lipgloss.AdaptiveColor{Light: "3", Dark: "3"},   // yellow
		InlineCode:      lipgloss.AdaptiveColor{Light: "2", Dark: "2"},   // green
		Link:            lipgloss.AdaptiveColor{Light: "4", Dark: "4"},   // blue
		Quote:           lipgloss.AdaptiveColor{Light: "8", Dark: "8"},   // dark grey
		TableBorder:     lipgloss.AdaptiveColor{Light: "14", Dark: "14"}, // dark cyan
		CodeBlockBorder: lipgloss.AdaptiveColor{Light: "8", Dark: "8"},   // dark grey
		SpinnerActive:   lipgloss.AdaptiveColor{Light: "4", Dark: "4"},   // blue
		SpinnerDone:     lipgloss.AdaptiveColor{Light: "2", Dark: "2"},   // green
		SpinnerFailed:   lipgloss.AdaptiveColor{Light: "1", Dark: "1"},   // red
	}
}

// Renderer is the interface for markdown rendering backends.
// This allows Bubble Tea, lipgloss, and plain output to coexist
// without duplicating markdown parsing logic.
type Renderer interface {
	// RenderMarkdown converts markdown text to styled terminal output.
	RenderMarkdown(markdown string) string
}

// MarkdownRenderer renders markdown text with ANSI terminal styling.
// It uses lipgloss for styling (GO_ONLY) rather than syntect/chroma.
// Implements the Renderer interface.
type MarkdownRenderer struct {
	theme ColorTheme
}

// Ensure MarkdownRenderer implements Renderer at compile time.
var _ Renderer = (*MarkdownRenderer)(nil)

// NewMarkdownRenderer creates a renderer with the default color theme.
func NewMarkdownRenderer() *MarkdownRenderer {
	return &MarkdownRenderer{theme: DefaultColorTheme()}
}

// NewMarkdownRendererWithTheme creates a renderer with a custom color theme.
func NewMarkdownRendererWithTheme(theme ColorTheme) *MarkdownRenderer {
	return &MarkdownRenderer{theme: theme}
}

// RenderMarkdown converts markdown text to ANSI-styled terminal output.
func (r *MarkdownRenderer) RenderMarkdown(markdown string) string {
	var output strings.Builder
	lines := strings.Split(markdown, "\n")
	inCodeBlock := false
	codeLanguage := ""
	var codeBuffer strings.Builder
	inList := false

	headingStyle := lipgloss.NewStyle().
		Foreground(r.theme.Heading).
		Bold(true)
	strongStyle := lipgloss.NewStyle().
		Foreground(r.theme.Strong).
		Bold(true)
	emphasisStyle := lipgloss.NewStyle().
		Foreground(r.theme.Emphasis).
		Italic(true)
	inlineCodeStyle := lipgloss.NewStyle().
		Foreground(r.theme.InlineCode)
	quoteStyle := lipgloss.NewStyle().
		Foreground(r.theme.Quote)
	codeBlockBorderStyle := lipgloss.NewStyle().
		Foreground(r.theme.CodeBlockBorder).
		Bold(true)

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Code block fence handling
		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			if inCodeBlock {
				// End code block: render buffered code
				output.WriteString(codeBuffer.String())
				output.WriteString(codeBlockBorderStyle.Render("╰─"))
				output.WriteString("\n\n")
				codeBuffer.Reset()
				inCodeBlock = false
				codeLanguage = ""
				continue
			}
			// Start code block
			inCodeBlock = true
			fenceChar := trimmed[:1]
			fenceLen := 0
			for _, c := range trimmed {
				if string(c) == fenceChar {
					fenceLen++
				} else {
					break
				}
			}
			codeLanguage = strings.TrimSpace(trimmed[fenceLen:])
			label := codeLanguage
			if label == "" {
				label = "code"
			}
			output.WriteString(codeBlockBorderStyle.Render(fmt.Sprintf("╭─ %s", label)))
			output.WriteString("\n")
			continue
		}

		if inCodeBlock {
			codeBuffer.WriteString(line)
			codeBuffer.WriteString("\n")
			continue
		}

		// Headings
		if strings.HasPrefix(trimmed, "# ") {
			if output.Len() > 0 {
				output.WriteString("\n")
			}
			output.WriteString(headingStyle.Render(trimmed[2:]))
			output.WriteString("\n\n")
			continue
		}
		if strings.HasPrefix(trimmed, "## ") {
			if output.Len() > 0 {
				output.WriteString("\n")
			}
			h2Style := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("15"))
			output.WriteString(h2Style.Render(trimmed[3:]))
			output.WriteString("\n\n")
			continue
		}
		if strings.HasPrefix(trimmed, "### ") {
			if output.Len() > 0 {
				output.WriteString("\n")
			}
			h3Style := lipgloss.NewStyle().Foreground(lipgloss.Color("4"))
			output.WriteString(h3Style.Render(trimmed[4:]))
			output.WriteString("\n\n")
			continue
		}

		// Horizontal rule
		if trimmed == "---" || trimmed == "***" || trimmed == "___" {
			output.WriteString("---\n")
			continue
		}

		// Block quote
		if strings.HasPrefix(trimmed, "> ") {
			output.WriteString(quoteStyle.Render("│ " + trimmed[2:]))
			output.WriteString("\n")
			continue
		}

		// Unordered list
		if strings.HasPrefix(trimmed, "- ") || strings.HasPrefix(trimmed, "* ") {
			indent := len(line) - len(strings.TrimLeft(line, " \t"))
			depth := indent / 2
			output.WriteString(strings.Repeat("  ", depth))
			output.WriteString("• ")
			output.WriteString(renderInlineMarkdown(trimmed[2:], strongStyle, emphasisStyle, inlineCodeStyle))
			output.WriteString("\n")
			inList = true
			continue
		}

		// Ordered list
		if isOrderedListItem(trimmed) {
			dotIdx := strings.IndexByte(trimmed, '.')
			indent := len(line) - len(strings.TrimLeft(line, " \t"))
			depth := indent / 2
			output.WriteString(strings.Repeat("  ", depth))
			output.WriteString(trimmed[:dotIdx+1])
			output.WriteString(" ")
			output.WriteString(renderInlineMarkdown(strings.TrimSpace(trimmed[dotIdx+1:]), strongStyle, emphasisStyle, inlineCodeStyle))
			output.WriteString("\n")
			inList = true
			continue
		}

		// Empty line
		if trimmed == "" {
			if inList {
				inList = false
				output.WriteString("\n")
			} else if i > 0 {
				output.WriteString("\n")
			}
			continue
		}

		// Normal paragraph text with inline formatting
		output.WriteString(renderInlineMarkdown(trimmed, strongStyle, emphasisStyle, inlineCodeStyle))
		output.WriteString("\n")
	}

	return strings.TrimRight(output.String(), "\n")
}

// MarkdownToANSI is an alias for RenderMarkdown.
func (r *MarkdownRenderer) MarkdownToANSI(markdown string) string {
	return r.RenderMarkdown(markdown)
}

// renderInlineMarkdown applies inline formatting: **bold**, *italic*, `code`.
func renderInlineMarkdown(text string, strong, emphasis, code lipgloss.Style) string {
	var result strings.Builder
	runes := []rune(text)
	i := 0

	for i < len(runes) {
		// Bold: **text**
		if i+1 < len(runes) && runes[i] == '*' && runes[i+1] == '*' {
			end := findClosingDouble(runes, i+2, '*')
			if end > 0 {
				inner := string(runes[i+2 : end])
				result.WriteString(strong.Render(inner))
				i = end + 2
				continue
			}
		}

		// Italic: *text*
		if runes[i] == '*' && (i == 0 || runes[i-1] != '*') {
			end := findClosingSingle(runes, i+1, '*')
			if end > 0 && (end+1 >= len(runes) || runes[end+1] != '*') {
				inner := string(runes[i+1 : end])
				result.WriteString(emphasis.Render(inner))
				i = end + 1
				continue
			}
		}

		// Inline code: `code`
		if runes[i] == '`' {
			end := findClosingSingle(runes, i+1, '`')
			if end > 0 {
				inner := string(runes[i+1 : end])
				result.WriteString(code.Render("`" + inner + "`"))
				i = end + 1
				continue
			}
		}

		result.WriteRune(runes[i])
		i++
	}

	return result.String()
}

func findClosingDouble(runes []rune, start int, ch rune) int {
	for i := start; i+1 < len(runes); i++ {
		if runes[i] == ch && runes[i+1] == ch {
			return i
		}
	}
	return -1
}

func findClosingSingle(runes []rune, start int, ch rune) int {
	for i := start; i < len(runes); i++ {
		if runes[i] == ch {
			return i
		}
	}
	return -1
}

func isOrderedListItem(line string) bool {
	for i, c := range line {
		if c == '.' && i > 0 {
			rest := strings.TrimSpace(line[i+1:])
			return len(rest) > 0
		}
		if !unicode.IsDigit(c) {
			return false
		}
	}
	return false
}

// MarkdownStreamState tracks the state of incrementally rendered markdown
// across SSE delta chunks. It buffers content until a safe boundary is found.
type MarkdownStreamState struct {
	pending string
}

// NewMarkdownStreamState creates a new streaming state.
func NewMarkdownStreamState() *MarkdownStreamState {
	return &MarkdownStreamState{}
}

// Push appends a delta chunk and returns rendered markdown if a safe
// boundary (end of paragraph or closed code fence) was found.
func (s *MarkdownStreamState) Push(renderer *MarkdownRenderer, delta string) (string, bool) {
	s.pending += delta
	split := findStreamSafeBoundary(s.pending)
	if split < 0 {
		return "", false
	}
	ready := s.pending[:split]
	s.pending = s.pending[split:]
	return renderer.MarkdownToANSI(ready), true
}

// Flush renders and returns any remaining buffered content.
func (s *MarkdownStreamState) Flush(renderer *MarkdownRenderer) (string, bool) {
	if strings.TrimSpace(s.pending) == "" {
		s.pending = ""
		return "", false
	}
	pending := s.pending
	s.pending = ""
	return renderer.MarkdownToANSI(pending), true
}

// findStreamSafeBoundary finds the latest position where it's safe to split
// the markdown for incremental rendering (outside code fences, at blank lines).
func findStreamSafeBoundary(markdown string) int {
	var openFence *fenceMarker
	lastBoundary := -1
	offset := 0

	for _, line := range strings.SplitAfter(markdown, "\n") {
		lineWithout := strings.TrimRight(line, "\n")

		if openFence != nil {
			if lineClosesFence(lineWithout, openFence) {
				openFence = nil
				lastBoundary = offset + len(line)
			}
			offset += len(line)
			continue
		}

		if opener := parseFenceOpener(lineWithout); opener != nil {
			openFence = opener
			offset += len(line)
			continue
		}

		if strings.TrimSpace(lineWithout) == "" {
			lastBoundary = offset + len(line)
		}

		offset += len(line)
	}

	return lastBoundary
}

type fenceMarker struct {
	character rune
	length    int
}

func parseFenceOpener(line string) *fenceMarker {
	indent := 0
	for _, c := range line {
		if c == ' ' {
			indent++
		} else {
			break
		}
	}
	if indent > 3 {
		return nil
	}
	rest := line[indent:]
	if len(rest) == 0 {
		return nil
	}
	ch := rune(rest[0])
	if ch != '`' && ch != '~' {
		return nil
	}
	length := 0
	for _, c := range rest {
		if c == ch {
			length++
		} else {
			break
		}
	}
	if length < 3 {
		return nil
	}
	infoString := rest[length:]
	if ch == '`' && strings.Contains(infoString, "`") {
		return nil
	}
	return &fenceMarker{character: ch, length: length}
}

func lineClosesFence(line string, opener *fenceMarker) bool {
	indent := 0
	for _, c := range line {
		if c == ' ' {
			indent++
		} else {
			break
		}
	}
	if indent > 3 {
		return false
	}
	rest := line[indent:]
	length := 0
	for _, c := range rest {
		if c == opener.character {
			length++
		} else {
			break
		}
	}
	if length < opener.length {
		return false
	}
	trailing := rest[length:]
	for _, c := range trailing {
		if c != ' ' && c != '\t' {
			return false
		}
	}
	return true
}
