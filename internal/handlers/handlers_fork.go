package handlers

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/metavert-io/lightcms/v7/internal/auth"
	"github.com/metavert-io/lightcms/v7/internal/models"
	"github.com/metavert-io/lightcms/v7/internal/services"

	"github.com/gorilla/mux"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

const forkPreviewCookie = "lc_fork_preview"

// SetForkService attaches a ForkService to the Handler.
func (h *Handler) SetForkService(fs *services.ForkService) {
	h.forkService = fs
}

// ---------------------------------------------------------------------------
// Fork list
// ---------------------------------------------------------------------------

func (h *Handler) ListForks(w http.ResponseWriter, r *http.Request) {
	user, ok := h.auth.GetCurrentUser(r)
	if !ok {
		http.Redirect(w, r, "/cm/login", http.StatusSeeOther)
		return
	}
	if !auth.HasPermission(user.Role, auth.PermForkCreate) {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}
	ctx := r.Context()
	forks, err := h.forkService.List(ctx)
	if err != nil {
		forks = []models.ContentFork{}
	}
	// Attach page counts
	type forkWithCount struct {
		models.ContentFork
		PageCount int64
	}
	var enriched []forkWithCount
	for _, f := range forks {
		count, _ := h.forkService.GetPageCount(ctx, f.ID)
		// A merged fork's copies are deleted on merge; show what it merged.
		if f.Status == "merged" && count == 0 {
			count = int64(f.MergedCreated + f.MergedUpdated)
		}
		enriched = append(enriched, forkWithCount{f, count})
	}
	// If coming from a content page via "Fork to workspace", carry the content ID forward
	forkPageID := r.URL.Query().Get("fork_page")
	h.renderAdmin(w, r, "forks_list", map[string]interface{}{
		"Forks":      enriched,
		"CanMerge":   auth.HasPermission(user.Role, auth.PermForkMerge),
		"ForkPageID": forkPageID,
	})
}

// ---------------------------------------------------------------------------
// Create fork
// ---------------------------------------------------------------------------

func (h *Handler) NewFork(w http.ResponseWriter, r *http.Request) {
	user, ok := h.auth.GetCurrentUser(r)
	if !ok {
		http.Redirect(w, r, "/cm/login", http.StatusSeeOther)
		return
	}
	if !auth.HasPermission(user.Role, auth.PermForkCreate) {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}
	h.renderAdmin(w, r, "fork_form", map[string]interface{}{})
}

func (h *Handler) CreateFork(w http.ResponseWriter, r *http.Request) {
	user, ok := h.auth.GetCurrentUser(r)
	if !ok {
		http.Redirect(w, r, "/cm/login", http.StatusSeeOther)
		return
	}
	if !auth.HasPermission(user.Role, auth.PermForkCreate) {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}
	r.ParseForm()
	name := strings.TrimSpace(r.FormValue("name"))
	description := strings.TrimSpace(r.FormValue("description"))
	if name == "" {
		h.renderAdmin(w, r, "fork_form", map[string]interface{}{
			"Error": "Fork name is required",
		})
		return
	}
	ctx := r.Context()
	userID, _ := primitive.ObjectIDFromHex(user.ID)
	fork, err := h.forkService.Create(ctx, name, description, userID, user.Email)
	if err != nil {
		h.renderAdmin(w, r, "fork_form", map[string]interface{}{
			"Error": fmt.Sprintf("Failed to create fork: %v", err),
		})
		return
	}
	auditResource(r, fork.ID.Hex())
	http.Redirect(w, r, "/cm/forks/"+fork.ID.Hex(), http.StatusSeeOther)
}

// ---------------------------------------------------------------------------
// View / manage fork
// ---------------------------------------------------------------------------

