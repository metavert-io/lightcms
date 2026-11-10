package handlers

import (
	"net/http"
	"strings"

	"github.com/metavert-io/lightcms/v7/internal/auth"

	"github.com/gorilla/mux"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// APIListComments returns all comments for a content item.
// GET /api/v1/content/{id}/comments
func (a *APIHandler) APIListComments(w http.ResponseWriter, r *http.Request) {
	if !a.requirePermission(w, r, auth.PermContentView) {
		return
	}
	id, err := primitive.ObjectIDFromHex(mux.Vars(r)["id"])
	if err != nil {
		a.jsonError(w, http.StatusBadRequest, "invalid content ID")
		return
	}
	comments, err := a.commentService.ListForContent(r.Context(), id)
	if err != nil {
		a.jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	a.jsonResponse(w, http.StatusOK, comments)
}

// APICreateComment posts a comment on a content item.
// POST /api/v1/content/{id}/comments
func (a *APIHandler) APICreateComment(w http.ResponseWriter, r *http.Request) {
	if !a.requirePermission(w, r, auth.PermDiscussionPost) {
		return
	}
	id, err := primitive.ObjectIDFromHex(mux.Vars(r)["id"])
	if err != nil {
		a.jsonError(w, http.StatusBadRequest, "invalid content ID")
		return
	}

	var req struct {
		Text     string   `json:"text"`
		Mentions []string `json:"mentions,omitempty"` // user ID strings
	}
	if err := a.decodeJSON(r, &req); err != nil || strings.TrimSpace(req.Text) == "" {
		a.jsonError(w, http.StatusBadRequest, "text is required")
		return
	}
	if len(req.Text) > 10000 {
		a.jsonError(w, http.StatusBadRequest, "comment text exceeds maximum length (10000 characters)")
		return
	}

	// A sandboxed caller comments only on pages in its own forks: comments
	// notify people and go out through webhooks.
	if a.getAPIUser(r).Sandboxed() {
		content, err := a.contentService.GetContent(r.Context(), id)
		if err != nil {
			a.jsonError(w, http.StatusNotFound, "content not found")
			return
		}
		if !a.callerMayWriteContent(w, r, content) {
			return
		}
	}

	user := a.getAPIUser(r)
	var (
		userID      primitive.ObjectID
		userEmail   string
		displayName string
	)
	if user != nil {
		userID, _ = primitive.ObjectIDFromHex(user.ID)
		userEmail = user.Email
		displayName = user.Email // API callers use email as display name fallback
	}

	var mentionIDs []primitive.ObjectID
	for _, m := range req.Mentions {
		if oid, err := primitive.ObjectIDFromHex(m); err == nil {
			mentionIDs = append(mentionIDs, oid)
		}
	}

	comment, err := a.commentService.Create(r.Context(), id, userID, userEmail, displayName,
		strings.TrimSpace(req.Text), mentionIDs)
	if err != nil {
		a.jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}

	a.auditLog(r, "comment.create", "content", id.Hex(), map[string]interface{}{"comment_id": comment.ID.Hex()})
	a.jsonResponse(w, http.StatusCreated, comment)
}

// APIDeleteComment deletes a comment. Requires admin role.
// DELETE /api/v1/content/{id}/comments/{cid}
func (a *APIHandler) APIDeleteComment(w http.ResponseWriter, r *http.Request) {
	if !a.requirePermission(w, r, auth.PermCommentDelete) {
		return
	}
	cid, err := primitive.ObjectIDFromHex(mux.Vars(r)["cid"])
	if err != nil {
		a.jsonError(w, http.StatusBadRequest, "invalid comment ID")
		return
	}
	if err := a.commentService.Delete(r.Context(), cid); err != nil {
		a.jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	a.auditLog(r, "comment.delete", "comment", cid.Hex(), nil)
	a.jsonResponse(w, http.StatusOK, map[string]interface{}{"success": true})
}
