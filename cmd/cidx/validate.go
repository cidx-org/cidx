package main

import (
	"fmt"

	"github.com/cidx-org/cidx/pkg/config"
	"github.com/urfave/cli/v2"
)

func validateCommand() *cli.Command {
	return &cli.Command{
		Name:  "validate",
		Usage: "Validate configuration file",
		Action: func(c *cli.Context) error {
			configPath := c.String("config")

			// Find config
			if configPath == "" {
				found, err := config.FindConfig()
				if err != nil {
					return err
				}
				configPath = found
			}

			// Load config
			cfg, err := config.Load(configPath)
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}

			// Validate
			result := config.Validate(cfg)

			fmt.Printf("Validating: %s\n\n", configPath)

			if len(result.Errors) > 0 {
				fmt.Println("Errors:")
				for _, err := range result.Errors {
					fmt.Printf("  ✗ %s\n", err)
				}
				fmt.Println()
			}

			if len(result.Warnings) > 0 {
				fmt.Println("Warnings:")
				for _, warn := range result.Warnings {
					fmt.Printf("  ⚠ %s\n", warn)
				}
				fmt.Println()
			}

			if result.Valid {
				fmt.Println("✓ Configuration is valid")
				return nil
			}

			return fmt.Errorf("configuration validation failed")
		},
	}
}
