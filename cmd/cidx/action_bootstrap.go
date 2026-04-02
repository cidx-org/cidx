package main

import (
	"fmt"
	"os"

	"github.com/cidx-org/cidx/pkg/config"
	"github.com/cidx-org/cidx/pkg/remote"
	"github.com/cidx-org/cidx/pkg/remote/github"
	"github.com/cidx-org/cidx/pkg/remote/gitlab"
	"github.com/cidx-org/cidx/pkg/vcs"
	"github.com/cli/go-gh/v2/pkg/auth"
)

// loadReleaseConfig loads the release configuration from cidx.toml or returns defaults
func loadReleaseConfig() config.ReleaseConfig {
	cfg, err := config.Load("cidx.toml")
	if err != nil {
		// Return defaults if no config file
		return config.DefaultReleaseConfig()
	}
	// Apply defaults for unset values
	if cfg.Release.MainBranch == "" {
		cfg.Release.MainBranch = "main"
	}
	// AutoCleanup defaults to true (zero value is false, so we need special handling)
	// This is already handled by the config loader or we use the default
	return cfg.Release
}

// loadTagConfig loads the tag configuration from cidx.toml or returns defaults
func loadTagConfig() config.TagConfig {
	cfg, err := config.Load("cidx.toml")
	if err != nil {
		// Return defaults if no config file
		return config.DefaultTagConfig()
	}
	return cfg.Tag
}

// loadProviderConfig loads the provider configuration from cidx.toml or returns empty config
func loadProviderConfig() config.ProviderConfig {
	cfg, err := config.Load("cidx.toml")
	if err != nil {
		return config.ProviderConfig{}
	}
	return cfg.Provider
}

// loadPRConfig loads the PR configuration from cidx.toml or returns defaults
// The parser starts with DefaultPRConfig() and only overrides explicitly set fields,
// so boolean defaults (true) are preserved when not specified in config
func loadPRConfig() config.PRConfig {
	cfg, err := config.Load("cidx.toml")
	if err != nil {
		return config.DefaultPRConfig()
	}
	return cfg.PR
}

// withRepo opens the repository and passes it to the callback
func withRepo(fn func(repo *vcs.Repository) error) error {
	repo, err := vcs.OpenRepository(".")
	if err != nil {
		return fmt.Errorf("failed to open repository: %w", err)
	}
	return fn(repo)
}

// withRepoAndProvider opens the repository, creates the provider, and passes both to the callback
func withRepoAndProvider(fn func(repo *vcs.Repository, provider remote.Provider) error) error {
	return withRepo(func(repo *vcs.Repository) error {
		provider, err := createProvider(repo)
		if err != nil {
			return err
		}
		return fn(repo, provider)
	})
}

// getGitHubToken retrieves GitHub token from env var or gh CLI auth
func getGitHubToken(host string) (string, error) {
	// 1. Try environment variable first
	if token := os.Getenv("GITHUB_TOKEN"); token != "" {
		return token, nil
	}

	// 2. Fallback to gh CLI auth
	if host == "" {
		host = "github.com"
	}
	token, _ := auth.TokenForHost(host)
	if token == "" {
		return "", fmt.Errorf("no GitHub token found: set GITHUB_TOKEN or run 'gh auth login'")
	}
	return token, nil
}

// createProvider creates the appropriate remote provider based on config and remote URL
func createProvider(repo *vcs.Repository) (remote.Provider, error) {
	// Load provider config
	providerCfg := loadProviderConfig()

	// Get remote URL for auto-detection
	remoteURL, err := repo.GetRemoteURL()
	if err != nil {
		return nil, fmt.Errorf("failed to get remote URL: %w", err)
	}

	// Get owner/repo info
	owner, repoName, err := repo.GetRemoteInfo()
	if err != nil {
		return nil, fmt.Errorf("failed to get remote info: %w", err)
	}

	// Determine provider type
	var providerType remote.ProviderType
	if providerCfg.Type != "" {
		// Explicit type from config
		providerType = remote.ProviderType(providerCfg.Type)
	} else {
		// Auto-detect from remote URL
		providerType = remote.DetectProviderFromURL(remoteURL)
	}

	// Get host for self-hosted instances
	host := remote.ExtractHostFromURL(remoteURL)

	// Create provider based on type
	switch providerType {
	case remote.ProviderTypeGitLab:
		// Get GitLab token
		token, err := gitlab.GetToken(host)
		if err != nil {
			return nil, err
		}

		// Check for custom base URL
		if providerCfg.URL != "" {
			return gitlab.NewClientWithBaseURL(token, owner, repoName, providerCfg.URL)
		}

		// Check if self-hosted (not gitlab.com)
		if host != "" && host != "gitlab.com" {
			baseURL := fmt.Sprintf("https://%s", host)
			return gitlab.NewClientWithBaseURL(token, owner, repoName, baseURL)
		}

		return gitlab.NewClient(token, owner, repoName), nil

	case remote.ProviderTypeGitHub:
		// Get GitHub token
		token, err := getGitHubToken(host)
		if err != nil {
			return nil, err
		}

		// Check for custom base URL (GitHub Enterprise)
		if providerCfg.URL != "" {
			return github.NewClientWithBaseURL(token, owner, repoName, providerCfg.URL)
		}

		return github.NewClient(token, owner, repoName), nil

	default:
		return nil, fmt.Errorf("unknown git provider for URL: %s (set [provider] type in cidx.toml)", remoteURL)
	}
}
