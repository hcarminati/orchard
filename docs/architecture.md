# Orchard Architecture

This document describes how Orchard is structured internally and how data flows through the system. It is intended for contributors and for anyone who wants to understand how the pieces fit together.

## Overview

Orchard is a single Go binary. It has no runtime dependencies — no databases, no daemons, nothing for the user to install alongside it. All state lives in memory while Orchard is running; persistent state (hidden sessions, expanded/collapsed tree state, config) lives in `~/.config/orchard/`.

```
┌─────────────────────────────────────────────────────────────────┐
│                          orchard binary                          │
│                                                                   │
│  cmd/orchard/main.go                                              │
│       │                                                           │
│       ├─► internal/session/   (read JSONL on startup)            │
│       ├─► internal/hooks/     (HTTP server on :7070)             │
│       ├─► internal/config/    (load config, hidden, state)       │
│       └─► internal/ui/        (Bubbletea TUI)                    │
│                │                                                  │
│                └─► internal/agent/   (tree data model)           │
│                                                                   │
│  Subcommands:                                                     │
│       orchard replay  ─► internal/replay/                        │
│       orchard setup   ─► internal/setup/                         │
│       orchard doctor  ─► internal/doctor/                        │
└─────────────────────────────────────────────────────────────────┘
```

## Data flow

```
Claude Code                  Orchard
──────────                   ──────────────────────────────────────
hook fires (PreToolUse) ──► POST /  → internal/hooks/server.go
                              │         • parse JSON payload
                              │         • filter by CWD
                              │         • convert to agent.Event
                              │         • send to eventCh chan
                              ▼
                            internal/ui/model.go  (waitForEvent)
                              │         • tea.Cmd blocks on channel
                              │         • returns hookEventMsg
                              ▼
                            Model.Update(hookEventMsg)
                              │         • tree.ApplyEvent(e)
                              │         • schedule idle timer
                              │         • re-render
                              ▼
                            Model.View()
                              │         • renderBody()
                              │         • renderFooter()
                              └─► terminal output
```

On startup, `internal/session/` reads JSONL files from `~/.claude/projects/` to hydrate the initial tree before any live events arrive. This means Orchard can show history from a session that started before Orchard was launched, and it handles restart mid-session gracefully.

## Package responsibilities

### `internal/agent`

The core data model. Owns:
- `Node` — one per agent/session, holds events, status, tool/skill lists, usage, loop detection state
- `Tree` — the parent-child hierarchy, aliasing for subagent placeholder matching, delegation tracking
- `Event` — a single hook event (type, tool, input, response, timestamp, usage)
- `ApplyEvent` — the single mutation point; all state changes flow through here

Package boundary: `internal/agent` imports nothing from the rest of Orchard. It is pure data model.

### `internal/hooks`

The embedded HTTP server. Owns:
- `Server` — wraps `http.Server`, filters events by CWD, converts JSON payloads to `agent.Event`, sends to a channel
- `payload` — mirrors the Claude Code hook JSON shape

Package boundary: `internal/hooks` only imports `internal/agent` (for the `Event` type). It never imports `internal/ui`.

### `internal/session`

Reads `~/.claude/projects/` JSONL on startup. Owns:
- `Load(cwd)` — finds the matching project directory, parses JSONL, returns `[]agent.Node`
- `loadFrom(base, cwd)` — testable core; can be called with a custom base path

Package boundary: reads files only. Does not write. Does not import `internal/ui` or `internal/hooks`.

### `internal/ui`

All TUI components. Owns:
- `Model` — all TUI state (cursor, scroll, collapsed map, filter mode, timeline mode, search, etc.)
- `model.go` — `New`, `Init`, `Update` — the Bubbletea contract
- `render.go` — `View`, `renderBody`, `renderFooter`, `agentsContent`, `eventsContent`, `filesContent`, `timelineContent`, and all helper renderers
- `tree.go` — `visibleNodes`, tree traversal, sort, filter logic
- `timeline.go` — timeline-specific helpers: `subtreeEntries`, `criticalPathNodes`, `renderBar`, `renderRuler`

