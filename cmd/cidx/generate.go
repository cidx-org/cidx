package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/cidx-org/cidx/pkg/config"
	"github.com/cidx-org/cidx/pkg/generate"
	"github.com/urfave/cli/v2"
)

var generateFlags = []cli.Flag{
	&cli.StringFlag{
		Name:    "output",
		Aliases: []string{"o"},
		Usage:   "Output file path (default: stdout)",
	},
	&cli.BoolFlag{
		Name:  "force",
		Usage: "Overwrite existing file without confirmation",
	},
}

func generateCommand() *cli.Command {
	return &cli.Command{
		Name:  "generate",
		Usage: "Generate CI platform configuration from cidx.toml",
		Subcommands: []*cli.Command{
			{
				Name:   "github",
				Usage:  "Generate GitHub Actions workflow",
				Flags:  generateFlags,
				Action: generateGitHubAction,
			},
			{
				Name:   "gitlab",
				Usage:  "Generate GitLab CI configuration",
				Flags:  generateFlags,
				Action: generateGitLabAction,
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

	return writeGeneratedOutput(c, output)
}

func generateGitLabAction(c *cli.Context) error {
	cfg, err := config.Load("cidx.toml")
	if err != nil {
		return fmt.Errorf("failed to load cidx.toml: %w", err)
	}

	output, err := generate.GitLab(cfg)
	if err != nil {
		return err
	}

	return writeGeneratedOutput(c, output)
}

func writeGeneratedOutput(c *cli.Context, output string) error {
	outputPath := c.String("output")
	if outputPath == "" {
		fmt.Print(output)
		return nil
	}

	if !c.Bool("force") {
		if _, err := os.Stat(outputPath); err == nil {
			return fmt.Errorf("file %s already exists (use --force to overwrite)", outputPath)
		}
	}

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
