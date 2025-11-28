package main

import (
	"fmt"
	"os/exec"
	"runtime"
	"time"

	"github.com/cidx-org/cidx/pkg/branch"
	"github.com/cidx-org/cidx/pkg/config"
	"github.com/urfave/cli/v2"
)

func branchCommand() *cli.Command {
	return &cli.Command{
		Name:  "branch",
		Usage: "Manage Git branches",
		Subcommands: []*cli.Command{
			branchListCommand(),
			branchCleanupCommand(),
			branchPRCommand(),
		},
	}
}

func branchListCommand() *cli.Command {
	return &cli.Command{
		Name:  "list",
		Usage: "List branches with status information",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:    "all",
				Aliases: []string{"a"},
				Usage:   "Include remote-only branches",
			},
			&cli.BoolFlag{
				Name:  "mine",
				Usage: "Only show branches by current user",
			},
			&cli.BoolFlag{
				Name:  "stale",
				Usage: "Only show stale branches",
			},
			&cli.BoolFlag{
				Name:  "merged",
				Usage: "Only show merged branches",
			},
			&cli.BoolFlag{
				Name:  "orphan",
				Usage: "Only show orphan branches (PR closed without merge)",
			},
			&cli.StringFlag{
				Name:  "author",
				Usage: "Filter by author email/name",
			},
			&cli.IntFlag{
				Name:  "stale-days",
				Usage: "Days threshold for stale branches",
				Value: 30,
			},
			&cli.BoolFlag{
				Name:  "json",
				Usage: "Output as JSON",
			},
		},
		Action: func(c *cli.Context) error {
			// Load config for branch settings
			cfg, _ := config.Load("cidx.toml")

			// Build branch config from loaded config or defaults
			branchCfg := branch.Config{
				StaleDays: c.Int("stale-days"),
				Protected: []string{"main", "master", "develop"},
			}

			// Override with config file if present
			if cfg != nil {
				if cfg.Branch.StaleDays > 0 {
					branchCfg.StaleDays = cfg.Branch.StaleDays
				}
				if len(cfg.Branch.Protected) > 0 {
					branchCfg.Protected = cfg.Branch.Protected
				}
				branchCfg.NamingPattern = cfg.Branch.NamingPattern
				branchCfg.AutoCleanup = cfg.Branch.AutoCleanup
			}

			// Create manager
			manager := branch.NewManager(branchCfg)

			// Build list options
			opts := branch.ListOptions{
				All:       c.Bool("all"),
				Mine:      c.Bool("mine"),
				Stale:     c.Bool("stale"),
				Merged:    c.Bool("merged"),
				Orphan:    c.Bool("orphan"),
				Author:    c.String("author"),
				StaleDays: branchCfg.StaleDays,
				JSON:      c.Bool("json"),
			}

			// List branches
			result, err := manager.List(opts)
			if err != nil {
				return fmt.Errorf("failed to list branches: %w", err)
			}

			// Format and print output
			output := branch.FormatList(result, opts.JSON)
			fmt.Print(output)

			return nil
		},
	}
}

func branchCleanupCommand() *cli.Command {
	return &cli.Command{
		Name:  "cleanup",
		Usage: "Delete merged branches (local and remote)",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:    "execute",
				Aliases: []string{"x"},
				Usage:   "Actually delete branches (default is dry-run)",
			},
			&cli.BoolFlag{
				Name:  "stale",
				Usage: "Also delete stale branches (inactive > N days)",
			},
			&cli.BoolFlag{
				Name:  "orphan",
				Usage: "Also delete orphan branches (PR closed without merge)",
			},
			&cli.BoolFlag{
				Name:    "force",
				Aliases: []string{"f"},
				Usage:   "Force delete even if not fully merged",
			},
		},
		Action: func(c *cli.Context) error {
			// Load config for branch settings
			cfg, _ := config.Load("cidx.toml")

			// Build branch config from loaded config or defaults
			branchCfg := branch.Config{
				StaleDays: 30,
				Protected: []string{"main", "master", "develop"},
			}

			// Override with config file if present
			if cfg != nil {
				if cfg.Branch.StaleDays > 0 {
					branchCfg.StaleDays = cfg.Branch.StaleDays
				}
				if len(cfg.Branch.Protected) > 0 {
					branchCfg.Protected = cfg.Branch.Protected
				}
			}

			// Create manager
			manager := branch.NewManager(branchCfg)

			// Build cleanup options (dry-run by default, --execute to actually delete)
			dryRun := !c.Bool("execute")
			opts := branch.CleanupOptions{
				DryRun:        dryRun,
				IncludeStale:  c.Bool("stale"),
				IncludeOrphan: c.Bool("orphan"),
				Force:         c.Bool("force"),
			}

			// Run cleanup
			result, err := manager.Cleanup(opts)
			if err != nil {
				return fmt.Errorf("failed to cleanup branches: %w", err)
			}

			// Format and print output
			output := branch.FormatCleanup(result, dryRun)
			fmt.Print(output)

			return nil
		},
	}
}

