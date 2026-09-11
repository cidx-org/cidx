package github

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/google/go-github/v76/github"
)

type mergeTransport func(*http.Request) (*http.Response, error)

func (f mergeTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestMergeReturnsLandedCommitInsteadOfPRHead(t *testing.T) {
	for _, method := range []string{"merge", "squash", "rebase"} {
		t.Run(method, func(t *testing.T) {
			calls := 0
			transport := mergeTransport(func(r *http.Request) (*http.Response, error) {
				calls++
				body := `{"base":{"ref":"trunk"},"head":{"sha":"feature-head"},"merge_commit_sha":"preview-merge"}`
				if calls == 1 {
					if r.Method != "GET" || r.URL.Path != "/repos/o/r/pulls/7" {
						t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
					}
				} else {
					if calls != 2 || r.Method != "PUT" || r.URL.Path != "/repos/o/r/pulls/7/merge" {
						t.Fatalf("unexpected merge request %s %s", r.Method, r.URL.Path)
					}
					body = `{"merged":true,"sha":"landed-commit"}`
				}
				return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
			})
			c := &Client{client: github.NewClient(&http.Client{Transport: transport}), owner: "o", repo: "r"}
			result, err := c.MergePullRequest(context.Background(), 7, method)
			if err != nil {
				t.Fatal(err)
			}
			if calls != 2 || result.SHA != "landed-commit" || result.Branch != "trunk" {
				t.Fatalf("wrong merge target: %+v (%d calls)", result, calls)
			}
		})
	}
}
