// Package hooks implements the embedded HTTP server that receives Claude Code hook events.
// Claude Code fires hooks (PreToolUse, PostToolUse, Stop, SubagentStop, Notification)
// by executing scripts; Orchard wires those scripts to POST JSON payloads to this server.
// The server filters events by working directory so Orchard only processes events
// belonging to its own session.
package hooks

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/hcarminati/orchard/internal/agent"
)

// payload mirrors the JSON body that Claude Code sends for each hook event.
// Fields not needed for v0.2 are omitted.
type payload struct {
	SessionID     string `json:"session_id"`
	HookEventName string `json:"hook_event_name"`
	CWD           string `json:"cwd"`
	ToolName      string `json:"tool_name"`
}

// Server is an embedded HTTP server that receives and forwards Claude Code hook events.
type Server struct {
	// cwd is the working directory Orchard is watching.
	// Events whose CWD field does not match are silently ignored.
	cwd string
	// out is the channel hook events are sent to after parsing and filtering.
	out chan<- agent.Event
	// addr is the TCP address to listen on, e.g. ":7070".
	addr string
}

// NewServer creates a Server that listens on addr, filters events by cwd,
// and sends parsed events to out.
func NewServer(cwd string, out chan<- agent.Event, addr string) *Server {
	return &Server{cwd: cwd, out: out, addr: addr}
}

// Start begins listening for hook events. It blocks until the server encounters
// a fatal error (typically program exit). Callers should run it in a goroutine.
func (s *Server) Start() error {
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handle)
	return http.ListenAndServe(s.addr, mux)
}

// handle processes a single incoming hook POST request.
func (s *Server) handle(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	var p payload
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// Only process events for our working directory.
	if p.CWD != s.cwd {
		w.WriteHeader(http.StatusOK)
		return
	}

	e := agent.Event{
		Type:      p.HookEventName,
		SessionID: p.SessionID,
		Tool:      p.ToolName,
		Timestamp: time.Now(),
	}

	// Non-blocking send: drop the event if the consumer is behind rather than
	// stalling the HTTP handler (and therefore Claude Code's hook execution).
	select {
	case s.out <- e:
	default:
	}

	w.WriteHeader(http.StatusOK)
}
