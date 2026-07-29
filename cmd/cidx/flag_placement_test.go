package main

import (
	"flag"
	"strings"
	"testing"

	"github.com/urfave/cli/v2"
)

// runnableCommand is a command that reaches an action, with the path it is
// reached by.
type runnableCommand struct {
	path string
	cmd  *cli.Command
}

// runnableCommands walks the real tree the way the guard installer does: a
// command with subcommands dispatches, a command without one acts.
func runnableCommands(path string, commands []*cli.Command) []runnableCommand {
	var found []runnableCommand
	for _, cmd := range commands {
		cmdPath := path + " " + cmd.Name
		if len(cmd.Subcommands) > 0 {
			found = append(found, runnableCommands(cmdPath, cmd.Subcommands)...)
			continue
		}
		found = append(found, runnableCommand{path: cmdPath, cmd: cmd})
	}
	return found
}

// contextFor builds the context urfave would hand cmd for these arguments, so
// the guard is exercised without an app, a repository or a network call.
func contextFor(t *testing.T, cmd *cli.Command, args ...string) *cli.Context {
	t.Helper()

	set := flag.NewFlagSet(cmd.Name, flag.ContinueOnError)
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

// TestEveryRunnableCommandRefusesFlagsAfterArguments is the census: it walks
// the tree cidx actually ships and checks that every command reachable by an
// action refuses a flag placed behind an argument. A command added tomorrow
// fails here until it is covered, which is what keeps the fix alive through the
// next reorganisation (issue #268, same intent as
// TestCatalogueImagesArePinnedByDigest).
//
// Before is the hook on purpose: it runs after parsing and before the action,
// and a test may call it without any command doing real work.
func TestEveryRunnableCommandRefusesFlagsAfterArguments(t *testing.T) {
	app := newApp()
	commands := runnableCommands(app.Name, app.Commands)

	// The walk finding nothing would make this test vacuously green.
	if len(commands) < 40 {
		t.Fatalf("the walk found %d runnable commands, the tree has far more", len(commands))
	}

	for _, rc := range commands {
		t.Run(strings.ReplaceAll(rc.path, " ", "_"), func(t *testing.T) {
			if rc.cmd.Before == nil {
				t.Fatalf("%q has no flag-placement guard", rc.path)
			}

			err := rc.cmd.Before(contextFor(t, rc.cmd, "argument", "--flag-after-argument"))
			if err == nil {
				t.Fatalf("%q accepted a flag placed after an argument", rc.path)
			}
			for _, want := range []string{"--flag-after-argument", rc.path} {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("%q: error %q should mention %q", rc.path, err, want)
				}
			}
		})
	}
}

// commandAt returns the command reached by the given path in the real tree.
func commandAt(t *testing.T, path ...string) *cli.Command {
	t.Helper()

	app := newApp()
	for _, rc := range runnableCommands(app.Name, app.Commands) {
		if rc.path == app.Name+" "+strings.Join(path, " ") {
			return rc.cmd
		}
	}
	t.Fatalf("command %q not found", strings.Join(path, " "))
	return nil
}

