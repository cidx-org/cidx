package commands

// The hidden `action` tree was marked deprecated on 2026-04-09 (8103f20), and
// the announcement lived in the Usage line of a command that `--help` does not
// list. Nobody reads that, so the window never opened: for four months the tree
// ran in silence, and the only people who could have migrated -- the ones with
// it in a script -- had no way to learn it was going away (issue #235).
//
// This is what starts the window. The warning fires on the way through the
// parent command, on every invocation, and says the three things a deprecation
// has to say: that the command is deprecated, which command replaces it
// exactly, and when it disappears. Then it lets the command run -- removal is
// v3.0.0, not today.

import (
	"strings"

	"github.com/sirupsen/logrus"
	"github.com/urfave/cli/v2"
)

// actionRemovedIn is the release the deprecated tree disappears in. It is named
// in the warning, in the command's own Usage and in docs/reference/cli.md, so
// the deadline is something a user can read rather than a private plan.
const actionRemovedIn = "v3.0.0"

// actionReplacements maps each entry of the deprecated tree to the command that
// replaces it. Aliases are keys too, because a user types those as readily as
// the full name. TestEveryDeprecatedActionEntryNamesItsReplacement checks the
// table against the tree that actually ships, in both directions.
var actionReplacements = map[string]string{
	"commit-push-watch": "cidx cpw",
	"cpw":               "cidx cpw",
	"pr":                "cidx pr",
	"tag":               "cidx release tag",
	"release":           "cidx release",
	"artifact":          "cidx repo artifact",
}

// warnDeprecatedAction is the Before of the deprecated parent command. urfave
// runs it after parsing and before dispatching to the subcommand, which is the
// single point every invocation of the tree passes through -- and a command
// that carries subcommands is exactly the kind the flag-placement guard skips
// (#274), so the two hooks never contend for the same Before.
func warnDeprecatedAction(c *cli.Context) error {
	path := commandPath(c.Command, c.Args().Slice())
	if len(path) == 0 {
		return nil
	}

	replacement, ok := actionReplacements[path[0]]
	if !ok {
		// `help`, or a name that is not in the tree: urfave answers for it, and
		// naming a replacement for it would be a guess.
		return nil
	}

	// Both sides are spelled out, so the correspondence is read rather than
	// derived. logrus writes to stderr, which keeps the warning visible to
	// someone redirecting the command's output.
	logrus.Warnf("⚠️  '%s' is deprecated and will be removed in cidx %s -- use: '%s'",
		"cidx action "+strings.Join(path, " "),
		actionRemovedIn,
		strings.Join(append([]string{replacement}, path[1:]...), " "))

	// A deprecation, not a removal: the command still runs.
	return nil
}

// commandPath returns the leading tokens of args that name commands under cmd.
// It stops at the first token that does not -- a flag, or an argument -- so the
// warning quotes a command and never a user's quoted PR title.
func commandPath(cmd *cli.Command, args []string) []string {
	var path []string
	for _, arg := range args {
		sub := cmd.Command(arg)
		if sub == nil {
			break
		}
		path = append(path, arg)
		cmd = sub
	}
	return path
}
