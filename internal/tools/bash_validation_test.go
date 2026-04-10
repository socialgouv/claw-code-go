package tools

import (
	"claw-code-go/internal/permissions"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// TestValidateReadOnly
// ---------------------------------------------------------------------------

func TestValidateReadOnly(t *testing.T) {
	tests := []struct {
		name      string
		command   string
		mode      permissions.PermissionMode
		wantKind  ValidationKind
		wantSubst string // substring expected in Reason or Message
	}{
		{
			name:      "blocks rm in read-only",
			command:   "rm -rf /tmp/x",
			mode:      permissions.ModeReadOnly,
			wantKind:  ValidationBlock,
			wantSubst: "rm",
		},
		{
			name:     "allows rm in workspace-write",
			command:  "rm -rf /tmp/x",
			mode:     permissions.ModeWorkspaceWrite,
			wantKind: ValidationAllow,
		},
		{
			name:      "blocks write redirections in read-only",
			command:   "echo hello > file.txt",
			mode:      permissions.ModeReadOnly,
			wantKind:  ValidationBlock,
			wantSubst: "redirection",
		},
		{
			name:     "allows ls in read-only",
			command:  "ls -la",
			mode:     permissions.ModeReadOnly,
			wantKind: ValidationAllow,
		},
		{
			name:     "allows cat in read-only",
			command:  "cat /etc/hosts",
			mode:     permissions.ModeReadOnly,
			wantKind: ValidationAllow,
		},
		{
			name:     "allows grep in read-only",
			command:  "grep -r pattern .",
			mode:     permissions.ModeReadOnly,
			wantKind: ValidationAllow,
		},
		{
			name:      "blocks sudo write in read-only",
			command:   "sudo rm -rf /tmp/x",
			mode:      permissions.ModeReadOnly,
			wantKind:  ValidationBlock,
			wantSubst: "rm",
		},
		{
			name:      "blocks git push in read-only",
			command:   "git push origin main",
			mode:      permissions.ModeReadOnly,
			wantKind:  ValidationBlock,
			wantSubst: "push",
		},
		{
			name:     "allows git status in read-only",
			command:  "git status",
			mode:     permissions.ModeReadOnly,
			wantKind: ValidationAllow,
		},
		{
			name:      "blocks package install in read-only",
			command:   "npm install express",
			mode:      permissions.ModeReadOnly,
			wantKind:  ValidationBlock,
			wantSubst: "npm",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ValidateReadOnly(tt.command, tt.mode)
			if result.Kind != tt.wantKind {
				t.Errorf("got Kind=%d, want %d", result.Kind, tt.wantKind)
			}
			if tt.wantSubst != "" {
				combined := result.Reason + result.Message
				if !strings.Contains(combined, tt.wantSubst) {
					t.Errorf("expected substring %q in %q", tt.wantSubst, combined)
				}
			}
		})
	}
}

// ---------------------------------------------------------------------------
// TestCheckDestructive
// ---------------------------------------------------------------------------

