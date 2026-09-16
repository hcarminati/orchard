# orchard

[![CI](https://github.com/hcarminati/orchard/actions/workflows/ci.yml/badge.svg)](https://github.com/hcarminati/orchard/actions/workflows/ci.yml)
[![License: MIT](https://img.shields.io/badge/License-MIT-violet.svg)](LICENSE)

Orchard visualizes your AI agent activity in real time — which agents spawned which subagents, what skills were triggered, what tools were called, and the live status of every agent in the session.

<!-- demo gif here -->

## Overview

Orchard runs in a terminal split pane **alongside** Claude Code. It reads Claude Code session data from `~/.claude/` and listens to hook events via an embedded local HTTP server. Claude Code keeps running normally. Orchard just gives you visibility into what's happening.

<img width="1391" height="688" alt="Screenshot 2026-04-13 at 1 45 09 PM" src="https://github.com/user-attachments/assets/39ab77d0-cba6-4557-8f50-11080a713169" />

Split the terminal in tmux, iTerm2, VS Code, or any IDE terminal; Orchard works identically in all of them.

See [CHANGELOG.md](CHANGELOG.md) for what's in flight and [ROADMAP.md](ROADMAP.md) for what's planned.

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
orchard                   # start on default port 7070
orchard --port 8080       # use a custom hook server port
orchard --version         # print version and exit
```

## Keybindings

| Key | Action |
|-----|--------|
| `tab` | Switch panel focus |
| `q` / `ctrl+c` | Quit |

More keybindings coming as features are added.

## Development

To run from source:

```sh
go run ./cmd/orchard
```

## Contributing

Active development happens on the `dev` branch. Features and fixes branch off `dev` as `feat/name` or `fix/name` and merge back via PR. Nothing commits directly to `main` — `main` is tagged releases only.

```sh
git checkout dev
git checkout -b feat/my-feature
# ... work ...
git push origin feat/my-feature
# open a PR targeting dev
```

See [CONTRIBUTING.md](.github/CONTRIBUTING.md) for dev setup and contribution guidelines.

## Disclaimer

Orchard is an independent open source project and is not affiliated with, endorsed by, or sponsored by Anthropic. Claude Code is a product of Anthropic. Orchard simply reads session data that Claude Code writes locally to your machine.

This software is provided as-is with no warranty. See the MIT license for details.

## License

MIT — see [LICENSE](LICENSE).
