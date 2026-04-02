package doctor

import (
	"testing"
)

func TestResult_Passed_AllPass(t *testing.T) {
	r := &Result{
		Checks: []Check{
			{Name: "a", Status: StatusPass},
			{Name: "b", Status: StatusPass},
		},
	}
	if !r.Passed() {
		t.Error("expected Passed() = true when all checks pass")
	}
}

func TestResult_Passed_WithFailure(t *testing.T) {
	r := &Result{
		Checks: []Check{
			{Name: "a", Status: StatusPass},
			{Name: "b", Status: StatusFail},
		},
	}
	if r.Passed() {
		t.Error("expected Passed() = false when a check fails")
	}
}

func TestResult_Passed_WarningIsNotFailure(t *testing.T) {
	r := &Result{
		Checks: []Check{
			{Name: "a", Status: StatusPass},
			{Name: "b", Status: StatusWarn},
		},
	}
	if !r.Passed() {
		t.Error("expected Passed() = true when only warnings (no failures)")
	}
}

func TestResult_Issues(t *testing.T) {
	r := &Result{
		Checks: []Check{
			{Name: "a", Status: StatusFail},
			{Name: "b", Status: StatusPass},
			{Name: "c", Status: StatusFail},
			{Name: "d", Status: StatusWarn},
		},
	}
	if got := r.Issues(); got != 2 {
		t.Errorf("Issues() = %d, want 2", got)
	}
}

func TestResult_Warnings(t *testing.T) {
	r := &Result{
		Checks: []Check{
			{Name: "a", Status: StatusWarn},
			{Name: "b", Status: StatusPass},
			{Name: "c", Status: StatusWarn},
		},
	}
	if got := r.Warnings(); got != 2 {
		t.Errorf("Warnings() = %d, want 2", got)
	}
}

func TestCheckContainerRuntime(t *testing.T) {
	check := checkContainerRuntime()
	// In most dev environments, Docker is available
	// We just verify the check runs without panic and returns a valid status
	if check.Name != "Container runtime" {
		t.Errorf("Name = %q, want 'Container runtime'", check.Name)
	}
	if check.Status != StatusPass && check.Status != StatusFail {
		t.Errorf("unexpected status %d", check.Status)
	}
	if check.Detail == "" {
		t.Error("expected non-empty detail")
	}
}

func TestCheckGitRepo(t *testing.T) {
	// This test runs inside the cidx repo, so git should be detected
	check := checkGitRepo()
	if check.Name != "Git repository" {
		t.Errorf("Name = %q, want 'Git repository'", check.Name)
	}
	if check.Status != StatusPass {
		t.Errorf("expected StatusPass in a git repo, got %d", check.Status)
	}
}

func TestCheckConfigFile(t *testing.T) {
	check := checkConfigFile()
	if check.Name != "Config file" {
		t.Errorf("Name = %q, want 'Config file'", check.Name)
	}
	// Either found or warn, both are valid in test context
	if check.Status != StatusPass && check.Status != StatusWarn {
		t.Errorf("unexpected status %d", check.Status)
	}
}

func TestRun(t *testing.T) {
	result := Run()
	if len(result.Checks) != 3 {
		t.Errorf("expected 3 checks, got %d", len(result.Checks))
	}
}
