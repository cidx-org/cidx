package main

import (
	"fmt"

	"github.com/cidx-org/cidx/pkg/presets"
	"github.com/urfave/cli/v2"
)

func infoCommand() *cli.Command {
	return &cli.Command{
		Name:      "info",
		Usage:     "Show detailed information about a tool (deprecated: use 'preset info')",
		ArgsUsage: "<tool>",
		Hidden:    true, // Hide from help, but still works
		Action: func(c *cli.Context) error {
			if c.NArg() != 1 {
				return fmt.Errorf("requires exactly one argument: tool name")
			}

			fmt.Println("\033[33mNote: 'cidx info' is deprecated. Use 'cidx preset info' instead.\033[0m")
			fmt.Println()

			toolName := c.Args().First()
			preset, err := presets.Get(toolName)
			if err != nil {
				return err
			}

			fmt.Printf("Tool: %s\n", preset.Name)
			fmt.Printf("Phase: %s\n", preset.Phase)
			fmt.Printf("Image: %s\n", preset.Image)
			fmt.Printf("Command: %s\n", preset.Command)
			fmt.Printf("Workdir: %s\n", preset.Workdir)
			fmt.Println()

			if len(preset.Volumes) > 0 {
				fmt.Println("Volumes:")
				for _, vol := range preset.Volumes {
					fmt.Printf("  - %s\n", vol)
				}
				fmt.Println()
			}

			if len(preset.Env) > 0 {
				fmt.Println("Environment:")
				for k, v := range preset.Env {
					fmt.Printf("  %s=%s\n", k, v)
				}
				fmt.Println()
			}

			if len(preset.ConfigFiles) > 0 {
				fmt.Println("Config files:")
				for _, file := range preset.ConfigFiles {
					fmt.Printf("  - %s\n", file)
				}
				fmt.Println()
			}

			if len(preset.Options) > 0 {
				fmt.Println("Options:")
				for name, opt := range preset.Options {
					fmt.Printf("  %s:\n", name)
					fmt.Printf("    Type: %s\n", opt.Type)
					fmt.Printf("    Default: %v\n", opt.Default)
					if opt.Description != "" {
						fmt.Printf("    Description: %s\n", opt.Description)
					}
					if opt.EnvVar != "" {
						fmt.Printf("    Environment: %s\n", opt.EnvVar)
					}
					if opt.CommandFlag != "" {
						fmt.Printf("    Flag: %s\n", opt.CommandFlag)
					}
				}
				fmt.Println()
			}

			return nil
		},
	}
}
