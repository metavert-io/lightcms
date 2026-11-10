package mcp

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/metavert-io/lightcms/v7/internal/apiclient"
)

// richServer serves a content list containing a deleted item plus generic
// success JSON for everything else — used to reach list-content counting
// branches and multi-pair search/replace success paths.
func richServer(t *testing.T) *Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/content", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode([]map[string]interface{}{
			{
				"id": "d1", "title": "Deleted Page", "slug": "gone", "full_path": "/gone",
				"published": false, "deleted": true, "updated_at": "2025-01-02T00:00:00Z",
				"data": map[string]interface{}{"body": "bye"},
			},
			{
				"id": "p1", "title": "Pub Page", "slug": "pub", "full_path": "/pub",
				"published": true, "deleted": false, "updated_at": "2025-01-02T00:00:00Z",
				"published_at": "2025-01-01T00:00:00Z",
				"data":         map[string]interface{}{"body": "hi"},
			},
		})
	})
	// Upsert responses report updated_at after created_at → action "updated".
	mux.HandleFunc("POST /api/v1/content", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(201)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"id": "u1", "title": "Upserted", "slug": "up", "full_path": "/up",
			"created_at": "2025-01-01T00:00:00Z", "updated_at": "2025-06-01T00:00:00Z",
		})
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("null"))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return NewServer(apiclient.New(srv.URL, "test-key"))
}

// TestTools_MoreErrorPaths drives errorResult branches (not covered by
// TestTools_ErrorPaths in dataformat_test.go) for tools whose backend call
// fails.
func TestTools_MoreErrorPaths(t *testing.T) {
	s := errorServer(t)

	calls := []struct {
		name string
		tool string
		args map[string]interface{}
	}{
		{"get_content_version", "get_content_version", map[string]interface{}{"content_id": "c1", "version": 1}},
		{"revert_to_version", "revert_to_version", map[string]interface{}{"content_id": "c1", "version": 1}},
		{"publish_multiple", "publish_multiple", map[string]interface{}{"ids": []string{"c1"}}},
		{"update_content_data_fetch", "update_content", map[string]interface{}{"id": "c1", "data": map[string]interface{}{"body": "b"}}},
		{"update_content_by_path", "update_content_by_path", map[string]interface{}{"path": "/x", "title": "T"}},
		{"scoped_preview", "scoped_search_replace_preview", map[string]interface{}{"search": "a", "replace": "b"}},
		{"scoped_execute", "scoped_search_replace_execute", map[string]interface{}{"search": "a", "replace": "b"}},
		{"bulk_create_content", "bulk_create_content", map[string]interface{}{"items": []map[string]interface{}{{"template_id": "t1", "title": "A", "slug": "a", "data": map[string]interface{}{}}}}},
		{"bulk_update_content", "bulk_update_content", map[string]interface{}{"updates": []map[string]interface{}{{"id": "c1", "title": "B"}}}},
		{"bulk_field_operation", "bulk_field_operation", map[string]interface{}{"operation": "clear", "field": "body"}},
		{"export_content", "export_content", map[string]interface{}{"template_name": "Blog Post"}},
		{"preview_pairs", "search_replace_preview", map[string]interface{}{"pairs": []map[string]interface{}{{"search": "a", "replace": "b"}}}},
		{"execute_single", "search_replace_execute", map[string]interface{}{"search": "a", "replace": "b"}},
		{"execute_pairs", "search_replace_execute", map[string]interface{}{"pairs": []map[string]interface{}{{"search": "a", "replace": "b"}}}},
		{"end_user_search", "end_user_search", map[string]interface{}{"query": "x"}},
		{"reindex_embeddings", "reindex_embeddings", map[string]interface{}{}},
		{"start_agent_sandbox", "start_agent_sandbox", map[string]interface{}{"name": "s"}},
		{"get_fork_diff", "get_fork_diff", map[string]interface{}{"id": "f1"}},
		{"update_content_by_path_data_fetch", "update_content_by_path", map[string]interface{}{"path": "/x", "data": map[string]interface{}{"body": "b"}}},
		{"get_agent_session_changes", "get_agent_session_changes", map[string]interface{}{"session_id": "s1"}},
		{"rollback_agent_session", "rollback_agent_session", map[string]interface{}{"session_id": "s1"}},
		{"get_maintenance_report", "get_maintenance_report", map[string]interface{}{}},
		{"run_maintenance_scan", "run_maintenance_scan", map[string]interface{}{"link_check": true}},
	}

	for _, c := range calls {
		t.Run(c.name, func(t *testing.T) {
			result := callTool(t, s, c.tool, c.args)
			if !result.IsError {
				t.Errorf("%s: expected error result, got: %s", c.tool, resultText(t, result))
			}
		})
	}
}

