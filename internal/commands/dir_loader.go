package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// LoadDirCommands walks from startDir up to the filesystem root and registers
// a slash command for every Markdown file under a `.claude/commands/`
// directory — the project-commands convention Claude Code uses (discovered
// there via --setting-sources project). Each `<name>.md` becomes `/<name>`;
// the file body is printed when the command is invoked (the same minimal
// "documentation that doubles as a command" contract as CLAUDE.md commands).
//
// Directories closer to startDir win on conflict (leaf → root walk; first
// definition for a name is kept), and a command already registered by another
// source (builtins, CLAUDE.md) is not overridden.
func LoadDirCommands(r *Registry, startDir string) error {
	if r == nil {
		return fmt.Errorf("commands: nil registry")
	}
	dirs, err := findAncestorCommandDirs(startDir)
	if err != nil {
		return err
	}
	for _, dir := range dirs {
		entries, rerr := os.ReadDir(dir)
		if rerr != nil {
			continue
		}
		// Deterministic order so a directory with several commands registers
		// reproducibly.
		sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
		for _, e := range entries {
			if e.IsDir() || !strings.EqualFold(filepath.Ext(e.Name()), ".md") {
				continue
			}
			name := strings.ToLower(strings.TrimSuffix(e.Name(), filepath.Ext(e.Name())))
			if name == "" {
				continue
			}
			if _, exists := r.Lookup(name); exists {
				continue
			}
			data, derr := os.ReadFile(filepath.Join(dir, e.Name()))
			if derr != nil {
				continue
			}
			body, desc := stripFrontmatter(string(data))
			source := filepath.Join(dir, e.Name())
			captured := strings.TrimSpace(body)
			cmdName := name
			r.Register(Command{
				Name:            "/" + name,
				Description:     desc,
				Category:        CategoryPlugin,
				ResumeSupported: true,
				Handler: func(args string, _ interface{}) error {
					fmt.Printf("[/%s — from %s]\n", cmdName, source)
					if captured != "" {
						fmt.Println(captured)
					}
					if args != "" {
						fmt.Printf("(args: %s)\n", args)
					}
					return nil
				},
			})
		}
	}
	return nil
}

// findAncestorCommandDirs returns existing `.claude/commands` directories from
// startDir up to the filesystem root, startDir's first (leaf wins).
func findAncestorCommandDirs(startDir string) ([]string, error) {
	abs, err := filepath.Abs(startDir)
	if err != nil {
		return nil, err
	}
	var dirs []string
	for {
		cand := filepath.Join(abs, ".claude", "commands")
		if info, statErr := os.Stat(cand); statErr == nil && info.IsDir() {
			dirs = append(dirs, cand)
		}
		parent := filepath.Dir(abs)
		if parent == abs {
			break
		}
		abs = parent
	}
	return dirs, nil
}

// stripFrontmatter splits an optional leading YAML frontmatter block
// (--- … ---) from the body and returns (body, description). The description
// is the frontmatter `description:` when present, else the first non-empty
// body line, capped for help display.
func stripFrontmatter(content string) (body, description string) {
	content = strings.ReplaceAll(content, "\r\n", "\n")
	body = content
	if strings.HasPrefix(content, "---\n") {
		if end := strings.Index(content[4:], "\n---"); end >= 0 {
			fm := content[4 : 4+end]
			rest := content[4+end+len("\n---"):]
			rest = strings.TrimPrefix(rest, "\n")
			body = rest
			for _, line := range strings.Split(fm, "\n") {
				if v, ok := strings.CutPrefix(strings.TrimSpace(line), "description:"); ok {
					description = strings.Trim(strings.TrimSpace(v), `"'`)
					break
				}
			}
		}
	}
	if description == "" {
		for _, line := range strings.Split(body, "\n") {
			if s := strings.TrimSpace(line); s != "" {
				description = s
				break
			}
		}
	}
	if len(description) > 120 {
		description = description[:117] + "..."
	}
	return body, description
}
