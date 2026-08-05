package commands

import (
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/urfave/cli/v2"
)

// aliasTarget reads the command an alias stands for out of its own usage line,
// which is where the tree already states it: "Alias for 'repo cpw'".
var aliasTarget = regexp.MustCompile(`^Alias for '([^']+)'$`)

// TestAliasesOfferTheSameFlagsAsTheCommandTheyAlias.
//
// The hidden top-level aliases are the spellings this repository actually uses
// all day -- `cidx cpw`, `cidx pr`, `cidx workflow`. `cpw` used to restate its
// flag list instead of taking the real command's, and a restated list drifts:
// `--no-verify` (#307) landed on `repo cpw` while `cidx cpw` answered "flag
// provided but not defined". The fix on the command nobody types is not a fix.
//
// So the shortcut is held to offering exactly what the long form offers. Any
// flag added to an aliased command from here on has to reach both.
func TestAliasesOfferTheSameFlagsAsTheCommandTheyAlias(t *testing.T) {
	app := NewApp()

	aliases := 0
	for _, alias := range app.Commands {
		match := aliasTarget.FindStringSubmatch(alias.Usage)
		if match == nil {
			continue
		}
		aliases++

		path := strings.Fields(match[1])
		target := resolveCommand(app.Commands, path)
		if target == nil {
			t.Errorf("%q claims to alias %q, which is not in the command tree", alias.Name, match[1])
			continue
		}

		compareFlags(t, alias.Name, alias, target)
	}

	if aliases == 0 {
		t.Fatal("no top-level alias found — the guard would pass vacuously")
	}
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

// TestTheDeprecatedActionTreeIsGone.
//
// `cidx action ...` was hidden on 2026-04-09 (8103f20), warned on every
// invocation from #235 on, and is removed in v3.0.0. The tree it wrapped is
// reachable in full under `repo`, `release` and `security`, so bringing it back
// would re-open a deprecation that has already had its window -- and it would
// come back the way it left, hidden, where `--help` would not show it and only
// this guard would notice.
func TestTheDeprecatedActionTreeIsGone(t *testing.T) {
	if cmd := resolveCommand(NewApp().Commands, []string{"action"}); cmd != nil {
		t.Error("'cidx action' is back in the command tree. It was removed in v3.0.0 " +
			"(issue #235); every subcommand it wrapped is reachable under 'repo', " +
			"'release' or 'security'.")
	}
}

// compareFlags checks one command against the one it aliases, then walks into
// the subcommands both declare.
func compareFlags(t *testing.T, path string, alias, target *cli.Command) {
	t.Helper()

	got, want := flagNames(alias.Flags), flagNames(target.Flags)
	if !slices.Equal(got, want) {
		t.Errorf("cidx %s offers %v, but the command it aliases offers %v.\n"+
			"A shortcut that is missing a flag answers \"flag provided but not defined\"\n"+
			"for something the long form accepts (#307). Take the flags from the real\n"+
			"command instead of restating them.", path, got, want)
	}

	for _, sub := range alias.Subcommands {
		for _, targetSub := range target.Subcommands {
			if sub.Name == targetSub.Name {
				compareFlags(t, path+" "+sub.Name, sub, targetSub)
				break
			}
		}
	}
}

// flagNames lists a command's flags by their primary name, sorted so the
// comparison is about what is offered and not about declaration order.
func flagNames(flags []cli.Flag) []string {
	names := make([]string, 0, len(flags))
	for _, flag := range flags {
		names = append(names, flag.Names()[0])
	}
	slices.Sort(names)

	return names
}
