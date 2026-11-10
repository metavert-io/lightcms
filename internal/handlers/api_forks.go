package handlers

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/metavert-io/lightcms/v7/internal/auth"
	"github.com/metavert-io/lightcms/v7/internal/services"

	"github.com/gorilla/mux"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// SetForkService attaches a ForkService to the APIHandler.
func (a *APIHandler) SetForkService(fs *services.ForkService) {
	a.forkService = fs
}

// APIListForks returns all forks.
func (a *APIHandler) APIListForks(w http.ResponseWriter, r *http.Request) {
	if !a.requirePermission(w, r, auth.PermForkCreate) {
		return
	}
	forks, err := a.forkService.List(r.Context())
	if err != nil {
		a.jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}

	type forkWithCount struct {
		ID             string  `json:"id"`
		Name           string  `json:"name"`
		Description    string  `json:"description"`
		Status         string  `json:"status"`
		PageCount      int64   `json:"page_count"`
		CreatedByEmail string  `json:"created_by_email"`
		CreatedAt      string  `json:"created_at"`
		MergedAt       *string `json:"merged_at,omitempty"`
		MergedByEmail  string  `json:"merged_by_email,omitempty"`
		MergedCreated  int     `json:"merged_created,omitempty"`
		MergedUpdated  int     `json:"merged_updated,omitempty"`
		ArchivedAt     *string `json:"archived_at,omitempty"`
	}

	ctx := r.Context()
	result := make([]forkWithCount, 0, len(forks))
	for _, f := range forks {
		count, _ := a.forkService.GetPageCount(ctx, f.ID)
		item := forkWithCount{
			ID:             f.ID.Hex(),
			Name:           f.Name,
			Description:    f.Description,
			Status:         f.Status,
			PageCount:      count,
			CreatedByEmail: f.CreatedByEmail,
			CreatedAt:      f.CreatedAt.Format("2006-01-02 15:04:05"),
			MergedByEmail:  f.MergedByEmail,
			MergedCreated:  f.MergedCreated,
			MergedUpdated:  f.MergedUpdated,
		}
		if f.MergedAt != nil {
			s := f.MergedAt.Format("2006-01-02 15:04:05")
			item.MergedAt = &s
		}
		if f.ArchivedAt != nil {
			s := f.ArchivedAt.Format("2006-01-02 15:04:05")
			item.ArchivedAt = &s
		}
		result = append(result, item)
	}
	a.jsonResponse(w, http.StatusOK, result)
}

