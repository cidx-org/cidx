# CIDX & DevOps Integration

This document explains how CIDX integrates into standard DevOps workflows and product lifecycle.

## Philosophy: DevOps Loop & CIDX Phases

The **DevOps loop** is a **logical representation** of the software lifecycle, not a strict linear process. It helps conceptualize the continuous flow from planning to monitoring and back to planning.

**CIDX scope**: CIDX is a **CI/CD automation platform** that covers the technical pipeline from code to deployment. It does **NOT** cover:

- **PLAN** → Product management tools (Jira, Linear, roadmaps)
- **OPERATE** → Production operations (Kubernetes, orchestration)
- **MONITOR** → Observability (Prometheus, Grafana, logs)

**CIDX covers**: CODE → BUILD → RELEASE → DEPLOY

At each covered stage, **CIDX phases** provide automated quality gates and validations that can be executed anywhere (local, CI, any platform).

## 1. CIDX in the DevOps Loop

```mermaid
flowchart TB
    subgraph DevOps["DevOps Lifecycle - Full Loop"]
        PLAN["📋 Plan<br/>(GitHub, Gitlab, Jira)"]
        CODE[💻 Code]
        BUILD[🔨 Build]
        TEST[🧪 Test]
        RELEASE[📦 Release]
        DEPLOY[🚀 Deploy]
        OPERATE["⚙️ Operate<br/>(K8s, infra)"]
        MONITOR["📊 Monitor<br/>(Prometheus, Grafana)"]

        PLAN --> CODE
        CODE --> BUILD
        BUILD --> TEST
        TEST --> RELEASE
        RELEASE --> DEPLOY
        DEPLOY --> OPERATE
        OPERATE --> MONITOR
        MONITOR --> PLAN
    end

    subgraph CIDX_SCOPE["CIDX Automation Scope"]
        subgraph SECURITY["🛡️ security phase"]
            SEC1[trivy]
            SEC2[gitleaks]
        end

        subgraph QUALITY["🎨 code phase"]
            QUAL1[prettier]
            QUAL2[golangci-lint]
        end

        subgraph TESTING["✅ test phase"]
            TEST1[go-test]
            TEST2[molecule]
        end

        subgraph BUILDING["🔧 build phase"]
            BUILD1[go-build]
            BUILD2[compile]
        end

        subgraph GH_RELEASE["📢 release phase"]
            REL1[gh-release]
            REL2[goreleaser]
        end

        subgraph DOCKER["🐳 docker phase"]
            DOCK1[kaniko]
            DOCK2[buildx]
        end
    end

    CODE -.->|cidx run security| SECURITY
    CODE -.->|cidx run code| QUALITY
    BUILD -.->|cidx run test| TESTING
    BUILD -.->|cidx run build| BUILDING
    RELEASE -.->|cidx run release| GH_RELEASE
    DEPLOY -.->|cidx run docker| DOCKER

    style DevOps fill:#1a1a1a,stroke:#666,stroke-width:2px,color:#888
    style PLAN fill:#2d2d2d,stroke:#666,stroke-width:1px,color:#666
    style CODE fill:#2d2d2d,stroke:#fff,stroke-width:2px,color:#fff
    style BUILD fill:#2d2d2d,stroke:#fff,stroke-width:2px,color:#fff
    style TEST fill:#2d2d2d,stroke:#fff,stroke-width:2px,color:#fff
    style RELEASE fill:#2d2d2d,stroke:#fff,stroke-width:2px,color:#fff
    style DEPLOY fill:#2d2d2d,stroke:#fff,stroke-width:2px,color:#fff
    style OPERATE fill:#2d2d2d,stroke:#666,stroke-width:1px,color:#666
    style MONITOR fill:#2d2d2d,stroke:#666,stroke-width:1px,color:#666

    style CIDX_SCOPE fill:#0d0d0d,stroke:#00ff00,stroke-width:3px,color:#fff
    style SECURITY fill:#1a1a1a,stroke:#fff,stroke-width:2px,color:#fff
    style QUALITY fill:#1a1a1a,stroke:#fff,stroke-width:2px,color:#fff
    style TESTING fill:#1a1a1a,stroke:#fff,stroke-width:2px,color:#fff
    style BUILDING fill:#1a1a1a,stroke:#fff,stroke-width:2px,color:#fff
    style GH_RELEASE fill:#1a1a1a,stroke:#fff,stroke-width:2px,color:#fff
    style DOCKER fill:#1a1a1a,stroke:#fff,stroke-width:2px,color:#fff
```

**Phase Mapping Logic**:

**CIDX covers (CI/CD automation)**:

- **CODE** → Security scanning + Code quality (shift-left approach)
- **BUILD** → Testing + Compilation (validate before release)
- **RELEASE** → Create GitHub release with artifacts (tag, notes, binaries)
- **DEPLOY** → Build and push Docker images (deployment-ready containers)

