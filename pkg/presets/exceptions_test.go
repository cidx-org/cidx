package presets

import "testing"

// catalogueRepos is what the catalogue runs, as exceptions key it: repository,
// no tag, no digest.
var catalogueRepos = []string{
	"dhi.io/trivy",
	"golangci/golangci-lint",
}

// The two evidence states most scenarios are written against.
//
// recordsAll is what security-audit.yml produces: something in the run was kept
// as suppressed, so an absence from the results is an absence. statesNothing is
// the zero value — results that neither kept a receipt nor say what went into
// their ignore file — and every conclusion drawn from an absence there is a
// guess. The third state, an ignore file declared empty, is what the tests below
// [TestNothingIsObsoleteWhenTheResultsKeptNoReceipt] exercise.
var (
	recordsAll    = SuppressionEvidence{Sighted: true}
	statesNothing = SuppressionEvidence{}
)

// scannedClean is the evidence state where every catalogue repository was
// scanned and none of them reported anything. Stated explicitly, because the
// difference between "scanned, nothing found" and "not scanned" is the whole
// point.
func scannedClean() map[string][]Finding {
	findings := make(map[string][]Finding, len(catalogueRepos))
	for _, repo := range catalogueRepos {
		findings[repo] = nil
	}
	return findings
}

// suppressedOn is what the audit's ignore file kept out of a repository's
// results — the record of a finding the scanners saw and an entry on file
// waived. It is the only place a live entry's own CVE can appear, because the
// ignore file is generated from that entry.
func suppressedOn(repo string, findings ...Finding) map[string][]Finding {
	return map[string][]Finding{repo: findings}
}

// TestExceptionOnARunningRepositoryIsLive: an exception whose CVE the scanners
// still see on the repository the catalogue runs is doing its job, and nothing
// is decided about it.
//
// The evidence is the suppressed record rather than the visible findings. The
// audit builds its ignore file from these very entries, so a CVE accepted on a
// running repository is deleted from that repository's own results; reading that
// absence as "gone" is what would purge every exception that is working, and
// reading it as "still there" is what made the lifecycle unable to close (#311).
func TestExceptionOnARunningRepositoryIsLive(t *testing.T) {
	suppressed := suppressedOn("dhi.io/trivy", Finding{ID: "CVE-2026-0001", Severity: "HIGH"})

	verdict := ClassifyException("CVE-2026-0001", "dhi.io/trivy", catalogueRepos, scannedClean(), suppressed, recordsAll)

	if verdict.State != ExceptionLive {
		t.Fatalf("state = %q (%s), want %q", verdict.State, verdict.Reason, ExceptionLive)
	}
}

// TestExceptionSurvivesATagMove: the reason the key is the repository. The
// catalogue promoting `dhi.io/trivy:0.68` to `0.71` changes none of what the
// judgement rests on, and under the old `repo:tag` key every entry for it
// stopped matching anything on the day of the promotion.
func TestExceptionSurvivesATagMove(t *testing.T) {
	suppressed := suppressedOn("dhi.io/trivy", Finding{ID: "CVE-2026-0001", Severity: "HIGH"})

	verdict := ClassifyException("CVE-2026-0001", "dhi.io/trivy", catalogueRepos, scannedClean(), suppressed, recordsAll)

	if verdict.State != ExceptionLive {
		t.Fatalf("state = %q (%s), want %q: the tag is context, not identity", verdict.State, verdict.Reason, ExceptionLive)
	}
}

// TestLiveEntryWhoseCVEIsGoneIsObsolete is #311, and the case the whole change
// exists for.
//
// The catalogue still runs the repository — a repin never changes that — and the
// CVE the entry accepts is in neither the visible findings nor what the scanners
// recorded as suppressed. Nothing carries it any more, so the entry waives
// nothing and can go. Until this, the repository match returned live before the
// findings were consulted at all, and the only way an exception could ever leave
// the file was for its repository to leave the catalogue.
func TestLiveEntryWhoseCVEIsGoneIsObsolete(t *testing.T) {
	findings := scannedClean()
	findings["dhi.io/trivy"] = []Finding{{ID: "CVE-2026-9999", Severity: "HIGH"}}
	// The repin removed CVE-2026-0001, so the ignore file generated from the
	// entry matched nothing and the scanners recorded no suppression for it.
	suppressed := suppressedOn("dhi.io/trivy", Finding{ID: "CVE-2026-0002", Severity: "HIGH"})

	verdict := ClassifyException("CVE-2026-0001", "dhi.io/trivy", catalogueRepos, findings, suppressed, recordsAll)

	if verdict.State != ExceptionObsolete {
		t.Fatalf("state = %q (%s), want %q: a repin that removes the CVE has to be able to retire the entry",
			verdict.State, verdict.Reason, ExceptionObsolete)
	}
}