func (h *Handler) ViewFork(w http.ResponseWriter, r *http.Request) {
	user, ok := h.auth.GetCurrentUser(r)
	if !ok {
		http.Redirect(w, r, "/cm/login", http.StatusSeeOther)
		return
	}
	if !auth.HasPermission(user.Role, auth.PermForkCreate) {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}
	vars := mux.Vars(r)
	forkID, err := primitive.ObjectIDFromHex(vars["id"])
	if err != nil {
		http.Error(w, "Invalid fork ID", http.StatusBadRequest)
		return
	}
	ctx := r.Context()
	fork, err := h.forkService.GetByID(ctx, forkID)
	if err != nil {
		http.Error(w, "Fork not found", http.StatusNotFound)
		return
	}
	pages, _ := h.forkService.ListPages(ctx, forkID)
	data := map[string]interface{}{
		"Fork":     fork,
		"Pages":    pages,
		"CanMerge": auth.HasPermission(user.Role, auth.PermForkMerge),
	}
	// The changes to review, and the digest the merge form sends back: the
	// merge is refused if anything changes after this page was rendered.
	if fork.Status == "active" {
		if review, err := h.forkService.Review(ctx, forkID); err == nil {
			data["Review"] = reviewView(review.Pages)
			data["ReviewDigest"] = review.Digest
		} else {
			data["ReviewError"] = "The changes could not be compared with the live site: " + err.Error()
		}
	}
	h.renderAdmin(w, r, "fork_detail", data)
}

// diffPreviewRunes is how much of a long value the review page shows before
// "Show all".
const diffPreviewRunes = 280

type fieldView struct {
	Name, Live, Fork          string
	LivePreview, ForkPreview  string
	LiveLong, ForkLong, IsNew bool
}

type pageReviewView struct {
	Path, Title, Status string
	Conflict            bool
	Fields              []fieldView
}

// reviewView prepares a fork review for the page: complete values, with a
// shortened preview of long ones (cut on a character boundary).
func reviewView(pages []services.ForkPageDiff) []pageReviewView {
	out := make([]pageReviewView, 0, len(pages))
	for _, p := range pages {
		v := pageReviewView{Path: p.Path, Title: p.Title, Status: p.Status, Conflict: p.Conflict}
		for _, f := range p.Fields {
			lp, ll := previewText(f.Live)
			fp, fl := previewText(f.Fork)
			v.Fields = append(v.Fields, fieldView{
				Name: f.Name, Live: f.Live, Fork: f.Fork,
				LivePreview: lp, ForkPreview: fp, LiveLong: ll, ForkLong: fl,
				IsNew: p.Status == "added",
			})
		}
		out = append(out, v)
	}
	return out
}

func previewText(s string) (string, bool) {
	runes := []rune(s)
	if len(runes) <= diffPreviewRunes {
		return s, false
	}
	return string(runes[:diffPreviewRunes]) + "…", true
}

// ---------------------------------------------------------------------------
// Fork a live page into the workspace
// ---------------------------------------------------------------------------

func (h *Handler) ForkPageHandler(w http.ResponseWriter, r *http.Request) {
	user, ok := h.auth.GetCurrentUser(r)
	if !ok {
		http.Redirect(w, r, "/cm/login", http.StatusSeeOther)
		return
	}
	if !auth.HasPermission(user.Role, auth.PermForkCreate) {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}
	vars := mux.Vars(r)
	forkID, err := primitive.ObjectIDFromHex(vars["id"])
	if err != nil {
		http.Error(w, "Invalid fork ID", http.StatusBadRequest)
		return
	}
	r.ParseForm()
	contentIDStr := r.FormValue("content_id")
	contentID, err := primitive.ObjectIDFromHex(contentIDStr)
	if err != nil {
		http.Error(w, "Invalid content ID", http.StatusBadRequest)
		return
	}
	ctx := r.Context()
	forkPage, err := h.forkService.ForkPage(ctx, forkID, contentID)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to fork page: %v", err), http.StatusInternalServerError)
		return
	}
	// Redirect to edit the fork copy
	http.Redirect(w, r, "/cm/content/"+forkPage.ID.Hex()+"?fork="+forkID.Hex(), http.StatusSeeOther)
}

// ---------------------------------------------------------------------------
// Remove a page from the fork
// ---------------------------------------------------------------------------

