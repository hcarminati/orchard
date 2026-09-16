# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [1.0.0] - 2026-05-25

### Added
- Comprehensive README.md: full feature list, ASCII art, keybindings table, quick start, architecture overview, install methods, common questions
- `install.sh`: curl-installable shell script with platform detection, checksum verification, sudo-aware binary placement
- `Formula/orchard.rb`: Homebrew formula template for multi-platform binary distribution via `brew tap hcarminati/tap`
- `docs/architecture.md`: full internal architecture — package map, data flow diagram, Bubbletea/Lipgloss patterns, testing approach
- `docs/configuration.md`: complete config reference for all config.toml fields, hidden.json, state.json, and environment variables
- `docs/getting-started.md`: step-by-step onboarding from install through first session, split-pane setup for tmux/VS Code/iTerm2
- Updated `CONTRIBUTING.md`: expanded with test patterns, feature checklist, package boundary rules, design principle

## [0.9.0] - 2026-05-25

### Added
- Help overlay (`?`): full-screen keybinding reference organized into sections (Navigation, Events, Tree, View Modes, Session Management); includes live "Current State" panel showing active filter/panel/tab; `?`, `esc`, or `q` closes it
- `orchard setup` subcommand: non-destructively merges Orchard hook entries (PreToolUse/PostToolUse/Stop/SubagentStop/Notification) into `~/.claude/settings.json`; `--port`, `--dry-run`, `--verify` flags
- `orchard doctor` subcommand: runs health checks (port availability, settings.json, hooks configured, projects directory readable, Go runtime version); exits 0 on all-pass, 1 otherwise
- MCP Servers tab (`MCP`): third right-panel tab reads `mcpServers` from `~/.claude/settings.json` and lists server name and command/URL
- `internal/setup` package with `Run()`, `DefaultSettingsPath()`, and `DryRun()` — full test coverage
- `internal/doctor` package with `RunAll()` and `AllPass()` — per-check tests

## [0.8.0] - 2026-05-25

### Added
- config.toml support: `watchdog_minutes` and `cost_alert` fields in `~/.config/orchard/config.toml`
- Watchdog timer: when a running agent makes no tool call for `watchdog_minutes`, a yellow `⏱` badge appears on its tree row; after `2×watchdog_minutes` the badge escalates to red `⏱⏱`; the badge clears automatically on new activity
- Cost alert banner: when a session's estimated cost exceeds `cost_alert`, an amber one-line banner appears above the footer; dismiss with `esc`
- Fuzzy search (`/`): press `/` to open a search bar in the footer; type to filter the events list by tool name, event type, or content; `esc` or `enter` closes the bar
- Terminal bell: `\a` fires when a session reaches Done or Error state (once per session)

## [0.7.0] - 2026-05-25

### Added
- Session replay: `orchard replay [--project PATH] [--speed N]` plays back any past session from JSONL history at configurable speed; the standard TUI renders the replay identically to a live session (`internal/replay` package)
- Files tab: real file-change data from `PostToolUse` Write/Edit/NotebookEdit events across all agents; shows operation label, path, agent name, and relative timestamp
- Session timeline view: press `T` to switch the Agents panel to a horizontal timeline showing each agent as a proportional bar on a shared time axis; critical path (longest root→leaf chain) is highlighted in accent color
- Loop/repetition detector: when an agent calls the same tool ≥ 3 times consecutively, a `⚠ loop×N` warning badge replaces the pills on its tree node; badge resets when a different tool fires
- Skill pills and tool pills inline on tree nodes: `▸skill-name` in green for skills, colored lowercase labels for tools (bash/read/edit/grep/etc.)
- Status summary in Agents panel header: `N running · N idle · N done` (only non-zero buckets); replaces raw count
- `Node.Tools` and `Node.Skills` slices populated from live hook events: tools deduplicated on `PreToolUse`, skills on `SkillTrigger`
- `Node.SpawnedAt` set for all live nodes (hook-created and subagent placeholders) from the triggering event timestamp
- `appendUnique` helper in `internal/agent` for deduplication

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
