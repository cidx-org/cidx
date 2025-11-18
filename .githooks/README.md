# Git Hooks for CIDX

This directory contains Git hooks that run CIDX checks automatically.

## Pre-commit Hook

The `pre-commit` hook runs security and code quality checks before allowing commits:

- 🛡️ **Security**: `gitleaks` (secrets) + `trivy` (vulnerabilities)
- 🎨 **Code Quality**: `prettier` (formatting) + `golangci-lint` (linting)

### Setup (One-time)

Enable the hooks for this repository:

```bash
git config core.hooksPath .githooks
```

### Usage

Once enabled, the hooks run automatically on every commit:

```bash
git commit -m "feat: add new feature"
# 🔍 Running CIDX pre-commit checks...
# 🛡️  Security checks...
# ✅ trivy completed
# ✅ gitleaks completed
# 🎨 Code quality checks...
# ✅ prettier completed
# ✅ golangci-lint completed
# ✅ All pre-commit checks passed!
```

### Bypass (when needed)

If you need to commit despite failing checks (not recommended):

```bash
git commit --no-verify -m "WIP: work in progress"
```

### Disable

To disable the hooks:

```bash
git config --unset core.hooksPath
```

## Benefits

- ✅ **Catch issues early**: Find secrets and bugs before they reach the repo
- ✅ **Consistent quality**: All commits meet the same standards
- ✅ **Fast feedback**: 2-3 seconds, faster than waiting for CI
- ✅ **Shift left**: Security and quality checks at the earliest stage