func TestFlagPlacementCases(t *testing.T) {
	tests := []struct {
		name    string
		command []string
		args    []string
		want    string // "" means the arguments are accepted
	}{
		{
			// The invocation of #266: --ref reached the action unset and the
			// workflow ran on the current branch.
			name:    "flag with a value after the argument",
			command: []string{"workflow", "run"},
			args:    []string{"ci.yml", "--ref", "main"},
			want:    "cidx workflow run --ref main ci.yml",
		},
		{
			name:    "flag with a value before the argument",
			command: []string{"workflow", "run"},
			args:    []string{"--ref", "main", "ci.yml"},
		},
		{
			name:    "short flag after the argument",
			command: []string{"workflow", "list"},
			args:    []string{"ci.yml", "-n", "3"},
			want:    "cidx workflow list -n 3 ci.yml",
		},
		{
			name:    "boolean flag after the argument",
			command: []string{"check", "workflow"},
			args:    []string{"ci", "--verbose"},
			want:    "cidx check workflow --verbose ci",
		},
		{
			// The worst case: the flag that makes the command do nothing is the
			// one silently dropped, so the command does everything.
			name:    "dry-run after the argument",
			command: []string{"repo", "pr", "create"},
			args:    []string{"feat: something", "--dry-run"},
			want:    `cidx repo pr create --dry-run "feat: something"`,
		},
		{
			name:    "flag=value after the argument",
			command: []string{"workflow", "list"},
			args:    []string{"ci.yml", "--limit=3"},
			want:    "cidx workflow list --limit=3 ci.yml",
		},
		{
			// A command with no flags of its own still drops the global ones.
			name:    "global flag after the argument",
			command: []string{"preset", "info"},
			args:    []string{"trivy", "--verbose"},
			want:    "cidx preset info --verbose trivy",
		},
		{
			name:    "no arguments at all",
			command: []string{"repo", "pr", "create"},
			args:    nil,
		},
		{
			name:    "arguments only",
			command: []string{"workflow", "run"},
			args:    []string{"ci.yml"},
		},
		{
			// A flag named inside a quoted title is one word of that title, not
			// a flag: it does not start with a dash.
			name:    "a flag named inside a quoted title",
			command: []string{"repo", "pr", "create"},
			args:    []string{"fix: --dry-run is no longer ignored"},
		},
		{
			// A quoted argument that starts with a dash carries whitespace; no
			// flag ever does, the shell splits on it.
			name:    "a quoted argument that starts with a dash",
			command: []string{"repo", "pr", "create"},
			args:    []string{"title", "-- reworked parser"},
		},
		{
			name:    "a negative number",
			command: []string{"workflow", "list"},
			args:    []string{"ci.yml", "-3"},
		},
		{
			name:    "a lone dash",
			command: []string{"workflow", "list"},
			args:    []string{"ci.yml", "-"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := commandAt(t, tt.command...)
			err := cmd.Before(contextFor(t, cmd, tt.args...))

			if tt.want == "" {
				if err != nil {
					t.Fatalf("%v should be accepted, got: %v", tt.args, err)
				}
				return
			}
			if err == nil {
				t.Fatalf("%v should be refused", tt.args)
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Errorf("error should suggest %q, got:\n%v", tt.want, err)
			}
		})
	}
}

// A `--` terminator is the documented way to pass an argument that starts with
// a dash, and refusing it would leave the user with no way out. Parsing
// consumes it, so the guard reads the tokens as they were typed -- which only
// a real run reproduces.
func TestExplicitTerminatorIsHonoured(t *testing.T) {
	cmd := commandAt(t, "workflow", "list")

	set := flag.NewFlagSet("list", flag.ContinueOnError)
	for _, f := range cmd.Flags {
		if err := f.Apply(set); err != nil {
			t.Fatalf("failed to apply flag: %v", err)
		}
	}
	if err := set.Parse([]string{"--", "--limit"}); err != nil {
		t.Fatalf("failed to parse: %v", err)
	}

	// The tokens as typed live in the parent context; parsing dropped the `--`.
	parent := flag.NewFlagSet("workflow", flag.ContinueOnError)
	if err := parent.Parse([]string{"list", "--", "--limit"}); err != nil {
		t.Fatalf("failed to parse: %v", err)
	}
	app := cli.NewApp()
	ctx := cli.NewContext(app, set, cli.NewContext(app, parent, nil))

	if got := ctx.Args().Slice(); len(got) != 1 || got[0] != "--limit" {
		t.Fatalf("expected the terminator to be consumed, got %v", got)
	}
	if err := cmd.Before(ctx); err != nil {
		t.Fatalf("an explicit -- should be honoured, got: %v", err)
	}
}

// The guard must not replace a command's own Before.
func TestGuardChainsAnExistingBefore(t *testing.T) {
	called := false
	cmd := &cli.Command{
		Name:   "leaf",
		Before: func(*cli.Context) error { called = true; return nil },
		Action: func(*cli.Context) error { return nil },
	}
	guardFlagPlacement("cidx", []*cli.Command{cmd})

	if err := cmd.Before(contextFor(t, cmd, "argument")); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Error("the command's own Before was dropped")
	}

	called = false
	if err := cmd.Before(contextFor(t, cmd, "argument", "--flag")); err == nil {
		t.Error("the guard should refuse a flag after an argument")
	}
	if called {
		t.Error("the command's own Before ran despite the refusal")
	}
}
