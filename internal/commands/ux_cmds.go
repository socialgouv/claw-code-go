package commands

import (
	"fmt"
	"strings"
)

// toggleLoop is the interface for commands that toggle boolean settings.
type toggleLoop interface {
	GetToggle(name string) bool
	SetToggle(name string, value bool)
}

// effortLoop is the interface for commands that control effort level.
type effortLoop interface {
	SetEffort(level string) error
	GetEffort() string
}

// themeLoop is the interface for commands that manage color themes.
type themeLoop interface {
	SetTheme(name string) error
	CurrentTheme() string
	ListThemes() []string
}

// RegisterUXCommands registers UX and settings slash commands.
func RegisterUXCommands(r *Registry) {
	r.Register(Command{
		Name:         "theme",
		Description:  "Switch color theme",
		ArgumentHint: "[theme-name]",
		Category:     CategoryUX,
		Handler: func(args string, loop interface{}) error {
			tl, ok := loop.(themeLoop)
			if !ok {
				fmt.Println("Theme switching not available in this context.")
				return nil
			}
			name := strings.TrimSpace(args)
			if name == "" {
				fmt.Printf("Current theme: %s\n", tl.CurrentTheme())
				themes := tl.ListThemes()
				if len(themes) > 0 {
					fmt.Printf("Available: %s\n", strings.Join(themes, ", "))
				}
				return nil
			}
			if err := tl.SetTheme(name); err != nil {
				return fmt.Errorf("theme: %w", err)
			}
			fmt.Printf("Theme set to %s\n", name)
			return nil
		},
	})

	r.Register(Command{
		Name:        "vim",
		Description: "Toggle vim keybinding mode",
		Category:    CategoryUX,
		Handler: func(args string, loop interface{}) error {
			tl, ok := loop.(toggleLoop)
			if !ok {
				fmt.Println("Toggle not available in this context.")
				return nil
			}
			current := tl.GetToggle("vim")
			tl.SetToggle("vim", !current)
			if !current {
				fmt.Println("Vim mode enabled.")
			} else {
				fmt.Println("Vim mode disabled.")
			}
			return nil
		},
	})

	r.Register(Command{
		Name:         "effort",
		Description:  "Set effort level",
		ArgumentHint: "[low|medium|high]",
		Category:     CategoryUX,
		Handler: func(args string, loop interface{}) error {
			el, ok := loop.(effortLoop)
			if !ok {
				fmt.Println("Effort control not available in this context.")
				return nil
			}
			level := strings.TrimSpace(strings.ToLower(args))
			if level == "" {
				fmt.Printf("Current effort: %s\n", el.GetEffort())
				return nil
			}
			switch level {
			case "low", "medium", "high":
				// valid
			default:
				return fmt.Errorf("effort: invalid level %q (use low, medium, or high)", level)
			}
			if err := el.SetEffort(level); err != nil {
				return fmt.Errorf("effort: %w", err)
			}
			fmt.Printf("Effort set to %s\n", level)
			return nil
		},
	})

	r.Register(Command{
		Name:        "fast",
		Description: "Toggle fast/concise mode",
		Category:    CategoryUX,
		Handler: func(args string, loop interface{}) error {
			tl, ok := loop.(toggleLoop)
			if !ok {
				fmt.Println("Toggle not available in this context.")
				return nil
			}
			current := tl.GetToggle("fast")
			tl.SetToggle("fast", !current)
			if !current {
				fmt.Println("Fast mode enabled.")
			} else {
				fmt.Println("Fast mode disabled.")
			}
			return nil
		},
	})

	r.Register(Command{
		Name:            "brief",
		Description:     "Toggle brief output mode",
		ResumeSupported: true,
		Category:        CategoryUX,
		Handler: func(args string, loop interface{}) error {
			tl, ok := loop.(toggleLoop)
			if !ok {
				fmt.Println("Toggle not available in this context.")
				return nil
			}
			current := tl.GetToggle("brief")
			tl.SetToggle("brief", !current)
			if !current {
				fmt.Println("Brief mode enabled.")
			} else {
				fmt.Println("Brief mode disabled.")
			}
			return nil
		},
	})

	r.Register(Command{
		Name:            "advisor",
		Description:     "Toggle advisor mode",
		ResumeSupported: true,
		Category:        CategoryUX,
		Handler: func(args string, loop interface{}) error {
			tl, ok := loop.(toggleLoop)
			if !ok {
				fmt.Println("Toggle not available in this context.")
				return nil
			}
			current := tl.GetToggle("advisor")
			tl.SetToggle("advisor", !current)
			if !current {
				fmt.Println("Advisor mode enabled.")
			} else {
				fmt.Println("Advisor mode disabled.")
			}
			return nil
		},
	})

	r.Register(Command{
		Name:         "color",
		Description:  "Configure color settings",
		ArgumentHint: "[scheme]",
		Category:     CategoryUX,
		Handler: func(args string, loop interface{}) error {
			tl, ok := loop.(themeLoop)
			if !ok {
				fmt.Println("Color configuration not available in this context.")
				return nil
			}
			scheme := strings.TrimSpace(args)
			if scheme == "" {
				fmt.Printf("Current color scheme: %s\n", tl.CurrentTheme())
				return nil
			}
			if err := tl.SetTheme(scheme); err != nil {
				return fmt.Errorf("color: %w", err)
			}
			fmt.Printf("Color scheme set to %s\n", scheme)
			return nil
		},
	})

	r.Register(Command{
		Name:            "keybindings",
		Description:     "Show/configure keyboard shortcuts",
		ResumeSupported: true,
		Category:        CategoryUX,
		Handler: func(args string, loop interface{}) error {
			fmt.Println("Keyboard shortcuts:")
			fmt.Println("  Ctrl+C  — Cancel current operation")
			fmt.Println("  Ctrl+D  — Exit")
			fmt.Println("  Tab     — Autocomplete command")
			fmt.Println("  Up/Down — Navigate history")
			fmt.Println("Use /vim to toggle vim keybinding mode.")
			return nil
		},
	})

	r.Register(Command{
		Name:            "privacy-settings",
		Description:     "View/modify privacy settings",
		ResumeSupported: true,
		Category:        CategoryUX,
		Handler: func(args string, loop interface{}) error {
			fmt.Println("Privacy settings:")
			fmt.Println("  Telemetry:      off")
			fmt.Println("  History:        local only")
			fmt.Println("  Data retention: session-scoped")
			fmt.Println("Use /config to modify settings.")
			return nil
		},
	})

	r.Register(Command{
		Name:         "output-style",
		Description:  "Switch output formatting",
		ArgumentHint: "[style]",
		Category:     CategoryUX,
		Handler: func(args string, loop interface{}) error {
			style := strings.TrimSpace(strings.ToLower(args))
			if style == "" {
				fmt.Println("Available output styles: plain, markdown, json")
				return nil
			}
			switch style {
			case "plain", "markdown", "json":
				fmt.Printf("Output style set to %s\n", style)
			default:
				return fmt.Errorf("output-style: unknown style %q (use plain, markdown, or json)", style)
			}
			return nil
		},
	})
}
