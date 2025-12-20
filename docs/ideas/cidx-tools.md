# CIDX-Tools Project

## Concept

Create a separate repository `cidx-org/cidx-tools` containing Docker images with all security and code quality tools pre-installed.

## Why

- DHI (dhi.io) doesn't have security tools like gitleaks, shellcheck, semgrep
- Multiple image pulls slow down CI
- Version consistency across tools
- Simpler setup for users

## Images to Create

### cidx-tools:security

Base: `dhi.io/alpine-base:3.21` (hardened)

Tools:

- trivy (vulnerability scanner)
- gitleaks (secrets detection)
- semgrep (SAST)
- grype (vulnerability scanner)

### cidx-tools:code

Base: `dhi.io/alpine-base:3.21`

Tools:

- golangci-lint
- shellcheck
- hadolint
- commitizen

### cidx-tools:full

All tools combined

## Meta/Dogfooding

The cidx-tools repo will use cidx itself for its CI/CD!

Bootstrap:

1. v1: Build with standard images
2. v2+: Build with cidx-tools images (self-hosting)

## Repository Structure

```
cidx-org/cidx-tools/
├── images/
│   ├── security/Dockerfile
│   ├── code/Dockerfile
│   └── full/Dockerfile
├── cidx.toml
├── .github/workflows/build.yml
└── README.md
```

## Integration with CIDX

In cidx.toml:

```toml
[security]
image = "ghcr.io/cidx-org/cidx-tools:security"
containers = ["trivy", "gitleaks", "semgrep"]
```

## Status

- Idea noted: 2025-12-20
- Next: Create cidx-org/cidx-tools repository
