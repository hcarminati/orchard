# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.4.0] - 2026-04-13

### Added
- Tab-based right panel: `[` / `]` cycles between Events and future tabs
- Events tab: real events for the focused agent, scrollable with timestamps
- Tool call inspector: expand any event with `enter` to see full input and output
- Skill triggers surfaced as a distinct `SkillTrigger` event type with a `⚡` label and green color
- Session context header in the Events tab for all nodes (root and subagent)

## [0.3.0] - 2026-04-10

### Added
- Session status lifecycle: Running → Idle (3 s after last tool call) → Done (10 min idle); status dots reflect live state
- Status dot colors: green (running), yellow (idle), grey (done), red (error); error message surfaced inline
- Parallel run grouping: agents sharing a `GroupID` appear under a collapsible `parallel × N` header; `enter` marks a winner
- Collapsible agent tree: `space` or mouse click expands/collapses nodes and group headers; `▶`/`▼` indicators
- Status filter toggle: `f` cycles All → Running → Errored → All; footer shows current mode
- Mouse click support: left-click in the Agents panel moves cursor and toggles collapse/expand
- Line truncation with ellipsis (`…`) prevents long names from wrapping and breaking click coordinates
- `--demo` flag: populates the tree with fake agents for local testing without a live Claude Code session

## [0.2.0] - 2026-04-10

### Added
- Agent tree data model (`internal/agent`): nodes, parent-child relationships, group IDs for parallel runs, status transitions
- Hook HTTP server (`internal/hooks`): receives Claude Code hook events (PreToolUse, PostToolUse, Stop, SubagentStop, Notification), filters by CWD, forwards to TUI via channel
- Session JSONL reader (`internal/session`): hydrates initial agent tree from `~/.claude/projects/` on startup
- Live data pipeline: hook events flow into agent tree and re-render the TUI in real time
- `--port` flag: start the hook server on a custom port (default 7070); validates range and type
- `--version` flag: print version and exit; version set at build time via `-ldflags`
- Unit and integration tests for all v0.2 packages (agent: 100%, hooks: 96%, session: 84%)

## [0.1.0] - 2026-04-09

### Added
- Initial TUI skeleton with two-panel layout (Agents / Events)
- `tab` to switch focus between panels, `q` to quit
- Bottom keybindings bar
- CI workflow: build, test, vet, golangci-lint on ubuntu-latest and macos-latest
- Release workflow: cross-compiled binaries for darwin/linux/windows + checksums
- GitHub issue templates and PR template
- MIT license