// TestLiveEntryWhoseCVEIsStillCarriedStaysLive is the other half of the same
// test, and the one that has to hold for the change to be safe: the entry next
// to the retired one, on the same repository, whose CVE the repin did not
// remove. It is still waiving a finding the audit would fail on.
func TestLiveEntryWhoseCVEIsStillCarriedStaysLive(t *testing.T) {
	findings := scannedClean()
	findings["dhi.io/trivy"] = []Finding{{ID: "CVE-2026-9999", Severity: "HIGH"}}
	suppressed := suppressedOn("dhi.io/trivy", Finding{ID: "CVE-2026-0001", Severity: "HIGH"})

	verdict := ClassifyException("CVE-2026-0001", "dhi.io/trivy", catalogueRepos, findings, suppressed, recordsAll)

	if verdict.State != ExceptionLive {
		t.Fatalf("state = %q (%s), want %q: the scanners saw it, the entry is why it is not in the results",
			verdict.State, verdict.Reason, ExceptionLive)
	}
}

// TestLiveEntryReadsUnfilteredResultsToo: `container-monitor.yml` generates no
// ignore file, so its results show everything and suppress nothing. A CVE
// visible in the findings of its own running repository is carried just as
// plainly as one recorded suppressed, and the union has to say so without
// knowing which workflow produced the artifacts.
func TestLiveEntryReadsUnfilteredResultsToo(t *testing.T) {
	findings := scannedClean()
	findings["dhi.io/trivy"] = []Finding{{ID: "CVE-2026-0001", Severity: "HIGH"}}

	verdict := ClassifyException("CVE-2026-0001", "dhi.io/trivy", catalogueRepos, findings, nil, recordsAll)

	if verdict.State != ExceptionLive {
		t.Fatalf("state = %q (%s), want %q", verdict.State, verdict.Reason, ExceptionLive)
	}
}

// TestLiveEntryWithoutScanEvidenceIsUnknown: fail-closed on a running repository
// too. A CVE cannot be shown absent from an image nobody scanned, and the
// repository still being in the catalogue is not evidence about the CVE — that
// conflation is what #311 was.
func TestLiveEntryWithoutScanEvidenceIsUnknown(t *testing.T) {
	verdict := ClassifyException("CVE-2026-0001", "dhi.io/trivy", catalogueRepos, nil, nil, recordsAll)

	if verdict.State != ExceptionUnknown {
		t.Fatalf("state = %q (%s), want %q: no result is not the same as no finding",
			verdict.State, verdict.Reason, ExceptionUnknown)
	}
}

// TestNothingIsObsoleteWhenTheResultsKeptNoReceipt: the gate that makes the
// change safe to ship.
//
// Trivy's report says nothing about whether `--show-suppressed` was passed —
// the field is simply omitted when a scan suppressed nothing — so results
// produced without it look exactly like results that hid nothing. Measured on
// the audit's own artifacts from the day before the flag landed: four ansible
// entries, every one of them still carried, all four reported obsolete. Absence
// only counts when something, somewhere, was recorded as suppressed.
func TestNothingIsObsoleteWhenTheResultsKeptNoReceipt(t *testing.T) {
	findings := scannedClean()
	findings["dhi.io/trivy"] = []Finding{{ID: "CVE-2026-9999", Severity: "HIGH"}}

	for _, repository := range []string{"dhi.io/trivy", "aquasec/trivy"} {
		verdict := ClassifyException("CVE-2026-0001", repository, catalogueRepos, findings, nil, statesNothing)

		if verdict.State != ExceptionUnknown {
			t.Errorf("%s: state = %q (%s), want %q: an unrecorded suppression is not an absent finding",
				repository, verdict.State, verdict.Reason, ExceptionUnknown)
		}
	}
}

