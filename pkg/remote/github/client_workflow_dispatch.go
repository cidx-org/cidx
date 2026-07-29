package github

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/cidx-org/cidx/v2/pkg/remote"
	"github.com/google/go-github/v76/github"
)

const (
	// dispatchPollInterval is how often the runs of the dispatched workflow are
	// re-listed while waiting for the run to appear.
	dispatchPollInterval = 2 * time.Second

	// dispatchPollTimeout bounds that wait. GitHub usually creates the run
	// within a second or two; past this the dispatch is reported as accepted
	// but unidentified rather than hanging.
	dispatchPollTimeout = 45 * time.Second

	// dispatchRunPageSize is how many recent runs are read, both for the
	// pre-dispatch snapshot and for each poll. Large enough that a burst of
	// dispatches cannot push our run off the page.
	dispatchRunPageSize = 30

	// dispatchClockTolerance backs off the "created before we started" filter
	// when no server clock could be read before the dispatch. A local clock
	// running ahead of GitHub's would otherwise rule out our own run; the
	// pre-dispatch snapshot is what actually excludes older runs.
	dispatchClockTolerance = 2 * time.Minute
)

// TriggerWorkflow dispatches workflowFile on ref and returns the run it
// created.
//
// The API this builds on (POST .../workflows/{file}/dispatches) answers 204
// with an empty body: it does not say which run it created. The run is
// therefore identified afterwards, and the identification is deliberately
// conservative (issue #266):
//
//   - the runs of that workflow, on that ref, with event workflow_dispatch are
//     listed *before* the dispatch, and every one of them is excluded;
//   - candidates older than that listing are excluded too, which is what keeps
//     the filter meaningful if the listing itself failed. The cutoff is the
//     server's own clock, read from that response's Date header: GitHub creates
//     the run before it answers the dispatch, so anchoring on the dispatch's own
//     Date would rule out the very run we are looking for;
//   - candidates triggered by another account are excluded, when the
//     authenticated login can be resolved;
//   - of what survives, the oldest is taken: a dispatch made after ours can
//     only have produced a newer run.
//
// What that guarantees: the returned run is never one that already existed,
// never belongs to another workflow, ref or trigger, and never belongs to
// another user. What it cannot guarantee: if the same account dispatches the
// same workflow on the same ref at the same moment, the two runs are
// indistinguishable through this API and the wrong one may be returned.
func (c *Client) TriggerWorkflow(ctx context.Context, workflowFile, ref string, inputs map[string]string) (*remote.Workflow, error) {
	if ref == "" {
		return nil, fmt.Errorf("a ref is required to trigger a workflow")
	}
	file := remote.NormalizeWorkflowFile(workflowFile)

	listRuns := func(ctx context.Context) (*github.WorkflowRuns, *github.Response, error) {
		return c.client.Actions.ListWorkflowRunsByFileName(ctx, c.owner, c.repo, file,
			&github.ListWorkflowRunsOptions{
				Branch:      ref,
				Event:       "workflow_dispatch",
				ListOptions: github.ListOptions{PerPage: dispatchRunPageSize},
			})
	}

	// Snapshot first: anything listed here predates the dispatch and can never
	// be the run it creates. A workflow that has never been dispatched 404s
	// here, which is simply the first-run case -- the snapshot stays empty and
	// the dispatch below reports any real problem with a message of its own.
	known := map[int64]bool{}
	snapshot, snapshotResp, err := listRuns(ctx)
	if err == nil && snapshot != nil {
		for _, run := range snapshot.WorkflowRuns {
			known[run.GetID()] = true
		}
	}

	since, fromServer := serverTime(snapshotResp)
	if !fromServer {
		since = time.Now().UTC().Add(-dispatchClockTolerance)
	}

	actor := c.authenticatedLogin(ctx)

	if err := c.dispatchWorkflow(ctx, file, ref, inputs); err != nil {
		return nil, err
	}

	run, err := resolveDispatchedRun(ctx, dispatchProbe{
		list: func(ctx context.Context) ([]*github.WorkflowRun, error) {
			runs, _, err := listRuns(ctx)
			if err != nil {
				return nil, err
			}
			if runs == nil {
				return nil, nil
			}
			return runs.WorkflowRuns, nil
		},
		known:    known,
		actor:    actor,
		since:    since,
		interval: dispatchPollInterval,
		timeout:  dispatchPollTimeout,
	})
	if err != nil {
		return nil, fmt.Errorf("workflow %s was dispatched on %s but its run could not be identified: %w", file, ref, err)
	}

	return c.convertWorkflow(ctx, run)
}

// dispatchWorkflow performs the dispatch, translating the API's refusals.
func (c *Client) dispatchWorkflow(ctx context.Context, file, ref string, inputs map[string]string) error {
	payload := github.CreateWorkflowDispatchEventRequest{Ref: ref}
	if len(inputs) > 0 {
		payload.Inputs = make(map[string]any, len(inputs))
		for k, v := range inputs {
			payload.Inputs[k] = v
		}
	}

	resp, err := c.client.Actions.CreateWorkflowDispatchEventByFileName(ctx, c.owner, c.repo, file, payload)
	if err != nil {
		return dispatchError(file, ref, resp, err)
	}

	return nil
}

