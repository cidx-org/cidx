package features

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"

	"github.com/cidx-org/cidx/v3/pkg/config"
	"github.com/cidx-org/cidx/v3/pkg/executor"
	"github.com/cidx-org/cidx/v3/pkg/pipeline"
	"github.com/cucumber/godog"
	"gopkg.in/yaml.v3"
)

// recordingExecutor replaces only the container boundary, leaving config
// resolution, phase order, failure handling and local safety to the runner.
type recordingExecutor struct {
	tc    *TestContext
	calls []string
}

func (e *recordingExecutor) Run(_ context.Context, cfg *config.ContainerConfig) error {
	e.calls = append(e.calls, cfg.Name)
	e.tc.ExecutedPhases = append(e.tc.ExecutedPhases, cfg.Phase)
	if e.tc.Config["phase_"+cfg.Phase+"_fails"] == true {
		e.tc.FailedPhases = append(e.tc.FailedPhases, cfg.Phase)
		return fmt.Errorf("staged failure in %s", cfg.Phase)
	}
	return nil
}
func (*recordingExecutor) Available() bool { return true }
func (*recordingExecutor) Name() string    { return "recording" }
func (*recordingExecutor) Close() error    { return nil }

type fixedExecutorSelector struct{ backend executor.Executor }

func (s fixedExecutorSelector) Select(_ string, _ executor.BackendType) (executor.Executor, error) {
	return s.backend, nil
}
func (s fixedExecutorSelector) DockerAvailable() bool { return s.backend.Available() }
func (fixedExecutorSelector) PodmanAvailable() bool   { return false }

func (tc *TestContext) runDeclaredPipeline(target string, phases []string, dryRun bool) error {
	cfg := &config.Config{
		Phases:    make(map[string]config.Phase),
		Pipelines: map[string]config.Pipeline{tc.Pipeline: {Phases: phases}},
		Overrides: make(map[string]map[string]any),
	}
	for _, phase := range phases {
		name := "scenario-" + phase
		cfg.Phases[phase] = config.Phase{Containers: []string{name}}
		cfg.Overrides[name] = map[string]any{
			"image": "scenario/image:fixture", "command": "true", "phase": phase,
		}
	}
	recorded := &recordingExecutor{tc: tc}
	var backend executor.Executor = recorded
	if dryRun {
		// Use the real preview renderer; it does not need a running daemon.
		preview, err := executor.NewDockerExecutor(true, false, false)
		if err != nil {
			return fmt.Errorf("create preview executor: %w", err)
		}
		defer func() { _ = preview.Close() }()
		backend = preview
	}
	return tc.runWithBackend(cfg, target, backend)
}

func (tc *TestContext) runWithBackend(cfg *config.Config, target string, backend executor.Executor) error {
	done, err := captureStderr()
	if err != nil {
		return err
	}
	output, runErr := captureOutput(func() error {
		runner := pipeline.NewRunnerWithOptions(cfg, fixedExecutorSelector{backend: backend},
			pipeline.RunnerOptions{Backend: executor.BackendDocker})
		return runner.Run(context.Background(), target)
	})
	tc.Output = done() + output
	tc.ExitCode = 0
	if runErr != nil {
		tc.Output += runErr.Error()
		tc.ExitCode = 1
	}
	return nil
}

func RegisterRunnerSteps(ctx *godog.ScenarioContext, tc *TestContext) {
	ctx.When(`^I run the release preset "([^"]*)" through the runner$`, tc.runReleasePreset)
	ctx.Then(`^no release container should have been executed$`, tc.releaseContainerCount(0))
	ctx.Then(`^exactly one release container should have been executed$`, tc.releaseContainerCount(1))
	ctx.When(`^I inspect this repository's CI workflow$`, tc.inspectRepositoryCI)
	ctx.Then(`^its validation jobs should depend on each preceding gate$`, tc.checkRepositoryGateOrder)
}

func (tc *TestContext) runReleasePreset(name string) error {
	recorded := &recordingExecutor{tc: tc}
	tc.Config["release_executor"] = recorded
	tc.LastCommand = "cidx run " + name
	return tc.runWithBackend(&config.Config{}, name, recorded)
}

func (tc *TestContext) releaseContainerCount(want int) func() error {
	return func() error {
		recorded, ok := tc.Config["release_executor"].(*recordingExecutor)
		if !ok {
			return fmt.Errorf("the release runner was never exercised")
		}
		if tc.ExitCode != 0 {
			return fmt.Errorf("release run failed: %s", tc.Output)
		}
		if len(recorded.calls) != want {
			return fmt.Errorf("release executor called %d times (%v), expected %d", len(recorded.calls), recorded.calls, want)
		}
		return nil
	}
}

func (tc *TestContext) inspectRepositoryCI() error {
	source, err := os.ReadFile(filepath.Join("..", ".github", "workflows", "ci.yml"))
	if err != nil {
		return fmt.Errorf("read repository CI: %w", err)
	}
	var workflow struct {
		Jobs map[string]struct {
			Needs []string `yaml:"needs"`
		} `yaml:"jobs"`
	}
	if err := yaml.Unmarshal(source, &workflow); err != nil {
		return fmt.Errorf("decode repository CI: %w", err)
	}
	dependencies := make(map[string][]string)
	for name, job := range workflow.Jobs {
		dependencies[name] = job.Needs
	}
	tc.Config["ci_dependencies"] = dependencies
	return nil
}

func (tc *TestContext) checkRepositoryGateOrder() error {
	dependencies, ok := tc.Config["ci_dependencies"].(map[string][]string)
	if !ok {
		return fmt.Errorf("repository CI was not inspected")
	}
	gates := []string{"code", "security", "test", "build"}
	for i, gate := range gates {
		needs, exists := dependencies[gate]
		if !exists {
			return fmt.Errorf("CI has no %s job", gate)
		}
		if i > 0 && !slices.Contains(needs, gates[i-1]) {
			return fmt.Errorf("CI job %s does not wait for %s (needs: %v)", gate, gates[i-1], needs)
		}
	}
	return nil
}
