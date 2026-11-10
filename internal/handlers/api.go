package handlers

import (
	"encoding/json"
	"io"
	"log"
	"net/http"

	"github.com/metavert-io/lightcms/v7/internal/auth"
	"github.com/metavert-io/lightcms/v7/internal/models"
	"github.com/metavert-io/lightcms/v7/internal/services"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// APIHandler handles REST API endpoints (JSON-only, no sessions/templates)
type APIHandler struct {
	contentService      *services.ContentService
	templateService     *services.TemplateService
	assetService        *services.AssetService
	settingsService     *services.SettingsService
	apiKeyService       *services.APIKeyService
	searchService       *services.SearchService
	auditService        *services.AuditService
	snippetService      *services.SnippetService
	forkService         *services.ForkService
	importService       *services.ImportService
	webhookService      *services.WebhookService
	lockService         *services.LockService
	linkCheckerService  *services.LinkCheckerService
	commentService      *services.CommentService
	approvalService     *services.ApprovalService
	userService         *services.UserService
	agentSessionService *services.AgentSessionService
	maintenanceService  *services.MaintenanceService
	indexNowService     *services.IndexNowService
	seoService          *services.SEOService
	analyticsService    *services.AnalyticsService
	oauthService        *services.OAuthService
	baseURL             string
}

// SetBaseURL sets the public site URL (used for previews such as robots.txt).
func (a *APIHandler) SetBaseURL(u string) { a.baseURL = u }

// SetCommentService wires in the comment service.
func (a *APIHandler) SetCommentService(cs *services.CommentService) { a.commentService = cs }

// SetApprovalService wires in the approval service.
func (a *APIHandler) SetApprovalService(as *services.ApprovalService) { a.approvalService = as }

// SetUserService wires in the user service (used for approver ID validation).
func (a *APIHandler) SetUserService(us *services.UserService) { a.userService = us }

// NewAPIHandler creates a new API handler
func NewAPIHandler(
	contentService *services.ContentService,
	templateService *services.TemplateService,
	assetService *services.AssetService,
	settingsService *services.SettingsService,
	apiKeyService *services.APIKeyService,
	auditService *services.AuditService,
	snippetService *services.SnippetService,
) *APIHandler {
	return &APIHandler{
		contentService:  contentService,
		templateService: templateService,
		assetService:    assetService,
		settingsService: settingsService,
		apiKeyService:   apiKeyService,
		auditService:    auditService,
		snippetService:  snippetService,
	}
}

// getAPIUser extracts the authenticated user from an API request context.
// Returns nil if no user is set (e.g., legacy/system key without user association).
func (a *APIHandler) getAPIUser(r *http.Request) *auth.SessionUser {
	user, _ := auth.UserFromAPIContext(r.Context())
	return user
}

// requirePermission checks if the authenticated API user has the required permission.
// Returns true if allowed, false if denied (and writes a 403 response).
// API keys without an associated user are rejected — legacy system keys must be
// re-created with a user owner via the admin UI or API.
func (a *APIHandler) requirePermission(w http.ResponseWriter, r *http.Request, perm string) bool {
	user := a.getAPIUser(r)
	if user == nil {
		log.Printf("[security] API request denied: key has no user context (perm=%s, path=%s)", perm, r.URL.Path)
		a.jsonError(w, http.StatusForbidden, "API key must be associated with a user — legacy keys without user context are no longer supported")
		return false
	}
	if !auth.UserHasPermission(user, perm) {
		msg := "insufficient permissions"
		if user.Sandboxed() && auth.HasPermission(user.Role, perm) {
			msg = sandboxRefusalFor(user)
		}
		a.jsonError(w, http.StatusForbidden, msg)
		return false
	}
	return true
}

// auditLog fires an async audit log entry for the current API user
func (a *APIHandler) auditLog(r *http.Request, action, resource, resourceID string, details map[string]interface{}) {
	user := a.getAPIUser(r)
	entry := models.AuditLog{
		Action:       action,
		Resource:     resource,
		ResourceID:   resourceID,
		Details:      details,
		AgentSession: r.Header.Get("X-Agent-Session"),
	}
	if user != nil {
		if oid, err := primitive.ObjectIDFromHex(user.ID); err == nil {
			entry.UserID = oid
		}
		entry.UserEmail = user.Email
		entry.ViaAPI = user.ViaAPIKey || user.ViaOAuth
		entry.Credential = user.Credential
	}
	a.auditService.LogAsync(entry)
}

// jsonResponse writes a JSON response with the given status code
func (a *APIHandler) jsonResponse(w http.ResponseWriter, statusCode int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(data)
}

// jsonError writes a JSON error response
func (a *APIHandler) jsonError(w http.ResponseWriter, statusCode int, message string) {
	a.jsonResponse(w, statusCode, map[string]interface{}{
		"error": message,
	})
}

// decodeJSON reads and decodes the request body into the given target
func (a *APIHandler) decodeJSON(r *http.Request, target interface{}) error {
	body, err := io.ReadAll(io.LimitReader(r.Body, 10<<20)) // 10MB limit
	if err != nil {
		return err
	}
	return json.Unmarshal(body, target)
}