Package boundary: `internal/ui` never imports `internal/hooks` or `internal/session`. Data arrives exclusively via Bubbletea messages. This is enforced at the package level — it makes the TUI trivially testable without a running HTTP server.

### `internal/config`

Config persistence. Owns:
- `LoadConfig()` / `SaveConfig()` — `~/.config/orchard/config.toml`
- `LoadHidden()` / `SaveHidden()` — `~/.config/orchard/hidden.json`
- `LoadExpanded()` / `SaveExpanded()` — `~/.config/orchard/state.json`

Config fields: `Budget`, `MaxTokens`, `WatchdogMinutes`, `CostAlert`.

### `internal/replay`

Session replay engine. Owns:
- `Run(nodes, out, opts, done)` — sorts all events chronologically, drips them to a channel with scaled delays
- `AllEvents(nodes)` — flat sorted event slice (useful for computing duration before starting)
- `Duration(nodes)` — total wall-clock span of events

No TUI awareness. Can be tested without a display.

### `internal/setup`

Hook config management. Owns:
- `MergeHooks(settingsPath, port string) ([]Change, error)` — reads `settings.json`, adds Orchard hook entries if not present, writes back
- `Change` — describes what was added/already-present/skipped
- `DryRun(settingsPath, port string) ([]Change, error)` — same but does not write

### `internal/doctor`

Integration health checks. Owns:
- `Check(port int) []Result` — runs all health checks, returns results
- `Result` — one check result: name, passed bool, message

## Bubbletea patterns

Orchard follows Bubbletea strictly:

1. **Model is a value type.** `Update` receives a copy, returns a modified copy. Never mutate `m` fields directly — always work on the copy and return it.

2. **No goroutine-to-model communication except through channels and `tea.Cmd`.** The HTTP server sends to a channel; the TUI reads from it via `waitForEvent` (a `tea.Cmd` that blocks). This keeps all model mutation on the main goroutine.

3. **Side effects via `tea.Cmd`.** File writes (save hidden, save state) return `tea.Cmd` values. The TUI handles the result via message types (`stateSavedMsg`, `hideSavedMsg`).

4. **Timer pattern.** `scheduleIdle` / `scheduleDone` return `tea.Cmd` values that sleep and return a typed message. A generation counter (`timerGen`) lets `Update` discard stale timer messages when new activity arrives.

## Lipgloss rendering rules

- All styles are package-level `var` — never inline. This allows easy theming.
- `lipgloss.Width()` always measures rendered strings; never use `len()` on ANSI-colored strings.
- The agents panel uses `truncate()` to cap every line at `innerW` visible columns. This is essential: mouse click coordinates map to line numbers, and wrapping would shift all subsequent rows.
- Panel borders are built by `renderPanel()` which replaces the top border line with a custom one embedding the title. This avoids the lipgloss border API fighting with dynamic titles.

## Testing approach

- Unit tests for all packages live alongside the source in `*_test.go` files.
- `internal/ui` tests use `newWithClock(nodes, nil, nil, time.Time{})` to create a Model without a live event channel. They call `m.Update(tea.WindowSizeMsg{...})` to set dimensions, then inspect `View()` output as strings.
- `internal/hooks` has an integration test using `httptest.NewServer`.
- `internal/replay` tests use `Options{Speed: 10000, MaxGap: time.Millisecond}` for instant replay in tests.
- No mocks for the filesystem — tests that need files use `t.TempDir()`.

## Adding a new feature

Typical checklist:
1. If it's data model change → `internal/agent/node.go`, update `ApplyEvent`, add tests to `node_test.go`
2. If it's a new render → `internal/ui/render.go` or a new `internal/ui/something.go`, tests in `something_test.go`
3. If it's a new TUI state → add to `Model` struct in `model.go`, handle in `Update`, expose via `View`
4. If it's a new command → add to `cmd/orchard/main.go` subcommand routing, add `internal/yourpkg/` with tests
5. Always update `CHANGELOG.md` under `[Unreleased]`
6. Always update keybindings in `README.md` if keybindings changed
