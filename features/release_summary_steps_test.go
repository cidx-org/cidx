package features

import (
	"fmt"
	"strings"

	"github.com/cidx-org/cidx/v3/pkg/actions"
	"github.com/cucumber/godog"
)

func RegisterReleaseSummarySteps(ctx *godog.ScenarioContext, tc *TestContext) {
	ctx.Given(`^(\d+) commits since the latest tag$`, tc.commitsSinceTheLatestTag)
	ctx.When(`^I render the release change summary$`, tc.renderTheReleaseChangeSummary)
	ctx.Then(`^the release summary lists (\d+) commits$`, tc.releaseSummaryListsCommits)
	ctx.Then(`^the release summary says "([^"]*)"$`, tc.releaseSummarySays)
}

func (tc *TestContext) commitsSinceTheLatestTag(count int) error {
	lines := make([]string, count)
	for i := range lines {
		lines[i] = fmt.Sprintf("%07d fix: change %d", i+1, i+1)
	}
	tc.Config["release_commits"] = lines
	return nil
}

func (tc *TestContext) renderTheReleaseChangeSummary() error {
	lines, ok := tc.Config["release_commits"].([]string)
	if !ok {
		return fmt.Errorf("no release commits were staged")
	}
	tc.Output = actions.FormatCommitSummary(lines, 10)
	return nil
}

func (tc *TestContext) releaseSummaryListsCommits(count int) error {
	listed := 0
	for _, line := range strings.Split(tc.Output, "\n") {
		if strings.HasPrefix(line, "- ") && !strings.HasPrefix(line, "- ...") {
			listed++
		}
	}
	if listed != count {
		return fmt.Errorf("release summary lists %d commits, want %d:\n%s", listed, count, tc.Output)
	}
	return nil
}

func (tc *TestContext) releaseSummarySays(expected string) error {
	if !strings.Contains(tc.Output, expected) {
		return fmt.Errorf("release summary does not contain %q:\n%s", expected, tc.Output)
	}
	return nil
}
