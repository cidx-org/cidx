// Package doctor validates the CIDX runtime environment.
package doctor

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/cidx-org/cidx/v2/pkg/config"
	"github.com/cidx-org/cidx/v2/pkg/executor"
)

// Package variables so tests can stub environment probes.
var (
	commandVersion = getCommandVersion
	podmanUsable   = executor.PodmanUsable
)

// Status represents the result of a single check.
type Status int

const (
	StatusPass Status = iota
	StatusWarn
	StatusFail
)

// Check is a single diagnostic result.
type Check struct {
	Name       string
	Status     Status
	Detail     string
	Suggestion string
}

// Result holds all diagnostic checks.
type Result struct {
	Checks []Check
}

// Passed returns true if all checks passed (no failures).
func (r *Result) Passed() bool {
	for _, c := range r.Checks {
		if c.Status == StatusFail {
			return false
		}
	}
	return true
}

// Issues returns the count of failed checks.
func (r *Result) Issues() int {
	n := 0
	for _, c := range r.Checks {
		if c.Status == StatusFail {
			n++
		}
	}
	return n
}

// Warnings returns the count of warning checks.
func (r *Result) Warnings() int {
	n := 0
	for _, c := range r.Checks {
		if c.Status == StatusWarn {
			n++
		}
	}
	return n
}

// Run executes all diagnostic checks.
func Run() *Result {
	r := &Result{}
	r.Checks = append(r.Checks, checkContainerRuntime())
	r.Checks = append(r.Checks, checkGitRepo())
	r.Checks = append(r.Checks, checkConfigFile())
	return r
}

// Format renders the checks the way `cidx doctor` prints them: one line per
// check, with its remedy attached underneath when there is one.
func Format(result *Result) string {
	var b strings.Builder
	for _, check := range result.Checks {
		var icon string
		switch check.Status {
		case StatusPass:
			icon = "\033[32m✓\033[0m"
		case StatusWarn:
			icon = "\033[33m⚠\033[0m"
		case StatusFail:
			icon = "\033[31m✗\033[0m"
		}

		fmt.Fprintf(&b, "  %s %-20s %s\n", icon, check.Name, check.Detail)
		if check.Suggestion != "" {
			fmt.Fprintf(&b, "    └─ %s\n", check.Suggestion)
		}
	}
	return b.String()
}

// Summary is the verdict `cidx doctor` closes with. A run with failures ends as
// the command's error, so that branch carries no colour of its own — the CLI
// renders errors.
func Summary(result *Result) string {
	issues, warnings := result.Issues(), result.Warnings()
	switch {
	case issues == 0 && warnings == 0:
		return "\033[32mAll checks passed.\033[0m"
	case issues == 0:
		return fmt.Sprintf("\033[33m%d warning(s)\033[0m, no issues.", warnings)
	default:
		return fmt.Sprintf("%d issue(s) found", issues)
	}
}

// checkContainerRuntime verifies a container runtime cidx can actually use.
// A runtime only passes when the executor would accept it: Docker's daemon
// responding, or Podman's Docker-compatible API socket responding (#190).
func checkContainerRuntime() Check {
	check := Check{Name: "Container runtime"}

	// Try Docker first
	if version, err := commandVersion("docker", "version", "--format", "{{.Server.Version}}"); err == nil {
		check.Status = StatusPass
		check.Detail = fmt.Sprintf("Docker %s", version)
		return check
	}

	// Try Podman — the CLI alone is not enough, cidx needs its API socket
	if version, err := commandVersion("podman", "version", "--format", "{{.Version}}"); err == nil {
		if podmanUsable() {
			check.Status = StatusPass
			check.Detail = fmt.Sprintf("Podman %s (Docker-compatible socket)", version)
			return check
		}
		check.Status = StatusWarn
		check.Detail = fmt.Sprintf("Podman %s detected, but cidx cannot use it — API socket not available", version)
		check.Suggestion = fmt.Sprintf("Start Docker, or enable the Podman socket: %s", executor.PodmanSocketHint())
		return check
	}

	check.Status = StatusFail
	check.Detail = "not found"
	check.Suggestion = "Install Docker (https://docs.docker.com/get-docker/) or Podman (https://podman.io/)"
	return check
}

// checkGitRepo verifies we're inside a Git repository.
func checkGitRepo() Check {
	check := Check{Name: "Git repository"}

	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	output, err := cmd.Output()
	if err != nil {
		check.Status = StatusFail
		check.Detail = "not a Git repository"
		check.Suggestion = "Run 'git init' or navigate to a Git repository"
		return check
	}

	// Try to get remote info for display
	remoteCmd := exec.Command("git", "remote", "get-url", "origin")
	if remoteOutput, err := remoteCmd.Output(); err == nil {
		check.Detail = fmt.Sprintf("detected (%s)", strings.TrimSpace(string(remoteOutput)))
	} else {
		check.Detail = fmt.Sprintf("detected (%s)", strings.TrimSpace(string(output)))
	}
	check.Status = StatusPass
	return check
}

// checkConfigFile verifies cidx.toml exists and is valid.
func checkConfigFile() Check {
	check := Check{Name: "Config file"}

	configPath, err := config.FindConfig()
	if err != nil {
		check.Status = StatusWarn
		check.Detail = "not found"
		check.Suggestion = "Run 'cidx init' to create a configuration"
		return check
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		check.Status = StatusFail
		check.Detail = fmt.Sprintf("invalid (%v)", err)
		check.Suggestion = "Run 'cidx validate' for details"
		return check
	}

	result := config.Validate(cfg)
	if !result.Valid {
		check.Status = StatusFail
		check.Detail = fmt.Sprintf("invalid (%d errors)", len(result.Errors))
		check.Suggestion = "Run 'cidx validate' for details"
		return check
	}

	check.Status = StatusPass
	check.Detail = fmt.Sprintf("valid (%s)", configPath)
	return check
}

// getCommandVersion runs a command and returns its trimmed output.
func getCommandVersion(name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}
