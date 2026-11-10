package mcp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/metavert-io/lightcms/v7/internal/apiclient"
)

// sandboxAPI is a minimal fake REST API that keeps the sandbox the way the
// server does: on the credential, outside any MCP session.
type sandboxAPI struct {
	mu          sync.Mutex
	active      bool // a sandbox is open
	restricted  bool // the credential is set to Sandbox access
	forkCreated bool
	forkDeleted bool
	forkPages   map[string]string // path -> fork page ID
	updates     map[string]int    // content ID -> update count
	diffFails   bool              // when true, the fork diff endpoint returns 500
	deleteFails bool              // when true, discarding the sandbox returns 500
}

func (state *sandboxAPI) sandboxJSON() map[string]interface{} {
	resp := map[string]interface{}{"active": state.active, "restricted": state.restricted}
	if state.active {
		resp["fork_id"] = "fork1"
		resp["name"] = "agent-session"
		resp["review_url"] = "/cm/forks/fork1"
	}
	return resp
}

func newSandboxServer(t *testing.T) (*Server, *sandboxAPI, func()) {
	t.Helper()
	state := &sandboxAPI{forkPages: map[string]string{}, updates: map[string]int{}}
	ts := httptest.NewServer(sandboxMux(state))
	return NewServer(apiclient.New(ts.URL, "test-key")), state, ts.Close
}

func sandboxMux(state *sandboxAPI) *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/sandbox", func(w http.ResponseWriter, r *http.Request) {
		state.mu.Lock()
		defer state.mu.Unlock()
		json.NewEncoder(w).Encode(state.sandboxJSON())
	})
	mux.HandleFunc("POST /api/v1/sandbox", func(w http.ResponseWriter, r *http.Request) {
		state.mu.Lock()
		defer state.mu.Unlock()
		if state.active {
			w.WriteHeader(http.StatusConflict)
			json.NewEncoder(w).Encode(map[string]string{"error": `a sandbox ("agent-session") is already open; end it first`})
			return
		}
		state.active, state.forkCreated = true, true
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(state.sandboxJSON())
	})
	mux.HandleFunc("POST /api/v1/sandbox/end", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]string
		json.NewDecoder(r.Body).Decode(&body)
		state.mu.Lock()
		defer state.mu.Unlock()
		if body["action"] == "discard" {
			if state.deleteFails {
				http.Error(w, `{"error":"cannot delete"}`, http.StatusInternalServerError)
				return
			}
			state.forkDeleted = true
		}
		state.active = false
		json.NewEncoder(w).Encode(map[string]interface{}{"action": body["action"], "fork_id": "fork1", "restricted": state.restricted})
	})
	// The server refuses live changes from a sandboxed caller.
	mux.HandleFunc("POST /api/v1/content/live1/publish", func(w http.ResponseWriter, r *http.Request) {
		state.mu.Lock()
		active := state.active || state.restricted
		state.mu.Unlock()
		if active {
			w.WriteHeader(http.StatusForbidden)
			json.NewEncoder(w).Encode(map[string]string{"error": "this connection is limited to its own forks"})
			return
		}
		json.NewEncoder(w).Encode(map[string]interface{}{"success": true})
	})
	mux.HandleFunc("GET /api/v1/forks/fork1/diff", func(w http.ResponseWriter, r *http.Request) {
		state.mu.Lock()
		fails := state.diffFails
		state.mu.Unlock()
		if fails {
			http.Error(w, `{"error":"diff unavailable"}`, http.StatusInternalServerError)
			return
		}
		json.NewEncoder(w).Encode(map[string]interface{}{
			"fork_id": "fork1",
			"pages": []map[string]interface{}{
				{"path": "/live-page", "title": "Live Page", "status": "modified",
					"fields": []map[string]string{{"name": "title", "live": "Old", "fork": "New"}}},
			},
		})
	})
	mux.HandleFunc("POST /api/v1/forks/fork1/fork-page", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]string
		json.NewDecoder(r.Body).Decode(&body)
		path := body["path"]
		if path == "/boom" || body["content_id"] == "liveboom" {
			http.Error(w, `{"error":"cannot fork this page"}`, http.StatusInternalServerError)
			return
		}
		if path == "" {
			path = "/live-page" // fork by content_id resolves to the live page
		}
		state.mu.Lock()
		id, ok := state.forkPages[path]
		if !ok {
			id = "fp-" + strings.TrimPrefix(path, "/")
			state.forkPages[path] = id
		}
		state.mu.Unlock()
		json.NewEncoder(w).Encode(map[string]interface{}{"id": id, "full_path": path, "title": "Fork copy"})
	})
	mux.HandleFunc("GET /api/v1/content/live1", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"id": "live1", "title": "Live Page", "full_path": "/live-page", "published": true,
			"data": map[string]interface{}{"body": "old"},
		})
	})
	mux.HandleFunc("GET /api/v1/content/liveboom", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"id": "liveboom", "title": "Unforkable", "full_path": "/boom", "published": true,
			"data": map[string]interface{}{},
		})
	})
	mux.HandleFunc("GET /api/v1/content/otherfork1", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"id": "otherfork1", "title": "Other Fork Copy", "full_path": "/other",
			"fork_id": "someone-elses-fork", "data": map[string]interface{}{},
		})
	})
	mux.HandleFunc("GET /api/v1/content/missing1", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(404)
		json.NewEncoder(w).Encode(map[string]string{"error": "not found"})
	})
	mux.HandleFunc("GET /api/v1/content/fp-live-page", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"id": "fp-live-page", "title": "Live Page", "full_path": "/live-page",
			"fork_id": "fork1", "data": map[string]interface{}{"body": "old"},
		})
	})
	mux.HandleFunc("PUT /api/v1/content/", func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimPrefix(r.URL.Path, "/api/v1/content/")
		state.mu.Lock()
		state.updates[id]++
		state.mu.Unlock()
		json.NewEncoder(w).Encode(map[string]interface{}{"id": id, "title": "Updated", "full_path": "/live-page"})
	})
	mux.HandleFunc("POST /api/v1/content", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]interface{}
		json.NewDecoder(r.Body).Decode(&body)
		forkID, _ := body["fork_id"].(string)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"id": "new1", "title": body["title"], "full_path": "/new-page", "fork_id": forkID,
		})
	})

	return mux
}

