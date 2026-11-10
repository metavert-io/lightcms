package mcp

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/metavert-io/lightcms/v7/internal/apiclient"
)

// recordedCall is one REST request made by an MCP tool.
type recordedCall struct {
	Method string
	Path   string // path + raw query
	Body   map[string]interface{}
}

// recordingServer returns an MCP Server whose REST backend records every
// request and answers with the canned JSON registered for "METHOD /path"
// (query string ignored), or `null` when none is registered.
func recordingServer(t *testing.T, replies map[string]string) (*Server, func() []recordedCall) {
	t.Helper()
	var mu sync.Mutex
	var calls []recordedCall
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		var body map[string]interface{}
		_ = json.Unmarshal(raw, &body)
		mu.Lock()
		calls = append(calls, recordedCall{Method: r.Method, Path: r.URL.RequestURI(), Body: body})
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		if reply, ok := replies[r.Method+" "+r.URL.Path]; ok {
			_, _ = w.Write([]byte(reply))
			return
		}
		_, _ = w.Write([]byte("null"))
	}))
	t.Cleanup(srv.Close)
	return NewServer(apiclient.New(srv.URL, "test-key")), func() []recordedCall {
		mu.Lock()
		defer mu.Unlock()
		return append([]recordedCall(nil), calls...)
	}
}

// lastCall returns the most recent recorded request.
func lastCall(t *testing.T, calls func() []recordedCall) recordedCall {
	t.Helper()
	all := calls()
	if len(all) == 0 {
		t.Fatal("no REST call was made")
	}
	return all[len(all)-1]
}

func TestDraftSafety_IncludeForksPassThrough(t *testing.T) {
	s, calls := recordingServer(t, nil)

	// list_content: off by default, forwarded when asked for.
	callTool(t, s, "list_content", map[string]interface{}{})
	if c := lastCall(t, calls); strings.Contains(c.Path, "include_forks") {
		t.Errorf("list_content default sent include_forks: %s", c.Path)
	}
	callTool(t, s, "list_content", map[string]interface{}{"include_forks": true})
	if c := lastCall(t, calls); !strings.Contains(c.Path, "include_forks=true") {
		t.Errorf("list_content include_forks not forwarded: %s", c.Path)
	}
	callTool(t, s, "list_content", map[string]interface{}{"include_forks": true, "limit": 10})
	if c := lastCall(t, calls); !strings.Contains(c.Path, "include_forks=true") || !strings.Contains(c.Path, "limit=10") {
		t.Errorf("paginated list_content include_forks not forwarded: %s", c.Path)
	}

	// search_content: same.
	callTool(t, s, "search_content", map[string]interface{}{"query": "x"})
	if c := lastCall(t, calls); strings.Contains(c.Path, "include_forks") {
		t.Errorf("search_content default sent include_forks: %s", c.Path)
	}
	callTool(t, s, "search_content", map[string]interface{}{"query": "x", "include_forks": true})
	if c := lastCall(t, calls); !strings.HasPrefix(c.Path, "/api/v1/search?") || !strings.Contains(c.Path, "include_forks=true") {
		t.Errorf("search_content include_forks not forwarded: %s", c.Path)
	}
}

func TestDraftSafety_ListContentShowsHoldAndFork(t *testing.T) {
	s, _ := recordingServer(t, map[string]string{
		"GET /api/v1/content": `[
			{"id":"a","title":"Held","full_path":"/held","hold":true,"updated_at":"2026-10-07T00:00:00Z"},
			{"id":"b","title":"Copy","full_path":"/p","fork_id":"f1","updated_at":"2026-10-07T00:00:00Z"}]`,
	})
	text := resultText(t, callTool(t, s, "list_content", map[string]interface{}{"include_forks": true}))
	if !strings.Contains(text, `"hold": true`) && !strings.Contains(text, `"hold":true`) {
		t.Errorf("list_content summary lacks the hold flag: %s", text)
	}
	if !strings.Contains(text, `"f1"`) {
		t.Errorf("list_content summary lacks fork_id for the fork copy: %s", text)
	}
}

func TestDraftSafety_HoldFlagOnCreateAndUpdate(t *testing.T) {
	s, calls := recordingServer(t, map[string]string{
		"POST /api/v1/content":   `{"id":"c1","title":"T","full_path":"/t","hold":true}`,
		"PUT /api/v1/content/c1": `{"id":"c1","title":"T","full_path":"/t"}`,
		"GET /api/v1/content/c1": `{"id":"c1","title":"T","full_path":"/t","data":{}}`,
	})

	// create_content forwards hold only when set.
	callTool(t, s, "create_content", map[string]interface{}{
		"template_id": "t1", "title": "T", "slug": "t", "data": map[string]interface{}{}, "hold": true,
	})
	if c := lastCall(t, calls); c.Method != "POST" || c.Body["hold"] != true {
		t.Errorf("create_content hold not sent: %s %s %v", c.Method, c.Path, c.Body)
	}
	callTool(t, s, "create_content", map[string]interface{}{
		"template_id": "t1", "title": "T", "slug": "t", "data": map[string]interface{}{},
	})
	if c := lastCall(t, calls); c.Body["hold"] != nil {
		t.Errorf("create_content sent hold when not asked: %v", c.Body)
	}

	// update_content: nullable — true sets, false clears, absent leaves it.
	putBody := func() map[string]interface{} {
		t.Helper()
		all := calls()
		for i := len(all) - 1; i >= 0; i-- {
			if all[i].Method == "PUT" {
				return all[i].Body
			}
		}
		t.Fatal("no PUT recorded")
		return nil
	}
	callTool(t, s, "update_content", map[string]interface{}{"id": "c1", "hold": true})
	if b := putBody(); b["hold"] != true {
		t.Errorf("update_content hold=true not sent: %v", b)
	}
	callTool(t, s, "update_content", map[string]interface{}{"id": "c1", "hold": false})
	if b := putBody(); b["hold"] != false {
		t.Errorf("update_content hold=false not sent: %v", b)
	}
	callTool(t, s, "update_content", map[string]interface{}{"id": "c1", "title": "New"})
	if b := putBody(); b["title"] != "New" {
		t.Errorf("update_content title not sent: %v", b)
	} else if _, present := b["hold"]; present {
		t.Errorf("update_content sent hold when not asked: %v", b)
	}

	// update_content_by_path carries it too.
	callTool(t, s, "update_content_by_path", map[string]interface{}{"path": "/t", "hold": true})
	if b := putBody(); b["hold"] != true {
		t.Errorf("update_content_by_path hold not sent: %v", b)
	}
}

