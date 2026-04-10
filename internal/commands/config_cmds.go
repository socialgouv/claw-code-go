package commands

import (
	"fmt"
	"os"
	"strings"
)

// ConfigSwitcher provides model and permission switching for config commands.
type ConfigSwitcher interface {
	CurrentModel() string
	SetModel(model string) error
	CurrentPermissionMode() string
	SetPermissionMode(mode string) error
}

// RegisterConfigCommands registers config-related slash commands.
func RegisterConfigCommands(r *Registry) {
	r.Register(Command{
		Name:            "config",
		Description:     "Inspect Claude config files or merged sections",
		ArgumentHint:    "[env|hooks|model|plugins]",
		ResumeSupported: true,
		Category:        CategoryConfig,
		Handler: func(args string, loop interface{}) error {
			sub := strings.TrimSpace(strings.ToLower(args))

			switch sub {
			case "env":
				fmt.Println("Environment variables:")
				for _, env := range relevantEnvVars() {
					val := os.Getenv(env)
					if val == "" {
						val = "(not set)"
					}
					fmt.Printf("  %s = %s\n", env, val)
				}

			case "model":
				cs, ok := loop.(ConfigSwitcher)
				if !ok {
					fmt.Println("Config not available in this context.")
					return nil
				}
				fmt.Printf("Current model: %s\n", cs.CurrentModel())

			case "hooks":
				fmt.Println("Hooks configuration:")
				fmt.Println("  Use /hooks list to inspect lifecycle hooks.")

			case "plugins":
				fmt.Println("Plugin configuration:")
				fmt.Println("  Use /plugin list to inspect installed plugins.")

			case "", "help":
				fmt.Println("Usage: /config [env|hooks|model|plugins]")
				fmt.Println("  env      — Show relevant environment variables")
				fmt.Println("  hooks    — Show hooks configuration")
				fmt.Println("  model    — Show current model")
				fmt.Println("  plugins  — Show plugin configuration")

			default:
				fmt.Printf("Unknown config subcommand: %s\n", sub)
				fmt.Println("Usage: /config [env|hooks|model|plugins]")
			}

			return nil
		},
	})

	r.Register(Command{
		Name:         "model",
		Description:  "Show or switch the active model",
		ArgumentHint: "[model]",
		Category:     CategoryConfig,
		Handler: func(args string, loop interface{}) error {
			cs, ok := loop.(ConfigSwitcher)
			if !ok {
				fmt.Println("Model switching not available in this context.")
				return nil
			}
			if args == "" {
				fmt.Printf("Current model: %s\n", cs.CurrentModel())
				return nil
			}
			if err := cs.SetModel(strings.TrimSpace(args)); err != nil {
				return fmt.Errorf("set model: %w", err)
			}
			fmt.Printf("Model set to %s\n", args)
			return nil
		},
	})

	r.Register(Command{
		Name:         "permissions",
		Description:  "Show or switch the active permission mode",
		ArgumentHint: "[read-only|workspace-write|danger-full-access]",
		Category:     CategoryConfig,
		Handler: func(args string, loop interface{}) error {
			cs, ok := loop.(ConfigSwitcher)
			if !ok {
				fmt.Println("Permission mode switching not available in this context.")
				return nil
			}
			if args == "" {
				fmt.Printf("Current permission mode: %s\n", cs.CurrentPermissionMode())
				return nil
			}
			if err := cs.SetPermissionMode(strings.TrimSpace(args)); err != nil {
				return fmt.Errorf("set permission mode: %w", err)
			}
			fmt.Printf("Permission mode set to %s\n", args)
			return nil
		},
	})

	r.Register(Command{
		Name:            "plan",
		Description:     "Toggle or inspect planning mode",
		ArgumentHint:    "[on|off]",
		ResumeSupported: true,
		Category:        CategoryConfig,
		Handler: func(args string, loop interface{}) error {
			type planToggler interface {
				PlanMode() bool
				SetPlanMode(on bool)
			}
			pt, ok := loop.(planToggler)
			if !ok {
				fmt.Println("Plan mode not available in this context.")
				return nil
			}
			switch strings.TrimSpace(strings.ToLower(args)) {
			case "on":
				pt.SetPlanMode(true)
				fmt.Println("Plan mode enabled.")
			case "off":
				pt.SetPlanMode(false)
				fmt.Println("Plan mode disabled.")
			case "":
				if pt.PlanMode() {
					fmt.Println("Plan mode: on")
				} else {
					fmt.Println("Plan mode: off")
				}
			default:
				fmt.Println("Usage: /plan [on|off]")
			}
			return nil
		},
	})

	r.Register(Command{
		Name:            "compact",
		Description:     "Compact local session history",
		ResumeSupported: true,
		Category:        CategoryConfig,
		Handler: func(args string, loop interface{}) error {
			type compactor interface {
				CompactSession() error
			}
			c, ok := loop.(compactor)
			if !ok {
				fmt.Println("Compact not available in this context.")
				return nil
			}
			if err := c.CompactSession(); err != nil {
				return fmt.Errorf("compact: %w", err)
			}
			fmt.Println("Session compacted.")
			return nil
		},
	})
}

func relevantEnvVars() []string {
	return []string{
		"ANTHROPIC_API_KEY",
		"CLAUDE_CODE_USE_BEDROCK",
		"CLAUDE_CODE_USE_VERTEX",
		"CLAUDE_CODE_USE_FOUNDRY",
		"ANTHROPIC_BASE_URL",
		"HTTP_PROXY",
		"HTTPS_PROXY",
		"NO_PROXY",
	}
}
