package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/cidx-org/cidx/pkg/config"
	"github.com/cidx-org/cidx/pkg/generate"
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
				Usage: "Overwrite existing files",
			},
			&cli.BoolFlag{
				Name:  "no-ci",
				Usage: "Skip CI workflow generation",
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

			// Generate cidx.toml
			content := scaffold.GenerateTOML(detection)

			if err := os.WriteFile(filename, []byte(content), 0644); err != nil {
				return fmt.Errorf("failed to write config: %w", err)
			}

			fmt.Printf("  ✓ Created %s\n", filename)

			// Generate CI workflow if remote detected and not skipped
			if !c.Bool("no-ci") && detection.Remote != "" {
				ciPath, err := generateCIWorkflow(detection, c.Bool("force"))
				if err != nil {
					fmt.Printf("  ⚠ CI workflow: %v\n", err)
				} else if ciPath != "" {
					fmt.Printf("  ✓ Created %s\n", ciPath)
				}
			}

			fmt.Println()
			fmt.Println("Ready. Next steps:")
			fmt.Println("  cidx doctor                 # verify environment")
			fmt.Println("  cidx run --dry-run ci       # preview pipeline")
			fmt.Println("  cidx run ci                 # execute")

			return nil
		},
	}
}

// generateCIWorkflow generates the CI workflow file for the detected platform.
func generateCIWorkflow(detection *scaffold.Detection, force bool) (string, error) {
	// Load the config we just wrote
	cfg, err := config.Load("cidx.toml")
	if err != nil {
		return "", fmt.Errorf("failed to load generated config: %w", err)
	}

	var output, ciPath string

	switch detection.Remote {
	case "github":
		ciPath = ".github/workflows/cidx.yml"
		output, err = generate.GitHub(cfg)
	case "gitlab":
		ciPath = ".gitlab-ci.yml"
		output, err = generate.GitLab(cfg)
	default:
		return "", nil
	}

	if err != nil {
		return "", err
	}

	// Check if exists
	if !force {
		if _, err := os.Stat(ciPath); err == nil {
			return "", fmt.Errorf("%s already exists (use --force to overwrite)", ciPath)
		}
	}

	// Ensure directory
	dir := filepath.Dir(ciPath)
	if dir != "." {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return "", fmt.Errorf("failed to create %s: %w", dir, err)
		}
	}

	if err := os.WriteFile(ciPath, []byte(output), 0644); err != nil {
		return "", fmt.Errorf("failed to write %s: %w", ciPath, err)
	}

	return ciPath, nil
}
