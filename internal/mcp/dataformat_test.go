package mcp

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/metavert-io/lightcms/v7/internal/apiclient"
)

// dataServer returns a backend that replies with realistic, populated JSON so
// the MCP tool handlers take their result-formatting paths (not just the empty
// success path covered by permissiveServer).
func dataServer(t *testing.T) *Server {
	t.Helper()
	const contentObj = `{"id":"c1","title":"Hello World","slug":"hello","full_path":"/hello","published":true,"template_id":"t1","data":{"body":"Some body text"},"created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-02T00:00:00Z"}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		const srResult = `{"success":true,"search":"foo","replace":"bar","total_matches":3,"total_replacements":3,"affected_pages":2,"pages_updated":2,"matches":[{"id":"c1","title":"Hello","full_path":"/hello","published":true,"match_count":2,"field_matches":{"body":2}}],"updated_pages":[{"id":"c1","title":"Hello","full_path":"/hello","published":true,"match_count":2,"fields_updated":["body"]}]}`
		p := r.URL.Path
		switch {
		case strings.Contains(p, "/search-replace"):
			_, _ = w.Write([]byte(srResult))
		case strings.Contains(p, "/search"):
			_, _ = w.Write([]byte(`[` + contentObj + `]`))
		case strings.Contains(p, "/backlinks"):
			_, _ = w.Write([]byte(`[` + contentObj + `]`))
		case strings.HasSuffix(p, "/versions"):
			_, _ = w.Write([]byte(`[{"version":1,"comment":"init","modified_by_email":"a@x.com","created_at":"2026-01-01T00:00:00Z"}]`))
		case strings.Contains(p, "/content/") && !strings.HasSuffix(p, "/content"):
			_, _ = w.Write([]byte(contentObj))
		case strings.HasSuffix(p, "/content"):
			_, _ = w.Write([]byte(`[` + contentObj + `]`))
		case strings.HasSuffix(p, "/templates"):
			_, _ = w.Write([]byte(`[{"id":"t1","name":"Blog Post","slug":"blog","fields":[{"name":"body","type":"markdown"}]}]`))
		case strings.Contains(p, "/templates/"):
			_, _ = w.Write([]byte(`{"id":"t1","name":"Blog Post","slug":"blog","fields":[{"name":"body","type":"markdown"}],"html_layout":"<x>"}`))
		case strings.HasSuffix(p, "/snippets"):
			_, _ = w.Write([]byte(`[{"id":"s1","name":"cta","html":"<b>x</b>"}]`))
		case strings.HasSuffix(p, "/assets"):
			_, _ = w.Write([]byte(`[{"id":"a1","filename":"x.png","serve_path":"/x.png","mime_type":"image/png","size":10}]`))
		case strings.HasSuffix(p, "/theme"):
			_, _ = w.Write([]byte(`{"site_name":"My Site","primary_color":"#333","font_family":"Inter"}`))
		case strings.HasSuffix(p, "/site-config"):
			_, _ = w.Write([]byte(`{"max_upload_bytes":1048576,"title_template":"%s | Site"}`))
		case strings.HasSuffix(p, "/collections"):
			_, _ = w.Write([]byte(`[{"id":"col1","name":"News","slug":"news","category":"news"}]`))
		case strings.HasSuffix(p, "/folders"):
			_, _ = w.Write([]byte(`[{"id":"f1","name":"Blog","slug":"blog"}]`))
		case strings.HasSuffix(p, "/redirects"):
			_, _ = w.Write([]byte(`[{"id":"r1","from_path":"/old","to_path":"/new","status_code":301}]`))
		default:
			_, _ = w.Write([]byte(`null`))
		}
	}))
	t.Cleanup(srv.Close)
	return NewServer(apiclient.New(srv.URL, "test-key"))
}

// errorServer returns an MCP Server whose backend replies 500 to every
// request, so the tool handlers take their error-handling branches.
func errorServer(t *testing.T) *Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"boom"}`))
	}))
	t.Cleanup(srv.Close)
	return NewServer(apiclient.New(srv.URL, "test-key"))
}

