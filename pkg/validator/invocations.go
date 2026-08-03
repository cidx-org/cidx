package validator

// Stale cidx invocations inside workflow YAML (issue #239).
//
// The incident: `security-audit.yml` kept calling `cidx vuln ...` for 8 days
// after that subcommand moved under `cidx security vuln`. Every nightly run
// died in 12 seconds, and nothing local said a word — `cidx validate` looked
// at the config only, `cidx check drift` compares phases and triggers, not the
// commands inside the steps.
//
// The check below closes that gap: it walks the `run:` steps of the workflow
// files, extracts the cidx invocations, and resolves each one against the live
// urfave/cli command tree (no hardcoded command list — a hardcoded list would
// go stale at the next reorganisation, which is exactly the bug).
//
// It is deliberately a simple check, not a shell parser. It stays silent
// whenever the shell is doing something it cannot read with certainty:
//
//   - invocations through a variable (`$CIDX run ci`) or with an expanded
//     token in command position (`cidx ${{ matrix.cmd }}`)
//   - heredoc bodies (`cat <<'EOF' … EOF`) — that is data, not commands
//   - comments, and anything after a `#` token
//   - a leading environment assignment (`FOO=bar cidx run ci`)
//   - quoted command names (`"./bin/cidx" run ci`)
//   - unknown flags before the subcommand, which would make the position of
//     the subcommand unknowable
//   - a token that is not a subcommand of a command that has an action of its
//     own — such a command consumes its own arguments, so we cannot tell an
//     argument from a typo
//
// False negatives are acceptable, false accusations are not.

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/urfave/cli/v2"
	"gopkg.in/yaml.v3"
)

// Invocation is a cidx command line found in a workflow step.
type Invocation struct {
	File string   // workflow file the invocation was found in
	Line int      // 1-based line in that file
	Step string   // name of the step, when the workflow gives one
	Args []string // arguments passed to cidx, in order
}

// String renders the invocation the way it would be typed.
func (i Invocation) String() string {
	return strings.TrimSpace("cidx " + strings.Join(i.Args, " "))
}

// StaleInvocation is an invocation that no longer resolves against the CLI.
type StaleInvocation struct {
	Invocation
	Reason string
}

// WorkflowInvocations returns every cidx invocation found in the workflow files
// of dir, in file then line order. A missing directory is not an error: there
// is simply nothing to check.
func WorkflowInvocations(dir string) ([]Invocation, error) {
	files, err := workflowFiles(dir)
	if err != nil {
		return nil, err
	}

	var invocations []Invocation
	for _, file := range files {
		found, err := fileInvocations(file)
		if err != nil {
			return nil, err
		}
		invocations = append(invocations, found...)
	}
	return invocations, nil
}

// workflowFiles lists the YAML files of dir, sorted, so results are stable.
func workflowFiles(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to read %s: %w", dir, err)
	}

	var files []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if ext := filepath.Ext(e.Name()); ext == ".yml" || ext == ".yaml" {
			files = append(files, filepath.Join(dir, e.Name()))
		}
	}
	sort.Strings(files)
	return files, nil
}

// stepsYAML is a minimal view of a workflow: the run scripts and where they
// are. It keeps the run script as a yaml.Node to report real line numbers.
type stepsYAML struct {
	Jobs map[string]struct {
		Steps []struct {
			Name string    `yaml:"name"`
			Run  yaml.Node `yaml:"run"`
		} `yaml:"steps"`
	} `yaml:"jobs"`
}

// fileInvocations extracts the cidx invocations of a single workflow file.
func fileInvocations(file string) ([]Invocation, error) {
	data, err := os.ReadFile(file)
	if err != nil {
		return nil, fmt.Errorf("failed to read workflow file: %w", err)
	}

	var doc stepsYAML
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("failed to parse %s: %w", file, err)
	}

	var invocations []Invocation
	for _, job := range doc.Jobs {
		for _, step := range job.Steps {
			if step.Run.Value == "" {
				continue
			}
			// A plain scalar starts on the `run:` line; a block scalar starts
			// on the line after the `|` indicator.
			base := step.Run.Line - 1
			if step.Run.Style == yaml.LiteralStyle || step.Run.Style == yaml.FoldedStyle {
				base = step.Run.Line
			}
			for _, inv := range ExtractInvocations(step.Run.Value) {
				inv.File = file
				inv.Line += base
				inv.Step = step.Name
				invocations = append(invocations, inv)
			}
		}
	}

	// Jobs come out of a Go map, so sort before returning (see #233).
	sort.Slice(invocations, func(i, j int) bool {
		return invocations[i].Line < invocations[j].Line
	})
	return invocations, nil
}

