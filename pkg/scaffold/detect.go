// Package scaffold detects project type and generates cidx.toml configuration.
package scaffold

import (
	"os"
	"path/filepath"
)

// Language represents a detected project language/ecosystem.
type Language struct {
	Name     string
	Marker   string // file that triggered detection
	Security []string
	Code     []string
	Test     []string
	Build    []string
}

// Detection holds the results of project analysis.
type Detection struct {
	Languages []Language
	HasGit    bool
	Remote    string // "github" or "gitlab" or ""
}

// Detect analyzes the current directory to determine project type.
func Detect(dir string) *Detection {
	d := &Detection{}

	// Check git
	if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
		d.HasGit = true
		d.Remote = detectRemote(dir)
	}

	// Detect languages
	markers := []struct {
		file string
		lang Language
	}{
		{"go.mod", Language{
			Name: "Go", Marker: "go.mod",
			Security: []string{"trivy", "gitleaks", "gosec"},
			Code:     []string{"golangci-lint", "gofmt", "prettier", "commitizen"},
			Test:     []string{"go-test"},
			Build:    []string{"go-build"},
		}},
		{"package.json", Language{
			Name: "Node.js", Marker: "package.json",
			Security: []string{"trivy", "gitleaks"},
			Code:     []string{"prettier", "commitizen"},
			Test:     []string{},
			Build:    []string{},
		}},
		{"pyproject.toml", Language{
			Name: "Python", Marker: "pyproject.toml",
			Security: []string{"trivy", "gitleaks", "bandit"},
			Code:     []string{"ruff", "black", "mypy", "prettier", "commitizen"},
			Test:     []string{"pytest"},
			Build:    []string{"python-build"},
		}},
		{"requirements.txt", Language{
			Name: "Python", Marker: "requirements.txt",
			Security: []string{"trivy", "gitleaks", "bandit", "pip-audit"},
			Code:     []string{"ruff", "black", "prettier", "commitizen"},
			Test:     []string{"pytest"},
			Build:    []string{},
		}},
		{"Cargo.toml", Language{
			Name: "Rust", Marker: "Cargo.toml",
			Security: []string{"trivy", "gitleaks", "cargo-audit"},
			Code:     []string{"clippy", "rustfmt", "prettier", "commitizen"},
			Test:     []string{"cargo-test"},
			Build:    []string{"cargo-build"},
		}},
		{"molecule/", Language{
			Name: "Ansible", Marker: "molecule/",
			Security: []string{"trivy", "gitleaks"},
			Code:     []string{"ansible-lint", "yamllint", "prettier", "commitizen"},
			Test:     []string{"molecule"},
			Build:    []string{},
		}},
	}

	seen := make(map[string]bool)
	for _, m := range markers {
		path := filepath.Join(dir, m.file)
		info, err := os.Stat(path)
		if err != nil {
			continue
		}
		// For directories (like molecule/), check it's a dir
		if m.file[len(m.file)-1] == '/' && !info.IsDir() {
			continue
		}
		if !seen[m.lang.Name] {
			seen[m.lang.Name] = true
			d.Languages = append(d.Languages, m.lang)
		}
	}

	return d
}

// detectRemote checks if the git remote is GitHub or GitLab.
func detectRemote(dir string) string {
	// Read .git/config for remote URL
	data, err := os.ReadFile(filepath.Join(dir, ".git", "config"))
	if err != nil {
		return ""
	}
	content := string(data)
	if contains(content, "github.com") {
		return "github"
	}
	if contains(content, "gitlab.com") || contains(content, "gitlab") {
		return "gitlab"
	}
	return ""
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
