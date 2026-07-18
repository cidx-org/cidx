package actions

import (
	"strings"
	"testing"
)

func TestTitleToBranchName(t *testing.T) {
	tests := []struct {
		name  string
		title string
		want  string
	}{
		// All conventional-commit types used in the repo
		{"feat", "feat: add auth system", "feat/add-auth-system"},
		{"fix", "fix: broken pipeline", "fix/broken-pipeline"},
		{"chore", "chore: bump dependencies", "chore/bump-dependencies"},
		{"docs", "docs: update readme", "docs/update-readme"},
		{"refactor", "refactor: split phases", "refactor/split-phases"},
		{"test", "test: add pr tests", "test/add-pr-tests"},
		{"ci", "ci: cache go modules", "ci/cache-go-modules"},
		{"perf", "perf: faster preset merge", "perf/faster-preset-merge"},
		{"build", "build: pin go version", "build/pin-go-version"},

		// Scoped title: type becomes prefix, scope stays in slug, no fix/fix- duplication
		{"scoped fix", "fix(generate): pin bootstrapped cidx", "fix/generate-pin-bootstrapped-cidx"},
		{"scoped feat", "feat(actions): add cpw command", "feat/actions-add-cpw-command"},

		// Breaking change marker
		{"breaking", "feat(api)!: drop v1 endpoints", "feat/api-drop-v1-endpoints"},
		{"breaking no scope", "fix!: remove legacy flag", "fix/remove-legacy-flag"},

		// No recognizable type: fall back to feat/
		{"no type", "Add Auth System", "feat/add-auth-system"},
		{"unknown type", "wip: something in progress", "feat/wip-something-in-progress"},

		// Case-insensitive type detection
		{"uppercase type", "Fix: Broken Thing", "fix/broken-thing"},

		// Special characters collapse into single hyphens
		{"special chars", "fix(actions): don't panic, really!", "fix/actions-don-t-panic-really"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := titleToBranchName(tt.title); got != tt.want {
				t.Errorf("titleToBranchName(%q) = %q, want %q", tt.title, got, tt.want)
			}
		})
	}
}

func TestPRNextStepsSuggestCIDXCommands(t *testing.T) {
	steps := strings.Join(prNextSteps, "\n")

	// Must suggest current cidx commands (dogfooding)
	for _, want := range []string{"cidx cpw", "cidx pr ready"} {
		if !strings.Contains(steps, want) {
			t.Errorf("next steps should suggest %q, got:\n%s", want, steps)
		}
	}

	// Must not suggest deprecated aliases or raw git
	for _, forbidden := range []string{"cidx action", "git add", "git commit", "git push"} {
		if strings.Contains(steps, forbidden) {
			t.Errorf("next steps should not suggest %q, got:\n%s", forbidden, steps)
		}
	}
}
