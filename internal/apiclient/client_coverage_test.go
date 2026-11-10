package apiclient

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// nullServer returns a test server that, on success, replies with the JSON
// literal `null` (which decodes cleanly into any result type), and on error
// replies with an {"error": ...} body and the given status. This lets a single
// table cover both the success and error branches of every thin client wrapper.
func nullServer(t *testing.T, status int) *Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		if status >= 400 {
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "boom"})
			return
		}
		_, _ = w.Write([]byte("null"))
	}))
	t.Cleanup(srv.Close)
	return New(srv.URL, "tok")
}

// TestClientWrappers exercises the thin HTTP wrapper methods of Client. Each
// entry is invoked twice: once against a 200/null server (success branch) and
// once against a 500/error server (error branch).
func TestClientWrappers(t *testing.T) {
	ctx := context.Background()
	ps := func(s string) *string { return &s }

	calls := map[string]func(c *Client) error{
		// Search / replace
		"SearchReplacePreviewPairs": func(c *Client) error { _, err := c.SearchReplacePreviewPairs(ctx, nil); return err },
		"SearchReplaceExecutePairs": func(c *Client) error { _, err := c.SearchReplaceExecutePairs(ctx, nil, "c", true); return err },
		"ScopedSearchReplacePreview": func(c *Client) error {
			_, err := c.ScopedSearchReplacePreview(ctx, "a", "b", false, ScopedSearchReplaceScope{})
			return err
		},
		"ScopedSearchReplaceExecute": func(c *Client) error {
			_, err := c.ScopedSearchReplaceExecute(ctx, "a", "b", "c", false, true, ScopedSearchReplaceScope{})
			return err
		},

		// API keys
		"ListAPIKeys":  func(c *Client) error { _, err := c.ListAPIKeys(ctx); return err },
		"CreateAPIKey": func(c *Client) error { _, err := c.CreateAPIKey(ctx, "n", "d"); return err },
		"DeleteAPIKey": func(c *Client) error { return c.DeleteAPIKey(ctx, "id") },

		// Content
		"RegenerateAllContent":   func(c *Client) error { return c.RegenerateAllContent(ctx) },
		"EndUserSearch":          func(c *Client) error { _, err := c.EndUserSearch(ctx, "q", "fuzzy", 5); return err },
		"ReindexEmbeddings":      func(c *Client) error { _, err := c.ReindexEmbeddings(ctx); return err },
		"BatchPublishContent":    func(c *Client) error { _, err := c.BatchPublishContent(ctx, []string{"a"}, false); return err },
		"PreviewContent":         func(c *Client) error { _, err := c.PreviewContent(ctx, "id", nil); return err },
		"UpdateContentByPath":    func(c *Client) error { _, err := c.UpdateContentByPath(ctx, "/p", nil); return err },
		"ListContentWithOptions": func(c *Client) error { _, err := c.ListContentWithOptions(ctx, ListContentOptions{}); return err },
		"ListContentPaginated":   func(c *Client) error { _, err := c.ListContentPaginated(ctx, ListContentOptions{}); return err },
		"BulkCreateContent":      func(c *Client) error { _, err := c.BulkCreateContent(ctx, map[string]interface{}{}); return err },
		"BulkUpdateContent":      func(c *Client) error { _, err := c.BulkUpdateContent(ctx, map[string]interface{}{}); return err },
		"BulkFieldOperation":     func(c *Client) error { _, err := c.BulkFieldOperation(ctx, map[string]interface{}{}); return err },
		"ExportContent":          func(c *Client) error { _, err := c.ExportContent(ctx, map[string]interface{}{}); return err },

		// Assets / theme
		"UploadAssetFromURL": func(c *Client) error { _, err := c.UploadAssetFromURL(ctx, "http://x/y.png", "/p", "d"); return err },
		"PinThemeVersion":    func(c *Client) error { return c.PinThemeVersion(ctx, 1) },
		"UnpinThemeVersion":  func(c *Client) error { return c.UnpinThemeVersion(ctx, 1) },

		// Snippets
		"ListSnippets":  func(c *Client) error { _, err := c.ListSnippets(ctx); return err },
		"GetSnippet":    func(c *Client) error { _, err := c.GetSnippet(ctx, "id"); return err },
		"CreateSnippet": func(c *Client) error { _, err := c.CreateSnippet(ctx, CreateSnippetRequest{}); return err },
		"UpdateSnippet": func(c *Client) error { _, err := c.UpdateSnippet(ctx, "id", UpdateSnippetRequest{}); return err },
		"DeleteSnippet": func(c *Client) error { return c.DeleteSnippet(ctx, "id") },

		// Forks
		"ListForks":           func(c *Client) error { _, err := c.ListForks(ctx); return err },
		"CreateFork":          func(c *Client) error { _, err := c.CreateFork(ctx, "n", "d"); return err },
		"GetFork":             func(c *Client) error { _, err := c.GetFork(ctx, "id"); return err },
		"ForkPage":            func(c *Client) error { _, err := c.ForkPage(ctx, "f", "c", "/p"); return err },
		"ListForkPages":       func(c *Client) error { _, err := c.ListForkPages(ctx, "f"); return err },
		"RemoveForkPage":      func(c *Client) error { return c.RemoveForkPage(ctx, "f", "p") },
		"MergeFork":           func(c *Client) error { _, err := c.MergeFork(ctx, "f", false, "d"); return err },
		"MergeForkPublishNew": func(c *Client) error { _, err := c.MergeFork(ctx, "f", true, "d"); return err },
		"PurgeForkCopies":     func(c *Client) error { _, err := c.PurgeForkCopies(ctx, "f", false); return err },
		"PurgeForkCopiesDry":  func(c *Client) error { _, err := c.PurgeForkCopies(ctx, "f", true); return err },
		"SearchContentForks":  func(c *Client) error { _, err := c.SearchContent(ctx, "q", "", false, true); return err },
		"ListContentForks": func(c *Client) error {
			_, err := c.ListContentWithOptions(ctx, ListContentOptions{IncludeForks: true})
			return err
		},
		"ListContentPaginatedForks": func(c *Client) error {
			_, err := c.ListContentPaginated(ctx, ListContentOptions{IncludeForks: true, Limit: 5})
			return err
		},
		"ArchiveFork": func(c *Client) error { return c.ArchiveFork(ctx, "f") },
		"DeleteFork":  func(c *Client) error { return c.DeleteFork(ctx, "f") },

		// Imports
		"ListImportSources":  func(c *Client) error { _, err := c.ListImportSources(ctx); return err },
		"CreateImportSource": func(c *Client) error { _, err := c.CreateImportSource(ctx, CreateImportSourceRequest{}); return err },
		"UpdateImportSource": func(c *Client) error {
			_, err := c.UpdateImportSource(ctx, "id", UpdateImportSourceRequest{})
			return err
		},
		"DeleteImportSource":  func(c *Client) error { return c.DeleteImportSource(ctx, "id") },
		"TriggerImportSource": func(c *Client) error { _, err := c.TriggerImportSource(ctx, "id"); return err },
		"ImportMarkdown":      func(c *Client) error { _, err := c.ImportMarkdown(ctx, ImportMarkdownRequest{}); return err },
		"ImportCSV":           func(c *Client) error { _, err := c.ImportCSV(ctx, ImportCSVRequest{}); return err },
		"ListImportJobs":      func(c *Client) error { _, err := c.ListImportJobs(ctx, 10); return err },
		"GetImportJob":        func(c *Client) error { _, err := c.GetImportJob(ctx, "id", true); return err },
		"CancelImportJob":     func(c *Client) error { return c.CancelImportJob(ctx, "id") },

		// Webhooks
		"ListWebhooks":            func(c *Client) error { _, err := c.ListWebhooks(ctx); return err },
		"CreateWebhook":           func(c *Client) error { _, err := c.CreateWebhook(ctx, CreateWebhookRequest{}); return err },
		"UpdateWebhook":           func(c *Client) error { _, err := c.UpdateWebhook(ctx, "id", UpdateWebhookRequest{}); return err },
		"DeleteWebhook":           func(c *Client) error { return c.DeleteWebhook(ctx, "id") },
		"RegenerateWebhookSecret": func(c *Client) error { _, err := c.RegenerateWebhookSecret(ctx, "id"); return err },
		"ListWebhookDeliveries":   func(c *Client) error { _, err := c.ListWebhookDeliveries(ctx, "id", 5); return err },

		// Locks
		"GetContentLock":     func(c *Client) error { _, err := c.GetContentLock(ctx, "id"); return err },
		"AcquireContentLock": func(c *Client) error { _, err := c.AcquireContentLock(ctx, "id"); return err },
		"ReleaseContentLock": func(c *Client) error { return c.ReleaseContentLock(ctx, "id") },
		"ForceUnlockContent": func(c *Client) error { return c.ForceUnlockContent(ctx, "id") },

		// Schedule
		"ScheduleContentPublish": func(c *Client) error {
			_, err := c.ScheduleContentPublish(ctx, "id", ps("2026-01-01T00:00:00Z"))
			return err
		},
		"ListScheduledContent": func(c *Client) error { _, err := c.ListScheduledContent(ctx, ""); return err },

		// Audit / link check
		"ListAuditLogs":   func(c *Client) error { _, err := c.ListAuditLogs(ctx, 10, "", ""); return err },
		"StartLinkCheck":  func(c *Client) error { _, err := c.StartLinkCheck(ctx); return err },
		"GetLinkCheckJob": func(c *Client) error { _, err := c.GetLinkCheckJob(ctx, "id"); return err },

		// Comments
		"ListComments":  func(c *Client) error { _, err := c.ListComments(ctx, "cid"); return err },
		"CreateComment": func(c *Client) error { _, err := c.CreateComment(ctx, "cid", CreateCommentRequest{}); return err },
		"DeleteComment": func(c *Client) error { return c.DeleteComment(ctx, "cid", "mid") },

		// Approvals
		"ListApprovalWorkflows":  func(c *Client) error { _, err := c.ListApprovalWorkflows(ctx); return err },
		"GetApprovalWorkflow":    func(c *Client) error { _, err := c.GetApprovalWorkflow(ctx, "id"); return err },
		"CreateApprovalWorkflow": func(c *Client) error { _, err := c.CreateApprovalWorkflow(ctx, CreateWorkflowRequest{}); return err },
		"UpdateApprovalWorkflow": func(c *Client) error { return c.UpdateApprovalWorkflow(ctx, "id", CreateWorkflowRequest{}) },
		"DeleteApprovalWorkflow": func(c *Client) error { return c.DeleteApprovalWorkflow(ctx, "id") },
		"ListApprovalRequests":   func(c *Client) error { _, err := c.ListApprovalRequests(ctx, ""); return err },
		"GetApprovalRequest":     func(c *Client) error { _, err := c.GetApprovalRequest(ctx, "id"); return err },
		"SubmitForApproval":      func(c *Client) error { _, err := c.SubmitForApproval(ctx, "cid"); return err },
		"ApproveRequest":         func(c *Client) error { return c.ApproveRequest(ctx, "id", ApproveRejectRequest{}) },
		"RejectRequest":          func(c *Client) error { return c.RejectRequest(ctx, "id", ApproveRejectRequest{}) },
		"CancelApprovalRequest":  func(c *Client) error { return c.CancelApprovalRequest(ctx, "id") },
	}

	for name, call := range calls {
		call := call
		t.Run(name+"/success", func(t *testing.T) {
			if err := call(nullServer(t, http.StatusOK)); err != nil {
				t.Errorf("%s: unexpected error on success: %v", name, err)
			}
		})
		t.Run(name+"/error", func(t *testing.T) {
			if err := call(nullServer(t, http.StatusInternalServerError)); err == nil {
				t.Errorf("%s: expected error on 500, got nil", name)
			}
		})
	}
}

