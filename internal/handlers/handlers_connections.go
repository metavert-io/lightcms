package handlers

import (
	"log"
	"net/http"
	"net/url"
	"strings"

	"github.com/metavert-io/lightcms/v7/internal/auth"
	"github.com/metavert-io/lightcms/v7/internal/models"
	"github.com/metavert-io/lightcms/v7/internal/services"

	"github.com/gorilla/mux"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// SetOAuthService wires in the OAuth service (the Connected Apps page).
func (h *Handler) SetOAuthService(s *services.OAuthService) { h.oauthService = s }

// credentialRow is one API key or connection on an admin page.
type credentialRow struct {
	Key        *models.APIKey     // set for an API key
	Grant      *models.OAuthGrant // set for a connection
	OwnerEmail string
	// MergeEditable: the person viewing may change the merge setting (their
	// role can merge, and so can the owner's).
	MergeEditable bool
}

// ownerInfo returns an account's email and whether its role can merge forks.
func (h *Handler) ownerInfo(r *http.Request, cache map[primitive.ObjectID]*models.User, id primitive.ObjectID) (string, bool) {
	user, ok := cache[id]
	if !ok {
		user, _ = h.userService.GetByID(r.Context(), id)
		cache[id] = user
	}
	if user == nil {
		return "(deleted account)", false
	}
	return user.Email, auth.HasPermission(user.Role, auth.PermForkMerge)
}

// accessFromForm reads the Access and merge settings posted from an admin
// page. Only someone whose role can merge may turn merging on, and only for
// an owner whose role can merge too; mergeShown is false when the page did
// not offer the checkbox, which then leaves the setting as it was.
func accessFromForm(r *http.Request, editor *auth.SessionUser, ownerCanMerge, currentMerge bool) (services.AccessSettings, string) {
	access := services.AccessSettings{
		SandboxOnly:      r.FormValue("access") != "full",
		CanMergeOwnForks: currentMerge,
	}
	if r.FormValue("merge_shown") != "1" {
		return access, ""
	}
	wantMerge := r.FormValue("can_merge_own_forks") == "1"
	if wantMerge && !currentMerge && (!auth.HasPermission(editor.Role, auth.PermForkMerge) || !ownerCanMerge) {
		return access, "Only an admin can let an agent merge its own forks, and only for an account that can merge."
	}
	access.CanMergeOwnForks = wantMerge
	return access, ""
}

func redirectWithMessage(w http.ResponseWriter, r *http.Request, path, kind, msg string) {
	http.Redirect(w, r, path+"?"+kind+"="+url.QueryEscape(msg), http.StatusSeeOther)
}

// ConnectedAppsPage lists OAuth connections: the user's own, or everyone's
// for someone who manages all API keys.
func (h *Handler) ConnectedAppsPage(w http.ResponseWriter, r *http.Request) {
	user, ok := h.auth.GetCurrentUser(r)
	if !ok {
		http.Redirect(w, r, "/cm/login", http.StatusSeeOther)
		return
	}
	data := map[string]interface{}{
		"Success": r.URL.Query().Get("success"),
		"Error":   r.URL.Query().Get("error"),
	}
	if h.oauthService == nil {
		data["Error"] = "Connections are not available on this server."
		h.renderAdmin(w, r, "connected_apps", data)
		return
	}

	seeAll := auth.HasPermission(user.Role, auth.PermAPIKeyManageAll)
	var owner *primitive.ObjectID
	if !seeAll {
		id, err := primitive.ObjectIDFromHex(user.ID)
		if err != nil {
			http.Redirect(w, r, "/cm/login", http.StatusSeeOther)
			return
		}
		owner = &id
	}
	grants, err := h.oauthService.ListGrants(r.Context(), owner)
	if err != nil {
		log.Printf("Failed to list connections: %v", err)
		data["Error"] = "Could not load connections."
	}

	cache := map[primitive.ObjectID]*models.User{}
	rows := make([]credentialRow, 0, len(grants))
	for i := range grants {
		email, ownerCanMerge := h.ownerInfo(r, cache, grants[i].UserID)
		rows = append(rows, credentialRow{
			Grant:         &grants[i],
			OwnerEmail:    email,
			MergeEditable: ownerCanMerge && auth.HasPermission(user.Role, auth.PermForkMerge),
		})
	}
	data["Connections"] = rows
	data["SeeAll"] = seeAll
	h.renderAdmin(w, r, "connected_apps", data)
}

// connectionForEditor loads a connection the signed-in user may change: their
// own, or anyone's with apikey.manage_all. It writes the response when not.
func (h *Handler) connectionForEditor(w http.ResponseWriter, r *http.Request, user *auth.SessionUser) *models.OAuthGrant {
	id, err := primitive.ObjectIDFromHex(mux.Vars(r)["id"])
	if err != nil || h.oauthService == nil {
		auditSkip(r)
		http.NotFound(w, r)
		return nil
	}
	grant, err := h.oauthService.GetGrant(r.Context(), id)
	if err != nil || grant.RevokedAt != nil {
		auditSkip(r)
		http.NotFound(w, r)
		return nil
	}
	if grant.UserID.Hex() != user.ID && !auth.HasPermission(user.Role, auth.PermAPIKeyManageAll) {
		h.refuseAdminMessage(w, r, false, "That connection is not yours. Nothing was changed.")
		return nil
	}
	return grant
}

// UpdateConnectedApp changes a connection's access.
func (h *Handler) UpdateConnectedApp(w http.ResponseWriter, r *http.Request) {
	user, ok := h.auth.GetCurrentUser(r)
	if !ok {
		http.Redirect(w, r, "/cm/login", http.StatusSeeOther)
		return
	}
	grant := h.connectionForEditor(w, r, user)
	if grant == nil {
		return
	}
	_, ownerCanMerge := h.ownerInfo(r, map[primitive.ObjectID]*models.User{}, grant.UserID)
	access, refusal := accessFromForm(r, user, ownerCanMerge, grant.CanMergeOwnForks)
	if refusal != "" {
		redirectWithMessage(w, r, "/cm/connected-apps", "error", refusal)
		return
	}
	if err := h.oauthService.UpdateGrantAccess(r.Context(), grant.ID, access); err != nil {
		redirectWithMessage(w, r, "/cm/connected-apps", "error", "Could not save the change.")
		return
	}
	auditDetail(r, "sandbox_only", access.SandboxOnly)
	auditDetail(r, "can_merge_own_forks", access.CanMergeOwnForks)
	redirectWithMessage(w, r, "/cm/connected-apps", "success", "Saved access for "+grant.ClientName+".")
}

// RevokeConnectedApp disconnects an app: its tokens stop working at once.
func (h *Handler) RevokeConnectedApp(w http.ResponseWriter, r *http.Request) {
	user, ok := h.auth.GetCurrentUser(r)
	if !ok {
		http.Redirect(w, r, "/cm/login", http.StatusSeeOther)
		return
	}
	grant := h.connectionForEditor(w, r, user)
	if grant == nil {
		return
	}
	if err := h.oauthService.RevokeGrant(r.Context(), grant.ID); err != nil {
		redirectWithMessage(w, r, "/cm/connected-apps", "error", "Could not disconnect the app.")
		return
	}
	redirectWithMessage(w, r, "/cm/connected-apps", "success", "Disconnected "+grant.ClientName+".")
}

// UpdateAPIKeyAccess changes an API key's access: the caller's own key, or
// any key with apikey.manage_all.
func (h *Handler) UpdateAPIKeyAccess(w http.ResponseWriter, r *http.Request) {
	user, ok := h.auth.GetCurrentUser(r)
	if !ok {
		http.Redirect(w, r, "/cm/login", http.StatusSeeOther)
		return
	}
	id, err := primitive.ObjectIDFromHex(mux.Vars(r)["id"])
	if err != nil {
		auditSkip(r)
		http.NotFound(w, r)
		return
	}
	key, err := h.apiKeyService.GetAPIKey(r.Context(), id)
	if err != nil {
		auditSkip(r)
		http.NotFound(w, r)
		return
	}
	mine := key.UserID != nil && key.UserID.Hex() == user.ID
	if !mine && !auth.HasPermission(user.Role, auth.PermAPIKeyManageAll) {
		h.refuseAdminMessage(w, r, false, "That API key was not found or is not yours. Nothing was changed.")
		return
	}
	ownerCanMerge := false
	if key.UserID != nil {
		_, ownerCanMerge = h.ownerInfo(r, map[primitive.ObjectID]*models.User{}, *key.UserID)
	}
	access, refusal := accessFromForm(r, user, ownerCanMerge, key.CanMergeOwnForks)
	if refusal != "" {
		redirectWithMessage(w, r, "/cm/api-keys", "error", refusal)
		return
	}
	if err := h.apiKeyService.UpdateAccess(r.Context(), key.ID, access); err != nil {
		redirectWithMessage(w, r, "/cm/api-keys", "error", "Could not save the change.")
		return
	}
	auditDetail(r, "sandbox_only", access.SandboxOnly)
	auditDetail(r, "can_merge_own_forks", access.CanMergeOwnForks)
	redirectWithMessage(w, r, "/cm/api-keys", "success", "Saved access for "+strings.TrimSpace(key.Name)+".")
}