// TestTools_ErrorPaths drives a broad set of MCP tools against a backend that
// always fails, exercising the error-handling branch of each tool handler.
func TestTools_ErrorPaths(t *testing.T) {
	s := errorServer(t)
	id := map[string]interface{}{"id": "x1"}
	calls := []struct {
		tool string
		args map[string]interface{}
	}{
		{"list_content", map[string]interface{}{}}, {"get_content", id},
		{"create_content", map[string]interface{}{"title": "T", "template_id": "t1", "slug": "t", "data": map[string]interface{}{"body": "x"}}},
		{"update_content", map[string]interface{}{"id": "x1", "title": "T"}},
		{"delete_content", id}, {"publish_content", id}, {"unpublish_content", id},
		{"restore_content", id}, {"preview_content", id},
		{"get_content_versions", map[string]interface{}{"content_id": "x1"}},
		{"get_backlinks", map[string]interface{}{"path": "/x"}},
		{"search_content", map[string]interface{}{"query": "q"}},
		{"search_replace_preview", map[string]interface{}{"search": "a", "replace": "b"}},
		{"list_templates", map[string]interface{}{}}, {"get_template", id},
		{"create_template", map[string]interface{}{"name": "N", "slug": "n", "fields": []map[string]interface{}{}, "html_layout": "<x>"}},
		{"delete_template", id},
		{"list_snippets", map[string]interface{}{}}, {"get_snippet", id},
		{"create_snippet", map[string]interface{}{"name": "n", "html": "<b>x</b>"}},
		{"delete_snippet", id},
		{"list_assets", map[string]interface{}{}}, {"get_asset", id}, {"delete_asset", id},
		{"get_theme", map[string]interface{}{}},
		{"update_theme", map[string]interface{}{"site_name": "S"}},
		{"get_site_config", map[string]interface{}{}},
		{"list_collections", map[string]interface{}{}}, {"get_collection", id},
		{"create_collection", map[string]interface{}{"name": "N", "category": "c", "slug": "n"}},
		{"delete_collection", id},
		{"list_folders", map[string]interface{}{}}, {"get_folder", id},
		{"create_folder", map[string]interface{}{"name": "N", "slug": "n"}},
		{"delete_folder", id},
		{"list_redirects", map[string]interface{}{}},
		{"create_redirect", map[string]interface{}{"from_path": "/a", "to_path": "/b"}},
		{"delete_redirect", id},
		{"list_forks", map[string]interface{}{}}, {"get_fork", id},
		{"list_webhooks", map[string]interface{}{}},
		{"list_comments", map[string]interface{}{"content_id": "x1"}},
		{"list_approval_workflows", map[string]interface{}{}},
		{"list_import_sources", map[string]interface{}{}},
		{"list_audit_logs", map[string]interface{}{}},
		{"list_scheduled_content", map[string]interface{}{}},
		{"get_content_lock", map[string]interface{}{"content_id": "x1"}},
		// Forks
		{"create_fork", map[string]interface{}{"name": "F"}}, {"get_fork", map[string]interface{}{"id": "f1"}},
		{"fork_page", map[string]interface{}{"fork_id": "f1", "content_id": "c1"}},
		{"remove_fork_page", map[string]interface{}{"fork_id": "f1", "page_id": "p1"}},
		{"merge_fork", map[string]interface{}{"fork_id": "f1", "digest": "d1"}},
		{"archive_fork", map[string]interface{}{"fork_id": "f1"}},
		{"delete_fork", map[string]interface{}{"fork_id": "f1"}},
		// Webhooks
		{"create_webhook", map[string]interface{}{"name": "W", "url": "https://x.com/h", "events": []string{"content.published"}}},
		{"update_webhook", map[string]interface{}{"id": "w1", "name": "W2"}},
		{"delete_webhook", map[string]interface{}{"id": "w1"}},
		{"regenerate_webhook_secret", map[string]interface{}{"id": "w1"}},
		{"list_webhook_deliveries", map[string]interface{}{"id": "w1"}},
		// Locks
		{"acquire_content_lock", map[string]interface{}{"content_id": "c1"}},
		{"release_content_lock", map[string]interface{}{"content_id": "c1"}},
		{"force_unlock_content", map[string]interface{}{"content_id": "c1"}},
		// Schedule
		{"schedule_content_publish", map[string]interface{}{"content_id": "c1", "publish_at": "2099-01-01T00:00:00Z"}},
		{"cancel_scheduled_publish", map[string]interface{}{"content_id": "c1"}},
		// Comments
		{"create_comment", map[string]interface{}{"content_id": "c1", "text": "hi"}},
		{"delete_comment", map[string]interface{}{"content_id": "c1", "comment_id": "m1"}},
		// Link check
		{"start_link_check", map[string]interface{}{}},
		{"get_link_check_results", map[string]interface{}{"job_id": "j1"}},
		// Imports
		{"create_import_source", map[string]interface{}{"name": "S", "url": "https://x.com/rss"}},
		{"update_import_source", map[string]interface{}{"id": "s1"}},
		{"delete_import_source", map[string]interface{}{"id": "s1"}},
		{"trigger_import_source", map[string]interface{}{"id": "s1"}},
		{"get_import_job", map[string]interface{}{"id": "j1"}},
		{"cancel_import_job", map[string]interface{}{"id": "j1"}},
		// Approvals
		{"get_approval_workflow", map[string]interface{}{"id": "w1"}},
		{"create_approval_workflow", map[string]interface{}{"name": "W", "trigger": "all_contributor", "mode": "concurrent"}},
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
			// Each tool should run its error branch without panicking.
			_ = callTool(t, s, c.tool, c.args)
		})
	}
}

// TestTools_FormatWithData drives the read/list MCP tools against a populated
// backend so their result-formatting branches execute.
func TestTools_FormatWithData(t *testing.T) {
	s := dataServer(t)
	calls := []struct {
		tool string
		args map[string]interface{}
	}{
		{"list_content", map[string]interface{}{}},
		{"get_content", map[string]interface{}{"id": "c1"}},
		{"get_content_versions", map[string]interface{}{"content_id": "c1"}},
		{"get_backlinks", map[string]interface{}{"path": "/hello"}},
		{"search_content", map[string]interface{}{"query": "hello"}},
		{"list_templates", map[string]interface{}{}},
		{"get_template", map[string]interface{}{"id": "t1"}},
		{"list_snippets", map[string]interface{}{}},
		{"list_assets", map[string]interface{}{}},
		{"get_theme", map[string]interface{}{}},
		{"get_site_config", map[string]interface{}{}},
		{"list_collections", map[string]interface{}{}},
		{"list_folders", map[string]interface{}{}},
		{"list_redirects", map[string]interface{}{}},
		{"search_replace_preview", map[string]interface{}{"search": "foo", "replace": "bar"}},
		{"search_replace_execute", map[string]interface{}{"search": "foo", "replace": "bar"}},
		{"end_user_search", map[string]interface{}{"query": "hello"}},
	}
	for _, c := range calls {
		t.Run(c.tool, func(t *testing.T) {
			res := callTool(t, s, c.tool, c.args)
			if res == nil {
				t.Errorf("%s: nil result", c.tool)
			}
		})
	}
}