// TestAnEmptyIgnoreFileSettlesAnAbsenceOnItsOwn is the case a receipt can never
// cover, and the state the catalogue is actually in (#327).
//
// Since #303 stopped an expired acceptance from filtering anything, every entry
// on file is past its date and every ignore file the audit builds is empty — so
// nothing is ever recorded as suppressed, and the gate above holds every absence
// for ever. An empty ignore file removed nothing; that is not something a scan
// result can say, so the step that builds the file says it.
func TestAnEmptyIgnoreFileSettlesAnAbsenceOnItsOwn(t *testing.T) {
	findings := scannedClean()
	findings["dhi.io/trivy"] = []Finding{{ID: "CVE-2026-9999", Severity: "HIGH"}}

	nothingFiltered := SuppressionEvidence{Declared: map[string]int{
		"dhi.io/trivy":           0,
		"golangci/golangci-lint": 0,
	}}

	for _, repository := range []string{"dhi.io/trivy", "aquasec/trivy"} {
		verdict := ClassifyException("CVE-2026-0001", repository, catalogueRepos, findings, nil, nothingFiltered)

		if verdict.State != ExceptionObsolete {
			t.Errorf("%s: state = %q (%s), want %q: nothing was filtering, so the absence is an absence",
				repository, verdict.State, verdict.Reason, ExceptionObsolete)
		}
	}
}

// TestADeclaredIgnoreFileWithEntriesNeedsTheReceiptToo: the declaration settles
// an absence only when it says the file was empty. A file with entries in it
// removed something, and nothing here says what.
func TestADeclaredIgnoreFileWithEntriesNeedsTheReceiptToo(t *testing.T) {
	findings := scannedClean()
	findings["dhi.io/trivy"] = []Finding{{ID: "CVE-2026-9999", Severity: "HIGH"}}

	filtered := SuppressionEvidence{Declared: map[string]int{"dhi.io/trivy": 3}}

	verdict := ClassifyException("CVE-2026-0001", "dhi.io/trivy", catalogueRepos, findings, nil, filtered)
	if verdict.State != ExceptionUnknown {
		t.Errorf("state = %q (%s), want %q: three entries were filtering and nothing recorded what they removed",
			verdict.State, verdict.Reason, ExceptionUnknown)
	}
}

// TestOneRepositoryThatCannotAccountForItselfHoldsTheVerdict: "no catalogue
// image carries it any more" is a claim about all of them. A repository that
// filtered something nothing recorded holds it, exactly as one that was never
// scanned does.
func TestOneRepositoryThatCannotAccountForItselfHoldsTheVerdict(t *testing.T) {
	findings := scannedClean()

	partial := SuppressionEvidence{Declared: map[string]int{"dhi.io/trivy": 0}}

	verdict := ClassifyException("CVE-2026-0001", "aquasec/trivy", catalogueRepos, findings, nil, partial)
	if verdict.State != ExceptionUnknown {
		t.Errorf("state = %q (%s), want %q: golangci-lint stated nothing about what it filtered",
			verdict.State, verdict.Reason, ExceptionUnknown)
	}
}

// TestConclusiveNeedsSomethingToLookAt: with no repository named there is
// nothing to have looked at, and a conclusion drawn from an empty list is the
// purest form of the mistake this type exists to prevent.
func TestConclusiveNeedsSomethingToLookAt(t *testing.T) {
	if recordsAll.Conclusive() {
		t.Error("an empty list of repositories reads as settled")
	}
}

