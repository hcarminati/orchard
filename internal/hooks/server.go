// Package hooks implements the embedded HTTP server that receives Claude Code hook events.
// Claude Code fires hooks (PreToolUse, PostToolUse, Stop, SubagentStop, Notification,
// PermissionRequest) by executing scripts; Orchard wires those scripts to POST JSON
// payloads to this server. The server filters events by working directory so Orchard
// only processes events belonging to its own session.
package hooks

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
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
	// Model is the model ID used by the agent for this session (e.g. "claude-sonnet-4-6").
	Model string `json:"model"`
}

// Server is an embedded HTTP server that receives and forwards Claude Code hook events.
type Server struct {
	// cwds is the set of working directories Orchard is watching.
	// Events whose CWD field is not in this set are silently ignored.
	// An empty set accepts events from all directories.
	cwds map[string]bool
	// out is the channel hook events are sent to after parsing and filtering.
	out chan<- agent.Event
	// httpServer is the underlying HTTP server, held so Shutdown can stop it.
	httpServer *http.Server
}

// NewServer creates a Server that listens on addr, filters events by cwds,
// and sends parsed events to out. Pass one or more working directories to
// watch; pass nil or an empty slice to accept events from all directories.
func NewServer(cwds []string, out chan<- agent.Event, addr string) *Server {
	cwdSet := make(map[string]bool, len(cwds))
	for _, c := range cwds {
		cwdSet[c] = true
	}
	s := &Server{cwds: cwdSet, out: out}
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

	// Capture the raw body for debug logging before decoding.
	var rawBody json.RawMessage
	if err := json.NewDecoder(r.Body).Decode(&rawBody); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	var p payload
	if err := json.Unmarshal(rawBody, &p); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// Log PostToolUse and Stop payloads to /tmp/orchard-hooks.log so we can inspect
	// what token usage fields (if any) Claude Code actually sends in these events.
	if p.HookEventName == "PostToolUse" || p.HookEventName == "Stop" {
		pretty, _ := json.MarshalIndent(rawBody, "", "  ")
		line := fmt.Sprintf("=== %s [%s] ===\n%s\n\n", p.HookEventName, time.Now().Format(time.RFC3339), pretty)
		_ = appendToFile("/tmp/orchard-hooks.log", line)
	}

	// Only process events for our watched working directories.
	// An empty set means "watch everything" (e.g. in tests without CWD filtering).
	if len(s.cwds) > 0 && !s.cwds[p.CWD] {
		w.WriteHeader(http.StatusOK)
		return
	}

	// ToolInput is absent when the field is missing or explicitly null in JSON.
	input := ""
	if len(p.ToolInput) > 0 && string(p.ToolInput) != "null" {
		input = string(p.ToolInput)
	}

	eventType := p.HookEventName
	toolName := p.ToolName

	// Skill invocations arrive as PreToolUse[Skill]. Promote them to a first-class
	// SkillTrigger event type and replace the tool name with the skill name so the
	// event log can display them distinctly.
	if eventType == "PreToolUse" && toolName == "Skill" && input != "" {
		var inp struct {
			Skill string `json:"skill"`
		}
		if err := json.Unmarshal([]byte(input), &inp); err == nil && inp.Skill != "" {
			eventType = "SkillTrigger"
			toolName = inp.Skill
		}
	}

	e := agent.Event{
		Type:      eventType,
		SessionID: p.SessionID,
		ParentID:  p.ParentSessionID,
		Tool:      toolName,
		ToolUseID: p.ToolUseID,
		Input:     input,
		Response:  p.ToolResponse,
		Message:   p.Message,
		Model:     agent.ParseModel(p.Model),
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

// appendToFile appends text to path, creating the file if it doesn't exist.
// Used only for debug hook logging; errors are intentionally ignored.
func appendToFile(path, text string) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.WriteString(text)
	return err
}
