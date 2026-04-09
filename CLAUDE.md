# Orchard

A lazygit-inspired TUI companion for Claude Code. Visualizes the live agent hierarchy, tool calls, and skill triggers from a Claude Code session in real time.

## Architecture

```
cmd/orchard/main.go       entry point — creates and runs the Bubbletea program
internal/ui/              all TUI components and layout (Bubbletea models)
internal/agent/           agent tree data model — nodes, parent-child, status
internal/hooks/           embedded HTTP server (port 7070) — receives Claude Code hook events
internal/session/         reads ~/.claude/projects/ JSONL files on startup
```

The data flow is: hook events → `internal/hooks/` → update `internal/agent/` tree → Bubbletea sends a message → `internal/ui/` re-renders.

## Build & run

```sh
go run ./cmd/orchard        # run locally
go build ./cmd/orchard      # build binary
go test ./...               # run all tests
go vet ./...                # vet
```

## Key conventions

- **Bubbletea patterns**: models are value types (not pointers), `Update` returns a new model. Never mutate state directly.
- **Lipgloss**: define styles as package-level vars, not inline. Use `lipgloss.Width()` to measure rendered strings — not `len()`.
- **Package boundaries**: `internal/ui/` must not import `internal/hooks/` or `internal/session/` directly. Data flows in via Bubbletea messages only.
- **No globals**: pass dependencies explicitly. The HTTP server sends events over a channel; the TUI reads from it.
- **Errors**: always handle them. No `_` for errors at package boundaries.

## Current milestone: v0.2 — Data pipeline

Next things to build:
1. `internal/agent/` — `Node` struct with ID, name, model, status, parentID, children, tools, skills
2. `internal/hooks/` — HTTP server on `:7070`, parse incoming hook JSON, emit to a channel
3. `internal/session/` — scan `~/.claude/projects/`, parse JSONL event files, return initial agent tree
4. Wire into `internal/ui/` via a `tea.Cmd` that listens on the channel

See ROADMAP.md for the full picture.

## What to avoid

- Do not add features beyond the current milestone scope
- Do not use `interface{}` — use concrete types or typed interfaces
- Do not add CGO — the binary must cross-compile cleanly
- Do not write to `~/.claude/` — read only, except for `orchard setup` (v0.6)
