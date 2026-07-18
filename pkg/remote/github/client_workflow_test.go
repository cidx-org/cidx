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
