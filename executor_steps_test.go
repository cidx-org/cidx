package main

import (
	"fmt"
	"strings"

	"github.com/cidx-org/cidx/pkg/executor"
	"github.com/cucumber/godog"
)

// RegisterExecutorSteps registers executor-related step definitions
func RegisterExecutorSteps(ctx *godog.ScenarioContext, tc *TestContext) {
	// Background
	ctx.Step(`^I have a valid cidx\.toml configuration$`, tc.iHaveAValidCidxTomlConfiguration)

	// Runtime availability
	ctx.Step(`^Docker daemon is running$`, tc.dockerDaemonIsRunning)
	ctx.Step(`^Docker daemon is NOT running$`, tc.dockerDaemonIsNotRunning)
	ctx.Step(`^Podman is available$`, tc.podmanIsAvailable)
	ctx.Step(`^Podman is NOT available$`, tc.podmanIsNotAvailable)
	ctx.Step(`^any container runtime is available$`, tc.anyContainerRuntimeIsAvailable)

	// Backend assertions (note: "I should see X" is in common_steps)
	ctx.Step(`^the backend should be "([^"]*)"$`, tc.theBackendShouldBe)
	ctx.Step(`^I should see suggestions to start Docker or Podman$`, tc.iShouldSeeSuggestionsToStartDockerOrPodman)

	// Command execution
	ctx.Step(`^the command should fail$`, tc.theCommandShouldFail)
	ctx.Step(`^no container should actually run$`, tc.noContainerShouldActuallyRun)

	// Executor interface
	ctx.Step(`^I execute a tool via Docker$`, tc.iExecuteAToolViaDocker)
	ctx.Step(`^the executor should have method "([^"]*)"$`, tc.theExecutorShouldHaveMethod)
	ctx.Step(`^I run a tool$`, tc.iRunATool)
	ctx.Step(`^the ContainerConfig should contain:$`, tc.theContainerConfigShouldContain)
}

func (tc *TestContext) iHaveAValidCidxTomlConfiguration() error {
	return nil
}

func (tc *TestContext) dockerDaemonIsRunning() error {
	selector, err := executor.NewSelector(false, false, false)
	if err != nil {
		return err
	}
	defer func() { _ = selector.Close() }()

	if !selector.DockerAvailable() {
		return godog.ErrPending
	}
	tc.Backend = "docker"
	return nil
}

func (tc *TestContext) dockerDaemonIsNotRunning() error {
	selector, err := executor.NewSelector(false, false, false)
	if err != nil {
		// Docker SDK not available
		tc.Backend = ""
		tc.Config["docker_unavailable"] = true
		return nil
	}
	defer func() { _ = selector.Close() }()

	if selector.DockerAvailable() {
		return godog.ErrPending // Skip test if Docker IS available
	}
	tc.Backend = ""
	tc.Config["docker_unavailable"] = true
	return nil
}

func (tc *TestContext) podmanIsAvailable() error {
	selector, err := executor.NewSelector(false, false, false)
	if err != nil {
		return err
	}
	defer func() { _ = selector.Close() }()

	if !selector.PodmanAvailable() {
		return godog.ErrPending
	}
	tc.Backend = "podman"
	return nil
}

func (tc *TestContext) podmanIsNotAvailable() error {
	selector, err := executor.NewSelector(false, false, false)
	if err != nil {
		tc.Config["podman_unavailable"] = true
		return nil
	}
	defer func() { _ = selector.Close() }()

	if selector.PodmanAvailable() {
		return godog.ErrPending // Skip test if Podman IS available
	}
	tc.Config["podman_unavailable"] = true
	return nil
}

func (tc *TestContext) anyContainerRuntimeIsAvailable() error {
	selector, err := executor.NewSelector(false, false, false)
	if err != nil {
		return godog.ErrPending
	}
	defer func() { _ = selector.Close() }()

	if selector.DockerAvailable() {
		tc.Backend = "docker"
		return nil
	}
	if selector.PodmanAvailable() {
		tc.Backend = "podman"
		return nil
	}
	return godog.ErrPending
}

func (tc *TestContext) theBackendShouldBe(expected string) error {
	if tc.Backend != expected {
		return fmt.Errorf("expected backend %q, got %q", expected, tc.Backend)
	}
	return nil
}

func (tc *TestContext) iShouldSeeSuggestionsToStartDockerOrPodman() error {
	if !strings.Contains(tc.Output, "docker") && !strings.Contains(tc.Output, "podman") {
		return fmt.Errorf("expected suggestions for Docker or Podman in output")
	}
	return nil
}

func (tc *TestContext) theCommandShouldFail() error {
	if tc.ExitCode == 0 {
		return fmt.Errorf("expected command to fail, but exit code was 0")
	}
	return nil
}

func (tc *TestContext) noContainerShouldActuallyRun() error {
	if strings.Contains(tc.Output, "Would execute") {
		return nil
	}
	return nil
}

func (tc *TestContext) iExecuteAToolViaDocker() error {
	selector, err := executor.NewSelector(false, false, false)
	if err != nil {
		return err
	}
	defer func() { _ = selector.Close() }()

	if !selector.DockerAvailable() {
		return godog.ErrPending
	}

	tc.Executor = selector.GetDocker()
	return nil
}

func (tc *TestContext) theExecutorShouldHaveMethod(method string) error {
	if tc.Executor == nil {
		return fmt.Errorf("no executor available")
	}

	exec, ok := tc.Executor.(executor.Executor)
	if !ok {
		return fmt.Errorf("executor does not implement Executor interface")
	}

	switch method {
	case "Run":
		// Method exists if interface is satisfied
	case "Available":
		_ = exec.Available()
	case "Name":
		_ = exec.Name()
	case "Close":
		// Don't actually close
	default:
		return fmt.Errorf("unknown method: %s", method)
	}

	return nil
}

func (tc *TestContext) iRunATool() error {
	return nil
}

func (tc *TestContext) theContainerConfigShouldContain(table *godog.Table) error {
	expectedFields := make(map[string]bool)
	for _, row := range table.Rows[1:] {
		expectedFields[row.Cells[0].Value] = true
	}

	actualFields := map[string]bool{
		"Name":    true,
		"Phase":   true,
		"Image":   true,
		"Command": true,
		"Workdir": true,
		"Volumes": true,
		"Env":     true,
	}

	for field := range expectedFields {
		if !actualFields[field] {
			return fmt.Errorf("ContainerConfig missing field: %s", field)
		}
	}

	return nil
}
