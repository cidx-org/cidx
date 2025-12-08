package remote

import "testing"

func TestDetectProviderFromURL(t *testing.T) {
	tests := []struct {
		name     string
		url      string
		expected ProviderType
	}{
		// GitHub URLs
		{
			name:     "GitHub HTTPS",
			url:      "https://github.com/owner/repo.git",
			expected: ProviderTypeGitHub,
		},
		{
			name:     "GitHub SSH",
			url:      "git@github.com:owner/repo.git",
			expected: ProviderTypeGitHub,
		},
		{
			name:     "GitHub HTTPS without .git",
			url:      "https://github.com/owner/repo",
			expected: ProviderTypeGitHub,
		},

		// GitLab URLs
		{
			name:     "GitLab HTTPS",
			url:      "https://gitlab.com/owner/repo.git",
			expected: ProviderTypeGitLab,
		},
		{
			name:     "GitLab SSH",
			url:      "git@gitlab.com:owner/repo.git",
			expected: ProviderTypeGitLab,
		},
		{
			name:     "GitLab self-hosted",
			url:      "https://gitlab.mycompany.com/owner/repo.git",
			expected: ProviderTypeGitLab,
		},
		{
			name:     "GitLab self-hosted SSH",
			url:      "git@gitlab.mycompany.com:owner/repo.git",
			expected: ProviderTypeGitLab,
		},

		// Unknown
		{
			name:     "Unknown provider",
			url:      "https://bitbucket.org/owner/repo.git",
			expected: ProviderTypeUnknown,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := DetectProviderFromURL(tt.url)
			if result != tt.expected {
				t.Errorf("DetectProviderFromURL(%q) = %v, want %v", tt.url, result, tt.expected)
			}
		})
	}
}

func TestExtractHostFromURL(t *testing.T) {
	tests := []struct {
		name     string
		url      string
		expected string
	}{
		{
			name:     "GitHub HTTPS",
			url:      "https://github.com/owner/repo.git",
			expected: "github.com",
		},
		{
			name:     "GitHub SSH",
			url:      "git@github.com:owner/repo.git",
			expected: "github.com",
		},
		{
			name:     "GitLab self-hosted HTTPS",
			url:      "https://gitlab.mycompany.com/owner/repo.git",
			expected: "gitlab.mycompany.com",
		},
		{
			name:     "GitLab self-hosted SSH",
			url:      "git@gitlab.mycompany.com:owner/repo.git",
			expected: "gitlab.mycompany.com",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ExtractHostFromURL(tt.url)
			if result != tt.expected {
				t.Errorf("ExtractHostFromURL(%q) = %v, want %v", tt.url, result, tt.expected)
			}
		})
	}
}

func TestParseRemoteURL(t *testing.T) {
	tests := []struct {
		name          string
		url           string
		expectedOwner string
		expectedRepo  string
		expectError   bool
	}{
		{
			name:          "GitHub HTTPS",
			url:           "https://github.com/owner/repo.git",
			expectedOwner: "owner",
			expectedRepo:  "repo",
			expectError:   false,
		},
		{
			name:          "GitHub SSH",
			url:           "git@github.com:owner/repo.git",
			expectedOwner: "owner",
			expectedRepo:  "repo",
			expectError:   false,
		},
		{
			name:          "GitLab HTTPS",
			url:           "https://gitlab.com/group/project.git",
			expectedOwner: "group",
			expectedRepo:  "project",
			expectError:   false,
		},
		{
			name:          "GitLab self-hosted",
			url:           "https://gitlab.mycompany.com/team/app.git",
			expectedOwner: "team",
			expectedRepo:  "app",
			expectError:   false,
		},
		{
			name:          "Without .git suffix",
			url:           "https://github.com/owner/repo",
			expectedOwner: "owner",
			expectedRepo:  "repo",
			expectError:   false,
		},
		{
			name:        "Invalid URL",
			url:         "not-a-valid-url",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			owner, repo, err := ParseRemoteURL(tt.url)
			if tt.expectError {
				if err == nil {
					t.Errorf("ParseRemoteURL(%q) expected error, got nil", tt.url)
				}
				return
			}
			if err != nil {
				t.Errorf("ParseRemoteURL(%q) unexpected error: %v", tt.url, err)
				return
			}
			if owner != tt.expectedOwner {
				t.Errorf("ParseRemoteURL(%q) owner = %q, want %q", tt.url, owner, tt.expectedOwner)
			}
			if repo != tt.expectedRepo {
				t.Errorf("ParseRemoteURL(%q) repo = %q, want %q", tt.url, repo, tt.expectedRepo)
			}
		})
	}
}
