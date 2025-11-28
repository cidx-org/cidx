# Contributing to CIDX

Thank you for your interest in contributing to CIDX! 🎉

## Development Workflow

CIDX uses **trunk-based development** with **manual releases**.

### 🔄 Daily Development (Pull Requests)

```bash
# 1. Create feature branch with draft PR
cidx action pr create "feat: add new feature"

# 2. Implement and commit using conventional commits
git commit -m "feat: implement feature"
git push

# 3. Mark PR ready for review when done
cidx action pr ready

# 4. Merge to main (no tag created)
cidx action pr merge
```

### 🚀 Creating Releases

After merging 3-5 PRs to main, create a release:

```bash
cidx action release create
```

This automatically:

- Analyzes commits since last tag
- Bumps version (PATCH/MINOR/MAJOR based on conventional commits)
- Creates version bump commit
- Creates and pushes git tag (e.g., v1.1.1)
- Triggers GitHub Release workflow
- Publishes release with binary + Docker image

**Key principle:** Tags = Releases (1:1). PRs don't create tags.

## 📝 Commit Convention

We use [Conventional Commits](https://www.conventionalcommits.org/):

```text
<type>: <description>

[optional body]
```

**Types:**

- `feat:` - New feature (MINOR version bump)
- `fix:` - Bug fix (PATCH version bump)
- `docs:` - Documentation only
- `style:` - Code style (formatting, no logic change)
- `refactor:` - Code refactoring
- `perf:` - Performance improvement
- `test:` - Adding/updating tests
- `chore:` - Maintenance tasks
- `ci:` - CI/CD changes

**Breaking changes:** Add `!` after type or `BREAKING CHANGE:` in footer (MAJOR bump)

```bash
feat!: redesign API interface

BREAKING CHANGE: API endpoints now require authentication
```

## 🏗️ Development Setup

### Prerequisites

- Go 1.21+
- Docker (for running containers)
- Git

### Build

```bash
go build -o bin/cidx ./cmd/cidx
```

### Run Tests

```bash
go test ./...
```

### Run Full CI Locally

```bash
./bin/cidx run ci
```

## 📖 Documentation

- **[Development Workflow Guide](docs/guides/development-workflow.md)** - Detailed workflow documentation
- **[CLAUDE.md](CLAUDE.md)** - Architecture and development guide
- **[cidx.toml](cidx.toml)** - Self-documenting configuration with workflow

## 🎯 Project Structure

```text
cidx/
├── cmd/cidx/          # CLI commands
├── pkg/
│   ├── presets/       # Built-in container presets
│   ├── config/        # Configuration parsing
│   ├── executor/      # Docker execution
│   └── pipeline/      # Pipeline orchestration
├── docs/              # Documentation
└── examples/          # Example configurations
```

## 🧪 Testing

Before submitting a PR:

1. Run tests: `go test ./...`
2. Run linting: `./bin/cidx run code`
3. Run full CI: `./bin/cidx run ci`

## 💡 Adding New Containers

See [Creating Presets Guide](docs/guides/creating-presets.md) for adding new container presets.

## ❓ Questions?

- Check [documentation](docs/)
- Read [existing issues](https://github.com/cidx-org/cidx/issues)
- Open a [new discussion](https://github.com/cidx-org/cidx/discussions)

## 📜 License

By contributing, you agree that your contributions will be licensed under the same license as the project.
