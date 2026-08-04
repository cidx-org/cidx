package actions

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/cidx-org/cidx/v2/pkg/remote"
	log "github.com/sirupsen/logrus"
)

// ArtifactDownloadAction fetches the artifacts of one run onto disk.
//
// It exists because `cidx security vuln prune --results DIR` and `cidx security
// baseline --results DIR` read files that only a workflow run produces, and
// until now the only way to put them there was `gh run download` -- a cidx
// command depending on artifacts with no cidx command able to obtain them
// (issue #285).
//
// Three things it does that the shell-out did not, each one a bug paid for:
//
//   - It reads the repository from the git remote of the working directory, the
//     way every other cidx command does. `gh run download <id>` run outside a
//     checkout resolves the id against whatever repository gh last knew about
//     and answers with another repository's artifacts, silently; that skewed a
//     before/after measurement in #327.
//   - It writes one flat directory. `gh run download` unpacks a subdirectory per
//     artifact, and the readers of these files join `dir` with a file name --
//     twelve `trivy-N/` subdirectories are twelve directories none of them looks
//     in (#333).
//   - It never fails on a name two artifacts share. Identical content is skipped;
//     differing content keeps the first copy and says which artifacts disagreed,
//     because that is a fact about the run worth hearing, not a reason to abandon
//     a download halfway through.
type ArtifactDownloadAction struct {
	provider remote.Provider
	runID    string
	patterns []string
	dir      string
}

// NewArtifactDownload creates an artifact download action. patterns are glob
// patterns matched against artifact names; an empty list takes every artifact of
// the run.
func NewArtifactDownload(provider remote.Provider, runID string, patterns []string, dir string) *ArtifactDownloadAction {
	return &ArtifactDownloadAction{
		provider: provider,
		runID:    runID,
		patterns: patterns,
		dir:      dir,
	}
}

// Execute lists the run's artifacts, selects the ones asked for, and extracts
// them flat into the destination directory.
func (a *ArtifactDownloadAction) Execute(ctx context.Context) error {
	if a.runID == "" {
		return fmt.Errorf("a run ID is required")
	}
	if a.dir == "" {
		return fmt.Errorf("a destination directory is required")
	}

	artifacts, err := a.provider.ListRunArtifacts(ctx, a.runID)
	if err != nil {
		return err
	}
	if len(artifacts) == 0 {
		return fmt.Errorf("run %s produced no artifact", a.runID)
	}

	selected, err := selectArtifacts(artifacts, a.patterns)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(a.dir, 0o755); err != nil {
		return fmt.Errorf("failed to create %s: %w", a.dir, err)
	}

	log.Infof("📦 Downloading %d artifact(s) of run %s into %s", len(selected), a.runID, a.dir)

	written := make(map[string]string)
	var files, skipped int

	for _, artifact := range selected {
		if artifact.Expired {
			log.Warnf("⏭️  %s has expired and cannot be downloaded", artifact.Name)
			skipped++
			continue
		}

		extracted, err := a.extract(ctx, artifact, written)
		if err != nil {
			return err
		}
		log.Infof("   %s → %d file(s)", artifact.Name, extracted)
		files += extracted
	}

	if files == 0 {
		return fmt.Errorf("nothing was extracted from the %d artifact(s) of run %s", len(selected), a.runID)
	}

	fmt.Printf("\n✅ %d file(s) from %d artifact(s) in %s\n", files, len(selected)-skipped, a.dir)
	fmt.Printf("   Read them with: cidx security vuln prune --results %s\n", a.dir)
	return nil
}