// TestTools_ValidationBranches exercises handler-level argument validation
// that runs before any backend call.
func TestTools_ValidationBranches(t *testing.T) {
	s := permissiveServer(t)

	calls := []struct {
		tool    string
		args    map[string]interface{}
		wantErr string
	}{
		{"search_content", map[string]interface{}{"query": ""}, "query is required"},
		{"end_user_search", map[string]interface{}{"query": ""}, "query is required"},
		{"search_replace_preview", map[string]interface{}{"search": ""}, "search text is required"},
		{"search_replace_execute", map[string]interface{}{"search": ""}, "search text is required"},
		{"get_backlinks", map[string]interface{}{"path": ""}, "path is required"},
	}
	for _, c := range calls {
		t.Run(c.tool, func(t *testing.T) {
			result := callTool(t, s, c.tool, c.args)
			if !result.IsError || !strings.Contains(resultText(t, result), c.wantErr) {
				t.Errorf("%s: expected %q error, got: %v %s", c.tool, c.wantErr, result.IsError, resultText(t, result))
			}
		})
	}
}

// TestTools_SuccessBranches covers optional-field branches on success paths.
func TestTools_SuccessBranches(t *testing.T) {
	s := permissiveServer(t)

	// update_content with every optional scalar field set.
	res := callTool(t, s, "update_content", map[string]interface{}{
		"id": "c1", "template_id": "t2", "title": "T", "slug": "s",
		"folder_path": "/f", "category": "cat", "tags": []string{"a"},
		"meta_description": "md", "og_image": "/img.png",
		"use_header": true, "use_footer": true, "use_theme": true, "raw_mode": true,
		"set_use_header": true, "set_use_footer": true, "set_use_theme": true, "set_raw_mode": true,
		"data": map[string]interface{}{"body": "b"}, "clear_fields": []string{"old"},
		"version_comment": "vc",
	})
	if res.IsError {
		t.Errorf("update_content all-fields: %s", resultText(t, res))
	}

	// update_content dry-run branch.
	res = callTool(t, s, "update_content", map[string]interface{}{
		"id": "c1", "title": "T", "dry_run": true, "version_comment": "vc",
	})
	if res.IsError {
		t.Errorf("update_content dry_run: %s", resultText(t, res))
	}

	// update_content_by_path with every optional field (incl. data merge and
	// explicit published pointer).
	res = callTool(t, s, "update_content_by_path", map[string]interface{}{
		"path": "/x", "title": "T", "data": map[string]interface{}{"body": "b"},
		"category": "c", "tags": []string{"t"}, "meta_description": "md",
		"og_image": "/i.png", "published": true, "version_comment": "vc",
	})
	if res.IsError {
		t.Errorf("update_content_by_path all-fields: %s", resultText(t, res))
	}

	// Multi-pair search/replace success paths.
	res = callTool(t, s, "search_replace_preview", map[string]interface{}{
		"pairs": []map[string]interface{}{{"search": "a", "replace": "b"}, {"search": "c", "replace": "d", "regex": true}},
	})
	if res.IsError {
		t.Errorf("search_replace_preview pairs: %s", resultText(t, res))
	}
	res = callTool(t, s, "search_replace_execute", map[string]interface{}{
		"pairs":           []map[string]interface{}{{"search": "a", "replace": "b"}},
		"version_comment": "vc", "auto_republish": true,
	})
	if res.IsError {
		t.Errorf("search_replace_execute pairs: %s", resultText(t, res))
	}

	// end_user_search default-limit branch.
	res = callTool(t, s, "end_user_search", map[string]interface{}{"query": "hello"})
	if res.IsError {
		t.Errorf("end_user_search: %s", resultText(t, res))
	}

	// preview_content with overrides.
	res = callTool(t, s, "preview_content", map[string]interface{}{
		"id": "c1", "title": "Alt", "data": map[string]interface{}{"body": "x"},
	})
	if res.IsError {
		t.Errorf("preview_content overrides: %s", resultText(t, res))
	}

	// start_agent_sandbox against a backend that returns no fork ID.
	res = callTool(t, s, "start_agent_sandbox", map[string]interface{}{})
	if res.IsError {
		t.Errorf("start_agent_sandbox null backend: %s", resultText(t, res))
	}
	if !strings.Contains(resultText(t, res), "no ID returned") {
		t.Errorf("expected 'no ID returned', got: %s", resultText(t, res))
	}
}

func TestListContent_DataAndDeletedBranches(t *testing.T) {
	s := richServer(t)

	res := callTool(t, s, "list_content", map[string]interface{}{
		"include_deleted": true, "include_data": true,
	})
	if res.IsError {
		t.Fatalf("list_content: %s", resultText(t, res))
	}
	var out struct {
		Total   int `json:"total"`
		Deleted int `json:"deleted"`
		Items   []struct {
			Data map[string]interface{} `json:"data,omitempty"`
		} `json:"items"`
	}
	resultJSON(t, res, &out)
	if out.Total != 2 || out.Deleted != 1 {
		t.Errorf("total=%d deleted=%d, want 2/1", out.Total, out.Deleted)
	}
	if len(out.Items) == 0 || out.Items[0].Data == nil {
		t.Errorf("include_data did not include field data: %+v", out.Items)
	}

	// create_content upsert reporting "updated" when updated_at > created_at.
	res = callTool(t, s, "create_content", map[string]interface{}{
		"template_id": "t1", "title": "Up", "slug": "up",
		"data": map[string]interface{}{"body": "b"}, "upsert": true,
	})
	if res.IsError {
		t.Fatalf("create_content upsert: %s", resultText(t, res))
	}
	if !strings.Contains(resultText(t, res), `"updated"`) {
		t.Errorf("expected action 'updated', got: %s", resultText(t, res))
	}
}

