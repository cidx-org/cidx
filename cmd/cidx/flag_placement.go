package main

// Flags placed after a positional argument (issue #268).
//
// urfave/cli v2 hands the command line to the standard flag package, which
// stops parsing at the first token that is not a flag. Everything after it
// arrives as an inert argument, and a command that does not read it says
// nothing: `cidx workflow run ci.yml --ref main` ran on the current branch,
// `cidx workflow list ci.yml --limit 3` listed ten runs. The dangerous shape is
// `--dry-run` dropped that way -- the command then does the real thing while
// reporting the run the user asked to preview, which is the same class of bug
// as a gate that is always green.
//
// The guard below refuses instead of reordering. Reordering means deciding, on
// a line the parser already gave up on, which token of `--ref main` is a flag
// and which is an argument -- unknown flags, values that start with a dash and
// `--` all make that a guess, and guessing on a command line is precisely what
// this bug is. Refusing costs one retry and prints the line to retry with.
//
// urfave/cli v3 parses flags in any position and would remove the trap at the
// root; that is a dependency migration, not a fix, and is tracked separately.

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/urfave/cli/v2"
)

// installFlagPlacementGuard wires the check into every runnable command of the
// tree. It is applied centrally, from newApp, so a command added later is
// covered without its author knowing this file exists -- the per-command guard
// `workflow run` carried since #266 is the shape that does not scale.
func installFlagPlacementGuard(app *cli.App) {
	guardFlagPlacement(app.Name, app.Commands)
}

func guardFlagPlacement(path string, commands []*cli.Command) {
	for _, cmd := range commands {
		cmdPath := path + " " + cmd.Name

		// A command that has subcommands dispatches on its first argument, and
		// urfave runs Before ahead of that dispatch: the arguments it sees are
		// the subcommand's, which guards them itself.
		if len(cmd.Subcommands) > 0 {
			guardFlagPlacement(cmdPath, cmd.Subcommands)
			continue
		}

		inner := cmd.Before
		cmd.Before = func(c *cli.Context) error {
			if err := checkFlagPlacement(cmdPath, c); err != nil {
				return err
			}
			if inner != nil {
				return inner(c)
			}
			return nil
		}
	}
}

// checkFlagPlacement refuses the arguments that were meant to be flags. By the
// time an action runs they are ordinary positional tokens, so every command
// that does not read them ignores them in silence.
func checkFlagPlacement(cmdPath string, c *cli.Context) error {
	args := c.Args().Slice()

	first := -1
	for i, arg := range args {
		if looksLikeFlag(arg) {
			first = i
			break
		}
	}
	if first < 0 {
		return nil
	}

	// `--` is how a user says "take the rest literally", and that is a legitimate
	// answer to this very error. Parsing consumes the terminator, so the only
	// place left to see it is the tokens as they were typed.
	for _, arg := range rawArgs(c) {
		if arg == "--" {
			return nil
		}
	}

	// The corrected line is the tail from the first flag, then the arguments it
	// was buried behind: `list ci.yml --limit 3` reads back as
	// `list --limit 3 ci.yml`.
	corrected := append(append([]string{}, args[first:]...), args[:first]...)
	return fmt.Errorf("%q comes after an argument, where it is not parsed -- put flags first:\n  %s %s",
		args[first], cmdPath, shellJoin(corrected))
}

// looksLikeFlag reports whether a token would have been parsed as a flag had it
// come first. A token carrying whitespace never would -- the shell splits the
// line, so a flag is always one word -- and neither would a negative number.
// Both are arguments a user quoted on purpose, a PR title or a threshold.
func looksLikeFlag(token string) bool {
	if len(token) < 2 || !strings.HasPrefix(token, "-") || token == "--" {
		return false
	}
	if strings.ContainsAny(token, " \t\n") {
		return false
	}
	if _, err := strconv.ParseFloat(token, 64); err == nil {
		return false
	}
	return true
}

// rawArgs returns the tokens the command was handed, before its own flag set
// consumed them. urfave passes them down as the tail of the parent context, and
// that is the only copy where `--` survives.
func rawArgs(c *cli.Context) []string {
	lineage := c.Lineage()
	if len(lineage) < 2 {
		return c.Args().Slice()
	}
	// The parent stopped parsing on the command name, which heads the tail it
	// passed down.
	return lineage[1].Args().Tail()
}

// shellJoin renders tokens the way they would have to be typed back.
func shellJoin(tokens []string) string {
	quoted := make([]string, len(tokens))
	for i, token := range tokens {
		if token == "" || strings.ContainsAny(token, " \t") {
			quoted[i] = strconv.Quote(token)
			continue
		}
		quoted[i] = token
	}
	return strings.Join(quoted, " ")
}
