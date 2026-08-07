package commands

import (
	"errors"
	"fmt"

	"github.com/cidx-org/cidx/v3/pkg/doctor"
	"github.com/urfave/cli/v2"
)

func doctorCommand() *cli.Command {
	return &cli.Command{
		Name:  "doctor",
		Usage: "Validate environment and diagnose common issues",
		Action: func(c *cli.Context) error {
			fmt.Println("Running environment checks...")
			fmt.Println()

			result := doctor.Run()

			fmt.Print(doctor.Format(result))
			fmt.Println()

			if result.Issues() > 0 {
				return errors.New(doctor.Summary(result))
			}

			fmt.Println(doctor.Summary(result))
			return nil
		},
	}
}
