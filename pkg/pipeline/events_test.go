package pipeline

import (
	"context"
	"slices"
	"sync"
	"testing"

	"github.com/cidx-org/cidx/v3/pkg/config"
	"github.com/cidx-org/cidx/v3/pkg/events"
	"github.com/cidx-org/cidx/v3/pkg/executor"
)

// collector keeps the events a run emitted.
type collector struct {
	mu  sync.Mutex
	got []events.Event
}

func (c *collector) Emit(e events.Event) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.got = append(c.got, e)
}
func (*collector) Close() {}

func (c *collector) kinds() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	var out []string
	for _, e := range c.got {
		name := string(e.Type)
		if e.Container != "" {
			name += ":" + e.Container
		} else if e.Phase != "" {
			name += ":" + e.Phase
		}
		out = append(out, name)
	}
	return out
}

func eventfulRun(t *testing.T, answers map[string]error, target string) (*collector, error) {
	t.Helper()
	custom := func() map[string]any { return map[string]any{"image": "alpine:3", "command": "true"} }
	cfg := &config.Config{
		Workspace: t.TempDir(),
		Phases: map[string]config.Phase{
			"first":  {Containers: []string{"a", "b"}},
			"second": {Containers: []string{"c"}},
		},
		Pipelines: map[string]config.Pipeline{"ci": {Phases: []string{"first", "second"}}},
		Overrides: map[string]map[string]any{"a": custom(), "b": custom(), "c": custom()},
	}
	col := &collector{}
	r := NewRunnerWithOptions(cfg, scriptedSelector{scriptedExecutor{answers}},
		RunnerOptions{Backend: executor.BackendDocker, Concurrency: 1, Events: col})
	return col, r.Run(context.Background(), target)
}

func TestRunEmitsEventsInOrder(t *testing.T) {
	col, err := eventfulRun(t, nil, "ci")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"run.started", "phase.started:first",
		"container.started:a", "container.finished:a",
		"container.started:b", "container.finished:b",
		"phase.finished:first", "phase.started:second",
		"container.started:c", "container.finished:c",
		"phase.finished:second", "run.finished",
	}
	if got := col.kinds(); !slices.Equal(got, want) {
		t.Fatalf("events = %v\nwant     %v", got, want)
	}

	last := col.got[len(col.got)-1]
	if last.Target != "ci" || last.Status != events.StatusPassed || last.ExitCode == nil || *last.ExitCode != 0 || last.DurationMS == nil {
		t.Errorf("run.finished = %+v", last)
	}
	container := col.got[3]
	if container.Image != "alpine:3" || container.Phase != "first" || container.Status != events.StatusPassed {
		t.Errorf("container.finished = %+v", container)
	}
}

func TestFailedContainerIsReportedWithItsExitCode(t *testing.T) {
	col, err := eventfulRun(t, map[string]error{"a": &executor.ExitError{Code: 7}}, "ci")
	if err == nil {
		t.Fatal("the run must still fail")
	}
	want := []string{
		"run.started", "phase.started:first",
		"container.started:a", "container.finished:a",
		"phase.finished:first", "run.finished",
	}
	if got := col.kinds(); !slices.Equal(got, want) {
		t.Fatalf("events = %v\nwant     %v", got, want)
	}
	for _, i := range []int{3, 4, 5} {
		e := col.got[i]
		if e.Status != events.StatusFailed || e.ExitCode == nil || *e.ExitCode != 7 {
			t.Errorf("%s = %+v, want failed with exit 7", e.Type, e)
		}
	}
}

func TestAToolTargetHasNoPhase(t *testing.T) {
	col, err := eventfulRun(t, nil, "a")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"run.started", "container.started:a", "container.finished:a", "run.finished"}
	if got := col.kinds(); !slices.Equal(got, want) {
		t.Fatalf("events = %v\nwant     %v", got, want)
	}
	if col.got[1].Phase != "" {
		t.Errorf("a tool run directly has no phase, got %q", col.got[1].Phase)
	}
}

func TestARunWithoutEmitterBehavesTheSame(t *testing.T) {
	out, err := summaryOf(t, false, map[string]error{"fails": &executor.ExitError{Code: 7}})
	if err == nil || out == "" {
		t.Fatalf("a runner built without Events must run as before (err=%v)", err)
	}
}
