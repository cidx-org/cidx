package commands

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/urfave/cli/v2"
)

// deprecatedActionCommand returns the deprecated tree from the app that ships.
func deprecatedActionCommand(t *testing.T) *cli.Command {
	t.Helper()

	for _, cmd := range NewApp().Commands {
		if cmd.Name == "action" {
			return cmd
		}
	}
	t.Fatal("the deprecated 'action' tree is no longer in the command tree -- " +
		"if it was removed on purpose, delete action_deprecated.go, this file, and the " +
		"'cidx action' section of docs/reference/cli.md with it")
	return nil
}

// resolveCommand walks a path of command names through the tree.
func resolveCommand(commands []*cli.Command, path []string) *cli.Command {
	for _, cmd := range commands {
		if !cmd.HasName(path[0]) {
			continue
		}
		if len(path) == 1 {
			return cmd
		}
		return resolveCommand(cmd.Subcommands, path[1:])
	}
	return nil
}

// TestEveryDeprecatedActionEntryNamesItsReplacement is the census that keeps the
// correspondence honest: every entry of the deprecated tree, under every name it
// answers to, has a replacement -- and that replacement is a command the current
// tree really has. An entry added to either side without the other fails here
// (issue #235, same intent as the flag-placement census from #274).
func TestEveryDeprecatedActionEntryNamesItsReplacement(t *testing.T) {
	app := NewApp()
	action := deprecatedActionCommand(t)

	named := map[string]bool{}
	for _, cmd := range action.Subcommands {
		if cmd.Name == "help" {
			// urfave appends its own; it is not part of the deprecation.
			continue
		}
		for _, name := range cmd.Names() {
			named[name] = true

			replacement, ok := actionReplacements[name]
			if !ok {
				t.Errorf("'cidx action %s' has no replacement in actionReplacements", name)
				continue
			}

			path := strings.Fields(strings.TrimPrefix(replacement, "cidx "))
			if resolveCommand(app.Commands, path) == nil {
				t.Errorf("'cidx action %s' points at %q, which is not in the command tree", name, replacement)
			}
		}
	}

	for name := range actionReplacements {
		if !named[name] {
			t.Errorf("actionReplacements maps %q, which the deprecated tree no longer has", name)
		}
	}
}

// runDeprecated runs the real app and separates what the command printed from
// what was logged, so a test can tell the output apart from the warning.
func runDeprecated(t *testing.T, args ...string) (printed, logged string) {
	t.Helper()

	var out, log bytes.Buffer
	app := NewApp()
	app.Writer = &out
	app.ErrWriter = &out

	logrus.SetOutput(&log)
	t.Cleanup(func() { logrus.SetOutput(os.Stderr) })

	err := app.Run(append([]string{"cidx"}, args...))
	if err != nil {
		t.Fatalf("%v: %v", args, err)
	}
	return out.String(), log.String()
}

