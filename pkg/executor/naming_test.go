package executor

import (
	"regexp"
	"strings"
	"testing"
)

// dockerNameRE is Docker's container name grammar. Every name cidx generates
// must satisfy it, whatever the workspace path or preset name looks like.
var dockerNameRE = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]*$`)

func TestContainerName_Shape(t *testing.T) {
	got := ContainerName("/home/dev/myrepo", "golangci-lint")

	if !strings.HasPrefix(got, "cidx_myrepo-") {
		t.Errorf("name should start with cidx_<basename>-, got %q", got)
	}
	if !strings.HasSuffix(got, "_golangci-lint") {
		t.Errorf("name should end with _<tool>, got %q", got)
	}
	// cidx_myrepo-xxxxxxxx_golangci-lint
	if parts := strings.Split(got, "_"); len(parts) != 3 {
		t.Errorf("name should have 3 underscore-separated parts, got %q", got)
	}
}

func TestProjectScope_StableForSamePath(t *testing.T) {
	a := ProjectScope("/home/dev/myrepo")
	b := ProjectScope("/home/dev/myrepo")
	if a != b {
		t.Errorf("scope should be stable for the same path: %q != %q", a, b)
	}

	// Non-canonical spellings of the same directory must collapse to the
	// same scope, otherwise `cd ./repo` and `cd repo` would use two
	// different containers.
	if c := ProjectScope("/home/dev/../dev/myrepo/"); c != a {
		t.Errorf("scope should be path-canonical: %q != %q", c, a)
	}
}

// Regression test for issue #197: two repositories sharing a basename must not
// share a container name.
func TestProjectScope_DiffersForSameBasenameDifferentPath(t *testing.T) {
	a := ProjectScope("/home/dev/work/api")
	b := ProjectScope("/home/dev/personal/api")

	if a == b {
		t.Fatalf("two distinct paths with the same basename must not share a scope: %q", a)
	}
	if !strings.HasPrefix(a, "api-") || !strings.HasPrefix(b, "api-") {
		t.Errorf("both scopes should stay readable: %q, %q", a, b)
	}
}

// Regression test for issue #197: the same preset run from two projects must
// produce two distinct container names.
func TestContainerName_DiffersAcrossProjects(t *testing.T) {
	a := ContainerName("/home/dev/projA", "trivy")
	b := ContainerName("/home/dev/projB", "trivy")

	if a == b {
		t.Fatalf("same tool in different projects must not share a container name: %q", a)
	}
}

func TestContainerName_ValidDockerName(t *testing.T) {
	tests := []struct {
		name      string
		workspace string
		tool      string
	}{
		{"plain", "/home/dev/myrepo", "trivy"},
		{"hyphenated tool", "/home/dev/myrepo", "golangci-lint"},
		{"spaces in basename", "/home/dev/My Repo", "trivy"},
		{"accents and symbols", "/home/dev/répo (copie)", "trivy"},
		{"leading dot", "/home/dev/.hidden", "trivy"},
		{"root workspace", "/", "trivy"},
		{"empty workspace", "", "trivy"},
		{"relative workspace", "./sub", "trivy"},
		{"very long basename", "/home/dev/" + strings.Repeat("verylongname", 10), "trivy"},
		{"basename entirely invalid", "/home/dev/@@@", "trivy"},
		{"tool entirely invalid", "/home/dev/myrepo", "@@@"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ContainerName(tt.workspace, tt.tool)
			if !dockerNameRE.MatchString(got) {
				t.Errorf("ContainerName(%q, %q) = %q, which is not a valid Docker name", tt.workspace, tt.tool, got)
			}
		})
	}
}

func TestProjectScope_EdgeCasesFallBackToReadablePlaceholder(t *testing.T) {
	tests := []struct {
		name      string
		workspace string
	}{
		{"root", "/"},
		{"only invalid characters", "/home/dev/@@@"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ProjectScope(tt.workspace)
			if !strings.HasPrefix(got, "workspace-") {
				t.Errorf("ProjectScope(%q) = %q, want the 'workspace-' placeholder prefix", tt.workspace, got)
			}
		})
	}
}

func TestProjectScope_TruncatesLongBasenames(t *testing.T) {
	long := strings.Repeat("a", 200)
	got := ProjectScope("/home/dev/" + long)

	// maxProjectBaseLen readable characters + "-" + projectHashLen hash chars.
	want := maxProjectBaseLen + 1 + projectHashLen
	if len(got) != want {
		t.Errorf("ProjectScope length = %d (%q), want %d", len(got), got, want)
	}
}

func TestSanitizeNamePart(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"trivy", "trivy"},
		{"golangci-lint", "golangci-lint"},
		{"My Repo", "My-Repo"},
		{"a//b", "a-b"},
		{"répo", "r-po"},
		{"-leading-and-trailing-", "leading-and-trailing"},
		{"...", ""},
		{"", ""},
		{"keep_dots.and_underscores", "keep_dots.and_underscores"},
		{"collapse   spaces", "collapse-spaces"},
	}

	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			if got := sanitizeNamePart(tt.in); got != tt.want {
				t.Errorf("sanitizeNamePart(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
