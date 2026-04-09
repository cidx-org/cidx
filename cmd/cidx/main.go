package main

import (
	"fmt"
	"os"

	"github.com/urfave/cli/v2"
)

// Version is set via ldflags during build
var Version = "dev"

func main() {
	app := &cli.App{
		Name:                   "cidx",
		Usage:                  "CI with Declarative eXecution - Integrate any project in two commands",
		Version:                Version,
		UseShortOptionHandling: true,
		Commands: []*cli.Command{
			// Core — the product story
			initCommand(),
			runCommand(),
			generateCommand(),
			validateCommand(),
			checkCommand,
			doctorCommand(),
			presetCommand(),
			statusCommand(),

			// Secondary — namespaced capabilities
			repoCommand(),
			releaseCommand(),
			securityCommand(),

			// Utility
			cleanupCommand(),
			aboutCommand(),

			// Hidden aliases for dogfooding convenience
			{
				Name:   "pr",
				Usage:  "Alias for 'repo pr'",
				Hidden: true,
				Action: func(c *cli.Context) error {
					return c.App.Run(append([]string{c.App.Name, "repo", "pr"}, c.Args().Slice()...))
				},
				Subcommands: prCommand().Subcommands,
			},
			{
				Name:   "cpw",
				Usage:  "Alias for 'repo cpw'",
				Hidden: true,
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:     "message",
						Aliases:  []string{"m"},
						Usage:    "Commit message",
						Required: true,
					},
				},
				Action: commitPushWatchAction,
			},

			// Deprecated — will be removed
			{
				Name:   "action",
				Usage:  "Deprecated: use 'repo', 'release', or 'security' instead",
				Hidden: true,
				Subcommands: []*cli.Command{
					cpwCommand(),
					prCommand(),
					{
						Name:        "tag",
						Usage:       "Deprecated: use 'release tag' instead",
						Subcommands: releaseTagCommand().Subcommands,
					},
					{
						Name:        "release",
						Usage:       "Deprecated: use 'release' instead",
						Subcommands: releaseCommand().Subcommands,
					},
					{
						Name:        "artifact",
						Usage:       "Deprecated: use 'repo artifact' instead",
						Subcommands: artifactCommand().Subcommands,
					},
				},
			},
			{
				Name:   "demo",
				Hidden: true,
				Usage:  "Demo mode",
				Subcommands: demoCommand().Subcommands,
			},
		},
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "config",
				Aliases: []string{"c"},
				Usage:   "Path to config file",
				Value:   "",
			},
			&cli.BoolFlag{
				Name:  "verbose",
				Usage: "Enable verbose output",
			},
		},
	}

	if err := app.Run(os.Args); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
