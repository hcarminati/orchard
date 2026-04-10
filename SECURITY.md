# Security Policy

## Reporting a vulnerability

Please **do not** open a public GitHub issue for security vulnerabilities.

Instead, email **hcarminati** (GitHub: [@hcarminati](https://github.com/hcarminati)) directly. You can find contact details on the GitHub profile. I will respond within **7 days**.

Please include:
- A description of the vulnerability
- Steps to reproduce
- Your assessment of the impact

## Scope

Orchard is a local-only tool. It:

- Reads files from `~/.claude/` on your local machine (read-only)
- Runs a local HTTP server on `localhost:7070` (or a custom port) to receive hook events from Claude Code
- Makes **no external network requests** — no telemetry, no analytics, no remote calls

The attack surface is limited to the local machine. Vulnerabilities in the local HTTP server (e.g., SSRF, request smuggling) or unsafe handling of session file data are in scope.

## Supported versions

Only the latest release is actively maintained.
