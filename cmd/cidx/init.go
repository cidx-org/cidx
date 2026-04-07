package main

import (
	"fmt"
	"os"

	"github.com/cidx-org/cidx/pkg/scaffold"
	"github.com/urfave/cli/v2"
)

func initCommand() *cli.Command {
	return &cli.Command{
		Name:  "init",
		Usage: "Initialize a new cidx configuration (auto-detects project type)",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:  "force",
				Usage: "Overwrite existing cidx.toml",
			},
		},
		Action: func(c *cli.Context) error {
			filename := "cidx.toml"

			// Check if file already exists
			if !c.Bool("force") {
				if _, err := os.Stat(filename); err == nil {
					return fmt.Errorf("file %s already exists (use --force to overwrite)", filename)
				}
			}

			// Detect project
			fmt.Println("Detecting project...")
			detection := scaffold.Detect(".")

			if detection.HasGit {
				remote := detection.Remote
				if remote == "" {
					remote = "local"
				}
				fmt.Printf("  ✓ Git repository (%s)\n", remote)
			}

			if len(detection.Languages) > 0 {
				for _, lang := range detection.Languages {
					fmt.Printf("  ✓ %s project (%s found)\n", lang.Name, lang.Marker)
				}
			} else {
				fmt.Println("  ⚠ No language detected (using defaults)")
			}

			fmt.Println()

			// Generate config
			content := scaffold.GenerateTOML(detection)

			if err := os.WriteFile(filename, []byte(content), 0644); err != nil {
				return fmt.Errorf("failed to write config: %w", err)
			}

			fmt.Printf("Created %s\n\n", filename)
			fmt.Println("Next steps:")
			fmt.Println("  cidx validate              # check configuration")
			fmt.Println("  cidx run --dry-run ci       # preview what would run")
			fmt.Println("  cidx run ci                 # execute pipeline")

			switch detection.Remote {
			case "github":
				fmt.Println("  cidx generate github -o .github/workflows/cidx.yml")
			case "gitlab":
				fmt.Println("  cidx generate gitlab -o .gitlab-ci.yml")
			}

			return nil
		},
	}
}
