package features

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/cidx-org/cidx/v3/internal/commands"
	"github.com/cidx-org/cidx/v3/pkg/presets"
	"github.com/cucumber/godog"
)

// decisionState is one scenario's slice of the grouped-decision world: the
// file being judged, the contexts derived for its repositories, and what the
// last resolution or lifecycle evaluation said.
type decisionState struct {
	file         commands.VulnerabilityFile
	contexts     map[string]commands.DecisionContext
	verdicts     []commands.MemberVerdict
	observations []commands.FixObservation
	transitions  []commands.FixTransition
	preset       presets.Preset
	derived      []string
	loadPath     string
	loadErr      error
	firstWrite   string
	secondWrite  string
	readBack     *commands.VulnerabilityFile
	stopped      []presets.Exception
}

// RegisterGroupedDecisionSteps registers the grouped vulnerability decision steps
func RegisterGroupedDecisionSteps(ctx *godog.ScenarioContext, tc *TestContext) {
	ctx.Step(`^a decision "([^"]*)" on "([^"]*)" reviewed until "([^"]*)"$`, tc.aDecisionReviewedUntil)
	ctx.Step(`^the decision "([^"]*)" expects capabilities "([^"]*)"$`, tc.decisionExpectsCapabilities)
	ctx.Step(`^the decision "([^"]*)" requires "([^"]*)"$`, tc.decisionRequires)
	ctx.Step(`^the decision "([^"]*)" expects no capabilities$`, tc.decisionExpectsNoCapabilities)
	ctx.Step(`^the file records a reviewed context for "([^"]*)" establishing "([^"]*)"$`, tc.fileRecordsReviewedContext)
	ctx.Step(`^the context is derived from the catalogue$`, tc.contextIsDerivedFromCatalogue)
	ctx.Step(`^the ignore file for "([^"]*)" is built on "([^"]*)"$`, tc.ignoreFileIsBuiltOn)
	ctx.Step(`^the file is written and read back twice$`, tc.fileIsWrittenAndReadBackTwice)
	ctx.Step(`^the second write should be byte-identical to the first$`, tc.secondWriteShouldBeIdentical)
	ctx.Step(`^the read-back file should carry (\d+) decision, (\d+) context and (\d+) entries$`, tc.readBackFileShouldCarry)
	ctx.Step(`^the acceptances that no longer stand are listed on "([^"]*)"$`, tc.acceptancesNoLongerStandingOn)
	ctx.Step(`^"([^"]*)" should be listed as no longer standing$`, tc.shouldBeListedAsNoLongerStanding)
	ctx.Step(`^"([^"]*)" should not be listed as no longer standing$`, tc.shouldNotBeListedAsNoLongerStanding)
	ctx.Step(`^the reason for "([^"]*)" should mention "([^"]*)"$`, tc.reasonShouldMention)
	ctx.Step(`^the "([^"]*)" context provides capabilities "([^"]*)"$`, tc.contextProvidesCapabilities)
	ctx.Step(`^the "([^"]*)" context establishes no semantic predicates$`, tc.contextEstablishesNothing)
	ctx.Step(`^the member "([^"]*)" on "([^"]*)" references decision "([^"]*)"$`, tc.memberReferencesDecision)
	ctx.Step(`^a legacy entry "([^"]*)" on "([^"]*)" expiring "([^"]*)"$`, tc.aLegacyEntry)
	ctx.Step(`^the waivers are resolved on "([^"]*)"$`, tc.waiversAreResolvedOn)
	ctx.Step(`^"([^"]*)" should be waived$`, tc.shouldBeWaived)
	ctx.Step(`^"([^"]*)" should not be waived$`, tc.shouldNotBeWaived)
	ctx.Step(`^the verdict for "([^"]*)" should mention "([^"]*)"$`, tc.verdictShouldMention)

	ctx.Step(`^a preset declaring env "([^"]*)" as "([^"]*)"$`, tc.aPresetDeclaringEnv)
	ctx.Step(`^the preset mounts "([^"]*)"$`, tc.presetMounts)
	ctx.Step(`^its capabilities are derived$`, tc.capabilitiesAreDerived)
	ctx.Step(`^the derived capabilities should be "([^"]*)"$`, tc.derivedCapabilitiesShouldBe)

	ctx.Step(`^a vulnerability file where "([^"]*)" references the unknown decision "([^"]*)"$`, tc.fileWithUnknownDecision)
	ctx.Step(`^the vulnerability file is loaded$`, tc.vulnerabilityFileIsLoaded)
	ctx.Step(`^loading should fail mentioning "([^"]*)"$`, tc.loadingShouldFailMentioning)

	ctx.Step(`^suppressed scanner evidence reports "([^"]*)" fixed in "([^"]*)" observed on "([^"]*)"$`, tc.suppressedEvidenceReportsFix)
	ctx.Step(`^the decision lifecycle is evaluated$`, tc.decisionLifecycleIsEvaluated)
	ctx.Step(`^"([^"]*)" should be queued for remediation with its clock starting "([^"]*)"$`, tc.shouldBeQueuedForRemediation)
	ctx.Step(`^"([^"]*)" should not be queued for remediation$`, tc.shouldNotBeQueuedForRemediation)
}

