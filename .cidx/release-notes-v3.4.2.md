# Release v3.4.2

This patch release improves the runtime image, workflow verification and dependency security.

## Fixes

- Include `curl` in the CIDX image for CI job scripts. Release image verification now requires both `cidx --version` and `curl --version` to succeed. (#477)
- Make `pr merge --watch` follow the commit actually landed on the target branch. Older successful runs cannot validate the new merge; missing identity, lookup errors and timeout fail verification explicitly. (#478)
- Exercise real pipeline execution in BDD scenarios, check phase order and failure handling, and make local draft release previews explicit. GitHub CI runs tests after security succeeds. (#472)

## Security

- Update `go-git` to 5.19.2 and its `golang.org/x/crypto`, `x/net` and `x/text` dependencies. (#430)
- A comparison of the Go dependency manifests with the same Trivy database on 2026-09-11 reduced HIGH/CRITICAL findings from 19 to 4; all four remaining findings are HIGH. This measures dependency findings, not runtime exploitability or every image in the preset catalogue.
- Remaining findings: `CVE-2026-41567` and `CVE-2026-42306` in `github.com/docker/docker`, `CVE-2026-29181` in OpenTelemetry, and `CVE-2026-56854` in `golang.org/x/crypto`.

## Maintenance and documentation

- Reuse the catalogue status summary in security audit reports and the tracking issue. (#473)
- Recommend release binaries and checksum verification as the primary installation method; Go installation remains available. (#475)

## Validation

- The complete CIDX pipeline passed after integrating the dependency update with the workflow fix: code quality, security, 414 BDD scenarios plus unit tests, and build.
- The corrected runtime image was built locally: CIDX and curl both start, and curl advertises HTTPS support.
