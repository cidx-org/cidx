package github

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/google/go-github/v76/github"
)

// Client implements remote.Provider for GitHub Actions
type Client struct {
	client *github.Client
	owner  string
	repo   string
}

// NewClient creates a new GitHub client with token authentication
func NewClient(token, owner, repo string) *Client {
	client := github.NewClient(nil).WithAuthToken(token)

	return &Client{
		client: client,
		owner:  owner,
		repo:   repo,
	}
}

// NewClientWithBaseURL creates a new GitHub client for GitHub Enterprise with custom base URL
func NewClientWithBaseURL(token, owner, repo, baseURL string) (*Client, error) {
	client := github.NewClient(nil).WithAuthToken(token)

	// Parse and set the base URL for Enterprise
	var err error
	client, err = client.WithEnterpriseURLs(baseURL, baseURL)
	if err != nil {
		return nil, fmt.Errorf("failed to set enterprise URLs: %w", err)
	}

	return &Client{
		client: client,
		owner:  owner,
		repo:   repo,
	}, nil
}

// NewClientFromEnv creates a GitHub client using environment variables and git remote
func NewClientFromEnv() (*Client, error) {
	// Get token from environment
	token := getEnvToken()
	if token == "" {
		return nil, fmt.Errorf("GITHUB_TOKEN not set")
	}

	// Get owner/repo from git remote
	owner, repo, err := getRepoFromRemote()
	if err != nil {
		return nil, fmt.Errorf("failed to detect repository: %w", err)
	}

	return NewClient(token, owner, repo), nil
}

// getEnvToken returns GitHub token from environment or gh CLI
func getEnvToken() string {
	// 1. Check environment variables first
	for _, key := range []string{"GITHUB_TOKEN", "GH_TOKEN"} {
		if token := os.Getenv(key); token != "" {
			return token
		}
	}

	// 2. Fallback to gh CLI auth
	cmd := exec.Command("gh", "auth", "token")
	out, err := cmd.Output()
	if err == nil && len(out) > 0 {
		return strings.TrimSpace(string(out))
	}

	return ""
}

// getRepoFromRemote extracts owner/repo from git remote URL
func getRepoFromRemote() (owner, repo string, err error) {
	cmd := exec.Command("git", "remote", "get-url", "origin")
	out, err := cmd.Output()
	if err != nil {
		return "", "", fmt.Errorf("failed to get remote URL: %w", err)
	}

	url := string(out[:len(out)-1]) // Remove trailing newline
	return parseGitHubURL(url)
}

// parseGitHubURL extracts owner/repo from various GitHub URL formats
func parseGitHubURL(url string) (owner, repo string, err error) {
	// Handle SSH format: git@github.com:owner/repo.git
	if len(url) > 15 && url[:15] == "git@github.com:" {
		path := url[15:]
		path = trimSuffix(path, ".git")
		parts := splitN(path, "/", 2)
		if len(parts) == 2 {
			return parts[0], parts[1], nil
		}
	}

	// Handle HTTPS format: https://github.com/owner/repo.git
	if len(url) > 19 && url[:19] == "https://github.com/" {
		path := url[19:]
		path = trimSuffix(path, ".git")
		parts := splitN(path, "/", 2)
		if len(parts) == 2 {
			return parts[0], parts[1], nil
		}
	}

	return "", "", fmt.Errorf("unsupported URL format: %s", url)
}

// trimSuffix removes suffix from string
func trimSuffix(s, suffix string) string {
	if len(s) >= len(suffix) && s[len(s)-len(suffix):] == suffix {
		return s[:len(s)-len(suffix)]
	}
	return s
}

// splitN splits string into at most n parts
func splitN(s, sep string, n int) []string {
	var parts []string
	for i := 0; i < n-1; i++ {
		idx := -1
		for j := 0; j < len(s)-len(sep)+1; j++ {
			if s[j:j+len(sep)] == sep {
				idx = j
				break
			}
		}
		if idx == -1 {
			break
		}
		parts = append(parts, s[:idx])
		s = s[idx+len(sep):]
	}
	parts = append(parts, s)
	return parts
}