// TestTheReceiptGateNeverHoldsBackPositiveEvidence: the gate is about absence
// alone. A CVE the scanners actually reported is carried whether or not anything
// else in the directory was recorded as suppressed, and holding those back would
// re-file nothing and leave the report saying "unknown" about findings it can
// see.
func TestTheReceiptGateNeverHoldsBackPositiveEvidence(t *testing.T) {
	findings := scannedClean()
	findings["dhi.io/trivy"] = []Finding{{ID: "CVE-2026-0001", Severity: "HIGH"}}

	if verdict := ClassifyException("CVE-2026-0001", "dhi.io/trivy", catalogueRepos, findings, nil, statesNothing); verdict.State != ExceptionLive {
		t.Errorf("state = %q (%s), want %q", verdict.State, verdict.Reason, ExceptionLive)
	}
	if verdict := ClassifyException("CVE-2026-0001", "aquasec/trivy", catalogueRepos, findings, nil, statesNothing); verdict.State != ExceptionCarryOver {
		t.Errorf("state = %q (%s), want %q", verdict.State, verdict.Reason, ExceptionCarryOver)
	}
}

// TestAnUnmigratedEntryMatchesNoRepository: an entry still keyed the old way
// carries a whole `repo:tag` where a repository belongs. It equals no
// repository, so it is judged on its CVE alone — which is exactly what re-keying
// it requires, and needs no special case.
func TestAnUnmigratedEntryMatchesNoRepository(t *testing.T) {
	findings := scannedClean()
	findings["dhi.io/trivy"] = []Finding{{ID: "CVE-2026-0001", Severity: "HIGH"}}

	verdict := ClassifyException("CVE-2026-0001", "golangci/golangci-lint:v2.6.2", catalogueRepos, findings, nil, recordsAll)

	if verdict.State != ExceptionCarryOver {
		t.Fatalf("state = %q (%s), want %q", verdict.State, verdict.Reason, ExceptionCarryOver)
	}
	if verdict.StillOn != "dhi.io/trivy" {
		t.Errorf("StillOn = %q, want the repository that carries it", verdict.StillOn)
	}
}

// TestExceptionSurvivingThePromotionIsCarriedOver: the trap this command exists
// for. The repository was replaced, so the entry matches nothing — but the CVE
// came along to the new image. Purging it would lose the justification and leave
// the next scan with an unexplained HIGH/CRITICAL.
func TestExceptionSurvivingThePromotionIsCarriedOver(t *testing.T) {
	findings := scannedClean()
	findings["dhi.io/trivy"] = []Finding{{ID: "CVE-2026-0001", Severity: "HIGH"}}

	verdict := ClassifyException("CVE-2026-0001", "aquasec/trivy", catalogueRepos, findings, nil, recordsAll)

	if verdict.State != ExceptionCarryOver {
		t.Fatalf("state = %q (%s), want %q", verdict.State, verdict.Reason, ExceptionCarryOver)
	}
	if verdict.StillOn != "dhi.io/trivy" {
		t.Errorf("StillOn = %q, want the repository that still carries it", verdict.StillOn)
	}
}

// TestCarryOverNamesTheFixWhenThereIsOne: a carried-over CVE that is fixed
// upstream is not exception territory at all — the policy says never to write
// one for it — so the verdict has to say so rather than let the entry be
// re-filed in silence.
func TestCarryOverNamesTheFixWhenThereIsOne(t *testing.T) {
	findings := scannedClean()
	findings["dhi.io/trivy"] = []Finding{{ID: "CVE-2026-0001", Severity: "HIGH", FixedIn: "1.2.8"}}

	verdict := ClassifyException("CVE-2026-0001", "aquasec/trivy", catalogueRepos, findings, nil, recordsAll)

	if verdict.FixedIn != "1.2.8" {
		t.Errorf("FixedIn = %q, want the version the scanners reported", verdict.FixedIn)
	}
}

// TestLiveEntryNamesTheFixTheIgnoreFileHid is #312.
//
// A live entry with a fix upstream is the entry the policy says should never
// have been written — a repin candidate, not a renewal — and it was the one case
// `prune` said nothing about. It cannot be read from the scan results: the audit
// builds its ignore file from these very entries, so a live entry's CVE is
// filtered out of its own repository's scan by construction (#311). The evidence
// is what the scanners recorded as suppressed, which is the second map.
func TestLiveEntryNamesTheFixTheIgnoreFileHid(t *testing.T) {
	// What a live entry's own repository reports: not one word about the CVE the
	// entry accepts, because the entry itself filtered it out.
	findings := scannedClean()
	findings["dhi.io/trivy"] = []Finding{{ID: "CVE-2026-9999", Severity: "HIGH"}}

	suppressed := map[string][]Finding{
		"dhi.io/trivy": {{ID: "CVE-2026-0001", Severity: "HIGH", FixedIn: "0.71.1", Scanner: "Grype"}},
	}

	verdict := ClassifyException("CVE-2026-0001", "dhi.io/trivy", catalogueRepos, findings, suppressed, recordsAll)

	if verdict.State != ExceptionLive {
		t.Fatalf("state = %q (%s), want %q: a fix upstream is a repin, not a purge", verdict.State, verdict.Reason, ExceptionLive)
	}
	if verdict.FixedIn != "0.71.1" {
		t.Errorf("FixedIn = %q, want the fix the scanners reported on the entry's own repository", verdict.FixedIn)
	}
}

