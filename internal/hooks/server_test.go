package hooks

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/hcarminati/orchard/internal/agent"
)

const testCWD = "/Users/test/myproject"

// newTestServer returns a Server and a buffered event channel wired together,
// using httptest.NewServer so we don't need a real TCP port.
func newTestServer(t *testing.T) (*httptest.Server, chan agent.Event) {
	t.Helper()
	ch := make(chan agent.Event, 10)
	s := NewServer([]string{testCWD}, ch, "")
	return httptest.NewServer(s.Handler()), ch
}

func postJSON(t *testing.T, srv *httptest.Server, body any) *http.Response {
	t.Helper()
	b, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	resp, err := http.Post(srv.URL+"/", "application/json", bytes.NewReader(b))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	return resp
}

// receiveEvent blocks until an event arrives on ch or the test times out.
func receiveEvent(t *testing.T, ch chan agent.Event) agent.Event {
	t.Helper()
	select {
	case e := <-ch:
		return e
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for event")
		return agent.Event{} // unreachable
	}
}

func TestHandle_ValidEvent_DeliveredToChannel(t *testing.T) {
	srv, ch := newTestServer(t)
	defer srv.Close()

	resp := postJSON(t, srv, payload{
		SessionID:     "session-1",
		HookEventName: "PreToolUse",
		CWD:           testCWD,
		ToolName:      "Bash",
	})
	resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	e := receiveEvent(t, ch)
	if e.Type != "PreToolUse" {
		t.Errorf("Type: got %q, want PreToolUse", e.Type)
	}
	if e.SessionID != "session-1" {
		t.Errorf("SessionID: got %q, want session-1", e.SessionID)
	}
	if e.Tool != "Bash" {
		t.Errorf("Tool: got %q, want Bash", e.Tool)
	}
	if e.Timestamp.IsZero() {
		t.Error("expected non-zero Timestamp")
	}
}

func TestHandle_WrongCWD_EventDropped(t *testing.T) {
	srv, ch := newTestServer(t)
	defer srv.Close()

	resp := postJSON(t, srv, payload{
		SessionID:     "session-2",
		HookEventName: "Stop",
		CWD:           "/some/other/project",
	})
	resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	select {
	case e := <-ch:
		t.Errorf("unexpected event delivered: %+v", e)
	default:
	}
}

func TestHandle_MalformedJSON_Returns400(t *testing.T) {
	srv, _ := newTestServer(t)
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/", "application/json", bytes.NewBufferString("{bad json"))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", resp.StatusCode)
	}
}

func TestHandle_GetMethod_Returns405(t *testing.T) {
	srv, _ := newTestServer(t)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", resp.StatusCode)
	}
}

func TestHandle_StopEvent_Delivered(t *testing.T) {
	srv, ch := newTestServer(t)
	defer srv.Close()

	postJSON(t, srv, payload{SessionID: "session-3", HookEventName: "Stop", CWD: testCWD}).Body.Close()

	e := receiveEvent(t, ch)
	if e.Type != "Stop" {
		t.Errorf("Type: got %q, want Stop", e.Type)
	}
}

func TestHandle_PostToolUse_InputAndResponseDelivered(t *testing.T) {
	srv, ch := newTestServer(t)
	defer srv.Close()

	resp := postJSON(t, srv, payload{
		SessionID:     "session-4",
		HookEventName: "PostToolUse",
		CWD:           testCWD,
		ToolName:      "Read",
		ToolInput:     json.RawMessage(`{"file_path":"/tmp/foo"}`),
		ToolResponse:  "file contents",
	})
	resp.Body.Close()

	e := receiveEvent(t, ch)
	if e.Type != "PostToolUse" {
		t.Errorf("Type: got %q, want PostToolUse", e.Type)
	}
	if e.Tool != "Read" {
		t.Errorf("Tool: got %q, want Read", e.Tool)
	}
	if e.Input != `{"file_path":"/tmp/foo"}` {
		t.Errorf("Input: got %q, want {\"file_path\":\"/tmp/foo\"}", e.Input)
	}
	if e.Response != "file contents" {
		t.Errorf("Response: got %q, want \"file contents\"", e.Response)
	}
}

