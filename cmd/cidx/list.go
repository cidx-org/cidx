package main

import (
	"fmt"
	"sort"

	"github.com/cidx-org/cidx/pkg/presets"
	"github.com/urfave/cli/v2"
)

func listCommand() *cli.Command {
	return &cli.Command{
		Name:   "list",
		Usage:  "List available tools and presets (deprecated: use 'preset list')",
		Hidden: true, // Hide from help, but still works
		Action: func(c *cli.Context) error {
			fmt.Println("\033[33mNote: 'cidx list' is deprecated. Use 'cidx preset list' instead.\033[0m")
			fmt.Println()
			phases := presets.GroupByPhase()

			fmt.Println("Available tools:")
			fmt.Println()

			// Get sorted phase names
			phaseNames := make([]string, 0, len(phases))
			for phase := range phases {
				phaseNames = append(phaseNames, phase)
			}
			sort.Strings(phaseNames)

			// Display by phase
			for _, phase := range phaseNames {
				tools := phases[phase]
				sort.Strings(tools)

				fmt.Printf("  %s:\n", phase)
				for _, tool := range tools {
					fmt.Printf("    - %s\n", tool)
				}
				fmt.Println()
			}

			return nil
		},
	}
}
