package pipeline

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/cidx-org/cidx/v3/pkg/config"
	"github.com/cidx-org/cidx/v3/pkg/executor"
	"github.com/sirupsen/logrus"
)

// scriptedExecutor answers each container by name: nil passes, an error fails.
type scriptedExecutor struct{ answers map[string]error }

func (s scriptedExecutor) Run(_ context.Context, c *config.ContainerConfig) error {
	return s.answers[c.Name]
}
func (scriptedExecutor) Available() bool { return true }
func (scriptedExecutor) Name() string    { return "scripted" }
func (scriptedExecutor) Close() error    { return nil }

type scriptedSelector struct{ exec executor.Executor }

func (s scriptedSelector) Select(string, executor.BackendType) (executor.Executor, error) {
	return s.exec, nil
}
func (scriptedSelector) DockerAvailable() bool { return true }
func (scriptedSelector) PodmanAvailable() bool { return false }

// summaryOf runs a two-phase pipeline whose containers answer as scripted, and
// returns the summary it printed and the error it returned (#547).
func summaryOf(t *testing.T, parallel bool, answers map[string]error) (string, error) {
	t.Helper()
	custom := func() map[string]any { return map[string]any{"image": "alpine:3", "command": "true"} }
	cfg := &config.Config{
		Workspace: t.TempDir(),
		Phases: map[string]config.Phase{
			"first":  {Containers: []string{"fails", "unreached"}},
			"second": {Containers: []string{"later"}},
		},
		Overrides: map[string]map[string]any{"fails": custom(), "unreached": custom(), "later": custom()},
	}
	r := NewRunnerWithOptions(cfg, scriptedSelector{scriptedExecutor{answers}}, RunnerOptions{Backend: executor.BackendDocker, Parallel: parallel, Concurrency: 1})
	var out bytes.Buffer
	r.logger.SetOutput(&out)
	r.logger.SetFormatter(&logrus.TextFormatter{DisableTimestamp: true, DisableColors: true})

	err := r.RunPipeline(context.Background(), "repro", config.Pipeline{Phases: []string{"first", "second"}})
	return out.String(), err
}

func TestSummaryMarksOnlyWhatPassed(t *testing.T) {
	t.Run("a failed container is not ticked, the ones never reached are not run", func(t *testing.T) {
		out, err := summaryOf(t, false, map[string]error{"fails": &executor.ExitError{Code: 7}})
		if err == nil {
			t.Fatal("the pipeline must still fail")
		}
		for _, want := range []string{"fails ✗ (exit 7)", "unreached (not run)", "second (not run) — later (not run)"} {
			if !strings.Contains(out, want) {
				t.Errorf("summary lacks %q:\n%s", want, out)
			}
		}
		if strings.Contains(out, "fails ✓") || strings.Contains(out, "unreached ✓") || strings.Contains(out, "later ✓") {
			t.Errorf("a container that did not pass is ticked:\n%s", out)
		}
	})

	t.Run("a failure before the container starts is a failure too", func(t *testing.T) {
		out, _ := summaryOf(t, false, map[string]error{"fails": errors.New("failed to pull image")})
		if !strings.Contains(out, "fails ✗") || strings.Contains(out, "fails ✓") {
			t.Errorf("a container that never launched is not marked failed:\n%s", out)
		}
	})

	t.Run("a passing pipeline ticks every container", func(t *testing.T) {
		out, err := summaryOf(t, false, nil)
		if err != nil {
			t.Fatal(err)
		}
		for _, want := range []string{"fails ✓", "unreached ✓", "later ✓"} {
			if !strings.Contains(out, want) {
				t.Errorf("summary lacks %q:\n%s", want, out)
			}
		}
	})

	t.Run("in parallel, the one that failed is the one marked", func(t *testing.T) {
		out, err := summaryOf(t, true, map[string]error{"fails": &executor.ExitError{Code: 3}})
		if err == nil {
			t.Fatal("the pipeline must still fail")
		}
		if !strings.Contains(out, "fails ✗ (exit 3)") || !strings.Contains(out, "unreached ✓") {
			t.Errorf("parallel summary is wrong:\n%s", out)
		}
	})
}
