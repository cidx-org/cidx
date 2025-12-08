package remote

import (
	"fmt"
	"regexp"
	"strings"
)

// ProviderType represents the type of git remote provider
type ProviderType string

const (
	ProviderTypeGitHub  ProviderType = "github"
	ProviderTypeGitLab  ProviderType = "gitlab"
	ProviderTypeUnknown ProviderType = "unknown"
)

// DetectProviderFromURL detects the provider type from a git remote URL
// Supports both SSH (git@host:owner/repo.git) and HTTPS (https://host/owner/repo.git) formats
func DetectProviderFromURL(remoteURL string) ProviderType {
	url := strings.ToLower(remoteURL)

	// Check for GitHub
	if strings.Contains(url, "github.com") {
		return ProviderTypeGitHub
	}

	// Check for GitLab (gitlab.com or gitlab.*)
	if strings.Contains(url, "gitlab.com") || strings.Contains(url, "gitlab.") {
		return ProviderTypeGitLab
	}

	return ProviderTypeUnknown
}

// ExtractHostFromURL extracts the hostname from a git remote URL
// Returns the host for self-hosted instances
func ExtractHostFromURL(remoteURL string) string {
	// SSH format: git@host:owner/repo.git
	sshPattern := regexp.MustCompile(`git@([^:]+):`)
	if matches := sshPattern.FindStringSubmatch(remoteURL); len(matches) == 2 {
		return matches[1]
	}

	// HTTPS format: https://host/owner/repo.git
	httpsPattern := regexp.MustCompile(`https://([^/]+)/`)
	if matches := httpsPattern.FindStringSubmatch(remoteURL); len(matches) == 2 {
		return matches[1]
	}

	return ""
}

// ParseRemoteURL extracts owner and repo from a git remote URL
func ParseRemoteURL(remoteURL string) (owner, repo string, err error) {
	// SSH format: git@host:owner/repo.git
	sshPattern := regexp.MustCompile(`git@[^:]+:([^/]+)/(.+?)(?:\.git)?$`)
	if matches := sshPattern.FindStringSubmatch(remoteURL); len(matches) == 3 {
		return matches[1], matches[2], nil
	}

	// HTTPS format: https://host/owner/repo.git
	httpsPattern := regexp.MustCompile(`https://[^/]+/([^/]+)/(.+?)(?:\.git)?$`)
	if matches := httpsPattern.FindStringSubmatch(remoteURL); len(matches) == 3 {
		return matches[1], matches[2], nil
	}

	return "", "", fmt.Errorf("unable to parse remote URL: %s", remoteURL)
}

// BuildBaseURL constructs the API base URL for a provider
// For GitHub: https://api.github.com (or custom for enterprise)
// For GitLab: https://gitlab.com/api/v4 (or custom for self-hosted)
func BuildBaseURL(host string, providerType ProviderType) string {
	switch providerType {
	case ProviderTypeGitHub:
		if host == "github.com" {
			return "" // Use default API URL
		}
		return fmt.Sprintf("https://%s/api/v3", host)

	case ProviderTypeGitLab:
		if host == "gitlab.com" {
			return "" // Use default API URL
		}
		return fmt.Sprintf("https://%s", host)

	default:
		return ""
	}
}
