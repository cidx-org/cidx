package main

import (
	"fmt"

	"github.com/cidx-org/cidx/pkg/doctor"
	"github.com/urfave/cli/v2"
)

func doctorCommand() *cli.Command {
	return &cli.Command{
		Name:  "doctor",
		Usage: "Validate environment and diagnose common issues",
		Action: func(c *cli.Context) error {
			fmt.Println("Running environment checks...")
			fmt.Println()

			result := doctor.Run()

			for _, check := range result.Checks {
				var icon string
				switch check.Status {
				case doctor.StatusPass:
					icon = "\033[32m✓\033[0m"
				case doctor.StatusWarn:
					icon = "\033[33m⚠\033[0m"
				case doctor.StatusFail:
					icon = "\033[31m✗\033[0m"
				}

				fmt.Printf("  %s %-20s %s\n", icon, check.Name, check.Detail)
				if check.Suggestion != "" {
					fmt.Printf("    └─ %s\n", check.Suggestion)
				}
			}

			fmt.Println()

			issues := result.Issues()
			warnings := result.Warnings()

			if issues == 0 && warnings == 0 {
				fmt.Println("\033[32mAll checks passed.\033[0m")
				return nil
			}

			if warnings > 0 && issues == 0 {
				fmt.Printf("\033[33m%d warning(s)\033[0m, no issues.\n", warnings)
				return nil
			}

			return fmt.Errorf("%d issue(s) found", issues)
		},
	}
}
