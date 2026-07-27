package actions

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// versionRepo builds a work directory with a VERSION file and stubs the git
// plumbing ResolveVersion issues. The recorder is returned so a test can add
// the commit range it needs.
func versionRepo(t *testing.T, fileVersion, lastTag string) (string, *gitRecorder) {
	t.Helper()

	workDir := t.TempDir()
	if fileVersion != "" {
		if err := os.WriteFile(filepath.Join(workDir, "VERSION"), []byte(fileVersion+"\n"), 0644); err != nil {
			t.Fatalf("could not write VERSION: %v", err)
		}
	}

	git := &gitRecorder{output: map[string]string{}}
	if lastTag != "" {
		git.output["describe"] = lastTag + "\n"
	} else {
		git.fail = map[string]error{"describe": errors.New("fatal: No names found")}
	}
	git.install(t)

	return workDir, git
}

func TestResolveVersion_FilesMatchTag(t *testing.T) {
	workDir, _ := versionRepo(t, "2.1.3", "v2.1.3")

	state := ResolveVersion(workDir)
	if state.Diverged() {
		t.Errorf("VERSION 2.1.3 and tag v2.1.3 agree, got divergence: %v", state.DivergenceError())
	}
	if state.Current() != "2.1.3" {
		t.Errorf("expected current version 2.1.3, got %q", state.Current())
	}
}

func TestResolveVersion_FilesBehindTagIsRefused(t *testing.T) {
	// The v2.1.0 case from issue #185: VERSION says 1.7.0, the tag says v2.0.0,
	// and preview happily suggested v1.8.0 -- below the latest release.
	workDir, _ := versionRepo(t, "1.7.0", "v2.0.0")

	state := ResolveVersion(workDir)
	if !state.Diverged() {
		t.Fatal("expected stale version files to be reported as divergent")
	}
	if state.FilesAhead() {
		t.Error("version files trail the tag, they are not ahead of it")
	}

	// The current version must come from the tag, so the next one stays above it.
	if state.Current() != "2.0.0" {
		t.Errorf("expected the tag to win, got current %q", state.Current())
	}

	err := state.DivergenceError()
	if err == nil {
		t.Fatal("expected an error naming the disagreement")
	}
	for _, want := range []string{"1.7.0", "v2.0.0", "VERSION"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("expected %q in the message, got: %v", want, err)
		}
	}
}

func TestResolveVersion_FilesAheadOfTagPointsAtTagging(t *testing.T) {
	// A bump landed but its tag was never pushed: the release must not bump
	// again, it must tag what is already there.
	workDir, _ := versionRepo(t, "2.2.0", "v2.1.3")

	state := ResolveVersion(workDir)
	if !state.Diverged() || !state.FilesAhead() {
		t.Fatal("expected an untagged bump to be reported as files ahead of the tag")
	}

	err := state.DivergenceError()
	if err == nil {
		t.Fatal("expected an error naming the disagreement")
	}
	for _, want := range []string{"2.2.0", "v2.1.3", "cidx release tag create"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("expected %q in the message, got: %v", want, err)
		}
	}
}

func TestResolveVersion_NoTagFallsBackToFiles(t *testing.T) {
	workDir, _ := versionRepo(t, "0.4.2", "")

	state := ResolveVersion(workDir)
	if state.Diverged() {
		t.Errorf("nothing to reconcile without a tag, got: %v", state.DivergenceError())
	}
	if state.Current() != "0.4.2" {
		t.Errorf("expected the VERSION file to be used, got %q", state.Current())
	}
	if state.LastTagDisplay() != "(none)" {
		t.Errorf("expected '(none)' for a repo without tags, got %q", state.LastTagDisplay())
	}
}

