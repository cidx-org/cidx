package environment

import (
	"os"
	"testing"
)

// clearCIEnvVars unsets all CI-related environment variables for clean test state
func clearCIEnvVars(t *testing.T) {
	t.Helper()
	vars := []string{
		"CI", "GITHUB_ACTIONS", "GITHUB_REF_NAME", "GITHUB_EVENT_NAME",
		"GITHUB_REF", "GITLAB_CI", "CI_COMMIT_REF_NAME", "CI_MERGE_REQUEST_ID",
		"CI_COMMIT_TAG", "JENKINS_HOME", "JENKINS_URL", "BRANCH_NAME",
		"TAG_NAME", "CIRCLECI", "CIRCLE_BRANCH", "CIRCLE_TAG", "CIRCLE_PULL_REQUEST",
	}
	for _, v := range vars {
		os.Unsetenv(v)
	}
}

func TestDetect_Local(t *testing.T) {
	clearCIEnvVars(t)

	env := Detect()

	if env.IsCI {
		t.Error("expected IsCI=false for local environment")
	}
	if env.Provider != "local" {
		t.Errorf("expected provider 'local', got %q", env.Provider)
	}
	if env.IsPR {
		t.Error("expected IsPR=false")
	}
	if env.IsTag {
		t.Error("expected IsTag=false")
	}
}

func TestDetect_GitHubActions(t *testing.T) {
	clearCIEnvVars(t)
	os.Setenv("GITHUB_ACTIONS", "true")
	os.Setenv("GITHUB_REF_NAME", "feature/test")
	defer clearCIEnvVars(t)

	env := Detect()

	if !env.IsCI {
		t.Error("expected IsCI=true")
	}
	if env.Provider != "github" {
		t.Errorf("expected provider 'github', got %q", env.Provider)
	}
	if env.BranchName != "feature/test" {
		t.Errorf("expected branch 'feature/test', got %q", env.BranchName)
	}
}

func TestDetect_GitHubActions_PR(t *testing.T) {
	clearCIEnvVars(t)
	os.Setenv("GITHUB_ACTIONS", "true")
	os.Setenv("GITHUB_EVENT_NAME", "pull_request")
	defer clearCIEnvVars(t)

	env := Detect()

	if !env.IsPR {
		t.Error("expected IsPR=true for pull_request event")
	}
}

func TestDetect_GitHubActions_Tag(t *testing.T) {
	clearCIEnvVars(t)
	os.Setenv("GITHUB_ACTIONS", "true")
	os.Setenv("GITHUB_REF", "refs/tags/v1.0.0")
	os.Setenv("GITHUB_REF_NAME", "v1.0.0")
	defer clearCIEnvVars(t)

	env := Detect()

	if !env.IsTag {
		t.Error("expected IsTag=true for tag ref")
	}
	if env.TagName != "v1.0.0" {
		t.Errorf("expected tag 'v1.0.0', got %q", env.TagName)
	}
}

func TestDetect_GitLabCI(t *testing.T) {
	clearCIEnvVars(t)
	os.Setenv("GITLAB_CI", "true")
	os.Setenv("CI_COMMIT_REF_NAME", "develop")
	defer clearCIEnvVars(t)

	env := Detect()

	if !env.IsCI {
		t.Error("expected IsCI=true")
	}
	if env.Provider != "gitlab" {
		t.Errorf("expected provider 'gitlab', got %q", env.Provider)
	}
	if env.BranchName != "develop" {
		t.Errorf("expected branch 'develop', got %q", env.BranchName)
	}
}

func TestDetect_GitLabCI_MR(t *testing.T) {
	clearCIEnvVars(t)
	os.Setenv("GITLAB_CI", "true")
	os.Setenv("CI_MERGE_REQUEST_ID", "42")
	defer clearCIEnvVars(t)

	env := Detect()

	if !env.IsPR {
		t.Error("expected IsPR=true for merge request")
	}
}

func TestDetect_GitLabCI_Tag(t *testing.T) {
	clearCIEnvVars(t)
	os.Setenv("GITLAB_CI", "true")
	os.Setenv("CI_COMMIT_TAG", "v2.0.0")
	defer clearCIEnvVars(t)

	env := Detect()

	if !env.IsTag {
		t.Error("expected IsTag=true")
	}
	if env.TagName != "v2.0.0" {
		t.Errorf("expected tag 'v2.0.0', got %q", env.TagName)
	}
}

func TestDetect_Jenkins(t *testing.T) {
	clearCIEnvVars(t)
	os.Setenv("JENKINS_HOME", "/var/jenkins")
	os.Setenv("BRANCH_NAME", "main")
	defer clearCIEnvVars(t)

	env := Detect()

	if !env.IsCI {
		t.Error("expected IsCI=true")
	}
	if env.Provider != "jenkins" {
		t.Errorf("expected provider 'jenkins', got %q", env.Provider)
	}
	if env.BranchName != "main" {
		t.Errorf("expected branch 'main', got %q", env.BranchName)
	}
}

func TestDetect_Jenkins_Tag(t *testing.T) {
	clearCIEnvVars(t)
	os.Setenv("JENKINS_URL", "http://jenkins.local")
	os.Setenv("TAG_NAME", "v3.0.0")
	defer clearCIEnvVars(t)

	env := Detect()

	if !env.IsTag {
		t.Error("expected IsTag=true")
	}
	if env.TagName != "v3.0.0" {
		t.Errorf("expected tag 'v3.0.0', got %q", env.TagName)
	}
}

func TestDetect_CircleCI(t *testing.T) {
	clearCIEnvVars(t)
	os.Setenv("CIRCLECI", "true")
	os.Setenv("CIRCLE_BRANCH", "feature/ci")
	defer clearCIEnvVars(t)

	env := Detect()

	if !env.IsCI {
		t.Error("expected IsCI=true")
	}
	if env.Provider != "circleci" {
		t.Errorf("expected provider 'circleci', got %q", env.Provider)
	}
	if env.BranchName != "feature/ci" {
		t.Errorf("expected branch 'feature/ci', got %q", env.BranchName)
	}
}

func TestDetect_CircleCI_Tag(t *testing.T) {
	clearCIEnvVars(t)
	os.Setenv("CIRCLECI", "true")
	os.Setenv("CIRCLE_TAG", "v4.0.0")
	defer clearCIEnvVars(t)

	env := Detect()

	if !env.IsTag {
		t.Error("expected IsTag=true")
	}
	if env.TagName != "v4.0.0" {
		t.Errorf("expected tag 'v4.0.0', got %q", env.TagName)
	}
}

func TestDetect_CircleCI_PR(t *testing.T) {
	clearCIEnvVars(t)
	os.Setenv("CIRCLECI", "true")
	os.Setenv("CIRCLE_PULL_REQUEST", "https://github.com/org/repo/pull/123")
	defer clearCIEnvVars(t)

	env := Detect()

	if !env.IsPR {
		t.Error("expected IsPR=true for Circle CI pull request")
	}
}

func TestDetect_GenericCI(t *testing.T) {
	clearCIEnvVars(t)
	os.Setenv("CI", "true")
	defer clearCIEnvVars(t)

	env := Detect()

	if !env.IsCI {
		t.Error("expected IsCI=true when CI=true")
	}
}
