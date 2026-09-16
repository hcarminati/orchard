package replay

import (
	"testing"
	"time"

	"github.com/hcarminati/orchard/internal/agent"
)

var baseTime = time.Date(2026, 5, 25, 12, 0, 0, 0, time.UTC)

// --- AllEvents ---

func TestAllEvents_SortedByTimestamp(t *testing.T) {
	nodes := []agent.Node{
		{
			ID: "a",
			Events: []agent.Event{
				{Type: "PostToolUse", Timestamp: baseTime.Add(10 * time.Second)},
				{Type: "PreToolUse", Timestamp: baseTime.Add(5 * time.Second)},
			},
		},
		{
			ID: "b",
			Events: []agent.Event{
				{Type: "PreToolUse", Timestamp: baseTime.Add(2 * time.Second)},
			},
		},
	}
	events := AllEvents(nodes)

	if len(events) != 3 {
		t.Fatalf("expected 3 events, got %d", len(events))
	}
	// Must be in ascending timestamp order.
	for i := 1; i < len(events); i++ {
		if events[i].Timestamp.Before(events[i-1].Timestamp) {
			t.Errorf("events[%d] (%v) is before events[%d] (%v) — not sorted",
				i, events[i].Timestamp, i-1, events[i-1].Timestamp)
		}
	}
}

func TestAllEvents_EmptyNodes(t *testing.T) {
	events := AllEvents(nil)
	if len(events) != 0 {
		t.Errorf("expected 0 events for nil nodes, got %d", len(events))
	}
}

// --- Duration ---

func TestDuration_CorrectSpan(t *testing.T) {
	nodes := []agent.Node{
		{
			ID: "a",
			Events: []agent.Event{
				{Timestamp: baseTime},
				{Timestamp: baseTime.Add(30 * time.Second)},
				{Timestamp: baseTime.Add(60 * time.Second)},
			},
		},
	}
	d := Duration(nodes)
	if d != 60*time.Second {
		t.Errorf("expected 60s, got %v", d)
	}
}

func TestDuration_LessThanTwoEvents(t *testing.T) {
	nodes := []agent.Node{
		{ID: "a", Events: []agent.Event{{Timestamp: baseTime}}},
	}
	d := Duration(nodes)
	if d != 0 {
		t.Errorf("expected 0 for single event, got %v", d)
	}
}

func TestDuration_EmptyNodes(t *testing.T) {
	d := Duration(nil)
	if d != 0 {
		t.Errorf("expected 0 for empty nodes, got %v", d)
	}
}

// --- Run ---

func TestRun_EmitsAllEvents(t *testing.T) {
	nodes := []agent.Node{
		{
			ID: "a",
			Events: []agent.Event{
				{Type: "PreToolUse", Timestamp: baseTime},
				{Type: "PostToolUse", Timestamp: baseTime.Add(time.Millisecond)},
				{Type: "Stop", Timestamp: baseTime.Add(2 * time.Millisecond)},
			},
		},
	}

	out := make(chan agent.Event, 10)
	done := make(chan struct{})
	// Use a very high speed so the test completes instantly.
	Run(nodes, out, Options{Speed: 10000, MaxGap: time.Millisecond}, done)

	var received []agent.Event
	for e := range out {
		received = append(received, e)
	}

	if len(received) != 3 {
		t.Fatalf("expected 3 events, got %d", len(received))
	}
}

func TestRun_EventsInTimestampOrder(t *testing.T) {
	nodes := []agent.Node{
		{
			ID: "a",
			Events: []agent.Event{
				{Type: "PostToolUse", Timestamp: baseTime.Add(10 * time.Millisecond)},
			},
		},
		{
			ID: "b",
			Events: []agent.Event{
				{Type: "PreToolUse", Timestamp: baseTime},
			},
		},
	}

	out := make(chan agent.Event, 10)
	done := make(chan struct{})
	Run(nodes, out, Options{Speed: 10000, MaxGap: time.Millisecond}, done)

	var received []agent.Event
	for e := range out {
		received = append(received, e)
	}

	if len(received) != 2 {
		t.Fatalf("expected 2 events, got %d", len(received))
	}
	if received[0].Type != "PreToolUse" {
		t.Errorf("expected PreToolUse first (earlier timestamp), got %q", received[0].Type)
	}
	if received[1].Type != "PostToolUse" {
		t.Errorf("expected PostToolUse second, got %q", received[1].Type)
	}
}

func TestRun_ClosesChannelWhenDone(t *testing.T) {
	out := make(chan agent.Event, 1)
	done := make(chan struct{})
	// Run with empty nodes: channel should be closed immediately.
	Run(nil, out, Options{Speed: 1}, done)

	// Draining a closed channel should give zero events and then return.
	count := 0
	for range out {
		count++
	}
	if count != 0 {
		t.Errorf("expected 0 events for nil nodes, got %d", count)
	}
}

func TestRun_RespectsMaxGap(t *testing.T) {
	// Two events 10 seconds apart; MaxGap = 1ms; Speed = 1.
	// The delay should be capped at MaxGap (1ms), not 10s.
	nodes := []agent.Node{
		{
			ID: "a",
			Events: []agent.Event{
				{Type: "PreToolUse", Timestamp: baseTime},
				{Type: "Stop", Timestamp: baseTime.Add(10 * time.Second)},
			},
		},
	}

	out := make(chan agent.Event, 10)
	done := make(chan struct{})
	start := time.Now()
	Run(nodes, out, Options{Speed: 1, MaxGap: time.Millisecond}, done)
	elapsed := time.Since(start)

	// Should complete well under 1 second since MaxGap caps each delay at 1ms.
	if elapsed > time.Second {
		t.Errorf("expected fast replay with MaxGap cap, took %v", elapsed)
	}
	var count int
	for range out {
		count++
	}
	if count != 2 {
		t.Errorf("expected 2 events, got %d", count)
	}
}

func TestRun_StopsWhenDoneClosed(t *testing.T) {
	// Many events with a real delay — closing done should abort quickly.
	var events []agent.Event
	for i := 0; i < 20; i++ {
		events = append(events, agent.Event{
			Type:      "PreToolUse",
			Timestamp: baseTime.Add(time.Duration(i) * time.Second),
		})
	}
	nodes := []agent.Node{{ID: "a", Events: events}}

	out := make(chan agent.Event, 5)
	done := make(chan struct{})

	go func() {
		// Let a couple events through, then signal done.
		time.Sleep(10 * time.Millisecond)
		close(done)
	}()

	// Speed=1 means real-time delays (1s per gap), so without the done signal
	// this would take 20 seconds. With done, it should stop quickly.
	start := time.Now()
	Run(nodes, out, Options{Speed: 1, MaxGap: 5 * time.Second}, done)
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Errorf("expected early exit after done closed, took %v", elapsed)
	}
}