// extract unpacks one artifact's archive into the destination directory, flat.
func (a *ArtifactDownloadAction) extract(ctx context.Context, artifact remote.Artifact, written map[string]string) (int, error) {
	body, err := a.provider.DownloadArtifact(ctx, artifact.ID)
	if err != nil {
		return 0, err
	}
	defer func() { _ = body.Close() }()

	archive, err := io.ReadAll(body)
	if err != nil {
		return 0, fmt.Errorf("failed to read the archive of %s: %w", artifact.Name, err)
	}

	// ErrInsecurePath is returned *with* a usable reader when an entry names a
	// path outside the archive. It is a warning aimed at code that would join
	// the entry name onto a destination directory, which is exactly what this
	// does not do -- only the base name is kept, a dozen lines below -- so the
	// archive is read and the sentinel is not an error here.
	//
	// Refusing it instead would make the download depend on the GODEBUG default
	// of whichever toolchain built the binary, and those disagree: go1.26.0 here
	// returns nil for the same archive that dhi.io/golang:1.26.5-alpine-dev, the
	// image the test phase runs in, rejects. An artifact that downloads on one
	// machine and fails on the next is a worse property than the one the
	// sentinel is warning about, which flattening has already removed.
	reader, err := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
	if err != nil && !errors.Is(err, zip.ErrInsecurePath) {
		return 0, fmt.Errorf("failed to open the archive of %s: %w", artifact.Name, err)
	}

	var extracted int
	for _, entry := range reader.File {
		if entry.FileInfo().IsDir() {
			continue
		}

		// filepath.Base is what flattens the layout, and it is also what makes
		// the write safe: an entry named `../../.ssh/authorized_keys` -- the zip
		// slip an archive from a remote is always a candidate for -- cannot name
		// anything outside the destination once only its last element is kept.
		name := filepath.Base(entry.Name)
		if name == "." || name == string(filepath.Separator) {
			continue
		}

		content, err := readZipEntry(entry)
		if err != nil {
			return extracted, fmt.Errorf("failed to read %s from %s: %w", entry.Name, artifact.Name, err)
		}

		if from, taken := written[name]; taken {
			existing, err := os.ReadFile(filepath.Join(a.dir, name))
			if err == nil && bytes.Equal(existing, content) {
				continue
			}
			log.Warnf("⚠️  %s is in both %s and %s with different content; keeping the copy from %s",
				name, from, artifact.Name, from)
			continue
		}

		if err := os.WriteFile(filepath.Join(a.dir, name), content, 0o644); err != nil {
			return extracted, fmt.Errorf("failed to write %s: %w", name, err)
		}
		written[name] = artifact.Name
		extracted++
	}

	return extracted, nil
}

// readZipEntry reads one archive entry in full.
func readZipEntry(entry *zip.File) ([]byte, error) {
	file, err := entry.Open()
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()
	return io.ReadAll(file)
}

// selectArtifacts keeps the artifacts whose name matches one of the patterns,
// in the order the run reports them. No pattern means every artifact.
//
// A pattern that matches nothing is an error naming what the run does have: a
// typo in `trivy-*` would otherwise download nothing, report success, and leave
// the reader to conclude from an empty directory that the audit found nothing.
func selectArtifacts(artifacts []remote.Artifact, patterns []string) ([]remote.Artifact, error) {
	if len(patterns) == 0 {
		return artifacts, nil
	}

	var selected []remote.Artifact
	for _, artifact := range artifacts {
		if matchesAny(artifact.Name, patterns) {
			selected = append(selected, artifact)
		}
	}

	if len(selected) == 0 {
		return nil, fmt.Errorf("no artifact matches %s; this run has: %s",
			strings.Join(patterns, ", "), strings.Join(artifactNames(artifacts), ", "))
	}
	return selected, nil
}

// matchesAny reports whether name matches one of the patterns. A malformed
// pattern is treated as a literal name, which is what a user typing an artifact
// name with a bracket in it means.
func matchesAny(name string, patterns []string) bool {
	for _, pattern := range patterns {
		matched, err := filepath.Match(pattern, name)
		if errors.Is(err, filepath.ErrBadPattern) {
			matched = pattern == name
		}
		if matched {
			return true
		}
	}
	return false
}

func artifactNames(artifacts []remote.Artifact) []string {
	names := make([]string, 0, len(artifacts))
	for _, a := range artifacts {
		names = append(names, a.Name)
	}
	return names
}
