package features

import (
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/cidx-org/cidx/v3/internal/commands"
	"github.com/cucumber/godog"
	"github.com/urfave/cli/v2"
)

// RegisterFlagPlacementSteps registers the steps for flags typed after a
// positional argument (issue #268).
//
// The steps type a command line against the tree cidx ships — commands.NewApp,
// the same constructor cmd/cidx runs (issue #317) — and exercise the guard the
// way urfave does: on the parsed context, through the command's Before hook.
// The action is never reached, so `repo pr create` can be specified here
// without opening a pull request.
func RegisterFlagPlacementSteps(ctx *godog.ScenarioContext, tc *TestContext) {
	ctx.When(`^I type the command line (.+)$`, tc.typeCommandLine)

	ctx.Then(`^cidx should refuse the line$`, tc.cidxShouldRefuseTheLine)
	ctx.Then(`^cidx should accept the line$`, tc.cidxShouldAcceptTheLine)
	ctx.Then(`^it should suggest (.+)$`, tc.itShouldSuggest)
}

func (tc *TestContext) typeCommandLine(line string) error {
	tokens, err := splitCommandLine(line)
	if err != nil {
		return err
	}
	if len(tokens) == 0 || tokens[0] != "cidx" {
		return fmt.Errorf("a command line starts with cidx, got %q", line)
	}

	app := commands.NewApp()
	cmd, args, err := commandTyped(app, tokens[1:])
	if err != nil {
		return err
	}

	// The context urfave would hand the command: its own flag set for the
	// tokens it parsed, and the parent holding them as they were typed — the
	// only copy where an explicit `--` survives.
	set := flag.NewFlagSet(cmd.Name, flag.ContinueOnError)
	set.SetOutput(io.Discard)
	for _, f := range cmd.Flags {
		if err := f.Apply(set); err != nil {
			return fmt.Errorf("failed to apply the flags of %q: %w", cmd.Name, err)
		}
	}
	if err := set.Parse(args); err != nil {
		return fmt.Errorf("failed to parse %v: %w", args, err)
	}

	parent := flag.NewFlagSet("cidx", flag.ContinueOnError)
	parent.SetOutput(io.Discard)
	if err := parent.Parse(append([]string{cmd.Name}, args...)); err != nil {
		return fmt.Errorf("failed to parse %v: %w", args, err)
	}

	tc.Output, tc.ExitCode = "", 0
	if err := cmd.Before(cli.NewContext(app, set, cli.NewContext(app, parent, nil))); err != nil {
		tc.Output, tc.ExitCode = err.Error(), 1
	}
	return nil
}

func (tc *TestContext) cidxShouldRefuseTheLine() error {
	if tc.ExitCode == 0 {
		return fmt.Errorf("the line was accepted")
	}
	return nil
}

func (tc *TestContext) cidxShouldAcceptTheLine() error {
	if tc.ExitCode != 0 {
		return fmt.Errorf("the line was refused:\n%s", tc.Output)
	}
	return nil
}

func (tc *TestContext) itShouldSuggest(line string) error {
	if !strings.Contains(tc.Output, line) {
		return fmt.Errorf("expected the refusal to suggest %q, got:\n%s", line, tc.Output)
	}
	return nil
}

// commandTyped walks the typed tokens down the real tree and returns the
// command they reach, with the arguments left for it. A line that no longer
// resolves is an error rather than a silent pass: that is the scenario telling
// its reader the CLI moved.
func commandTyped(app *cli.App, tokens []string) (*cli.Command, []string, error) {
	commands, path := app.Commands, app.Name

	var cmd *cli.Command
	for len(tokens) > 0 && len(commands) > 0 {
		next := lookupCommand(commands, tokens[0])
		if next == nil {
			break
		}
		cmd, tokens, commands, path = next, tokens[1:], next.Subcommands, path+" "+next.Name
	}

	if cmd == nil {
		return nil, nil, fmt.Errorf("no command of the cidx tree is named %q", tokens[0])
	}
	if len(cmd.Subcommands) > 0 {
		return nil, nil, fmt.Errorf("%q is a namespace, not a command that takes arguments", path)
	}
	return cmd, tokens, nil
}

func lookupCommand(commands []*cli.Command, name string) *cli.Command {
	for _, c := range commands {
		if c.HasName(name) {
			return c
		}
	}
	return nil
}

// splitCommandLine splits a typed line into tokens the way a shell would, which
// for these scenarios means honouring double quotes: a quoted PR title is one
// argument, and that is exactly what tells it apart from a flag.
func splitCommandLine(line string) ([]string, error) {
	var (
		tokens  []string
		current strings.Builder
		quoted  bool
		started bool
	)

	for _, r := range line {
		switch {
		case r == '"':
			quoted, started = !quoted, true
		case (r == ' ' || r == '\t') && !quoted:
			if started {
				tokens = append(tokens, current.String())
				current.Reset()
				started = false
			}
		default:
			current.WriteRune(r)
			started = true
		}
	}

	if quoted {
		return nil, fmt.Errorf("unbalanced quote in %q", line)
	}
	if started {
		tokens = append(tokens, current.String())
	}
	return tokens, nil
}