func branchPRCommand() *cli.Command {
	return &cli.Command{
		Name:      "pr",
		Usage:     "Show PR status for current branch",
		ArgsUsage: "[branch]",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:    "watch",
				Aliases: []string{"w"},
				Usage:   "Watch checks until they complete",
			},
			&cli.BoolFlag{
				Name:    "open",
				Aliases: []string{"o"},
				Usage:   "Open PR in browser",
			},
		},
		Action: func(c *cli.Context) error {
			// Get branch name (default to current)
			branchName := c.Args().First()
			if branchName == "" {
				var err error
				branchName, err = branch.GetCurrentBranch()
				if err != nil {
					return fmt.Errorf("failed to get current branch: %w", err)
				}
			}

			// Load config for branch settings
			cfg, _ := config.Load("cidx.toml")

			// Build branch config
			branchCfg := branch.Config{
				Protected: []string{"main", "master", "develop"},
			}
			if cfg != nil && len(cfg.Branch.Protected) > 0 {
				branchCfg.Protected = cfg.Branch.Protected
			}

			// Create manager
			manager := branch.NewManager(branchCfg)

			// Get PR info
			info, err := manager.GetPRInfo(branchName)
			if err != nil {
				return fmt.Errorf("no PR found for branch '%s': %w", branchName, err)
			}

			// Open in browser if requested
			if c.Bool("open") {
				if err := openBrowser(info.URL); err != nil {
					fmt.Printf("Failed to open browser: %v\n", err)
					fmt.Printf("URL: %s\n", info.URL)
				}
				return nil
			}

			// Watch mode
			if c.Bool("watch") {
				return watchPRChecks(manager, branchName, info)
			}

			// Format and print output
			output := branch.FormatPRInfo(info)
			fmt.Print(output)

			return nil
		},
	}
}

// watchPRChecks watches PR checks until they complete
func watchPRChecks(manager *branch.Manager, branchName string, initialInfo *branch.PRInfo) error {
	// ANSI codes for terminal control
	const (
		clearLine  = "\033[2K"
		moveUp     = "\033[1A"
		hideCursor = "\033[?25l"
		showCursor = "\033[?25h"
	)

	// Hide cursor during watch
	fmt.Print(hideCursor)
	defer fmt.Print(showCursor)

	// Print initial header
	fmt.Printf("\n\033[1m#%d\033[0m %s\n", initialInfo.Number, initialInfo.Title)
	fmt.Printf("\033[2m%s\033[0m\n\n", initialInfo.URL)
	fmt.Printf("Watching checks for \033[36m%s\033[0m... (Ctrl+C to stop)\n\n", branchName)

	// Equalizer style spinner with varying bar heights
	spinnerFrames := []string{
		"▁▂▃▄▅▆▇█", "▂▃▄▅▆▇█▇", "▃▄▅▆▇█▇▆", "▄▅▆▇█▇▆▅",
		"▅▆▇█▇▆▅▄", "▆▇█▇▆▅▄▃", "▇█▇▆▅▄▃▂", "█▇▆▅▄▃▂▁",
		"▇▆▅▄▃▂▁▂", "▆▅▄▃▂▁▂▃", "▅▄▃▂▁▂▃▄", "▄▃▂▁▂▃▄▅",
		"▃▂▁▂▃▄▅▆", "▂▁▂▃▄▅▆▇",
	}

	// Current state (updated by API polling)
	currentInfo := initialInfo
	done := make(chan struct{})
	firstPrint := true

	// Spinner animation goroutine (runs every 100ms)
	go func() {
		frame := 0
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()

		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				info := currentInfo
				if info == nil || info.Checks == nil {
					continue
				}

				// Don't animate if checks are complete
				if info.Checks.Status != "pending" {
					continue
				}

				// Build and display spinner line
				checksColor := "\033[33m" // yellow for pending
				checksIcon := spinnerFrames[frame%len(spinnerFrames)]

				statusLine := fmt.Sprintf("%s%s %d/%d checks passed\033[0m",
					checksColor, checksIcon, info.Checks.Success, info.Checks.Total)

				if info.Checks.Pending > 0 {
					statusLine += fmt.Sprintf(" (%d pending)", info.Checks.Pending)
				}
				if info.Checks.Failure > 0 {
					statusLine += fmt.Sprintf(" (\033[31m%d failed\033[0m)", info.Checks.Failure)
				}

				// Update display
				if !firstPrint {
					fmt.Printf("%s%s", moveUp, clearLine)
				}
				fmt.Printf("  %s\n", statusLine)
				firstPrint = false
				frame++
			}
		}
	}()

	// API polling loop (every 3 seconds)
	pollTicker := time.NewTicker(3 * time.Second)
	defer pollTicker.Stop()

	for {
		info, err := manager.GetPRInfo(branchName)
		if err != nil {
			close(done)
			return fmt.Errorf("failed to get PR info: %w", err)
		}
		currentInfo = info

		// Check if we're done
		if info.Checks != nil && info.Checks.Pending == 0 {
			close(done)
			time.Sleep(150 * time.Millisecond) // Let spinner goroutine exit

			// Final status display
			checksColor := "\033[32m" // green for success
			checksIcon := "✓"
			if info.Checks.Status == "failure" {
				checksColor = "\033[31m" // red
				checksIcon = "✗"
			}

			statusLine := fmt.Sprintf("%s%s %d/%d checks passed\033[0m",
				checksColor, checksIcon, info.Checks.Success, info.Checks.Total)
			if info.Checks.Failure > 0 {
				statusLine += fmt.Sprintf(" (\033[31m%d failed\033[0m)", info.Checks.Failure)
			}

			if !firstPrint {
				fmt.Printf("%s%s", moveUp, clearLine)
			}
			fmt.Printf("  %s\n", statusLine)

			fmt.Println()
			switch info.Checks.Status {
			case "success":
				fmt.Printf("\033[32m✓ All checks passed!\033[0m\n")
				if info.Mergeable {
					fmt.Printf("\033[2mReady to merge: cidx pr merge\033[0m\n")
				}
			case "failure":
				fmt.Printf("\033[31m✗ Some checks failed\033[0m\n")
			}
			fmt.Println()
			return nil
		}

		<-pollTicker.C
	}
}

// openBrowser opens a URL in the default browser
func openBrowser(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", url)
	default: // linux, etc.
		cmd = exec.Command("xdg-open", url)
	}
	return cmd.Start()
}