func TestSandbox_Lifecycle(t *testing.T) {
	s, state, cleanup := newSandboxServer(t)
	defer cleanup()

	// No sandbox: status reports inactive.
	res := callTool(t, s, "get_agent_sandbox", struct{}{})
	if !strings.Contains(resultText(t, res), "No agent sandbox") {
		t.Errorf("expected inactive status, got %s", resultText(t, res))
	}

	// Start.
	res = callTool(t, s, "start_agent_sandbox", map[string]string{"name": "test-session"})
	if !strings.Contains(resultText(t, res), "fork1") {
		t.Fatalf("start: %s", resultText(t, res))
	}
	if !state.forkCreated {
		t.Fatal("fork was not created")
	}
	if _, _, active := s.sandboxFork(context.Background()); !active {
		t.Fatal("sandbox not active after start")
	}

	// Double-start is refused.
	res = callTool(t, s, "start_agent_sandbox", map[string]string{})
	if !strings.Contains(resultText(t, res), "already open") {
		t.Errorf("double start: %s", resultText(t, res))
	}

	// Status includes diff.
	res = callTool(t, s, "get_agent_sandbox", struct{}{})
	if !strings.Contains(resultText(t, res), "modified") {
		t.Errorf("status should include changes: %s", resultText(t, res))
	}

	// Submit ends the session and returns the review URL.
	res = callTool(t, s, "end_agent_sandbox", map[string]string{"action": "submit"})
	txt := resultText(t, res)
	if !strings.Contains(txt, "/cm/forks/fork1") {
		t.Errorf("submit should include review URL: %s", txt)
	}
	if _, _, active := s.sandboxFork(context.Background()); active {
		t.Fatal("sandbox still active after submit")
	}
}

