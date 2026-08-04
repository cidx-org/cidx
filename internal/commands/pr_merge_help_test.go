package commands

import (
	"strings"
	"testing"
)

// `cidx pr merge` merges without asking for confirmation and offers no
// --yes/--force flag. That is deliberate (it has to stay scriptable), so the
// help must say so instead of leaving the flag list and the behaviour at odds.
// Guards issue #217.
func TestPRMergeHelpDocumentsNonInteractiveBehaviour(t *testing.T) {
	var help string
	for _, sub := range prCommand().Subcommands {
		if sub.Name == "merge" {
			help = strings.ToLower(sub.Usage + "\n" + sub.Description)
		}
	}
	if help == "" {
		t.Fatal("pr merge subcommand not found")
	}

	for _, want := range []string{"non-interactive", "no confirmation", "--dry-run"} {
		if !strings.Contains(help, want) {
			t.Errorf("pr merge help should mention %q, got:\n%s", want, help)
		}
	}
}
