package tui

import (
	"strings"
	"testing"
)

func TestSyntaxHighlightGoKeywords(t *testing.T) {
	t.Parallel()
	h := NewSyntaxHighlighter()
	code := `func main() {
	if x := 0; x < 10 {
		return
	}
}`
	result := h.Highlight(code, "go")
	// In a non-TTY test env, lipgloss may not emit ANSI escapes.
	// Verify keywords are present and the output is not empty.
	if result == "" {
		t.Error("expected non-empty highlighted output")
	}
	plain := stripANSI(result)
	if !strings.Contains(plain, "func") {
		t.Error("expected 'func' keyword in output")
	}
	if !strings.Contains(plain, "return") {
		t.Error("expected 'return' keyword in output")
	}
	if !strings.Contains(plain, "main()") {
		t.Error("expected 'main()' function call in output")
	}
}

func TestSyntaxHighlightRust(t *testing.T) {
	t.Parallel()
	h := NewSyntaxHighlighter()
	code := `fn greet(name: &str) -> String {
	let msg = format!("hello {}", name);
	msg
}`
	result := h.Highlight(code, "rust")
	plain := stripANSI(result)
	if !strings.Contains(plain, "fn") {
		t.Error("expected 'fn' keyword")
	}
	if !strings.Contains(plain, "let") {
		t.Error("expected 'let' keyword")
	}
	if !strings.Contains(plain, "String") {
		t.Error("expected 'String' type")
	}
}

func TestSyntaxHighlightPython(t *testing.T) {
	t.Parallel()
	h := NewSyntaxHighlighter()
	code := `def hello():
    if True:
        return "world"
`
	result := h.Highlight(code, "python")
	plain := stripANSI(result)
	if !strings.Contains(plain, "def") {
		t.Error("expected 'def' keyword")
	}
	if !strings.Contains(plain, "return") {
		t.Error("expected 'return' keyword")
	}
}

func TestSyntaxHighlightUnknownLanguage(t *testing.T) {
	t.Parallel()
	h := NewSyntaxHighlighter()
	code := "some random code here"
	result := h.Highlight(code, "brainfuck")
	// Unknown language should return code unchanged.
	if result != code {
		t.Errorf("unknown language should return unchanged code, got: %q", result)
	}
}

func TestSyntaxHighlightEmpty(t *testing.T) {
	t.Parallel()
	h := NewSyntaxHighlighter()
	result := h.Highlight("", "go")
	if result != "" {
		t.Errorf("empty input should produce empty output, got: %q", result)
	}
}

func TestSyntaxHighlightStrings(t *testing.T) {
	t.Parallel()
	h := NewSyntaxHighlighter()
	code := `x := "hello world"`
	result := h.Highlight(code, "go")
	plain := stripANSI(result)
	if !strings.Contains(plain, `"hello world"`) {
		t.Errorf("expected string literal preserved, got: %s", plain)
	}
}

func TestSyntaxHighlightComments(t *testing.T) {
	t.Parallel()
	h := NewSyntaxHighlighter()
	code := `// this is a comment
func main() {}`
	result := h.Highlight(code, "go")
	plain := stripANSI(result)
	if !strings.Contains(plain, "// this is a comment") {
		t.Errorf("expected comment preserved, got: %s", plain)
	}
}

func TestSyntaxHighlightPythonComment(t *testing.T) {
	t.Parallel()
	h := NewSyntaxHighlighter()
	code := `# comment
x = 42`
	result := h.Highlight(code, "python")
	plain := stripANSI(result)
	if !strings.Contains(plain, "# comment") {
		t.Errorf("expected Python comment preserved, got: %s", plain)
	}
}

func TestSyntaxHighlightNumbers(t *testing.T) {
	t.Parallel()
	h := NewSyntaxHighlighter()
	code := `x := 42
y := 3.14`
	result := h.Highlight(code, "go")
	plain := stripANSI(result)
	if !strings.Contains(plain, "42") {
		t.Error("expected number 42")
	}
	if !strings.Contains(plain, "3.14") {
		t.Error("expected number 3.14")
	}
}

