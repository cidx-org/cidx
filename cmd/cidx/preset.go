package main

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/cidx-org/cidx/pkg/presets"
	"github.com/urfave/cli/v2"
)

func presetCommand() *cli.Command {
	return &cli.Command{
		Name:  "preset",
		Usage: "Manage built-in container presets",
		Subcommands: []*cli.Command{
			presetListCommand(),
			presetInfoCommand(),
			presetShowCommand(),
			presetExportCommand(),
			presetSearchCommand(),
			presetCheckUpdatesCommand(),
			presetScanCommand(),
			presetImagesCommand(),
			presetScanTargetsCommand(),
			presetAuditCommand(),
		},
	}
}

// presetListCommand lists all available presets grouped by phase
func presetListCommand() *cli.Command {
	return &cli.Command{
		Name:  "list",
		Usage: "List available presets grouped by phase",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "phase",
				Aliases: []string{"p"},
				Usage:   "Filter by phase (security, code, test, build, docker, release)",
			},
			&cli.BoolFlag{
				Name:  "json",
				Usage: "Output as JSON",
			},
		},
		Action: func(c *cli.Context) error {
			phaseFilter := c.String("phase")
			phases := presets.GroupByPhase()

			if c.Bool("json") {
				return printPresetsJSON(phases, phaseFilter)
			}

			fmt.Println("Available presets:")
			fmt.Println()

			// Get sorted phase names
			phaseNames := make([]string, 0, len(phases))
			for phase := range phases {
				if phaseFilter == "" || phase == phaseFilter {
					phaseNames = append(phaseNames, phase)
				}
			}
			sort.Strings(phaseNames)

			if len(phaseNames) == 0 && phaseFilter != "" {
				return fmt.Errorf("no presets found for phase '%s'", phaseFilter)
			}

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

// presetInfoCommand shows detailed information about a preset
func presetInfoCommand() *cli.Command {
	return &cli.Command{
		Name:      "info",
		Usage:     "Show detailed information about a preset",
		ArgsUsage: "<preset>",
		Action: func(c *cli.Context) error {
			if c.NArg() != 1 {
				return fmt.Errorf("requires exactly one argument: preset name")
			}

			presetName := c.Args().First()
			preset, err := presets.Get(presetName)
			if err != nil {
				return err
			}

			fmt.Printf("Preset: %s\n", preset.Name)
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
				// #151: surface the mount contract explicitly so users
				// who override `workdir` know they must keep it under
				// the mount target (or override `volumes` too).
				fmt.Printf("Mount contract: workdir must be inside one of the volume mount targets.\n")
				fmt.Printf("                Override `volumes` if you change `workdir` to a different root.\n")
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

// presetShowCommand shows the raw TOML definition of a preset
func presetShowCommand() *cli.Command {
	return &cli.Command{
		Name:      "show",
		Usage:     "Show raw TOML definition of a preset",
		ArgsUsage: "<preset>",
		Action: func(c *cli.Context) error {
			if c.NArg() != 1 {
				return fmt.Errorf("requires exactly one argument: preset name")
			}

			presetName := c.Args().First()
			preset, err := presets.Get(presetName)
			if err != nil {
				return err
			}

			// Convert preset to TOML-friendly structure
			presetMap := map[string]presets.Preset{
				presetName: preset,
			}

			fmt.Printf("# Embedded preset: %s\n", presetName)
			fmt.Println()

			encoder := toml.NewEncoder(os.Stdout)
			encoder.Indent = ""
			return encoder.Encode(map[string]interface{}{
				"presets": presetMap,
			})
		},
	}
}

// presetExportCommand exports all presets to a file
func presetExportCommand() *cli.Command {
	return &cli.Command{
		Name:  "export",
		Usage: "Export all embedded presets to a TOML file",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "output",
				Aliases: []string{"o"},
				Usage:   "Output file path (default: stdout)",
			},
			&cli.StringFlag{
				Name:    "phase",
				Aliases: []string{"p"},
				Usage:   "Export only presets from specific phase",
			},
		},
		Action: func(c *cli.Context) error {
			phaseFilter := c.String("phase")
			outputPath := c.String("output")

			// Collect presets
			allPresets := make(map[string]presets.Preset)
			for _, name := range presets.List() {
				preset, _ := presets.Get(name)
				if phaseFilter == "" || preset.Phase == phaseFilter {
					allPresets[name] = preset
				}
			}

			if len(allPresets) == 0 {
				return fmt.Errorf("no presets found")
			}

			// Determine output destination
			var output *os.File
			if outputPath == "" {
				output = os.Stdout
			} else {
				var err error
				output, err = os.Create(outputPath)
				if err != nil {
					return fmt.Errorf("failed to create output file: %w", err)
				}
				defer func() { _ = output.Close() }()
			}

			// Write header
			_, _ = fmt.Fprintln(output, "# CIDX Embedded Presets Export")
			_, _ = fmt.Fprintln(output, "# Generated from compiled binary")
			_, _ = fmt.Fprintf(output, "# Total presets: %d\n", len(allPresets))
			if phaseFilter != "" {
				_, _ = fmt.Fprintf(output, "# Filtered by phase: %s\n", phaseFilter)
			}
			_, _ = fmt.Fprintln(output)

			// Encode presets
			encoder := toml.NewEncoder(output)
			encoder.Indent = ""
			err := encoder.Encode(map[string]interface{}{
				"presets": allPresets,
			})

			if outputPath != "" && err == nil {
				fmt.Fprintf(os.Stderr, "Exported %d presets to %s\n", len(allPresets), outputPath)
			}

			return err
		},
	}
}

// presetSearchCommand searches for presets matching a keyword
func presetSearchCommand() *cli.Command {
	return &cli.Command{
		Name:      "search",
		Usage:     "Search presets by name, image, or description",
		ArgsUsage: "<keyword>",
		Action: func(c *cli.Context) error {
			if c.NArg() != 1 {
				return fmt.Errorf("requires exactly one argument: search keyword")
			}

			keyword := strings.ToLower(c.Args().First())
			var matches []presets.Preset

			for _, name := range presets.List() {
				preset, _ := presets.Get(name)

				// Search in name, image, command, phase
				searchable := strings.ToLower(fmt.Sprintf("%s %s %s %s",
					preset.Name, preset.Image, preset.Command, preset.Phase))

				// Also search in option descriptions
				for _, opt := range preset.Options {
					searchable += " " + strings.ToLower(opt.Description)
				}

				if strings.Contains(searchable, keyword) {
					matches = append(matches, preset)
				}
			}

			if len(matches) == 0 {
				fmt.Printf("No presets found matching '%s'\n", keyword)
				return nil
			}

			fmt.Printf("Found %d preset(s) matching '%s':\n\n", len(matches), keyword)

			// Sort by name
			sort.Slice(matches, func(i, j int) bool {
				return matches[i].Name < matches[j].Name
			})

			for _, preset := range matches {
				fmt.Printf("  \033[1m%s\033[0m (%s)\n", preset.Name, preset.Phase)
				fmt.Printf("    Image: %s\n", preset.Image)
				fmt.Printf("    Command: %s\n", truncate(preset.Command, 60))
				fmt.Println()
			}

			return nil
		},
	}
}

// printPresetsJSON outputs presets as JSON
func printPresetsJSON(phases map[string][]string, phaseFilter string) error {
	fmt.Println("{")

	phaseNames := make([]string, 0, len(phases))
	for phase := range phases {
		if phaseFilter == "" || phase == phaseFilter {
			phaseNames = append(phaseNames, phase)
		}
	}
	sort.Strings(phaseNames)

	for i, phase := range phaseNames {
		tools := phases[phase]
		sort.Strings(tools)

		fmt.Printf("  \"%s\": [", phase)
		for j, tool := range tools {
			fmt.Printf("\"%s\"", tool)
			if j < len(tools)-1 {
				fmt.Print(", ")
			}
		}
		fmt.Print("]")
		if i < len(phaseNames)-1 {
			fmt.Print(",")
		}
		fmt.Println()
	}

	fmt.Println("}")
	return nil
}

// truncate shortens a string to max length
func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}
