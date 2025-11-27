# CIDX Philosophy: Smart Event-Driven Automation

## The Problem We're Solving

### What's Wrong with "Always Full Loop"

Some CI/CD tools try to execute the entire DevOps loop (PLAN → CODE → BUILD → TEST → RELEASE → DEPLOY → OPERATE → MONITOR) on every single event. **This is fundamentally broken** because:

1. **PLAN**: Planning happens before coding, not during every commit
2. **MONITOR**: Can't monitor what's not yet deployed
3. **WASTE**: 90% of phases are irrelevant for most events
4. **SLOW**: Full pipeline takes 30+ minutes for a typo fix
5. **CONFUSION**: Developers don't know what will actually run

### The Real DevOps Loop

The DevOps loop is a **logical representation** of the software lifecycle, not a strict linear process. It helps us conceptualize the continuous flow:

```
PLAN → CODE → BUILD → TEST → RELEASE → DEPLOY → OPERATE → MONITOR → PLAN
```

**But**: Different events in this lifecycle require different actions!

## The CIDX Approach: Context-Aware Execution

### Event-Driven Principle

**Different events trigger different phases** based on their context and purpose:

```
Developer commits code
├─→ Quick validation (security + code)
└─→ Fast feedback (< 2 minutes)

Pull Request opened
├─→ Full validation (security + code + test)
└─→ Quality assured (5-10 minutes)

Merge to main
├─→ Validation + Build (security + code + test + build)
└─→ Production-ready artifacts (10-15 minutes)

Tag pushed (v1.0.0)
├─→ Complete pipeline (security + code + test + build + release + docker)
└─→ Deployed to production (15-30 minutes)
```

### Why This Works

✅ **Efficient**: Run only what's needed for each event
✅ **Fast**: Quick feedback for developers (iterative work)
✅ **Safe**: Full validation before production (releases)
✅ **Cost-effective**: No wasted CI minutes on irrelevant phases
✅ **Clear**: Developers know exactly what runs when

## Convention Over Configuration

### The GitLab/GitHub Problem

**GitLab CI / GitHub Actions approach**:

```yaml
# 200 lines of YAML defining when each job runs
test-unit:
  only: [merge_requests, branches]
  except: [tags]

build:
  only: [main]
  except: [merge_requests, tags]

deploy:
  only: [tags]
  except: [merge_requests, branches]
```

**Problems**:

- Verbose configuration
- Error-prone conditions
- Hard to understand
- Repeated logic

**CIDX approach**:

```toml
# 20 lines defining phases and pipelines
[pipelines.pr]
phases = ["security", "code", "test"]

[pipelines.main]
phases = ["security", "code", "test", "build"]

[pipelines.release]
phases = ["security", "code", "test", "build", "release", "docker"]
```

**CIDX knows**:

- PR → run `pr` pipeline
- Main branch → run `main` pipeline
- Tag → run `release` pipeline
- **No conditions needed** → convention!

## CIDX Scope: CI/CD Automation

### What CIDX IS

✅ **Convention-based CI/CD orchestrator**

- Event-driven pipelines
- Context-aware phase execution
- Platform-agnostic (GitHub, GitLab, Jenkins, CircleCI)
- Environment-aware (local vs CI)
- Safe by default (local safety modes)

### What CIDX is NOT

❌ **Not a project management tool** → Use Jira, Linear, GitHub Projects
❌ **Not a monitoring platform** → Use Prometheus, Grafana, DataDog
❌ **Not an infrastructure orchestrator** → Use Kubernetes, Terraform, Ansible
❌ **Not executing all phases on every event** → That's wasteful and wrong

**CIDX covers**: CODE → BUILD → RELEASE → DEPLOY

**Out of scope**: PLAN, OPERATE (continuous), MONITOR

## CIDX Phase Mapping to DevOps Loop

### How CIDX Phases Map to DevOps Stages

CIDX phases are organized to align with the DevOps loop stages:

| DevOps Stage | CIDX Phases         | Purpose               | Containers Example                       |
| ------------ | ------------------- | --------------------- | ---------------------------------------- |
| **CODE**     | `security`, `code`  | Validation & quality  | trivy, gitleaks, golangci-lint, prettier |
| **BUILD**    | `test`, `build`     | Create artifacts      | go-test, godog, go-build                 |
| **RELEASE**  | `release`, `docker` | **Publish artifacts** | gh-release, goreleaser, docker-buildx    |
| **DEPLOY**   | _(future)_          | Run in production     | kubectl, docker-compose, cloud-run       |

