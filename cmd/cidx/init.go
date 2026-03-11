package main

import (
	"fmt"
	"os"

	"github.com/urfave/cli/v2"
)

func initCommand() *cli.Command {
	return &cli.Command{
		Name:  "init",
		Usage: "Initialize a new cidx configuration",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "format",
				Aliases: []string{"f"},
				Usage:   "Config format (currently only toml is supported)",
				Value:   "toml",
			},
		},
		Action: func(c *cli.Context) error {
			format := c.String("format")
			var filename, content string

			switch format {
			case "toml":
				filename = "cidx.toml"
				content = defaultTOMLConfig
			default:
				return fmt.Errorf("unsupported format: %s (only toml is currently supported)", format)
			}

			// Check if file already exists
			if _, err := os.Stat(filename); err == nil {
				return fmt.Errorf("file %s already exists", filename)
			}

			// Write config file
			if err := os.WriteFile(filename, []byte(content), 0644); err != nil {
				return fmt.Errorf("failed to write config: %w", err)
			}

			fmt.Printf("Created %s\n", filename)
			fmt.Println("\nEdit the file to enable the tools you need.")
			fmt.Println("Run 'cidx preset list' to see available tools.")
			fmt.Println("Run 'cidx validate' to check your configuration.")
			fmt.Println("Run 'cidx run ci' or 'cidx run <tool>' to execute.")

			return nil
		},
	}
}

const defaultTOMLConfig = `# CIDX Configuration
#
# Workspace: By default, CIDX uses the current directory as the workspace.
# All tools will be executed in the directory where you run the command.

# Version Pinning (Optional but recommended for CI consistency)
# required_version = "1.2.3"

[security]
containers = ["trivy", "gitleaks"]

[code]
containers = ["prettier"]

[pipelines.ci]
phases = ["security", "code"]

# Optional: Override tool settings
# [containers.prettier]
# write = true

# [containers.trivy]
# severity = "HIGH,CRITICAL"
# exit_code = 1
`
