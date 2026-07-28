package validator

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/urfave/cli/v2"
)

// testApp mirrors the shape of the real tree: namespaces without an action of
// their own, leaf commands that take arguments, a hidden alias that handles its
// own arguments, and `vuln` living under `security` — the move that caused #239.
func testApp() *cli.App {
	noop := func(*cli.Context) error { return nil }
	return &cli.App{
		Name: "cidx",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "config", Aliases: []string{"c"}},
			&cli.BoolFlag{Name: "verbose"},
		},
		Commands: []*cli.Command{
			{Name: "run", Action: noop, Flags: []cli.Flag{&cli.BoolFlag{Name: "dry-run"}}},
			{Name: "validate", Action: noop},
			{Name: "check", Subcommands: []*cli.Command{
				{Name: "drift", Action: noop},
				{Name: "workflow", Action: noop},
			}},
			{Name: "security", Subcommands: []*cli.Command{
				{Name: "vuln", Subcommands: []*cli.Command{
					{Name: "list", Action: noop},
					{Name: "check", Action: noop},
				}},
			}},
			{Name: "preset", Subcommands: []*cli.Command{
				{Name: "images", Action: noop},
				{Name: "scan-targets", Action: noop},
			}},
			// Hidden alias with both an action and subcommands.
			{Name: "pr", Hidden: true, Action: noop, Subcommands: []*cli.Command{
				{Name: "create", Action: noop},
			}},
		},
	}
}

func TestResolve(t *testing.T) {
	tests := []struct {
		name  string
		args  []string
		stale bool
	}{
		// Valid invocations
		{"leaf command with argument", []string{"run", "security"}, false},
		{"nested command", []string{"security", "vuln", "list"}, false},
		{"namespace subcommand", []string{"check", "drift"}, false},
		{"flag before the command", []string{"--verbose", "run", "ci"}, false},
		{"valued flag before the command", []string{"--config", "ci/pipeline.toml", "run", "ci"}, false},
		{"valued flag in --flag=value form", []string{"--config=ci/pipeline.toml", "validate"}, false},
		{"short valued flag", []string{"-c", "other.toml", "validate"}, false},
		{"flags after the command", []string{"run", "--dry-run", "ci"}, false},
		{"arguments of a leaf command", []string{"preset", "images", "--json"}, false},
		{"hidden alias", []string{"pr", "create", "feat: x"}, false},

		// Stale invocations — the class #239 is about
		{"command moved under a namespace", []string{"vuln", "list"}, true},
		{"command that never existed", []string{"nope"}, true},
		{"unknown subcommand of a namespace", []string{"check", "drifts"}, true},
		{"unknown subcommand two levels down", []string{"security", "vulns", "list"}, true},

		// Deliberately not judged
		{"no command at all", nil, false},
		{"flags only", []string{"--verbose"}, false},
		{"shell variable in command position", []string{"$COMMAND", "list"}, false},
		{"expanded expression in command position", []string{"${{", "matrix.cmd", "}}"}, false},
		{"unknown flag hides the command position", []string{"--unknown", "value", "list"}, false},
		{"command with an action of its own", []string{"pr", "whatever"}, false},
	}

	app := testApp()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reason := Resolve(app, tt.args)
			if tt.stale && reason == "" {
				t.Errorf("expected %v to be reported as stale", tt.args)
			}
			if !tt.stale && reason != "" {
				t.Errorf("expected %v to pass, got %q", tt.args, reason)
			}
		})
	}
}

// TestResolve_ReportsWhereTheCommandMoved covers the actual incident message:
// `cidx vuln list` must name the command and point at its new home.
func TestResolve_ReportsWhereTheCommandMoved(t *testing.T) {
	reason := Resolve(testApp(), []string{"vuln", "list"})

	if !strings.Contains(reason, `unknown command "vuln" for "cidx"`) {
		t.Errorf("expected the unknown command to be named, got %q", reason)
	}
	if !strings.Contains(reason, `did you mean "cidx security vuln"`) {
		t.Errorf("expected a pointer to the new location, got %q", reason)
	}
}

// TestResolve_RootIsJudgedEvenWithAnAction: urfave installs the help printer as
// the root action at run time, so an app with an action still has to report an
// unknown root command — without this, the real CLI stayed silent on the very
// invocation of #239.
func TestResolve_RootIsJudgedEvenWithAnAction(t *testing.T) {
	app := testApp()
	app.Action = func(*cli.Context) error { return nil }

	if reason := Resolve(app, []string{"vuln", "list"}); reason == "" {
		t.Error("expected the unknown root command to be reported")
	}
}

