package github

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/cidx-org/cidx/v3/pkg/remote"
	"github.com/google/go-github/v76/github"
)

// workflowAppSlug is the GitHub App that posts the check runs of the
// repository's own workflows. Every other app -- dependabot validating
// .github/dependabot.yml, an external CI service -- posts checks that exist
// without any workflow of this repository having started (issue #257).
const workflowAppSlug = "github-actions"

// checksPollInterval is how often the wait re-reads the checks of a PR.
// Package-level so tests can drive the loop without waiting on the clock.
var checksPollInterval = 2 * time.Second

// nonPullRequestEvents are the workflow-run events a pull request never causes.
// A run started by hand or by the clock lands on the PR's head SHA and its check
// runs show up against that commit, but they are not the PR's gate: dispatching
// Security Audit on a PR branch -- the natural way to validate a workflow change
// before merging -- made cpw exit red and pr merge refuse, forcing --skip-checks
// and defeating the gate entirely (issue #240).
//
// It is a deny list, not an allow list of "pull_request": a repository whose CI
// triggers on push alone still gates its PRs with those checks, and dropping
// them would turn a red gate green -- a worse failure than the one being fixed.
var nonPullRequestEvents = map[string]bool{
	"workflow_dispatch":   true,
	"schedule":            true,
	"repository_dispatch": true,
}

// getJobFunc reads one Actions job. It exists as a seam so the naming of the
// failing step can be unit-tested without a real GitHub client.
type getJobFunc func(ctx context.Context, jobID int64) (*github.WorkflowJob, error)

// getWorkflowJob is the real implementation behind getJobFunc.
func (c *Client) getWorkflowJob(ctx context.Context, jobID int64) (*github.WorkflowJob, error) {
	job, _, err := c.client.Actions.GetWorkflowJobByID(ctx, c.owner, c.repo, jobID)
	return job, err
}

// failedStepCache remembers, per head commit, the step a failed check run died
// on -- including the fact that there is none to name.
//
// It is what makes the read affordable at all. `cidx pr watch` re-reads the
// checks every few seconds, so without it a red run would re-request the job of
// every failed check on every cycle, for as long as someone watches: the
// rate-limit cost that had this deferred in #354. A completed job is a settled
// fact, so one request per failed check is enough however long the watch lasts.
//
// Re-running a job does not overwrite the old answer, it mints a new check run
// with a new ID -- verified on run 30898949737, whose second attempt renumbered
// every job (Test 91958590860 -> 91959086260) and whose commit's check-run
// listing returns only the new IDs. A cached entry therefore can never be
// served for a different attempt of the same job.
//
// Entries are scoped to the head SHA and dropped whole when it moves, so a
// watch spanning several pushes does not accumulate the checks of commits
// nobody is looking at any more.
type failedStepCache struct {
	mu    sync.Mutex
	sha   string
	steps map[int64]string
}

// lookup returns the step recorded for a check run of sha, and whether the read
// has already happened.
func (c *failedStepCache) lookup(sha string, id int64) (string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.sha != sha {
		return "", false
	}
	step, ok := c.steps[id]
	return step, ok
}

// store records step for a check run of sha, discarding the entries of any
// other commit.
func (c *failedStepCache) store(sha string, id int64, step string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.sha != sha || c.steps == nil {
		c.sha = sha
		c.steps = make(map[int64]string)
	}
	c.steps[id] = step
}

// failedStepOf names the step a failed check run died on.
//
// The check run itself does not carry one; the Actions job behind it does, in
// its steps. No lookup is needed to reach that job: a GitHub Actions check run
// and its job share an ID -- verified against run 30989450607, where check run
// 92251901265 "Test" is job 92251901265, reporting `Run tests` as the step that
// failed.
//
// Whatever the read cannot answer -- a 404, a rate limit, a job whose steps
// name no failure -- is cached as "no step" like any other answer. Retrying
// every three seconds is precisely what the cache exists to prevent, and a
// completed job that named no failing step now will not name one before the
// next push.
func failedStepOf(ctx context.Context, sha string, id int64, cache *failedStepCache, get getJobFunc) string {
	if step, cached := cache.lookup(sha, id); cached {
		return step
	}

	var step string
	if job, err := get(ctx, id); err == nil {
		step = firstFailedStep(job)
	}

	cache.store(sha, id, step)
	return step
}

