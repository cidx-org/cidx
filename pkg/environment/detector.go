package environment

import (
	"os"
	"strings"
)

// Environment represents the execution environment
type Environment struct {
	IsCI       bool
	Provider   string // github, gitlab, jenkins, etc.
	IsPR       bool   // Is this a pull/merge request?
	IsTag      bool   // Is this a tag build?
	BranchName string
	TagName    string
}

// Detect automatically detects the current environment
func Detect() *Environment {
	env := &Environment{
		IsCI:       false,
		Provider:   "local",
		IsPR:       false,
		IsTag:      false,
		BranchName: "",
		TagName:    "",
	}

	// Detect CI environment
	if os.Getenv("CI") == "true" || os.Getenv("CI") == "1" {
		env.IsCI = true
	}

	// Detect GitHub Actions
	if os.Getenv("GITHUB_ACTIONS") == "true" {
		env.IsCI = true
		env.Provider = "github"
		env.BranchName = os.Getenv("GITHUB_REF_NAME")

		// Detect PR
		if os.Getenv("GITHUB_EVENT_NAME") == "pull_request" {
			env.IsPR = true
		}

		// Detect tag
		ref := os.Getenv("GITHUB_REF")
		if strings.HasPrefix(ref, "refs/tags/") {
			env.IsTag = true
			env.TagName = os.Getenv("GITHUB_REF_NAME")
		}
	}

	// Detect GitLab CI
	if os.Getenv("GITLAB_CI") == "true" {
		env.IsCI = true
		env.Provider = "gitlab"
		env.BranchName = os.Getenv("CI_COMMIT_REF_NAME")

		// Detect MR
		if os.Getenv("CI_MERGE_REQUEST_ID") != "" {
			env.IsPR = true
		}

		// Detect tag
		if os.Getenv("CI_COMMIT_TAG") != "" {
			env.IsTag = true
			env.TagName = os.Getenv("CI_COMMIT_TAG")
		}
	}

	// Detect Jenkins
	if os.Getenv("JENKINS_HOME") != "" || os.Getenv("JENKINS_URL") != "" {
		env.IsCI = true
		env.Provider = "jenkins"
		env.BranchName = os.Getenv("BRANCH_NAME")
		env.TagName = os.Getenv("TAG_NAME")
		if env.TagName != "" {
			env.IsTag = true
		}
	}

	// Detect CircleCI
	if os.Getenv("CIRCLECI") == "true" {
		env.IsCI = true
		env.Provider = "circleci"
		env.BranchName = os.Getenv("CIRCLE_BRANCH")
		env.TagName = os.Getenv("CIRCLE_TAG")
		if env.TagName != "" {
			env.IsTag = true
		}
		if os.Getenv("CIRCLE_PULL_REQUEST") != "" {
			env.IsPR = true
		}
	}

	return env
}

// IsLocal returns true if running in local environment
func (e *Environment) IsLocal() bool {
	return !e.IsCI
}

// String returns a human-readable representation
func (e *Environment) String() string {
	if e.IsLocal() {
		return "local"
	}
	return e.Provider
}