func TestNextVersion_BumpsFromTheGivenBase(t *testing.T) {
	cases := []struct {
		name    string
		current string
		counts  CommitCounts
		want    string
	}{
		{"breaking wins", "2.1.3", CommitCounts{Breaking: 1, Feat: 2, Fix: 3}, "3.0.0"},
		{"feature bumps minor", "2.1.3", CommitCounts{Feat: 1, Fix: 4}, "2.2.0"},
		{"fixes bump patch", "2.1.3", CommitCounts{Fix: 7}, "2.1.4"},
		{"chores still bump patch", "2.1.3", CommitCounts{Other: 3}, "2.1.4"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := NextVersion(tc.current, tc.counts); got != tc.want {
				t.Errorf("NextVersion(%s) = %s, want %s", tc.current, got, tc.want)
			}
		})
	}
}

func TestCountCommits_CountsCommitsNotBodyLines(t *testing.T) {
	// The regression behind "Other: 131" for a 9-commit range: bodies span
	// several lines and every line was counted as a commit.
	log := strings.Join([]string{
		"feat(x): add a thing\x00body line one\nbody line two\nbody line three\x1e",
		"fix(y): repair a thing\x00\x1e",
		"chore: tidy up\x00Co-Authored-By: someone\nRefs: #1\x1e",
	}, "")

	git := &gitRecorder{output: map[string]string{"log": log}}
	git.install(t)

	counts := countCommits("/repo", "v2.1.3")
	if counts.Total() != 3 {
		t.Errorf("expected 3 commits, got %d (%+v)", counts.Total(), counts)
	}
	if counts.Feat != 1 || counts.Fix != 1 || counts.Other != 1 {
		t.Errorf("expected one feat, one fix and one other, got %+v", counts)
	}

	// The range must be the announced one, not the whole history.
	if !git.ran("log", "v2.1.3..HEAD") {
		t.Errorf("expected the commits to be read from v2.1.3..HEAD, got %v", git.calls)
	}
}

func TestCountCommits_DetectsBreakingChanges(t *testing.T) {
	log := "feat(api)!: drop the old flag\x00\x1efix: unrelated\x00BREAKING CHANGE: config moved\x1e"

	git := &gitRecorder{output: map[string]string{"log": log}}
	git.install(t)

	counts := countCommits("/repo", "v1.0.0")
	if counts.Breaking != 2 {
		t.Errorf("expected both breaking markers to be counted, got %+v", counts)
	}
}

func TestParseCommit(t *testing.T) {
	tests := []struct {
		name    string
		subject string
		body    string
		want    CommitInfo
	}{
		{
			name:    "plain type",
			subject: "feat: add the thing",
			want:    CommitInfo{Type: "feat", Subject: "add the thing"},
		},
		{
			name:    "type with scope",
			subject: "fix(release): stop guessing",
			want:    CommitInfo{Type: "fix", Scope: "release", Subject: "stop guessing"},
		},
		{
			// Issue #175: the "!" used to make the whole subject unparseable.
			name:    "breaking marker",
			subject: "feat!: drop the legacy flag",
			want:    CommitInfo{Type: "feat", Subject: "drop the legacy flag", Breaking: true},
		},
		{
			name:    "breaking marker with scope",
			subject: "feat(api)!: drop the legacy flag",
			want:    CommitInfo{Type: "feat", Scope: "api", Subject: "drop the legacy flag", Breaking: true},
		},
		{
			name:    "breaking footer in the body",
			subject: "fix: unrelated",
			body:    "BREAKING CHANGE: config moved",
			want:    CommitInfo{Type: "fix", Subject: "unrelated", Breaking: true},
		},
		{
			name:    "squash merge keeps its PR number",
			subject: "feat(release): share the parser (#226)",
			want:    CommitInfo{Type: "feat", Scope: "release", Subject: "share the parser (#226)", PR: 226},
		},
		{
			name:    "non-conventional subject",
			subject: "Merge branch 'main'",
			want:    CommitInfo{Type: "other", Subject: "Merge branch 'main'"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			want := tt.want
			want.Body = tt.body
			if got := ParseCommit(tt.subject, tt.body); got != want {
				t.Errorf("ParseCommit(%q, %q)\n got %+v\nwant %+v", tt.subject, tt.body, got, want)
			}
		})
	}
}

