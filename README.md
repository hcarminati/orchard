# orchard

[![CI](https://github.com/hcarminati/orchard/actions/workflows/ci.yml/badge.svg)](https://github.com/hcarminati/orchard/actions/workflows/ci.yml)
[![License: MIT](https://img.shields.io/badge/License-MIT-violet.svg)](LICENSE)

A lazygit-inspired TUI companion for Claude Code. Orchard visualizes your AI agent activity in real time — which agents spawned which subagents, what skills were triggered, what tools were called, and the live status of every agent in the session.

<!-- demo gif here -->

## Overview

Orchard runs in a terminal split pane **alongside** Claude Code. It reads Claude Code session data from `~/.claude/` and listens to hook events via an embedded local HTTP server. Claude Code keeps running normally — Orchard just gives you visibility into what's happening.

```
┌─────────────────────┬──────────────────────────────────────────┐
│  orchard            │  Claude Code (running normally)          │
│                     │                                          │
│  Agents             │                                          │
│  ● main-agent       │                                          │
│    ├─ ● explore     │                                          │
│    └─ ● plan        │                                          │
│                     │                                          │
│  tab: switch  q: quit                                          │
└─────────────────────┴──────────────────────────────────────────┘
```

Split the terminal in tmux, iTerm2, VS Code, or any IDE terminal — Orchard works identically in all of them.

## Install

**Go install:**
```sh
go install github.com/hcarminati/orchard/cmd/orchard@latest
```

**Homebrew:** coming soon

**curl script:** coming soon

## Usage

Run Orchard in one pane and Claude Code in the other:

```sh
orchard
```

## Keybindings

| Key | Action |
|-----|--------|
| `tab` | Switch panel focus |
| `q` / `ctrl+c` | Quit |

More keybindings coming as features are added.

## Contributing

Active development happens on the `dev` branch. Features and fixes branch off `dev` as `feat/name` or `fix/name` and merge back via PR. Nothing commits directly to `main` — `main` is tagged releases only.

```sh
git checkout dev
git checkout -b feat/my-feature
# ... work ...
git push origin feat/my-feature
# open a PR targeting dev
```

See [CHANGELOG.md](CHANGELOG.md) for what's in flight and [ROADMAP.md](ROADMAP.md) for what's planned.

## License

MIT — see [LICENSE](LICENSE).
