package commands

import (
	"testing"
)

func TestRegistryListDeduplicates(t *testing.T) {
	r := NewRegistry()
	r.Register(Command{
		Name:        "test",
		Aliases:     []string{"t", "tst"},
		Description: "test command",
		Handler:     func(args string, loop interface{}) error { return nil },
	})

	cmds := r.List()
	count := 0
	for _, c := range cmds {
		if c.Name == "test" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("expected 1 'test' command in list, got %d (total commands: %d)", count, len(cmds))
	}
}

func TestRegistryLookup(t *testing.T) {
	r := NewRegistry()
	r.Register(Command{
		Name:        "test",
		Aliases:     []string{"t"},
		Description: "test command",
		Handler:     func(args string, loop interface{}) error { return nil },
	})

	// Lookup by primary name
	cmd, ok := r.Lookup("test")
	if !ok {
		t.Fatal("expected to find 'test'")
	}
	if cmd.Name != "test" {
		t.Errorf("expected Name='test', got %q", cmd.Name)
	}

	// Lookup by alias
	cmd, ok = r.Lookup("t")
	if !ok {
		t.Fatal("expected to find 't' alias")
	}
	if cmd.Name != "test" {
		t.Errorf("expected Name='test' from alias, got %q", cmd.Name)
	}

	// Lookup with / prefix
	cmd, ok = r.Lookup("/test")
	if !ok {
		t.Fatal("expected to find '/test'")
	}

	// Lookup missing
	_, ok = r.Lookup("nonexistent")
	if ok {
		t.Error("expected not to find 'nonexistent'")
	}
}

func TestRegistryCount(t *testing.T) {
	r := NewRegistry()
	// Builtins: help, exit, quit, clear, session-list = 5
	baseline := r.Count()
	r.Register(Command{
		Name:    "custom",
		Aliases: []string{"c"},
		Handler: func(args string, loop interface{}) error { return nil },
	})
	if got := r.Count(); got != baseline+1 {
		t.Errorf("expected %d commands after adding 1, got %d", baseline+1, got)
	}
}

func TestRegistrySuggestCommands(t *testing.T) {
	r := NewRegistry()
	r.Register(Command{
		Name:    "session",
		Handler: func(args string, loop interface{}) error { return nil },
	})
	r.Register(Command{
		Name:    "status",
		Handler: func(args string, loop interface{}) error { return nil },
	})
	r.Register(Command{
		Name:    "compact",
		Handler: func(args string, loop interface{}) error { return nil },
	})

	suggestions := r.SuggestCommands("s", 10)
	if len(suggestions) < 2 {
		t.Errorf("expected at least 2 suggestions for 's', got %d", len(suggestions))
	}
	for _, s := range suggestions {
		if s[0] != '/' {
			t.Errorf("suggestion %q should start with '/'", s)
		}
	}

	// Limit
	limited := r.SuggestCommands("", 2)
	if len(limited) > 2 {
		t.Errorf("expected at most 2 suggestions, got %d", len(limited))
	}
}

func TestRegistryResumeSupportedCommands(t *testing.T) {
	r := NewRegistry()
	r.Register(Command{
		Name:            "resumable",
		ResumeSupported: true,
		Handler:         func(args string, loop interface{}) error { return nil },
	})
	r.Register(Command{
		Name:    "not-resumable",
		Handler: func(args string, loop interface{}) error { return nil },
	})

	resumable := r.ResumeSupportedCommands()
	for _, cmd := range resumable {
		if !cmd.ResumeSupported {
			t.Errorf("command %q in resume list but ResumeSupported=false", cmd.Name)
		}
	}
	// The built-in /help is resume-supported
	found := false
	for _, cmd := range resumable {
		if cmd.Name == "help" || cmd.Name == "resumable" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected at least one resume-supported command")
	}
}

func TestExecuteExit(t *testing.T) {
	r := NewRegistry()

	handled, err := r.Execute("/exit", nil)
	if !handled {
		t.Error("expected /exit to be handled")
	}
	if err != ErrExit {
		t.Errorf("expected ErrExit, got %v", err)
	}
}

func TestExecuteQuit(t *testing.T) {
	r := NewRegistry()

	handled, err := r.Execute("/quit", nil)
	if !handled {
		t.Error("expected /quit to be handled")
	}
	if err != ErrExit {
		t.Errorf("expected ErrExit, got %v", err)
	}
}

func TestExecuteUnknown(t *testing.T) {
	r := NewRegistry()

	handled, _ := r.Execute("/nonexistent", nil)
	if !handled {
		t.Error("expected unknown command to still be handled (with error message)")
	}
}

func TestExecuteNonCommand(t *testing.T) {
	r := NewRegistry()

	handled, _ := r.Execute("just a message", nil)
	if handled {
		t.Error("expected non-command input to return handled=false")
	}
}

func TestCommandsByCategory(t *testing.T) {
	r := NewRegistry()
	RegisterStatusCommands(r)
	RegisterConfigCommands(r)
	RegisterDiagnosticCommands(r)

	statusCmds := r.CommandsByCategory(CategoryStatus)
	if len(statusCmds) == 0 {
		t.Error("expected at least one status command")
	}
	for _, cmd := range statusCmds {
		if cmd.Category != CategoryStatus {
			t.Errorf("command %q has category %q, want %q", cmd.Name, cmd.Category, CategoryStatus)
		}
	}

	configCmds := r.CommandsByCategory(CategoryConfig)
	if len(configCmds) == 0 {
		t.Error("expected at least one config command")
	}

	diagCmds := r.CommandsByCategory(CategoryDiagnostics)
	if len(diagCmds) == 0 {
		t.Error("expected at least one diagnostic command")
	}
}

func TestCategories(t *testing.T) {
	r := NewRegistry()
	RegisterStatusCommands(r)
	RegisterConfigCommands(r)

	cats := r.Categories()
	if len(cats) < 2 {
		t.Errorf("expected at least 2 categories, got %d", len(cats))
	}
	foundStatus := false
	foundConfig := false
	for _, c := range cats {
		if c == CategoryStatus {
			foundStatus = true
		}
		if c == CategoryConfig {
			foundConfig = true
		}
	}
	if !foundStatus {
		t.Error("expected CategoryStatus in categories")
	}
	if !foundConfig {
		t.Error("expected CategoryConfig in categories")
	}
}

func TestHelpWithArgShowsDetail(t *testing.T) {
	r := NewRegistry()
	r.Register(Command{
		Name:         "test-cmd",
		Aliases:      []string{"tc"},
		Description:  "A test command",
		ArgumentHint: "<arg>",
		Handler:      func(args string, loop interface{}) error { return nil },
	})

	// Should not error
	handled, err := r.Execute("/help test-cmd", nil)
	if !handled || err != nil {
		t.Errorf("expected help detail: handled=%v, err=%v", handled, err)
	}

	// Unknown command help
	handled, err = r.Execute("/help nonexistent", nil)
	if !handled || err != nil {
		t.Errorf("expected help for unknown: handled=%v, err=%v", handled, err)
	}
}
