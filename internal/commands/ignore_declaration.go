package commands

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/cidx-org/cidx/v2/pkg/presets"
)

// What the audit states about the ignore file it built (#327).
//
// `cidx security vuln prune` and `cidx security baseline` both have to know
// whether a CVE missing from a scan result is missing because the image stopped
// carrying it, or because the audit's own ignore file removed it before the JSON
// was written. Until now that was *inferred*, from a finding somewhere in the
// results having been recorded as suppressed — the only positive proof a Trivy
// report can offer, since `ExperimentalModifiedFindings` is `omitempty` and says
// nothing about whether `--show-suppressed` was passed (#324).
//
// The inference has a blind spot that is not an edge case but the catalogue's
// current state: since #303 every acceptance on file is past its date, so every
// ignore file the audit writes is empty, so nothing is ever recorded as
// suppressed — and the readers hedge every conclusion on the one ground that
// makes a conclusion certain. An empty ignore file hid nothing.
//
// So the step that builds the file says what it built, instead of the readers
// guessing it from an absence. It already knows: `cidx security vuln ignore`
// prints the same two numbers on stderr for the run log. Writing them next to
// the results is the whole change — no new measurement, no metadata format, one
// small JSON file per image filed under the naming convention the scan results
// already use.
//
// Readers that never see one are unaffected: a directory with no declaration
// reads exactly as it did before, which is the fail-closed posture #324 built.

// ignoreDeclaration is what `cidx security vuln ignore --results` writes beside
// the scan results: for one image, how many accepted entries went into the
// ignore file the scanners were then run with, and how many were left out
// because their acceptance had lapsed.
//
// `expired` is not read by any decision. It is what turns "nothing was
// filtering" from a bare fact into the sentence a reader can act on — the file
// is empty *because* eighteen acceptances are past their date — and it is the
// same number the generating step already prints.
type ignoreDeclaration struct {
	Image      string `json:"image"`
	Repository string `json:"repository"`
	Entries    int    `json:"entries"`
	Expired    int    `json:"expired"`
}

// ignoreDeclarationName is the file the declaration is filed under. It borrows
// the scan results' own convention — `<what>-<flattened image>.json` — so the
// readers find it with the same lookup, under either workflow's flattening of an
// image reference (see scanResultFiles).
const ignoreDeclarationName = "ignored"

// writeIgnoreDeclaration records what was written for one image, creating the
// directory if the scan step has not made it yet.
func writeIgnoreDeclaration(dir string, declaration ignoreDeclaration) error {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create %s: %w", dir, err)
	}

	encoded, err := json.MarshalIndent(declaration, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to encode the ignore declaration: %w", err)
	}

	path := filepath.Join(dir, fmt.Sprintf("%s-%s.json", ignoreDeclarationName, auditFileName.Replace(declaration.Image)))
	if err := os.WriteFile(path, append(encoded, '\n'), 0644); err != nil {
		return fmt.Errorf("failed to write %s: %w", path, err)
	}
	return nil
}

// readIgnoreDeclaration returns what the audit stated about this image's ignore
// file, and whether it stated anything at all.
//
// Absent and unreadable are the same answer: nothing was declared. There is no
// error to report, because saying nothing is a legitimate state — the container
// monitor builds no ignore file and every artifact produced before #327 predates
// the declaration.
func readIgnoreDeclaration(dir, image string) (ignoreDeclaration, bool) {
	data, err := readFirstResult(dir, scanResultFiles(ignoreDeclarationName, image))
	if err != nil {
		return ignoreDeclaration{}, false
	}

	var declaration ignoreDeclaration
	if err := json.Unmarshal(data, &declaration); err != nil {
		return ignoreDeclaration{}, false
	}
	return declaration, true
}

// ignoreEvidence is what a whole results directory says about its own filtering:
// the record its scanners kept, and the declaration the audit wrote next to it.
//
// The first is [presets.SuppressionEvidence], which is what the lifecycle
// decides on. The second is the count of acceptances the audit dropped as
// lapsed, which decides nothing and is printed: without it "nothing was
// filtering" is a fact with no cause attached.
type ignoreEvidence struct {
	presets.SuppressionEvidence

	// expired is per repository rather than a running total, because the
	// catalogue runs two tags of the same repository and they declare the same
	// entries twice.
	expired map[string]int
}

func newIgnoreEvidence() *ignoreEvidence {
	return &ignoreEvidence{
		SuppressionEvidence: presets.SuppressionEvidence{Declared: map[string]int{}},
		expired:             map[string]int{},
	}
}

// observe folds one image's results into the evidence: what its scanners
// recorded as suppressed, and what the audit declared it wrote for it.
//
// The Trivy sighting is what the whole directory is judged on, because the flag
// that produces it is passed per workflow run. The declaration is per repository,
// and where two tags of one repository disagree the larger count wins — a
// repository that filtered something on any of its images is one whose absences
// still need the sighting.
func (e *ignoreEvidence) observe(dir, image string, suppressed []presets.Finding) {
	e.Sighted = e.Sighted || reportedBy(suppressed, "Trivy")

	declaration, declared := readIgnoreDeclaration(dir, image)
	if !declared {
		return
	}

	repository := imageRepository(image)
	if entries, seen := e.Declared[repository]; !seen || declaration.Entries > entries {
		e.Declared[repository] = declaration.Entries
	}
	if e.expired[repository] < declaration.Expired {
		e.expired[repository] = declaration.Expired
	}
}

// emptyIgnoreFiles counts the repositories the audit declared it filtered
// nothing on — the population whose absences are readable without a sighting.
func (e ignoreEvidence) emptyIgnoreFiles() int {
	var empty int
	for _, entries := range e.Declared {
		if entries == 0 {
			empty++
		}
	}
	return empty
}

// expiredAcceptances is how many acceptances were left out of those files
// because their date had passed.
func (e ignoreEvidence) expiredAcceptances() int {
	var total int
	for _, expired := range e.expired {
		total += expired
	}
	return total
}