// TestDeprecatedActionWarnsAndStillRuns covers the whole point of the change:
// each entry of the tree names its own replacement, and the command it was
// asked to run still runs. `--help` is the invocation used because it reaches
// the command with no side effect on the repository.
func TestDeprecatedActionWarnsAndStillRuns(t *testing.T) {
	tests := []struct {
		args []string
		want string
	}{
		{
			args: []string{"action", "pr", "create", "--help"},
			want: "'cidx action pr create' is deprecated and will be removed in cidx v3.0.0 -- use: 'cidx pr create'",
		},
		{
			args: []string{"action", "cpw", "--help"},
			want: "'cidx action cpw' is deprecated and will be removed in cidx v3.0.0 -- use: 'cidx cpw'",
		},
		{
			args: []string{"action", "commit-push-watch", "--help"},
			want: "'cidx action commit-push-watch' is deprecated and will be removed in cidx v3.0.0 -- use: 'cidx cpw'",
		},
		{
			args: []string{"action", "tag", "list", "--help"},
			want: "'cidx action tag list' is deprecated and will be removed in cidx v3.0.0 -- use: 'cidx release tag list'",
		},
		{
			args: []string{"action", "release", "create", "--help"},
			want: "'cidx action release create' is deprecated and will be removed in cidx v3.0.0 -- use: 'cidx release create'",
		},
		{
			args: []string{"action", "artifact", "stats", "--help"},
			want: "'cidx action artifact stats' is deprecated and will be removed in cidx v3.0.0 -- use: 'cidx repo artifact stats'",
		},
		{
			// An alias of the replacement's own subcommand, and a tree two
			// levels deep on both sides.
			args: []string{"action", "release", "tag", "prepare", "--help"},
			want: "'cidx action release tag prepare' is deprecated and will be removed in cidx v3.0.0 -- use: 'cidx release tag prepare'",
		},
	}

	for _, tt := range tests {
		t.Run(strings.Join(tt.args, "_"), func(t *testing.T) {
			printed, logged := runDeprecated(t, tt.args...)

			if !strings.Contains(logged, tt.want) {
				t.Errorf("warning should read %q, got:\n%s", tt.want, logged)
			}
			// The command did its work despite the warning.
			if !strings.Contains(printed, "USAGE") {
				t.Errorf("%v printed no help, so the warning stopped it:\n%s", tt.args, printed)
			}
			// And the warning is not part of that work's output.
			if strings.Contains(printed, "deprecated and will be removed") {
				t.Errorf("the warning leaked into the command's output:\n%s", printed)
			}
		})
	}
}

// The warning has to survive `cidx action ... > file`: someone capturing the
// output of a script is exactly the user who still runs the deprecated tree.
// logrus writes to stderr and cidx never repoints it.
func TestDeprecationWarningGoesToStderr(t *testing.T) {
	if out := logrus.StandardLogger().Out; out != os.Stderr {
		t.Fatalf("logrus writes to %v, so the warning would follow a redirected stdout", out)
	}
}

// A command that is not part of the tree is left alone.
func TestUnknownActionSubcommandIsNotWarnedAbout(t *testing.T) {
	action := deprecatedActionCommand(t)
	if got := commandPath(action, []string{"nonexistent"}); len(got) != 0 {
		t.Errorf("expected no command path, got %v", got)
	}
	// `help` is urfave's, not ours: it has no replacement to name.
	if _, ok := actionReplacements["help"]; ok {
		t.Error("'help' should not be in actionReplacements")
	}
}

// The deprecation warning and the flag-placement guard (#274) are installed by
// different code on different commands -- the warning on the parent, the guard
// on the leaves it dispatches to. This is the invocation where both fire, and
// it checks that neither dropped the other.
func TestDeprecationWarningCoexistsWithFlagPlacementGuard(t *testing.T) {
	var out, log bytes.Buffer
	app := NewApp()
	app.Writer = &out
	app.ErrWriter = &out

	logrus.SetOutput(&log)
	t.Cleanup(func() { logrus.SetOutput(os.Stderr) })

	err := app.Run([]string{"cidx", "action", "pr", "create", "feat: something", "--dry-run"})
	if err == nil {
		t.Fatal("the flag-placement guard should still refuse a flag placed after an argument")
	}
	if !strings.Contains(err.Error(), "--dry-run") {
		t.Errorf("the guard's error should name the misplaced flag, got: %v", err)
	}
	if !strings.Contains(log.String(), "'cidx action pr create' is deprecated") {
		t.Errorf("the deprecation warning should still fire, got:\n%s", log.String())
	}
}

// The deadline is named where a user meets it: in the warning (covered above)
// and in the command's own help.
func TestDeprecatedActionHelpNamesTheRemovalRelease(t *testing.T) {
	if usage := deprecatedActionCommand(t).Usage; !strings.Contains(usage, actionRemovedIn) {
		t.Errorf("the command's Usage should name %s, got: %s", actionRemovedIn, usage)
	}
}