// TestAgentGovernanceWrappers exercises the agent-session, fork-diff, and
// maintenance client methods plus the X-Agent-Session header plumbing.
func TestAgentGovernanceWrappers(t *testing.T) {
	ctx := context.Background()

	var lastSessionHeader string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		lastSessionHeader = r.Header.Get("X-Agent-Session")
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"fork_id":"f1","pages":[]}`))
	}))
	defer srv.Close()

	c := New(srv.URL, "k")
	if c.AgentSession() != "" {
		t.Error("agent session should start empty")
	}
	c.SetAgentSession("agent-xyz")
	if c.AgentSession() != "agent-xyz" {
		t.Error("SetAgentSession not stored")
	}

	if _, err := c.GetForkDiff(ctx, "f1"); err != nil {
		t.Errorf("GetForkDiff: %v", err)
	}
	if lastSessionHeader != "agent-xyz" {
		t.Errorf("X-Agent-Session = %q, want agent-xyz", lastSessionHeader)
	}
	if _, err := c.GetAgentSessionChanges(ctx, "agent-xyz"); err != nil {
		t.Errorf("GetAgentSessionChanges: %v", err)
	}
	if _, err := c.RollbackAgentSession(ctx, "agent-xyz"); err != nil {
		t.Errorf("RollbackAgentSession: %v", err)
	}
	if _, err := c.GetMaintenanceReport(ctx); err != nil {
		t.Errorf("GetMaintenanceReport: %v", err)
	}
	if _, err := c.RunMaintenanceScan(ctx, true); err != nil {
		t.Errorf("RunMaintenanceScan: %v", err)
	}
}
