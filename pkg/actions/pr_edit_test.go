package actions

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// prEditFakeProvider extends fakeProvider with configurable PR lookup and a
// record of the UpdatePullRequest call, to exercise the pr edit flow.
type prEditFakeProvider struct {
	fakeProvider

	prNumber int
	prErr    error

	updateErr    error
	updateCalled bool
	updatedPR    int
	updatedTitle string
	updatedBody  string
}

func (f *prEditFakeProvider) GetPullRequestByBranch(_ context.Context, _ string) (int, string, error) {
	if f.prErr != nil {
		return 0, "", f.prErr
	}
	return f.prNumber, "https://example.test/pr", nil
}

func (f *prEditFakeProvider) UpdatePullRequest(_ context.Context, prNumber int, title, body string) error {
	f.updateCalled = true
	f.updatedPR = prNumber
	f.updatedTitle = title
	f.updatedBody = body
	return f.updateErr
}

func newPREditAction(provider *prEditFakeProvider, title, body string) *PREditAction {
	return &PREditAction{provider: provider, title: title, body: body}
}

func TestPREdit_TitleOnly(t *testing.T) {
	provider := &prEditFakeProvider{prNumber: 176}

	if err := newPREditAction(provider, "fix: better title", "").editPR(context.Background(), "feat/x"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !provider.updateCalled {
		t.Fatal("expected UpdatePullRequest to be called")
	}
	if provider.updatedPR != 176 {
		t.Errorf("expected update on PR #176, got #%d", provider.updatedPR)
	}
	if provider.updatedTitle != "fix: better title" {
		t.Errorf("expected title %q, got %q", "fix: better title", provider.updatedTitle)
	}
	if provider.updatedBody != "" {
		t.Errorf("expected body to be left unchanged (empty), got %q", provider.updatedBody)
	}
}

func TestPREdit_BodyOnly(t *testing.T) {
	provider := &prEditFakeProvider{prNumber: 176}

	if err := newPREditAction(provider, "", "New body").editPR(context.Background(), "feat/x"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if provider.updatedTitle != "" {
		t.Errorf("expected title to be left unchanged (empty), got %q", provider.updatedTitle)
	}
	if provider.updatedBody != "New body" {
		t.Errorf("expected body %q, got %q", "New body", provider.updatedBody)
	}
}

func TestPREdit_TitleAndBody(t *testing.T) {
	provider := &prEditFakeProvider{prNumber: 176}

	if err := newPREditAction(provider, "fix: title", "body").editPR(context.Background(), "feat/x"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if provider.updatedTitle != "fix: title" || provider.updatedBody != "body" {
		t.Errorf("expected title+body update, got title=%q body=%q", provider.updatedTitle, provider.updatedBody)
	}
}

func TestPREdit_NoFlagsIsAnError(t *testing.T) {
	provider := &prEditFakeProvider{prNumber: 176}

	err := newPREditAction(provider, "", "").editPR(context.Background(), "feat/x")
	if err == nil || !strings.Contains(err.Error(), "--title") || !strings.Contains(err.Error(), "--body") {
		t.Fatalf("expected error mentioning --title and --body, got: %v", err)
	}
	if provider.updateCalled {
		t.Error("expected no update call without flags")
	}
}

func TestPREdit_NoPRForBranchIsAClearError(t *testing.T) {
	provider := &prEditFakeProvider{prErr: errors.New("no open pull request found for branch feat/x")}

	err := newPREditAction(provider, "fix: title", "").editPR(context.Background(), "feat/x")
	if err == nil || !strings.Contains(err.Error(), "failed to find PR") {
		t.Fatalf("expected clear no-PR error, got: %v", err)
	}
	if provider.updateCalled {
		t.Error("expected no update call without a PR")
	}
}

func TestPREdit_UpdateErrorIsPropagated(t *testing.T) {
	provider := &prEditFakeProvider{prNumber: 176, updateErr: errors.New("API blew up")}

	err := newPREditAction(provider, "fix: title", "").editPR(context.Background(), "feat/x")
	if err == nil || !strings.Contains(err.Error(), "API blew up") {
		t.Fatalf("expected wrapped update error, got: %v", err)
	}
}
