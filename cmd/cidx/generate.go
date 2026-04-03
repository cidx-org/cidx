package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/cidx-org/cidx/pkg/config"
	"github.com/cidx-org/cidx/pkg/generate"
	"github.com/urfave/cli/v2"
)

func generateCommand() *cli.Command {
	return &cli.Command{
		Name:  "generate",
		Usage: "Generate CI platform configuration from cidx.toml",
		Subcommands: []*cli.Command{
			{
				Name:  "github",
				Usage: "Generate GitHub Actions workflow",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:    "output",
						Aliases: []string{"o"},
						Usage:   "Output file path (default: stdout)",
					},
					&cli.BoolFlag{
						Name:  "force",
						Usage: "Overwrite existing file without confirmation",
					},
				},
				Action: generateGitHubAction,
			},
		},
	}
}

func generateGitHubAction(c *cli.Context) error {
	cfg, err := config.Load("cidx.toml")
	if err != nil {
		return fmt.Errorf("failed to load cidx.toml: %w", err)
	}

	output, err := generate.GitHub(cfg)
	if err != nil {
		return err
	}

	outputPath := c.String("output")
	if outputPath == "" {
		fmt.Print(output)
		return nil
	}

	// Check if file exists
	if !c.Bool("force") {
		if _, err := os.Stat(outputPath); err == nil {
			return fmt.Errorf("file %s already exists (use --force to overwrite)", outputPath)
		}
	}

	// Ensure directory exists
	dir := filepath.Dir(outputPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory %s: %w", dir, err)
	}

	if err := os.WriteFile(outputPath, []byte(output), 0644); err != nil {
		return fmt.Errorf("failed to write %s: %w", outputPath, err)
	}

	fmt.Fprintf(os.Stderr, "Generated %s\n", outputPath)
	return nil
}
