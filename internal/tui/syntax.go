package tui

import (
	"strings"
	"sync"
	"unicode"

	"github.com/charmbracelet/lipgloss"
)

// SyntaxTheme holds colors for language-aware code highlighting.
// This is a Go-only feature replacing Rust's syntect integration,
// implemented without external deps (chroma/v2 would add ~3MB).
type SyntaxTheme struct {
	Keyword  lipgloss.AdaptiveColor
	String   lipgloss.AdaptiveColor
	Comment  lipgloss.AdaptiveColor
	Number   lipgloss.AdaptiveColor
	Type     lipgloss.AdaptiveColor
	Function lipgloss.AdaptiveColor
	Operator lipgloss.AdaptiveColor
}

// DefaultSyntaxTheme returns a terminal-friendly syntax theme.
func DefaultSyntaxTheme() SyntaxTheme {
	return SyntaxTheme{
		Keyword:  lipgloss.AdaptiveColor{Light: "5", Dark: "5"}, // magenta
		String:   lipgloss.AdaptiveColor{Light: "2", Dark: "2"}, // green
		Comment:  lipgloss.AdaptiveColor{Light: "8", Dark: "8"}, // dark grey
		Number:   lipgloss.AdaptiveColor{Light: "3", Dark: "3"}, // yellow
		Type:     lipgloss.AdaptiveColor{Light: "6", Dark: "6"}, // cyan
		Function: lipgloss.AdaptiveColor{Light: "4", Dark: "4"}, // blue
		Operator: lipgloss.AdaptiveColor{Light: "1", Dark: "1"}, // red
	}
}

// SyntaxHighlighter provides language-aware code block colorization.
type SyntaxHighlighter struct {
	theme SyntaxTheme
	// Cached styles (computed once per highlighter instance).
	once          sync.Once
	keywordStyle  lipgloss.Style
	stringStyle   lipgloss.Style
	commentStyle  lipgloss.Style
	numberStyle   lipgloss.Style
	typeStyle     lipgloss.Style
	functionStyle lipgloss.Style
	operatorStyle lipgloss.Style
}

// NewSyntaxHighlighter creates a highlighter with the default theme.
func NewSyntaxHighlighter() *SyntaxHighlighter {
	return &SyntaxHighlighter{theme: DefaultSyntaxTheme()}
}

// NewSyntaxHighlighterWithTheme creates a highlighter with a custom theme.
func NewSyntaxHighlighterWithTheme(theme SyntaxTheme) *SyntaxHighlighter {
	return &SyntaxHighlighter{theme: theme}
}

func (h *SyntaxHighlighter) init() {
	h.once.Do(func() {
		h.keywordStyle = lipgloss.NewStyle().Foreground(h.theme.Keyword).Bold(true)
		h.stringStyle = lipgloss.NewStyle().Foreground(h.theme.String)
		h.commentStyle = lipgloss.NewStyle().Foreground(h.theme.Comment).Italic(true)
		h.numberStyle = lipgloss.NewStyle().Foreground(h.theme.Number)
		h.typeStyle = lipgloss.NewStyle().Foreground(h.theme.Type)
		h.functionStyle = lipgloss.NewStyle().Foreground(h.theme.Function)
		h.operatorStyle = lipgloss.NewStyle().Foreground(h.theme.Operator)
	})
}

// Highlight applies syntax highlighting to a code block.
// language is the fence info string (e.g. "go", "rust", "python").
// Returns the highlighted text or the original if the language is unknown.
func (h *SyntaxHighlighter) Highlight(code, language string) string {
	h.init()
	lang := normalizeLanguage(language)
	kw := languageKeywords[lang]
	if kw == nil {
		// Unknown language — return code unchanged.
		return code
	}

	types := languageTypes[lang]
	lineComment, blockStart, blockEnd := commentSyntax(lang)

	var result strings.Builder
	lines := strings.Split(code, "\n")
	inBlockComment := false

	for lineIdx, line := range lines {
		if lineIdx > 0 {
			result.WriteRune('\n')
		}

		if inBlockComment {
			endIdx := strings.Index(line, blockEnd)
			if endIdx >= 0 {
				result.WriteString(h.commentStyle.Render(line[:endIdx+len(blockEnd)]))
				inBlockComment = false
				rest := line[endIdx+len(blockEnd):]
				result.WriteString(h.highlightLine(rest, kw, types, lineComment, blockStart, &inBlockComment))
			} else {
				result.WriteString(h.commentStyle.Render(line))
			}
			continue
		}

		result.WriteString(h.highlightLine(line, kw, types, lineComment, blockStart, &inBlockComment))
	}

	return result.String()
}

