// Package catalog reads Claude Code agent types and skills from ~/.claude/settings.json
// and from local .claude/settings.json, then aggregates them for display.
package catalog

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// AgentType describes a registered Claude Code subagent type.
type AgentType struct {
	Name        string // display name (e.g. "Explore", "Plan")
	Description string // from the agentType's description field, if any
	Source      string // "global" or "local"
}

// Skill describes a registered skill (slash command or prompt template).
type Skill struct {
	Name        string // skill trigger name
	Description string // from description field
	Source      string // "global" or "local"
}

// Catalog holds the discovered agents and skills for a project.
type Catalog struct {
	Agents []AgentType
	Skills []Skill
}

// rawSettings is the subset of settings.json we care about.
type rawSettings struct {
	AgentTypes []rawAgentType `json:"agentTypes"`
	Skills     []rawSkill     `json:"skills"`
	// Some versions use "subagents" instead of "agentTypes".
	Subagents []rawAgentType `json:"subagents"`
}

type rawAgentType struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

type rawSkill struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Trigger     string `json:"trigger"`
}

// Load reads agent types and skills from global (~/.claude/settings.json) and
// local (.claude/settings.json in cwd) settings, merging them into one Catalog.
func Load(cwd string) (Catalog, error) {
	var cat Catalog

	// Load global settings.
	if path, err := globalSettingsPath(); err == nil {
		if s, err := readSettings(path); err == nil {
			cat.Agents = append(cat.Agents, convertAgents(s, "global")...)
			cat.Skills = append(cat.Skills, convertSkills(s, "global")...)
		}
	}

	// Load local settings (cwd/.claude/settings.json or cwd/.claude/settings.local.json).
	for _, name := range []string{"settings.json", "settings.local.json"} {
		path := filepath.Join(cwd, ".claude", name)
		if s, err := readSettings(path); err == nil {
			cat.Agents = append(cat.Agents, convertAgents(s, "local")...)
			cat.Skills = append(cat.Skills, convertSkills(s, "local")...)
		}
	}

	// Deduplicate by name (local overrides global for same name).
	cat.Agents = dedupeAgents(cat.Agents)
	cat.Skills = dedupeSkills(cat.Skills)

	// Sort alphabetically for stable display.
	sort.Slice(cat.Agents, func(i, j int) bool {
		return strings.ToLower(cat.Agents[i].Name) < strings.ToLower(cat.Agents[j].Name)
	})
	sort.Slice(cat.Skills, func(i, j int) bool {
		return strings.ToLower(cat.Skills[i].Name) < strings.ToLower(cat.Skills[j].Name)
	})

	return cat, nil
}

// LoadFromHistory aggregates agent types and skills observed in past session JSONL files.
// This is a fallback when settings.json doesn't list them explicitly.
// It reads the sessionDir for JSONL files and extracts unique agent type names and skill names.
func LoadFromHistory(sessionDir string) (Catalog, error) {
	entries, err := os.ReadDir(sessionDir)
	if err != nil {
		if os.IsNotExist(err) {
			return Catalog{}, nil
		}
		return Catalog{}, err
	}

	agentSet := make(map[string]bool)
	skillSet := make(map[string]bool)

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".jsonl") {
			continue
		}
		path := filepath.Join(sessionDir, entry.Name())
		agents, skills := extractFromJSONL(path)
		for _, a := range agents {
			agentSet[a] = true
		}
		for _, s := range skills {
			skillSet[s] = true
		}
	}

	var cat Catalog
	for name := range agentSet {
		cat.Agents = append(cat.Agents, AgentType{Name: name, Source: "observed"})
	}
	for name := range skillSet {
		cat.Skills = append(cat.Skills, Skill{Name: name, Source: "observed"})
	}

	sort.Slice(cat.Agents, func(i, j int) bool {
		return strings.ToLower(cat.Agents[i].Name) < strings.ToLower(cat.Agents[j].Name)
	})
	sort.Slice(cat.Skills, func(i, j int) bool {
		return strings.ToLower(cat.Skills[i].Name) < strings.ToLower(cat.Skills[j].Name)
	})

	return cat, nil
}

// extractFromJSONL scans a single JSONL file for agent type names and skill triggers.
func extractFromJSONL(path string) (agents []string, skills []string) {
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}

	type line struct {
		Type      string `json:"type"`
		HookEvent string `json:"hook_event_name"`
		ToolName  string `json:"tool_name"`
		ToolInput struct {
			SubagentType string `json:"subagent_type"`
			Skill        string `json:"skill"`
		} `json:"tool_input"`
		Message struct {
			Role    string `json:"role"`
			Content json.RawMessage `json:"content"`
		} `json:"message"`
	}

	agentSet := make(map[string]bool)
	skillSet := make(map[string]bool)

	for _, rawLine := range strings.Split(string(data), "\n") {
		rawLine = strings.TrimSpace(rawLine)
		if rawLine == "" {
			continue
		}
		var l line
		if err := json.Unmarshal([]byte(rawLine), &l); err != nil {
			continue
		}
		if l.ToolName == "Agent" && l.ToolInput.SubagentType != "" {
			agentSet[l.ToolInput.SubagentType] = true
		}
		if l.ToolName == "Skill" && l.ToolInput.Skill != "" {
			skillSet[l.ToolInput.Skill] = true
		}
	}

	for a := range agentSet {
		agents = append(agents, a)
	}
	for s := range skillSet {
		skills = append(skills, s)
	}
	return
}

func globalSettingsPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".claude", "settings.json"), nil
}

func readSettings(path string) (rawSettings, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return rawSettings{}, err
	}
	var s rawSettings
	if err := json.Unmarshal(data, &s); err != nil {
		return rawSettings{}, err
	}
	return s, nil
}

func convertAgents(s rawSettings, source string) []AgentType {
	var out []AgentType
	all := append(s.AgentTypes, s.Subagents...)
	for _, a := range all {
		if a.Name != "" {
			out = append(out, AgentType{Name: a.Name, Description: a.Description, Source: source})
		}
	}
	return out
}

func convertSkills(s rawSettings, source string) []Skill {
	var out []Skill
	for _, sk := range s.Skills {
		name := sk.Name
		if name == "" {
			name = sk.Trigger
		}
		if name != "" {
			out = append(out, Skill{Name: name, Description: sk.Description, Source: source})
		}
	}
	return out
}

func dedupeAgents(agents []AgentType) []AgentType {
	seen := make(map[string]bool)
	var out []AgentType
	// Process in reverse so local (appended last) wins over global.
	for i := len(agents) - 1; i >= 0; i-- {
		key := strings.ToLower(agents[i].Name)
		if !seen[key] {
			seen[key] = true
			out = append([]AgentType{agents[i]}, out...)
		}
	}
	return out
}

func dedupeSkills(skills []Skill) []Skill {
	seen := make(map[string]bool)
	var out []Skill
	for i := len(skills) - 1; i >= 0; i-- {
		key := strings.ToLower(skills[i].Name)
		if !seen[key] {
			seen[key] = true
			out = append([]Skill{skills[i]}, out...)
		}
	}
	return out
}
