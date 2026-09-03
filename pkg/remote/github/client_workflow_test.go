package github

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/google/go-github/v76/github"
)

// fakeRuns builds a WorkflowRuns response containing a single run with the
// given ID.
func fakeRuns(id int64) *github.WorkflowRuns {
	return &github.WorkflowRuns{WorkflowRuns: []*github.WorkflowRun{{ID: &id}}}
}

func notFoundResponse() *github.Response {
	return &github.Response{Response: &http.Response{StatusCode: http.StatusNotFound}}
}

func TestAllWorkflowJobsReadsPastTheFirstPage(t *testing.T) {
	failed := "failure"
	name := "Report"
	var pages []int

	jobs, err := allWorkflowJobs(context.Background(), func(_ context.Context, opts *github.ListWorkflowJobsOptions) (*github.Jobs, *github.Response, error) {
		pages = append(pages, opts.Page)
		if opts.Page == 0 {
			return &github.Jobs{Jobs: []*github.WorkflowJob{{Name: github.Ptr("Setup")}}}, &github.Response{NextPage: 2}, nil
		}
		return &github.Jobs{Jobs: []*github.WorkflowJob{{Name: &name, Conclusion: &failed}}}, &github.Response{}, nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(jobs) != 2 || jobs[1].GetName() != "Report" {
		t.Fatalf("expected the failed job from page 2, got %+v", jobs)
	}
	if len(pages) != 2 || pages[0] != 0 || pages[1] != 2 {
		t.Fatalf("expected pages [0 2], got %v", pages)
	}
}

func TestLatestRunFromCandidates(t *testing.T) {
	ctx := context.Background()

	t.Run("uses cidx.yml when it has runs", func(t *testing.T) {
		var asked []string
		run, err := latestRunFromCandidates(ctx, "main", func(_ context.Context, file string) (*github.WorkflowRuns, *github.Response, error) {
			asked = append(asked, file)
			return fakeRuns(1), nil, nil
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if run.GetID() != 1 {
			t.Errorf("got run ID %d, want 1", run.GetID())
		}
		if len(asked) != 1 || asked[0] != "cidx.yml" {
			t.Errorf("expected a single probe of cidx.yml, got %v", asked)
		}
	})

	t.Run("falls back to ci.yml when cidx.yml does not exist (404)", func(t *testing.T) {
		var asked []string
		run, err := latestRunFromCandidates(ctx, "main", func(_ context.Context, file string) (*github.WorkflowRuns, *github.Response, error) {
			asked = append(asked, file)
			if file == "cidx.yml" {
				return nil, notFoundResponse(), errors.New("404 Not Found")
			}
			return fakeRuns(2), nil, nil
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if run.GetID() != 2 {
			t.Errorf("got run ID %d, want 2", run.GetID())
		}
		if want := []string{"cidx.yml", "ci.yml"}; len(asked) != 2 || asked[0] != want[0] || asked[1] != want[1] {
			t.Errorf("expected probes %v, got %v", want, asked)
		}
	})

	t.Run("falls back to ci.yml when cidx.yml exists but has no runs for the branch", func(t *testing.T) {
		run, err := latestRunFromCandidates(ctx, "main", func(_ context.Context, file string) (*github.WorkflowRuns, *github.Response, error) {
			if file == "cidx.yml" {
				return &github.WorkflowRuns{}, nil, nil
			}
			return fakeRuns(3), nil, nil
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if run.GetID() != 3 {
			t.Errorf("got run ID %d, want 3", run.GetID())
		}
	})

	t.Run("errors listing every candidate when none has runs", func(t *testing.T) {
		_, err := latestRunFromCandidates(ctx, "feature-x", func(_ context.Context, _ string) (*github.WorkflowRuns, *github.Response, error) {
			return &github.WorkflowRuns{}, nil, nil
		})
		if err == nil {
			t.Fatal("expected an error, got nil")
		}
		for _, want := range []string{"feature-x", "cidx.yml", "ci.yml"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("error %q should mention %q", err, want)
			}
		}
	})

	t.Run("propagates non-404 API errors without probing further", func(t *testing.T) {
		var calls int
		apiErr := errors.New("503 Service Unavailable")
		_, err := latestRunFromCandidates(ctx, "main", func(_ context.Context, _ string) (*github.WorkflowRuns, *github.Response, error) {
			calls++
			return nil, &github.Response{Response: &http.Response{StatusCode: http.StatusServiceUnavailable}}, apiErr
		})
		if err == nil || !errors.Is(err, apiErr) {
			t.Fatalf("expected wrapped API error, got %v", err)
		}
		if calls != 1 {
			t.Errorf("expected 1 call before aborting, got %d", calls)
		}
	})
}

func TestLatestRunForRef(t *testing.T) {
	ctx := context.Background()

	t.Run("filters the repository-wide runs on the tag ref", func(t *testing.T) {
		var got *github.ListWorkflowRunsOptions
		run, err := latestRunForRef(ctx, "v2.1.4", "tag", func(_ context.Context, opts *github.ListWorkflowRunsOptions) (*github.WorkflowRuns, *github.Response, error) {
			got = opts
			// GitHub stores the ref that triggered the run in head_branch; for
			// a tag push that is the tag name, so the release run comes back
			// and the CI/nightly runs of the same commit do not.
			return fakeRuns(30283021151), nil, nil
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if run.GetID() != 30283021151 {
			t.Errorf("got run ID %d, want the release run", run.GetID())
		}
		if got == nil || got.Branch != "v2.1.4" {
			t.Errorf("expected the listing to be filtered on ref v2.1.4, got %+v", got)
		}
		if got.PerPage != 1 {
			t.Errorf("expected only the latest run to be fetched, got PerPage=%d", got.PerPage)
		}
	})

	t.Run("names the ref kind when nothing matches", func(t *testing.T) {
		_, err := latestRunForRef(ctx, "v9.9.9", "tag", func(_ context.Context, _ *github.ListWorkflowRunsOptions) (*github.WorkflowRuns, *github.Response, error) {
			return &github.WorkflowRuns{}, nil, nil
		})
		if err == nil {
			t.Fatal("expected an error when the tag triggered no run")
		}
		for _, want := range []string{"tag", "v9.9.9"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("error %q should mention %q", err, want)
			}
		}
	})

	t.Run("branch lookups keep their own wording", func(t *testing.T) {
		_, err := latestRunForRef(ctx, "feature-x", "branch", func(_ context.Context, _ *github.ListWorkflowRunsOptions) (*github.WorkflowRuns, *github.Response, error) {
			return nil, nil, nil
		})
		if err == nil || !strings.Contains(err.Error(), "branch feature-x") {
			t.Fatalf("expected a branch-specific error, got %v", err)
		}
	})

	t.Run("propagates API errors", func(t *testing.T) {
		apiErr := errors.New("503 Service Unavailable")
		_, err := latestRunForRef(ctx, "v2.1.4", "tag", func(_ context.Context, _ *github.ListWorkflowRunsOptions) (*github.WorkflowRuns, *github.Response, error) {
			return nil, &github.Response{Response: &http.Response{StatusCode: http.StatusServiceUnavailable}}, apiErr
		})
		if err == nil || !errors.Is(err, apiErr) {
			t.Fatalf("expected wrapped API error, got %v", err)
		}
	})
}
