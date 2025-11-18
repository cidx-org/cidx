package main

import (
	"fmt"
	"sort"

	"github.com/arcker/cidx/pkg/presets"
	"github.com/urfave/cli/v2"
)

func listCommand() *cli.Command {
	return &cli.Command{
		Name:  "list",
		Usage: "List available tools and presets",
		Action: func(c *cli.Context) error {
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