func (h *Handler) RemoveForkPage(w http.ResponseWriter, r *http.Request) {
	user, ok := h.auth.GetCurrentUser(r)
	if !ok {
		http.Redirect(w, r, "/cm/login", http.StatusSeeOther)
		return
	}
	if !auth.HasPermission(user.Role, auth.PermForkCreate) {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}
	vars := mux.Vars(r)
	forkID, err := primitive.ObjectIDFromHex(vars["id"])
	if err != nil {
		http.Error(w, "Invalid fork ID", http.StatusBadRequest)
		return
	}
	pageID, err := primitive.ObjectIDFromHex(vars["pageID"])
	if err != nil {
		http.Error(w, "Invalid page ID", http.StatusBadRequest)
		return
	}
	ctx := r.Context()
	if err := h.forkService.RemovePage(ctx, forkID, pageID); err != nil {
		http.Error(w, fmt.Sprintf("Failed to remove page: %v", err), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/cm/forks/"+forkID.Hex(), http.StatusSeeOther)
}

// ---------------------------------------------------------------------------
// Preview activation / deactivation
// ---------------------------------------------------------------------------

func (h *Handler) StartForkPreview(w http.ResponseWriter, r *http.Request) {
	user, ok := h.auth.GetCurrentUser(r)
	if !ok {
		http.Redirect(w, r, "/cm/login", http.StatusSeeOther)
		return
	}
	if !auth.HasPermission(user.Role, auth.PermForkCreate) {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}
	vars := mux.Vars(r)
	forkID, err := primitive.ObjectIDFromHex(vars["id"])
	if err != nil {
		http.Error(w, "Invalid fork ID", http.StatusBadRequest)
		return
	}
	ctx := r.Context()
	fork, err := h.forkService.GetByID(ctx, forkID)
	if err != nil {
		http.Error(w, "Fork not found", http.StatusNotFound)
		return
	}
	if fork.Status != "active" {
		http.Error(w, "Fork is not active", http.StatusBadRequest)
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     forkPreviewCookie,
		Value:    fork.PreviewToken,
		Path:     "/",
		Expires:  time.Now().Add(24 * time.Hour),
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
	// Redirect to home so they can browse the site in preview mode
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (h *Handler) ExitForkPreview(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     forkPreviewCookie,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
	})
	// Return to where they came from when that is a page of this site,
	// otherwise the forks list. The Referer header is whatever the linking
	// page was: redirecting to it as-is would be an open redirect.
	http.Redirect(w, r, sameSitePath(r, r.Header.Get("Referer"), "/cm/forks"), http.StatusSeeOther)
}

// sameSitePath turns a URL taken from the request (a Referer header, a
// return parameter) into a path on this site that is safe to redirect to,
// or returns fallback. It accepts a relative path, or an absolute URL whose
// host is the one the request was sent to, and keeps only path and query —
// never a scheme or host. "//host" and "/\host" (which browsers read as
// another origin) and anything unparsable fall back.
func sameSitePath(r *http.Request, raw, fallback string) string {
	if raw == "" || strings.ContainsAny(raw, "\\\r\n\t") {
		return fallback
	}
	u, err := url.Parse(raw)
	if err != nil {
		return fallback
	}
	if u.Scheme != "" || u.Host != "" {
		if (u.Scheme != "http" && u.Scheme != "https") || !strings.EqualFold(u.Host, r.Host) {
			return fallback
		}
	}
	p := u.EscapedPath()
	if !strings.HasPrefix(p, "/") || strings.HasPrefix(p, "//") || p == "/cm/forks/exit-preview" {
		return fallback
	}
	if u.RawQuery != "" {
		p += "?" + u.RawQuery
	}
	return p
}

// ---------------------------------------------------------------------------
// Merge fork into live (admin only)
// ---------------------------------------------------------------------------

func (h *Handler) MergeFork(w http.ResponseWriter, r *http.Request) {
	user, ok := h.auth.GetCurrentUser(r)
	if !ok {
		http.Redirect(w, r, "/cm/login", http.StatusSeeOther)
		return
	}
	if !auth.HasPermission(user.Role, auth.PermForkMerge) {
		http.Error(w, "Forbidden — only admins can merge forks", http.StatusForbidden)
		return
	}
	vars := mux.Vars(r)
	forkID, err := primitive.ObjectIDFromHex(vars["id"])
	if err != nil {
		http.Error(w, "Invalid fork ID", http.StatusBadRequest)
		return
	}
	r.ParseForm()
	publishNew := r.FormValue("publish_new") == "on"
	// The merge writes versions; stamp who did it.
	ctx := services.WithEditorEmail(r.Context(), user.Email)
	ctx = services.WithProvenance(ctx, services.Provenance{Actor: "human", Via: "ui"})
	userID, _ := primitive.ObjectIDFromHex(user.ID)
	result, err := h.forkService.Merge(ctx, forkID, userID, user.Email, publishNew, r.FormValue("digest"))
	if err != nil {
		msg := err.Error()
		if errors.Is(err, services.ErrForkChanged) || errors.Is(err, services.ErrReviewRequired) {
			msg = "The fork, or a live page it changes, was edited after you opened it. Nothing was merged. Open the fork again to review the current changes."
		}
		h.renderAdmin(w, r, "fork_merge_result", map[string]interface{}{
			"Error":  msg,
			"ForkID": forkID.Hex(),
		})
		return
	}
	if h.auditService != nil {
		h.auditService.LogAsync(models.AuditLog{
			UserID:     userID,
			UserEmail:  user.Email,
			Action:     "fork.merge",
			Resource:   "fork",
			ResourceID: forkID.Hex(),
			Details: map[string]interface{}{
				"updated":     result.Updated,
				"created":     result.Created,
				"conflicts":   len(result.Conflicts),
				"publish_new": publishNew,
				"created_ids": hexIDs(result.CreatedIDs),
				"updated_ids": hexIDs(result.UpdatedIDs),
			},
		})
	}
	h.renderAdmin(w, r, "fork_merge_result", map[string]interface{}{
		"Result":     result,
		"ForkID":     forkID.Hex(),
		"PublishNew": publishNew,
	})
}

// ---------------------------------------------------------------------------
// Archive / delete fork
// ---------------------------------------------------------------------------

func (h *Handler) ArchiveFork(w http.ResponseWriter, r *http.Request) {
	user, ok := h.auth.GetCurrentUser(r)
	if !ok {
		http.Redirect(w, r, "/cm/login", http.StatusSeeOther)
		return
	}
	if !auth.HasPermission(user.Role, auth.PermForkMerge) {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}
	vars := mux.Vars(r)
	forkID, err := primitive.ObjectIDFromHex(vars["id"])
	if err != nil {
		http.Error(w, "Invalid fork ID", http.StatusBadRequest)
		return
	}
	ctx := r.Context()
	if err := h.forkService.Archive(ctx, forkID); err != nil {
		http.Error(w, fmt.Sprintf("Failed to archive fork: %v", err), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/cm/forks", http.StatusSeeOther)
}

func (h *Handler) DeleteForkHandler(w http.ResponseWriter, r *http.Request) {
	user, ok := h.auth.GetCurrentUser(r)
	if !ok {
		http.Redirect(w, r, "/cm/login", http.StatusSeeOther)
		return
	}
	if !auth.HasPermission(user.Role, auth.PermForkMerge) {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}
	vars := mux.Vars(r)
	forkID, err := primitive.ObjectIDFromHex(vars["id"])
	if err != nil {
		http.Error(w, "Invalid fork ID", http.StatusBadRequest)
		return
	}
	ctx := r.Context()
	if err := h.forkService.Delete(ctx, forkID); err != nil {
		http.Error(w, fmt.Sprintf("Failed to delete fork: %v", err), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/cm/forks", http.StatusSeeOther)
}

// ---------------------------------------------------------------------------
// getActiveForkPreview checks the request for a valid fork preview cookie
// and returns the fork if one is active. Returns nil if no preview is active.
// ---------------------------------------------------------------------------

func (h *Handler) getActiveForkPreview(r *http.Request) *models.ContentFork {
	if h.forkService == nil {
		return nil
	}
	cookie, err := r.Cookie(forkPreviewCookie)
	if err != nil || cookie.Value == "" {
		return nil
	}
	fork, err := h.forkService.GetByPreviewToken(r.Context(), cookie.Value)
	if err != nil {
		return nil
	}
	return fork
}