// TestSandbox_TargetResolutionBranches covers the copy-on-write edge cases:
// content in another fork, unresolvable content IDs, fork-page failures, and
// data merges against the sandbox copy.
func TestSandbox_TargetResolutionBranches(t *testing.T) {
	s, _, cleanup := newSandboxServer(t)
	defer cleanup()

	callTool(t, s, "start_agent_sandbox", map[string]string{})

	// Content that belongs to a different fork is rejected.
	res := callTool(t, s, "update_content", map[string]interface{}{
		"id": "otherfork1", "title": "X",
	})
	if !res.IsError || !strings.Contains(resultText(t, res), "another fork") {
		t.Errorf("expected another-fork rejection, got: %v %s", res.IsError, resultText(t, res))
	}

	// Content that can't be resolved at all.
	res = callTool(t, s, "update_content", map[string]interface{}{
		"id": "missing1", "title": "X",
	})
	if !res.IsError || !strings.Contains(resultText(t, res), "resolve sandbox target") {
		t.Errorf("expected resolve error, got: %v %s", res.IsError, resultText(t, res))
	}

	// Path-based copy-on-write failure.
	res = callTool(t, s, "update_content_by_path", map[string]interface{}{
		"path": "/boom", "title": "X",
	})
	if !res.IsError || !strings.Contains(resultText(t, res), "copy-on-write failed") {
		t.Errorf("expected copy-on-write failure, got: %v %s", res.IsError, resultText(t, res))
	}

	// Path-based update with a data merge goes through the sandbox copy.
	res = callTool(t, s, "update_content_by_path", map[string]interface{}{
		"path": "/live-page", "data": map[string]interface{}{"body": "merged"},
		"version_comment": "vc",
	})
	if res.IsError {
		t.Errorf("sandbox data merge by path: %s", resultText(t, res))
	}

	// Ending with an unknown action hits the validation branch.
	res = callTool(t, s, "end_agent_sandbox", map[string]string{"action": "bogus"})
	if !strings.Contains(resultText(t, res), "must be 'submit' or 'discard'") {
		t.Errorf("expected action validation, got: %s", resultText(t, res))
	}

	// get_fork_diff success path.
	res = callTool(t, s, "get_fork_diff", map[string]string{"id": "fork1"})
	if res.IsError {
		t.Errorf("get_fork_diff: %s", resultText(t, res))
	}
}

// TestSandbox_PassthroughAndFailureBranches covers the remaining sandbox
// resolution branches: updating the fork copy directly, copy-on-write fork
// failure by content ID, and diff/delete backend failures during status,
// submit, and discard.
func TestSandbox_PassthroughAndFailureBranches(t *testing.T) {
	s, state, cleanup := newSandboxServer(t)
	defer cleanup()

	callTool(t, s, "start_agent_sandbox", map[string]string{})

	// Updating the sandbox copy itself passes straight through.
	res := callTool(t, s, "update_content", map[string]interface{}{
		"id": "fp-live-page", "title": "Direct fork edit",
	})
	if res.IsError {
		t.Errorf("update of fork copy: %s", resultText(t, res))
	}

	// Live page whose copy-on-write fork fails.
	res = callTool(t, s, "update_content", map[string]interface{}{
		"id": "liveboom", "title": "X",
	})
	if !res.IsError || !strings.Contains(resultText(t, res), "copy-on-write failed") {
		t.Errorf("expected copy-on-write failure by ID, got: %v %s", res.IsError, resultText(t, res))
	}

	// Diff backend failure: status and submit both surface the error.
	state.mu.Lock()
	state.diffFails = true
	state.mu.Unlock()
	res = callTool(t, s, "get_agent_sandbox", struct{}{})
	if !res.IsError {
		t.Errorf("expected diff error in get_agent_sandbox, got: %s", resultText(t, res))
	}
	res = callTool(t, s, "end_agent_sandbox", map[string]string{"action": "submit"})
	if !res.IsError {
		t.Errorf("expected diff error in submit, got: %s", resultText(t, res))
	}
	state.mu.Lock()
	state.diffFails = false
	state.mu.Unlock()

	// Delete backend failure during discard keeps the sandbox active.
	state.mu.Lock()
	state.deleteFails = true
	state.mu.Unlock()
	res = callTool(t, s, "end_agent_sandbox", map[string]string{"action": "discard"})
	if !res.IsError {
		t.Errorf("expected delete error in discard, got: %s", resultText(t, res))
	}
	state.mu.Lock()
	state.deleteFails = false
	state.mu.Unlock()

	// Now discard succeeds.
	res = callTool(t, s, "end_agent_sandbox", map[string]string{"action": "discard"})
	if res.IsError {
		t.Errorf("final discard failed: %s", resultText(t, res))
	}
}
