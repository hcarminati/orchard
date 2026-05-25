// Package replay provides session replay functionality for Orchard.
// It loads past Claude Code sessions from JSONL files and re-emits their
// events in chronological order, with inter-event delays scaled by a speed
// multiplier. This lets users watch any past session unfold as if it were live.
package replay

import (
	"sort"
	"time"

	"github.com/hcarminati/orchard/internal/agent"
)

// Options controls how a replay runs.
type Options struct {
	// Speed is the playback multiplier. 1.0 = real time, 2.0 = double speed,
	// 0.5 = half speed. Values ≤ 0 default to 1.0.
	Speed float64

	// MaxGap is the maximum inter-event delay regardless of the real gap in
	// the recording. Prevents very long pauses in inactive sessions from making
	// the replay feel broken. Defaults to 3 seconds when zero.
	MaxGap time.Duration
}

// defaultMaxGap is used when Options.MaxGap is zero.
const defaultMaxGap = 3 * time.Second

// Run collects all events from nodes, sorts them by timestamp, and emits them
// to out with scaled delays that reflect the original session timing.
//
// Callers should pass a buffered channel and run this in a goroutine.
// Run closes out when all events have been sent or ctx is cancelled.
func Run(nodes []agent.Node, out chan<- agent.Event, opts Options, done <-chan struct{}) {
	if opts.Speed <= 0 {
		opts.Speed = 1.0
	}
	if opts.MaxGap <= 0 {
		opts.MaxGap = defaultMaxGap
	}

	// Collect all events from every node into a flat slice.
	var events []agent.Event
	for _, n := range nodes {
		events = append(events, n.Events...)
	}

	// Sort strictly by timestamp so replay order is always deterministic.
	sort.SliceStable(events, func(i, j int) bool {
		return events[i].Timestamp.Before(events[j].Timestamp)
	})

	defer close(out)

	var prev time.Time
	for _, e := range events {
		// Compute how long to wait before emitting this event.
		if !prev.IsZero() && !e.Timestamp.IsZero() {
			gap := e.Timestamp.Sub(prev)
			if gap > opts.MaxGap {
				gap = opts.MaxGap
			}
			if gap > 0 {
				scaled := time.Duration(float64(gap) / opts.Speed)
				select {
				case <-done:
					return
				case <-time.After(scaled):
				}
			}
		}
		if !e.Timestamp.IsZero() {
			prev = e.Timestamp
		}

		select {
		case <-done:
			return
		case out <- e:
		}
	}
}

// AllEvents returns all events from nodes sorted by timestamp.
// Useful for computing total replay duration before starting.
func AllEvents(nodes []agent.Node) []agent.Event {
	var events []agent.Event
	for _, n := range nodes {
		events = append(events, n.Events...)
	}
	sort.SliceStable(events, func(i, j int) bool {
		return events[i].Timestamp.Before(events[j].Timestamp)
	})
	return events
}

// Duration returns the wall-clock span of events in nodes (last minus first timestamp).
// Returns 0 when nodes have fewer than 2 events.
func Duration(nodes []agent.Node) time.Duration {
	events := AllEvents(nodes)
	if len(events) < 2 {
		return 0
	}
	first := events[0].Timestamp
	last := events[len(events)-1].Timestamp
	if last.Before(first) {
		return 0
	}
	return last.Sub(first)
}
