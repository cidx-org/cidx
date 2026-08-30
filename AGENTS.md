# Working on CIDX

Read and follow [`CLAUDE.md`](CLAUDE.md) in full before changing this
repository. Its product guardrails, BDD-first workflow, architecture and code
style apply equally to every coding agent.

## Dogfood CIDX

Use CIDX itself for repository work and validation:

- Run `go run ./cmd/cidx run code` before security checks, tests or builds.
- Run the relevant CIDX pipeline rather than substituting ad-hoc host commands.
- Use `go run ./cmd/cidx pr create`, `go run ./cmd/cidx cpw`,
  `go run ./cmd/cidx pr status`, `go run ./cmd/cidx pr watch` and
  `go run ./cmd/cidx pr merge` for the PR lifecycle.
- Do not use `gh` or raw `git` for operations CIDX supports. If CIDX lacks the
  operation or its UX blocks the workflow, report that gap and treat fixing it
  as a product concern instead of silently bypassing it.

Keep the execution order observable: code quality is the first gate, followed
by security, tests and build. For feature behavior, discuss first, write the BDD
scenario, implement only what it specifies, then validate the complete suite.
