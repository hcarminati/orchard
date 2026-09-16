# orchard

[![CI](https://github.com/hcarminati/orchard/actions/workflows/ci.yml/badge.svg)](https://github.com/hcarminati/orchard/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/hcarminati/orchard)](https://github.com/hcarminati/orchard/releases)
[![License: MIT](https://img.shields.io/badge/License-MIT-violet.svg)](LICENSE)
[![Go 1.22+](https://img.shields.io/badge/Go-1.22+-00ADD8)](https://go.dev)

**Orchard** is a terminal-native companion for Claude Code multi-agent sessions. It visualizes your live agent hierarchy, tool calls, skill triggers, costs, and timing — all in a single window, while Claude Code keeps running normally.

```
╭─Agents [Timeline] · 3 running · 1 idle──────────╮╭─[Events] Files MCP──────────────────────────────╮
│ > ● session:a1b2c3d4  bash read  ▸commit         ││session:a1b2c3d4  ·  3 subagents  ·  running    │
│     └─ ● researcher   bash grep read             ││──────────────────────────────────────────────── │
│     └─ ● implementer  bash edit write  ▸build    ││14:02:31  PreToolUse   Edit                      │
│     └─ ● reviewer   ⚠ loop×4                     ││14:02:33  PostToolUse  Edit                      │
│                                                   ││14:02:35  PreToolUse   Bash                      │
│ 0s          15s         30s         45s           ││14:02:38  PostToolUse  Bash                      │
│ session ████████████████████████████████████████  ││                                                 │
│   researcher ██████████████                       ││                                                 │
│   implementer        ██████████████████           ││                                                 │
│   reviewer                    ████████████████    ││                                                 │
╰──────────────────────────────────────────────────╯╰─────────────────────────────────────────────────╯
 j/k navigate  t timeline  f filter:all  [/] switch tab  tab switch panel  q quit
```

No wrappers. No account required. No changes to how you run Claude Code. Just open a split pane, run `orchard`, and get instant visibility into every agent in your session.

---

## Why Orchard

When you run Claude Code with parallel subagents, it's hard to know what's actually happening:

- Which agent is stuck in a loop calling the same tool repeatedly?
- Which agent is the bottleneck holding up the whole session?
- What did each agent actually write to disk?
- How much did this session cost, broken down by agent?

Existing observability tools are web dashboards (require shipping your data out) or simple log dumpers (require reading walls of JSON). Orchard is neither: it's a terminal-native companion that runs locally, reads only what Claude Code already writes to `~/.claude/`, and renders everything in a single window alongside your editor.

---

## Features

### Live agent tree
Every agent, subagent, and parallel group shown in real time. Status dots (● running / ● idle / ● done / ● error) update live. Error agents float to the top with their error message inline.

### Skill pills and tool pills
Each agent shows which tools it has called (colored by category: `bash` `read` `edit`) and which skills it triggered (`▸commit` `▸build`). Compact, glanceable, never verbose.

### Loop / repetition detector
When an agent calls the same tool 3+ times in a row without switching, a `⚠ loop×N` badge replaces the pills. The badge resets the moment a different tool fires. Catches stuck agents before they burn more tokens.

### Session timeline view
Press `T` to switch the left panel to a horizontal timeline. Each agent is a bar proportional to its wall-clock span. The critical path (longest root-to-leaf chain) is highlighted. Press `T` again to return to the tree.

### Files tab
The Files tab shows every file write, edit, and notebook change across all agents — most recent first — with the operation, path, agent name, and relative timestamp. No more wondering what your agents actually modified.

### Token usage and estimated cost
Per-agent and session-total token counts with USD cost estimates (Haiku / Sonnet / Opus pricing built in). Configurable budget cap with color thresholds (amber at 80%, red at 100%).

### Session replay
```sh
orchard replay                        # replay the most recent session for this project
orchard replay --project /my/project  # replay a specific project
orchard replay --speed 4              # 4× faster than real time
```
Watch any past session unfold in the TUI, exactly as it happened, at any speed. Uses only the JSONL files Claude Code already writes locally — no data sent anywhere.

### Fuzzy search
Press `/` in the Events panel to search across all tool inputs, outputs, and messages for the focused agent. Press `esc` to clear. Prefix with `/r ` for regex mode.

### Watchdog alerts
Configurable alert when an agent has been quiet for too long — yellow badge at N minutes, red at 2N. Clears automatically when activity resumes.

### Help overlay
Press `?` at any time for a full keybindings reference, organized by section.

### `orchard init`
Interactive first-time setup wizard — merges hooks, runs health checks, and creates config in one shot:
```sh
orchard init
orchard init --port 7071   # use a custom port
```

### `orchard setup`
Non-interactive hook wiring for scripted environments:
```sh
orchard setup        # merges hook config into ~/.claude/settings.json
orchard setup --dry-run  # preview without writing
```
Non-destructive: never overwrites existing config, only appends.

### `orchard doctor`
Verify the integration is healthy before you start:
```sh
orchard doctor
# ✓ Port 7070 available
# ✓ ~/.claude/settings.json exists
# ✓ PreToolUse hook configured
# ✓ PostToolUse hook configured
# ✓ ~/.claude/projects/ readable
```

### `orchard history`
Browse past sessions for a project in a TUI picker. From there you can replay or export any session:
```sh
orchard history                        # sessions for the current directory
orchard history --project /my/project  # sessions for a specific project
```

### `orchard cancel`
Send SIGINT to running Claude Code processes for a project:
```sh
orchard cancel                         # cancel processes in the current directory
orchard cancel --project /my/project   # cancel a specific project
orchard cancel --dry-run               # show which processes would be cancelled
```

### `orchard diff`
Compare two sessions and show what changed — agent count, duration, cost, tool usage, skill triggers:
```sh
orchard diff session-a.jsonl session-b.jsonl
```
Session JSONL files are in `~/.claude/projects/<encoded-path>/`.

### `orchard agents`
List registered Claude Code agent types (from `~/.claude/settings.json` and past sessions):
```sh
orchard agents
orchard agents --project /my/project
```

### `orchard skills`
List registered skills (slash commands) visible to Orchard:
```sh
orchard skills
orchard skills --project /my/project
```

---

## Install

**Go install** (requires Go 1.22+):
```sh
go install github.com/hcarminati/orchard/cmd/orchard@latest
```

**Homebrew** (macOS / Linux):
```sh
brew install hcarminati/tap/orchard
```

**curl install script** (macOS / Linux):
```sh
curl -fsSL https://raw.githubusercontent.com/hcarminati/orchard/main/install.sh | sh
```

**Download binary** from [Releases](https://github.com/hcarminati/orchard/releases) — pre-built for macOS (arm64/amd64), Linux (amd64/arm64), and Windows (amd64).

---

## Quick start

1. **Install** using one of the methods above.

2. **Wire up hooks** so Claude Code notifies Orchard:
   ```sh
   orchard setup
   ```
   This adds hook entries to `~/.claude/settings.json`. Run `orchard doctor` to verify.

3. **Open a split pane** in your terminal (tmux, iTerm2, VS Code terminal, etc.).

4. **Start Orchard** in one pane:
   ```sh
   cd /your/project
   orchard
   ```

5. **Run Claude Code** in the other pane as normal:
   ```sh
   claude
   ```

Orchard will start displaying agents, events, and costs in real time.

---

## Keybindings

### Global
| Key | Action |
|-----|--------|
| `tab` | Switch panel focus (Agents ↔ Events) |
| `q` / `ctrl+c` | Quit |
| `?` | Open help overlay |
| `t` / `T` | Toggle timeline view |

### Agents panel
| Key | Action |
|-----|--------|
| `j` / `↓` | Move cursor down |
| `k` / `↑` | Move cursor up |
| `space` | Expand / collapse node or group |
| `enter` | Mark winner in a parallel group |
| `f` | Cycle filter: all → running → errored → hidden → all |
| `d` | Hide a completed session |
| `r` | Restore a hidden session (when in hidden filter) |

### Events panel
| Key | Action |
|-----|--------|
| `j` / `↓` | Scroll down / next event |
| `k` / `↑` | Scroll up / previous event |
| `space` | Expand / collapse inline tool call detail |
| `enter` | Open event detail modal |
| `g` | Jump to first event |
| `G` | Jump to last event |
| `/` | Fuzzy search events |
| `esc` | Close search / modal |

### Right panel tabs
| Key | Action |
|-----|--------|
| `[` / `]` | Cycle tabs: Events → Files → MCP → Events |

### Timeline view
| Key | Action |
|-----|--------|
| `t` | Return to tree view |

---

## Configuration

Orchard reads `~/.config/orchard/config.toml` on startup. The file is created automatically by `orchard setup`. All settings are optional.

```toml
# Monthly spend cap in USD. When session cost approaches this, the header
# color shifts amber (≥80%) then red (≥100%).
budget = 300.0

# Monthly token cap. Color shifts the same as budget.
max_tokens = 10000000

# Watchdog: alert when an agent has been quiet for this many minutes.
# 0 disables the watchdog.
watchdog_minutes = 10

# Cost alert: show a banner when any single session exceeds this amount.
cost_alert = 5.0
```

---

## How it works

```
Claude Code            Orchard
──────────             ──────────────────────────────────────────────
runs normally   ──►   hook server (port 7070) receives events
writes JSONL    ──►   session reader hydrates the initial tree
                      Bubbletea TUI re-renders on each event
```

Orchard listens on a local HTTP port. Claude Code's hook system POSTs a small JSON payload to that port each time it calls a tool, triggers a skill, or stops a session. Orchard parses those events, updates an in-memory agent tree, and re-renders the TUI — typically within a few milliseconds.

On startup, Orchard reads `~/.claude/projects/` JSONL files to hydrate any sessions that started before Orchard was launched. This means you can start Orchard mid-session and still see the full history.

No data leaves your machine. No API calls. No accounts. The hook server accepts connections only on localhost.

---

## Architecture

```
cmd/orchard/main.go       entry point — subcommand routing, program setup
internal/ui/              all TUI components (Bubbletea model, render, tree, timeline)
internal/agent/           agent tree data model (nodes, events, status, loop detection)
internal/hooks/           embedded HTTP server — receives and forwards hook events
internal/session/         reads ~/.claude/projects/ JSONL on startup
internal/config/          config loading/saving (config.toml, hidden.json, state.json)
internal/replay/          session replay engine (sorts events, scales timing)
internal/setup/           orchard setup logic (merges hook config into settings.json)
internal/doctor/          integration health checks
```

Data flow: hook events → `internal/hooks/` → channel → `internal/ui/` via Bubbletea messages → re-render. The `internal/agent/` package is the only shared state; all other packages are stateless transformers.

See [CLAUDE.md](CLAUDE.md) for full architecture conventions and package boundary rules.

---

## Development

To run from source:

```sh
go run ./cmd/orchard
```

## Contributing

See [CONTRIBUTING.md](.github/CONTRIBUTING.md) for dev setup, branch workflow, and code style.

**Quick start:**
```sh
git clone https://github.com/hcarminati/orchard.git
cd orchard
go run ./cmd/orchard --demo   # try the demo mode
go test ./...                  # run tests
```

---

## Disclaimer

Orchard is an independent open source project and is not affiliated with, endorsed by, or sponsored by Anthropic. Claude Code is a product of Anthropic. Orchard reads only session data that Claude Code writes locally to your machine.

## License

MIT — see [LICENSE](LICENSE).
