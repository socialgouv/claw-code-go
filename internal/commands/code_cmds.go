package commands

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// runGit executes a git command with separate arguments (never shell-interpolated)
// and returns the trimmed combined output.
func runGit(args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("git %s: %w", args[0], err)
	}
	return strings.TrimSpace(string(out)), nil
}

// gitAvailable returns true if the git binary is on PATH.
func gitAvailable() bool {
	_, err := exec.LookPath("git")
	return err == nil
}

// RegisterCodeCommands registers code-operations slash commands.
func RegisterCodeCommands(r *Registry) {
	r.Register(Command{
		Name:        "commit",
		Description: "Generate commit message from git diff",
		Category:    CategoryCode,
		Handler: func(args string, loop interface{}) error {
			if !gitAvailable() {
				fmt.Println("git is not available on PATH.")
				return nil
			}

			diff, err := runGit("diff", "--cached", "--stat")
			if err != nil {
				return fmt.Errorf("commit: %w", err)
			}
			if diff == "" {
				fmt.Println("No staged changes. Use `git add` to stage files first.")
				return nil
			}

			// Try the optional commitLoop interface for AI-generated messages.
			type commitLoop interface {
				GenerateCommitMessage(diff string) (string, error)
			}
			if cl, ok := loop.(commitLoop); ok {
				msg, err := cl.GenerateCommitMessage(diff)
				if err != nil {
					return fmt.Errorf("commit: generate message: %w", err)
				}
				fmt.Println(msg)
				return nil
			}

			// Fallback: print the diff stats as a template.
			fmt.Println("Staged changes:")
			fmt.Println(diff)
			fmt.Println("\nDraft a commit message based on the above changes.")
			return nil
		},
	})

	r.Register(Command{
		Name:         "pr",
		Description:  "Draft/create pull request",
		ArgumentHint: "[title]",
		Category:     CategoryCode,
		Handler: func(args string, loop interface{}) error {
			if !gitAvailable() {
				fmt.Println("git is not available on PATH.")
				return nil
			}

			branch, err := runGit("rev-parse", "--abbrev-ref", "HEAD")
			if err != nil {
				return fmt.Errorf("pr: %w", err)
			}

			log, err := runGit("log", "--oneline", "-20")
			if err != nil {
				return fmt.Errorf("pr: %w", err)
			}

			diff, err := runGit("diff", "HEAD~5..HEAD", "--stat")
			if err != nil {
				// Non-fatal: repo might have fewer than 5 commits.
				diff = "(diff unavailable)"
			}

			title := strings.TrimSpace(args)
			if title == "" {
				title = branch
			}

			fmt.Printf("## Pull Request: %s\n\n", title)
			fmt.Printf("**Branch:** %s\n\n", branch)
			fmt.Println("### Recent commits")
			fmt.Println(log)
			fmt.Println("\n### Diff summary")
			fmt.Println(diff)
			fmt.Println("\nDraft a PR description based on the above context.")
			return nil
		},
	})

	r.Register(Command{
		Name:         "issue",
		Description:  "Draft GitHub issue from conversation context",
		ArgumentHint: "[title]",
		Category:     CategoryCode,
		Handler: func(args string, loop interface{}) error {
			title := strings.TrimSpace(args)
			if title == "" {
				title = "New Issue"
			}
			fmt.Printf("## GitHub Issue: %s\n\n", title)
			fmt.Println("### Description")
			fmt.Println()
			fmt.Println("### Steps to Reproduce")
			fmt.Println()
			fmt.Println("### Expected Behavior")
			fmt.Println()
			fmt.Println("### Actual Behavior")
			fmt.Println()
			fmt.Println("\nFill in the sections above based on the conversation context.")
			return nil
		},
	})

	r.Register(Command{
		Name:         "bughunter",
		Description:  "Scope-aware bug hunting prompt",
		ArgumentHint: "[scope]",
		Category:     CategoryCode,
		Handler: func(args string, loop interface{}) error {
			scope := strings.TrimSpace(args)
			if scope == "" {
				scope = "the current codebase"
			}
			fmt.Printf("Bug Hunting Scope: %s\n\n", scope)
			fmt.Println("Analyze the code in the given scope for:")
			fmt.Println("  1. Logic errors and off-by-one mistakes")
			fmt.Println("  2. Nil/null pointer dereferences")
			fmt.Println("  3. Resource leaks (unclosed files, connections)")
			fmt.Println("  4. Race conditions and concurrency issues")
			fmt.Println("  5. Error handling gaps (swallowed errors, missing checks)")
			fmt.Println("  6. Edge cases in input validation")
			fmt.Printf("\nFocus on: %s\n", scope)
			return nil
		},
	})

	r.Register(Command{
		Name:         "review",
		Description:  "Code review from git diff",
		ArgumentHint: "[--staged]",
		Category:     CategoryCode,
		Handler: func(args string, loop interface{}) error {
			if !gitAvailable() {
				fmt.Println("git is not available on PATH.")
				return nil
			}

			diffArgs := []string{"diff"}
			if strings.TrimSpace(args) == "--staged" {
				diffArgs = append(diffArgs, "--cached")
			}

			diff, err := runGit(diffArgs...)
			if err != nil {
				return fmt.Errorf("review: %w", err)
			}
			if diff == "" {
				fmt.Println("No changes to review.")
				return nil
			}

			fmt.Println("## Code Review")
			fmt.Println()
			fmt.Println("Review the following diff for:")
			fmt.Println("  - Correctness and logic errors")
			fmt.Println("  - Code style and readability")
			fmt.Println("  - Performance concerns")
			fmt.Println("  - Test coverage gaps")
			fmt.Println("\n```diff")
			fmt.Println(diff)
			fmt.Println("```")
			return nil
		},
	})

	r.Register(Command{
		Name:         "security-review",
		Description:  "Security-focused code review",
		ArgumentHint: "[--staged]",
		Category:     CategoryCode,
		Handler: func(args string, loop interface{}) error {
			if !gitAvailable() {
				fmt.Println("git is not available on PATH.")
				return nil
			}

			diffArgs := []string{"diff"}
			if strings.TrimSpace(args) == "--staged" {
				diffArgs = append(diffArgs, "--cached")
			}

			diff, err := runGit(diffArgs...)
			if err != nil {
				return fmt.Errorf("security-review: %w", err)
			}
			if diff == "" {
				fmt.Println("No changes to review.")
				return nil
			}

			fmt.Println("## Security Review")
			fmt.Println()
			fmt.Println("Review the following diff for security issues:")
			fmt.Println("  - Injection vulnerabilities (SQL, command, XSS)")
			fmt.Println("  - Authentication and authorization flaws")
			fmt.Println("  - Sensitive data exposure (secrets, PII)")
			fmt.Println("  - Insecure deserialization")
			fmt.Println("  - Dependency vulnerabilities")
			fmt.Println("  - Path traversal and file access issues")
			fmt.Println("\n```diff")
			fmt.Println(diff)
			fmt.Println("```")
			return nil
		},
	})

	r.Register(Command{
		Name:         "release-notes",
		Description:  "Generate changelog from git log",
		ArgumentHint: "[since-tag]",
		Category:     CategoryCode,
		Handler: func(args string, loop interface{}) error {
			if !gitAvailable() {
				fmt.Println("git is not available on PATH.")
				return nil
			}

			sinceTag := strings.TrimSpace(args)
			var logArgs []string
			if sinceTag != "" {
				logArgs = []string{"log", "--oneline", sinceTag + "..HEAD"}
			} else {
				// Try to find the last tag automatically.
				tag, err := runGit("describe", "--tags", "--abbrev=0")
				if err != nil || tag == "" {
					logArgs = []string{"log", "--oneline", "-30"}
				} else {
					logArgs = []string{"log", "--oneline", tag + "..HEAD"}
				}
			}

			log, err := runGit(logArgs...)
			if err != nil {
				return fmt.Errorf("release-notes: %w", err)
			}
			if log == "" {
				fmt.Println("No commits found for release notes.")
				return nil
			}

			fmt.Println("## Release Notes")
			fmt.Println()
			fmt.Println("Commits:")
			fmt.Println(log)
			fmt.Println("\nGenerate categorized release notes (features, fixes, breaking changes) from the above.")
			return nil
		},
	})

	r.Register(Command{
		Name:         "test",
		Description:  "Run project tests",
		ArgumentHint: "[--watch]",
		Category:     CategoryCode,
		Handler: func(args string, loop interface{}) error {
			testCmd, testArgs := detectTestCommand()
			if testCmd == "" {
				fmt.Println("Could not detect project type. No test command found.")
				return nil
			}

			if strings.TrimSpace(args) == "--watch" {
				fmt.Printf("Watch mode requested. Run: %s %s (watch not built-in; use project tooling).\n", testCmd, strings.Join(testArgs, " "))
				return nil
			}

			fmt.Printf("Running: %s %s\n", testCmd, strings.Join(testArgs, " "))
			cmd := exec.Command(testCmd, testArgs...)
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			if err := cmd.Run(); err != nil {
				return fmt.Errorf("test: %w", err)
			}
			return nil
		},
	})
}

// detectTestCommand inspects the working directory for known project markers
// and returns the appropriate test command and arguments.
func detectTestCommand() (string, []string) {
	if _, err := os.Stat("go.mod"); err == nil {
		return "go", []string{"test", "./..."}
	}
	if _, err := os.Stat("Cargo.toml"); err == nil {
		return "cargo", []string{"test"}
	}
	if _, err := os.Stat("package.json"); err == nil {
		return "npm", []string{"test"}
	}
	if _, err := os.Stat("Makefile"); err == nil {
		return "make", []string{"test"}
	}
	return "", nil
}