func (h *SyntaxHighlighter) highlightLine(
	line string,
	keywords map[string]bool,
	types map[string]bool,
	lineComment, blockStart string,
	inBlockComment *bool,
) string {
	var result strings.Builder
	runes := []rune(line)
	lineCommentRunes := []rune(lineComment)
	blockStartRunes := []rune(blockStart)
	i := 0

	for i < len(runes) {
		// Line comment.
		if len(lineCommentRunes) > 0 && i+len(lineCommentRunes) <= len(runes) {
			if string(runes[i:i+len(lineCommentRunes)]) == lineComment {
				result.WriteString(h.commentStyle.Render(string(runes[i:])))
				return result.String()
			}
		}

		// Block comment start.
		if len(blockStartRunes) > 0 && i+len(blockStartRunes) <= len(runes) {
			if string(runes[i:i+len(blockStartRunes)]) == blockStart {
				*inBlockComment = true
				result.WriteString(h.commentStyle.Render(string(runes[i:])))
				return result.String()
			}
		}

		// String literals.
		if runes[i] == '"' || runes[i] == '\'' || runes[i] == '`' {
			quote := runes[i]
			end := findStringEnd(runes, i+1, quote)
			if end > i {
				result.WriteString(h.stringStyle.Render(string(runes[i : end+1])))
				i = end + 1
				continue
			}
		}

		// Numbers.
		if unicode.IsDigit(runes[i]) || (runes[i] == '.' && i+1 < len(runes) && unicode.IsDigit(runes[i+1])) {
			end := i + 1
			for end < len(runes) && (unicode.IsDigit(runes[end]) || runes[end] == '.' || runes[end] == 'x' || runes[end] == 'X' ||
				(runes[end] >= 'a' && runes[end] <= 'f') || (runes[end] >= 'A' && runes[end] <= 'F') || runes[end] == '_') {
				end++
			}
			result.WriteString(h.numberStyle.Render(string(runes[i:end])))
			i = end
			continue
		}

		// Identifiers (keywords, types, functions).
		if unicode.IsLetter(runes[i]) || runes[i] == '_' {
			end := i + 1
			for end < len(runes) && (unicode.IsLetter(runes[end]) || unicode.IsDigit(runes[end]) || runes[end] == '_') {
				end++
			}
			word := string(runes[i:end])
			// Check if followed by '(' — it's a function call.
			isFunc := end < len(runes) && runes[end] == '('
			if keywords[word] {
				result.WriteString(h.keywordStyle.Render(word))
			} else if types != nil && types[word] {
				result.WriteString(h.typeStyle.Render(word))
			} else if isFunc {
				result.WriteString(h.functionStyle.Render(word))
			} else {
				result.WriteString(word)
			}
			i = end
			continue
		}

		// Operators.
		if isOperatorRune(runes[i]) {
			result.WriteString(h.operatorStyle.Render(string(runes[i])))
			i++
			continue
		}

		result.WriteRune(runes[i])
		i++
	}

	return result.String()
}

func findStringEnd(runes []rune, start int, quote rune) int {
	for i := start; i < len(runes); i++ {
		if runes[i] == '\\' {
			i++ // skip escaped char
			continue
		}
		if runes[i] == quote {
			return i
		}
	}
	return -1
}

func isOperatorRune(r rune) bool {
	return r == '=' || r == '+' || r == '-' || r == '*' || r == '/' ||
		r == '<' || r == '>' || r == '!' || r == '&' || r == '|' ||
		r == '^' || r == '%' || r == '~'
}

