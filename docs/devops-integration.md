# CIDX & DevOps Integration

This document explains how CIDX integrates into standard DevOps workflows and product lifecycle.

## 1. CIDX in the DevOps Loop

```mermaid
graph TB
    subgraph "DevOps Loop"
        PLAN[📋 Plan]
        CODE[💻 Code]
        BUILD[🔨 Build]
        TEST[🧪 Test]
        RELEASE[📦 Release]
        DEPLOY[🚀 Deploy]
        OPERATE[⚙️ Operate]
        MONITOR[📊 Monitor]
    end

    subgraph "CIDX Phases"
        SECURITY[🛡️ Security<br/>trivy, gitleaks]
        QUALITY[🎨 Code Quality<br/>prettier, golangci-lint]
        TESTING[✅ Testing<br/>go-test, molecule]
        BUILDING[🔧 Build<br/>go-build, compile]
        DOCKER[🐳 Docker<br/>kaniko, buildx]
        GH_RELEASE[📢 Release<br/>gh-release, goreleaser]
    end

    PLAN --> CODE
    CODE --> BUILD
    CODE -.-> SECURITY
    CODE -.-> QUALITY
    BUILD --> TEST
    BUILD -.-> TESTING
    BUILD -.-> BUILDING
    TEST --> RELEASE
    RELEASE -.-> DOCKER
    RELEASE -.-> GH_RELEASE
    RELEASE --> DEPLOY
    DEPLOY --> OPERATE
    OPERATE --> MONITOR
    MONITOR --> PLAN

    style SECURITY fill:#ff6b6b
    style QUALITY fill:#4ecdc4
    style TESTING fill:#95e1d3
    style BUILDING fill:#f9ca24
    style DOCKER fill:#00b894
    style GH_RELEASE fill:#6c5ce7
```

## 2. Git Workflow & CIDX Pipelines

```mermaid
graph LR
    subgraph "Developer Flow"
        LOCAL[👨‍💻 Local Dev]
        COMMIT[📝 Commit]
        PR[🔀 Pull Request]
        MERGE[✅ Merge to Main]
        TAG[🏷️ Tag Release]
    end

    subgraph "CIDX Pipelines"
        LOCAL_PIPE[pre-push<br/>security + code + test]
        PR_PIPE[pr<br/>security + code + test]
        MAIN_PIPE[main<br/>security + code + test + build]
        RELEASE_PIPE[release<br/>security + code + test<br/>+ build + docker + release]
    end

    LOCAL --> |git push| COMMIT
    COMMIT --> PR
    PR -.-> PR_PIPE
    PR --> |approved| MERGE
    MERGE -.-> MAIN_PIPE
    MERGE --> TAG
    TAG -.-> RELEASE_PIPE

    LOCAL -.-> |cidx run pre-push| LOCAL_PIPE

    style LOCAL_PIPE fill:#95e1d3
    style PR_PIPE fill:#4ecdc4
    style MAIN_PIPE fill:#f9ca24
    style RELEASE_PIPE fill:#6c5ce7
```

## 3. Environment-Based Execution

