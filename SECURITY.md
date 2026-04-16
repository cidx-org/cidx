# Security Policy

## Reporting a Vulnerability

If you discover a security vulnerability in CIDX, please report it responsibly.

**Email:** crashrunover@gmail.com

Please include:

- Description of the vulnerability
- Steps to reproduce
- Affected versions (if known)

I will acknowledge receipt within 48 hours and aim to provide a fix or mitigation within 7 days.

## Scope

CIDX runs containers on your local machine and generates CI configurations. Security concerns include:

- **Container escape** -- Preset configurations that could break isolation
- **Secret exposure** -- Accidental leaking of credentials in logs or generated configs
- **Config injection** -- Malicious `cidx.toml` values that execute unintended commands

## Vulnerability Management

CIDX uses Trivy and Grype for dependency scanning. Known exceptions are documented in `.trivyignore` and `known-vulnerabilities.toml` with justification.
