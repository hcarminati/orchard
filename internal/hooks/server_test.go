package hooks

import (
	"bytes"
	"encoding/json"
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
	s := NewServer(testCWD, ch, "") // addr unused when using httptest
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handle)
	return httptest.NewServer(mux), ch
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

	select {
	case e := <-ch:
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
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for event")
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
		// good — nothing in the channel
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

	resp := postJSON(t, srv, payload{
		SessionID:     "session-3",
		HookEventName: "Stop",
		CWD:           testCWD,
	})
	resp.Body.Close()

	select {
	case e := <-ch:
		if e.Type != "Stop" {
			t.Errorf("expected Stop event, got %q", e.Type)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for Stop event")
	}
}
