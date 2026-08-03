package main

import (
	"fmt"
	"os"

	"github.com/cidx-org/cidx/v2/pkg/executor"
	"github.com/cidx-org/cidx/v2/pkg/generate"
	"github.com/urfave/cli/v2"
)

// Version is set via ldflags during build
var Version = "dev"

func main() {
	// Resolve the version once, before any propagation: `go install` applies no
	// ldflags, so the real version comes from the embedded build info (#205).
	Version = resolveVersion(Version)

	// Propagate the build version to the executor package so created
	// containers carry the `cidx.version` label (issue #144).
	executor.Version = Version

	// Propagate the build version to the generate package so generated
	// workflows pin the bootstrap to the generating release (issue #163).
	generate.Version = Version

	if err := newApp().Run(os.Args); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

// newApp builds the command tree. It is a function, not an inline literal, so
// the tree can be walked outside of a run — `cidx validate` resolves the
// invocations it finds in the workflows against it (issue #239).
func newApp() *cli.App {
	app := buildApp()

	// Applied to the finished tree, not written into each command: every
	// command is covered, including the ones added after this line (issue #268).
	installFlagPlacementGuard(app)

	return app
}

func buildApp() *cli.App {
	return &cli.App{
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
			checkCommand(),
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
			{
				Name:   "workflow",
				Usage:  "Alias for 'repo workflow'",
				Hidden: true,
				Action: func(c *cli.Context) error {
					return c.App.Run(append([]string{c.App.Name, "repo", "workflow"}, c.Args().Slice()...))
				},
				Subcommands: workflowCommand().Subcommands,
			},

			// Deprecated — removed in v3.0.0 (issue #235). Every invocation
			// warns and names its exact replacement; see action_deprecated.go.
			{
				Name:   "action",
				Usage:  "Deprecated: use 'repo', 'release', or 'security' instead — removed in cidx " + actionRemovedIn,
				Hidden: true,
				Before: warnDeprecatedAction,
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
				Name:        "demo",
				Hidden:      true,
				Usage:       "Demo mode",
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
}
