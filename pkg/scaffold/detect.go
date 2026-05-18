// Package scaffold detects project type and generates cidx.toml configuration.
package scaffold

import (
	"os"
	"path/filepath"
	"sort"
)

// Language represents a detected project language/ecosystem.
type Language struct {
	Name     string
	Marker   string // file (or subdir/file) that triggered detection
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

// languageMarker associates a filesystem marker with a Language template.
type languageMarker struct {
	file string
	lang Language
}

// languageMarkers is the canonical list of detection signals.
// Order matters for the per-directory "primary marker" choice (first match wins).
func languageMarkers() []languageMarker {
	return []languageMarker{
		{"go.mod", Language{
			Name:     "Go",
			Security: []string{"trivy", "gitleaks", "gosec"},
			Code:     []string{"golangci-lint", "gofmt", "prettier", "commitizen"},
			Test:     []string{"go-test"},
			Build:    []string{"go-build"},
		}},
		{"pyproject.toml", Language{
			Name:     "Python",
			Security: []string{"trivy", "gitleaks", "bandit"},
			Code:     []string{"ruff", "black", "mypy", "prettier", "commitizen"},
			Test:     []string{"pytest"},
			Build:    []string{"python-build"},
		}},
		{"requirements.txt", Language{
			Name:     "Python",
			Security: []string{"trivy", "gitleaks", "bandit", "pip-audit"},
			Code:     []string{"ruff", "black", "prettier", "commitizen"},
			Test:     []string{"pytest"},
			Build:    []string{},
		}},
		{"package.json", Language{
			Name:     "Node.js",
			Security: []string{"trivy", "gitleaks"},
			Code:     []string{"prettier", "commitizen"},
			Test:     []string{},
			Build:    []string{},
		}},
		{"Cargo.toml", Language{
			Name:     "Rust",
			Security: []string{"trivy", "gitleaks", "cargo-audit"},
			Code:     []string{"clippy", "rustfmt", "prettier", "commitizen"},
			Test:     []string{"cargo-test"},
			Build:    []string{"cargo-build"},
		}},
		{"molecule/", Language{
			Name:     "Ansible",
			Security: []string{"trivy", "gitleaks"},
			Code:     []string{"ansible-lint", "yamllint", "prettier", "commitizen"},
			Test:     []string{"molecule"},
			Build:    []string{},
		}},
	}
}

// skipDirs are directories never descended into when looking for monorepo
// sub-projects (depth-2 walk). Build artifacts, dependency caches, VCS, etc.
var skipDirs = map[string]struct{}{
	".git":          {},
	"node_modules":  {},
	"venv":          {},
	".venv":         {},
	"__pycache__":   {},
	"target":        {},
	"dist":          {},
	"build":         {},
	"bin":           {},
	".idea":         {},
	".vscode":       {},
	".tox":          {},
	".mypy_cache":   {},
	".pytest_cache": {},
}

// Detect analyzes dir (and its immediate subdirectories, for monorepo layouts)
// to determine the project type(s). Languages from subdirs are aggregated and
// deduplicated by name — a project with backend/requirements.txt and
// frontend/package.json produces one Python entry plus one Node.js entry.
func Detect(dir string) *Detection {
	d := &Detection{}

	// Check git
	if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
		d.HasGit = true
		d.Remote = detectRemote(dir)
	}

	// scanDir returns the languages detected at exactly one directory level.
	// Each scan reports at most one language per Name (multiple markers in the
	// same directory pick the first one in languageMarkers order). The caller
	// merges across directories.
	scanDir := func(scanRoot, displayPrefix string) []Language {
		var found []Language
		seenAtThisLevel := make(map[string]bool)
		for _, m := range languageMarkers() {
			path := filepath.Join(scanRoot, m.file)
			info, err := os.Stat(path)
			if err != nil {
				continue
			}
			// Directory-style markers (e.g. molecule/) must actually be a directory.
			if m.file[len(m.file)-1] == '/' && !info.IsDir() {
				continue
			}
			if seenAtThisLevel[m.lang.Name] {
				continue
			}
			seenAtThisLevel[m.lang.Name] = true

			lang := m.lang
			if displayPrefix == "" {
				lang.Marker = m.file
			} else {
				lang.Marker = displayPrefix + m.file
			}
			found = append(found, lang)
		}
		return found
	}

	// Pass 1 — repo root.
	rootLangs := scanDir(dir, "")

	// Pass 2 — immediate subdirectories (depth 2 total). Sort entries so
	// detection is deterministic across filesystems.
	var subLangs []Language
	if entries, err := os.ReadDir(dir); err == nil {
		names := make([]string, 0, len(entries))
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			name := entry.Name()
			if _, skip := skipDirs[name]; skip {
				continue
			}
			// Hidden directories beyond the explicit skip list (.cache,
			// .pytest_cache aliases, etc.) are uninteresting for monorepo layout.
			if len(name) > 0 && name[0] == '.' {
				continue
			}
			names = append(names, name)
		}
		sort.Strings(names)

		for _, name := range names {
			sub := scanDir(filepath.Join(dir, name), name+"/")
			subLangs = append(subLangs, sub...)
		}
	}

	// Merge: root wins for marker text when a language is present at both
	// levels (root is more representative). Subdir markers fill in languages
	// not seen at root.
	seen := make(map[string]bool)
	for _, l := range rootLangs {
		if !seen[l.Name] {
			seen[l.Name] = true
			d.Languages = append(d.Languages, l)
		}
	}
	for _, l := range subLangs {
		if !seen[l.Name] {
			seen[l.Name] = true
			d.Languages = append(d.Languages, l)
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
