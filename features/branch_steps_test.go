package features

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/cidx-org/cidx/v3/pkg/branch"
	"github.com/cucumber/godog"
)

// RegisterBranchSteps registers step definitions for branch cleanup scenarios.
//
// The steps stage a branch set and run the decision `cidx repo branch cleanup`
// runs over it -- branch.SelectForCleanup, the real one, not a copy: what comes
// back is exactly what the command deletes. The flags are parsed here rather
// than through the CLI, so these scenarios speak about the decision and not
// about the command tree (issue #317); the flag wiring itself is covered by the
// unit tests in pkg/branch and by the command's own flags.
func RegisterBranchSteps(ctx *godog.ScenarioContext, tc *TestContext) {
	ctx.Given(`^the repository has branches:$`, tc.repositoryHasBranches)
	ctx.Given(`^the current branch is "([^"]*)"$`, tc.theCurrentBranchIs)

	ctx.When(`^I clean up branches$`, func() error { return tc.cleanUpBranches("") })
	ctx.When(`^I clean up branches with "([^"]*)"$`, tc.cleanUpBranches)

	ctx.Then(`^no branch is deleted$`, tc.noBranchIsDeleted)
	ctx.Then(`^branches "([^"]*)" are deleted$`, tc.branchesAreDeleted)
	ctx.Then(`^branch "([^"]*)" is kept because "([^"]*)"$`, tc.branchIsKeptBecause)
	ctx.Then(`^the cleanup fails with "([^"]*)"$`, tc.theCleanupFailsWith)
}

// repositoryHasBranches stages the branch set, replacing any previous one so a
// scenario can narrow the Background to the case it is about.
func (tc *TestContext) repositoryHasBranches(table *godog.Table) error {
	var branches []branch.Info

	for _, row := range table.Rows[1:] {
		info := branch.Info{Name: row.Cells[0].Value, Location: branch.LocationLocal}

		switch status := row.Cells[1].Value; status {
		case "protected":
			info.Status = branch.StatusProtected
			info.IsProtected = true
		case "merged":
			info.Status = branch.StatusMerged
		case "orphan":
			info.Status = branch.StatusOrphan
		case "stale":
			info.Status = branch.StatusStale
		case "active":
			info.Status = branch.StatusActive
		default:
			return fmt.Errorf("unknown branch status %q", status)
		}

		// "42 open", "43 closed", "44 merged" -- or nothing at all.
		if pr := strings.Fields(row.Cells[2].Value); len(pr) == 2 {
			number, err := strconv.Atoi(pr[0])
			if err != nil {
				return fmt.Errorf("bad PR number %q: %w", pr[0], err)
			}
			info.PRNumber = number
			info.PRStatus = branch.PRStatus(pr[1])
		}

		branches = append(branches, info)
	}

	tc.Config["branches"] = branches
	return nil
}

func (tc *TestContext) theCurrentBranchIs(name string) error {
	tc.Config["current_branch"] = name
	return nil
}

// cleanUpBranches runs the cleanup decision with the given command-line flags.
func (tc *TestContext) cleanUpBranches(flags string) error {
	branches, _ := tc.Config["branches"].([]branch.Info)
	current, _ := tc.Config["current_branch"].(string)

	opts, err := parseCleanupFlags(flags)
	if err != nil {
		return err
	}

	selected, skipped, err := branch.SelectForCleanup(branches, opts, current)
	tc.Config["deleted"] = selected
	tc.Config["skipped"] = skipped
	tc.Config["cleanup_error"] = err
	return nil
}

func parseCleanupFlags(flags string) (branch.CleanupOptions, error) {
	opts := branch.CleanupOptions{}
	fields := strings.Fields(flags)

	for i := 0; i < len(fields); i++ {
		switch fields[i] {
		case "--all":
			opts.All = true
		case "--force":
			opts.Force = true
		case "--stale":
			opts.IncludeStale = true
		case "--orphan":
			opts.IncludeOrphan = true
		case "--branch":
			if i+1 >= len(fields) {
				return opts, fmt.Errorf("--branch needs a name")
			}
			i++
			opts.Branch = fields[i]
		default:
			return opts, fmt.Errorf("unknown flag %q", fields[i])
		}
	}

	return opts, nil
}

func (tc *TestContext) noBranchIsDeleted() error {
	deleted, _ := tc.Config["deleted"].([]branch.Info)
	if len(deleted) > 0 {
		return fmt.Errorf("expected nothing to be deleted, got %s", strings.Join(deletedNames(deleted), ", "))
	}
	return nil
}

func (tc *TestContext) branchesAreDeleted(expected string) error {
	if err, _ := tc.Config["cleanup_error"].(error); err != nil {
		return fmt.Errorf("cleanup failed: %w", err)
	}

	deleted, _ := tc.Config["deleted"].([]branch.Info)
	got := strings.Join(deletedNames(deleted), ", ")

	var want []string
	for _, name := range strings.Split(expected, ",") {
		want = append(want, strings.TrimSpace(name))
	}

	if got != strings.Join(want, ", ") {
		return fmt.Errorf("deleted %q, expected %q", got, strings.Join(want, ", "))
	}
	return nil
}

func (tc *TestContext) branchIsKeptBecause(name, reason string) error {
	skipped, _ := tc.Config["skipped"].([]branch.SkippedBranch)
	for _, s := range skipped {
		if s.Name != name {
			continue
		}
		if !strings.Contains(s.Reason, reason) {
			return fmt.Errorf("branch %q kept because %q, expected it to mention %q", name, s.Reason, reason)
		}
		return nil
	}
	return fmt.Errorf("branch %q was not reported as kept", name)
}

func (tc *TestContext) theCleanupFailsWith(message string) error {
	err, _ := tc.Config["cleanup_error"].(error)
	if err == nil {
		return fmt.Errorf("expected a failure mentioning %q, got none", message)
	}
	if !strings.Contains(err.Error(), message) {
		return fmt.Errorf("failure %q does not mention %q", err, message)
	}
	return nil
}

func deletedNames(branches []branch.Info) []string {
	names := make([]string, 0, len(branches))
	for _, b := range branches {
		names = append(names, b.Name)
	}
	return names
}
