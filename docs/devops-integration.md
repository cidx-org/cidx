# CIDX & DevOps Integration

This document explains how CIDX integrates into standard DevOps workflows and product lifecycle.

## 1. CIDX in the DevOps Loop

```mermaid
flowchart TB
    PLAN[📋 Plan]
    CODE[💻 Code]
    BUILD[🔨 Build]
    TEST[🧪 Test]
    RELEASE[📦 Release]
    DEPLOY[🚀 Deploy]
    OPERATE[⚙️ Operate]
    MONITOR[📊 Monitor]

    SECURITY[🛡️ Security\ntrivy, gitleaks]
    QUALITY[🎨 Code Quality\nprettier, golangci-lint]
    TESTING[✅ Testing\ngo-test, molecule]
    BUILDING[🔧 Build\ngo-build, compile]
    DOCKER[🐳 Docker\nkaniko, buildx]
    GH_RELEASE[📢 Release\ngh-release, goreleaser]

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
flowchart LR
    LOCAL[👨‍💻 Local Dev]
    COMMIT[📝 Commit]
    PR[🔀 Pull Request]
    MERGE[✅ Merge to Main]
    TAG[🏷️ Tag Release]

    LOCAL_PIPE[pre-push\nsecurity + code + test]
    PR_PIPE[pr\nsecurity + code + test]
    MAIN_PIPE[main\nsecurity + code + test + build]
    RELEASE_PIPE[release\nsecurity + code + test\n+ build + docker + release]

    LOCAL -->|git push| COMMIT
    COMMIT --> PR
    PR -.-> PR_PIPE
    PR -->|approved| MERGE
    MERGE -.-> MAIN_PIPE
    MERGE --> TAG
    TAG -.-> RELEASE_PIPE
    LOCAL -.->|cidx run pre-push| LOCAL_PIPE

    style LOCAL_PIPE fill:#95e1d3
    style PR_PIPE fill:#4ecdc4
    style MAIN_PIPE fill:#f9ca24
    style RELEASE_PIPE fill:#6c5ce7
```

## 3. Environment-Based Execution

```mermaid
flowchart TB
    CMD[cidx run release]
    DETECT{Environment\nDetection}

    CMD --> DETECT

    DETECT -->|Local| LOCAL_MODE[🏠 Local Environment]
    DETECT -->|CI| CI_MODE[☁️ CI Environment]

    LOCAL_MODE --> SEC_LOCAL[🛡️ Security: Full scan]
    LOCAL_MODE --> CODE_LOCAL[🎨 Code: Full check]
    LOCAL_MODE --> TEST_LOCAL[✅ Test: Full suite]
    LOCAL_MODE --> BUILD_LOCAL[🔧 Build: Full build]
    LOCAL_MODE --> DOCKER_LOCAL[🐳 Docker: Build\n⚠️ NO PUSH]
    LOCAL_MODE --> RELEASE_LOCAL[📢 Release: Draft\n⚠️ NOT PUBLISHED]

    CI_MODE --> SEC_CI[🛡️ Security: Full scan]
    CI_MODE --> CODE_CI[🎨 Code: Full check]
    CI_MODE --> TEST_CI[✅ Test: Full suite]
    CI_MODE --> BUILD_CI[🔧 Build: Full build]
    CI_MODE --> DOCKER_CI[🐳 Docker: Build\n✅ PUSH TO REGISTRY]
    CI_MODE --> RELEASE_CI[📢 Release: Publish\n✅ PUBLIC RELEASE]

    DOCKER_LOCAL -.->|Safe| DRAFT1[✓ Testable locally]
    RELEASE_LOCAL -.->|Safe| DRAFT2[✓ Testable locally]
    DOCKER_CI -.->|Production| PROD1[✓ Published to GHCR]
    RELEASE_CI -.->|Production| PROD2[✓ Published to GitHub]

    style LOCAL_MODE fill:#95e1d3
    style CI_MODE fill:#00b894
    style DOCKER_LOCAL fill:#fdcb6e
    style RELEASE_LOCAL fill:#fdcb6e
    style DOCKER_CI fill:#00b894
    style RELEASE_CI fill:#00b894
```

## 4. Security Modes Detail

```mermaid
flowchart LR
    PRESET[Preset Definition]
    RC[require_ci: false]
    LB[local_behavior]

    PRESET --> RC
    PRESET --> LB

    LB --> DRAFT[draft\nGitHub releases\nas drafts]
    LB --> NOPUSH[no-push\nDocker build\nwithout push]
    LB --> DRYRUN[dry-run\nSimulation\nonly]
    LB --> DISABLED[disabled\nRefuse\nexecution]
    LB --> PROD[production\nFull\nexecution]

    DRAFT -.-> DRAFT_EX[✓ gh-release\n✓ goreleaser]
    NOPUSH -.-> NOPUSH_EX[✓ docker-buildx\n✓ kaniko]
    DRYRUN -.-> DRYRUN_EX[✓ Any preset\nsimulation mode]
    DISABLED -.-> DISABLED_EX[✓ Highly sensitive\noperations]
    PROD -.-> PROD_EX[⚠️ Use with\ncaution!]

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

    GH_PR_JOB --> SAME[Same CIDX\nDifferent Platform]
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