```mermaid
graph TB
    subgraph "Execution Context"
        CMD[cidx run release]
    end

    CMD --> DETECT{Environment<br/>Detection}

    DETECT -->|Local| LOCAL_MODE
    DETECT -->|CI| CI_MODE

    subgraph "Local Mode"
        LOCAL_MODE[🏠 Local Environment]
        LOCAL_MODE --> SEC_LOCAL[🛡️ Security: Full scan]
        LOCAL_MODE --> CODE_LOCAL[🎨 Code: Full check]
        LOCAL_MODE --> TEST_LOCAL[✅ Test: Full suite]
        LOCAL_MODE --> BUILD_LOCAL[🔧 Build: Full build]
        LOCAL_MODE --> DOCKER_LOCAL[🐳 Docker: Build<br/>⚠️ NO PUSH]
        LOCAL_MODE --> RELEASE_LOCAL[📢 Release: Draft<br/>⚠️ NOT PUBLISHED]

        DOCKER_LOCAL -.->|Safe| DRAFT1[✓ Testable locally]
        RELEASE_LOCAL -.->|Safe| DRAFT2[✓ Testable locally]
    end

    subgraph "CI Mode"
        CI_MODE[☁️ CI Environment]
        CI_MODE --> SEC_CI[🛡️ Security: Full scan]
        CI_MODE --> CODE_CI[🎨 Code: Full check]
        CI_MODE --> TEST_CI[✅ Test: Full suite]
        CI_MODE --> BUILD_CI[🔧 Build: Full build]
        CI_MODE --> DOCKER_CI[🐳 Docker: Build<br/>✅ PUSH TO REGISTRY]
        CI_MODE --> RELEASE_CI[📢 Release: Publish<br/>✅ PUBLIC RELEASE]

        DOCKER_CI -.->|Production| PROD1[✓ Published to GHCR]
        RELEASE_CI -.->|Production| PROD2[✓ Published to GitHub]
    end

    style LOCAL_MODE fill:#95e1d3
    style CI_MODE fill:#00b894
    style DOCKER_LOCAL fill:#fdcb6e
    style RELEASE_LOCAL fill:#fdcb6e
    style DOCKER_CI fill:#00b894
    style RELEASE_CI fill:#00b894
```

## 4. Security Modes Detail

```mermaid
graph LR
    subgraph "Preset Configuration"
        PRESET[Preset Definition]
        PRESET --> RC[require_ci: false]
        PRESET --> LB[local_behavior]
    end

    LB --> DRAFT[draft<br/>GitHub releases<br/>as drafts]
    LB --> NOPUSH[no-push<br/>Docker build<br/>without push]
    LB --> DRYRUN[dry-run<br/>Simulation<br/>only]
    LB --> DISABLED[disabled<br/>Refuse<br/>execution]
    LB --> PROD[production<br/>Full<br/>execution]

    subgraph "Local Behavior"
        DRAFT -.-> DRAFT_EX[✓ gh-release<br/>✓ goreleaser]
        NOPUSH -.-> NOPUSH_EX[✓ docker-buildx<br/>✓ kaniko]
        DRYRUN -.-> DRYRUN_EX[✓ Any preset<br/>simulation mode]
        DISABLED -.-> DISABLED_EX[✓ Highly sensitive<br/>operations]
        PROD -.-> PROD_EX[⚠️ Use with<br/>caution!]
    end

    style DRAFT fill:#a8e6cf
    style NOPUSH fill:#a8e6cf
    style DRYRUN fill:#ffd3b6
    style DISABLED fill:#ff8b94
    style PROD fill:#ffaaa5
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
graph TB
    subgraph "GitHub Actions Example"
        GH_PR[Pull Request Event]
        GH_MAIN[Push to Main Event]
        GH_TAG[Tag Push Event]

        GH_PR --> GH_PR_JOB[cidx run pr]
        GH_MAIN --> GH_MAIN_JOB[cidx run main]
        GH_TAG --> GH_TAG_JOB[cidx run release]
    end

    subgraph "GitLab CI Example"
        GL_MR[Merge Request Event]
        GL_MAIN[Push to Main Event]
        GL_TAG[Tag Push Event]

        GL_MR --> GL_MR_JOB[cidx run pr]
        GL_MAIN --> GL_MAIN_JOB[cidx run main]
        GL_TAG --> GL_TAG_JOB[cidx run release]
    end

    subgraph "Jenkins Example"
        JE_PR[PR Build]
        JE_MAIN[Main Build]
        JE_TAG[Tag Build]

        JE_PR --> JE_PR_JOB[cidx run pr]
        JE_MAIN --> JE_MAIN_JOB[cidx run main]
        JE_TAG --> JE_TAG_JOB[cidx run release]
    end

    GH_PR_JOB --> SAME[Same CIDX<br/>Different Platform]
    GL_MR_JOB --> SAME
    JE_PR_JOB --> SAME

    style SAME fill:#6c5ce7,color:#fff
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
