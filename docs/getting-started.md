# Getting Started with Orchard

This guide walks you through installing Orchard, wiring it up to Claude Code, and understanding the interface.

## Prerequisites

- **Claude Code** installed and working (`claude --version`)
- **macOS or Linux** (Windows support is experimental)
- **Go 1.22+** if installing via `go install`; not needed for binary installs

## Step 1: Install

Choose the method that works best for you.

**Homebrew (recommended for macOS):**
```sh
brew tap hcarminati/tap
brew install orchard
```

**curl install script:**
```sh
curl -fsSL https://raw.githubusercontent.com/hcarminati/orchard/main/install.sh | sh
```

**Go install:**
```sh
go install github.com/hcarminati/orchard/cmd/orchard@latest
```

**Pre-built binary:** Download from [Releases](https://github.com/hcarminati/orchard/releases) and place in your `$PATH`.

Verify the install:
```sh
orchard --version
```

## Step 2: Wire up Claude Code hooks

Orchard receives events from Claude Code via its hook system. Run `orchard setup` to configure this automatically:

```sh
orchard setup
```

This adds hook entries to `~/.claude/settings.json` without touching any existing entries. You'll see output like:

```
✓ Added PreToolUse hook (port 7070)
✓ Added PostToolUse hook (port 7070)
✓ Added Stop hook (port 7070)
✓ Added SubagentStop hook (port 7070)
✓ Config written to ~/.claude/settings.json
```

Verify the setup:
```sh
orchard doctor
```

Expected output:
```
✓ Port 7070 available
✓ ~/.claude/settings.json exists
✓ PreToolUse hook configured
✓ PostToolUse hook configured
✓ Stop hook configured
✓ ~/.claude/projects/ readable
All checks passed.
```

## Step 3: Open a split pane

Orchard works in any split-pane setup. Some options:

**tmux:**
```sh
tmux new-session \; split-window -h \; send-keys 'claude' C-m \; select-pane -L \; send-keys 'cd /your/project && orchard' C-m
```

**VS Code:** Open the integrated terminal, split it, run `orchard` in one pane.

**iTerm2:** Split the pane with `Cmd+D`, run `orchard` in the left pane.

**Any terminal emulator** with split-pane support works.

## Step 4: Run Orchard

In the Orchard pane, navigate to your project and start Orchard:

```sh
cd /your/project
orchard
```

You'll see:
```
╭─Agents──────────────────╮╭─[Events] Files MCP──────────────────────╮
│                         ││                                          │
│ Waiting for session…    ││ Waiting for session…                     │
│                         ││                                          │
╰─────────────────────────╯╰──────────────────────────────────────────╯
 j/k navigate  t timeline  f filter:all  [/] switch tab  tab switch panel  q quit
```

## Step 5: Start Claude Code

In the other pane, run Claude Code as normal:

```sh
claude
```

As soon as you give Claude Code a prompt, Orchard will start populating with agent nodes, events, and real-time status.

## Understanding the interface

```
╭─Agents · 2 running · 1 done────────────────╮╭─[Events] Files MCP──────────────────────────╮
│ > ● session:a1b2c3d4  bash read  ▸commit    ││session:a1b2c3d4  ·  2 subagents  ·  running │
│     └─ ● researcher   bash grep             ││────────────────────────────────────────────  │
│     └─ ● implementer  bash edit write       ││14:02:31  PreToolUse   Edit                   │
│                                             ││  > internal/ui/render.go                     │
╰─────────────────────────────────────────────╯╰──────────────────────────────────────────────╯
```

**Left panel (Agents):**
- One row per agent. Indented rows are subagents.
- **Status dot**: green = running, yellow = idle, grey = done, red = error
- **Pills**: lowercase colored tool names (`bash` `read` `edit`) and green skill triggers (`▸commit`)
- **Header**: status summary (`2 running · 1 done`) + total cost + total tokens

**Right panel (Events / Files / MCP):**
- **Events tab**: all tool calls for the focused agent, scrollable. Press `enter` or click to see full input/output.
- **Files tab**: all file writes and edits across all agents, most-recent first.
- **MCP tab**: MCP servers loaded for this session.

## Key things to try

**Navigate agents:**
- `j` / `k` to move the cursor up and down
- `space` to collapse or expand a node
- Click any node with the mouse

**Inspect tool calls:**
- Tab to the Events panel (`tab`)
- `j` / `k` to move through events
- `enter` to open the full detail modal for any event
- `space` to toggle inline expansion

**Switch to timeline view:**
- Press `t` to see agents as horizontal time bars
- Critical path is highlighted in purple
- Press `t` again to return to the tree

**Search events:**
- Press `/` to open fuzzy search
- Type to filter; `esc` to clear
- Prefix with `/r ` for regex mode

**Check costs:**
- The Agents panel header always shows total cost and token count
- Each agent's cost is shown in the right panel header when focused

**Replay a past session:**
```sh
orchard replay
orchard replay --speed 3   # 3× faster
```

## Common questions

**Orchard shows "Waiting for session…" and nothing appears.**
1. Check `orchard doctor` — are the hooks configured?
2. Restart Claude Code after running `orchard setup` — the hook config is read at startup.
3. Make sure you're running `orchard` from the same directory as Claude Code.

**The hook server port is already in use.**
Use a different port: `orchard --port 8080` and `orchard setup --port 8080`.

**I want Orchard to show sessions from a different directory.**
Use `orchard --project /path/to/project`.

**How do I hide old sessions?**
Press `d` on any completed root session. Hidden sessions are recoverable with `f` (filter to hidden), then `r` to restore.

## Next steps

- See [configuration.md](configuration.md) for budget caps, watchdog timers, and cost alerts
- See [architecture.md](architecture.md) if you want to understand the internals or contribute
- See [CHANGELOG.md](../CHANGELOG.md) for what's changed in each release