### The RELEASE Philosophy: Why Docker = RELEASE

**Key insight**: The `docker` phase is part of **RELEASE**, not DEPLOY.

#### RELEASE = Artifact Publication

The RELEASE stage **publishes artifacts to public stores**, making them available for download:

**`release` phase** → Publish binaries to GitHub Releases

```bash
gh release create v1.0.0 bin/cidx --generate-notes
# Artifact available at: https://github.com/org/repo/releases/v1.0.0
```

**`docker` phase** → Publish images to Container Registry

```bash
docker buildx build -t ghcr.io/org/repo:v1.0.0 --push .
# Artifact available at: ghcr.io/org/repo:v1.0.0
```

Both operations:

- ✅ Make artifacts **available** for download
- ✅ Publish to **artifact stores** (GitHub Releases, GHCR, Docker Hub)
- ❌ Do **NOT** run the application
- ❌ Do **NOT** deploy to infrastructure

**Analogy**: Publishing to GHCR is like publishing to NPM, Maven Central, or PyPI - you're making the artifact available, not deploying it.

#### DEPLOY = Infrastructure Execution

The DEPLOY stage **runs the application in production**:

```bash
# Deploy the published Docker image to Kubernetes
kubectl apply -f deployment.yaml
# → Pods running, app serving traffic

# Deploy to Cloud Run
gcloud run deploy --image ghcr.io/org/repo:v1.0.0
# → Service available at https://app.run.app

# Deploy with Docker Compose
docker-compose up -d
# → Containers running locally or on server
```

DEPLOY operations:

- ✅ **Run** the application
- ✅ Serve traffic to users
- ✅ Use infrastructure (k8s, cloud, servers)

### The Complete Flow

```
Tag v1.0.0 pushed:
  ├─ BUILD phase
  │  ├─ go build              → creates bin/cidx
  │  └─ docker build          → creates image locally
  │
  ├─ RELEASE phase
  │  ├─ gh release            → publishes to GitHub
  │  └─ docker push           → publishes to GHCR
  │
  └─ DEPLOY phase (future)
     ├─ kubectl apply         → deploys to k8s
     └─ users access app      → application running
```

### Why This Matters

This separation enables powerful workflows:

**Scenario 1: Test before deploy**

```bash
# 1. Build and publish (RELEASE)
cidx run release  # → binaries + images on GitHub/GHCR

# 2. Deploy to staging (DEPLOY)
kubectl apply -f staging.yaml

# 3. Test staging environment
./run-e2e-tests.sh

# 4. Deploy to production (DEPLOY)
kubectl apply -f production.yaml
```

**Scenario 2: Different deployment targets**

```bash
# Same release, multiple deployments
cidx run release  # → image at ghcr.io/org/app:v1.0.0

# Deploy to different environments using the SAME image
kubectl apply -f k8s/dev.yaml      # → dev cluster
kubectl apply -f k8s/staging.yaml  # → staging cluster
kubectl apply -f k8s/prod.yaml     # → production cluster
```

**Scenario 3: Separate teams**

```bash
# Dev team: build and release
cidx run release

# Ops team: deploy when ready
# (can be hours or days later)
kubectl apply -f deployment.yaml
```

## Behavior-Driven Development

### Why BDD with Gherkin

CIDX behavior is **specified in executable scenarios** using Gherkin:

```gherkin
Feature: Pull Request Validation
  Scenario: PR triggers only validation phases
    Given I create a pull request
    When I run "cidx run pr"
    Then it should execute the "security" phase
    And it should execute the "code" phase
    And it should execute the "test" phase
    But it should NOT execute the "build" phase
    And it should NOT execute the "release" phase
```

**Benefits**:

1. **Living documentation**: Scenarios ARE the specification
2. **Testable**: Scenarios are executed as tests with godog
3. **Clear scope**: If it's not in a scenario, we don't build it
4. **Communication**: Everyone (dev, ops, product) can read and understand
5. **Dogfooding**: CIDX tests CIDX using its own BDD scenarios