// firstFailedStep returns the name of the first step of job whose conclusion is
// neither success nor skipped -- where the job stopped going right. A step with
// no conclusion at all never ran, so it is not where anything failed.
func firstFailedStep(job *github.WorkflowJob) string {
	for _, step := range job.Steps {
		switch step.GetConclusion() {
		case "", "success", "skipped":
			continue
		default:
			return step.GetName()
		}
	}
	return ""
}

// GetPullRequestChecks returns the status of all checks/workflows for a PR
func (c *Client) GetPullRequestChecks(ctx context.Context, prNumber int) (*remote.PRChecks, error) {
	// Get PR details to get the head SHA
	pr, _, err := c.client.PullRequests.Get(ctx, c.owner, c.repo, prNumber)
	if err != nil {
		return nil, fmt.Errorf("failed to get pull request: %w", err)
	}

	headSHA := pr.GetHead().GetSHA()

	checks := &remote.PRChecks{
		HeadSHA:      headSHA,
		UpdatedAt:    time.Now(),
		Checks:       []remote.CheckRun{},
		StatusChecks: []remote.StatusCheck{},
	}

	// Get check runs (GitHub Actions). PerPage is raised above the default 30
	// because a single dispatched workflow can post dozens of check runs on the
	// SHA and push the PR's own ones off the first page (issue #240).
	checkRuns, _, err := c.client.Checks.ListCheckRunsForRef(ctx, c.owner, c.repo, headSHA, &github.ListCheckRunsOptions{
		ListOptions: github.ListOptions{PerPage: 100},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list check runs: %w", err)
	}

	dispatched, inProgress := runsOnHead(ctx, headSHA, c.listRepositoryRuns)
	checks.RunsInProgress = inProgress

	addCheckRuns(checks, checkRuns.CheckRuns, dispatched, func(id int64) string {
		return failedStepOf(ctx, headSHA, id, &c.failedSteps, c.getWorkflowJob)
	})

	// Get commit status checks (legacy status API)
	statuses, _, err := c.client.Repositories.GetCombinedStatus(ctx, c.owner, c.repo, headSHA, &github.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list check runs: %w", err)
	}

	for _, status := range statuses.Statuses {
		statusCheck := remote.StatusCheck{
			Context: status.GetContext(),
			State:   status.GetState(),
			URL:     status.GetTargetURL(),
		}
		checks.StatusChecks = append(checks.StatusChecks, statusCheck)

		// Count by status
		checks.TotalCount++
		if status.GetState() == "pending" {
			checks.Pending++
		} else if status.GetState() == "success" {
			checks.Success++
		} else {
			checks.Failure++
		}
	}

	// Determine overall status. A run still going is pending even when every
	// check it has posted so far is green: the jobs it has not created yet are
	// the ones that would say otherwise (issue #367).
	if checks.Failure > 0 {
		checks.Status = "failure"
	} else if !checks.Complete() {
		checks.Status = "pending"
	} else {
		checks.Status = "success"
	}

	return checks, nil
}

// addCheckRuns folds the check runs of the PR's head commit into checks,
// leaving out those whose check suite belongs to a run the pull request did not
// cause (issue #240).
//
// failedStep names the step behind a failure, and is asked only about the
// checks that are counted as failures -- one request each, so a green check or
// one still running costs nothing (issue #355). A nil failedStep leaves the
// field empty.
func addCheckRuns(checks *remote.PRChecks, runs []*github.CheckRun, dispatched map[int64]bool, failedStep func(id int64) string) {
	for _, run := range runs {
		if dispatched[run.GetCheckSuite().GetID()] {
			continue
		}

		check := remote.CheckRun{
			ID:          run.GetID(),
			Name:        run.GetName(),
			Status:      run.GetStatus(),
			Conclusion:  run.GetConclusion(),
			URL:         run.GetHTMLURL(),
			StartedAt:   run.GetStartedAt().Time,
			CompletedAt: run.GetCompletedAt().Time,
		}

		// The error excerpt is whatever the app that posted the check chose to
		// summarise it with. GitHub Actions posts none -- output.summary comes
		// back null on every job check run of a red build -- so on Actions this
		// stays empty and the failing step below is the whole answer. The job
		// log is not read for it: the tail of a job log is the runner's own
		// cleanup, the tail of the failing step is `Process completed with exit
		// code 1`, and that is also all the annotations endpoint carries -- a
		// ~400 KB download per failed check to repeat what the step's name
		// already says (issue #355).
		if run.GetConclusion() == "failure" && run.Output != nil {
			if run.Output.Summary != nil && *run.Output.Summary != "" {
				// Truncate summary to first 200 chars for error preview
				summary := *run.Output.Summary
				if len(summary) > 200 {
					summary = summary[:200] + "..."
				}
				check.ErrorLog = summary
			}
		}

		// Only a check run posted by a workflow of the repository has an
		// Actions job behind it to read; another app's check run shares nothing
		// but the ID space with one (issue #355).
		if failedStep != nil && countsAsFailure(run) && isWorkflowCheck(run) {
			check.FailedStep = failedStep(run.GetID())
		}

		checks.Checks = append(checks.Checks, check)

		// Count by status
		checks.TotalCount++
		if isWorkflowCheck(run) {
			checks.WorkflowChecks++
		}
		switch run.GetStatus() {
		case "queued":
			checks.Queued++
			checks.Pending++
		case "in_progress":
			checks.InProgress++
			checks.Pending++
		case "completed":
			if countsAsFailure(run) {
				checks.Failure++
			} else {
				checks.Success++
			}
		default:
			checks.Pending++
		}
	}
}

// countsAsFailure reports whether run is one of the checks the Failure counter
// counts: a completed run whose conclusion is not success, skipped or neutral.
//
// It is one function rather than a rule written twice so the checks that get a
// step named are exactly the ones reported as failed -- a status block naming a
// check the count calls passed would contradict itself (issue #347).
func countsAsFailure(run *github.CheckRun) bool {
	if run.GetStatus() != "completed" {
		return false
	}
	switch run.GetConclusion() {
	case "success", "skipped", "neutral":
		return false
	}
	return true
}

// runsOnHead reads the repository's workflow runs for headSHA once and answers
// the two questions the PR's checks need of them.
//
// dispatched are the check suites produced by a run the pull request did not
// cause: a check run names its check suite but never the event behind it, so
// the mapping comes from this listing -- the only read that carries the event
// (issue #240).
//
// inProgress counts the runs the pull request *did* cause that have not
// finished. That is the count no amount of reading check runs can produce, and
// the reason this listing is now read for two things: a run stays in progress
// until its last job ends, including the jobs a `needs:` has not made eligible
// yet and which therefore have no check to be pending (issue #367).
//
// A failure yields no suites and no count, which keeps the pre-#240 behaviour
// of counting everything rather than failing the whole read. The cost is
// unchanged: one request per read of the PR's checks, as before.
func runsOnHead(ctx context.Context, headSHA string, list listRepoRunsFunc) (dispatched map[int64]bool, inProgress int) {
	runs, _, err := list(ctx, &github.ListWorkflowRunsOptions{
		HeadSHA:     headSHA,
		ListOptions: github.ListOptions{PerPage: 100},
	})
	if err != nil || runs == nil {
		return nil, 0
	}

	suites := make(map[int64]bool)
	for _, run := range runs.WorkflowRuns {
		if nonPullRequestEvents[run.GetEvent()] {
			if id := run.GetCheckSuiteID(); id != 0 {
				suites[id] = true
			}
			// Not this pull request's run, so whether it has finished says
			// nothing about whether the PR's checks are all in.
			continue
		}
		if run.GetStatus() != "completed" {
			inProgress++
		}
	}

	return suites, inProgress
}

// isWorkflowCheck reports whether a check run was posted by a workflow of the
// repository, as opposed to another app that happens to check the commit.
func isWorkflowCheck(run *github.CheckRun) bool {
	return run.GetApp().GetSlug() == workflowAppSlug
}

// WaitForChecksToStart waits for CI checks to start for a PR
// This solves the race condition where CI hasn't started yet when we query
func (c *Client) WaitForChecksToStart(ctx context.Context, prNumber int, expectedSHA string, timeout time.Duration) (string, *remote.PRChecks, error) {
	if expectedSHA == "" {
		// Resolve the head SHA from the API. Right after a push this read
		// can lag behind the true head; callers that know the SHA they
		// pushed should pass it explicitly (issue #167).
		pr, _, err := c.client.PullRequests.Get(ctx, c.owner, c.repo, prNumber)
		if err != nil {
			return "", nil, fmt.Errorf("failed to get pull request: %w", err)
		}
		expectedSHA = pr.GetHead().GetSHA()
	}

	checks, err := waitForWorkflowCheck(ctx, expectedSHA, timeout, func(ctx context.Context) (*remote.PRChecks, error) {
		return c.GetPullRequestChecks(ctx, prNumber)
	})
	return expectedSHA, checks, err
}

// getChecksFunc reads the current checks of a PR. It exists as a seam so
// waitForWorkflowCheck can be unit-tested without a real GitHub client.
type getChecksFunc func(ctx context.Context) (*remote.PRChecks, error)

// waitForWorkflowCheck polls until a check produced by a workflow of the
// repository appears on expectedSHA. A check posted by another app does not
// mean CI has started -- GitHub attaches its dependabot config check to every
// PR touching .github/dependabot.yml, long before the workflows are queued --
// so it does not end the wait (issue #257). On timeout whatever exists is
// returned: no checks at all means the repository has no CI, reported through
// the error; foreign checks only come back with WorkflowChecks == 0 for the
// caller to report them for what they are.
func waitForWorkflowCheck(ctx context.Context, expectedSHA string, timeout time.Duration, get getChecksFunc) (*remote.PRChecks, error) {
	// Create timeout context
	timeoutCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Poll for checks to appear
	ticker := time.NewTicker(checksPollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-timeoutCtx.Done():
			// Timeout reached - return current state with warning
			checks, err := get(ctx)
			if err != nil {
				return nil, fmt.Errorf("timeout waiting for CI to start (waited %v): %w", timeout, err)
			}
			// If no checks after timeout, it might be a repo without CI
			if checks.TotalCount == 0 {
				return checks, fmt.Errorf("no CI checks found after %v - repository may not have CI configured", timeout)
			}
			return checks, nil

		case <-ticker.C:
			checks, err := get(ctx)
			if err != nil {
				continue // Retry on transient errors
			}

			// Verify we're checking the right commit
			if checks.HeadSHA != expectedSHA {
				// SHA mismatch - CI might be running for old commit, wait for new one
				continue
			}

			// Check if a workflow of the repository has started
			if checks.WorkflowChecks > 0 {
				return checks, nil
			}

			// No workflow check yet, continue waiting
		}
	}
}

