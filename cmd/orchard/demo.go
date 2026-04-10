package main

import "github.com/hcarminati/orchard/internal/agent"

// demoNodes returns a realistic set of fake agent nodes for local testing.
func demoNodes() []agent.Node {
	return []agent.Node{
		// Root orchestrator with two child agents.
		{
			ID:     "root",
			Name:   "session:abc123",
			Status: agent.StatusRunning,
			Tools:  []string{"Bash", "Read", "Glob"},
		},
		{
			ID:       "child-read",
			ParentID: "root",
			Name:     "session:def456",
			Status:   agent.StatusDone,
			Tools:    []string{"Read", "Grep"},
		},
		{
			ID:       "child-write",
			ParentID: "root",
			Name:     "session:ghi789",
			Status:   agent.StatusRunning,
			Tools:    []string{"Edit", "Write"},
		},
		// A second standalone session that errored.
		{
			ID:       "errored",
			Name:     "session:err000",
			Status:   agent.StatusError,
			ErrorMsg: "context length exceeded",
			Tools:    []string{"Bash"},
		},
		// A parallel group of three competing agents.
		{
			ID:      "par-a",
			Name:    "session:par111",
			GroupID: "g1",
			Status:  agent.StatusDone,
			Winner:  true,
			Tools:   []string{"Bash", "Read"},
		},
		{
			ID:      "par-b",
			Name:    "session:par222",
			GroupID: "g1",
			Status:  agent.StatusDone,
			Tools:   []string{"Read"},
		},
		{
			ID:      "par-c",
			Name:    "session:par333",
			GroupID: "g1",
			Status:  agent.StatusIdle,
			Tools:   []string{"Bash", "Glob", "Read"},
		},
	}
}