func TestExtractInvocations(t *testing.T) {
	tests := []struct {
		name   string
		script string
		want   [][]string
	}{
		{"plain invocation", "cidx run ci", [][]string{{"run", "ci"}}},
		{"built binary", "./bin/cidx run security", [][]string{{"run", "security"}}},
		{"relative binary", "bin/cidx run security", [][]string{{"run", "security"}}},
		{"go run", "go run ./cmd/cidx pr create", [][]string{{"pr", "create"}}},
		{"command substitution", "TARGETS=$(./bin/cidx preset scan-targets)", [][]string{{"preset", "scan-targets"}}},
		{"pipeline inside substitution", `IMAGES=$(./bin/cidx preset images --json | jq -c '.')`, [][]string{{"preset", "images", "--json"}}},
		{"after a shell keyword", "if ./bin/cidx security vuln check --days 7; then", [][]string{{"security", "vuln", "check", "--days", "7"}}},
		{"after &&", "chmod +x bin/cidx && ./bin/cidx run test", [][]string{{"run", "test"}}},
		{"line continuation", "./bin/cidx run \\\n  security", [][]string{{"run", "security"}}},
		{"several lines", "./bin/cidx run code\n./bin/cidx run test", [][]string{{"run", "code"}, {"run", "test"}}},
		{"trailing comment", "./bin/cidx run ci # publishes nothing", [][]string{{"run", "ci"}}},
		{"trailing redirection", "./bin/cidx preset list > /dev/null", [][]string{{"preset", "list"}}},
		{"trailing error redirection", "./bin/cidx security vuln list 2>/dev/null", [][]string{{"security", "vuln", "list"}}},

		// Forms that must never be read as invocations
		{"go build writing a cidx binary", "go build -o bin/cidx ./cmd/cidx", nil},
		{"chmod on the binary", "chmod +x bin/cidx", nil},
		{"copying the binary", "cp dist/cidx-linux-amd64 bin/cidx", nil},
		{"full comment line", "# cidx vuln list is the old form", nil},
		{"documented in an echo", `echo "   Add exceptions with: cidx vuln add <CVE>"`, nil},
		{"invoked through a variable", "$CIDX run ci", nil},
		{"heredoc body", "cat <<'EOF' > doc\ncidx vuln list\nEOF", nil},
		{"github output idiom is not a heredoc", "echo \"targets<<EOF\" >> $GITHUB_OUTPUT\n./bin/cidx run ci\necho \"EOF\" >> $GITHUB_OUTPUT", [][]string{{"run", "ci"}}},
		{"empty script", "", nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got [][]string
			for _, inv := range ExtractInvocations(tt.script) {
				got = append(got, inv.Args)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ExtractInvocations(%q) = %v, want %v", tt.script, got, tt.want)
			}
		})
	}
}

func TestExtractInvocations_ReportsTheLine(t *testing.T) {
	script := "echo start\n\n./bin/cidx run test\n"

	got := ExtractInvocations(script)
	if len(got) != 1 {
		t.Fatalf("expected 1 invocation, got %v", got)
	}
	if got[0].Line != 3 {
		t.Errorf("expected line 3, got %d", got[0].Line)
	}
	if got[0].String() != "cidx run test" {
		t.Errorf("unexpected rendering: %q", got[0].String())
	}
}

const invocationWorkflow = `name: CI
jobs:
  security:
    steps:
      - name: Scan
        run: ./bin/cidx run security
  audit:
    steps:
      - name: Build
        run: go build -o bin/cidx ./cmd/cidx
      - name: Ignore file
        run: |
          IMAGE=alpine
          ./bin/cidx vuln ignore --format trivy "$IMAGE"
`

func TestWorkflowInvocations(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ci.yml"), []byte(invocationWorkflow), 0644); err != nil {
		t.Fatal(err)
	}

	got, err := WorkflowInvocations(dir)
	if err != nil {
		t.Fatalf("WorkflowInvocations: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 invocations, got %v", got)
	}

	// Line 6 is the plain `run:` scalar, line 14 the second line of the block.
	if got[0].Line != 6 || got[0].Step != "Scan" {
		t.Errorf("first invocation: got line %d step %q", got[0].Line, got[0].Step)
	}
	if got[1].Line != 14 || got[1].Step != "Ignore file" {
		t.Errorf("second invocation: got line %d step %q", got[1].Line, got[1].Step)
	}
	if got[1].File != filepath.Join(dir, "ci.yml") {
		t.Errorf("unexpected file: %q", got[1].File)
	}
}

// TestWorkflowInvocations_CatchesTheIncident is the regression for #239: the
// workflow calls `cidx vuln ...` after the subcommand moved under `security`.
func TestWorkflowInvocations_CatchesTheIncident(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "security-audit.yml"), []byte(invocationWorkflow), 0644); err != nil {
		t.Fatal(err)
	}

	invocations, err := WorkflowInvocations(dir)
	if err != nil {
		t.Fatalf("WorkflowInvocations: %v", err)
	}

	var stale []string
	app := testApp()
	for _, inv := range invocations {
		if reason := Resolve(app, inv.Args); reason != "" {
			stale = append(stale, reason)
		}
	}

	if len(stale) != 1 {
		t.Fatalf("expected exactly the stale invocation, got %v", stale)
	}
	if !strings.Contains(stale[0], `unknown command "vuln"`) {
		t.Errorf("unexpected reason: %q", stale[0])
	}
}

// TestWorkflowInvocations_IsDeterministic guards the report order: steps are
// read out of a Go map of jobs, whose iteration order is randomised (#233).
func TestWorkflowInvocations_IsDeterministic(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ci.yml"), []byte(invocationWorkflow), 0644); err != nil {
		t.Fatal(err)
	}

	first, err := WorkflowInvocations(dir)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 20; i++ {
		got, err := WorkflowInvocations(dir)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got, first) {
			t.Fatalf("run %d returned %v, first run returned %v", i, got, first)
		}
	}
}

func TestWorkflowInvocations_MissingDirectory(t *testing.T) {
	got, err := WorkflowInvocations(filepath.Join(t.TempDir(), "absent"))
	if err != nil {
		t.Errorf("a missing workflow directory is not an error, got %v", err)
	}
	if got != nil {
		t.Errorf("expected no invocations, got %v", got)
	}
}