func TestDraftSafety_PublishMultipleReturnsSkipped(t *testing.T) {
	s, _ := recordingServer(t, map[string]string{
		"POST /api/v1/content/batch-publish": `{"published":["a"],"skipped":[{"id":"h","reason":"on hold"}],"failed":null}`,
	})
	text := resultText(t, callTool(t, s, "publish_multiple", map[string]interface{}{"publish_all_drafts": true}))
	if !strings.Contains(text, "skipped") || !strings.Contains(text, "on hold") {
		t.Errorf("publish_multiple result lacks skipped items: %s", text)
	}
}

func TestDraftSafety_MergeForkPublishNew(t *testing.T) {
	s, calls := recordingServer(t, map[string]string{
		"POST /api/v1/forks/f1/merge": `{"success":true,"updated":1,"created":2,"created_ids":["n1","n2"],"updated_ids":["u1"],
			"publish_new":true,"not_published":[{"id":"n2","path":"/held","reason":"on hold"}],"conflicts":[],"message":"ok"}`,
	})

	// The digest is required: without one (or with an empty one) nothing is sent.
	before := len(calls())
	if !callToolExpectError(t, s, "merge_fork", map[string]interface{}{"fork_id": "f1"}) {
		t.Error("merge_fork without a digest should fail")
	}
	if res := callTool(t, s, "merge_fork", map[string]interface{}{"fork_id": "f1", "digest": ""}); !res.IsError || !strings.Contains(resultText(t, res), "get_fork_diff") {
		t.Errorf("merge_fork with an empty digest: %s", resultText(t, res))
	}
	if len(calls()) != before {
		t.Error("a merge without a digest reached the API")
	}

	// Default: no publish_new in the request; the digest is sent.
	callTool(t, s, "merge_fork", map[string]interface{}{"fork_id": "f1", "digest": "d1"})
	if c := lastCall(t, calls); c.Method != "POST" || c.Path != "/api/v1/forks/f1/merge" || c.Body["publish_new"] != nil || c.Body["digest"] != "d1" {
		t.Errorf("merge_fork default request: %s %s %v", c.Method, c.Path, c.Body)
	}

	res := callTool(t, s, "merge_fork", map[string]interface{}{"fork_id": "f1", "publish_new": true, "digest": "d1"})
	if c := lastCall(t, calls); c.Body["publish_new"] != true {
		t.Errorf("merge_fork publish_new not sent: %v", c.Body)
	}
	text := resultText(t, res)
	for _, want := range []string{"created_ids", "n1", "updated_ids", "u1", "not_published", "on hold"} {
		if !strings.Contains(text, want) {
			t.Errorf("merge_fork result lacks %q: %s", want, text)
		}
	}
}

func TestDraftSafety_PurgeForkCopies(t *testing.T) {
	s, calls := recordingServer(t, map[string]string{
		"POST /api/v1/forks/f1/purge-copies": `{"fork_id":"f1","dry_run":true,"count":2,"copies":[{"id":"p1","full_path":"/a"},{"id":"p2","full_path":"/b"}]}`,
	})

	// fork_id is required.
	if res := callTool(t, s, "purge_fork_copies", map[string]interface{}{"fork_id": ""}); !res.IsError {
		t.Errorf("purge_fork_copies without fork_id: expected error, got %s", resultText(t, res))
	}

	res := callTool(t, s, "purge_fork_copies", map[string]interface{}{"fork_id": "f1", "dry_run": true})
	if c := lastCall(t, calls); c.Method != "POST" || c.Path != "/api/v1/forks/f1/purge-copies?dry_run=true" {
		t.Errorf("dry run request: %s %s", c.Method, c.Path)
	}
	text := resultText(t, res)
	if !strings.Contains(text, `"/a"`) || !strings.Contains(text, "p2") {
		t.Errorf("dry run result lacks the copy list: %s", text)
	}

	callTool(t, s, "purge_fork_copies", map[string]interface{}{"fork_id": "f1"})
	if c := lastCall(t, calls); c.Path != "/api/v1/forks/f1/purge-copies" {
		t.Errorf("purge request: %s", c.Path)
	}
}

func TestDraftSafety_PurgeRefusedOnActiveFork(t *testing.T) {
	// The API refuses active forks with 409; the tool surfaces it as an error.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"error":"fork is active; only merged or archived forks can be purged"}`))
	}))
	t.Cleanup(srv.Close)
	s := NewServer(apiclient.New(srv.URL, "test-key"))

	res := callTool(t, s, "purge_fork_copies", map[string]interface{}{"fork_id": "f1"})
	if !res.IsError || !strings.Contains(resultText(t, res), "only merged or archived") {
		t.Errorf("expected refusal for an active fork, got: %v %s", res.IsError, resultText(t, res))
	}
}
