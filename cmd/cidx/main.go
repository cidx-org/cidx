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
		Name:    "cidx",
		Usage:   "CI with Declarative eXecution - Ultra-declarative DevSecOps pipeline runner",
		Version: Version,
		Commands: []*cli.Command{
			runCommand(),
			listCommand(),
			infoCommand(),
			validateCommand(),
			initCommand(),
			actionCommand(),
			checkCommand,
			branchCommand(),
			demoCommand(),
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
