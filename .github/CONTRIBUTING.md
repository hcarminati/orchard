# Contributing to Orchard

Thank you for contributing! Orchard is a Go + Bubbletea TUI for Claude Code multi-agent sessions. Read this guide before opening a PR.

## Dev environment

```sh
git clone https://github.com/hcarminati/orchard.git
cd orchard
go run ./cmd/orchard --demo     # run with fake agents (no Claude Code needed)
go run ./cmd/orchard --version  # print version
go test ./...                   # run all tests
go vet ./...                    # vet
golangci-lint run               # lint (install: https://golangci-lint.run)
```

Go 1.22+ is required. No external tools or runtime dependencies — the binary is self-contained.

## Branch workflow

1. Fork the repo on GitHub
2. Create a feature branch off `dev`:
   ```sh
   git checkout dev
   git checkout -b feat/your-feature   # or fix/your-fix
   ```
3. Commit your changes and open a PR targeting `dev`
4. `main` is tagged releases only — do not PR directly to `main`

## Code style and conventions

- Run `go fmt ./...` and `go vet ./...` before submitting
- Follow all conventions in [CLAUDE.md](../CLAUDE.md) — especially:
  - No `interface{}` — use concrete types or typed interfaces
  - No CGO — the binary must cross-compile cleanly
  - No new runtime dependencies — binary must be self-contained
  - Package boundaries: `internal/ui/` must not import `internal/hooks/` or `internal/session/`
  - `internal/agent/` must not import anything from the rest of Orchard
- Bubbletea: model is a value type. `Update` returns a new model — never mutate in place.
- Lipgloss: styles as package-level vars, never inline. Use `lipgloss.Width()` for measurement.

## Tests are required

**A milestone is not done until its tests are written and passing.**

Every new feature must include:
- Unit tests for any new functions or data model changes
- Integration or render tests for any new TUI behavior
- All existing tests must still pass (`go test ./...`)

Test patterns:
- For UI: `newWithClock(nodes, nil, nil, time.Time{})` creates a testable Model
- For events: send `tea.WindowSizeMsg{Width: 120, Height: 40}` then check `View()` output
- For new packages: use `t.TempDir()` for filesystem tests; no mocks for the filesystem
- For replay: use `Options{Speed: 10000, MaxGap: time.Millisecond}` for instant tests

## Adding a new feature: checklist

1. **Data model change?** → `internal/agent/node.go`, update `ApplyEvent`, tests in `node_test.go`
2. **New render?** → `internal/ui/render.go` or `internal/ui/newfeature.go`, tests in `newfeature_test.go`
3. **New TUI state?** → add field to `Model` in `model.go`, handle in `Update`, expose in `View`
4. **New subcommand?** → route in `cmd/orchard/main.go`, implement in `internal/yourpkg/`, tests there
5. **Always:** update `CHANGELOG.md` under `[Unreleased]`
6. **Keybinding change?** → update keybindings table in `README.md`
7. **Behavior change?** → update `docs/` if applicable

See [docs/architecture.md](../docs/architecture.md) for a full description of package responsibilities and data flow.

## Opening a good issue

Please include:

- **Steps to reproduce** — what you ran, what you expected, what happened
- **OS and terminal** — e.g., macOS 26.3, iTerm2 3.5, tmux 3.4
- **Go version** — `go version`
- **Orchard version** — `orchard --version`
- **Relevant log output** — any error messages from Orchard or Claude Code

Bug reports without reproduction steps are hard to act on.

## Design principle

> One window, full picture. If understanding a session requires looking in more than one place, Orchard has failed.

Before adding a feature, ask: does this make the existing view richer or clearer? Does it require switching context? If it fragments attention, reconsider.

## License

By contributing, you agree that your changes are licensed under the project's [MIT License](../LICENSE).
