package commands

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestClaudeMdLoader_FindsAncestorFiles(t *testing.T) {
	dir := t.TempDir()
	parent := filepath.Join(dir, "parent")
	child := filepath.Join(parent, "child")
	writeFile(t, filepath.Join(parent, "CLAUDE.md"), "## /parent-cmd\nDoes parent stuff.\n")
	writeFile(t, filepath.Join(child, "CLAUDE.md"), "## /child-cmd\nDoes child stuff.\n")

	r := NewRegistry()
	if err := LoadClaudeMdCommands(r, child); err != nil {
		t.Fatalf("LoadClaudeMdCommands: %v", err)
	}
	for _, name := range []string{"child-cmd", "parent-cmd"} {
		if _, err := r.Execute("/"+name, nil); err != nil {
			t.Errorf("/%s execute: %v", name, err)
		}
	}
}

func TestClaudeMdLoader_ParsesSlashBlocks(t *testing.T) {
	src := `# Project

Some intro.

## /alpha
First command body.
Second line of alpha.

## /beta
Beta description.

## Section that is not a slash command
Regular section.

## /gamma
Gamma command body.
`
	r := NewRegistry()
	cmds, err := parseClaudeMdCommands(strings.NewReader(src), "test.md")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	names := make([]string, 0, len(cmds))
	for _, c := range cmds {
		names = append(names, c.Name)
	}
	want := []string{"alpha", "beta", "gamma"}
	if len(names) != len(want) {
		t.Fatalf("expected %d commands, got %d (%v)", len(want), len(names), names)
	}
	for i, n := range want {
		if names[i] != n {
			t.Errorf("at %d: got %q want %q", i, names[i], n)
		}
	}
	// Description should be the first body line.
	for _, c := range cmds {
		if c.Description == "" {
			t.Errorf("command %q has empty description", c.Name)
		}
	}
	_ = r
}

func TestClaudeMdLoader_LeafWinsOnConflict(t *testing.T) {
	dir := t.TempDir()
	parent := filepath.Join(dir, "parent")
	child := filepath.Join(parent, "child")
	writeFile(t, filepath.Join(parent, "CLAUDE.md"), "## /shared\nFROM PARENT\n")
	writeFile(t, filepath.Join(child, "CLAUDE.md"), "## /shared\nFROM CHILD\n")

	r := NewRegistry()
	if err := LoadClaudeMdCommands(r, child); err != nil {
		t.Fatalf("load: %v", err)
	}
	// Capture stdout via a pipe to confirm the child body ran.
	old := os.Stdout
	rPipe, wPipe, _ := os.Pipe()
	os.Stdout = wPipe
	if _, err := r.Execute("/shared", nil); err != nil {
		t.Errorf("execute: %v", err)
	}
	wPipe.Close()
	os.Stdout = old
	out, _ := readAll(rPipe)
	if !strings.Contains(out, "FROM CHILD") {
		t.Errorf("expected child body in output, got:\n%s", out)
	}
}

func TestClaudeMdLoader_RegistersAsDynamicCommands(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "CLAUDE.md"), "## /dynamic\nThis was loaded from CLAUDE.md.\n")
	r := NewRegistry()
	before := r.commands["dynamic"]
	_ = before
	if err := LoadClaudeMdCommands(r, dir); err != nil {
		t.Fatalf("load: %v", err)
	}
	cmd, ok := r.commands["dynamic"]
	if !ok {
		t.Fatal("expected 'dynamic' command registered")
	}
	if cmd.Handler == nil {
		t.Error("expected non-nil handler")
	}
	if !cmd.ResumeSupported {
		t.Error("expected ResumeSupported=true on auto-loaded commands")
	}
}

func TestExtractCommandName(t *testing.T) {
	cases := []struct{ in, out string }{
		{"## /foo", "foo"},
		{"## /foo bar baz", "foo"},
		{"## /Camel-Case", "camel-case"},
		{"## not-a-cmd", "not-a-cmd"},
	}
	for _, tc := range cases {
		if got := extractCommandName(tc.in); got != tc.out {
			t.Errorf("extractCommandName(%q) = %q, want %q", tc.in, got, tc.out)
		}
	}
}

func readAll(r *os.File) (string, error) {
	var sb strings.Builder
	buf := make([]byte, 4096)
	for {
		n, err := r.Read(buf)
		if n > 0 {
			sb.Write(buf[:n])
		}
		if err != nil {
			break
		}
	}
	return sb.String(), nil
}
