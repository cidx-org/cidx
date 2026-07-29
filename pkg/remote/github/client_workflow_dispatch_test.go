package github

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/google/go-github/v76/github"
)

// dispatchedRun builds a workflow_dispatch run as the API reports it.
func dispatchedRun(id int64, actor string, createdAt time.Time) *github.WorkflowRun {
	run := &github.WorkflowRun{ID: github.Ptr(id)}
	if actor != "" {
		run.Actor = &github.User{Login: github.Ptr(actor)}
	}
	if !createdAt.IsZero() {
		run.CreatedAt = &github.Timestamp{Time: createdAt}
	}
	return run
}

func TestPickDispatchedRun(t *testing.T) {
	dispatchedAt := time.Date(2026, 7, 29, 10, 0, 0, 0, time.UTC)

	t.Run("returns the single run that appeared after the dispatch", func(t *testing.T) {
		known := map[int64]bool{100: true}
		runs := []*github.WorkflowRun{
			dispatchedRun(101, "me", dispatchedAt.Add(time.Second)),
			dispatchedRun(100, "me", dispatchedAt.Add(-time.Hour)),
		}

		got := pickDispatchedRun(runs, known, "me", dispatchedAt)
		if got == nil || got.GetID() != 101 {
			t.Fatalf("expected run 101, got %v", got)
		}
	})

	t.Run("never returns a run that existed before the dispatch", func(t *testing.T) {
		known := map[int64]bool{100: true}
		runs := []*github.WorkflowRun{dispatchedRun(100, "me", dispatchedAt.Add(time.Second))}

		// Even with a created_at that would otherwise qualify: the snapshot
		// is the authority on what predates the dispatch.
		if got := pickDispatchedRun(runs, known, "me", dispatchedAt); got != nil {
			t.Fatalf("expected no candidate, got run %d", got.GetID())
		}
	})

	t.Run("ignores a run someone else triggered at the same moment", func(t *testing.T) {
		runs := []*github.WorkflowRun{
			// Lower ID, so it would win on age alone -- but it is not ours.
			dispatchedRun(101, "someone-else", dispatchedAt),
			dispatchedRun(102, "me", dispatchedAt),
		}

		got := pickDispatchedRun(runs, nil, "me", dispatchedAt)
		if got == nil || got.GetID() != 102 {
			t.Fatalf("expected our run 102, got %v", got)
		}
	})

	t.Run("takes the oldest run created at or after the dispatch", func(t *testing.T) {
		// Same account dispatching again while we poll: the later run is not
		// ours, and picking the newest would return it.
		runs := []*github.WorkflowRun{
			dispatchedRun(300, "me", dispatchedAt.Add(10*time.Second)),
			dispatchedRun(200, "me", dispatchedAt),
		}

		got := pickDispatchedRun(runs, nil, "me", dispatchedAt)
		if got == nil || got.GetID() != 200 {
			t.Fatalf("expected run 200, got %v", got)
		}
	})

	t.Run("drops runs created before the dispatch even when unknown", func(t *testing.T) {
		runs := []*github.WorkflowRun{dispatchedRun(50, "me", dispatchedAt.Add(-time.Minute))}

		if got := pickDispatchedRun(runs, nil, "me", dispatchedAt); got != nil {
			t.Fatalf("expected no candidate, got run %d", got.GetID())
		}
	})

	t.Run("keeps a run created in the same second as the dispatch", func(t *testing.T) {
		// created_at is reported to the second; the dispatch timestamp is not.
		runs := []*github.WorkflowRun{dispatchedRun(60, "me", dispatchedAt)}

		got := pickDispatchedRun(runs, nil, "me", dispatchedAt.Add(400*time.Millisecond))
		if got == nil || got.GetID() != 60 {
			t.Fatalf("expected run 60 to survive sub-second truncation, got %v", got)
		}
	})

	t.Run("skips the actor filter when the login is unknown", func(t *testing.T) {
		// A token with no user (GitHub App): the other filters still apply.
		runs := []*github.WorkflowRun{dispatchedRun(70, "whoever", dispatchedAt)}

		got := pickDispatchedRun(runs, nil, "", dispatchedAt)
		if got == nil || got.GetID() != 70 {
			t.Fatalf("expected run 70, got %v", got)
		}
	})
}

