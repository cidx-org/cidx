package presets

import (
	"os"
	"strings"
	"testing"
)

// TestEveryOverrideKeyIsDocumented fails when overrideKeys and the reference
// table disagree.
//
// The table in docs/reference/container-options.md is where someone looks to
// find out what a `[containers.<name>]` section accepts -- there is nowhere
// else. A key missing from it works and is undiscoverable; a key listed after
// being removed is worse, because cidx.toml rejects it and the documentation
// says it should work. Neither shows up in any run.
//
// Sibling of TestOverrideKeysMatchTheReaders, which keeps the same list honest
// against the code that reads it (#371, #434).
func TestEveryOverrideKeyIsDocumented(t *testing.T) {
	const doc = "../../docs/reference/container-options.md"

	source, err := os.ReadFile(doc)
	if err != nil {
		t.Fatalf("failed to read %s: %v", doc, err)
	}
	// Only the table counts. Scanning the whole page passes on a key that the
	// prose happens to mention -- which is how the first version of this test
	// reported green while the row it was meant to require was gone.
	documented := map[string]bool{}
	for _, row := range strings.Split(string(source), "\n") {
		if !strings.HasPrefix(row, "| `") {
			continue
		}
		documented[strings.Trim(strings.Fields(row)[1], "`")] = true
	}
	if len(documented) == 0 {
		t.Fatalf("no table row found in %s -- this guard would pass by reading nothing", doc)
	}

	for key := range overrideKeys {
		if !documented[key] {
			t.Errorf("`[containers.<name>] %s` is accepted but the table in %s does not list it -- "+
				"a key that works and cannot be found is one nobody uses", key, doc)
		}
	}

	// And the other direction: the table naming a key cidx would reject.
	for key := range documented {
		if !overrideKeys[key] {
			t.Errorf("%s documents `%s`, which cidx.toml rejects", doc, key)
		}
	}
}
