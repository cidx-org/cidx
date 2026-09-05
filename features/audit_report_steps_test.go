package features

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/cucumber/godog"
	"gopkg.in/yaml.v3"
)

func RegisterAuditReportSteps(ctx *godog.ScenarioContext, tc *TestContext) {
	ctx.Given(`^the audit scanner jobs both finished with "([^"]*)"$`, func(result string) {
		tc.Config["audit_scanner_result"] = result
	})
	ctx.Given(`^the catalogue summary command will fail$`, func() { tc.Config["audit_render_failure"] = true })
	ctx.When(`^the audit workflow writes its job summary$`, tc.writeAuditJobSummary)
	ctx.Then(`^the audit report should contain the complete catalogue status page$`, tc.auditContainsStatusPage)
	ctx.Then(`^the audit report should link to the raw scan artifacts$`, func() error {
		return tc.auditReportContains("https://github.com/cidx-org/cidx/actions/runs/42#artifacts")
	})
	ctx.Then(`^the audit report should show the scanner job results$`, tc.auditShowsScannerResults)
	ctx.Then(`^the status issue should reuse the generated page$`, tc.auditReusesStatusPage)
	ctx.Then(`^the audit report step should fail$`, func() error {
		if tc.ExitCode == 0 {
			return fmt.Errorf("the report succeeded after its renderer failed")
		}
		return nil
	})
	ctx.Then(`^the status issue should not be published without a generated page$`, tc.auditRefusesMissingPage)
}

type auditWorkflowStep struct {
	Name string            `yaml:"name"`
	If   string            `yaml:"if"`
	Env  map[string]string `yaml:"env"`
	Run  string            `yaml:"run"`
}

func auditStep(name string) (auditWorkflowStep, error) {
	source, err := os.ReadFile(filepath.Join("..", ".github", "workflows", "security-audit.yml"))
	if err != nil {
		return auditWorkflowStep{}, fmt.Errorf("read audit workflow: %w", err)
	}
	var workflow struct {
		Jobs map[string]struct {
			Steps []auditWorkflowStep `yaml:"steps"`
		} `yaml:"jobs"`
	}
	if err := yaml.Unmarshal(source, &workflow); err != nil {
		return auditWorkflowStep{}, fmt.Errorf("decode audit workflow: %w", err)
	}
	for _, step := range workflow.Jobs["report"].Steps {
		if step.Name == name {
			return step, nil
		}
	}
	return auditWorkflowStep{}, fmt.Errorf("audit has no step %q", name)
}

// Exercise the actual workflow shell. Only process boundaries are substituted:
// CIDX returns the real renderer's staged page; gh records the body it receives.
func (tc *TestContext) writeAuditJobSummary() error {
	if err := tc.summariseCatalogueStatus(); err != nil {
		return err
	}
	_, page, err := tc.summary()
	if err != nil {
		return err
	}
	dir, err := tc.scenarioDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(dir, "bin"), 0o700); err != nil {
		return err
	}
	files := map[string]string{
		"page.md": page,
		"job.md":  "",
		"bin/cidx": `#!/bin/sh
set -eu
test "$#" -eq 6
test "$1 $2 $3 $4 $5" = 'security summary --results artifacts -o'
test "$AUDIT_RENDER_FAILURE" != true
cp "$AUDIT_PAGE" "$6"
`,
		"bin/gh": `#!/bin/sh
set -eu
printf '%s\n' "$*" >> "$AUDIT_GH_CALLS"
case "$1 $2" in
 'label create') ;;
 'issue list') printf '7\n' ;;
 'issue edit') test "$4" = '--body-file'; cp "$5" "$AUDIT_PUBLISHED" ;;
 *) exit 99 ;;
esac
`,
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o700); err != nil {
			return err
		}
	}
	step, err := auditStep("Generate Report")
	if err != nil {
		return err
	}
	if step.If != "always()" {
		return fmt.Errorf("the report must run after failed scan jobs too")
	}
	return tc.executeAuditStep(step, tc.Config["audit_render_failure"] == true)
}

