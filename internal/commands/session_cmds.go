package commands

import (
	"fmt"
	"strings"
)

// SessionManager is the interface that session-related commands require.
type SessionManager interface {
	ListSessions() ([]string, error)
	ForkSession(name string) (string, error)
	LoadSession(id string) error
	DeleteSession(id string) error
}

// RegisterSessionCommands registers session-related slash commands.
func RegisterSessionCommands(r *Registry) {
	r.Register(Command{
		Name:         "session",
		Description:  "List, switch, fork, or delete managed local sessions",
		ArgumentHint: "[list|switch <session-id>|fork [branch-name]|delete <session-id> [--force]]",
		Category:     CategorySession,
		Handler: func(args string, loop interface{}) error {
			parts := strings.Fields(args)
			sub := "list"
			if len(parts) > 0 {
				sub = strings.ToLower(parts[0])
			}

			sm, ok := loop.(SessionManager)
			if !ok {
				fmt.Println("Session management not available in this context.")
				return nil
			}

			switch sub {
			case "list", "":
				sessions, err := sm.ListSessions()
				if err != nil {
					return fmt.Errorf("list sessions: %w", err)
				}
				if len(sessions) == 0 {
					fmt.Println("No saved sessions.")
					return nil
				}
				fmt.Println("Saved sessions:")
				for _, s := range sessions {
					fmt.Printf("  %s\n", s)
				}

			case "switch":
				if len(parts) < 2 {
					fmt.Println("Usage: /session switch <session-id>")
					return nil
				}
				if err := sm.LoadSession(parts[1]); err != nil {
					return fmt.Errorf("switch session: %w", err)
				}
				fmt.Printf("Switched to session %s\n", parts[1])

			case "fork":
				name := ""
				if len(parts) > 1 {
					name = parts[1]
				}
				id, err := sm.ForkSession(name)
				if err != nil {
					return fmt.Errorf("fork session: %w", err)
				}
				fmt.Printf("Forked session: %s\n", id)

			case "delete":
				if len(parts) < 2 {
					fmt.Println("Usage: /session delete <session-id>")
					return nil
				}
				if err := sm.DeleteSession(parts[1]); err != nil {
					return fmt.Errorf("delete session: %w", err)
				}
				fmt.Printf("Deleted session %s\n", parts[1])

			default:
				fmt.Printf("Unknown session subcommand: %s\nUsage: /session [list|switch|fork|delete]\n", sub)
			}

			return nil
		},
	})

	r.Register(Command{
		Name:         "resume",
		Description:  "Load a saved session into the REPL",
		ArgumentHint: "<session-path>",
		Category:     CategorySession,
		Handler: func(args string, loop interface{}) error {
			if args == "" {
				fmt.Println("Usage: /resume <session-path>")
				return nil
			}
			sm, ok := loop.(SessionManager)
			if !ok {
				fmt.Println("Session management not available in this context.")
				return nil
			}
			if err := sm.LoadSession(strings.TrimSpace(args)); err != nil {
				return fmt.Errorf("resume session: %w", err)
			}
			fmt.Printf("Resumed session from %s\n", args)
			return nil
		},
	})

	r.Register(Command{
		Name:         "rename",
		Description:  "Rename the current session",
		ArgumentHint: "<name>",
		Category:     CategorySession,
		Handler: func(args string, loop interface{}) error {
			if args == "" {
				fmt.Println("Usage: /rename <name>")
				return nil
			}
			type sessionRenamer interface {
				RenameSession(name string) error
			}
			if sr, ok := loop.(sessionRenamer); ok {
				if err := sr.RenameSession(strings.TrimSpace(args)); err != nil {
					return fmt.Errorf("rename: %w", err)
				}
				fmt.Printf("Session renamed to %s\n", args)
			} else {
				fmt.Println("Session rename not available in this context.")
			}
			return nil
		},
	})

	r.Register(Command{
		Name:            "export",
		Description:     "Export the current conversation to a file",
		ArgumentHint:    "[file]",
		ResumeSupported: true,
		Category:        CategorySession,
		Handler: func(args string, loop interface{}) error {
			type exporter interface {
				ExportConversation(path string) error
			}
			if e, ok := loop.(exporter); ok {
				path := strings.TrimSpace(args)
				if path == "" {
					path = "conversation.json"
				}
				if err := e.ExportConversation(path); err != nil {
					return fmt.Errorf("export: %w", err)
				}
				fmt.Printf("Exported conversation to %s\n", path)
			} else {
				fmt.Println("Export not available in this context.")
			}
			return nil
		},
	})
}
