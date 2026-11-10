package handlers

import (
	"net/http"
	"strings"

	"github.com/gorilla/mux"

	"github.com/metavert-io/lightcms/v7/internal/auth"
)

// AdminRoute is one route under /cm together with the access it requires.
// Every admin route is declared in AdminRoutes and registered through
// RegisterAdminRoutes, which puts the access check in front of the handler:
// a route cannot be added without stating who may call it.
//
// Exactly one of Perms, Session or Public is set.
type AdminRoute struct {
	Method string
	Path   string // relative to /cm
	// Perms: the signed-in user's role needs any one of these permissions.
	// Where a route has a /api/v1 equivalent this is the permission that
	// endpoint passes to requirePermission.
	Perms []string
	// Session: any signed-in user may call the route; the value says why.
	Session string
	// Public: no session is needed; the value says why.
	Public string
	// JSON marks a fetch/XHR endpoint: it is refused with a JSON body
	// (401 without a session, 403 without the permission). Other routes are
	// refused with a redirect to the login page or the styled 403 page.
	JSON bool
	// Audit is the audit-log action written after the handler succeeds
	// ("template.update"; the part before the dot is the resource). It is the
	// name the /api/v1 equivalent logs. Every non-GET route has one, or is
	// listed with its reason in writeRoutesNotAudited (the guard test).
	Audit string

	handler http.HandlerFunc
}

// audited returns the route with its audit action set (see admin_audit_wrap.go).
func (rt AdminRoute) audited(action string) AdminRoute {
	rt.Audit = action
	return rt
}

