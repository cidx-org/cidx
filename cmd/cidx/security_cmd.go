package main

import "github.com/urfave/cli/v2"

func securityCommand() *cli.Command {
	return &cli.Command{
		Name:  "security",
		Usage: "Vulnerability and registry management",
		Subcommands: []*cli.Command{
			vulnCommand(),
			registryCommand(),
		},
	}
}