func TestNormalizeLanguage(t *testing.T) {
	t.Parallel()
	cases := []struct {
		input, want string
	}{
		{"go", "go"},
		{"golang", "go"},
		{"Go", "go"},
		{"GOLANG", "go"},
		{"rust", "rust"},
		{"rs", "rust"},
		{"python", "python"},
		{"py", "python"},
		{"python3", "python"},
		{"javascript", "javascript"},
		{"js", "javascript"},
		{"jsx", "javascript"},
		{"typescript", "typescript"},
		{"ts", "typescript"},
		{"tsx", "typescript"},
		{"bash", "bash"},
		{"sh", "bash"},
		{"shell", "bash"},
		{"zsh", "bash"},
		{"ruby", "ruby"},
		{"rb", "ruby"},
		{"java", "java"},
		{"c", "c"},
		{"cpp", "cpp"},
		{"c++", "cpp"},
		{"json", "json"},
		{"yaml", "yaml"},
		{"yml", "yaml"},
		{"sql", "sql"},
		{"unknown", "unknown"},
	}
	for _, tc := range cases {
		got := normalizeLanguage(tc.input)
		if got != tc.want {
			t.Errorf("normalizeLanguage(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestSyntaxHighlightBash(t *testing.T) {
	t.Parallel()
	h := NewSyntaxHighlighter()
	code := `#!/bin/bash
if [ -f "$1" ]; then
    echo "file exists"
fi`
	result := h.Highlight(code, "bash")
	plain := stripANSI(result)
	if !strings.Contains(plain, "if") {
		t.Error("expected 'if' keyword")
	}
	if !strings.Contains(plain, "then") {
		t.Error("expected 'then' keyword")
	}
	if !strings.Contains(plain, "fi") {
		t.Error("expected 'fi' keyword")
	}
}

func TestSyntaxHighlightJavaScript(t *testing.T) {
	t.Parallel()
	h := NewSyntaxHighlighter()
	code := `const greet = async (name) => {
	return await fetch(name);
};`
	result := h.Highlight(code, "js")
	plain := stripANSI(result)
	if !strings.Contains(plain, "const") {
		t.Error("expected 'const' keyword")
	}
	if !strings.Contains(plain, "async") {
		t.Error("expected 'async' keyword")
	}
	if !strings.Contains(plain, "await") {
		t.Error("expected 'await' keyword")
	}
}

func TestSyntaxHighlightTypeScript(t *testing.T) {
	t.Parallel()
	h := NewSyntaxHighlighter()
	code := `interface User {
	name: string;
	age: number;
}`
	result := h.Highlight(code, "ts")
	plain := stripANSI(result)
	if !strings.Contains(plain, "interface") {
		t.Error("expected 'interface' keyword")
	}
}

func TestSyntaxHighlightWithBlockComment(t *testing.T) {
	t.Parallel()
	h := NewSyntaxHighlighter()
	code := `/* block comment
   spanning lines */
func main() {}`
	result := h.Highlight(code, "go")
	plain := stripANSI(result)
	if !strings.Contains(plain, "block comment") {
		t.Error("expected block comment text preserved")
	}
	if !strings.Contains(plain, "func") {
		t.Error("expected 'func' keyword after block comment")
	}
}

func TestMarkdownRendererWithSyntaxHighlighting(t *testing.T) {
	t.Parallel()
	r := NewMarkdownRenderer()
	md := "```go\nfunc main() {\n\treturn\n}\n```"
	output := r.RenderMarkdown(md)
	plain := stripANSI(output)
	if !strings.Contains(plain, "╭─ go") {
		t.Errorf("expected code block header, got: %s", plain)
	}
	if !strings.Contains(plain, "func") {
		t.Error("expected 'func' in highlighted output")
	}
	if !strings.Contains(plain, "return") {
		t.Error("expected 'return' in highlighted output")
	}
	if !strings.Contains(plain, "╰─") {
		t.Error("expected closing fence")
	}
}