// AdminRoutes returns the /cm route table in registration order (gorilla/mux
// matches in order, so /templates/new must stay ahead of /templates/{id}).
func (h *Handler) AdminRoutes() []AdminRoute {
	perm := func(method, path string, fn http.HandlerFunc, perms ...string) AdminRoute {
		return AdminRoute{Method: method, Path: path, Perms: perms, handler: fn}
	}
	jsonPerm := func(method, path string, fn http.HandlerFunc, perms ...string) AdminRoute {
		return AdminRoute{Method: method, Path: path, Perms: perms, JSON: true, handler: fn}
	}
	session := func(method, path string, fn http.HandlerFunc, why string) AdminRoute {
		return AdminRoute{Method: method, Path: path, Session: why, handler: fn}
	}
	public := func(method, path string, fn http.HandlerFunc, why string) AdminRoute {
		return AdminRoute{Method: method, Path: path, Public: why, handler: fn}
	}

	return []AdminRoute{
		// Sign-in, sign-out and password flows
		public("GET", "/login", h.LoginPage, "the sign-in form"),
		public("POST", "/login", h.LoginHandler, "signing in is what creates the session"),
		public("POST", "/logout", h.LogoutHandler, "signing out only clears the caller's own session"),
		session("GET", "/change-password", h.ForceChangePasswordPage, "forced change of the user's own password"),
		session("POST", "/change-password", h.ForceChangePasswordHandler, "forced change of the user's own password"),
		session("GET", "", h.AdminDashboard, "dashboard"),
		session("GET", "/", h.AdminDashboard, "dashboard"),

		// Templates
		perm("GET", "/templates", h.ListTemplates, auth.PermTemplateView),
		perm("GET", "/templates/new", h.NewTemplate, auth.PermTemplateCreate),
		perm("POST", "/templates/new", h.CreateTemplate, auth.PermTemplateCreate).audited("template.create"),
		perm("GET", "/templates/{id}", h.EditTemplate, auth.PermTemplateView),
		perm("POST", "/templates/{id}", h.UpdateTemplate, auth.PermTemplateEdit).audited("template.update"),
		perm("POST", "/templates/{id}/delete", h.DeleteTemplate, auth.PermTemplateDelete).audited("template.delete"),

		// Content. Saving a page needs content.edit; a contributor
		// (content.create only) may save a draft, which UpdateContent enforces.
		// Deleting needs content.delete, or fork.create for a fork copy, which
		// DeleteContent enforces.
		perm("GET", "/content", h.ListContent, auth.PermContentView),
		perm("GET", "/content/new", h.NewContent, auth.PermContentCreate),
		perm("GET", "/content/new/{templateID}", h.NewContentWithTemplate, auth.PermContentCreate),
		perm("POST", "/content/create", h.CreateContent, auth.PermContentCreate).audited("content.create"),
		perm("GET", "/content/{id}", h.EditContent, auth.PermContentView),
		perm("POST", "/content/{id}", h.UpdateContent, auth.PermContentEdit, auth.PermContentCreate).audited("content.update"),
		perm("POST", "/content/{id}/delete", h.DeleteContent, auth.PermContentDelete, auth.PermForkCreate).audited("content.delete"),
		perm("POST", "/content/{id}/undelete", h.UndeleteContent, auth.PermContentEdit).audited("content.restore"),
		perm("POST", "/content/{id}/regenerate", h.RegenerateContent, auth.PermContentPublish).audited("content.regenerate"),
		perm("GET", "/content/{id}/change-template/{template_id}", h.ChangeTemplatePreview, auth.PermContentEdit),
		perm("POST", "/content/{id}/change-template/{template_id}/confirm", h.ConfirmChangeTemplate, auth.PermContentEdit).audited("content.change_template"),
		perm("GET", "/content/{id}/versions", h.ListContentVersions, auth.PermContentView),
		perm("GET", "/content/{id}/versions/{version}/view", h.ViewContentVersion, auth.PermContentView),
		perm("GET", "/content/{id}/versions/{version}/diff", h.DiffContentVersion, auth.PermContentView),
		perm("POST", "/content/{id}/versions/{version}/revert", h.RevertContentVersion, auth.PermContentEdit).audited("content.revert"),

		// Collections
		perm("GET", "/collections", h.ListCollections, auth.PermSettingsView),
		perm("GET", "/collections/new", h.NewCollection, auth.PermSettingsEdit),
		perm("POST", "/collections/new", h.CreateCollection, auth.PermSettingsEdit).audited("collection.create"),
		perm("GET", "/collections/{id}", h.EditCollection, auth.PermSettingsView),
		perm("POST", "/collections/{id}", h.UpdateCollection, auth.PermSettingsEdit).audited("collection.update"),
		perm("POST", "/collections/{id}/delete", h.DeleteCollection, auth.PermSettingsEdit).audited("collection.delete"),

		// Theme
		perm("GET", "/theme", h.ThemeSettings, auth.PermSettingsView),
		perm("POST", "/theme", h.UpdateTheme, auth.PermSettingsEdit).audited("theme.update"),
		perm("GET", "/theme/versions", h.ThemeVersions, auth.PermSettingsView),
		perm("GET", "/theme/versions/{version}", h.ThemeVersionDiff, auth.PermSettingsView),
		perm("POST", "/theme/versions/{version}/revert", h.RevertThemeVersion, auth.PermSettingsEdit).audited("theme.revert"),

		// The user's own password
		session("GET", "/security", h.SecuritySettings, "the user's own password form"),
		session("POST", "/security", h.UpdatePassword, "changes only the caller's own password, and needs the current one"),

		// Site configuration (the page shows the Cloudflare token only to settings.edit)
		perm("GET", "/config", h.SiteConfiguration, auth.PermSettingsView),
		perm("POST", "/config", h.UpdateSiteConfiguration, auth.PermSettingsEdit).audited("config.update"),

		// Folders
		perm("GET", "/folders", h.ListFolders, auth.PermSettingsView),
		perm("GET", "/folders/new", h.NewFolder, auth.PermSettingsEdit),
		perm("POST", "/folders/new", h.CreateFolder, auth.PermSettingsEdit).audited("folder.create"),
		perm("GET", "/folders/{id}", h.EditFolder, auth.PermSettingsView),
		perm("POST", "/folders/{id}", h.UpdateFolder, auth.PermSettingsEdit).audited("folder.update"),
		perm("POST", "/folders/{id}/delete", h.DeleteFolder, auth.PermSettingsEdit).audited("folder.delete"),

		// Redirects
		perm("GET", "/redirects", h.ListRedirects, auth.PermSettingsView),
		perm("GET", "/redirects/new", h.NewRedirect, auth.PermSettingsEdit),
		perm("POST", "/redirects/new", h.CreateRedirect, auth.PermSettingsEdit).audited("redirect.create"),
		perm("GET", "/redirects/{id}", h.EditRedirect, auth.PermSettingsView),
		perm("POST", "/redirects/{id}", h.UpdateRedirect, auth.PermSettingsEdit).audited("redirect.update"),
		perm("POST", "/redirects/{id}/delete", h.DeleteRedirect, auth.PermSettingsEdit).audited("redirect.delete"),

		// Contact-form inbox (no /api/v1 equivalent)
		perm("GET", "/messages", h.ListContactMessages, auth.PermContentView),
		perm("POST", "/messages/mark-all-read", h.MarkAllMessagesRead, auth.PermContentEdit).audited("message.mark_all_read"),
		perm("GET", "/messages/{id}", h.ViewContactMessage, auth.PermContentView),
		perm("POST", "/messages/{id}/delete", h.DeleteContactMessage, auth.PermContentDelete).audited("message.delete"),

		// Assets
		perm("GET", "/assets", h.AssetLibrary, auth.PermAssetView),
		perm("GET", "/assets/upload", h.AssetUploadForm, auth.PermAssetUpload),
		perm("POST", "/assets/upload", h.AssetUpload, auth.PermAssetUpload).audited("asset.upload"),
		perm("POST", "/assets/{id}/delete", h.DeleteAsset, auth.PermAssetDelete).audited("asset.delete"),

		// API keys (own keys; DeleteAPIKey checks ownership below apikey.manage_all)
		perm("GET", "/api-keys", h.APIKeysPage, auth.PermAPIKeyManage),
		perm("GET", "/api-keys/new", h.NewAPIKeyPage, auth.PermAPIKeyManage),
		perm("POST", "/api-keys/new", h.CreateAPIKey, auth.PermAPIKeyManage).audited("apikey.create"),
		perm("POST", "/api-keys/{id}/delete", h.DeleteAPIKey, auth.PermAPIKeyManage).audited("apikey.delete"),
		perm("POST", "/api-keys/{id}/access", h.UpdateAPIKeyAccess, auth.PermAPIKeyManage).audited("apikey.update"),

		// Connected apps (OAuth connections): everyone manages their own; the
		// handlers refuse anyone else's below apikey.manage_all, and only a
		// role that can merge may let an app merge its own forks.
		session("GET", "/connected-apps", h.ConnectedAppsPage, "lists the signed-in user's own app connections"),
		session("POST", "/connected-apps/{id}/access", h.UpdateConnectedApp, "changes the signed-in user's own app connection").audited("oauth.update"),
		session("POST", "/connected-apps/{id}/revoke", h.RevokeConnectedApp, "disconnects the signed-in user's own app").audited("oauth.revoke"),

		// Approvals dashboard
		perm("GET", "/approvals", h.ApprovalsPage, auth.PermApprovalView),

		// User management
		perm("GET", "/users", h.UsersPage, auth.PermUserManage),
		perm("GET", "/users/new", h.NewUserPage, auth.PermUserManage),
		perm("POST", "/users/new", h.CreateUser, auth.PermUserManage),
		perm("GET", "/users/{id}", h.EditUserPage, auth.PermUserManage),
		perm("POST", "/users/{id}", h.UpdateUser, auth.PermUserManage),
		perm("POST", "/users/{id}/toggle-disabled", h.ToggleUserDisabled, auth.PermUserManage),
		perm("POST", "/users/{id}/reset-password", h.ResetUserPassword, auth.PermUserManage),

		// Audit log and analytics
		perm("GET", "/audit", h.AuditLogPage, auth.PermAuditView),
		perm("GET", "/analytics", h.AnalyticsPage, auth.PermAuditView),
		perm("GET", "/analytics/page", h.AnalyticsPageDetail, auth.PermAuditView),
		perm("GET", "/analytics/referrer", h.AnalyticsReferrerReport, auth.PermAuditView),
		perm("POST", "/audit/ratelimits/{ip}/clear", h.ClearRateLimit, auth.PermAuditView).audited("ratelimit.clear"),

		// Webhooks
		perm("GET", "/webhooks/docs", h.WebhookDocsPage, auth.PermContentEdit),
		perm("GET", "/webhooks/new", h.NewWebhookPage, auth.PermContentEdit),
		perm("GET", "/webhooks", h.WebhooksPage, auth.PermContentEdit),
		perm("POST", "/webhooks", h.CreateWebhook, auth.PermContentEdit).audited("webhook.create"),
		perm("GET", "/webhooks/{id}/edit", h.EditWebhookPage, auth.PermContentEdit),
		perm("GET", "/webhooks/{id}/deliveries", h.WebhookDeliveriesPage, auth.PermContentEdit),
		perm("POST", "/webhooks/{id}/regenerate-secret", h.RegenerateWebhookSecret, auth.PermContentEdit).audited("webhook.regenerate_secret"),
		perm("POST", "/webhooks/{id}", h.UpdateWebhook, auth.PermContentEdit).audited("webhook.update"),
		perm("POST", "/webhooks/{id}/delete", h.DeleteWebhook, auth.PermContentEdit).audited("webhook.delete"),

		// Import pipeline
		perm("GET", "/imports", h.ImportsPage, auth.PermContentEdit),
		perm("GET", "/imports/sources/new", h.NewRSSSourcePage, auth.PermContentEdit),
		perm("POST", "/imports/sources", h.CreateRSSSource, auth.PermContentEdit).audited("import_source.create"),
		perm("GET", "/imports/sources/{id}/edit", h.EditRSSSourcePage, auth.PermContentEdit),
		perm("POST", "/imports/sources/{id}", h.UpdateRSSSource, auth.PermContentEdit).audited("import_source.update"),
		perm("POST", "/imports/sources/{id}/delete", h.DeleteRSSSource, auth.PermContentEdit).audited("import_source.delete"),
		perm("POST", "/imports/sources/{id}/trigger", h.TriggerRSSSource, auth.PermContentEdit).audited("import_source.trigger"),
		perm("GET", "/imports/markdown", h.ImportMarkdownPage, auth.PermContentEdit),
		perm("POST", "/imports/markdown", h.DoImportMarkdown, auth.PermContentEdit).audited("import.markdown"),
		perm("GET", "/imports/csv", h.ImportCSVPage, auth.PermContentEdit),
		perm("POST", "/imports/csv", h.DoImportCSV, auth.PermContentEdit).audited("import.csv"),
		perm("GET", "/imports/{id}/stream", h.ImportJobSSE, auth.PermContentEdit),
		perm("GET", "/imports/{id}", h.ImportJobPage, auth.PermContentEdit),

		// Snippets (the API files them under the template permissions)
		perm("GET", "/snippets", h.ListSnippets, auth.PermTemplateView),
		perm("GET", "/snippets/new", h.NewSnippet, auth.PermTemplateEdit),
		perm("POST", "/snippets/new", h.CreateSnippet, auth.PermTemplateEdit).audited("snippet.create"),
		perm("GET", "/snippets/{id}", h.EditSnippet, auth.PermTemplateView),
		perm("POST", "/snippets/{id}", h.UpdateSnippet, auth.PermTemplateEdit).audited("snippet.update"),
		perm("POST", "/snippets/{id}/delete", h.DeleteSnippet, auth.PermTemplateEdit).audited("snippet.delete"),

		// Tools
		jsonPerm("GET", "/replace/preview", h.ReplacePreview, auth.PermSearchReplace),
		jsonPerm("POST", "/replace/execute", h.ReplaceExecute, auth.PermSearchReplace),
		perm("GET", "/tools/broken-links", h.BrokenLinkFinder, auth.PermContentEdit),
		jsonPerm("POST", "/tools/broken-links/fix", h.FixBrokenLink, auth.PermContentEdit).audited("content.fix_link"),
		perm("GET", "/tools/search", h.SearchToolPage, auth.PermSettingsView),
		jsonPerm("GET", "/tools/search/test", h.SearchToolTest, auth.PermSettingsView),
		jsonPerm("POST", "/tools/search/reindex", h.SearchToolReindex, auth.PermSettingsEdit).audited("search.reindex"),
		perm("POST", "/tools/search/config", h.SearchToolSaveConfig, auth.PermSettingsEdit).audited("search.config_update"),
		perm("GET", "/copilot", h.CopilotPage, auth.PermContentEdit),
		perm("GET", "/tools/agent", h.AgentToolPage, auth.PermSettingsEdit),
		perm("GET", "/tools/indexnow", h.IndexNowToolPage, auth.PermSettingsEdit),
		perm("GET", "/tools/seo", h.SEOToolPage, auth.PermSettingsEdit),
		perm("POST", "/tools/seo", h.SEOToolSave, auth.PermSettingsEdit),
		perm("GET", "/analytics/ai", h.AnalyticsAIPage, auth.PermAuditView),
		perm("POST", "/tools/indexnow", h.IndexNowToolAction, auth.PermSettingsEdit),
		perm("POST", "/tools/agent/config", h.AgentToolSaveConfig, auth.PermSettingsEdit),
		perm("POST", "/tools/agent/test", h.AgentToolSendTest, auth.PermSettingsEdit),
		jsonPerm("POST", "/copilot/chat", h.CopilotChat, auth.PermContentEdit),
		perm("GET", "/tools/chat", h.ChatWidgetPage, auth.PermSettingsView),
		perm("POST", "/tools/chat/config", h.ChatWidgetSaveConfig, auth.PermSettingsEdit).audited("chat.config_update"),

		// Content forks (editor+: create/preview; admin: merge/archive/delete)
		perm("GET", "/forks", h.ListForks, auth.PermForkCreate),
		perm("GET", "/forks/new", h.NewFork, auth.PermForkCreate),
		perm("POST", "/forks/new", h.CreateFork, auth.PermForkCreate).audited("fork.create"),
		public("GET", "/forks/exit-preview", h.ExitForkPreview, "only clears the caller's own preview cookie (the link sits on previewed public pages)"),
		perm("GET", "/forks/{id}", h.ViewFork, auth.PermForkCreate),
		perm("POST", "/forks/{id}/fork-page", h.ForkPageHandler, auth.PermForkCreate).audited("fork.add_page"),
		perm("POST", "/forks/{id}/pages/{pageID}/remove", h.RemoveForkPage, auth.PermForkCreate).audited("fork.remove_page"),
		perm("GET", "/forks/{id}/preview", h.StartForkPreview, auth.PermForkCreate),
		perm("POST", "/forks/{id}/merge", h.MergeFork, auth.PermForkMerge),
		perm("POST", "/forks/{id}/archive", h.ArchiveFork, auth.PermForkMerge).audited("fork.archive"),
		perm("POST", "/forks/{id}/delete", h.DeleteForkHandler, auth.PermForkMerge).audited("fork.delete"),
	}
}