func TestHandle_SubagentStop_Delivered(t *testing.T) {
	srv, ch := newTestServer(t)
	defer srv.Close()

	postJSON(t, srv, payload{SessionID: "session-5", HookEventName: "SubagentStop", CWD: testCWD}).Body.Close()

	e := receiveEvent(t, ch)
	if e.Type != "SubagentStop" {
		t.Errorf("Type: got %q, want SubagentStop", e.Type)
	}
	if e.SessionID != "session-5" {
		t.Errorf("SessionID: got %q, want session-5", e.SessionID)
	}
}

func TestHandle_Notification_MessageDelivered(t *testing.T) {
	srv, ch := newTestServer(t)
	defer srv.Close()

	resp := postJSON(t, srv, payload{
		SessionID:     "session-6",
		HookEventName: "Notification",
		CWD:           testCWD,
		Message:       "agent is waiting for user input",
	})
	resp.Body.Close()

	e := receiveEvent(t, ch)
	if e.Type != "Notification" {
		t.Errorf("Type: got %q, want Notification", e.Type)
	}
	if e.Message != "agent is waiting for user input" {
		t.Errorf("Message: got %q, want \"agent is waiting for user input\"", e.Message)
	}
}

func TestServer_GracefulShutdown(t *testing.T) {
	ch := make(chan agent.Event, 1)
	s := NewServer([]string{testCWD}, ch, "")

	// Pre-bind the listener so the port is guaranteed ready before we call Shutdown.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	go func() { _ = s.StartOn(ln) }()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := s.Shutdown(ctx); err != nil {
		t.Errorf("Shutdown returned error: %v", err)
	}
}

func TestHandle_ParentSessionID_PropagatedToEvent(t *testing.T) {
	srv, ch := newTestServer(t)
	defer srv.Close()

	postJSON(t, srv, payload{
		SessionID:       "child-session",
		ParentSessionID: "parent-session",
		HookEventName:   "PreToolUse",
		CWD:             testCWD,
		ToolName:        "Bash",
	}).Body.Close()

	e := receiveEvent(t, ch)
	if e.ParentID != "parent-session" {
		t.Errorf("expected ParentID='parent-session', got %q", e.ParentID)
	}
	if e.SessionID != "child-session" {
		t.Errorf("expected SessionID='child-session', got %q", e.SessionID)
	}
}

func TestHandle_SkillTrigger_PromotedFromPreToolUse(t *testing.T) {
	srv, ch := newTestServer(t)
	defer srv.Close()

	postJSON(t, srv, payload{
		SessionID:     "session-skill",
		HookEventName: "PreToolUse",
		CWD:           testCWD,
		ToolName:      "Skill",
		ToolInput:     json.RawMessage(`{"skill":"build","args":""}`),
	}).Body.Close()

	e := receiveEvent(t, ch)
	if e.Type != "SkillTrigger" {
		t.Errorf("Type: got %q, want SkillTrigger", e.Type)
	}
	if e.Tool != "build" {
		t.Errorf("Tool: got %q, want build (skill name extracted from input)", e.Tool)
	}
	if e.Input == "" {
		t.Error("expected Input to be preserved on SkillTrigger event")
	}
}

func TestHandle_OversizedBody_Returns400(t *testing.T) {
	srv, _ := newTestServer(t)
	defer srv.Close()

	oversized := make([]byte, maxBodyBytes+1)
	for i := range oversized {
		oversized[i] = 'x'
	}
	resp, err := http.Post(srv.URL+"/", "application/json", bytes.NewReader(oversized))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400 for oversized body, got %d", resp.StatusCode)
	}
}