func TestCheckDestructive(t *testing.T) {
	tests := []struct {
		name      string
		command   string
		wantKind  ValidationKind
		wantSubst string
	}{
		{"warns rm -rf /", "rm -rf /", ValidationWarn, "root"},
		{"warns rm -rf ~", "rm -rf ~", ValidationWarn, "home"},
		{"warns shred", "shred /dev/sda", ValidationWarn, "destructive"},
		{"warns fork bomb", ":(){ :|:& };:", ValidationWarn, "Fork bomb"},
		{"allows ls", "ls -la", ValidationAllow, ""},
		{"allows echo", "echo hello", ValidationAllow, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CheckDestructive(tt.command)
			if result.Kind != tt.wantKind {
				t.Errorf("got Kind=%d, want %d", result.Kind, tt.wantKind)
			}
			if tt.wantSubst != "" && !strings.Contains(result.Message, tt.wantSubst) {
				t.Errorf("expected substring %q in message %q", tt.wantSubst, result.Message)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// TestValidateMode
// ---------------------------------------------------------------------------

func TestValidateMode(t *testing.T) {
	tests := []struct {
		name      string
		command   string
		mode      permissions.PermissionMode
		wantKind  ValidationKind
		wantSubst string
	}{
		{
			name:      "workspace-write warns system paths",
			command:   "cp file.txt /etc/config",
			mode:      permissions.ModeWorkspaceWrite,
			wantKind:  ValidationWarn,
			wantSubst: "outside the workspace",
		},
		{
			name:     "workspace-write allows local writes",
			command:  "cp file.txt ./backup/",
			mode:     permissions.ModeWorkspaceWrite,
			wantKind: ValidationAllow,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ValidateMode(tt.command, tt.mode)
			if result.Kind != tt.wantKind {
				t.Errorf("got Kind=%d, want %d", result.Kind, tt.wantKind)
			}
			if tt.wantSubst != "" && !strings.Contains(result.Message, tt.wantSubst) {
				t.Errorf("expected substring %q in message %q", tt.wantSubst, result.Message)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// TestValidateSed
// ---------------------------------------------------------------------------

func TestValidateSed(t *testing.T) {
	tests := []struct {
		name      string
		command   string
		mode      permissions.PermissionMode
		wantKind  ValidationKind
		wantSubst string
	}{
		{
			name:      "blocks sed -i in read-only",
			command:   "sed -i 's/old/new/' file.txt",
			mode:      permissions.ModeReadOnly,
			wantKind:  ValidationBlock,
			wantSubst: "sed -i",
		},
		{
			name:     "allows sed stdout in read-only",
			command:  "sed 's/old/new/' file.txt",
			mode:     permissions.ModeReadOnly,
			wantKind: ValidationAllow,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ValidateSed(tt.command, tt.mode)
			if result.Kind != tt.wantKind {
				t.Errorf("got Kind=%d, want %d", result.Kind, tt.wantKind)
			}
			if tt.wantSubst != "" && !strings.Contains(result.Reason, tt.wantSubst) {
				t.Errorf("expected substring %q in reason %q", tt.wantSubst, result.Reason)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// TestValidatePaths
// ---------------------------------------------------------------------------

func TestValidatePaths(t *testing.T) {
	workspace := "/workspace/project"
	tests := []struct {
		name      string
		command   string
		wantKind  ValidationKind
		wantSubst string
	}{
		{
			name:      "warns directory traversal",
			command:   "cat ../../../etc/passwd",
			wantKind:  ValidationWarn,
			wantSubst: "traversal",
		},
		{
			name:      "warns home directory reference",
			command:   "cat ~/.ssh/id_rsa",
			wantKind:  ValidationWarn,
			wantSubst: "home directory",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ValidatePaths(tt.command, workspace)
			if result.Kind != tt.wantKind {
				t.Errorf("got Kind=%d, want %d", result.Kind, tt.wantKind)
			}
			if tt.wantSubst != "" && !strings.Contains(result.Message, tt.wantSubst) {
				t.Errorf("expected substring %q in message %q", tt.wantSubst, result.Message)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// TestClassifyCommand
// ---------------------------------------------------------------------------

func TestClassifyCommand(t *testing.T) {
	tests := []struct {
		name    string
		command string
		want    CommandIntent
	}{
		{"ls is read-only", "ls -la", ReadOnly},
		{"cat is read-only", "cat file.txt", ReadOnly},
		{"grep is read-only", "grep -r pattern .", ReadOnly},
		{"find is read-only", "find . -name '*.rs'", ReadOnly},
		{"cp is write", "cp a.txt b.txt", Write},
		{"mv is write", "mv old.txt new.txt", Write},
		{"mkdir is write", "mkdir -p /tmp/dir", Write},
		{"rm is destructive", "rm -rf /tmp/x", Destructive},
		{"shred is destructive", "shred /dev/sda", Destructive},
		{"curl is network", "curl https://example.com", Network},
		{"wget is network", "wget file.zip", Network},
		{"sed -i is write", "sed -i 's/old/new/' file.txt", Write},
		{"sed stdout is read-only", "sed 's/old/new/' file.txt", ReadOnly},
		{"git status is read-only", "git status", ReadOnly},
		{"git log is read-only", "git log --oneline", ReadOnly},
		{"git push is write", "git push origin main", Write},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ClassifyCommand(tt.command)
			if got != tt.want {
				t.Errorf("ClassifyCommand(%q) = %d, want %d", tt.command, got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// TestValidateCommand_Pipeline
// ---------------------------------------------------------------------------

func TestValidateCommand_Pipeline(t *testing.T) {
	workspace := "/workspace"
	tests := []struct {
		name     string
		command  string
		mode     permissions.PermissionMode
		wantKind ValidationKind
	}{
		{
			name:     "blocks write in read-only",
			command:  "rm -rf /tmp/x",
			mode:     permissions.ModeReadOnly,
			wantKind: ValidationBlock,
		},
		{
			name:     "warns destructive in write mode",
			command:  "rm -rf /",
			mode:     permissions.ModeWorkspaceWrite,
			wantKind: ValidationWarn,
		},
		{
			name:     "allows safe read in read-only",
			command:  "ls -la",
			mode:     permissions.ModeReadOnly,
			wantKind: ValidationAllow,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ValidateCommand(tt.command, tt.mode, workspace)
			if result.Kind != tt.wantKind {
				t.Errorf("got Kind=%d, want %d", result.Kind, tt.wantKind)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// TestExtractFirstCommand
// ---------------------------------------------------------------------------

func TestExtractFirstCommand(t *testing.T) {
	tests := []struct {
		name    string
		command string
		want    string
	}{
		{"command from env prefix", "FOO=bar ls -la", "ls"},
		{"multiple env prefixes", "A=1 B=2 echo hello", "echo"},
		{"plain command", "grep -r pattern .", "grep"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractFirstCommand(tt.command)
			if got != tt.want {
				t.Errorf("extractFirstCommand(%q) = %q, want %q", tt.command, got, tt.want)
			}
		})
	}
}
