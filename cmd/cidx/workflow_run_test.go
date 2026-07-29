package main

import (
	"flag"
	"strings"
	"testing"

	"github.com/urfave/cli/v2"
)

// runContext builds the parsed context urfave/cli would hand the action for
// the given argument list, so the argument handling is tested without an app,
// a repository or a network call.
func runContext(t *testing.T, args ...string) *cli.Context {
	t.Helper()

	var cmd *cli.Command
	for _, sub := range workflowCommand().Subcommands {
		if sub.Name == "run" {
			cmd = sub
		}
	}
	if cmd == nil {
		t.Fatal("workflow run subcommand not found")
	}

	set := flag.NewFlagSet("run", flag.ContinueOnError)
	for _, f := range cmd.Flags {
		if err := f.Apply(set); err != nil {
			t.Fatalf("failed to apply flag: %v", err)
		}
	}
	if err := set.Parse(args); err != nil {
		t.Fatalf("failed to parse %v: %v", args, err)
	}

	return cli.NewContext(cli.NewApp(), set, nil)
}

func TestWorkflowRunActionRequiresAWorkflowName(t *testing.T) {
	err := workflowRunAction(runContext(t))
	if err == nil || !strings.Contains(err.Error(), "workflow name is required") {
		t.Fatalf("expected a missing-name error, got: %v", err)
	}
}

// urfave/cli stops parsing flags at the first positional argument, so
// `workflow run ci --ref main` reaches the action with --ref unset and would
// silently run on the current branch. Refusing beats guessing (issue #266).
func TestWorkflowRunActionRefusesOptionsAfterTheName(t *testing.T) {
	err := workflowRunAction(runContext(t, "ci.yml", "--ref", "main"))
	if err == nil {
		t.Fatal("expected an error for options placed after the workflow name")
	}
	for _, want := range []string{"--ref main", "cidx workflow run --ref main ci.yml"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q should mention %q", err, want)
		}
	}
}

func TestWorkflowRunActionRejectsMalformedInputs(t *testing.T) {
	// The input check must fire before anything reaches for a repository or a
	// remote, so a typo is reported instantly.
	err := workflowRunAction(runContext(t, "--input", "dry_run", "ci.yml"))
	if err == nil || !strings.Contains(err.Error(), "key=value") {
		t.Fatalf("expected a key=value error, got: %v", err)
	}
}

func TestWorkflowRunHelpDocumentsFlagPlacement(t *testing.T) {
	var cmd *cli.Command
	for _, sub := range workflowCommand().Subcommands {
		if sub.Name == "run" {
			cmd = sub
		}
	}
	if cmd == nil {
		t.Fatal("workflow run subcommand not found")
	}

	help := strings.ToLower(cmd.Usage + "\n" + cmd.Description)
	for _, want := range []string{"workflow_dispatch", "--no-watch", "before the workflow name"} {
		if !strings.Contains(help, want) {
			t.Errorf("workflow run help should mention %q, got:\n%s", want, help)
		}
	}
}
