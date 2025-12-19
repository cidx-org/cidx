package main

import (
	"fmt"

	"github.com/cidx-org/cidx/pkg/registry"
	"github.com/urfave/cli/v2"
)

func registryCommand() *cli.Command {
	return &cli.Command{
		Name:  "registry",
		Usage: "Manage Docker registry authentication",
		Description: `Interact with Docker registry authentication.

CIDX uses Docker Hardened Images (dhi.io) by default for maximum security.
DHI requires Docker Hub credentials to pull images.

Examples:
  cidx registry list              # Show configured registries
  cidx registry status dhi.io     # Check DHI authentication
  cidx registry login dhi.io      # Login to DHI (uses Docker Hub creds)
  cidx registry check             # Verify DHI is ready for CIDX`,
		Subcommands: []*cli.Command{
			registryListCommand(),
			registryStatusCommand(),
			registryLoginCommand(),
			registryLogoutCommand(),
			registryCheckCommand(),
		},
	}
}

func registryListCommand() *cli.Command {
	return &cli.Command{
		Name:  "list",
		Usage: "List configured Docker registries",
		Action: func(c *cli.Context) error {
			manager := registry.NewManager()

			registries, err := manager.List()
			if err != nil {
				return fmt.Errorf("failed to list registries: %w", err)
			}

			fmt.Print(registry.FormatList(registries))
			return nil
		},
	}
}

func registryStatusCommand() *cli.Command {
	return &cli.Command{
		Name:      "status",
		Usage:     "Check authentication status for a registry",
		ArgsUsage: "<registry>",
		Action: func(c *cli.Context) error {
			registryName := c.Args().First()
			if registryName == "" {
				return fmt.Errorf("registry name required\n\nUsage: cidx registry status <registry>\nExample: cidx registry status dhi.io")
			}

			manager := registry.NewManager()

			info, err := manager.Status(registryName)
			if err != nil {
				return fmt.Errorf("failed to check registry status: %w", err)
			}

			fmt.Print(registry.FormatStatus(info))
			return nil
		},
	}
}

func registryLoginCommand() *cli.Command {
	return &cli.Command{
		Name:      "login",
		Usage:     "Login to a Docker registry",
		ArgsUsage: "<registry>",
		Description: `Login to a Docker registry using docker login.

For DHI (dhi.io), use your Docker Hub credentials.
DHI is free and included with any Docker Hub account.

Examples:
  cidx registry login dhi.io
  cidx registry login ghcr.io`,
		Action: func(c *cli.Context) error {
			registryName := c.Args().First()
			if registryName == "" {
				return fmt.Errorf("registry name required\n\nUsage: cidx registry login <registry>\nExample: cidx registry login dhi.io")
			}

			manager := registry.NewManager()

			fmt.Printf("Logging in to %s...\n", registryName)
			if registryName == registry.DHIRegistry {
				fmt.Println("(Use your Docker Hub credentials)")
			}
			fmt.Println()

			if err := manager.Login(registryName); err != nil {
				return fmt.Errorf("login failed: %w", err)
			}

			// Verify the login worked
			info, err := manager.Status(registryName)
			if err == nil && info.Authenticated {
				fmt.Printf("\n\033[32m✓ Successfully logged in to %s\033[0m\n", registryName)
			}

			return nil
		},
	}
}

func registryLogoutCommand() *cli.Command {
	return &cli.Command{
		Name:      "logout",
		Usage:     "Logout from a Docker registry",
		ArgsUsage: "<registry>",
		Action: func(c *cli.Context) error {
			registryName := c.Args().First()
			if registryName == "" {
				return fmt.Errorf("registry name required\n\nUsage: cidx registry logout <registry>")
			}

			manager := registry.NewManager()

			if err := manager.Logout(registryName); err != nil {
				return fmt.Errorf("logout failed: %w", err)
			}

			fmt.Printf("\033[32m✓ Logged out from %s\033[0m\n", registryName)
			return nil
		},
	}
}

func registryCheckCommand() *cli.Command {
	return &cli.Command{
		Name:  "check",
		Usage: "Check if Docker Hardened Images (DHI) is ready",
		Description: `Verify that DHI authentication is configured for CIDX.

CIDX uses Docker Hardened Images (dhi.io) by default for presets.
This command checks if you can pull hardened images.

If not authenticated, run: cidx registry login dhi.io`,
		Action: func(c *cli.Context) error {
			manager := registry.NewManager()

			info, err := manager.CheckDHI()
			if err != nil {
				return fmt.Errorf("failed to check DHI status: %w", err)
			}

			fmt.Print(registry.FormatDHICheck(info))

			if !info.Authenticated {
				// Return exit code 1 if not authenticated (useful for scripts)
				return cli.Exit("", 1)
			}
			return nil
		},
	}
}