var (
	// A heredoc redirection: the `<<` must open a token, so the GitHub Actions
	// multiline-output idiom (`echo "value<<EOF" >> $GITHUB_OUTPUT`) is not
	// mistaken for one.
	heredocStart = regexp.MustCompile(`(^|\s)<<-?\s*['"]?(\w+)['"]?(\s|$)`)

	// Where one shell command ends and the next begins. `$(` opens a command
	// substitution, so what follows is a command too.
	commandSeparator = regexp.MustCompile(`\$\(|&&|\|\||[|;()&{}]`)

	// A redirection: `> log`, `2>/dev/null`, `>>$GITHUB_OUTPUT`.
	redirection = regexp.MustCompile(`^[0-9&]*[<>]`)

	// Words that may sit in front of a command without being the command.
	commandPrefixes = map[string]bool{
		"!": true, "if": true, "elif": true, "then": true, "else": true,
		"while": true, "until": true, "do": true, "time": true,
		"sudo": true, "exec": true,
	}
)

// ExtractInvocations returns the cidx invocations of a shell script, with Line
// set to the 1-based line of the script each was found on.
func ExtractInvocations(script string) []Invocation {
	var invocations []Invocation

	lines := strings.Split(script, "\n")
	for i := 0; i < len(lines); i++ {
		// A heredoc body is data: skip to its delimiter, or to the end when
		// the delimiter never comes (rather than read the body as commands).
		if m := heredocStart.FindStringSubmatch(strings.TrimRight(lines[i], " \t")); m != nil {
			for i++; i < len(lines) && strings.TrimSpace(lines[i]) != m[2]; i++ {
			}
			continue
		}

		// Join continuation lines, reporting the line the command starts on.
		start, line := i, lines[i]
		for strings.HasSuffix(strings.TrimRight(line, " \t"), `\`) && i+1 < len(lines) {
			line = strings.TrimSuffix(strings.TrimRight(line, " \t"), `\`) + " " + lines[i+1]
			i++
		}

		for _, segment := range commandSeparator.Split(line, -1) {
			if args, ok := cidxArgs(segment); ok {
				invocations = append(invocations, Invocation{Line: start + 1, Args: args})
			}
		}
	}
	return invocations
}

// cidxArgs reports whether a single shell command invokes cidx, and with which
// arguments. `go build -o bin/cidx ./cmd/cidx` is not an invocation; `go run
// ./cmd/cidx run ci` is.
func cidxArgs(command string) ([]string, bool) {
	fields := strings.Fields(command)

	// Drop a trailing comment or redirection: neither belongs to the command.
	for i, f := range fields {
		if strings.HasPrefix(f, "#") || redirection.MatchString(f) {
			fields = fields[:i]
			break
		}
	}

	for len(fields) > 0 && commandPrefixes[fields[0]] {
		fields = fields[1:]
	}

	switch {
	case len(fields) >= 1 && isCidxBinary(fields[0]):
		return fields[1:], true
	case len(fields) >= 3 && fields[0] == "go" && fields[1] == "run" && isCidxBinary(fields[2]):
		return fields[3:], true
	}
	return nil, false
}

// isCidxBinary matches the forms cidx is really called by: `cidx`, `bin/cidx`,
// `./bin/cidx`, `./cmd/cidx`.
func isCidxBinary(token string) bool {
	return path.Base(token) == "cidx"
}

// Resolve walks args down app's command tree and returns why they do not
// resolve, or "" when they do — including when the check cannot tell.
//
// The root is always judged: cidx dispatches to a command, and urfave replaces
// a nil root action with the help printer at run time (which exits non-zero on
// an unknown topic — the 12-second failure of #239), so app.Action says nothing
// about whether the root accepts free arguments.
func Resolve(app *cli.App, args []string) string {
	return resolve(args, app.Commands, app.Flags, false, app.Name)
}

// RunTarget returns the phase, tool or pipeline named by a `cidx run <target>`
// invocation, and "" for anything else — another subcommand, or a `run` whose
// target cannot be read with certainty.
//
// `check workflow` used to look for the literal substring "cidx run " in the
// step, which lost the phase as soon as a flag sat between the binary and the
// subcommand: `./bin/cidx --verbose run test` reported no phase at all, so the
// command claimed a phase was missing from a workflow that runs it (issue #233).
// Reading the command line is what the parser above already does, flags
// included, so `run` is resolved the same way `validate` resolves every other
// invocation.
//
// It inherits the refusals of Resolve: a target the shell would rewrite, or
// flags that cannot be parsed with certainty, yield no target rather than a
// wrong one.
func RunTarget(app *cli.App, args []string) string {
	args, stop := skipFlags(args, app.Flags)
	if stop || len(args) == 0 {
		return ""
	}

	// Looked up in the live tree rather than compared to "run", so the aliases
	// of the command are honoured and a rename cannot leave this behind.
	cmd := lookup(app.Commands, args[0])
	if cmd == nil || cmd.Name != "run" {
		return ""
	}

	// Flags may also sit between the subcommand and its argument:
	// `cidx run --dry-run security`.
	args, stop = skipFlags(args[1:], cmd.Flags)
	if stop || len(args) == 0 {
		return ""
	}

	target := args[0]
	if strings.ContainsAny(target, "$`'\"*?") || strings.HasPrefix(target, "-") {
		return ""
	}
	return target
}

func resolve(args []string, commands []*cli.Command, flags []cli.Flag, hasAction bool, cmdPath string) string {
	for {
		// A command without subcommands takes arguments, not commands.
		if len(commands) == 0 {
			return ""
		}

		var stop bool
		if args, stop = skipFlags(args, flags); stop {
			return ""
		}
		if len(args) == 0 {
			return ""
		}

		name := args[0]
		// Anything the shell would rewrite, or a flag we failed to recognise,
		// is not a command name we can judge.
		if strings.ContainsAny(name, "$`'\"*?") || strings.HasPrefix(name, "-") {
			return ""
		}

		sub := lookup(commands, name)
		if sub == nil {
			if hasAction {
				// The command handles its own arguments — this may be one.
				return ""
			}
			return fmt.Sprintf("unknown command %q for %q%s", name, cmdPath, didYouMean(commands, name, cmdPath))
		}

		args, commands, flags, hasAction, cmdPath = args[1:], sub.Subcommands, sub.Flags, sub.Action != nil, cmdPath+" "+name
	}
}

// skipFlags consumes the flags in front of a subcommand. It reports stop when
// the flags cannot be parsed with certainty, since the position of the
// subcommand then becomes a guess.
func skipFlags(args []string, flags []cli.Flag) (rest []string, stop bool) {
	for len(args) > 0 && len(args[0]) > 1 && strings.HasPrefix(args[0], "-") {
		if args[0] == "--" {
			return args[1:], false
		}
		name := strings.TrimLeft(args[0], "-")
		if i := strings.Index(name, "="); i >= 0 {
			args = args[1:]
			continue
		}

		flag := lookupFlag(flags, name)
		if flag == nil {
			return args, true
		}
		doc, ok := flag.(cli.DocGenerationFlag)
		if !ok {
			return args, true
		}
		if doc.TakesValue() {
			if len(args) < 2 {
				return args, true
			}
			args = args[2:]
			continue
		}
		args = args[1:]
	}
	return args, false
}

func lookup(commands []*cli.Command, name string) *cli.Command {
	for _, c := range commands {
		if c.HasName(name) {
			return c
		}
	}
	return nil
}

func lookupFlag(flags []cli.Flag, name string) cli.Flag {
	for _, f := range flags {
		for _, n := range f.Names() {
			if n == name {
				return f
			}
		}
	}
	return nil
}

// didYouMean points at the command when it merely moved elsewhere in the tree —
// which is what happened to `cidx vuln`. The search starts at the level that
// failed, so a root-level miss searches the whole tree.
func didYouMean(commands []*cli.Command, name, cmdPath string) string {
	var found []string
	var walk func(cmds []*cli.Command, prefix string)
	walk = func(cmds []*cli.Command, prefix string) {
		for _, c := range cmds {
			// Hidden branches are aliases and deprecated paths — pointing at
			// them would send the reader to the wrong place.
			if c.Hidden {
				continue
			}
			if c.HasName(name) {
				found = append(found, prefix+" "+c.Name)
			}
			walk(c.Subcommands, prefix+" "+c.Name)
		}
	}
	walk(commands, cmdPath)

	if len(found) == 0 {
		return ""
	}
	sort.Strings(found)
	return fmt.Sprintf(" — did you mean %q?", found[0])
}