// serverTime reads GitHub's own clock from a response, reporting whether it
// could. Preferring it over the local one keeps a skewed workstation clock from
// ruling out the run the dispatch is about to create.
func serverTime(resp *github.Response) (time.Time, bool) {
	if resp != nil && resp.Response != nil {
		if t, err := http.ParseTime(resp.Header.Get("Date")); err == nil {
			return t.UTC(), true
		}
	}
	return time.Time{}, false
}

// authenticatedLogin returns the login the token belongs to, or "" when it
// cannot be resolved (a GitHub App token has no user). An empty login only
// widens the candidate set -- it never picks a wrong run on its own.
func (c *Client) authenticatedLogin(ctx context.Context) string {
	user, _, err := c.client.Users.Get(ctx, "")
	if err != nil {
		return ""
	}
	return user.GetLogin()
}

// dispatchError turns the API's refusals into messages that name what is
// wrong. GitHub answers every one of them with a bare 404 or 422 whose real
// explanation is buried in the go-github error string (issue #266).
func dispatchError(file, ref string, resp *github.Response, err error) error {
	msg := err.Error()
	status := 0
	if resp != nil && resp.Response != nil {
		status = resp.StatusCode
	}

	switch {
	// The trigger the whole command depends on. GitHub reads it from the
	// workflow file on the *default* branch, so declaring it only on a feature
	// branch is not enough -- which is the confusing half of this failure.
	case strings.Contains(msg, "workflow_dispatch"):
		return fmt.Errorf("workflow %s cannot be triggered: it does not declare the 'workflow_dispatch' trigger. Add `on: workflow_dispatch:` to .github/workflows/%s and merge it to the default branch first: %w", file, file, err)

	case strings.Contains(msg, "Unexpected inputs provided"):
		return fmt.Errorf("workflow %s rejected an input it does not declare -- check the `inputs:` block of .github/workflows/%s: %w", file, file, err)

	case strings.Contains(msg, "Required input"):
		return fmt.Errorf("workflow %s needs an input that was not given -- pass it with --input key=value: %w", file, err)

	case strings.Contains(msg, "No ref found"):
		return fmt.Errorf("ref %q does not exist on the remote -- push it first: %w", ref, err)

	case status == http.StatusNotFound:
		return fmt.Errorf("workflow %s not found in this repository (checked .github/workflows/%s): %w", file, file, err)
	}

	return fmt.Errorf("failed to trigger workflow %s on %s: %w", file, ref, err)
}

// dispatchRunLister lists the workflow_dispatch runs of one workflow on one
// ref. It is a seam so the identification below is unit-testable without a
// GitHub client.
type dispatchRunLister func(ctx context.Context) ([]*github.WorkflowRun, error)

// dispatchProbe carries everything resolveDispatchedRun needs to tell our run
// apart from anyone else's.
type dispatchProbe struct {
	list     dispatchRunLister
	known    map[int64]bool // run IDs that already existed before the dispatch
	actor    string         // authenticated login; "" disables the actor filter
	since    time.Time      // server time observed before the dispatch
	interval time.Duration
	timeout  time.Duration

	// now and sleep are seams so tests neither wait nor depend on the clock.
	now   func() time.Time
	sleep func(time.Duration)
}

// resolveDispatchedRun polls until the dispatched run shows up, or gives up.
func resolveDispatchedRun(ctx context.Context, p dispatchProbe) (*github.WorkflowRun, error) {
	if p.now == nil {
		p.now = time.Now
	}
	if p.sleep == nil {
		p.sleep = time.Sleep
	}

	deadline := p.now().Add(p.timeout)
	var lastErr error

	for {
		runs, err := p.list(ctx)
		if err != nil {
			lastErr = err
		} else if run := pickDispatchedRun(runs, p.known, p.actor, p.since); run != nil {
			return run, nil
		}

		if !p.now().Before(deadline) {
			break
		}

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
		p.sleep(p.interval)
	}

	if lastErr != nil {
		return nil, fmt.Errorf("no new run appeared within %s (last listing failed: %w)", p.timeout, lastErr)
	}
	return nil, fmt.Errorf("no new run appeared within %s", p.timeout)
}

// pickDispatchedRun returns the run a dispatch created, or nil when none of the
// listed runs can be shown to be it. See TriggerWorkflow for what the
// combination of filters does and does not guarantee.
func pickDispatchedRun(runs []*github.WorkflowRun, known map[int64]bool, actor string, since time.Time) *github.WorkflowRun {
	// created_at is reported to the second, so a run created in the same second
	// as the cutoff must not be dropped for being a fraction "too early".
	cutoff := since.Truncate(time.Second)

	var best *github.WorkflowRun
	for _, run := range runs {
		if run == nil || known[run.GetID()] {
			continue
		}
		if actor != "" && !strings.EqualFold(run.GetActor().GetLogin(), actor) {
			continue
		}
		if created := run.GetCreatedAt().Time; !created.IsZero() && created.Before(cutoff) {
			continue
		}
		// Run IDs increase with creation, so the smallest one is the earliest
		// run created at or after our dispatch -- ours, unless somebody raced
		// us within the same account.
		if best == nil || run.GetID() < best.GetID() {
			best = run
		}
	}

	return best
}
