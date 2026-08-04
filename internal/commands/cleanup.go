package commands

import (
	"context"
	"fmt"
	"strings"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/client"
	"github.com/urfave/cli/v2"
)

func cleanupCommand() *cli.Command {
	return &cli.Command{
		Name:  "cleanup",
		Usage: "Remove stopped CIDX containers to free resources",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:  "dry-run",
				Usage: "Show what would be removed without deleting",
			},
			&cli.BoolFlag{
				Name:    "all",
				Aliases: []string{"a"},
				Usage:   "Remove all CIDX containers (including running)",
			},
		},
		Action: func(c *cli.Context) error {
			ctx := context.Background()

			docker, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
			if err != nil {
				return fmt.Errorf("failed to connect to Docker: %w", err)
			}
			defer func() { _ = docker.Close() }()

			// Select by ownership label, not by name. Container names are
			// project-scoped (cidx_<project>_<tool>, #197) and containers
			// created by older cidx versions still use the legacy
			// cidx_<tool> form — both carry `managed-by=cidx`, so the label
			// collects every generation while never touching a container
			// that merely happens to be named like ours.
			filterArgs := filters.NewArgs()
			filterArgs.Add("label", "managed-by=cidx")

			listOpts := container.ListOptions{
				All:     true,
				Filters: filterArgs,
			}

			containers, err := docker.ContainerList(ctx, listOpts)
			if err != nil {
				return fmt.Errorf("failed to list containers: %w", err)
			}

			if len(containers) == 0 {
				fmt.Println("No CIDX containers found.")
				return nil
			}

			removeAll := c.Bool("all")
			dryRun := c.Bool("dry-run")
			removed := 0
			skipped := 0

			for _, ctr := range containers {
				name := strings.TrimPrefix(ctr.Names[0], "/")
				state := ctr.State

				// Skip running containers unless --all
				if state == "running" && !removeAll {
					fmt.Printf("  ⏭  %-40s %s (running, use --all to include)\n", name, ctr.Image)
					skipped++
					continue
				}

				if dryRun {
					fmt.Printf("  🗑  %-40s %s (%s) — would remove\n", name, ctr.Image, state)
				} else {
					if err := docker.ContainerRemove(ctx, ctr.ID, container.RemoveOptions{Force: removeAll}); err != nil {
						fmt.Printf("  ✗  %-40s failed: %v\n", name, err)
						continue
					}
					fmt.Printf("  ✓  %-40s %s removed\n", name, ctr.Image)
				}
				removed++
			}

			fmt.Println()
			if dryRun {
				fmt.Printf("Would remove %d container(s)", removed)
			} else {
				fmt.Printf("Removed %d container(s)", removed)
			}
			if skipped > 0 {
				fmt.Printf(", skipped %d running", skipped)
			}
			fmt.Println()

			return nil
		},
	}
}
