package config

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Config holds user-configurable Orchard settings loaded from config.toml.
type Config struct {
	// Budget is the monthly spend cap in USD. When set, cost display shows
	// "$spent/$budget". Zero means no budget is configured.
	Budget float64
	// MaxTokens is the monthly token cap. When set, token display shows
	// "used/max". Zero means no cap is configured.
	MaxTokens int
	// WatchdogMinutes is the number of minutes without any tool call before a
	// running agent is considered stuck. A yellow ⏱ badge appears at this
	// threshold; a red ⏱⏱ badge appears at 2×. Zero disables the watchdog.
	WatchdogMinutes int
	// CostAlert is the per-session USD threshold. When a session's estimated
	// cost exceeds this value, an amber banner is shown above the footer.
	// Zero disables cost alerts.
	CostAlert float64
}

// LoadConfig reads ~/.config/orchard/config.toml and returns the parsed settings.
// Missing file returns a zero Config (no budget). Parse errors are silently
// skipped per-line — a partially valid file is better than a hard failure.
func LoadConfig() (Config, error) {
	path, err := configPath("config.toml")
	if err != nil {
		return Config{}, err
	}
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return Config{}, nil
	}
	if err != nil {
		return Config{}, err
	}
	defer f.Close()

	var cfg Config
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		// Skip comments and blank lines.
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, val, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		val = strings.TrimSpace(val)
		switch key {
		case "budget":
			if n, err := strconv.ParseFloat(val, 64); err == nil && n >= 0 {
				cfg.Budget = n
			}
		case "max_tokens":
			if n, err := strconv.Atoi(val); err == nil && n >= 0 {
				cfg.MaxTokens = n
			}
		case "watchdog_minutes":
			if n, err := strconv.Atoi(val); err == nil && n >= 0 {
				cfg.WatchdogMinutes = n
			}
		case "cost_alert":
			if n, err := strconv.ParseFloat(val, 64); err == nil && n >= 0 {
				cfg.CostAlert = n
			}
		}
	}
	return cfg, scanner.Err()
}

func configPath(name string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "orchard", name), nil
}

func loadSet(name string) (map[string]bool, error) {
	path, err := configPath(name)
	if err != nil {
		return make(map[string]bool), err
	}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return make(map[string]bool), nil
	}
	if err != nil {
		return make(map[string]bool), err
	}
	var ids []string
	if err := json.Unmarshal(data, &ids); err != nil {
		return make(map[string]bool), err
	}
	m := make(map[string]bool, len(ids))
	for _, id := range ids {
		m[id] = true
	}
	return m, nil
}

func saveSet(name string, ids map[string]bool) error {
	path, err := configPath(name)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	keys := make([]string, 0, len(ids))
	for id := range ids {
		keys = append(keys, id)
	}
	data, err := json.Marshal(keys)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func LoadHidden() (map[string]bool, error)   { return loadSet("hidden.json") }
func SaveHidden(h map[string]bool) error     { return saveSet("hidden.json", h) }
func LoadExpanded() (map[string]bool, error) { return loadSet("state.json") }
func SaveExpanded(e map[string]bool) error   { return saveSet("state.json", e) }