func TestHandle_SkillTrigger_NoSkillField_PassesThroughAsPreToolUse(t *testing.T) {
	// If the Skill tool input does not contain a "skill" field, the event should
	// remain as a regular PreToolUse rather than being silently dropped.
	srv, ch := newTestServer(t)
	defer srv.Close()

	postJSON(t, srv, payload{
		SessionID:     "session-skill2",
		HookEventName: "PreToolUse",
		CWD:           testCWD,
		ToolName:      "Skill",
		ToolInput:     json.RawMessage(`{"name":"other"}`),
	}).Body.Close()

	e := receiveEvent(t, ch)
	if e.Type != "PreToolUse" {
		t.Errorf("Type: got %q, want PreToolUse (no skill field = no promotion)", e.Type)
	}
	if e.Tool != "Skill" {
		t.Errorf("Tool: got %q, want Skill", e.Tool)
	}
}

func TestHandle_ModelParsedAndForwarded(t *testing.T) {
	tests := []struct {
		rawModel  string
		wantModel agent.Model
	}{
		{"claude-sonnet-4-6", agent.ModelSonnet},
		{"claude-haiku-4-5-20251001", agent.ModelHaiku},
		{"claude-opus-4-6", agent.ModelOpus},
		{"", agent.ModelUnknown},
		{"some-future-model", agent.Model("some-future-model")},
	}

	for _, tc := range tests {
		srv, ch := newTestServer(t)

		postJSON(t, srv, payload{
			SessionID:     "session-model",
			HookEventName: "PreToolUse",
			CWD:           testCWD,
			ToolName:      "Bash",
			Model:         tc.rawModel,
		}).Body.Close()

		e := receiveEvent(t, ch)
		if e.Model != tc.wantModel {
			t.Errorf("rawModel=%q: got Model=%q, want %q", tc.rawModel, e.Model, tc.wantModel)
		}

		srv.Close()
	}
}

const testCWD2 = "/Users/test/otherproject"

func TestNewServer_MultiCWD_AcceptsAllWatchedDirs(t *testing.T) {
	ch := make(chan agent.Event, 10)
	s := NewServer([]string{testCWD, testCWD2}, ch, "")
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	// Post event from first watched dir.
	postJSON(t, srv, payload{SessionID: "s1", HookEventName: "PreToolUse", CWD: testCWD, ToolName: "Bash"}).Body.Close()
	e1 := receiveEvent(t, ch)
	if e1.SessionID != "s1" {
		t.Errorf("expected s1, got %q", e1.SessionID)
	}

	// Post event from second watched dir.
	postJSON(t, srv, payload{SessionID: "s2", HookEventName: "PreToolUse", CWD: testCWD2, ToolName: "Grep"}).Body.Close()
	e2 := receiveEvent(t, ch)
	if e2.SessionID != "s2" {
		t.Errorf("expected s2, got %q", e2.SessionID)
	}
}

func TestNewServer_MultiCWD_RejectsUnwatchedDir(t *testing.T) {
	ch := make(chan agent.Event, 10)
	s := NewServer([]string{testCWD, testCWD2}, ch, "")
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	resp := postJSON(t, srv, payload{
		SessionID:     "s-other",
		HookEventName: "PreToolUse",
		CWD:           "/some/other/path",
		ToolName:      "Bash",
	})
	resp.Body.Close()

	// Channel should remain empty since the CWD is not in our watch list.
	select {
	case e := <-ch:
		t.Errorf("unexpected event for unwatched CWD: %v", e)
	default:
		// expected: no event
	}
}

func TestNewServer_EmptyCWDList_AcceptsAll(t *testing.T) {
	ch := make(chan agent.Event, 10)
	s := NewServer(nil, ch, "")
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	postJSON(t, srv, payload{SessionID: "s1", HookEventName: "PreToolUse", CWD: "/any/path", ToolName: "Bash"}).Body.Close()
	e := receiveEvent(t, ch)
	if e.SessionID != "s1" {
		t.Errorf("expected s1, got %q", e.SessionID)
	}
}
