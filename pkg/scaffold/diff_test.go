package scaffold

import (
	"strings"
	"testing"
)

func TestCompare_NoChanges(t *testing.T) {
	detection := &Detection{
		Languages: []Language{{
			Name: "Go", Marker: "go.mod",
			Security: []string{"trivy", "gitleaks"},
			Code:     []string{"golangci-lint"},
		}},
	}

	existing := map[string][]string{
		"security": {"trivy", "gitleaks"},
		"code":     {"golangci-lint"},
	}

	diff := Compare(detection, existing)

	if diff.HasChanges() {
		t.Error("expected no changes")
	}
}

func TestCompare_NewContainersInExistingPhase(t *testing.T) {
	detection := &Detection{
		Languages: []Language{{
			Name: "Go", Marker: "go.mod",
			Security: []string{"trivy", "gitleaks", "gosec"},
			Code:     []string{"golangci-lint", "gofmt"},
		}},
	}

	existing := map[string][]string{
		"security": {"trivy", "gitleaks"},
		"code":     {"golangci-lint"},
	}

	diff := Compare(detection, existing)

	if !diff.HasChanges() {
		t.Fatal("expected changes")
	}

	if diff.TotalAdded() != 2 {
		t.Errorf("expected 2 additions, got %d", diff.TotalAdded())
	}

	// Check security phase has gosec added
	found := false
	for _, c := range diff.Changes {
		if c.Phase == "security" {
			for _, a := range c.Added {
				if a == "gosec" {
					found = true
				}
			}
		}
	}
	if !found {
		t.Error("expected gosec in security additions")
	}
}

func TestCompare_NewPhase(t *testing.T) {
	detection := &Detection{
		Languages: []Language{{
			Name: "Go", Marker: "go.mod",
			Security: []string{"trivy"},
			Test:     []string{"go-test"},
			Build:    []string{"go-build"},
		}},
	}

	existing := map[string][]string{
		"security": {"trivy"},
	}

	diff := Compare(detection, existing)

	if !diff.HasChanges() {
		t.Fatal("expected changes")
	}

	if len(diff.NewPhases) != 2 {
		t.Errorf("expected 2 new phases, got %d", len(diff.NewPhases))
	}
}

func TestCompare_PreservesUserContainers(t *testing.T) {
	detection := &Detection{
		Languages: []Language{{
			Name: "Go", Marker: "go.mod",
			Security: []string{"trivy", "gitleaks"},
		}},
	}

	// User has added a custom container "semgrep" that detection doesn't know about
	existing := map[string][]string{
		"security": {"trivy", "gitleaks", "semgrep"},
	}

	diff := Compare(detection, existing)

	if diff.HasChanges() {
		t.Error("expected no changes — user's custom containers should not be removed")
	}
}

func TestCompare_MultiLanguage(t *testing.T) {
	detection := &Detection{
		Languages: []Language{
			{
				Name: "Go", Marker: "go.mod",
				Security: []string{"trivy", "gitleaks", "gosec"},
				Code:     []string{"golangci-lint"},
			},
			{
				Name: "Python", Marker: "pyproject.toml",
				Security: []string{"trivy", "gitleaks", "bandit"},
				Code:     []string{"ruff", "black"},
			},
		},
	}

	existing := map[string][]string{
		"security": {"trivy", "gitleaks"},
		"code":     {"golangci-lint"},
	}

	diff := Compare(detection, existing)

	if !diff.HasChanges() {
		t.Fatal("expected changes")
	}

	// Should detect gosec and bandit as new security containers
	for _, c := range diff.Changes {
		if c.Phase == "security" {
			if len(c.Added) != 2 {
				t.Errorf("expected 2 security additions (gosec, bandit), got %d: %v", len(c.Added), c.Added)
			}
		}
	}
}

func TestCompare_EmptyDetection(t *testing.T) {
	detection := &Detection{}
	existing := map[string][]string{
		"security": {"trivy"},
	}

	diff := Compare(detection, existing)

	if diff.HasChanges() {
		t.Error("expected no changes for empty detection")
	}
}

func TestUpdateTOML_AddContainersToExistingPhase(t *testing.T) {
	raw := `[security]
containers = ["trivy", "gitleaks"]

[code]
containers = ["golangci-lint"]

[pipelines.ci]
phases = ["security", "code"]
`

	diff := &DiffResult{
		Changes: []PhaseChange{
			{Phase: "security", Added: []string{"gosec"}},
		},
	}

	result := UpdateTOML(raw, diff)

	if !strings.Contains(result, `"gosec"`) {
		t.Error("expected gosec in output")
	}
	if !strings.Contains(result, `"trivy", "gitleaks", "gosec"`) {
		t.Errorf("expected gosec appended to existing list, got:\n%s", result)
	}
}

func TestUpdateTOML_AddNewPhase(t *testing.T) {
	raw := `[security]
containers = ["trivy"]

[pipelines.ci]
phases = ["security"]
`

	diff := &DiffResult{
		NewPhases: []PhaseChange{
			{Phase: "test", Added: []string{"go-test"}},
		},
	}

	result := UpdateTOML(raw, diff)

	if !strings.Contains(result, "[test]") {
		t.Error("expected [test] section in output")
	}
	if !strings.Contains(result, `"go-test"`) {
		t.Error("expected go-test in output")
	}

	// New phase should appear before [pipelines.*]
	testIdx := strings.Index(result, "[test]")
	pipelineIdx := strings.Index(result, "[pipelines.ci]")
	if testIdx > pipelineIdx {
		t.Error("expected [test] to appear before [pipelines.ci]")
	}
}

func TestUpdateTOML_NoChanges(t *testing.T) {
	raw := `[security]
containers = ["trivy"]
`

	diff := &DiffResult{}

	result := UpdateTOML(raw, diff)

	if result != raw {
		t.Error("expected no modification for empty diff")
	}
}

func TestUpdateTOML_PreservesOverrides(t *testing.T) {
	raw := `[security]
containers = ["trivy", "gitleaks"]

[containers.trivy]
severity = "HIGH,CRITICAL"
exit_code = 1

[code]
containers = ["golangci-lint"]

[pipelines.ci]
phases = ["security", "code"]
`

	diff := &DiffResult{
		Changes: []PhaseChange{
			{Phase: "security", Added: []string{"gosec"}},
		},
	}

	result := UpdateTOML(raw, diff)

	// Override section must be preserved
	if !strings.Contains(result, `severity = "HIGH,CRITICAL"`) {
		t.Error("expected trivy override preserved")
	}
	if !strings.Contains(result, `"gosec"`) {
		t.Error("expected gosec added")
	}
}

func TestFormatDiff_NoChanges(t *testing.T) {
	diff := &DiffResult{}
	output := FormatDiff(diff)

	if !strings.Contains(output, "up to date") {
		t.Errorf("expected 'up to date' message, got: %s", output)
	}
}

func TestFormatDiff_WithChanges(t *testing.T) {
	diff := &DiffResult{
		Changes: []PhaseChange{
			{Phase: "security", Added: []string{"gosec"}, Existing: []string{"trivy"}},
		},
		NewPhases: []PhaseChange{
			{Phase: "test", Added: []string{"go-test"}},
		},
	}

	output := FormatDiff(diff)

	if !strings.Contains(output, "+ gosec") {
		t.Error("expected gosec in output")
	}
	if !strings.Contains(output, "(new phase)") {
		t.Error("expected 'new phase' marker")
	}
	if !strings.Contains(output, "2 container(s)") {
		t.Errorf("expected count of 2, got: %s", output)
	}
}
