package drift

import (
	"strings"
	"testing"

	"github.com/cidx-org/cidx/pkg/config"
)

func TestCompareFromData_AllMatch(t *testing.T) {
	cfg := &config.Config{
		Pipelines: map[string]config.Pipeline{
			"ci": {Phases: []string{"security", "code", "test"}},
		},
	}

	workflow := `
name: CIDX CI
on:
  push:
    branches: [main]
jobs:
  bootstrap:
    name: Bootstrap
  security:
    name: Security
    needs: [bootstrap]
  code:
    name: Code Quality
    needs: [bootstrap]
  test:
    name: Test
    needs: [bootstrap]
`

	result, err := CompareFromData(cfg, []byte(workflow))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.HasDrift() {
		t.Error("expected no drift")
	}

	for _, p := range result.Phases {
		if p.Status != StatusMatch {
			t.Errorf("phase %q: expected match, got %q", p.Name, p.Status)
		}
	}
}

func TestCompareFromData_MissingPhase(t *testing.T) {
	cfg := &config.Config{
		Pipelines: map[string]config.Pipeline{
			"ci": {Phases: []string{"security", "code", "test", "build"}},
		},
	}

	workflow := `
name: CIDX CI
on:
  push:
    branches: [main]
jobs:
  bootstrap:
    name: Bootstrap
  security:
    name: Security
  code:
    name: Code
  test:
    name: Test
`

	result, err := CompareFromData(cfg, []byte(workflow))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !result.HasDrift() {
		t.Error("expected drift (build missing from CI)")
	}

	var found bool
	for _, p := range result.Phases {
		if p.Name == "build" && p.Status == StatusMissing {
			found = true
		}
	}
	if !found {
		t.Error("expected 'build' phase with status 'missing from CI'")
	}
}

func TestCompareFromData_ExtraJob(t *testing.T) {
	cfg := &config.Config{
		Pipelines: map[string]config.Pipeline{
			"ci": {Phases: []string{"security", "code"}},
		},
	}

	workflow := `
name: CIDX CI
on:
  push:
    branches: [main]
jobs:
  bootstrap:
    name: Bootstrap
  security:
    name: Security
  code:
    name: Code
  test:
    name: Test
`

	result, err := CompareFromData(cfg, []byte(workflow))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var found bool
	for _, p := range result.Phases {
		if p.Name == "test" && p.Status == StatusExtra {
			found = true
		}
	}
	if !found {
		t.Error("expected 'test' job with status 'extra in CI'")
	}
}

func TestCompareFromData_TriggerMatch(t *testing.T) {
	cfg := &config.Config{
		Pipelines: map[string]config.Pipeline{
			"pr":   {Phases: []string{"security"}},
			"main": {Phases: []string{"security", "build"}},
		},
	}

	workflow := `
name: CIDX CI
on:
  push:
    branches: [main]
  pull_request:
    branches: [main]
jobs:
  security:
    name: Security
  build:
    name: Build
`

	result, err := CompareFromData(cfg, []byte(workflow))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, tr := range result.Triggers {
		if tr.Status != StatusMatch {
			t.Errorf("trigger %q: expected match, got %q", tr.Event, tr.Status)
		}
	}
}

func TestCompareFromData_MissingTrigger(t *testing.T) {
	cfg := &config.Config{
		Pipelines: map[string]config.Pipeline{
			"pr": {Phases: []string{"security"}},
		},
	}

	workflow := `
name: CIDX CI
on:
  push:
    branches: [main]
jobs:
  security:
    name: Security
`

	result, err := CompareFromData(cfg, []byte(workflow))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var found bool
	for _, tr := range result.Triggers {
		if tr.Event == "pull_request" && tr.Status == "missing" {
			found = true
		}
	}
	if !found {
		t.Error("expected pull_request trigger with status 'missing'")
	}
}

func TestResult_DiffCount(t *testing.T) {
	r := &Result{
		Phases: []PhaseDiff{
			{Name: "a", Status: StatusMatch},
			{Name: "b", Status: StatusMissing},
			{Name: "c", Status: StatusExtra},
		},
		Triggers: []TriggerDiff{
			{Event: "push", Status: StatusMatch},
		},
	}

	if got := r.DiffCount(); got != 2 {
		t.Errorf("DiffCount() = %d, want 2", got)
	}
}

func TestFormat(t *testing.T) {
	r := &Result{
		Phases: []PhaseDiff{
			{Name: "security", CIDX: true, CI: true, Status: StatusMatch},
			{Name: "build", CIDX: true, CI: false, Status: StatusMissing},
		},
		Triggers: []TriggerDiff{
			{Event: "push", CIDX: true, CI: true, Status: StatusMatch},
		},
	}

	output := Format(r)
	if !strings.Contains(output, "security") {
		t.Error("expected security in output")
	}
	if !strings.Contains(output, "build") {
		t.Error("expected build in output")
	}
	if !strings.Contains(output, "missing from CI") {
		t.Error("expected 'missing from CI' in output")
	}
	if !strings.Contains(output, "Phases:") {
		t.Error("expected Phases header")
	}
	if !strings.Contains(output, "Triggers:") {
		t.Error("expected Triggers header")
	}
}

func TestCompareFromData_BootstrapIgnored(t *testing.T) {
	cfg := &config.Config{
		Pipelines: map[string]config.Pipeline{
			"ci": {Phases: []string{"security"}},
		},
	}

	workflow := `
name: CIDX CI
on:
  push:
    branches: [main]
jobs:
  bootstrap:
    name: Bootstrap
  security:
    name: Security
    needs: [bootstrap]
`

	result, err := CompareFromData(cfg, []byte(workflow))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// bootstrap should not appear as "extra in CI"
	for _, p := range result.Phases {
		if p.Name == "bootstrap" {
			t.Error("bootstrap should be excluded from phase comparison")
		}
	}
}
