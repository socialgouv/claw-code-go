package commands

import (
	"fmt"
)

// UsageTracker provides session usage information for status commands.
type UsageTracker interface {
	ModelName() string
	TurnCount() int
	InputTokens() int
	OutputTokens() int
	EstimatedCostUSD() float64
}

// RegisterStatusCommands registers status-related slash commands.
func RegisterStatusCommands(r *Registry) {
	r.Register(Command{
		Name:            "status",
		Description:     "Show current session status",
		ResumeSupported: true,
		Category:        CategoryStatus,
		Handler: func(args string, loop interface{}) error {
			ut, ok := loop.(UsageTracker)
			if !ok {
				fmt.Println("Status not available in this context.")
				return nil
			}
			fmt.Printf("Model:         %s\n", ut.ModelName())
			fmt.Printf("Turns:         %d\n", ut.TurnCount())
			fmt.Printf("Input tokens:  %d\n", ut.InputTokens())
			fmt.Printf("Output tokens: %d\n", ut.OutputTokens())
			return nil
		},
	})

	r.Register(Command{
		Name:            "cost",
		Description:     "Show cumulative token usage for this session",
		ResumeSupported: true,
		Category:        CategoryStatus,
		Handler: func(args string, loop interface{}) error {
			ut, ok := loop.(UsageTracker)
			if !ok {
				fmt.Println("Cost tracking not available in this context.")
				return nil
			}
			fmt.Printf("Input tokens:  %d\n", ut.InputTokens())
			fmt.Printf("Output tokens: %d\n", ut.OutputTokens())
			fmt.Printf("Estimated cost: $%.4f\n", ut.EstimatedCostUSD())
			return nil
		},
	})

	r.Register(Command{
		Name:            "usage",
		Description:     "Show detailed API usage statistics",
		ResumeSupported: true,
		Category:        CategoryStatus,
		Handler: func(args string, loop interface{}) error {
			ut, ok := loop.(UsageTracker)
			if !ok {
				fmt.Println("Usage tracking not available in this context.")
				return nil
			}
			fmt.Printf("Session Usage Summary\n")
			fmt.Printf("  Model:         %s\n", ut.ModelName())
			fmt.Printf("  Turns:         %d\n", ut.TurnCount())
			fmt.Printf("  Input tokens:  %d\n", ut.InputTokens())
			fmt.Printf("  Output tokens: %d\n", ut.OutputTokens())
			fmt.Printf("  Estimated cost: $%.4f\n", ut.EstimatedCostUSD())
			return nil
		},
	})

	r.Register(Command{
		Name:            "version",
		Description:     "Show CLI version and build information",
		ResumeSupported: true,
		Category:        CategoryStatus,
		Handler: func(args string, loop interface{}) error {
			type versionProvider interface {
				Version() string
				Commit() string
			}
			if vp, ok := loop.(versionProvider); ok {
				fmt.Printf("Version: %s\n", vp.Version())
				fmt.Printf("Commit:  %s\n", vp.Commit())
			} else {
				fmt.Println("claw-code-go (version unknown)")
			}
			return nil
		},
	})
}