// RegisterAdminRoutes registers every /cm route on the admin subrouter with
// its access check in front of the handler. It is the only place /cm routes
// are registered (TestAdminRoutes_EveryWriteRouteIsGuarded keeps it so).
func (h *Handler) RegisterAdminRoutes(admin *mux.Router) {
	for _, rt := range h.AdminRoutes() {
		admin.HandleFunc(rt.Path, h.guardAdminRoute(rt)).Methods(rt.Method)
	}
}

// guardAdminRoute wraps a route's handler in the access check its table
// entry declares.
func (h *Handler) guardAdminRoute(rt AdminRoute) http.HandlerFunc {
	if rt.Public != "" {
		return rt.handler
	}
	return func(w http.ResponseWriter, r *http.Request) {
		user, ok := h.auth.GetCurrentUser(r)
		if !ok {
			if rt.JSON {
				adminJSONError(w, http.StatusUnauthorized, "Unauthorized")
				return
			}
			http.Redirect(w, r, "/cm/login", http.StatusSeeOther)
			return
		}
		if rt.Session == "" && !roleHasAny(user.Role, rt.Perms) {
			h.refuseAdmin(w, r, rt.JSON, rt.Perms)
			return
		}
		if rt.Audit != "" {
			h.withAdminAudit(rt, user, w, r)
			return
		}
		rt.handler(w, r)
	}
}

// roleHasAny reports whether the role holds at least one of the permissions.
func roleHasAny(role string, perms []string) bool {
	for _, p := range perms {
		if auth.HasPermission(role, p) {
			return true
		}
	}
	return false
}

// forbiddenMessage is the text a refused user sees.
func forbiddenMessage(perms []string) string {
	msg := "Your role does not have permission to do this."
	if len(perms) > 0 {
		msg += " It needs " + strings.Join(perms, " or ") + "."
	}
	return msg + " Nothing was changed."
}

// refuseAdmin answers a signed-in user whose role may not call a route: a
// JSON 403 for fetch endpoints, the styled 403 page for everything else.
func (h *Handler) refuseAdmin(w http.ResponseWriter, r *http.Request, asJSON bool, perms []string) {
	h.refuseAdminMessage(w, r, asJSON, forbiddenMessage(perms))
}

func (h *Handler) refuseAdminMessage(w http.ResponseWriter, r *http.Request, asJSON bool, msg string) {
	if asJSON {
		adminJSONError(w, http.StatusForbidden, msg)
		return
	}
	h.renderAdminStatus(w, r, http.StatusForbidden, "forbidden", map[string]interface{}{
		"Message": msg,
	})
}
