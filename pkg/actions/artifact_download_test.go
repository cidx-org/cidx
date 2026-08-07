package actions

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/cidx-org/cidx/v2/pkg/remote"
)

// zipOf builds an artifact archive from a name/content map. Nothing here
// touches the network: the provider hands the action bytes, which is the same
// shape a real download has once the redirect has been followed.
func zipOf(t *testing.T, entries map[string]string) []byte {
	t.Helper()

	var buf bytes.Buffer
	writer := zip.NewWriter(&buf)

	names := make([]string, 0, len(entries))
	for name := range entries {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		file, err := writer.Create(name)
		if err != nil {
			t.Fatalf("zip create %s: %v", name, err)
		}
		if _, err := file.Write([]byte(entries[name])); err != nil {
			t.Fatalf("zip write %s: %v", name, err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("zip close: %v", err)
	}
	return buf.Bytes()
}

// filesIn lists the evidence the download left on disk, sorted. Dotfiles are
// not evidence: the run marker of #359 is metadata about the directory, and
// the readers -- which look a result up by the image it is about -- never see
// it either.
func filesIn(t *testing.T, dir string) []string {
	t.Helper()

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read %s: %v", dir, err)
	}

	var names []string
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		if entry.IsDir() {
			names = append(names, entry.Name()+"/")
			continue
		}
		names = append(names, entry.Name())
	}
	sort.Strings(names)
	return names
}

// TestArtifactDownloadWritesOneFlatDirectory covers the second half of #333:
// `gh run download` unpacks a subdirectory per artifact, and every reader of
// these files joins the results directory with a bare file name. Twelve
// `trivy-N/` directories are twelve directories none of them looks in.
func TestArtifactDownloadWritesOneFlatDirectory(t *testing.T) {
	provider := &fakeProvider{
		artifacts: map[string][]remote.Artifact{
			"724": {
				{ID: 1, Name: "trivy-0"},
				{ID: 2, Name: "trivy-1"},
			},
		},
		archives: map[int64][]byte{
			1: zipOf(t, map[string]string{"trivy-alpine_3.20.json": `{"Results":[]}`}),
			2: zipOf(t, map[string]string{"results/trivy-golang_1.26.json": `{"Results":[]}`}),
		},
	}

	dir := t.TempDir()
	err := NewArtifactDownload(provider, "724", nil, dir).Execute(context.Background())
	if err != nil {
		t.Fatalf("download: %v", err)
	}

	want := []string{"trivy-alpine_3.20.json", "trivy-golang_1.26.json"}
	got := filesIn(t, dir)
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("the results directory is not flat: want %v, got %v", want, got)
	}
}

// TestArtifactDownloadKeepsArchiveEntriesInsideTheDestination: an archive comes
// from a remote and can name anything it likes. Flattening to the base name is
// what makes the write safe, and this pins it.
func TestArtifactDownloadKeepsArchiveEntriesInsideTheDestination(t *testing.T) {
	provider := &fakeProvider{
		artifacts: map[string][]remote.Artifact{"1": {{ID: 7, Name: "hostile"}}},
		archives: map[int64][]byte{
			7: zipOf(t, map[string]string{"../../escaped.json": "{}"}),
		},
	}

	parent := t.TempDir()
	dir := filepath.Join(parent, "results")
	if err := NewArtifactDownload(provider, "1", nil, dir).Execute(context.Background()); err != nil {
		t.Fatalf("download: %v", err)
	}

	if _, err := os.Stat(filepath.Join(parent, "escaped.json")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("an archive entry escaped the destination directory")
	}
	if got := filesIn(t, dir); strings.Join(got, ",") != "escaped.json" {
		t.Errorf("want the entry written under its base name, got %v", got)
	}
}

