# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.2.0] - 2026-04-10

### Added
- Agent tree data model (`internal/agent`): nodes, parent-child relationships, group IDs for parallel runs, status transitions
- Hook HTTP server (`internal/hooks`): receives Claude Code hook events (PreToolUse, PostToolUse, Stop, SubagentStop, Notification), filters by CWD, forwards to TUI via channel
- Session JSONL reader (`internal/session`): hydrates initial agent tree from `~/.claude/projects/` on startup
- Live data pipeline: hook events flow into agent tree and re-render the TUI in real time
- `--port` flag: start the hook server on a custom port (default 7070); validates range and type
- `--version` flag: print version and exit; version set at build time via `-ldflags`
- Unit and integration tests for all v0.2 packages (agent: 100%, hooks: 96%, session: 84%)

## [0.1.0] - 2026-03-01

### Added
- Initial TUI skeleton with two-panel layout (Agents / Events)
- `tab` to switch focus between panels, `q` to quit
- Bottom keybindings bar
- CI workflow: build, test, vet, golangci-lint on ubuntu-latest and macos-latest
- Release workflow: cross-compiled binaries for darwin/linux/windows + checksums
- GitHub issue templates and PR template
- MIT license