func TestResolveDispatchedRun(t *testing.T) {
	ctx := context.Background()
	dispatchedAt := time.Date(2026, 7, 29, 10, 0, 0, 0, time.UTC)

	// fakeClock advances only when the code under test sleeps, so the polling
	// loop is exercised without waiting.
	newProbe := func(list dispatchRunLister) *dispatchProbe {
		now := dispatchedAt
		return &dispatchProbe{
			list:     list,
			since:    dispatchedAt,
			interval: 2 * time.Second,
			timeout:  10 * time.Second,
			now:      func() time.Time { return now },
			sleep:    func(d time.Duration) { now = now.Add(d) },
		}
	}

	t.Run("polls until the run shows up", func(t *testing.T) {
		var calls int
		probe := newProbe(func(_ context.Context) ([]*github.WorkflowRun, error) {
			calls++
			if calls < 3 {
				return nil, nil
			}
			return []*github.WorkflowRun{dispatchedRun(900, "", dispatchedAt)}, nil
		})

		run, err := resolveDispatchedRun(ctx, *probe)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if run.GetID() != 900 {
			t.Errorf("got run %d, want 900", run.GetID())
		}
		if calls != 3 {
			t.Errorf("expected 3 listings, got %d", calls)
		}
	})

	t.Run("gives up with a message naming the wait when nothing appears", func(t *testing.T) {
		probe := newProbe(func(_ context.Context) ([]*github.WorkflowRun, error) {
			return nil, nil
		})

		_, err := resolveDispatchedRun(ctx, *probe)
		if err == nil {
			t.Fatal("expected an error when no run ever appears")
		}
		if !strings.Contains(err.Error(), "10s") {
			t.Errorf("error should name the timeout, got: %v", err)
		}
	})

	t.Run("reports the last listing failure when it never succeeded", func(t *testing.T) {
		apiErr := errors.New("503 Service Unavailable")
		probe := newProbe(func(_ context.Context) ([]*github.WorkflowRun, error) {
			return nil, apiErr
		})

		_, err := resolveDispatchedRun(ctx, *probe)
		if err == nil || !errors.Is(err, apiErr) {
			t.Fatalf("expected the listing error to be surfaced, got: %v", err)
		}
	})

	t.Run("a transient listing failure does not abort the wait", func(t *testing.T) {
		var calls int
		probe := newProbe(func(_ context.Context) ([]*github.WorkflowRun, error) {
			calls++
			if calls == 1 {
				return nil, errors.New("502 Bad Gateway")
			}
			return []*github.WorkflowRun{dispatchedRun(901, "", dispatchedAt)}, nil
		})

		run, err := resolveDispatchedRun(ctx, *probe)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if run.GetID() != 901 {
			t.Errorf("got run %d, want 901", run.GetID())
		}
	})
}

// errorResponse builds the go-github error a failed API call produces, so the
// mapping below is tested against the shape it really sees.
func errorResponse(status int, message string) (*github.Response, error) {
	httpResp := &http.Response{
		StatusCode: status,
		Request:    &http.Request{Method: "POST"},
	}
	return &github.Response{Response: httpResp}, &github.ErrorResponse{
		Response: httpResp,
		Message:  message,
	}
}

func TestDispatchError(t *testing.T) {
	tests := []struct {
		name     string
		status   int
		message  string
		contains []string
	}{
		{
			name:     "workflow without the workflow_dispatch trigger",
			status:   http.StatusUnprocessableEntity,
			message:  "Workflow does not have 'workflow_dispatch' trigger",
			contains: []string{"ci.yml", "workflow_dispatch", "default branch"},
		},
		{
			name:     "input the workflow does not declare",
			status:   http.StatusUnprocessableEntity,
			message:  `Unexpected inputs provided: ["nope"]`,
			contains: []string{"ci.yml", "inputs:", "nope"},
		},
		{
			name:     "required input missing",
			status:   http.StatusUnprocessableEntity,
			message:  `Required input 'environment' not provided`,
			contains: []string{"ci.yml", "--input key=value"},
		},
		{
			name:     "ref that was never pushed",
			status:   http.StatusUnprocessableEntity,
			message:  "No ref found for: feat/not-pushed",
			contains: []string{"feat/not-pushed", "push it first"},
		},
		{
			name:     "unknown workflow",
			status:   http.StatusNotFound,
			message:  "Not Found",
			contains: []string{"ci.yml", "not found"},
		},
		{
			name:     "anything else keeps the raw cause",
			status:   http.StatusInternalServerError,
			message:  "Server Error",
			contains: []string{"ci.yml", "feat/not-pushed", "Server Error"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp, apiErr := errorResponse(tt.status, tt.message)
			err := dispatchError("ci.yml", "feat/not-pushed", resp, apiErr)
			if err == nil {
				t.Fatal("expected an error")
			}
			for _, want := range tt.contains {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error %q should mention %q", err, want)
				}
			}
			if !errors.Is(err, apiErr) {
				t.Errorf("the API error should stay wrapped, got: %v", err)
			}
		})
	}
}

func TestResponseTime(t *testing.T) {
	t.Run("prefers the server clock", func(t *testing.T) {
		header := http.Header{}
		header.Set("Date", "Wed, 29 Jul 2026 10:00:00 GMT")
		resp := &github.Response{Response: &http.Response{Header: header}}

		got := responseTime(resp)
		want := time.Date(2026, 7, 29, 10, 0, 0, 0, time.UTC)
		if !got.Equal(want) {
			t.Errorf("got %s, want %s", got, want)
		}
	})

	t.Run("falls back to the local clock without a usable Date", func(t *testing.T) {
		before := time.Now().Add(-time.Second)
		if got := responseTime(nil); got.Before(before) {
			t.Errorf("expected a current timestamp, got %s", got)
		}
	})
}
