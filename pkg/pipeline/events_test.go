package pipeline

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/cidx-org/cidx/v3/pkg/config"
	"github.com/cidx-org/cidx/v3/pkg/events"
	"github.com/cidx-org/cidx/v3/pkg/executor"
)

func eventsRun(t *testing.T, target string, answers map[string]error) (*events.Recorder, error) {
	t.Helper()
	custom := func() map[string]any { return map[string]any{"image": "alpine:3", "command": "true"} }
	cfg := &config.Config{
		Workspace: t.TempDir(),
		Phases:    map[string]config.Phase{"first": {Containers: []string{"fails", "later"}}},
		Pipelines: map[string]config.Pipeline{"ci": {Phases: []string{"first"}}},
		Overrides: map[string]map[string]any{"fails": custom(), "later": custom()},
	}
	rec := &events.Recorder{}
	r := NewRunnerWithOptions(cfg, scriptedSelector{scriptedExecutor{answers}},
		RunnerOptions{Backend: executor.BackendDocker, Concurrency: 1, Events: rec})
	return rec, r.Run(context.Background(), target)
}

func types(evs []events.Event) []string {
	var out []string
	for _, e := range evs {
		out = append(out, e.Type)
	}
	return out
}

func TestRunEmitsTheStructureOfThePipeline(t *testing.T) {
	rec, err := eventsRun(t, "ci", nil)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		events.RunStarted, events.PhaseStarted,
		events.ContainerStarted, events.ContainerFinished,
		events.ContainerStarted, events.ContainerFinished,
		events.PhaseFinished, events.RunFinished,
	}
	evs := rec.Events()
	if got := types(evs); !slices.Equal(got, want) {
		t.Fatalf("events = %v, want %v", got, want)
	}
	c := evs[3]
	if c.Container != "fails" || c.Image != "alpine:3" || c.Phase != "first" || c.Status != events.StatusPassed {
		t.Errorf("container.finished lacks its identity: %+v", c)
	}
	if c.ExitCode == nil || *c.ExitCode != 0 || c.DurationMS == nil {
		t.Errorf("container.finished lacks exit code or duration: %+v", c)
	}
	if last := evs[len(evs)-1]; last.Target != "ci" || last.Status != events.StatusPassed {
		t.Errorf("run.finished = %+v", last)
	}
}

func TestFailureEventsCarryTheExitCodeAndStopAtTheFailure(t *testing.T) {
	rec, err := eventsRun(t, "ci", map[string]error{"fails": &executor.ExitError{Code: 7}})
	if err == nil {
		t.Fatal("the run must still fail")
	}
	evs := rec.Events()
	var failedContainer, run events.Event
	for _, e := range evs {
		if e.Type == events.ContainerFinished && e.Container == "fails" {
			failedContainer = e
		}
		if e.Type == events.RunFinished {
			run = e
		}
		if e.Container == "later" {
			t.Errorf("a container that was never reached has events: %+v", e)
		}
	}
	if failedContainer.Status != events.StatusFailed || failedContainer.ExitCode == nil || *failedContainer.ExitCode != 7 {
		t.Errorf("failed container = %+v", failedContainer)
	}
	if run.Status != events.StatusFailed || run.ExitCode == nil || *run.ExitCode != 7 {
		t.Errorf("run.finished = %+v", run)
	}
}

func TestAContainerThatNeverStartedHasNoExitCode(t *testing.T) {
	rec, _ := eventsRun(t, "ci", map[string]error{"fails": errors.New("failed to pull image")})
	for _, e := range rec.Events() {
		if e.Type == events.ContainerFinished && e.Container == "fails" {
			if e.Status != events.StatusFailed || e.ExitCode != nil {
				t.Errorf("a container that never ran has no exit code: %+v", e)
			}
			return
		}
	}
	t.Fatal("no container.finished for the failed container")
}

func TestEventsAreOptional(t *testing.T) {
	cfg := &config.Config{
		Workspace: t.TempDir(),
		Overrides: map[string]map[string]any{"fails": {"image": "alpine:3", "command": "true"}},
	}
	r := NewRunnerWithOptions(cfg, scriptedSelector{scriptedExecutor{}}, RunnerOptions{Backend: executor.BackendDocker})
	if err := r.Run(context.Background(), "fails"); err != nil {
		t.Fatalf("a runner without an emitter must work: %v", err)
	}
}
