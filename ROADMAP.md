# Orchard Roadmap

Orchard is built in two tracks that share the same binary:

- **Observability** — real-time visibility into Claude Code agent sessions
- **Project setup** — scaffolding and managing Claude Code projects

The goal is to be the definitive developer tool for Claude Code multi-agent workflows — the equivalent of lazygit, but for AI orchestration.

---

## Track 1: Observability

### v0.1 — Skeleton ✅
- Two-panel Bubbletea TUI (Agents / Events)
- `tab` to switch panels, `q` to quit
- Bottom keybindings bar
- CI/CD, release workflow, repo infrastructure

### v0.2 — Data pipeline
The foundation everything else builds on.
- `internal/agent/` — agent tree data model (nodes, parent-child relationships, status, tools, skills, prompt/instructions)
- Node model includes a `groupID` field to link parallel competing runs from the start
- `internal/hooks/` — embedded HTTP server (port `7070`) that receives Claude Code hook events: `PreToolUse`, `PostToolUse`, `Stop`, `SubagentStop`, `Notification`
- `internal/session/` — reads `~/.claude/projects/` JSONL on startup to hydrate initial state; re-hydrates automatically if Orchard is restarted mid-session
- `--port` flag to override the default port if 7070 is in use
- `--version` flag
- Wire all three into the TUI — real data flowing even if rendering is still basic

### v0.3 — Agent tree
- Collapsible tree rendered in the left panel
- `j`/`k` to navigate nodes, `space` to collapse/expand
- Status dots colored by state: running / idle / done / error
- Error surfacing — errored agents pulled to top of tree with message visible
- Click to focus a node (mouse)
- **Parallel runs**: sibling nodes sharing a `groupID` are visually grouped with a `parallel × N` label; when one is selected as best it gets a ✓ indicator, the others render as dismissed

### v0.4 — Event log & right panel tabs
- Right panel becomes tab-based: `Events`, `Files`, and future tabs share the same panel
- `[` / `]` to cycle right panel tabs
- Events tab: real events for the focused agent, scrollable, with timestamps
- Tool call inspector: expand any call to see full input and output
- Skill triggers surfaced as distinct event types

### v0.5 — Rich node display
- Model badges on each node (haiku / sonnet / opus)
- Green skill pills and blue tool pills inline on tree nodes
- Agent prompt/instructions visible when a node is focused (in event log or tooltip)
- Status summary in the header: `3 running · 2 idle · 1 done`
- Token usage and estimated cost per agent and session total

### v0.6 — Skills context menu
- Focus any agent node and press `s` to open a popup showing skills wired to that agent
- Each skill shows its name and one-line description
- Select and confirm writes a trigger instruction to a named pipe or temp file that Claude Code watches — exact IPC mechanism defined during implementation
- Escape closes without running anything

### v0.7 — File awareness
- Files tab in the right panel (introduced in v0.4): every file touched this session, by which agent, in order
- Diff viewer: expand any file change to see the diff inline, sourced from `PostToolUse` Write/Edit events

### v0.8 — Search, alerts & config
- Orchard config file at `~/.config/orchard/config.toml` — port, cost display preferences, watchdog settings
- `f` to fuzzy search across all agent event logs in the current session
- Loop/repeat detector: highlight agents calling the same tool repeatedly
- Watchdog: configurable alert when an agent runs for more than N minutes without output (N set in config)
- Notifications: terminal bell or desktop notification on agent done/error

### v0.9 — UX polish
- `?` opens a help overlay
- Full mouse support throughout all panels and tabs
- MCP server panel: which servers are loaded, what tools they provide
- `orchard setup` subcommand: non-destructively merges hook config into `~/.claude/settings.json`

### v1.0 — Distribution
- Homebrew tap
- curl install script
- Demo gif in README
- Full onboarding docs: how to split the terminal, run `orchard setup`, and get started

---

## Track 1 (continued): Power features

### v1.1 — History & replay
- Session history: browse past Claude Code sessions from `~/.claude/projects/`, not just the live one
- Replay mode: play back a recorded session in real time
- Export: save a full session as markdown or JSON for sharing or post-mortem
- Copy to clipboard: any agent output, tool call, or full session summary

### v1.2 — Multi-session observation
- Observe multiple concurrent Claude Code sessions simultaneously (each on a different port or session ID)
- Switch between sessions within Orchard
- Per-session status summary in a top-level session picker

### v1.3 — Spawn & control
- From a focused agent node, manually spawn a new subagent with a prompt
- Requires a defined IPC mechanism between Orchard and Claude Code (built on the same channel established in v0.6)

---

## Track 2: Project setup

### v1.4 — `orchard init`
A wizard-style TUI flow for scaffolding a new Claude Code project:
- Project name and stack
- Generate `CLAUDE.md` with tailored guidance
- Define agents: roles, models, instructions
- Wire up skills
- Write Claude Code settings and hook config via the same mechanism as `orchard setup`
- Confirm → write all files

### v1.5 — Project management
- `orchard agents` — view and edit agent definitions for the current project
- `orchard skills` — manage skills attached to agents
- Edit `CLAUDE.md` inline from the TUI
- Diff view: current config vs last committed version

### v1.6 — Templates
- Built-in project templates: web app, CLI tool, data pipeline, etc.
- Community template registry (pull from GitHub)
- Export a project config as a shareable template

---

## Guiding principles

- **Observability first** — the tree and event log are the core product; everything else is additive
- **Each version ships value** — every milestone is useful on its own, not just a stepping stone
- **Zero runtime dependencies** — single binary, nothing to install
- **Terminal-native** — works identically in any terminal, any IDE
- **Non-intrusive** — Orchard is a companion, never a wrapper around Claude Code
- **Non-destructive** — Orchard never overwrites user config; it merges and appends only
