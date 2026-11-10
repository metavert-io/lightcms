package handlers

import (
	"net/http"

	"github.com/metavert-io/lightcms/v7/internal/auth"
	"github.com/metavert-io/lightcms/v7/internal/middleware"
	"github.com/metavert-io/lightcms/v7/internal/services"

	"github.com/gorilla/mux"
)

// StampProvenance records who is behind each API change on the request context
// (change provenance and version attribution). An agent is identified by an
// OAuth connection, or by the X-Agent-Session header, which the MCP endpoint
// sets itself and the stdio MCP client sends; from any other caller that header
// is only a label.
func StampProvenance(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		user, _ := auth.UserFromAPIContext(ctx)
		prov := services.Provenance{Actor: "human", Via: "api"}
		if sess := r.Header.Get("X-Agent-Session"); sess != "" {
			prov.Actor = "agent"
			prov.AgentSession = sess
		}
		if user != nil && user.ViaOAuth {
			prov.Actor = "agent"
		}
		ctx = services.WithProvenance(ctx, prov)
		if user != nil {
			ctx = services.WithEditorEmail(ctx, user.Email)
		}
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// MountAPI creates the /api/v1 subrouter on r with its middleware and every
// REST route, and returns it.
func (a *APIHandler) MountAPI(r *mux.Router, apiAuth *middleware.APIAuth) *mux.Router {
	apiv1 := r.PathPrefix("/api/v1").Subrouter()
	apiv1.Use(apiAuth.Middleware)
	apiv1.Use(middleware.APIBurstRateLimit) // per-credential burst limit (20 req/s)
	apiv1.Use(middleware.APIRateLimit)      // per-credential sliding-window rate limit (300 req/min)
	apiv1.Use(middleware.APIBodySizeLimit)  // cap request body at 10 MiB to prevent memory exhaustion
	apiv1.Use(StampProvenance)

	// Content
	apiv1.HandleFunc("/content", a.APIListContent).Methods("GET")
	apiv1.HandleFunc("/content", a.APICreateContent).Methods("POST")
	apiv1.HandleFunc("/content/by-path", a.APIGetContentByPath).Methods("GET")
	apiv1.HandleFunc("/content/by-path", a.APIUpdateContentByPath).Methods("PUT")
	apiv1.HandleFunc("/content/backlinks", a.APIGetBacklinks).Methods("GET")
	apiv1.HandleFunc("/content/batch-publish", a.APIBatchPublishContent).Methods("POST")
	apiv1.Handle("/content/bulk-create", middleware.BulkUpdateLimiter()(http.HandlerFunc(a.APIBulkCreateContent))).Methods("POST")
	apiv1.Handle("/content/bulk-update", middleware.BulkUpdateLimiter()(http.HandlerFunc(a.APIBulkUpdateContent))).Methods("POST")
	apiv1.Handle("/content/bulk-field-op", middleware.BulkUpdateLimiter()(http.HandlerFunc(a.APIBulkFieldOperation))).Methods("POST")
	apiv1.Handle("/content/export", middleware.ExportLimiter()(http.HandlerFunc(a.APIExportContent))).Methods("POST")
	apiv1.HandleFunc("/content/{id}", a.APIGetContent).Methods("GET")
	apiv1.HandleFunc("/content/{id}", a.APIUpdateContent).Methods("PUT")
	apiv1.HandleFunc("/content/{id}", a.APIDeleteContent).Methods("DELETE")
	apiv1.HandleFunc("/content/{id}/restore", a.APIRestoreContent).Methods("POST")
	apiv1.HandleFunc("/content/{id}/publish", a.APIPublishContent).Methods("POST")
	apiv1.HandleFunc("/content/{id}/unpublish", a.APIUnpublishContent).Methods("POST")
	apiv1.HandleFunc("/content/{id}/preview", a.APIPreviewContent).Methods("GET", "POST")
	apiv1.HandleFunc("/content/{id}/versions", a.APIListContentVersions).Methods("GET")
	apiv1.HandleFunc("/content/{id}/versions/{version}", a.APIGetContentVersion).Methods("GET")
	apiv1.HandleFunc("/content/{id}/versions/{version}/revert", a.APIRevertContentVersion).Methods("POST")

	// Templates
	apiv1.HandleFunc("/templates", a.APIListTemplates).Methods("GET")
	apiv1.HandleFunc("/templates", a.APICreateTemplate).Methods("POST")
	apiv1.HandleFunc("/templates/{id}", a.APIGetTemplate).Methods("GET")
	apiv1.HandleFunc("/templates/{id}", a.APIUpdateTemplate).Methods("PUT")
	apiv1.HandleFunc("/templates/{id}", a.APIDeleteTemplate).Methods("DELETE")

	// Assets
	apiv1.HandleFunc("/assets", a.APIListAssets).Methods("GET")
	apiv1.HandleFunc("/assets", a.APIUploadAsset).Methods("POST")
	apiv1.Handle("/assets/from-url", middleware.AssetFromURLLimiter()(http.HandlerFunc(a.APIUploadAssetFromURL))).Methods("POST")
	apiv1.HandleFunc("/assets/folders", a.APIListAssetFolders).Methods("GET")
	apiv1.HandleFunc("/assets/by-path", a.APIGetAssetByPath).Methods("GET")
	apiv1.HandleFunc("/assets/{id}", a.APIGetAsset).Methods("GET")
	apiv1.HandleFunc("/assets/{id}", a.APIDeleteAsset).Methods("DELETE")

	// Theme
	apiv1.HandleFunc("/theme", a.APIGetTheme).Methods("GET")
	apiv1.HandleFunc("/theme", a.APIUpdateTheme).Methods("PUT")
	apiv1.HandleFunc("/theme/versions", a.APIListThemeVersions).Methods("GET")
	apiv1.HandleFunc("/theme/versions/{version}", a.APIGetThemeVersion).Methods("GET")
	apiv1.HandleFunc("/theme/versions/{version}/revert", a.APIRevertThemeVersion).Methods("POST")
	apiv1.HandleFunc("/theme/versions/{version}/pin", a.APIPinThemeVersion).Methods("POST")
	apiv1.HandleFunc("/theme/versions/{version}/unpin", a.APIPinThemeVersion).Methods("POST")

	// Site Config
	apiv1.HandleFunc("/config", a.APIGetSiteConfig).Methods("GET")
	apiv1.HandleFunc("/config", a.APIUpdateSiteConfig).Methods("PUT")

	// Redirects
	apiv1.HandleFunc("/redirects", a.APIListRedirects).Methods("GET")
	apiv1.HandleFunc("/redirects", a.APICreateRedirect).Methods("POST")
	apiv1.HandleFunc("/redirects/{id}", a.APIGetRedirect).Methods("GET")
	apiv1.HandleFunc("/redirects/{id}", a.APIUpdateRedirect).Methods("PUT")
	apiv1.HandleFunc("/redirects/{id}", a.APIDeleteRedirect).Methods("DELETE")

	// Folders
	apiv1.HandleFunc("/folders", a.APIListFolders).Methods("GET")
	apiv1.HandleFunc("/folders", a.APICreateFolder).Methods("POST")
	apiv1.HandleFunc("/folders/{id}", a.APIGetFolder).Methods("GET")
	apiv1.HandleFunc("/folders/{id}", a.APIDeleteFolder).Methods("DELETE")

	// Collections
	apiv1.HandleFunc("/collections", a.APIListCollections).Methods("GET")
	apiv1.HandleFunc("/collections", a.APICreateCollection).Methods("POST")
	apiv1.HandleFunc("/collections/{id}", a.APIGetCollection).Methods("GET")
	apiv1.HandleFunc("/collections/{id}", a.APIUpdateCollection).Methods("PUT")
	apiv1.HandleFunc("/collections/{id}", a.APIDeleteCollection).Methods("DELETE")

	// Search
	apiv1.HandleFunc("/search", a.APISearchContent).Methods("GET")
	apiv1.HandleFunc("/search-replace/preview", a.APISearchReplacePreview).Methods("POST")
	apiv1.Handle("/search-replace/execute", middleware.SearchReplaceExecuteLimiter()(http.HandlerFunc(a.APISearchReplaceExecute))).Methods("POST")
	apiv1.HandleFunc("/search-replace/scoped/preview", a.APIScopedSearchReplacePreview).Methods("POST")
	apiv1.Handle("/search-replace/scoped/execute", middleware.SearchReplaceExecuteLimiter()(http.HandlerFunc(a.APIScopedSearchReplaceExecute))).Methods("POST")

	// End-user search (authenticated API)
	apiv1.HandleFunc("/end-user-search", a.APIEndUserSearch).Methods("GET")
	apiv1.HandleFunc("/end-user-search/suggest", a.APIEndUserSearchSuggest).Methods("GET")
	apiv1.Handle("/reindex-embeddings", middleware.ReindexLimiter()(http.HandlerFunc(a.APIReindexEmbeddings))).Methods("POST")

	// API Keys
	apiv1.HandleFunc("/api-keys", a.APIListAPIKeys).Methods("GET")
	apiv1.HandleFunc("/api-keys", a.APICreateAPIKey).Methods("POST")
	apiv1.HandleFunc("/api-keys/{id}", a.APIDeleteAPIKey).Methods("DELETE")

	// Snippets
	apiv1.HandleFunc("/snippets", a.APIListSnippets).Methods("GET")
	apiv1.HandleFunc("/snippets", a.APICreateSnippet).Methods("POST")
	apiv1.HandleFunc("/snippets/{id}", a.APIGetSnippet).Methods("GET")
	apiv1.HandleFunc("/snippets/{id}", a.APIUpdateSnippet).Methods("PUT")
	apiv1.HandleFunc("/snippets/{id}", a.APIDeleteSnippet).Methods("DELETE")

	// Forks
	apiv1.HandleFunc("/forks", a.APIListForks).Methods("GET")
	apiv1.HandleFunc("/forks", a.APICreateFork).Methods("POST")
	apiv1.HandleFunc("/forks/{id}", a.APIGetFork).Methods("GET")
	apiv1.HandleFunc("/forks/{id}", a.APIDeleteFork).Methods("DELETE")
	apiv1.HandleFunc("/forks/{id}/fork-page", a.APIForkPage).Methods("POST")
	apiv1.HandleFunc("/forks/{id}/pages", a.APIListForkPages).Methods("GET")
	apiv1.HandleFunc("/forks/{id}/pages/{pageID}", a.APIRemoveForkPage).Methods("DELETE")
	apiv1.HandleFunc("/forks/{id}/diff", a.APIForkDiff).Methods("GET")

	// Agent session ledger & rollback
	apiv1.HandleFunc("/agent-sessions/{id}/changes", a.APIAgentSessionChanges).Methods("GET")
	apiv1.HandleFunc("/agent-sessions/{id}/rollback", a.APIAgentSessionRollback).Methods("POST")

	// Maintenance scans (self-maintaining site routines)
	apiv1.HandleFunc("/maintenance/report", a.APIMaintenanceReport).Methods("GET")
	apiv1.HandleFunc("/indexnow", a.APIIndexNowStatus).Methods("GET")
	apiv1.HandleFunc("/seo", a.APIGetSEO).Methods("GET")
	apiv1.HandleFunc("/seo", a.APIUpdateSEO).Methods("PUT")
	apiv1.HandleFunc("/analytics/ai", a.APIAITraffic).Methods("GET")
	apiv1.HandleFunc("/indexnow", a.APIIndexNowUpdate).Methods("PUT")
	apiv1.HandleFunc("/indexnow/submit", a.APIIndexNowSubmit).Methods("POST")
	apiv1.HandleFunc("/maintenance/scan", a.APIMaintenanceScan).Methods("POST")
	apiv1.HandleFunc("/maintenance/backfill-published-dates", a.APIBackfillPublishedDates).Methods("POST")
	apiv1.HandleFunc("/maintenance/repair-fork-damage", a.APIRepairForkDamage).Methods("POST")
	apiv1.HandleFunc("/forks/{id}/merge", a.APIMergeFork).Methods("POST")
	apiv1.HandleFunc("/forks/{id}/archive", a.APIArchiveFork).Methods("POST")
	apiv1.HandleFunc("/forks/{id}/purge-copies", a.APIPurgeForkCopies).Methods("POST")

	// Regenerate (rate-limited: 2/min — full rebuild is expensive)
	apiv1.Handle("/regenerate", middleware.RegenerateLimiter()(http.HandlerFunc(a.APIRegenerateAllContent))).Methods("POST")

	// Webhooks API
	apiv1.HandleFunc("/webhooks", a.APIListWebhooks).Methods("GET")
	apiv1.HandleFunc("/webhooks", a.APICreateWebhook).Methods("POST")
	apiv1.HandleFunc("/webhooks/{id}", a.APIUpdateWebhook).Methods("PUT")
	apiv1.HandleFunc("/webhooks/{id}", a.APIDeleteWebhook).Methods("DELETE")
	apiv1.HandleFunc("/webhooks/{id}/regenerate-secret", a.APIRegenerateWebhookSecret).Methods("POST")
	apiv1.HandleFunc("/webhooks/{id}/deliveries", a.APIListWebhookDeliveries).Methods("GET")

	// Content locks API
	apiv1.HandleFunc("/content/{id}/lock", a.APIGetContentLock).Methods("GET")
	apiv1.HandleFunc("/content/{id}/lock", a.APIAcquireContentLock).Methods("POST")
	apiv1.HandleFunc("/content/{id}/lock", a.APIReleaseContentLock).Methods("DELETE")
	apiv1.HandleFunc("/content/{id}/lock/force", a.APIForceUnlockContent).Methods("POST")

	// Scheduled publish API
	apiv1.HandleFunc("/content/scheduled", a.APIListScheduledContent).Methods("GET")
	apiv1.HandleFunc("/content/{id}/schedule", a.APIScheduleContentPublish).Methods("POST")

	// Audit log API
	apiv1.HandleFunc("/audit", a.APIListAuditLogs).Methods("GET")

	// Link check API
	apiv1.HandleFunc("/link-check", a.APIStartLinkCheck).Methods("POST")
	apiv1.HandleFunc("/link-check/{id}", a.APIGetLinkCheckJob).Methods("GET")

	// Import pipeline
	apiv1.HandleFunc("/imports/sources", a.APIListImportSources).Methods("GET")
	apiv1.HandleFunc("/imports/sources", a.APICreateImportSource).Methods("POST")
	apiv1.HandleFunc("/imports/sources/{id}", a.APIUpdateImportSource).Methods("PUT")
	apiv1.HandleFunc("/imports/sources/{id}", a.APIDeleteImportSource).Methods("DELETE")
	apiv1.HandleFunc("/imports/sources/{id}/trigger", a.APITriggerImportSource).Methods("POST")
	apiv1.HandleFunc("/imports/markdown", a.APIImportMarkdown).Methods("POST")
	apiv1.HandleFunc("/imports/csv", a.APIImportCSV).Methods("POST")
	apiv1.HandleFunc("/imports/jobs", a.APIListImportJobs).Methods("GET")
	apiv1.HandleFunc("/imports/jobs/{id}", a.APIGetImportJob).Methods("GET")
	apiv1.HandleFunc("/imports/jobs/{id}/cancel", a.APICancelImportJob).Methods("POST")

	// Comments API
	apiv1.HandleFunc("/content/{id}/comments", a.APIListComments).Methods("GET")
	apiv1.Handle("/content/{id}/comments", middleware.CommentCreateLimiter()(http.HandlerFunc(a.APICreateComment))).Methods("POST")
	apiv1.HandleFunc("/content/{id}/comments/{cid}", a.APIDeleteComment).Methods("DELETE")
	apiv1.HandleFunc("/content/{id}/submit-approval", a.APISubmitForApproval).Methods("POST")

	// Users list (for @mention and approver search in UI)
	apiv1.HandleFunc("/users", a.APIListUsers).Methods("GET")

	// Approval workflows API
	apiv1.HandleFunc("/approval-workflows", a.APIListApprovalWorkflows).Methods("GET")
	apiv1.HandleFunc("/approval-workflows", a.APICreateApprovalWorkflow).Methods("POST")
	apiv1.HandleFunc("/approval-workflows/{id}", a.APIGetApprovalWorkflow).Methods("GET")
	apiv1.HandleFunc("/approval-workflows/{id}", a.APIUpdateApprovalWorkflow).Methods("PUT")
	apiv1.HandleFunc("/approval-workflows/{id}", a.APIDeleteApprovalWorkflow).Methods("DELETE")

	// Approval requests API
	apiv1.HandleFunc("/approval-requests", a.APIListApprovalRequests).Methods("GET")
	apiv1.HandleFunc("/approval-requests/{id}", a.APIGetApprovalRequest).Methods("GET")
	apiv1.HandleFunc("/approval-requests/{id}/approve", a.APIApproveRequest).Methods("POST")
	apiv1.HandleFunc("/approval-requests/{id}/reject", a.APIRejectRequest).Methods("POST")
	apiv1.HandleFunc("/approval-requests/{id}/cancel", a.APICancelRequest).Methods("POST")

	return apiv1
}
