package features

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/cucumber/godog"
	"gopkg.in/yaml.v3"
)

func RegisterRuntimeImageSteps(ctx *godog.ScenarioContext, tc *TestContext) {
	ctx.Given(`^the runtime image command "([^"]*)" fails$`, func(command string) {
		tc.Config["runtime_failure"] = command
	})
	ctx.When(`^the release workflow verifies the runtime image$`, func() error {
		source, err := os.ReadFile("../.github/workflows/release.yml")
		if err != nil {
			return err
		}
		var workflow struct {
			Jobs map[string]struct {
				Steps []auditWorkflowStep `yaml:"steps"`
			} `yaml:"jobs"`
		}
		if err := yaml.Unmarshal(source, &workflow); err != nil {
			return err
		}
		var script string
		for _, job := range workflow.Jobs {
			for _, step := range job.Steps {
				if step.Name == "Verify the published image runs" {
					script = step.Run
				}
			}
		}
		if script == "" {
			return fmt.Errorf("release image verification step is missing")
		}
		script = strings.ReplaceAll(script, "${{ github.repository }}", "cidx-org/cidx")
		script = strings.ReplaceAll(script, "${{ github.ref_name }}", "v-test")
		dir, err := tc.scenarioDir()
		if err != nil {
			return err
		}
		// Substitute only Docker: execute the actual release workflow shell and
		// validate its image, entrypoint and arguments at the process boundary.
		stub := `#!/bin/sh
set -eu
case "$*" in
 'run --rm ghcr.io/cidx-org/cidx:v-test --version') tool=cidx ;;
 'run --rm --entrypoint curl ghcr.io/cidx-org/cidx:v-test --version') tool=curl ;;
 *) exit 99 ;;
esac
printf '%s\n' "$tool" >> "$RUNTIME_CALLS"
test "$tool" != "$RUNTIME_FAILURE"
`
		if err := os.WriteFile(filepath.Join(dir, "docker"), []byte(stub), 0o700); err != nil {
			return err
		}
		calls := filepath.Join(dir, "runtime-calls")
		cmd := exec.Command("sh", "-e", "-c", script)
		cmd.Env = append(os.Environ(), "PATH="+dir+":"+os.Getenv("PATH"), "RUNTIME_CALLS="+calls, "RUNTIME_FAILURE="+tc.Config["runtime_failure"].(string))
		output, runErr := cmd.CombinedOutput()
		tc.ExitCode = 0
		if runErr != nil {
			if _, ok := runErr.(*exec.ExitError); !ok {
				return runErr
			}
			tc.ExitCode = 1
		}
		tc.Output = string(output)
		if tc.Config["runtime_failure"] == "none" {
			recorded, err := os.ReadFile(calls)
			if err != nil {
				return err
			}
			if string(recorded) != "cidx\ncurl\n" {
				return fmt.Errorf("expected both runtime checks, got %q", recorded)
			}
		}
		return nil
	})
	ctx.Then(`^runtime image verification should (succeed|fail)$`, func(result string) error {
		if (tc.ExitCode == 0) != (result == "succeed") {
			return fmt.Errorf("expected verification to %s, exit=%d: %s", result, tc.ExitCode, tc.Output)
		}
		return nil
	})
}
