package main

import (
	"fmt"

	"github.com/urfave/cli/v2"
)

func aboutCommand() *cli.Command {
	return &cli.Command{
		Name:  "about",
		Usage: "Show project information and credits",
		Action: func(c *cli.Context) error {
			printAbout()
			return nil
		},
	}
}

func printAbout() {
	const about = `
┌─────────────────────────────────────────────────────────────────┐
│                            CIDX                                 │
│           CI with Declarative eXecution                         │
└─────────────────────────────────────────────────────────────────┘

  Version:    %s

  Created by: Arcker (Yoan Roblet)
              https://github.com/arcker

─────────────────────────────────────────────────────────────────

  What is CIDX?

  CIDX is two tools in one:

  1. CI/CD Abstraction Layer
     Write once, run everywhere. Define containers in cidx.toml,
     run them locally or in any CI platform.

  2. Git Workflow Facilitator
     Human-friendly commands that simplify common git workflows
     like PR creation, branch management, and releases.

─────────────────────────────────────────────────────────────────

  Core Principles

  • Convention over Configuration
    Declare what to run, CIDX knows how.

  • Minimal Setup
    One config file. One binary.

  • Transparency
    Dry-run shows exactly what will execute.

─────────────────────────────────────────────────────────────────

  License:    MIT
  Repository: https://github.com/cidx-org/cidx

`
	fmt.Printf(about, Version)
}
