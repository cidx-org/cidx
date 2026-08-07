package guards

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/cidx-org/cidx/v2/internal/commands"
	"github.com/urfave/cli/v2"
)

// suggestedCommand matches a cidx command line a message tells the user to run,
// and only that: inside backticks, or opening its own line. Prose mentioning
// the tool — "cidx does not read this key" — is not a suggestion and must not
// be read as one.
var suggestedCommand = regexp.MustCompile("`cidx ([^`]+)`" + `|^\s*cidx ([a-z].*)$`)

// TestEverySuggestedCommandExists is the standing guard behind issue #399.
//
// What it decides, and the reason it can: a suggestion is wrong when its first
// word *is* a real command that lives somewhere else in the tree. `cidx
// registry login` (#399) and `cidx vuln ...` (#239) are both that — commands
// that moved under a namespace while the messages kept the old spelling. A
// first word that names nothing anywhere is prose about the tool, not a
// suggestion, and is left alone: "cidx will not remove containers it does not
// own" is a sentence, and no mechanical rule tells a sentence from a command
// line except this one.
//
// Fifteen messages across the code and the docs told the user to run `cidx
// registry login dhi.io`. There is no such command — the tree carries it under
// `security` — so the suggestion answered "No help topic for 'registry'", at
// the exact moment someone is locked out of a registry and reading for a way
// back in. It was the documented spelling rather than a slip, which is what a
// guard is for: nobody re-reads a suggestion they have written.
//
// The check resolves each suggestion against the tree the binary runs
// (commands.NewApp, never a copy of it — #317), so a command that is renamed
// or moved under a namespace fails here rather than at the moment it is needed.
func TestEverySuggestedCommandExists(t *testing.T) {
	app := commands.NewApp()

	scanned, checked := 0, 0
	err := filepath.WalkDir(projectRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			name := d.Name()
			// path != projectRoot: the root is spelled "../..", whose base
			// begins with a dot and would skip the whole walk.
			if path != projectRoot && (strings.HasPrefix(name, ".") || name == "vendor" || name == "node_modules") {
				return fs.SkipDir
			}
			return nil
		}
		// Go sources only. A message in the code is what a user meets at the
		// moment something failed; the documentation legitimately names
		// commands that no longer exist, since the correspondence table of a
		// removal is exactly that (#235).
		if filepath.Ext(path) != ".go" {
			return nil
		}
		if strings.HasSuffix(path, "_test.go") {
			return nil
		}

		source, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		scanned++

		for i, line := range strings.Split(string(source), "\n") {
			// Comments explain what happened, and what happened includes
			// commands that have since moved — invocations.go recounts the
			// `cidx vuln` incident of #239 on purpose.
			if code, _, found := strings.Cut(line, "//"); found {
				line = code
			}

			for _, match := range suggestedCommand.FindAllStringSubmatch(line, -1) {
				words := commandWords(match[1] + match[2])
				if len(words) == 0 || !namesACommand(app.Commands, words[0]) {
					continue
				}
				checked++
				if resolves(app.Commands, words) {
					continue
				}
				t.Errorf("%s:%d suggests `cidx %s`, which the tree does not answer at that path — "+
					"%q exists elsewhere in it, so the suggestion sends the reader nowhere:\n  %s",
					path, i+1, strings.Join(words, " "), words[0], strings.TrimSpace(line))
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking the sources: %v", err)
	}

	if scanned == 0 || checked == 0 {
		t.Fatalf("scanned %d file(s) and checked %d suggestion(s) — the guard would pass vacuously", scanned, checked)
	}
}

// resolves reports whether the tree answers a command line, matching as many of
// its leading words as name commands. A suggestion ends in arguments and flags,
// which are not commands and stop the walk: `cidx pr create "title"` resolves
// on `pr create`.
func resolves(available []*cli.Command, words []string) bool {
	for _, word := range words {
		var found *cli.Command
		for _, candidate := range available {
			if candidate.HasName(word) {
				found = candidate
				break
			}
		}
		if found == nil {
			// The first word has to be a command; anything after it may be an
			// argument, and an argument is not this guard's business.
			return len(words) > 0 && word != words[0]
		}
		available = found.Subcommands
	}

	return true
}

// commandWords keeps the leading words of a suggestion that could name a
// command, stopping at the first that could not: a flag, an argument, a
// placeholder, or the rest of a sentence.
func commandWords(suggestion string) []string {
	var words []string
	for _, field := range strings.Fields(suggestion) {
		if !commandName.MatchString(field) {
			break
		}
		words = append(words, field)
	}

	return words
}

// commandName is the shape of a command in this tree: lowercase, hyphenated.
var commandName = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)

// namesACommand reports whether a word names a command anywhere in the tree, at
// any depth. That is what separates a path that moved from a word in a
// sentence.
func namesACommand(available []*cli.Command, word string) bool {
	for _, candidate := range available {
		if candidate.HasName(word) {
			return true
		}
		if namesACommand(candidate.Subcommands, word) {
			return true
		}
	}

	return false
}