// normalizeLanguage maps common language aliases to canonical names.
func normalizeLanguage(lang string) string {
	l := strings.ToLower(strings.TrimSpace(lang))
	switch l {
	case "go", "golang":
		return "go"
	case "rust", "rs":
		return "rust"
	case "python", "py", "python3":
		return "python"
	case "javascript", "js", "jsx":
		return "javascript"
	case "typescript", "ts", "tsx":
		return "typescript"
	case "bash", "sh", "shell", "zsh":
		return "bash"
	case "ruby", "rb":
		return "ruby"
	case "java":
		return "java"
	case "c":
		return "c"
	case "cpp", "c++", "cxx":
		return "cpp"
	case "json":
		return "json"
	case "yaml", "yml":
		return "yaml"
	case "sql":
		return "sql"
	default:
		return l
	}
}

func commentSyntax(lang string) (lineComment, blockStart, blockEnd string) {
	switch lang {
	case "go", "rust", "javascript", "typescript", "java", "c", "cpp":
		return "//", "/*", "*/"
	case "python", "ruby", "bash", "yaml":
		return "#", "", ""
	case "sql":
		return "--", "/*", "*/"
	default:
		return "//", "/*", "*/"
	}
}

// Language keyword and type tables. Kept minimal — common keywords only.
// Using sync.Map would be overkill since these are read-only after init.