// APICreateFork creates a new fork workspace.
func (a *APIHandler) APICreateFork(w http.ResponseWriter, r *http.Request) {
	if !a.requirePermission(w, r, auth.PermForkCreate) {
		return
	}
	var body struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	if err := a.decodeJSON(r, &body); err != nil {
		a.jsonError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if body.Name == "" {
		a.jsonError(w, http.StatusBadRequest, "name is required")
		return
	}
	user := a.getAPIUser(r)
	var userID primitive.ObjectID
	var userEmail string
	if user != nil {
		userID, _ = primitive.ObjectIDFromHex(user.ID)
		userEmail = user.Email
	}
	fork, err := a.forkService.Create(r.Context(), body.Name, body.Description, userID, userEmail)
	if err != nil {
		a.jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	a.auditLog(r, "fork.create", "fork", fork.ID.Hex(), map[string]interface{}{"name": fork.Name})
	a.jsonResponse(w, http.StatusCreated, map[string]interface{}{
		"id":          fork.ID.Hex(),
		"name":        fork.Name,
		"description": fork.Description,
		"status":      fork.Status,
		"created_at":  fork.CreatedAt.Format("2006-01-02 15:04:05"),
		"message":     fmt.Sprintf("Fork '%s' created", fork.Name),
	})
}

// APIGetFork returns a fork with its page list.
func (a *APIHandler) APIGetFork(w http.ResponseWriter, r *http.Request) {
	if !a.requirePermission(w, r, auth.PermForkCreate) {
		return
	}
	vars := mux.Vars(r)
	forkID, err := primitive.ObjectIDFromHex(vars["id"])
	if err != nil {
		a.jsonError(w, http.StatusBadRequest, "invalid fork ID")
		return
	}
	ctx := r.Context()
	fork, err := a.forkService.GetByID(ctx, forkID)
	if err != nil {
		a.jsonError(w, http.StatusNotFound, "fork not found")
		return
	}
	pages, err := a.forkService.ListPages(ctx, forkID)
	if err != nil {
		pages = nil
	}

	type pageSummary struct {
		ID        string `json:"id"`
		Title     string `json:"title"`
		FullPath  string `json:"full_path"`
		UpdatedAt string `json:"updated_at"`
	}
	summaries := make([]pageSummary, len(pages))
	for i, p := range pages {
		summaries[i] = pageSummary{
			ID:        p.ID.Hex(),
			Title:     p.Title,
			FullPath:  p.FullPath,
			UpdatedAt: p.UpdatedAt.Format("2006-01-02 15:04:05"),
		}
	}
	resp := map[string]interface{}{
		"id":               fork.ID.Hex(),
		"name":             fork.Name,
		"description":      fork.Description,
		"status":           fork.Status,
		"created_by_email": fork.CreatedByEmail,
		"created_at":       fork.CreatedAt.Format("2006-01-02 15:04:05"),
		"page_count":       len(pages),
		"pages":            summaries,
	}
	// A merged fork's page copies are deleted on merge; the counts recorded
	// then are what is left to show.
	if fork.Status == "merged" {
		resp["merged_created"] = fork.MergedCreated
		resp["merged_updated"] = fork.MergedUpdated
		resp["merged_by_email"] = fork.MergedByEmail
		if fork.MergedAt != nil {
			resp["merged_at"] = fork.MergedAt.Format("2006-01-02 15:04:05")
		}
	}
	a.jsonResponse(w, http.StatusOK, resp)
}

// APIForkPage copies a live page into a fork workspace.
// Accepts {"content_id": "..."} or {"path": "/..."} in the request body.
func (a *APIHandler) APIForkPage(w http.ResponseWriter, r *http.Request) {
	if !a.requirePermission(w, r, auth.PermForkCreate) {
		return
	}
	vars := mux.Vars(r)
	forkID, err := primitive.ObjectIDFromHex(vars["id"])
	if err != nil {
		a.jsonError(w, http.StatusBadRequest, "invalid fork ID")
		return
	}
	var body struct {
		ContentID string `json:"content_id"`
		Path      string `json:"path"`
	}
	if err := a.decodeJSON(r, &body); err != nil {
		a.jsonError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	ctx := r.Context()

	var contentID primitive.ObjectID
	if body.ContentID != "" {
		contentID, err = primitive.ObjectIDFromHex(body.ContentID)
		if err != nil {
			a.jsonError(w, http.StatusBadRequest, "invalid content_id")
			return
		}
	} else if body.Path != "" {
		// A page that already has a copy in this fork (including one created
		// inside the fork, which has no live page) resolves to that copy;
		// ForkPage then returns it unchanged. The agent sandbox relies on
		// this to edit its own new pages by path.
		if existing, ferr := a.forkService.GetForkPageByPath(ctx, forkID, body.Path); ferr == nil && existing != nil {
			contentID = existing.ID
		} else {
			// Otherwise resolve the path to the live page (never a fork copy).
			content, err := a.contentService.GetContentByPath(ctx, body.Path)
			if err != nil {
				a.jsonError(w, http.StatusNotFound, fmt.Sprintf("content not found at path: %s", body.Path))
				return
			}
			contentID = content.ID
		}
	} else {
		a.jsonError(w, http.StatusBadRequest, "content_id or path is required")
		return
	}

	forkPage, err := a.forkService.ForkPage(ctx, forkID, contentID)
	if err != nil {
		a.jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	a.auditLog(r, "fork.add_page", "fork", forkID.Hex(), map[string]interface{}{
		"path":         forkPage.FullPath,
		"fork_page_id": forkPage.ID.Hex(),
	})
	a.jsonResponse(w, http.StatusOK, map[string]interface{}{
		"id":        forkPage.ID.Hex(),
		"full_path": forkPage.FullPath,
		"title":     forkPage.Title,
		"message":   fmt.Sprintf("Page '%s' added to fork. Use update_content with id='%s' to edit it.", forkPage.FullPath, forkPage.ID.Hex()),
	})
}

// APIListForkPages lists all pages in a fork.
func (a *APIHandler) APIListForkPages(w http.ResponseWriter, r *http.Request) {
	if !a.requirePermission(w, r, auth.PermForkCreate) {
		return
	}
	vars := mux.Vars(r)
	forkID, err := primitive.ObjectIDFromHex(vars["id"])
	if err != nil {
		a.jsonError(w, http.StatusBadRequest, "invalid fork ID")
		return
	}
	pages, err := a.forkService.ListPages(r.Context(), forkID)
	if err != nil {
		a.jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	type pageSummary struct {
		ID        string `json:"id"`
		Title     string `json:"title"`
		FullPath  string `json:"full_path"`
		UpdatedAt string `json:"updated_at"`
	}
	summaries := make([]pageSummary, len(pages))
	for i, p := range pages {
		summaries[i] = pageSummary{
			ID:        p.ID.Hex(),
			Title:     p.Title,
			FullPath:  p.FullPath,
			UpdatedAt: p.UpdatedAt.Format("2006-01-02 15:04:05"),
		}
	}
	a.jsonResponse(w, http.StatusOK, summaries)
}

// APIRemoveForkPage removes a page from a fork (deletes the fork copy).
func (a *APIHandler) APIRemoveForkPage(w http.ResponseWriter, r *http.Request) {
	if !a.requirePermission(w, r, auth.PermForkCreate) {
		return
	}
	vars := mux.Vars(r)
	forkID, err := primitive.ObjectIDFromHex(vars["id"])
	if err != nil {
		a.jsonError(w, http.StatusBadRequest, "invalid fork ID")
		return
	}
	pageID, err := primitive.ObjectIDFromHex(vars["pageID"])
	if err != nil {
		a.jsonError(w, http.StatusBadRequest, "invalid page ID")
		return
	}
	if err := a.forkService.RemovePage(r.Context(), forkID, pageID); err != nil {
		a.jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	a.jsonResponse(w, http.StatusOK, map[string]interface{}{"success": true})
}

// APIMergeFork merges all fork pages into live (admin only).
// Optional body: {"publish_new": true} also publishes the pages the merge
// creates (held pages stay drafts). The body may be omitted.
func (a *APIHandler) APIMergeFork(w http.ResponseWriter, r *http.Request) {
	if !a.requirePermission(w, r, auth.PermForkMerge) {
		return
	}
	vars := mux.Vars(r)
	forkID, err := primitive.ObjectIDFromHex(vars["id"])
	if err != nil {
		a.jsonError(w, http.StatusBadRequest, "invalid fork ID")
		return
	}
	var body struct {
		PublishNew bool `json:"publish_new"`
	}
	if raw, _ := io.ReadAll(io.LimitReader(r.Body, 1<<20)); len(raw) > 0 {
		if err := json.Unmarshal(raw, &body); err != nil {
			a.jsonError(w, http.StatusBadRequest, "invalid JSON")
			return
		}
	}
	if body.PublishNew && !a.requirePermission(w, r, auth.PermContentPublish) {
		return
	}
	user := a.getAPIUser(r)
	var userID primitive.ObjectID
	var userEmail string
	if user != nil {
		userID, _ = primitive.ObjectIDFromHex(user.ID)
		userEmail = user.Email
	}
	result, err := a.forkService.Merge(r.Context(), forkID, userID, userEmail, body.PublishNew)
	if err != nil {
		a.jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	a.auditLog(r, "fork.merge", "fork", forkID.Hex(), map[string]interface{}{
		"updated":     result.Updated,
		"created":     result.Created,
		"conflicts":   len(result.Conflicts),
		"publish_new": body.PublishNew,
	})

	type conflictSummary struct {
		Path      string `json:"path"`
		ForkTitle string `json:"fork_title"`
		LiveTitle string `json:"live_title"`
	}
	conflicts := make([]conflictSummary, len(result.Conflicts))
	for i, c := range result.Conflicts {
		conflicts[i] = conflictSummary{
			Path:      c.LivePath,
			ForkTitle: c.ForkItem.Title,
			LiveTitle: c.LiveTitle,
		}
	}
	// Created pages that publish_new left as drafts (held, or publish failed).
	notPublished := make([]map[string]string, len(result.NotPublished))
	for i, n := range result.NotPublished {
		notPublished[i] = map[string]string{"id": n.ID.Hex(), "path": n.Path, "reason": n.Reason}
	}
	a.jsonResponse(w, http.StatusOK, map[string]interface{}{
		"success":       true,
		"updated":       result.Updated,
		"created":       result.Created,
		"created_ids":   hexIDs(result.CreatedIDs),
		"updated_ids":   hexIDs(result.UpdatedIDs),
		"publish_new":   body.PublishNew,
		"not_published": notPublished,
		"conflicts":     conflicts,
		"message":       fmt.Sprintf("Fork merged: %d updated, %d created, %d conflicts (fork won)", result.Updated, result.Created, len(result.Conflicts)),
	})
}

// hexIDs renders ObjectIDs as hex strings (never nil, so JSON gets []).
func hexIDs(ids []primitive.ObjectID) []string {
	out := make([]string, len(ids))
	for i, id := range ids {
		out[i] = id.Hex()
	}
	return out
}

// APIArchiveFork archives a fork without merging (admin only).
func (a *APIHandler) APIArchiveFork(w http.ResponseWriter, r *http.Request) {
	if !a.requirePermission(w, r, auth.PermForkMerge) {
		return
	}
	vars := mux.Vars(r)
	forkID, err := primitive.ObjectIDFromHex(vars["id"])
	if err != nil {
		a.jsonError(w, http.StatusBadRequest, "invalid fork ID")
		return
	}
	if err := a.forkService.Archive(r.Context(), forkID); err != nil {
		a.jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	a.auditLog(r, "fork.archive", "fork", forkID.Hex(), nil)
	a.jsonResponse(w, http.StatusOK, map[string]interface{}{"success": true})
}

// APIPurgeForkCopies deletes the page copies still attached to a merged or
// archived fork (admin only). ?dry_run=true lists them without deleting.
// POST /api/v1/forks/{id}/purge-copies
func (a *APIHandler) APIPurgeForkCopies(w http.ResponseWriter, r *http.Request) {
	if !a.requirePermission(w, r, auth.PermForkMerge) {
		return
	}
	vars := mux.Vars(r)
	forkID, err := primitive.ObjectIDFromHex(vars["id"])
	if err != nil {
		a.jsonError(w, http.StatusBadRequest, "invalid fork ID")
		return
	}
	fork, err := a.forkService.GetByID(r.Context(), forkID)
	if err != nil {
		a.jsonError(w, http.StatusNotFound, "fork not found")
		return
	}
	if fork.Status != "merged" && fork.Status != "archived" {
		a.jsonError(w, http.StatusConflict, fmt.Sprintf("fork is %s; only merged or archived forks can be purged", fork.Status))
		return
	}
	dryRun := r.URL.Query().Get("dry_run") == "true"
	copies, err := a.forkService.PurgeCopies(r.Context(), forkID, dryRun)
	if err != nil {
		a.jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	resp := map[string]interface{}{
		"fork_id": forkID.Hex(),
		"dry_run": dryRun,
		"count":   len(copies),
	}
	if dryRun {
		resp["copies"] = copies
	} else {
		resp["deleted"] = len(copies)
		a.auditLog(r, "fork.purge_copies", "fork", forkID.Hex(), map[string]interface{}{
			"deleted": len(copies), "status": fork.Status,
		})
	}
	a.jsonResponse(w, http.StatusOK, resp)
}

// APIDeleteFork permanently deletes a fork and all its pages (admin only).
func (a *APIHandler) APIDeleteFork(w http.ResponseWriter, r *http.Request) {
	if !a.requirePermission(w, r, auth.PermForkMerge) {
		return
	}
	vars := mux.Vars(r)
	forkID, err := primitive.ObjectIDFromHex(vars["id"])
	if err != nil {
		a.jsonError(w, http.StatusBadRequest, "invalid fork ID")
		return
	}
	if err := a.forkService.Delete(r.Context(), forkID); err != nil {
		a.jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	a.auditLog(r, "fork.delete", "fork", forkID.Hex(), nil)
	a.jsonResponse(w, http.StatusOK, map[string]interface{}{"success": true})
}

// APIForkDiff returns per-field differences between each fork page and its
// live counterpart — the review surface for fork ("content PR") approval.
func (a *APIHandler) APIForkDiff(w http.ResponseWriter, r *http.Request) {
	if !a.requirePermission(w, r, auth.PermForkCreate) {
		return
	}
	vars := mux.Vars(r)
	forkID, err := primitive.ObjectIDFromHex(vars["id"])
	if err != nil {
		a.jsonError(w, http.StatusBadRequest, "invalid fork ID")
		return
	}
	diffs, err := a.forkService.Diff(r.Context(), forkID)
	if err != nil {
		a.jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	resp := map[string]interface{}{
		"fork_id": forkID.Hex(),
		"pages":   diffs,
	}
	// After a merge there are no copies left to diff; report what it did.
	if fork, err := a.forkService.GetByID(r.Context(), forkID); err == nil {
		resp["status"] = fork.Status
		if fork.Status == "merged" {
			resp["merged_created"] = fork.MergedCreated
			resp["merged_updated"] = fork.MergedUpdated
		}
	}
	a.jsonResponse(w, http.StatusOK, resp)
}