### BDD Prevents Scope Creep

Before adding any feature, we ask:

1. **Can we write a Gherkin scenario for it?**
2. **Does it fit the DevOps lifecycle context?**
3. **Is it event-driven or scheduled?**

If the answer is unclear → we don't build it (yet).

## Safe by Default

### Local Safety Modes

CIDX ensures **dangerous operations are safe to test locally**:

#### Docker: Build Without Push

```
Local environment:
  cidx run docker
  → Builds image ✓
  → Does NOT push ✗
  → Message: "Local safety: no-push"

CI environment:
  cidx run docker
  → Builds image ✓
  → Pushes to registry ✓
```

#### Release: Draft Only

```
Local environment:
  cidx run release
  → Creates draft release ✓
  → Does NOT publish ✗
  → Message: "Local safety: draft"

CI environment:
  cidx run release
  → Creates public release ✓
  → Publishes to GitHub ✓
```

### Environment Detection

CIDX automatically detects where it runs:

```
Local machine:
  → Safe modes enabled
  → Test everything without risk

GitHub Actions / GitLab CI / Jenkins:
  → Production modes enabled
  → Full automation
```

**No flags needed** → convention!

## Pipeline Design Patterns

### PR Pipeline: Validation Only

**Purpose**: Ensure code quality before merge
**Phases**: security → code → test
**Duration**: ~5-10 minutes
**Output**: Pass/Fail feedback

```toml
[pipelines.pr]
phases = ["security", "code", "test"]
description = "Pull request validation (no build artifacts)"
```

### Main Pipeline: Build Artifacts

**Purpose**: Keep main branch production-ready
**Phases**: security → code → test → build
**Duration**: ~10-15 minutes
**Output**: Validated artifacts ready for release

```toml
[pipelines.main]
phases = ["security", "code", "test", "build"]
description = "Main branch pipeline (builds artifacts)"
```

### Release Pipeline: Full Deployment

**Purpose**: Deploy to production
**Phases**: security → code → test → build → release → docker
**Duration**: ~15-30 minutes
**Output**: GitHub release + Docker images published

```toml
[pipelines.release]
phases = ["security", "code", "test", "build", "release", "docker"]
description = "Complete release pipeline (tags only)"
```

## Industry Alignment

### CIDX Follows Best Practices

Every major CI/CD platform uses event-driven conditional execution:

- **GitLab CI**: `only:`, `except:`, `rules:`
- **GitHub Actions**: `on: pull_request`, `on: push`, `if: github.ref == 'refs/tags/v*'`
- **CircleCI**: Filters, conditional workflows
- **Jenkins**: When conditions, branch selectors

**CIDX improvement**: You don't write these conditions → convention handles it!

## Future: Scheduled Operations (Phase 2)

### Vision: Multiple Loops

While CIDX focuses on **event-driven CI/CD** today, future versions may support **scheduled operations**:

```toml
# EVENT-DRIVEN (Core - Today)
[pipelines.release]
trigger = "tag"
phases = ["security", "code", "test", "build", "release", "docker"]

# SCHEDULED (Extension - Future)
[pipelines.infra-check]
trigger = "schedule"
schedule = "0 */6 * * *"  # Every 6 hours
phases = ["operate"]

[operate]
containers = ["terraform-validate", "drift-detection", "compliance-check"]
```

**Important**: Scheduled tasks will be:

- **Separate from CI/CD loop** (different trigger type)
- **Opt-in and experimental** (phase 2)
- **Complementary, not core** (CI/CD remains primary focus)

## Key Principles Summary

1. **Convention over Configuration**: CIDX knows what to run based on context
2. **Event-Driven is King**: Different events → different phases
3. **Safe by Default**: Dangerous operations protected locally
4. **BDD Specification**: Behavior defined in executable Gherkin scenarios
5. **Scope Discipline**: CI/CD automation only, not full DevOps platform
6. **Platform Agnostic**: Same config works on GitHub, GitLab, Jenkins, local
7. **Developer Experience**: Fast feedback, clear errors, testable locally

## Learn More

- [DevOps Integration](./devops.md) - How CIDX fits in DevOps lifecycle
- [Environment Security](./security.md) - Local safety modes explained
- [BDD Scenarios](../../features/) - Living documentation of CIDX behavior