// decisions lazily initializes the scenario's decision state, so every step
// shares one world and Reset gets a fresh one.
func (tc *TestContext) decisions() *decisionState {
	if tc.decisionScenario == nil {
		tc.decisionScenario = &decisionState{contexts: map[string]commands.DecisionContext{}}
	}
	return tc.decisionScenario
}

func splitList(list string) []string {
	if list == "" {
		return nil
	}
	parts := strings.Split(list, ",")
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}
	return parts
}

func (tc *TestContext) aDecisionReviewedUntil(id, repository, reviewBy string) error {
	ds := tc.decisions()
	ds.file.Decisions = append(ds.file.Decisions, commands.Decision{ID: id, Repository: repository, ReviewBy: reviewBy})
	// A context entry must exist for the repository: a decision judged with no
	// derived context at all fails closed, which is its own scenario.
	if _, ok := ds.contexts[repository]; !ok {
		ds.contexts[repository] = commands.DecisionContext{}
	}
	return nil
}

func (tc *TestContext) withDecision(id string, change func(*commands.Decision)) error {
	ds := tc.decisions()
	for i := range ds.file.Decisions {
		if ds.file.Decisions[i].ID == id {
			change(&ds.file.Decisions[i])
			return nil
		}
	}
	return fmt.Errorf("no decision %q staged", id)
}

func (tc *TestContext) decisionExpectsCapabilities(id, capabilities string) error {
	return tc.withDecision(id, func(d *commands.Decision) { d.Capabilities = splitList(capabilities) })
}

func (tc *TestContext) decisionRequires(id, predicates string) error {
	return tc.withDecision(id, func(d *commands.Decision) { d.Requires = splitList(predicates) })
}

func (tc *TestContext) contextProvidesCapabilities(repository, capabilities string) error {
	ds := tc.decisions()
	ctx := ds.contexts[repository]
	ctx.Capabilities = splitList(capabilities)
	ds.contexts[repository] = ctx
	return nil
}

func (tc *TestContext) contextEstablishesNothing(repository string) error {
	ds := tc.decisions()
	ctx := ds.contexts[repository]
	ctx.Semantics = nil
	ds.contexts[repository] = ctx
	return nil
}

func (tc *TestContext) memberReferencesDecision(cve, repository, decision string) error {
	ds := tc.decisions()
	ds.file.Vulnerabilities = append(ds.file.Vulnerabilities, commands.Vulnerability{CVE: cve, Repository: repository, Severity: "HIGH", Decision: decision})
	return nil
}

func (tc *TestContext) aLegacyEntry(cve, repository, expires string) error {
	ds := tc.decisions()
	ds.file.Vulnerabilities = append(ds.file.Vulnerabilities, commands.Vulnerability{CVE: cve, Repository: repository, Severity: "HIGH", Expires: expires})
	return nil
}

func (tc *TestContext) waiversAreResolvedOn(date string) error {
	day, err := time.Parse(time.DateOnly, date)
	if err != nil {
		return fmt.Errorf("bad day %q: %w", date, err)
	}
	ds := tc.decisions()
	ds.verdicts = commands.ResolveWaivers(&ds.file, ds.contexts, day)
	return nil
}

func (tc *TestContext) verdictFor(cve string) (commands.MemberVerdict, error) {
	for _, v := range tc.decisions().verdicts {
		if v.CVE == cve {
			return v, nil
		}
	}
	return commands.MemberVerdict{}, fmt.Errorf("no verdict for %q", cve)
}

func (tc *TestContext) shouldBeWaived(cve string) error {
	v, err := tc.verdictFor(cve)
	if err != nil {
		return err
	}
	if !v.Waived {
		return fmt.Errorf("%s is not waived: %s", cve, v.Reason)
	}
	return nil
}

