package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cidx-org/cidx/v2/pkg/validator"
)

// repoWorkflowDir is this repository's own workflow directory, seen from the
// package under test.
const repoWorkflowDir = "../../.github/workflows"

// TestValidate_RealWorkflowsResolve is the standing guard for #239: every cidx
// invocation in this repository's workflows must resolve against the real
// command tree. It reads files only — no network, no workflow run.
func TestValidate_RealWorkflowsResolve(t *testing.T) {
	invocations, err := validator.WorkflowInvocations(repoWorkflowDir)
	if err != nil {
		t.Fatalf("WorkflowInvocations: %v", err)
	}
	if len(invocations) == 0 {
		t.Fatal("no cidx invocation found in the workflows — the check would guard nothing")
	}

	app := newApp()
	for _, inv := range invocations {
		if reason := validator.Resolve(app, inv.Args); reason != "" {
			t.Errorf("%s:%d — %s\n      %s", inv.File, inv.Line, reason, inv)
		}
	}
	t.Logf("%d cidx invocations resolved against the command tree", len(invocations))
}

// TestValidateWorkflowInvocations_ReportsStale drives the command helper: a
// workflow calling a moved subcommand must fail validation, and the error must
// count what it found.
func TestValidateWorkflowInvocations_ReportsStale(t *testing.T) {
	dir := t.TempDir()
	workflow := `name: Security Audit
jobs:
  audit:
    steps:
      - name: Generate ignore file
        run: ./bin/cidx vuln ignore --format trivy -o .trivyignore alpine
`
	if err := os.WriteFile(filepath.Join(dir, "security-audit.yml"), []byte(workflow), 0644); err != nil {
		t.Fatal(err)
	}

	err := validateWorkflowInvocations(newApp(), dir)
	if err == nil {
		t.Fatal("expected the stale invocation to fail validation")
	}
	if want := "1 stale cidx invocation(s)"; !strings.Contains(err.Error(), want) {
		t.Errorf("expected the error to report %q, got %q", want, err)
	}
}

// TestValidateWorkflowInvocations_AcceptsCurrentCommands: the same step, with
// the command where it lives today, passes.
func TestValidateWorkflowInvocations_AcceptsCurrentCommands(t *testing.T) {
	dir := t.TempDir()
	workflow := `name: Security Audit
jobs:
  audit:
    steps:
      - name: Generate ignore file
        run: ./bin/cidx security vuln ignore --format trivy -o .trivyignore alpine
`
	if err := os.WriteFile(filepath.Join(dir, "security-audit.yml"), []byte(workflow), 0644); err != nil {
		t.Fatal(err)
	}

	if err := validateWorkflowInvocations(newApp(), dir); err != nil {
		t.Errorf("expected the current form to pass, got %v", err)
	}
}