**Out of CIDX scope**:

- **PLAN** → Product management, roadmaps, user stories (Jira, Linear, etc.)
- **OPERATE** → Production operations, infrastructure management (Kubernetes, Terraform, etc.)
- **MONITOR** → Observability, metrics, logs, alerting (Prometheus, Grafana, DataDog, etc.)

For CIDX itself: once the GitHub release is created, the Docker phase builds and pushes the `cidx` container image to registries (GHCR, Docker Hub), making it deployable anywhere.

## 2. Git Workflow & CIDX Pipelines

```mermaid
flowchart LR
    LOCAL[👨‍💻 Local Dev]
    COMMIT[📝 Commit]
    PR[🔀 Pull Request]
    MERGE[✅ Merge to Main]
    TAG[🏷️ Tag Release]

    LOCAL_PIPE["pre-push<br/>security + code + test"]
    PR_PIPE["pr<br/>security + code + test"]
    MAIN_PIPE["main<br/>security + code + test + build"]
    RELEASE_PIPE["release<br/>security + code + test<br/>+ build + docker + release"]

    LOCAL -->|git push| COMMIT
    COMMIT --> PR
    PR -.-> PR_PIPE
    PR -->|approved| MERGE
    MERGE -.-> MAIN_PIPE
    MERGE --> TAG
    TAG -.-> RELEASE_PIPE
    LOCAL -.->|cidx run pre-push| LOCAL_PIPE

    style LOCAL_PIPE fill:#2d2d2d,stroke:#fff,stroke-width:2px,color:#fff
    style PR_PIPE fill:#3d3d3d,stroke:#fff,stroke-width:2px,color:#fff
    style MAIN_PIPE fill:#4d4d4d,stroke:#fff,stroke-width:2px,color:#fff
    style RELEASE_PIPE fill:#1a1a1a,stroke:#fff,stroke-width:3px,color:#fff
```

## 3. Environment-Based Execution

```mermaid
flowchart TB
    CMD[cidx run release]
    DETECT{"Environment<br/>Detection"}

    CMD --> DETECT

    DETECT -->|Local| LOCAL_MODE[🏠 Local Environment]
    DETECT -->|CI| CI_MODE[☁️ CI Environment]

    LOCAL_MODE --> SEC_LOCAL["🛡️ Security: Full scan"]
    LOCAL_MODE --> CODE_LOCAL["🎨 Code: Full check"]
    LOCAL_MODE --> TEST_LOCAL["✅ Test: Full suite"]
    LOCAL_MODE --> BUILD_LOCAL["🔧 Build: Full build"]
    LOCAL_MODE --> DOCKER_LOCAL["🐳 Docker: Build<br/>⚠️ NO PUSH"]
    LOCAL_MODE --> RELEASE_LOCAL["📢 Release: Draft<br/>⚠️ NOT PUBLISHED"]

    CI_MODE --> SEC_CI["🛡️ Security: Full scan"]
    CI_MODE --> CODE_CI["🎨 Code: Full check"]
    CI_MODE --> TEST_CI["✅ Test: Full suite"]
    CI_MODE --> BUILD_CI["🔧 Build: Full build"]
    CI_MODE --> DOCKER_CI["🐳 Docker: Build<br/>✅ PUSH TO REGISTRY"]
    CI_MODE --> RELEASE_CI["📢 Release: Publish<br/>✅ PUBLIC RELEASE"]

    DOCKER_LOCAL -.->|Safe| DRAFT1["✓ Testable locally"]
    RELEASE_LOCAL -.->|Safe| DRAFT2["✓ Testable locally"]
    DOCKER_CI -.->|Production| PROD1["✓ Published to GHCR"]
    RELEASE_CI -.->|Production| PROD2["✓ Published to GitHub"]

    style LOCAL_MODE fill:#2d2d2d,stroke:#fff,stroke-width:2px,color:#fff
    style CI_MODE fill:#1a1a1a,stroke:#fff,stroke-width:2px,color:#fff
    style DOCKER_LOCAL fill:#3d3d3d,stroke:#ffa500,stroke-width:2px,color:#fff
    style RELEASE_LOCAL fill:#3d3d3d,stroke:#ffa500,stroke-width:2px,color:#fff
    style DOCKER_CI fill:#1a1a1a,stroke:#00ff00,stroke-width:2px,color:#fff
    style RELEASE_CI fill:#1a1a1a,stroke:#00ff00,stroke-width:2px,color:#fff
```

## 4. Security Modes Detail