// TestArtifactDownloadSurvivesAFileNameTwoArtifactsShare: the flat layout makes
// collisions possible, and `gh run download` fails on them. Failing halfway
// through a twelve-artifact download leaves a directory that reads as a complete
// scan and is not one, so identical content is skipped and differing content
// keeps the first copy.
func TestArtifactDownloadSurvivesAFileNameTwoArtifactsShare(t *testing.T) {
	provider := &fakeProvider{
		artifacts: map[string][]remote.Artifact{
			"724": {
				{ID: 1, Name: "trivy-0"},
				{ID: 2, Name: "trivy-1"},
				{ID: 3, Name: "grype-0"},
			},
		},
		archives: map[int64][]byte{
			1: zipOf(t, map[string]string{"ignore-report.json": `{"suppressed":0}`}),
			2: zipOf(t, map[string]string{"ignore-report.json": `{"suppressed":0}`}),
			3: zipOf(t, map[string]string{"ignore-report.json": `{"suppressed":4}`}),
		},
	}

	dir := t.TempDir()
	if err := NewArtifactDownload(provider, "724", nil, dir).Execute(context.Background()); err != nil {
		t.Fatalf("a shared file name aborted the download: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(dir, "ignore-report.json"))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(content) != `{"suppressed":0}` {
		t.Errorf("the first copy was not the one kept: %s", content)
	}
}

// TestArtifactDownloadSelectsByPattern: the audit uploads trivy-N and grype-N,
// and reading only one scanner's results is a legitimate ask.
func TestArtifactDownloadSelectsByPattern(t *testing.T) {
	provider := &fakeProvider{
		artifacts: map[string][]remote.Artifact{
			"724": {
				{ID: 1, Name: "trivy-0"},
				{ID: 2, Name: "grype-0"},
			},
		},
		archives: map[int64][]byte{
			1: zipOf(t, map[string]string{"trivy-alpine.json": "{}"}),
			2: zipOf(t, map[string]string{"grype-alpine.json": "{}"}),
		},
	}

	dir := t.TempDir()
	err := NewArtifactDownload(provider, "724", []string{"trivy-*"}, dir).Execute(context.Background())
	if err != nil {
		t.Fatalf("download: %v", err)
	}

	if got := filesIn(t, dir); strings.Join(got, ",") != "trivy-alpine.json" {
		t.Errorf("the pattern did not select: %v", got)
	}
}

// TestArtifactDownloadRefusesAPatternThatMatchesNothing: a typo would otherwise
// download nothing, report success, and leave an empty directory that reads --
// to `vuln prune` and to whoever runs it -- as a scan that found nothing.
func TestArtifactDownloadRefusesAPatternThatMatchesNothing(t *testing.T) {
	provider := &fakeProvider{
		artifacts: map[string][]remote.Artifact{
			"724": {{ID: 1, Name: "trivy-0"}, {ID: 2, Name: "grype-0"}},
		},
	}

	err := NewArtifactDownload(provider, "724", []string{"trivvy-*"}, t.TempDir()).Execute(context.Background())
	if err == nil {
		t.Fatal("a pattern matching nothing was accepted")
	}
	for _, name := range []string{"trivy-0", "grype-0"} {
		if !strings.Contains(err.Error(), name) {
			t.Errorf("the refusal does not say what the run has (%s): %v", name, err)
		}
	}
}

// TestArtifactDownloadReportsARunWithNoArtifacts: an empty directory must never
// be the answer to a run that uploaded nothing.
func TestArtifactDownloadReportsARunWithNoArtifacts(t *testing.T) {
	provider := &fakeProvider{artifacts: map[string][]remote.Artifact{}}

	err := NewArtifactDownload(provider, "999", nil, t.TempDir()).Execute(context.Background())
	if err == nil || !strings.Contains(err.Error(), "999") {
		t.Fatalf("want an error naming the run, got %v", err)
	}
}

// TestArtifactDownloadSkipsExpiredArtifacts: an expired artifact cannot be
// fetched, and the run's other artifacts still can.
func TestArtifactDownloadSkipsExpiredArtifacts(t *testing.T) {
	provider := &fakeProvider{
		artifacts: map[string][]remote.Artifact{
			"724": {
				{ID: 1, Name: "trivy-0", Expired: true},
				{ID: 2, Name: "trivy-1"},
			},
		},
		archives: map[int64][]byte{
			2: zipOf(t, map[string]string{"trivy-golang.json": "{}"}),
		},
	}

	dir := t.TempDir()
	if err := NewArtifactDownload(provider, "724", nil, dir).Execute(context.Background()); err != nil {
		t.Fatalf("an expired artifact aborted the download: %v", err)
	}
	if got := filesIn(t, dir); strings.Join(got, ",") != "trivy-golang.json" {
		t.Errorf("want the live artifact's file, got %v", got)
	}
}
