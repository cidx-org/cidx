package main

import (
	"fmt"
	"time"

	"github.com/urfave/cli/v2"
)

func demoCommand() *cli.Command {
	return &cli.Command{
		Name:  "demo",
		Usage: "Demo commands for testing and fun",
		Subcommands: []*cli.Command{
			demoSpinnerCommand(),
		},
	}
}

func demoSpinnerCommand() *cli.Command {
	return &cli.Command{
		Name:  "spinner",
		Usage: "Show the equalizer spinner animation",
		Flags: []cli.Flag{
			&cli.IntFlag{
				Name:    "duration",
				Aliases: []string{"d"},
				Usage:   "Duration in seconds",
				Value:   5,
			},
		},
		Action: func(c *cli.Context) error {
			duration := c.Int("duration")
			return runSpinnerDemo(duration)
		},
	}
}

func runSpinnerDemo(durationSec int) error {
	const (
		clearLine  = "\033[2K"
		moveUp     = "\033[1A"
		hideCursor = "\033[?25l"
		showCursor = "\033[?25h"
		colorYellow = "\033[33m"
		colorReset = "\033[0m"
	)

	// Equalizer style spinner with varying bar heights
	spinnerFrames := []string{
		"▁▂▃▄▅▆▇█", "▂▃▄▅▆▇█▇", "▃▄▅▆▇█▇▆", "▄▅▆▇█▇▆▅",
		"▅▆▇█▇▆▅▄", "▆▇█▇▆▅▄▃", "▇█▇▆▅▄▃▂", "█▇▆▅▄▃▂▁",
		"▇▆▅▄▃▂▁▂", "▆▅▄▃▂▁▂▃", "▅▄▃▂▁▂▃▄", "▄▃▂▁▂▃▄▅",
		"▃▂▁▂▃▄▅▆", "▂▁▂▃▄▅▆▇",
	}

	fmt.Print(hideCursor)
	defer fmt.Print(showCursor)

	fmt.Println()
	fmt.Println("  🎵 Equalizer Spinner Demo")
	fmt.Println()

	frame := 0
	firstPrint := true
	endTime := time.Now().Add(time.Duration(durationSec) * time.Second)

	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for time.Now().Before(endTime) {
		<-ticker.C

		remaining := time.Until(endTime).Seconds()
		statusLine := fmt.Sprintf("%s%s%s  %.1fs remaining",
			colorYellow, spinnerFrames[frame%len(spinnerFrames)], colorReset, remaining)

		if !firstPrint {
			fmt.Printf("%s%s", moveUp, clearLine)
		}
		fmt.Printf("  %s\n", statusLine)
		firstPrint = false
		frame++
	}

	// Final message
	fmt.Printf("%s%s", moveUp, clearLine)
	fmt.Println("  ✓ Demo complete!")
	fmt.Println()

	return nil
}