```mermaid
flowchart LR
    PRESET[Preset Definition]
    RC[require_ci: false]
    LB[local_behavior]

    PRESET --> RC
    PRESET --> LB

    LB --> DRAFT["draft<br/>GitHub releases<br/>as drafts"]
    LB --> NOPUSH["no-push<br/>Docker build<br/>without push"]
    LB --> DRYRUN["dry-run<br/>Simulation<br/>only"]
    LB --> DISABLED["disabled<br/>Refuse<br/>execution"]
    LB --> PROD["production<br/>Full<br/>execution"]

    DRAFT -.-> DRAFT_EX["✓ gh-release<br/>✓ goreleaser"]
    NOPUSH -.-> NOPUSH_EX["✓ docker-buildx<br/>✓ kaniko"]
    DRYRUN -.-> DRYRUN_EX["✓ Any preset<br/>simulation mode"]
    DISABLED -.-> DISABLED_EX["✓ Highly sensitive<br/>operations"]
    PROD -.-> PROD_EX["⚠️ Use with<br/>caution!"]

    style DRAFT fill:#2d4d2d,stroke:#00ff00,stroke-width:2px,color:#fff
    style NOPUSH fill:#2d4d2d,stroke:#00ff00,stroke-width:2px,color:#fff
    style DRYRUN fill:#4d4d2d,stroke:#ffa500,stroke-width:2px,color:#fff
    style DISABLED fill:#4d2d2d,stroke:#ff0000,stroke-width:2px,color:#fff
    style PROD fill:#4d2d2d,stroke:#ff0000,stroke-width:3px,color:#fff
```

## 5. Complete Product Lifecycle

```mermaid
stateDiagram-v2
    [*] --> Development

    state Development {
        [*] --> LocalDev
        LocalDev --> PrePush: cidx run pre-push
        PrePush --> Push: ✓ Passed
    }

    Push --> PullRequest

    state PullRequest {
        [*] --> PRValidation
        PRValidation --> CIDXPRPipeline: cidx run pr
        CIDXPRPipeline --> Review: ✓ Security + Code + Test
        Review --> Approved
    }

    Approved --> MainBranch

    state MainBranch {
        [*] --> MainValidation
        MainValidation --> CIDXMainPipeline: cidx run main
        CIDXMainPipeline --> Artifacts: ✓ Build artifacts
    }

    Artifacts --> Tagging

    state Tagging {
        [*] --> CreateTag
        CreateTag --> CIDXReleasePipeline: cidx run release
        CIDXReleasePipeline --> DockerPush: ✓ Push to registry
        DockerPush --> GitHubRelease: ✓ Create release
    }

    GitHubRelease --> Production

    state Production {
        [*] --> Deploy
        Deploy --> Monitor
        Monitor --> Feedback
    }

    Feedback --> [*]
```

## 6. CI/CD Platform Integration

```mermaid
flowchart TB
    GH_PR[GitHub: Pull Request Event]
    GH_MAIN[GitHub: Push to Main Event]
    GH_TAG[GitHub: Tag Push Event]

    GL_MR[GitLab: Merge Request Event]
    GL_MAIN[GitLab: Push to Main Event]
    GL_TAG[GitLab: Tag Push Event]

    JE_PR[Jenkins: PR Build]
    JE_MAIN[Jenkins: Main Build]
    JE_TAG[Jenkins: Tag Build]

    GH_PR --> GH_PR_JOB[cidx run pr]
    GH_MAIN --> GH_MAIN_JOB[cidx run main]
    GH_TAG --> GH_TAG_JOB[cidx run release]

    GL_MR --> GL_MR_JOB[cidx run pr]
    GL_MAIN --> GL_MAIN_JOB[cidx run main]
    GL_TAG --> GL_TAG_JOB[cidx run release]

    JE_PR --> JE_PR_JOB[cidx run pr]
    JE_MAIN --> JE_MAIN_JOB[cidx run main]
    JE_TAG --> JE_TAG_JOB[cidx run release]

    GH_PR_JOB --> SAME["Same CIDX<br/>Different Platform"]
    GL_MR_JOB --> SAME
    JE_PR_JOB --> SAME

    style SAME fill:#1a1a1a,stroke:#fff,stroke-width:3px,color:#fff
```

## Key Principles

1. **Convention over Configuration**: CIDX knows what to do based on environment
2. **Safe by Default**: Sensitive operations protected in local environments
3. **Consistent Everywhere**: Same commands work on local, GitHub, GitLab, Jenkins
4. **Product Lifecycle Aware**: Different pipelines for different stages
5. **Testable Locally**: Full pipeline testable without publishing

## Benefits

- ✅ **Developers**: Test release process locally without risk
- ✅ **CI/CD**: Simplified configuration (just call CIDX)
- ✅ **Security**: Protected against accidental production publishes
- ✅ **Portability**: Switch CI platforms without changing CIDX config
- ✅ **Clarity**: Clear separation between development stages
