package doctor

import (
	"errors"
	"strings"
	"testing"
)

// stubRuntimeProbes replaces the environment probes for the duration of a test.
// dockerVersion/podmanVersion == "" simulates the command failing.
func stubRuntimeProbes(t *testing.T, dockerVersion, podmanVersion string, socketUsable bool) {
	t.Helper()
	origVersion, origUsable := commandVersion, podmanUsable
	t.Cleanup(func() {
		commandVersion = origVersion
		podmanUsable = origUsable
	})

	commandVersion = func(name string, args ...string) (string, error) {
		switch {
		case name == "docker" && dockerVersion != "":
			return dockerVersion, nil
		case name == "podman" && podmanVersion != "":
			return podmanVersion, nil
		}
		return "", errors.New("command failed")
	}
	podmanUsable = func() bool { return socketUsable }
}

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
	// We just verify the check runs against the real environment without
	// panicking and returns a coherent result; any status is legitimate.
	if check.Name != "Container runtime" {
		t.Errorf("Name = %q, want 'Container runtime'", check.Name)
	}
	if check.Detail == "" {
		t.Error("expected non-empty detail")
	}
}

func TestCheckContainerRuntime_DockerAvailable(t *testing.T) {
	stubRuntimeProbes(t, "29.6.1", "", false)

	check := checkContainerRuntime()
	if check.Status != StatusPass {
		t.Errorf("Status = %d, want StatusPass", check.Status)
	}
	if check.Detail != "Docker 29.6.1" {
		t.Errorf("Detail = %q, want 'Docker 29.6.1'", check.Detail)
	}
}

func TestCheckContainerRuntime_PodmanWithoutSocket_Warns(t *testing.T) {
	// The #190 environment: Docker down, Podman CLI works, no API socket.
	stubRuntimeProbes(t, "", "4.9.3", false)

	check := checkContainerRuntime()
	if check.Status != StatusWarn {
		t.Errorf("Status = %d, want StatusWarn", check.Status)
	}
	if !strings.Contains(check.Detail, "Podman 4.9.3") || !strings.Contains(check.Detail, "cannot use it") {
		t.Errorf("Detail = %q, want mention of Podman 4.9.3 being unusable", check.Detail)
	}
	if !strings.Contains(check.Suggestion, "Docker") || !strings.Contains(check.Suggestion, "socket") {
		t.Errorf("Suggestion = %q, want Docker and Podman socket remedies", check.Suggestion)
	}
}

func TestCheckContainerRuntime_PodmanWithSocket_Passes(t *testing.T) {
	stubRuntimeProbes(t, "", "4.9.3", true)

	check := checkContainerRuntime()
	if check.Status != StatusPass {
		t.Errorf("Status = %d, want StatusPass", check.Status)
	}
	if !strings.Contains(check.Detail, "Podman 4.9.3") {
		t.Errorf("Detail = %q, want mention of Podman 4.9.3", check.Detail)
	}
}

func TestCheckContainerRuntime_NoRuntime_Fails(t *testing.T) {
	stubRuntimeProbes(t, "", "", false)

	check := checkContainerRuntime()
	if check.Status != StatusFail {
		t.Errorf("Status = %d, want StatusFail", check.Status)
	}
	if !strings.Contains(check.Suggestion, "Install Docker") {
		t.Errorf("Suggestion = %q, want install suggestion", check.Suggestion)
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