var languageKeywords = map[string]map[string]bool{
	"go": toSet("break", "case", "chan", "const", "continue", "default", "defer",
		"else", "fallthrough", "for", "func", "go", "goto", "if", "import",
		"interface", "map", "package", "range", "return", "select", "struct",
		"switch", "type", "var"),

	"rust": toSet("as", "async", "await", "break", "const", "continue", "crate",
		"dyn", "else", "enum", "extern", "false", "fn", "for", "if", "impl",
		"in", "let", "loop", "match", "mod", "move", "mut", "pub", "ref",
		"return", "self", "static", "struct", "super", "trait", "true", "type",
		"unsafe", "use", "where", "while"),

	"python": toSet("and", "as", "assert", "async", "await", "break", "class",
		"continue", "def", "del", "elif", "else", "except", "False", "finally",
		"for", "from", "global", "if", "import", "in", "is", "lambda", "None",
		"nonlocal", "not", "or", "pass", "raise", "return", "True", "try",
		"while", "with", "yield"),

	"javascript": toSet("async", "await", "break", "case", "catch", "class",
		"const", "continue", "debugger", "default", "delete", "do", "else",
		"export", "extends", "false", "finally", "for", "function", "if",
		"import", "in", "instanceof", "let", "new", "null", "of", "return",
		"static", "super", "switch", "this", "throw", "true", "try", "typeof",
		"undefined", "var", "void", "while", "with", "yield"),

	"typescript": toSet("abstract", "any", "as", "async", "await", "boolean",
		"break", "case", "catch", "class", "const", "continue", "debugger",
		"declare", "default", "delete", "do", "else", "enum", "export",
		"extends", "false", "finally", "for", "from", "function", "if",
		"implements", "import", "in", "instanceof", "interface", "is", "keyof",
		"let", "module", "namespace", "never", "new", "null", "number", "of",
		"package", "private", "protected", "public", "readonly", "return",
		"static", "string", "super", "switch", "symbol", "this", "throw",
		"true", "try", "type", "typeof", "undefined", "unique", "unknown",
		"var", "void", "while", "with", "yield"),

	"bash": toSet("case", "do", "done", "elif", "else", "esac", "fi", "for",
		"function", "if", "in", "local", "return", "then", "until", "while",
		"export", "source", "readonly", "shift", "exit", "set", "unset"),

	"java": toSet("abstract", "assert", "boolean", "break", "byte", "case",
		"catch", "char", "class", "const", "continue", "default", "do",
		"double", "else", "enum", "extends", "final", "finally", "float",
		"for", "goto", "if", "implements", "import", "instanceof", "int",
		"interface", "long", "native", "new", "null", "package", "private",
		"protected", "public", "return", "short", "static", "strictfp",
		"super", "switch", "synchronized", "this", "throw", "throws",
		"transient", "try", "void", "volatile", "while"),

	"c": toSet("auto", "break", "case", "char", "const", "continue", "default",
		"do", "double", "else", "enum", "extern", "float", "for", "goto",
		"if", "inline", "int", "long", "register", "restrict", "return",
		"short", "signed", "sizeof", "static", "struct", "switch", "typedef",
		"union", "unsigned", "void", "volatile", "while"),

	"cpp": toSet("alignas", "alignof", "and", "and_eq", "asm", "auto", "bitand",
		"bitor", "bool", "break", "case", "catch", "char", "class", "const",
		"constexpr", "continue", "decltype", "default", "delete", "do",
		"double", "dynamic_cast", "else", "enum", "explicit", "export",
		"extern", "false", "float", "for", "friend", "goto", "if", "inline",
		"int", "long", "mutable", "namespace", "new", "noexcept", "not",
		"nullptr", "operator", "or", "private", "protected", "public",
		"register", "return", "short", "signed", "sizeof", "static",
		"static_cast", "struct", "switch", "template", "this", "throw",
		"true", "try", "typedef", "typeid", "typename", "union", "unsigned",
		"using", "virtual", "void", "volatile", "while"),

	"ruby": toSet("BEGIN", "END", "alias", "and", "begin", "break", "case",
		"class", "def", "defined?", "do", "else", "elsif", "end", "ensure",
		"false", "for", "if", "in", "module", "next", "nil", "not", "or",
		"redo", "rescue", "retry", "return", "self", "super", "then", "true",
		"undef", "unless", "until", "when", "while", "yield"),

	"sql": toSet("ADD", "ALL", "ALTER", "AND", "AS", "ASC", "BETWEEN", "BY",
		"CASE", "CHECK", "COLUMN", "CONSTRAINT", "CREATE", "CROSS", "DATABASE",
		"DEFAULT", "DELETE", "DESC", "DISTINCT", "DROP", "ELSE", "END",
		"EXISTS", "FALSE", "FOREIGN", "FROM", "FULL", "GROUP", "HAVING",
		"IF", "IN", "INDEX", "INNER", "INSERT", "INTO", "IS", "JOIN", "KEY",
		"LEFT", "LIKE", "LIMIT", "NOT", "NULL", "ON", "OR", "ORDER", "OUTER",
		"PRIMARY", "REFERENCES", "RIGHT", "SELECT", "SET", "TABLE", "THEN",
		"TRUE", "UNION", "UNIQUE", "UPDATE", "VALUES", "WHEN", "WHERE"),
}

var languageTypes = map[string]map[string]bool{
	"go": toSet("bool", "byte", "complex64", "complex128", "error", "float32",
		"float64", "int", "int8", "int16", "int32", "int64", "rune",
		"string", "uint", "uint8", "uint16", "uint32", "uint64", "uintptr",
		"any", "comparable"),
	"rust": toSet("bool", "char", "f32", "f64", "i8", "i16", "i32", "i64",
		"i128", "isize", "str", "u8", "u16", "u32", "u64", "u128",
		"usize", "String", "Vec", "Option", "Result", "Box", "Arc", "Rc",
		"HashMap", "HashSet", "BTreeMap", "BTreeSet"),
	"java": toSet("boolean", "byte", "char", "double", "float", "int", "long",
		"short", "String", "Integer", "Boolean", "Long", "Double", "Float",
		"Object", "List", "Map", "Set", "ArrayList", "HashMap"),
	"typescript": toSet("string", "number", "boolean", "any", "void", "never",
		"unknown", "undefined", "null", "object", "symbol", "bigint",
		"Array", "Promise", "Record", "Partial", "Required", "Readonly"),
}

func toSet(words ...string) map[string]bool {
	m := make(map[string]bool, len(words))
	for _, w := range words {
		m[w] = true
	}
	return m
}
