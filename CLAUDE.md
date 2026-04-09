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

## Design principle

Unified visibility in a single terminal window. The user should be able to see many agents running in parallel and trace the reasoning path from task to result without switching contexts. Every feature decision should make the existing view richer or clearer — cut anything that fragments attention or adds a new place to look.

## What to avoid

- Do not use `interface{}` — use concrete types or typed interfaces
- Do not add CGO — the binary must cross-compile cleanly
- Do not write to `~/.claude/` — read only, except for `orchard setup` (v0.6)
- Do not add runtime dependencies — the binary must be self-contained; nothing for the user to install alongside it
- Do not overwrite user config — merge and append only; never destructively replace existing settings

## Tests

A milestone is not done until its tests are written and passing. When implementing any feature, write the tests as part of the same change — not after.
