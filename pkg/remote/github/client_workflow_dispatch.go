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
//   - candidates triggered by another account are excluded, when the
//     authenticated login can be resolved;
//   - candidates created before the dispatch (server clock, from the response's
//     Date header) are excluded;
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

	list := func(ctx context.Context) ([]*github.WorkflowRun, error) {
		runs, _, err := c.client.Actions.ListWorkflowRunsByFileName(ctx, c.owner, c.repo, file,
			&github.ListWorkflowRunsOptions{
				Branch:      ref,
				Event:       "workflow_dispatch",
				ListOptions: github.ListOptions{PerPage: dispatchRunPageSize},
			})
		if err != nil {
			return nil, err
		}
		if runs == nil {
			return nil, nil
		}
		return runs.WorkflowRuns, nil
	}

	// Snapshot first: anything listed here predates the dispatch and can never
	// be the run it creates.
	known := map[int64]bool{}
	existing, err := list(ctx)
	if err != nil {
		// A workflow that has never been dispatched 404s here. That is not a
		// reason to refuse the dispatch -- it is exactly the first-run case --
		// so the snapshot stays empty and the dispatch below reports any real
		// problem with a message of its own.
		existing = nil
	}
	for _, run := range existing {
		known[run.GetID()] = true
	}

	actor := c.authenticatedLogin(ctx)

	dispatchedAt, err := c.dispatchWorkflow(ctx, file, ref, inputs)
	if err != nil {
		return nil, err
	}

	run, err := resolveDispatchedRun(ctx, dispatchProbe{
		list:     list,
		known:    known,
		actor:    actor,
		since:    dispatchedAt,
		interval: dispatchPollInterval,
		timeout:  dispatchPollTimeout,
	})
	if err != nil {
		return nil, fmt.Errorf("workflow %s was dispatched on %s but its run could not be identified: %w", file, ref, err)
	}

	return c.convertWorkflow(ctx, run)
}

// dispatchWorkflow performs the dispatch and returns the server time it was
// accepted at, which anchors the identification below.
func (c *Client) dispatchWorkflow(ctx context.Context, file, ref string, inputs map[string]string) (time.Time, error) {
	payload := github.CreateWorkflowDispatchEventRequest{Ref: ref}
	if len(inputs) > 0 {
		payload.Inputs = make(map[string]any, len(inputs))
		for k, v := range inputs {
			payload.Inputs[k] = v
		}
	}

	resp, err := c.client.Actions.CreateWorkflowDispatchEventByFileName(ctx, c.owner, c.repo, file, payload)
	if err != nil {
		return time.Time{}, dispatchError(file, ref, resp, err)
	}

	return responseTime(resp), nil
}

// responseTime reads the server's clock from the response, falling back to the
// local one. Using the server's avoids a skewed local clock making our own run
// look older than the dispatch that created it.
func responseTime(resp *github.Response) time.Time {
	if resp != nil && resp.Response != nil {
		if t, err := http.ParseTime(resp.Header.Get("Date")); err == nil {
			return t.UTC()
		}
	}
	return time.Now().UTC()
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
	since    time.Time      // server time the dispatch was accepted at
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
	// as the dispatch must not be dropped for being a fraction "too early".
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
