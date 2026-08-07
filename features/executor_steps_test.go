package features

import (
	"fmt"
	"reflect"
	"regexp"
	"strings"

	"github.com/cidx-org/cidx/v3/pkg/config"
	"github.com/cidx-org/cidx/v3/pkg/executor"
	"github.com/cidx-org/cidx/v3/pkg/presets"
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

	// Project-scoped container names (#197)
	ctx.When(`^I compute the container name for tool "([^"]*)" in workspace "([^"]*)"$`, tc.computeContainerName)
	ctx.Then(`^the container name should start with "([^"]*)"$`, tc.containerNameShouldStartWith)
	ctx.Then(`^the container name should end with "([^"]*)"$`, tc.containerNameShouldEndWith)
	ctx.Then(`^the container name should be a valid Docker container name$`, tc.containerNameShouldBeValid)
	ctx.Then(`^the two container names should differ$`, tc.twoContainerNamesShouldDiffer)
	ctx.Then(`^the two container names should be identical$`, tc.twoContainerNamesShouldBeIdentical)
}

// dockerContainerNameRE is Docker's container name grammar.
var dockerContainerNameRE = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]*$`)

// computeContainerName resolves the container name cidx would use for a tool
// in a given workspace, appending it to the names collected by the scenario.
func (tc *TestContext) computeContainerName(tool, workspace string) error {
	names, _ := tc.Config["container_names"].([]string)
	tc.Config["container_names"] = append(names, executor.ContainerName(workspace, tool))
	return nil
}

func (tc *TestContext) lastContainerName() (string, error) {
	names, _ := tc.Config["container_names"].([]string)
	if len(names) == 0 {
		return "", fmt.Errorf("no container name has been computed yet")
	}
	return names[len(names)-1], nil
}

func (tc *TestContext) containerNameShouldStartWith(prefix string) error {
	name, err := tc.lastContainerName()
	if err != nil {
		return err
	}
	if !strings.HasPrefix(name, prefix) {
		return fmt.Errorf("container name %q does not start with %q", name, prefix)
	}
	return nil
}

func (tc *TestContext) containerNameShouldEndWith(suffix string) error {
	name, err := tc.lastContainerName()
	if err != nil {
		return err
	}
	if !strings.HasSuffix(name, suffix) {
		return fmt.Errorf("container name %q does not end with %q", name, suffix)
	}
	return nil
}

func (tc *TestContext) containerNameShouldBeValid() error {
	name, err := tc.lastContainerName()
	if err != nil {
		return err
	}
	if !dockerContainerNameRE.MatchString(name) {
		return fmt.Errorf("container name %q is not a valid Docker container name", name)
	}
	return nil
}

func (tc *TestContext) twoContainerNames() (string, string, error) {
	names, _ := tc.Config["container_names"].([]string)
	if len(names) < 2 {
		return "", "", fmt.Errorf("expected 2 computed container names, got %d", len(names))
	}
	return names[len(names)-2], names[len(names)-1], nil
}

func (tc *TestContext) twoContainerNamesShouldDiffer() error {
	first, second, err := tc.twoContainerNames()
	if err != nil {
		return err
	}
	if first == second {
		return fmt.Errorf("expected distinct container names, both are %q", first)
	}
	return nil
}

func (tc *TestContext) twoContainerNamesShouldBeIdentical() error {
	first, second, err := tc.twoContainerNames()
	if err != nil {
		return err
	}
	if first != second {
		return fmt.Errorf("expected identical container names, got %q and %q", first, second)
	}
	return nil
}

// iHaveAValidCidxTomlConfiguration stages a cidx.toml in the scenario's project
// directory and loads it through config.Load, so "valid" means the real loader
// accepted it rather than the scenario saying so.
func (tc *TestContext) iHaveAValidCidxTomlConfiguration() error {
	cfg, err := tc.loadStagedConfig()
	if err != nil {
		return err
	}
	tc.Config["loaded_config"] = cfg
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
	if !strings.Contains(tc.Output, "Would execute") {
		return fmt.Errorf("the run did not report itself as a dry-run:\n%s", tc.Output)
	}
	if len(tc.ExecutedPhases) > 0 {
		return fmt.Errorf("a dry-run executed %v", tc.ExecutedPhases)
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

// iRunATool resolves a catalogue preset into the config.ContainerConfig every
// backend receives — the struct pipeline.RunTool hands to Executor.Run — so the
// next step can look at a real one instead of a list of field names.
func (tc *TestContext) iRunATool() error {
	preset, err := presets.Get("trivy")
	if err != nil {
		return err
	}
	tc.Config["container_config"] = &config.ContainerConfig{
		Name:        preset.Name,
		Phase:       preset.Phase,
		Image:       preset.Image,
		Command:     preset.Command,
		Entrypoint:  preset.Entrypoint,
		Workdir:     preset.Workdir,
		Volumes:     preset.Volumes,
		Env:         preset.Env,
		ConfigFiles: preset.ConfigFiles,
		PullPolicy:  preset.PullPolicy,
		Timeout:     preset.Timeout,
	}
	return nil
}

// theContainerConfigShouldContain asks the config.ContainerConfig the run
// produced, by reflection: the field has to exist on the type every backend
// receives, and it has to carry something.
func (tc *TestContext) theContainerConfigShouldContain(table *godog.Table) error {
	containerConfig, ok := tc.Config["container_config"].(*config.ContainerConfig)
	if !ok {
		return fmt.Errorf("no tool was run, so there is no ContainerConfig to inspect")
	}
	value := reflect.ValueOf(*containerConfig)

	for _, row := range table.Rows[1:] {
		name := row.Cells[0].Value
		field := value.FieldByName(name)
		if !field.IsValid() {
			return fmt.Errorf("config.ContainerConfig has no field %q", name)
		}
		if field.IsZero() {
			return fmt.Errorf("config.ContainerConfig field %q came back empty for %q", name, containerConfig.Name)
		}
	}
	return nil
}
