# CIDX — Product Scope

## Product promise

CIDX integrates into existing projects in two commands.

One config file. Same checks locally and in CI. The tool adapts to the project — never the other way around.

If CIDX cannot be explained in 20 seconds and adopted in 2 commands, it is drifting.

---

## Two-command integration

Every product decision must be judged against these two canonical paths.

### Path A — local integration

```
cidx init
cidx run ci
```

Detection, config generation, immediate proof that it works.

### Path B — CI integration

```
cidx init
cidx generate <platform> -o <file>
```

Plug an existing project into CI without rewriting its plumbing by hand.

If a feature does not make one of these paths clearer, faster, or safer — it does not belong in the product core.

---

## Core product

These capabilities define what CIDX is.

| Command        | Role                                              |
| -------------- | ------------------------------------------------- |
| `init`         | Detect project, generate config                   |
| `run`          | Execute phases, tools, pipelines                  |
| `generate`     | Produce CI workflow from config                   |
| `validate`     | Verify config correctness                         |
| `doctor`       | Diagnose environment readiness                    |
| `preset`       | Inspect and manage built-in container definitions |
| `status` / TUI | Unified project state view                        |

Supporting concepts that are part of the core:

- **Project detection** — language, framework, remote provider
- **Presets + overrides** — convention over configuration, customizable when needed
- **Local/CI parity** — same phases, same tools, different safety modes
- **Safe local execution** — no-push, draft, dry-run defaults protect the developer
- **Dry-run everywhere** — see what will execute before it runs

---

## Secondary capabilities

These exist, are maintained, and are dogfooded daily — but they do not define the product's identity.

| Capability                         | Purpose                                               |
| ---------------------------------- | ----------------------------------------------------- |
| PR helpers (`pr`, `cpw`)           | Dogfooding workflow, find CIDX bugs through daily use |
| Release helpers (`release`, `tag`) | Orchestrate semantic versioning and changelogs        |
| Branch management (`branch`)       | List, filter, clean up branches                       |
| Workflow watching (`workflow`)     | Monitor CI runs in real time                          |
| Artifact management (`artifact`)   | Inspect and clean GitHub Actions artifacts            |
| Vulnerability workflows (`vuln`)   | Exception management for security scanners            |
| Registry utilities (`registry`)    | Docker registry auth and status                       |

These are useful. They are real. They solve real friction.

But they answer different questions than the core:

- How do I manage my PRs better?
- How do I pilot my releases?
- How do I track my CI runs?

These are not the same question as: **how do I integrate CIDX into my project?**

---

## CLI hierarchy principle

**Top-level commands = core product only.**

The top-level CLI must tell the product story. When a user runs `cidx --help`, they should see an integration engine — not a DevOps suite.

Secondary capabilities live under namespaces:

- `cidx repo` — PR, branches, workflows, artifacts
- `cidx release` — prepare, tag, create
- `cidx security` — vuln exceptions, registry

This does not reduce their accessibility. It clarifies what CIDX is about.

---

## TUI principle

One entry point. One product facade. No parallel identities.

### The TUI must show

- Project state (branch, config, environment)
- Detected configuration and presets
- Pipelines, phases, and tools that will execute
- Local vs CI execution differences
- Quick diagnostics

### The TUI must not become

- A complete repo command center
- A GitHub/GitLab cockpit
- A global governance dashboard
- A total DevSecOps control panel

Multiple standalone TUIs (merge, release, artifact) create parallel product identities. They should become screens or panels within the central TUI — not separate entry points.

---

## Anti-confusion

CIDX is an integration engine for existing projects.

CIDX is not:

- a DevOps suite
- a repo management platform
- a full delivery cockpit
- a toolbox that does everything

The moment CIDX starts looking like any of these, it is drifting. See [docs/GUARDRAILS.md](GUARDRAILS.md) for the non-negotiable constraints that prevent this.

---

## Feature triage rule

For every proposed feature, ask:

**Does this help an existing project adopt CIDX faster, more clearly, or more safely?**

- **Yes** — core candidate
- **No** — secondary capability, lives under a namespace
- **Unclear** — danger zone, defer until the answer is obvious
