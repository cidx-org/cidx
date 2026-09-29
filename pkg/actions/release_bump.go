package actions

import (
	"fmt"
	"maps"
	"strings"

	"github.com/cidx-org/cidx/v3/pkg/semver"
)

// BumpAuthorEnv returns the action's environment with the git identity the
// bump commit is authored under (#484).
//
// The container runs as the invoking uid with no git config of its own, so
// `cz bump` failed on "Author identity unknown" unless something outside the
// config supplied one. The host's identity is passed as the variables git
// consults before any config file; a key the action already declares wins, so
// a release bot identity stays a one-line override. With neither, the release
// is refused here, before the container runs, rather than inside it.
func BumpAuthorEnv(declared map[string]string, name, email string) (map[string]string, error) {
	env := maps.Clone(declared)
	if env == nil {
		env = map[string]string{}
	}
	for key, value := range map[string]string{
		"GIT_AUTHOR_NAME":     name,
		"GIT_COMMITTER_NAME":  name,
		"GIT_AUTHOR_EMAIL":    email,
		"GIT_COMMITTER_EMAIL": email,
	} {
		if _, set := env[key]; set {
			continue
		}
		if value == "" {
			return nil, fmt.Errorf("no git identity to author the release bump with: %s is unset\n"+
				"   Set one with: git config user.name \"Your Name\" && git config user.email you@example.com\n"+
				"   Or declare it in [actions.<name>.env]", key)
		}
		env[key] = value
	}
	return env, nil
}

// hostGitIdentity reads the identity git would use on the host for workDir —
// repository config first, then global — the same answer `git commit` gets.
func hostGitIdentity(workDir string) (name, email string) {
	read := func(key string) string {
		out, err := runGit(workDir, "config", key)
		if err != nil {
			return ""
		}
		return strings.TrimSpace(string(out))
	}
	return read("user.name"), read("user.email")
}

// BumpedVersion returns the version the bump container tagged its commit with.
//
// It used to be read back from a VERSION file, which only the projects whose
// commitizen `version_files` list one ever write — and it was read after the
// commit and tag existed, so a project without the file was left with both on
// its base branch (#484). `cz bump` always tags the commit it creates; that
// tag is the one source every configuration shares.
func BumpedVersion(workDir, baseSHA string) (string, error) {
	head, err := runGit(workDir, "rev-parse", "HEAD")
	if err != nil {
		return "", fmt.Errorf("failed to read HEAD: %w\n%s", err, head)
	}
	if strings.TrimSpace(string(head)) == baseSHA {
		return "", fmt.Errorf("the bump created no commit: HEAD is still %s", shortSHA(baseSHA))
	}

	out, err := runGit(workDir, "tag", "--points-at", "HEAD")
	if err != nil {
		return "", fmt.Errorf("failed to list the tags on the bump commit: %w\n%s", err, out)
	}
	var versions []string
	for _, tag := range strings.Fields(string(out)) {
		if v := semver.Trim(tag); semver.IsValid(v) {
			versions = append(versions, v)
		}
	}
	switch len(versions) {
	case 1:
		return versions[0], nil
	case 0:
		return "", fmt.Errorf("the bump commit carries no version tag")
	default:
		return "", fmt.Errorf("the bump commit carries several version tags: %s", strings.Join(versions, ", "))
	}
}

// UndoBump returns the branch to baseSHA after a bump that did not go through.
//
// The release refuses to start on a dirty tree, so baseSHA with a clean tree
// is exactly the state before the container ran: resetting to it undoes the
// bump commit and any file a failed `git commit` left modified. The tags on
// the bump commit go with it — only when HEAD moved, so a tag on the starting
// commit (the previous release) is never touched. Untracked files are left
// alone: nothing proves the container created them.
func UndoBump(workDir, baseSHA string) error {
	head, err := runGit(workDir, "rev-parse", "HEAD")
	if err != nil {
		return fmt.Errorf("failed to read HEAD: %w\n%s", err, head)
	}
	if strings.TrimSpace(string(head)) != baseSHA {
		out, err := runGit(workDir, "tag", "--points-at", "HEAD")
		if err != nil {
			return fmt.Errorf("failed to list the tags on the bump commit: %w\n%s", err, out)
		}
		for _, tag := range strings.Fields(string(out)) {
			if out, err := runGit(workDir, "tag", "-d", tag); err != nil {
				return fmt.Errorf("failed to delete tag %s: %w\n%s", tag, err, out)
			}
		}
	}
	if out, err := runGit(workDir, "reset", "--hard", baseSHA); err != nil {
		return fmt.Errorf("failed to restore %s: %w\n%s", shortSHA(baseSHA), err, out)
	}
	return nil
}

// VerifyTaggable refuses to tag unless HEAD is the merged release commit (#510).
//
// releaseViaPR tags HEAD after the squash-merge, on the assumption that
// `pr merge` left the checkout on the base branch with the merge pulled. When
// the pull failed on a network blip that was false: HEAD was still the commit
// the release started from, and the tag went onto the notes commit, one before
// the bump. Only the tag push failing for the same reason kept it from shipping.
//
// The check needs no network. After a pulled merge, HEAD is on the base
// branch, has moved off baseSHA, and descends from it — a checkout left on the
// release branch also descends, so the branch is checked too (that tag would sit
// on the pre-squash commit, the mistake #184 fixed).
func VerifyTaggable(workDir, baseBranch, baseSHA string) error {
	branch, err := runGit(workDir, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return fmt.Errorf("failed to read the current branch: %w\n%s", err, branch)
	}
	if got := strings.TrimSpace(string(branch)); got != baseBranch {
		return fmt.Errorf("HEAD is on '%s', not '%s': the merged release commit is on '%s'.\n"+
			"   Nothing was tagged. To finish: git checkout %s && git pull, then cidx release tag prepare && cidx release tag create",
			got, baseBranch, baseBranch, baseBranch)
	}

	head, err := runGit(workDir, "rev-parse", "HEAD")
	if err != nil {
		return fmt.Errorf("failed to read HEAD: %w\n%s", err, head)
	}
	if strings.TrimSpace(string(head)) == baseSHA {
		return fmt.Errorf("the release PR was merged, but '%s' has not pulled it: HEAD is still %s, where the release started.\n"+
			"   Nothing was tagged. To finish: git pull, then cidx release tag prepare && cidx release tag create",
			baseBranch, shortSHA(baseSHA))
	}
	if out, err := runGit(workDir, "merge-base", "--is-ancestor", baseSHA, "HEAD"); err != nil {
		return fmt.Errorf("HEAD does not descend from %s, where the release started: it is not the merged release commit.\n%s\n"+
			"   Nothing was tagged. To finish: git checkout %s && git pull, then cidx release tag prepare && cidx release tag create",
			shortSHA(baseSHA), strings.TrimSpace(string(out)), baseBranch)
	}
	return nil
}