func (tc *TestContext) shouldNotBeWaived(cve string) error {
	v, err := tc.verdictFor(cve)
	if err != nil {
		return err
	}
	if v.Waived {
		return fmt.Errorf("%s is waived and should not be", cve)
	}
	return nil
}

func (tc *TestContext) verdictShouldMention(cve, fragment string) error {
	v, err := tc.verdictFor(cve)
	if err != nil {
		return err
	}
	if !strings.Contains(v.Reason, fragment) {
		return fmt.Errorf("verdict for %s says %q, which does not mention %q", cve, v.Reason, fragment)
	}
	return nil
}

func (tc *TestContext) aPresetDeclaringEnv(key, value string) error {
	ds := tc.decisions()
	if ds.preset.Env == nil {
		ds.preset.Env = map[string]string{}
	}
	ds.preset.Env[key] = value
	return nil
}

func (tc *TestContext) presetMounts(path string) error {
	ds := tc.decisions()
	ds.preset.Volumes = append(ds.preset.Volumes, "${WORKSPACE}:/work", path+":"+path)
	return nil
}

func (tc *TestContext) capabilitiesAreDerived() error {
	ds := tc.decisions()
	ds.derived = presets.Capabilities(ds.preset)
	return nil
}

func (tc *TestContext) derivedCapabilitiesShouldBe(expected string) error {
	got := strings.Join(tc.decisions().derived, ", ")
	if got != expected {
		return fmt.Errorf("derived %q, expected %q", got, expected)
	}
	return nil
}

func (tc *TestContext) fileWithUnknownDecision(cve, decision string) error {
	ds := tc.decisions()
	dir, err := os.MkdirTemp("", "cidx-decisions")
	if err != nil {
		return err
	}
	tc.GitRepo = dir // reuse the context's cleanup of scenario directories
	ds.loadPath = filepath.Join(dir, "known-vulnerabilities.toml")
	content := fmt.Sprintf("[[vulnerabilities]]\n  cve = %q\n  repository = \"example\"\n  decision = %q\n", cve, decision)
	return os.WriteFile(ds.loadPath, []byte(content), 0o644)
}

func (tc *TestContext) vulnerabilityFileIsLoaded() error {
	ds := tc.decisions()
	_, ds.loadErr = commands.LoadVulnerabilityFile(ds.loadPath)
	return nil
}

func (tc *TestContext) loadingShouldFailMentioning(fragment string) error {
	ds := tc.decisions()
	if ds.loadErr == nil {
		return fmt.Errorf("loading succeeded, expected a failure mentioning %q", fragment)
	}
	if !strings.Contains(ds.loadErr.Error(), fragment) {
		return fmt.Errorf("load error %q does not mention %q", ds.loadErr, fragment)
	}
	return nil
}

func (tc *TestContext) suppressedEvidenceReportsFix(cve, version, observed string) error {
	ds := tc.decisions()
	ds.observations = append(ds.observations, commands.FixObservation{CVE: cve, FixedVersion: version, Observed: observed})
	return nil
}

func (tc *TestContext) decisionLifecycleIsEvaluated() error {
	ds := tc.decisions()
	ds.transitions = commands.FixTransitions(&ds.file, ds.observations)
	return nil
}

func (tc *TestContext) shouldBeQueuedForRemediation(cve, clockStart string) error {
	for _, tr := range tc.decisions().transitions {
		if tr.CVE == cve {
			if tr.ClockStart != clockStart {
				return fmt.Errorf("%s queued with clock %q, expected %q", cve, tr.ClockStart, clockStart)
			}
			return nil
		}
	}
	return fmt.Errorf("%s is not queued for remediation", cve)
}

func (tc *TestContext) shouldNotBeQueuedForRemediation(cve string) error {
	for _, tr := range tc.decisions().transitions {
		if tr.CVE == cve {
			return fmt.Errorf("%s is queued for remediation and should not be", cve)
		}
	}
	return nil
}

func (tc *TestContext) decisionExpectsNoCapabilities(id string) error {
	return tc.withDecision(id, func(d *commands.Decision) { d.Capabilities = nil })
}

func (tc *TestContext) fileRecordsReviewedContext(repository, predicates string) error {
	ds := tc.decisions()
	ds.file.Contexts = append(ds.file.Contexts, commands.ReviewedContext{
		Repository:  repository,
		Established: splitList(predicates),
		Reviewed:    "2026-09-01",
		Basis:       "staged by a scenario",
	})
	return nil
}