func (tc *TestContext) executeAuditStep(step auditWorkflowStep, rendererFails bool) error {
	dir := tc.GitRepo
	result, _ := tc.Config["audit_scanner_result"].(string)
	values := map[string]string{
		"${{ needs.trivy-scan.result }}":                 result,
		"${{ needs.grype-scan.result }}":                 result,
		"${{ needs.check-exceptions.result }}":           "success",
		"${{ needs.check-exceptions.outputs.expiring }}": "2",
		"${{ github.token }}":                            "unused-fixture-token",
		"${{ github.repository }}":                       "cidx-org/cidx",
	}
	cmd := exec.Command("sh", "-e", "-c", step.Run)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"PATH="+filepath.Join(dir, "bin")+string(os.PathListSeparator)+os.Getenv("PATH"),
		"RUNNER_TEMP="+dir, "GITHUB_STEP_SUMMARY="+filepath.Join(dir, "job.md"),
		"GITHUB_SERVER_URL=https://github.com", "GITHUB_REPOSITORY=cidx-org/cidx", "GITHUB_RUN_ID=42",
		"AUDIT_PAGE="+filepath.Join(dir, "page.md"), "AUDIT_PUBLISHED="+filepath.Join(dir, "published.md"),
		"AUDIT_GH_CALLS="+filepath.Join(dir, "gh-calls"), fmt.Sprintf("AUDIT_RENDER_FAILURE=%t", rendererFails),
	)
	for key, value := range step.Env {
		if resolved, ok := values[value]; ok {
			value = resolved
		}
		if strings.Contains(value, "${{") {
			return fmt.Errorf("unstaged workflow expression %q", value)
		}
		cmd.Env = append(cmd.Env, key+"="+value)
	}
	output, runErr := cmd.CombinedOutput()
	tc.ExitCode = 0
	tc.Output = string(output)
	if runErr != nil {
		tc.ExitCode = 1
		tc.Output += "\n" + runErr.Error()
	}
	return nil
}

func (tc *TestContext) auditReportContains(want string) error {
	body, err := os.ReadFile(filepath.Join(tc.GitRepo, "job.md"))
	if err != nil {
		return err
	}
	if !strings.Contains(string(body), want) {
		return fmt.Errorf("audit report is missing %q:\n%s", want, body)
	}
	return nil
}

func (tc *TestContext) auditContainsStatusPage() error {
	if tc.ExitCode != 0 {
		return fmt.Errorf("audit report failed: %s", tc.Output)
	}
	_, page, err := tc.summary()
	if err != nil {
		return err
	}
	return tc.auditReportContains(page)
}

func (tc *TestContext) auditShowsScannerResults() error {
	result, _ := tc.Config["audit_scanner_result"].(string)
	for _, scanner := range []string{"Trivy", "Grype"} {
		if err := tc.auditReportContains("| " + scanner + " | " + result + " |"); err != nil {
			return err
		}
	}
	return tc.auditReportContains("Exceptions expiring within 7 days: 2")
}

func (tc *TestContext) auditReusesStatusPage() error {
	step, err := auditStep("Publish the status summary")
	if err != nil {
		return err
	}
	// A second render now fails, proving publication reuses the first file.
	if err := tc.executeAuditStep(step, true); err != nil {
		return err
	}
	if tc.ExitCode != 0 {
		return fmt.Errorf("publication did not reuse the existing page: %s", tc.Output)
	}
	body, err := os.ReadFile(filepath.Join(tc.GitRepo, "published.md"))
	if err != nil {
		return err
	}
	_, page, err := tc.summary()
	if err != nil {
		return err
	}
	if string(body) != page {
		return fmt.Errorf("the issue received a different page")
	}
	return nil
}

func (tc *TestContext) auditRefusesMissingPage() error {
	step, err := auditStep("Publish the status summary")
	if err != nil {
		return err
	}
	if !strings.Contains(step.If, "steps.catalogue-summary.outcome == 'success'") {
		return fmt.Errorf("publication is not conditional on successful rendering")
	}
	if err := tc.executeAuditStep(step, true); err != nil {
		return err
	}
	if tc.ExitCode == 0 {
		return fmt.Errorf("publication accepted a missing page")
	}
	if _, err := os.Stat(filepath.Join(tc.GitRepo, "gh-calls")); !os.IsNotExist(err) {
		return fmt.Errorf("publication reached gh without a generated page (stat: %v)", err)
	}
	return nil
}
