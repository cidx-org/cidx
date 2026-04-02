// Package doctor validates the CIDX runtime environment.
package doctor

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/cidx-org/cidx/pkg/config"
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

// checkContainerRuntime verifies Docker or Podman is available.
func checkContainerRuntime() Check {
	check := Check{Name: "Container runtime"}

	// Try Docker first
	if version, err := getCommandVersion("docker", "version", "--format", "{{.Server.Version}}"); err == nil {
		check.Status = StatusPass
		check.Detail = fmt.Sprintf("Docker %s", version)
		return check
	}

	// Try Podman
	if version, err := getCommandVersion("podman", "version", "--format", "{{.Version}}"); err == nil {
		check.Status = StatusPass
		check.Detail = fmt.Sprintf("Podman %s", version)
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
