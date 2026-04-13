// Package hooks implements the embedded HTTP server that receives Claude Code hook events.
// Claude Code fires hooks (PreToolUse, PostToolUse, Stop, SubagentStop, Notification,
// PermissionRequest) by executing scripts; Orchard wires those scripts to POST JSON
// payloads to this server. The server filters events by working directory so Orchard
// only processes events belonging to its own session.
package hooks

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"time"

	"github.com/hcarminati/orchard/internal/agent"
)

// maxBodyBytes caps the request body size. Hook payloads are never larger than
// a few KB; this prevents a local process from OOM-ing Orchard via the hook port.
const maxBodyBytes = 1 << 20 // 1 MiB

// payload mirrors the JSON body that Claude Code sends for each hook event.
// All six event types share this structure; unused fields are zero-valued.
type payload struct {
	SessionID string `json:"session_id"`
	// ParentSessionID is the session ID of the parent agent that spawned this one.
	// Present in hook events fired by subagent sessions.
	ParentSessionID string `json:"parent_session_id"`
	HookEventName   string `json:"hook_event_name"`
	CWD             string `json:"cwd"`
	ToolName        string `json:"tool_name"`
	// ToolUseID is the unique identifier for this tool call instance.
	// Used to correlate PreToolUse[Agent] events with their subagent child nodes.
	ToolUseID string `json:"tool_use_id"`
	// ToolInput is the raw JSON object describing what the tool was called with.
	// Present for PreToolUse and PostToolUse.
	ToolInput json.RawMessage `json:"tool_input"`
	// ToolResponse is the tool's output. Present for PostToolUse.
	ToolResponse string `json:"tool_response"`
	// Message is the notification text. Present for Notification events.
	Message string `json:"message"`
}

// Server is an embedded HTTP server that receives and forwards Claude Code hook events.
type Server struct {
	// cwd is the working directory Orchard is watching.
	// Events whose CWD field does not match are silently ignored.
	cwd string
	// out is the channel hook events are sent to after parsing and filtering.
	out chan<- agent.Event
	// httpServer is the underlying HTTP server, held so Shutdown can stop it.
	httpServer *http.Server
}

// NewServer creates a Server that listens on addr, filters events by cwd,
// and sends parsed events to out.
func NewServer(cwd string, out chan<- agent.Event, addr string) *Server {
	s := &Server{cwd: cwd, out: out}
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handle)
	s.httpServer = &http.Server{Addr: addr, Handler: mux}
	return s
}

// Handler returns the server's http.Handler, useful for testing with httptest.NewServer.
func (s *Server) Handler() http.Handler {
	return s.httpServer.Handler
}

// Start begins listening for hook events. It blocks until Shutdown is called or
// the server encounters a fatal error. Callers should run it in a goroutine.
// http.ErrServerClosed is returned on clean shutdown and should not be treated as an error.
func (s *Server) Start() error {
	return s.httpServer.ListenAndServe()
}

// StartOn begins serving on the provided listener. Useful in tests where the
// listener is pre-bound to guarantee the port is ready before returning.
func (s *Server) StartOn(l net.Listener) error {
	return s.httpServer.Serve(l)
}

// Shutdown gracefully stops the HTTP server, waiting for in-flight requests to
// complete or until ctx is cancelled.
func (s *Server) Shutdown(ctx context.Context) error {
	return s.httpServer.Shutdown(ctx)
}

// handle processes a single incoming hook POST request.
func (s *Server) handle(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	defer r.Body.Close()

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

	// ToolInput is absent when the field is missing or explicitly null in JSON.
	input := ""
	if len(p.ToolInput) > 0 && string(p.ToolInput) != "null" {
		input = string(p.ToolInput)
	}

	e := agent.Event{
		Type:      p.HookEventName,
		SessionID: p.SessionID,
		ParentID:  p.ParentSessionID,
		Tool:      p.ToolName,
		ToolUseID: p.ToolUseID,
		Input:     input,
		Response:  p.ToolResponse,
		Message:   p.Message,
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