// A breaking commit must bump the major everywhere: release prepare reads its
// own log format, the tag flow reads another, and they disagreed while only
// one of them understood "!" (issue #175).
func TestBreakingCommit_BumpsTheMajorInBothReleasePaths(t *testing.T) {
	workDir, git := versionRepo(t, "1.0.0", "v1.0.0")
	git.output["log"] = "feat(api)!: drop the legacy flag\x00\x1e"

	prepared := ParseCommitLog("abcdef1234567890|feat(api)!: drop the legacy flag|<<<END>>>")
	prepareNext := NextVersion(ResolveVersion(workDir).Current(), CountCommitInfos(prepared))

	if prepareNext != "2.0.0" {
		t.Errorf("release prepare must bump the major on a breaking commit, got %s", prepareNext)
	}
	if tagNext := SuggestTagVersion(workDir); tagNext != prepareNext {
		t.Errorf("tag preview suggests %s but release prepare suggests %s", tagNext, prepareNext)
	}
}

func TestSuggestTagVersion_AgreesWithTheReleasePreview(t *testing.T) {
	workDir, git := versionRepo(t, "2.1.3", "v2.1.3")
	git.output["log"] = "feat: something new\x00\x1efix: something broken\x00\x1e"

	state := ResolveVersion(workDir)
	previewNext := NextVersion(state.Current(), countCommits(workDir, state.LastTag))

	if got := SuggestTagVersion(workDir); got != previewNext {
		t.Errorf("tag preview suggests %s but release preview suggests %s", got, previewNext)
	}
	if previewNext != "2.2.0" {
		t.Errorf("expected a feat in range to bump the minor from the tag, got %s", previewNext)
	}
}

func TestSuggestTagVersion_TagsThePendingBump(t *testing.T) {
	// VERSION already carries the bump: tag that version instead of inventing
	// a new one on top of it.
	workDir, _ := versionRepo(t, "2.2.0", "v2.1.3")

	if got := SuggestTagVersion(workDir); got != "2.2.0" {
		t.Errorf("expected the untagged bump 2.2.0 to be tagged, got %s", got)
	}
}

func TestChangelogTagGap_ReportsUntaggedSection(t *testing.T) {
	workDir := t.TempDir()
	changelog := "## [Unreleased]\n\n### Fix\n\n- something\n\n## v2.2.0 (2026-07-27)\n\n### Feat\n\n- a feature\n"
	if err := os.WriteFile(filepath.Join(workDir, "CHANGELOG.md"), []byte(changelog), 0644); err != nil {
		t.Fatalf("could not write CHANGELOG.md: %v", err)
	}

	git := &gitRecorder{fail: map[string]error{"rev-parse": errors.New("fatal: bad revision")}}
	git.install(t)

	if got := changelogTagGap(workDir); got != "2.2.0" {
		t.Errorf("expected the untagged 2.2.0 section to be reported, got %q", got)
	}
}

func TestChangelogTagGap_SilentWhenSectionIsTagged(t *testing.T) {
	workDir := t.TempDir()
	changelog := "## [Unreleased]\n\n## [2.1.3] - 2026-07-27\n\n### Fix\n\n- a fix\n"
	if err := os.WriteFile(filepath.Join(workDir, "CHANGELOG.md"), []byte(changelog), 0644); err != nil {
		t.Fatalf("could not write CHANGELOG.md: %v", err)
	}

	git := &gitRecorder{output: map[string]string{"rev-parse": "d34db33f\n"}}
	git.install(t)

	if got := changelogTagGap(workDir); got != "" {
		t.Errorf("expected no gap for a tagged section, got %q", got)
	}
}

func TestChangelogTagGap_NoChangelogIsNotAGap(t *testing.T) {
	git := &gitRecorder{}
	git.install(t)

	if got := changelogTagGap(t.TempDir()); got != "" {
		t.Errorf("a project without CHANGELOG.md has nothing to reconcile, got %q", got)
	}
}
