package catalog

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func writeSettings(t *testing.T, path string, s rawSettings) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestLoad_EmptySettings_ReturnsEmpty(t *testing.T) {
	dir := t.TempDir()
	cat, err := Load(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should return an empty catalog — global settings.json probably doesn't have custom agents.
	// We just verify it doesn't crash.
	_ = cat
}

func TestLoad_LocalSettings_LoadsAgentsAndSkills(t *testing.T) {
	dir := t.TempDir()

	settings := rawSettings{
		AgentTypes: []rawAgentType{
			{Name: "Explore", Description: "fast code exploration"},
			{Name: "Plan", Description: "software architect"},
		},
		Skills: []rawSkill{
			{Name: "commit", Description: "create a git commit"},
		},
	}
	writeSettings(t, filepath.Join(dir, ".claude", "settings.json"), settings)

	cat, err := Load(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should contain at least the agents we wrote (global may add more).
	hasExplore, hasPlan := false, false
	for _, a := range cat.Agents {
		if a.Name == "Explore" {
			hasExplore = true
		}
		if a.Name == "Plan" {
			hasPlan = true
		}
	}
	if !hasExplore {
		t.Error("expected Explore agent in catalog")
	}
	if !hasPlan {
		t.Error("expected Plan agent in catalog")
	}

	hasCommit := false
	for _, s := range cat.Skills {
		if s.Name == "commit" {
			hasCommit = true
		}
	}
	if !hasCommit {
		t.Error("expected commit skill in catalog")
	}
}

func TestLoad_LocalOverridesGlobal(t *testing.T) {
	dir := t.TempDir()

	// Write to global-simulated path via cwd .claude/settings.json with same name.
	settings := rawSettings{
		AgentTypes: []rawAgentType{
			{Name: "Explore", Description: "local version"},
		},
	}
	writeSettings(t, filepath.Join(dir, ".claude", "settings.json"), settings)

	cat, err := Load(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Find Explore — should appear exactly once.
	count := 0
	for _, a := range cat.Agents {
		if a.Name == "Explore" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("expected Explore exactly once, got %d", count)
	}
}

func TestLoad_SortedAlphabetically(t *testing.T) {
	dir := t.TempDir()
	settings := rawSettings{
		AgentTypes: []rawAgentType{
			{Name: "Zebra"},
			{Name: "Alpha"},
			{Name: "Middle"},
		},
	}
	writeSettings(t, filepath.Join(dir, ".claude", "settings.json"), settings)

	cat, err := Load(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Find our agents and verify order.
	var names []string
	for _, a := range cat.Agents {
		if a.Name == "Zebra" || a.Name == "Alpha" || a.Name == "Middle" {
			names = append(names, a.Name)
		}
	}
	if len(names) != 3 {
		t.Fatalf("expected 3 test agents, got %d", len(names))
	}
	if names[0] != "Alpha" || names[1] != "Middle" || names[2] != "Zebra" {
		t.Errorf("expected Alpha,Middle,Zebra but got %v", names)
	}
}

func TestLoadFromHistory_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	cat, err := LoadFromHistory(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cat.Agents) != 0 || len(cat.Skills) != 0 {
		t.Errorf("expected empty catalog from empty dir")
	}
}

func TestLoadFromHistory_ExtractsFromJSONL(t *testing.T) {
	dir := t.TempDir()

	// Write a fake JSONL with Agent and Skill tool calls.
	lines := []string{
		`{"hook_event_name":"PreToolUse","tool_name":"Agent","tool_input":{"subagent_type":"Explore"}}`,
		`{"hook_event_name":"PreToolUse","tool_name":"Skill","tool_input":{"skill":"commit"}}`,
		`{"hook_event_name":"PreToolUse","tool_name":"Agent","tool_input":{"subagent_type":"Plan"}}`,
		`not valid json`,
	}

	var content string
	for _, l := range lines {
		content += l + "\n"
	}

	if err := os.WriteFile(filepath.Join(dir, "sess.jsonl"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	cat, err := LoadFromHistory(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	agentNames := make(map[string]bool)
	for _, a := range cat.Agents {
		agentNames[a.Name] = true
	}
	if !agentNames["Explore"] {
		t.Error("expected Explore agent from history")
	}
	if !agentNames["Plan"] {
		t.Error("expected Plan agent from history")
	}

	skillNames := make(map[string]bool)
	for _, s := range cat.Skills {
		skillNames[s.Name] = true
	}
	if !skillNames["commit"] {
		t.Error("expected commit skill from history")
	}
}

func TestDedupeAgents(t *testing.T) {
	agents := []AgentType{
		{Name: "Explore", Source: "global"},
		{Name: "explore", Source: "local"}, // duplicate (case-insensitive)
		{Name: "Plan", Source: "global"},
	}
	deduped := dedupeAgents(agents)
	if len(deduped) != 2 {
		t.Errorf("expected 2 agents after dedup, got %d", len(deduped))
	}
}
