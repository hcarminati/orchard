# Orchard Configuration

Orchard reads `~/.config/orchard/config.toml` on startup. The file is optional — Orchard works with zero configuration. All settings can be added incrementally; you only need to specify what you want to change.

## Creating the config file

Run `orchard setup` to create the file automatically, or create it manually:

```sh
mkdir -p ~/.config/orchard
cat > ~/.config/orchard/config.toml << 'EOF'
# Orchard configuration
budget = 300.0
watchdog_minutes = 10
EOF
```

## Config reference

### `budget`

```toml
budget = 300.0
```

Monthly spend cap in USD. When the total estimated cost across all visible sessions approaches this limit, the cost display in the Agents panel header changes color:
- **Amber** when ≥ 80% of the budget is used
- **Red** when 100% or more is used

Set to `0` (or omit) to disable budget tracking. The budget is a visual indicator only — Orchard never stops or throttles Claude Code.

### `max_tokens`

```toml
max_tokens = 10000000
```

Monthly token cap (total tokens across all sessions). Color thresholds work the same as `budget`. Displayed as `1.2M/10M` in the header.

Set to `0` (or omit) to disable token cap tracking.

### `watchdog_minutes`

```toml
watchdog_minutes = 10
```

Number of minutes without any tool call before a running agent is considered stuck. When the threshold is reached:
- A **yellow `⏱` badge** appears on the node at `watchdog_minutes`
- A **red `⏱⏱` badge** appears at `2 × watchdog_minutes`

The badge clears automatically when a new tool call arrives. Set to `0` (or omit) to disable the watchdog.

### `cost_alert`

```toml
cost_alert = 5.0
```

USD threshold for a single session. When a session's estimated cost exceeds this value, an amber banner appears above the footer:

```
⚠ Cost alert: session cost $6.23 exceeded threshold $5.00
```

Dismiss with `esc`. Set to `0` (or omit) to disable.

## Persistent UI state

Orchard also maintains two auto-managed files in `~/.config/orchard/`:

### `hidden.json`

Tracks which session IDs have been hidden by the user (via `d` in the Agents panel). Managed automatically. Sessions older than 7 days are auto-hidden on startup. Edit or delete this file to reset.

### `state.json`

Tracks which root sessions were expanded when Orchard last exited. On next launch, those sessions are re-expanded. Edit or delete to reset to default collapsed state.

## Environment

Orchard also respects these environment variables:

| Variable | Default | Description |
|----------|---------|-------------|
| `ORCHARD_PORT` | `7070` | Hook server port (overridden by `--port` flag) |
| `NO_COLOR` | unset | If set, disables ANSI color output |

## Hook server port

If port 7070 is in use (e.g. by another Orchard instance), start on a different port:

```sh
orchard --port 8080
```

Then update the hook config to match:

```sh
orchard setup --port 8080
```

Or run `orchard doctor` to see which port is in the configured hooks.