// The sandbox lives on the server, so a new MCP session (or a restarted
// server) carries on in the same fork.
func TestSandbox_ContinuesInANewSession(t *testing.T) {
	state := &sandboxAPI{forkPages: map[string]string{}, updates: map[string]int{}}
	ts := httptest.NewServer(sandboxMux(state))
	defer ts.Close()

	first := NewServer(apiclient.New(ts.URL, "test-key"))
	callTool(t, first, "start_agent_sandbox", map[string]string{})

	second := NewServer(apiclient.New(ts.URL, "test-key"))
	if _, _, active := second.sandboxFork(context.Background()); !active {
		t.Fatal("a new session does not see the open sandbox")
	}
	callTool(t, second, "update_content", map[string]interface{}{"id": "live1", "title": "From the new session"})
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.updates["live1"] != 0 || state.updates["fp-live-page"] != 1 {
		t.Errorf("live=%d fork=%d, want 0/1", state.updates["live1"], state.updates["fp-live-page"])
	}
}

func TestSandbox_StatusSaysWhenTheConnectionIsSandboxOnly(t *testing.T) {
	s, state, cleanup := newSandboxServer(t)
	defer cleanup()
	state.restricted = true

	res := callTool(t, s, "get_agent_sandbox", struct{}{})
	if !strings.Contains(resultText(t, res), "set to Sandbox access") {
		t.Errorf("status: %s", resultText(t, res))
	}
	res = callTool(t, s, "start_agent_sandbox", map[string]string{})
	callTool(t, s, "end_agent_sandbox", map[string]string{"action": "discard"})
	res = callTool(t, s, "start_agent_sandbox", map[string]string{})
	res = callTool(t, s, "end_agent_sandbox", map[string]string{"action": "discard"})
	if !strings.Contains(resultText(t, res), "can still change only forks") {
		t.Errorf("discard on a sandbox-only connection: %s", resultText(t, res))
	}
}

func TestSandbox_CopyOnWriteUpdate(t *testing.T) {
	s, state, cleanup := newSandboxServer(t)
	defer cleanup()

	callTool(t, s, "start_agent_sandbox", map[string]string{})

	// Updating a live page must hit the fork copy, not the live document.
	res := callTool(t, s, "update_content", map[string]interface{}{
		"id": "live1", "title": "New Title", "version_comment": "sandbox edit",
	})
	if strings.Contains(resultText(t, res), "error") {
		t.Fatalf("update failed: %s", resultText(t, res))
	}
	state.mu.Lock()
	liveUpdates, forkUpdates := state.updates["live1"], state.updates["fp-live-page"]
	state.mu.Unlock()
	if liveUpdates != 0 {
		t.Errorf("live content was updated %d times — sandbox must protect it", liveUpdates)
	}
	if forkUpdates != 1 {
		t.Errorf("fork copy updates = %d, want 1", forkUpdates)
	}

	// By-path update also lands on the fork copy.
	callTool(t, s, "update_content_by_path", map[string]interface{}{
		"path": "/live-page", "title": "Another Title",
	})
	state.mu.Lock()
	liveUpdates, forkUpdates = state.updates["live1"], state.updates["fp-live-page"]
	state.mu.Unlock()
	if liveUpdates != 0 || forkUpdates != 2 {
		t.Errorf("after by-path: live=%d fork=%d, want 0/2", liveUpdates, forkUpdates)
	}
}

func TestSandbox_CreateGoesIntoFork(t *testing.T) {
	s, _, cleanup := newSandboxServer(t)
	defer cleanup()

	callTool(t, s, "start_agent_sandbox", map[string]string{})
	res := callTool(t, s, "create_content", map[string]interface{}{
		"template_id": "t1", "title": "Brand New", "slug": "new-page",
		"data": map[string]interface{}{"body": "x"},
	})
	if !strings.Contains(resultText(t, res), "new1") {
		t.Fatalf("create: %s", resultText(t, res))
	}

	// Upsert is refused in sandbox mode.
	res = callTool(t, s, "create_content", map[string]interface{}{
		"template_id": "t1", "title": "Up", "slug": "up", "upsert": true,
		"data": map[string]interface{}{},
	})
	if !strings.Contains(resultText(t, res), "not available") {
		t.Errorf("upsert should be blocked: %s", resultText(t, res))
	}
}