// WatchPullRequestChecks streams updates for PR checks until all complete
func (c *Client) WatchPullRequestChecks(ctx context.Context, prNumber int) (<-chan remote.PRChecksUpdate, error) {
	updates := make(chan remote.PRChecksUpdate, 1)

	go func() {
		defer close(updates)

		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()

		var lastStatus string

		for {
			select {
			case <-ctx.Done():
				return

			case <-ticker.C:
				checks, err := c.GetPullRequestChecks(ctx, prNumber)
				if err != nil {
					updates <- remote.PRChecksUpdate{Error: err}
					return
				}

				// Send update only if status changed. RunsInProgress is part of
				// the fingerprint: the last thing that happens on a green run is
				// the run itself completing, with no check changing (issue #367).
				currentStatus := fmt.Sprintf("%s:%d:%d:%d:%d", checks.Status, checks.Pending, checks.Success, checks.Failure, checks.RunsInProgress)
				if currentStatus != lastStatus {
					updates <- remote.PRChecksUpdate{Checks: checks}
					lastStatus = currentStatus
				}

				// Stop when there is nothing left to wait for. Closing on
				// `Pending == 0` ended the stream in the gap between two stages,
				// so a consumer could not have waited longer even knowing better
				// (issue #367).
				if checks.Complete() {
					return
				}
			}
		}
	}()

	return updates, nil
}