// contextIsDerivedFromCatalogue replaces whatever a scenario staged by hand
// with what production computes: the real catalogue's consumers, and the
// reviewed contexts the file records.
func (tc *TestContext) contextIsDerivedFromCatalogue() error {
	catalogue, err := presets.Catalogue()
	if err != nil {
		return err
	}
	ds := tc.decisions()
	ds.contexts = commands.DeriveContexts(&ds.file, catalogue)
	return nil
}

// ignoreFileIsBuiltOn applies what `cidx security vuln ignore` applies: the
// waived entries of the whole file, kept for the one repository asked about.
// It fills the same slot the SARIF scenarios read, so "the ignore file should
// carry" means one thing across the suite.
func (tc *TestContext) ignoreFileIsBuiltOn(repository, date string) error {
	day, err := time.Parse(time.DateOnly, date)
	if err != nil {
		return fmt.Errorf("bad day %q: %w", date, err)
	}
	ds := tc.decisions()
	ds.verdicts = commands.ResolveWaivers(&ds.file, ds.contexts, day)
	var carried []string
	for _, v := range commands.WaivedEntries(&ds.file, ds.contexts, day) {
		if v.Repository == repository {
			carried = append(carried, strings.ToUpper(v.CVE))
		}
	}
	tc.Config["ignore_file"] = carried
	return nil
}

func (tc *TestContext) fileIsWrittenAndReadBackTwice() error {
	ds := tc.decisions()
	dir, err := os.MkdirTemp("", "cidx-decisions")
	if err != nil {
		return err
	}
	tc.GitRepo = dir
	path := filepath.Join(dir, "known-vulnerabilities.toml")

	if err := commands.SaveVulnerabilityFile(path, &ds.file); err != nil {
		return err
	}
	first, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	ds.firstWrite = string(first)

	ds.readBack, err = commands.LoadVulnerabilityFile(path)
	if err != nil {
		return err
	}
	if err := commands.SaveVulnerabilityFile(path, ds.readBack); err != nil {
		return err
	}
	second, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	ds.secondWrite = string(second)
	return nil
}

func (tc *TestContext) secondWriteShouldBeIdentical() error {
	ds := tc.decisions()
	if ds.firstWrite != ds.secondWrite {
		return fmt.Errorf("the two writes differ:\n--- first\n%s\n--- second\n%s", ds.firstWrite, ds.secondWrite)
	}
	return nil
}

func (tc *TestContext) readBackFileShouldCarry(decisions, contexts, entries int) error {
	f := tc.decisions().readBack
	if f == nil {
		return fmt.Errorf("nothing was read back")
	}
	if len(f.Decisions) != decisions || len(f.Contexts) != contexts || len(f.Vulnerabilities) != entries {
		return fmt.Errorf("read back %d decisions, %d contexts, %d entries; expected %d, %d, %d",
			len(f.Decisions), len(f.Contexts), len(f.Vulnerabilities), decisions, contexts, entries)
	}
	return nil
}

// acceptancesNoLongerStandingOn applies what the Security tab and the status
// page apply: ExceptionsFor over the record (the real catalogue's contexts
// included), then ExpiredExceptions — the one definition both views read.
func (tc *TestContext) acceptancesNoLongerStandingOn(date string) error {
	day, err := time.Parse(time.DateOnly, date)
	if err != nil {
		return fmt.Errorf("bad day %q: %w", date, err)
	}
	ds := tc.decisions()
	ds.stopped = presets.ExpiredExceptions(commands.ExceptionsFor(&ds.file, day, ""), day)
	return nil
}

func (tc *TestContext) stoppedFor(cve string) (presets.Exception, bool) {
	for _, e := range tc.decisions().stopped {
		if strings.EqualFold(e.CVE, cve) {
			return e, true
		}
	}
	return presets.Exception{}, false
}

func (tc *TestContext) shouldBeListedAsNoLongerStanding(cve string) error {
	if _, listed := tc.stoppedFor(cve); !listed {
		return fmt.Errorf("%s is not listed among the acceptances that no longer stand", cve)
	}
	return nil
}

func (tc *TestContext) shouldNotBeListedAsNoLongerStanding(cve string) error {
	if e, listed := tc.stoppedFor(cve); listed {
		return fmt.Errorf("%s is listed as no longer standing (%s / %s)", cve, e.Expires, e.Stopped)
	}
	return nil
}

func (tc *TestContext) reasonShouldMention(cve, fragment string) error {
	e, listed := tc.stoppedFor(cve)
	if !listed {
		return fmt.Errorf("%s is not listed", cve)
	}
	if !strings.Contains(e.Stopped, fragment) {
		return fmt.Errorf("reason for %s is %q, which does not mention %q", cve, e.Stopped, fragment)
	}
	return nil
}