// TestLiveEntryNamesNoFixWhenNobodyReportedOne: an empty FixedIn says nobody
// said, not that no fix exists. The finding is on the record as suppressed, so
// the entry is plainly still doing its job — neither scanner named a version.
func TestLiveEntryNamesNoFixWhenNobodyReportedOne(t *testing.T) {
	suppressed := suppressedOn("dhi.io/trivy", Finding{ID: "CVE-2026-0001", Severity: "HIGH"})

	verdict := ClassifyException("CVE-2026-0001", "dhi.io/trivy", catalogueRepos, scannedClean(), suppressed, recordsAll)

	if verdict.State != ExceptionLive {
		t.Fatalf("state = %q (%s), want %q", verdict.State, verdict.Reason, ExceptionLive)
	}
	if verdict.FixedIn != "" {
		t.Errorf("FixedIn = %q, want none reported", verdict.FixedIn)
	}
}

// TestLiveEntryReadsOnlyItsOwnRepository: the suppressed findings are keyed by
// repository like everything else here, and a fix reported on a neighbour says
// nothing about this entry — it would name a version of another image entirely.
func TestLiveEntryReadsOnlyItsOwnRepository(t *testing.T) {
	suppressed := map[string][]Finding{
		"dhi.io/trivy":           {{ID: "CVE-2026-0001", Severity: "HIGH", Scanner: "Grype"}},
		"golangci/golangci-lint": {{ID: "CVE-2026-0001", Severity: "HIGH", FixedIn: "2.13.0", Scanner: "Grype"}},
	}

	verdict := ClassifyException("CVE-2026-0001", "dhi.io/trivy", catalogueRepos, scannedClean(), suppressed, recordsAll)

	if verdict.State != ExceptionLive {
		t.Fatalf("state = %q (%s), want %q", verdict.State, verdict.Reason, ExceptionLive)
	}
	if verdict.FixedIn != "" {
		t.Errorf("FixedIn = %q, want the fix of another repository to be ignored", verdict.FixedIn)
	}
}

// TestCarryOverReadsWhatTheNeighbourSuppressed: the repository was replaced, and
// the CVE followed it into an image that has an accepted entry of its own — so
// it is filtered out of that image's visible results too. Judging on those alone
// reads it as gone and deletes a justification the audit is still relying on.
func TestCarryOverReadsWhatTheNeighbourSuppressed(t *testing.T) {
	suppressed := suppressedOn("dhi.io/trivy", Finding{ID: "CVE-2026-0001", Severity: "HIGH"})

	verdict := ClassifyException("CVE-2026-0001", "aquasec/trivy", catalogueRepos, scannedClean(), suppressed, recordsAll)

	if verdict.State != ExceptionCarryOver {
		t.Fatalf("state = %q (%s), want %q", verdict.State, verdict.Reason, ExceptionCarryOver)
	}
	if verdict.StillOn != "dhi.io/trivy" {
		t.Errorf("StillOn = %q, want the repository that carries it", verdict.StillOn)
	}
}

// TestCarryOverIsCaseInsensitive: Trivy shouts identifiers, Grype capitalises
// them, and an exception must not read as obsolete because the scanner that
// found it spelled it differently.
func TestCarryOverIsCaseInsensitive(t *testing.T) {
	findings := scannedClean()
	findings["dhi.io/trivy"] = []Finding{{ID: "ghsa-cgrx-mc8f-2prm", Severity: "HIGH"}}

	verdict := ClassifyException("GHSA-cgrx-mc8f-2prm", "aquasec/trivy", catalogueRepos, findings, nil, recordsAll)

	if verdict.State != ExceptionCarryOver {
		t.Fatalf("state = %q (%s), want %q", verdict.State, verdict.Reason, ExceptionCarryOver)
	}
}

