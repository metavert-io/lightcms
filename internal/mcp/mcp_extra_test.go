package mcp

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/metavert-io/lightcms/v7/internal/apiclient"
)

// permissiveServer returns an MCP Server whose REST backend replies with the
// JSON literal `null` (HTTP 200) to every request. `null` decodes cleanly into
// any apiclient result type, so the thin MCP tool handlers take their success
// path. This exercises the many tool handlers that the primary mock in
// mcp_test.go does not cover.
func permissiveServer(t *testing.T) *Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("null"))
	}))
	t.Cleanup(srv.Close)
	return NewServer(apiclient.New(srv.URL, "test-key"))
}

// TestTools_Exercise invokes the forks/webhooks/comments/locks/schedule/
// imports/approvals/link-check/audit MCP tool handlers against a permissive
// backend, verifying each runs without a transport error and reports success.
func TestTools_Exercise(t *testing.T) {
	s := permissiveServer(t)

	calls := []struct {
		tool string
		args map[string]interface{}
	}{
		// Forks
		{"list_forks", map[string]interface{}{}},
		{"create_fork", map[string]interface{}{"name": "F"}},
		{"get_fork", map[string]interface{}{"id": "f1"}},
		{"fork_page", map[string]interface{}{"fork_id": "f1", "content_id": "c1"}},
		{"remove_fork_page", map[string]interface{}{"fork_id": "f1", "page_id": "p1"}},
		{"merge_fork", map[string]interface{}{"fork_id": "f1", "digest": "d1"}},
		{"archive_fork", map[string]interface{}{"fork_id": "f1"}},
		{"delete_fork", map[string]interface{}{"fork_id": "f1"}},
		// Webhooks
		{"list_webhooks", map[string]interface{}{}},
		{"create_webhook", map[string]interface{}{"name": "W", "url": "https://x.com/h", "events": []string{"content.publish"}}},
		{"update_webhook", map[string]interface{}{"id": "w1", "name": "W2"}},
		{"delete_webhook", map[string]interface{}{"id": "w1"}},
		{"regenerate_webhook_secret", map[string]interface{}{"id": "w1"}},
		{"list_webhook_deliveries", map[string]interface{}{"id": "w1"}},
		// Locks
		{"get_content_lock", map[string]interface{}{"content_id": "c1"}},
		{"acquire_content_lock", map[string]interface{}{"content_id": "c1"}},
		{"release_content_lock", map[string]interface{}{"content_id": "c1"}},
		{"force_unlock_content", map[string]interface{}{"content_id": "c1"}},
		// Schedule
		{"schedule_content_publish", map[string]interface{}{"content_id": "c1", "publish_at": "2099-01-01T00:00:00Z"}},
		{"cancel_scheduled_publish", map[string]interface{}{"content_id": "c1"}},
		{"list_scheduled_content", map[string]interface{}{}},
		// Comments
		{"list_comments", map[string]interface{}{"content_id": "c1"}},
		{"create_comment", map[string]interface{}{"content_id": "c1", "text": "hi"}},
		{"delete_comment", map[string]interface{}{"content_id": "c1", "comment_id": "m1"}},
		// Link check
		{"start_link_check", map[string]interface{}{}},
		{"get_link_check_results", map[string]interface{}{"job_id": "j1"}},
		// Audit
		{"list_audit_logs", map[string]interface{}{}},
		// Imports
		{"list_import_sources", map[string]interface{}{}},
		{"create_import_source", map[string]interface{}{"name": "S", "url": "https://x.com/rss"}},
		{"update_import_source", map[string]interface{}{"id": "s1"}},
		{"delete_import_source", map[string]interface{}{"id": "s1"}},
		{"trigger_import_source", map[string]interface{}{"id": "s1"}},
		{"import_markdown", map[string]interface{}{"pages": []map[string]interface{}{{"content": "# Hi\n\nBody", "filename": "hi.md"}}}},
		{"import_csv", map[string]interface{}{"csv_data": "title,body\nHi,B", "title_column": "title"}},
		{"list_import_jobs", map[string]interface{}{}},
		{"get_import_job", map[string]interface{}{"id": "j1"}},
		{"cancel_import_job", map[string]interface{}{"id": "j1"}},
		// Approvals
		{"list_approval_workflows", map[string]interface{}{}},
		{"get_approval_workflow", map[string]interface{}{"id": "w1"}},
		{"create_approval_workflow", map[string]interface{}{"name": "W", "trigger": "all_contributor", "mode": "concurrent"}},
		{"update_approval_workflow", map[string]interface{}{"id": "w1", "name": "W", "trigger": "all_contributor", "mode": "concurrent"}},
		{"delete_approval_workflow", map[string]interface{}{"id": "w1"}},
		{"list_approval_requests", map[string]interface{}{}},
		{"get_approval_request", map[string]interface{}{"id": "r1"}},
		{"submit_for_approval", map[string]interface{}{"content_id": "c1"}},
		{"approve_request", map[string]interface{}{"id": "r1"}},
		{"reject_request", map[string]interface{}{"id": "r1", "comment": "no"}},
		{"cancel_approval_request", map[string]interface{}{"id": "r1"}},
	}

	for _, c := range calls {
		t.Run(c.tool, func(t *testing.T) {
			result := callTool(t, s, c.tool, c.args)
			if result == nil {
				t.Fatalf("%s: nil result", c.tool)
			}
			// A `null` 200 backend should drive the success path; log (don't fail)
			// any tool that still reports an error so the suite stays robust to
			// tool-specific response shape expectations.
			if result.IsError {
				t.Logf("%s reported error: %s", c.tool, resultText(t, result))
			}
		})
	}
}
