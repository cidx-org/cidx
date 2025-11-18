# CIDX Development Roadmap

## Phase 1: MVP ✅ (COMPLETED)

- [x] Parse TOML/YAML configuration
- [x] Preset registry with 6 core tools (trivy, gitleaks, megalinter, commitlint, ansible-lint, molecule)
- [x] Docker executor with SDK integration
- [x] CLI commands (run, list, info, validate, init)
- [x] Config merge (presets + user overrides)
- [x] Workspace variable expansion
- [x] Pipeline orchestration by phase
- [x] Dry-run mode

## Phase 2: Expansion (Next 1-2 weeks)

### Core Features
- [ ] Add 15+ more presets for popular DevSecOps tools:
  - Security: semgrep, snyk, bandit, safety, gosec, hadolint
  - Code: eslint, ruff, black, prettier, yamllint, shellcheck
  - Test: pytest, jest, go test, newman
  - Build: kaniko, buildah
- [ ] Auto-detection of project type
  - Detect language (Go, Python, Node.js, etc.)
  - Detect framework (Ansible, Django, React, etc.)
  - Suggest relevant tools
- [ ] Enhance `cidx init` with auto-detection
- [ ] Add more validation rules
- [ ] Improve error messages and user feedback

### Testing
- [ ] Unit tests for all packages
- [ ] Integration tests with real Docker images
- [ ] Test coverage > 80%
- [ ] CI/CD pipeline (GitHub Actions)

### Documentation
- [ ] API documentation (godoc)
- [ ] More examples (monorepo, multi-language)
- [ ] Video tutorial
- [ ] Blog post about CIDX philosophy

## Phase 3: Advanced Features (2-4 weeks)

### User Experience
- [ ] User preset extensions (`~/.config/cidx/presets.toml`)
- [ ] Interactive mode for tool selection
- [ ] Progress bars for long-running tools
- [ ] Colored output for better readability
- [ ] JSON output mode for CI integration

### Performance
- [ ] Parallel tool execution within phases
- [ ] Caching of Docker images
- [ ] Incremental scanning (only changed files)
- [ ] Resource limits (CPU, memory)

### Enterprise Features
- [ ] Registry override (private Docker registries)
- [ ] Image signature verification
- [ ] SBOM generation and tracking
- [ ] Audit logging
- [ ] Vault integration for secrets
- [ ] Policy enforcement (mandatory tools)

### Integration
- [ ] GitLab CI template
- [ ] GitHub Actions workflow
- [ ] Jenkins plugin
- [ ] Pre-commit hooks
- [ ] Git hooks integration

## Phase 4: Ecosystem (Long-term)

### Tools
- [ ] Web UI for configuration
  - Visual pipeline designer
  - Tool marketplace
  - Results dashboard
- [ ] VSCode extension
- [ ] IDE integrations (JetBrains, Vim)

### Platform
- [ ] Cloud-hosted service (cidx.io)
- [ ] Shared preset marketplace
- [ ] Result aggregation and trends
- [ ] Team collaboration features

### Advanced Capabilities
- [ ] Custom plugin system
- [ ] Support for non-Docker executors (Podman, nerdctl)
- [ ] Remote execution (SSH, Kubernetes)
- [ ] Distributed execution
- [ ] AI-powered tool recommendations

## Bugs / Issues

- [ ] None currently tracked

## Ideas / Future Considerations

- Support for matrix builds (multiple versions)
- Conditional execution (skip tools based on conditions)
- Secrets management integration
- Report generation and formatting
- Integration with issue trackers (Jira, GitHub Issues)
- Notification systems (Slack, email)
- Cost tracking for cloud executions
- Performance profiling of tools