// TestExceptionWhoseCVEIsGoneIsObsolete: the repository was replaced and the
// finding went with it. Nothing left to waive.
func TestExceptionWhoseCVEIsGoneIsObsolete(t *testing.T) {
	findings := scannedClean()
	findings["dhi.io/trivy"] = []Finding{{ID: "CVE-2026-9999", Severity: "HIGH"}}

	verdict := ClassifyException("CVE-2026-0001", "aquasec/trivy", catalogueRepos, findings, nil, recordsAll)

	if verdict.State != ExceptionObsolete {
		t.Fatalf("state = %q (%s), want %q", verdict.State, verdict.Reason, ExceptionObsolete)
	}
}

// TestExceptionWithoutScanEvidenceIsUnknown: a CVE cannot be shown absent from
// an image nobody scanned. Fail-closed, like the cooldown on an undatable
// candidate and the scan gate on an unreadable result.
func TestExceptionWithoutScanEvidenceIsUnknown(t *testing.T) {
	verdict := ClassifyException("CVE-2026-0001", "aquasec/trivy", catalogueRepos, nil, nil, recordsAll)

	if verdict.State != ExceptionUnknown {
		t.Fatalf("state = %q (%s), want %q", verdict.State, verdict.Reason, ExceptionUnknown)
	}
}

// TestPartialScanEvidenceIsUnknown: one repository missing from the results is
// enough to leave the question open — the CVE could be on exactly that one.
func TestPartialScanEvidenceIsUnknown(t *testing.T) {
	findings := map[string][]Finding{"dhi.io/trivy": nil}

	verdict := ClassifyException("CVE-2026-0001", "aquasec/trivy", catalogueRepos, findings, nil, recordsAll)

	if verdict.State != ExceptionUnknown {
		t.Fatalf("state = %q (%s), want %q", verdict.State, verdict.Reason, ExceptionUnknown)
	}
}

// TestCarryOverBeatsMissingEvidence: a CVE found on a scanned repository is an
// answer on its own — no amount of unscanned images makes it less true.
func TestCarryOverBeatsMissingEvidence(t *testing.T) {
	findings := map[string][]Finding{"dhi.io/trivy": {{ID: "CVE-2026-0001", Severity: "HIGH"}}}

	verdict := ClassifyException("CVE-2026-0001", "aquasec/trivy", catalogueRepos, findings, nil, recordsAll)

	if verdict.State != ExceptionCarryOver {
		t.Fatalf("state = %q (%s), want %q", verdict.State, verdict.Reason, ExceptionCarryOver)
	}
}

// TestEveryVerdictStatesAReason: an entry the report leaves in place has to say
// what it is waiting for, or the file goes quiet exactly the way it did before.
func TestEveryVerdictStatesAReason(t *testing.T) {
	cases := []struct {
		name       string
		repository string
		findings   map[string][]Finding
		suppressed map[string][]Finding
	}{
		{"live", "dhi.io/trivy", scannedClean(), suppressedOn("dhi.io/trivy", Finding{ID: "CVE-2026-0001"})},
		{"carry-over", "aquasec/trivy", scannedClean(), suppressedOn("dhi.io/trivy", Finding{ID: "CVE-2026-0001"})},
		{"obsolete on a running repository", "dhi.io/trivy", scannedClean(), nil},
		{"obsolete", "aquasec/trivy", scannedClean(), nil},
		{"unknown", "aquasec/trivy", nil, nil},
		{"unknown on a running repository", "dhi.io/trivy", nil, nil},
	}

	for _, tc := range cases {
		verdict := ClassifyException("CVE-2026-0001", tc.repository, catalogueRepos, tc.findings, tc.suppressed, recordsAll)
		if verdict.Reason == "" {
			t.Errorf("%s: verdict %q states no reason", tc.name, verdict.State)
		}
	}
}