// Tools no longer refuse anything themselves: the server does, for every
// client, and its reason reaches the agent.
func TestSandbox_LiveChangesAreRefusedByTheServer(t *testing.T) {
	s, _, cleanup := newSandboxServer(t)
	defer cleanup()

	if res := callTool(t, s, "publish_content", map[string]string{"id": "live1"}); res.IsError {
		t.Fatalf("without a sandbox: %s", resultText(t, res))
	}
	callTool(t, s, "start_agent_sandbox", map[string]string{})
	res := callTool(t, s, "publish_content", map[string]string{"id": "live1"})
	if !res.IsError || !strings.Contains(resultText(t, res), "limited to its own forks") {
		t.Errorf("publish in a sandbox: %v %s", res.IsError, resultText(t, res))
	}
}

func TestSandbox_DiscardDeletesFork(t *testing.T) {
	s, state, cleanup := newSandboxServer(t)
	defer cleanup()

	callTool(t, s, "start_agent_sandbox", map[string]string{})
	res := callTool(t, s, "end_agent_sandbox", map[string]string{"action": "discard"})
	if !strings.Contains(resultText(t, res), "discarded") {
		t.Fatalf("discard: %s", resultText(t, res))
	}
	if !state.forkDeleted {
		t.Error("fork was not deleted on discard")
	}
	if _, _, active := s.sandboxFork(context.Background()); active {
		t.Error("sandbox still active after discard")
	}
}

func TestGovernanceAndMaintenanceTools(t *testing.T) {
	state := &sandboxAPI{forkPages: map[string]string{}, updates: map[string]int{}}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/agent-sessions/", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"session_id": "s1", "entries": 2,
			"content_items": []map[string]interface{}{{"content_id": "c1", "actions": []string{"content.update"}}},
		})
	})
	mux.HandleFunc("POST /api/v1/agent-sessions/", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{"session_id": "s1", "reverted": []string{"c1"}})
	})
	mux.HandleFunc("GET /api/v1/maintenance/report", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{"page_count": 5, "stale_pages": []interface{}{}})
	})
	mux.HandleFunc("POST /api/v1/maintenance/scan", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{"page_count": 5, "link_job_id": "j1"})
	})
	_ = state

	ts := httptest.NewServer(mux)
	defer ts.Close()
	client := apiclient.New(ts.URL, "k")
	client.SetAgentSession("agent-default-1")
	s := NewServer(client)

	// Session tools default to the client's own session.
	out := resultText(t, callTool(t, s, "get_agent_session_changes", struct{}{}))
	if !strings.Contains(out, "content.update") {
		t.Errorf("get_agent_session_changes: %s", out)
	}
	out = resultText(t, callTool(t, s, "rollback_agent_session", struct{}{}))
	if !strings.Contains(out, "reverted") {
		t.Errorf("rollback_agent_session: %s", out)
	}
	// Explicit session ID also works.
	out = resultText(t, callTool(t, s, "get_agent_session_changes", map[string]string{"session_id": "s1"}))
	if !strings.Contains(out, "s1") {
		t.Errorf("explicit session: %s", out)
	}

	// Maintenance tools.
	out = resultText(t, callTool(t, s, "get_maintenance_report", struct{}{}))
	if !strings.Contains(out, "page_count") {
		t.Errorf("get_maintenance_report: %s", out)
	}
	out = resultText(t, callTool(t, s, "run_maintenance_scan", map[string]bool{"link_check": true}))
	if !strings.Contains(out, "j1") {
		t.Errorf("run_maintenance_scan: %s", out)
	}

	// No session anywhere → helpful message.
	client2 := apiclient.New(ts.URL, "k")
	s2 := NewServer(client2)
	out = resultText(t, callTool(t, s2, "get_agent_session_changes", struct{}{}))
	if !strings.Contains(out, "no session_id") {
		t.Errorf("missing session message: %s", out)
	}
	out = resultText(t, callTool(t, s2, "rollback_agent_session", struct{}{}))
	if !strings.Contains(out, "no session_id") {
		t.Errorf("rollback missing session message: %s", out)
	}
}
