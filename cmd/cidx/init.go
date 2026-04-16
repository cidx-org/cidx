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
			&cli.BoolFlag{
				Name:  "diff",
				Usage: "Show what detection would change in existing config",
			},
			&cli.BoolFlag{
				Name:  "update",
				Usage: "Re-detect and add new tools to existing config (additive only)",
			},
		},
		Action: func(c *cli.Context) error {
			filename := "cidx.toml"

			// --diff or --update: compare with existing config
			if c.Bool("diff") || c.Bool("update") {
				return initUpdate(filename, c.Bool("update"), c.Bool("no-ci"))
			}

			// Default: create new config
			if !c.Bool("force") {
				if _, err := os.Stat(filename); err == nil {
					return fmt.Errorf("file %s already exists (use --force to overwrite, --diff to compare, --update to add new tools)", filename)
				}
			}

			return initCreate(filename, c.Bool("force"), c.Bool("no-ci"))
		},
	}
}

// initCreate generates a new cidx.toml from scratch (original behavior).
func initCreate(filename string, force, noCi bool) error {
	fmt.Println("Detecting project...")
	detection := scaffold.Detect(".")
	printDetection(detection)

	content := scaffold.GenerateTOML(detection)

	if err := os.WriteFile(filename, []byte(content), 0644); err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}

	fmt.Printf("  ✓ Created %s\n", filename)

	if !noCi && detection.Remote != "" {
		ciPath, err := generateCIWorkflow(detection, force)
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
}

// initUpdate re-detects the project and shows/applies additive changes.
func initUpdate(filename string, apply, noCi bool) error {
	// Load existing config
	raw, err := os.ReadFile(filename)
	if err != nil {
		return fmt.Errorf("no existing %s found (use 'cidx init' to create one)", filename)
	}

	cfg, err := config.Load(filename)
	if err != nil {
		return fmt.Errorf("failed to parse %s: %w", filename, err)
	}

	// Re-detect project
	fmt.Println("Re-detecting project...")
	detection := scaffold.Detect(".")
	printDetection(detection)

	// Build existing phases map from config
	existingPhases := make(map[string][]string)
	for name, phase := range cfg.Phases {
		existingPhases[name] = phase.Containers
	}

	// Compare
	diff := scaffold.Compare(detection, existingPhases)

	fmt.Println("Changes:")
	fmt.Print(scaffold.FormatDiff(diff))

	if !diff.HasChanges() {
		return nil
	}

	if !apply {
		fmt.Println()
		fmt.Println("To apply: cidx init --update")
		return nil
	}

	// Apply changes
	updated := scaffold.UpdateTOML(string(raw), diff)

	if err := os.WriteFile(filename, []byte(updated), 0644); err != nil {
		return fmt.Errorf("failed to write %s: %w", filename, err)
	}

	fmt.Printf("\n  ✓ Updated %s\n", filename)

	// Optionally regenerate CI workflow
	if !noCi && detection.Remote != "" {
		fmt.Println()
		fmt.Println("Tip: run 'cidx generate' to update CI workflow if needed.")
	}

	return nil
}

// printDetection displays the detection results.
func printDetection(detection *scaffold.Detection) {
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
}

// generateCIWorkflow generates the CI workflow file for the detected platform.
func generateCIWorkflow(detection *scaffold.Detection, force bool) (string, error) {
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

	if !force {
		if _, err := os.Stat(ciPath); err == nil {
			return "", fmt.Errorf("%s already exists (use --force to overwrite)", ciPath)
		}
	}

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
