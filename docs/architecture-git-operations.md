# Why Keep Git Commands for Git Operations?

CIDX uses a hybrid approach for version control operations:

- **Git operations**: Native `git` binary via `os/exec`
- **Provider APIs**: Go libraries (`google/go-github`, `gitlab`, etc.)

This document explains why.

## The Problem

During development of the PR workflow, we considered using the `go-git` library (a pure Go implementation of Git) instead of calling the `git` binary. This would make CIDX 100% Go with no external dependencies.

However, **go-git does not support Git hooks**.

## Why Git Hooks Matter

Git hooks are essential for CIDX workflows:

- **pre-commit**: Runs commitizen for semantic versioning
- **pre-push**: Validates code before pushing
- **prepare-commit-msg**: Formats commit messages
- **post-commit**: Triggers automation

Without hooks, critical workflow automation breaks.

## The go-git Limitation

The `go-git` library is a pure Go implementation of Git, but it has a known limitation:

- **No hook execution**: Commit, push, and other operations skip hooks entirely
- **No hook detection**: Cannot detect or run `.git/hooks/` scripts
- **Won't be fixed**: This is documented in [issue #1429](https://github.com/go-git/go-git/issues/1429) and [issue #913](https://github.com/src-d/go-git/issues/913)

From the go-git maintainers:

> "Git is a humongous project with years of development by thousands of contributors. Implementing full hook support is outside the scope of this library."

## Manual Hook Implementation?

We considered implementing hook execution ourselves:

```go
func (r *Repository) Commit(msg string) error {
    // 1. Detect .git/hooks/pre-commit
    // 2. Execute with proper environment variables
    // 3. Handle exit codes and output
    // 4. Do the actual commit with go-git
    // 5. Execute post-commit hooks
}
```

**Why we rejected this:**

- **300-500 lines of code**: Hook detection, execution, environment setup
- **Complex edge cases**: Hook chaining, error handling, environment variables
- **Maintenance burden**: Keep parity with Git's hook behavior
- **High risk**: Easy to introduce bugs or diverge from Git behavior
- **Not CIDX's mission**: We "macroize" workflows, not reimplement Git

## The Solution: Hybrid Approach

Use the right tool for each job:

### Git Operations → Native `git` Binary

```go
func (r *Repository) Commit(message string) error {
    cmd := exec.Command("git", "commit", "-m", message)
    return cmd.Run() // Hooks execute automatically
}
```

**Benefits:**

- ✅ Full hook support out of the box
- ✅ Identical behavior to user's Git workflow
- ✅ Zero custom implementation
- ✅ Battle-tested and reliable

**Trade-off:**

- ⚠️ Requires `git` binary installed (acceptable - users already have it)

### Provider APIs → Go Libraries

```go
func (c *Client) CreatePullRequest(...) error {
    pr := &github.NewPullRequest{...}
    _, _, err := c.client.PullRequests.Create(ctx, ...)
    return err
}
```

**Benefits:**

- ✅ Type-safe API interactions
- ✅ Better error handling and retry logic
- ✅ No CLI dependencies (`gh`, `glab`)
- ✅ Programmatic control
- ✅ Easy to test and mock

**Why this is safe:**

Provider APIs are straightforward HTTP requests with no complex state or hooks. The risk is low, and Go libraries provide better ergonomics than CLI tools.

## Implementation Summary

### Git Operations (using `git` binary)

Located in `pkg/vcs/repository.go`:

- `Commit()` - Triggers: pre-commit, prepare-commit-msg, commit-msg, post-commit
- `Push()` - Triggers: pre-push, post-push
- `Checkout()` - Triggers: post-checkout
- `Pull()` - Triggers: post-merge

### Provider Operations (using Go libraries)

Located in `pkg/remote/github/`:

- Creating/updating pull requests
- Watching workflow runs
- Triggering releases
- Managing issues
- Repository metadata

### Special Case: Draft PR Conversion

During PR workflow implementation, we discovered that GitHub's REST API has a critical limitation:

**Problem**: The REST API cannot convert draft pull requests to ready status. Attempting to use `PullRequests.Edit` with `Draft: false` results in a 405 error.

**Solution**: Use `gh` CLI (which uses GraphQL API internally) for this specific operation:

```go
func (c *Client) MarkPullRequestReady(ctx context.Context, prNumber int) error {
    // Use gh CLI to mark PR as ready (uses GraphQL API internally)
    cmd := exec.Command("gh", "pr", "ready", strconv.Itoa(prNumber),
                        "--repo", fmt.Sprintf("%s/%s", c.owner, c.repo))
    output, err := cmd.CombinedOutput()
    if err != nil {
        return fmt.Errorf("failed to mark PR as ready: %w\n%s", err, output)
    }
    return nil
}
```

This follows our hybrid approach: use native CLI tools when the REST API has limitations, while continuing to use Go libraries for other operations.

## When to Revisit This Decision

This approach should be reconsidered if:

1. **go-git adds hook support** - Monitor [go-git repository](https://github.com/go-git/go-git) for updates
2. **CIDX no longer needs hooks** - If workflow automation changes
3. **Git becomes unavailable** - If targeting environments without Git (unlikely)

## References

- [go-git hooks issue #1429](https://github.com/go-git/go-git/issues/1429)
- [Original hooks issue #913](https://github.com/src-d/go-git/issues/913)
- [go-git COMPATIBILITY.md](https://github.com/go-git/go-git/blob/master/COMPATIBILITY.md)
- [google/go-github](https://github.com/google/go-github)
