package handlers

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/metavert-io/lightcms/v7/internal/auth"
	"github.com/metavert-io/lightcms/v7/internal/models"
	"github.com/metavert-io/lightcms/v7/internal/services"

	"github.com/gorilla/mux"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

// sandboxWriteRoutes are the only /api/v1 routes that change state which a
// sandboxed caller (auth.SessionUser.Sandboxed) may call; SandboxGate refuses
// every other non-read request from one. Each entry says why it cannot reach
// the live site. A new route is refused until it is added here, and
// TestSandboxRoutes_EveryWriteRouteIsDecided fails until it is listed here or
// in that test's list of refusals.
var sandboxWriteRoutes = map[string]string{
	"POST /api/v1/content":                     "creates a page inside a fork the caller created (fork_id is required)",
	"PUT /api/v1/content/{id}":                 "edits a page copy in a fork the caller created",
	"PUT /api/v1/content/by-path":              "edits a page copy in a fork the caller created",
	"POST /api/v1/content/{id}/preview":        "renders a page without saving anything",
	"POST /api/v1/content/export":              "reads content without changing it",
	"POST /api/v1/content/{id}/comments":       "comments on a page copy in a fork the caller created (APICreateComment)",
	"POST /api/v1/assets":                      "uploads a new image, PDF, font or text file; never replaces one, never HTML, JS, CSS or SVG",
	"POST /api/v1/assets/from-url":             "the same, fetched from a URL",
	"POST /api/v1/forks":                       "creates a fork owned by the caller",
	"DELETE /api/v1/forks/{id}":                "discards a fork the caller created",
	"POST /api/v1/forks/{id}/fork-page":        "copies a live page into a fork the caller created",
	"DELETE /api/v1/forks/{id}/pages/{pageID}": "removes a page copy from a fork the caller created",
	"POST /api/v1/forks/{id}/merge":            "merges only a fork the caller created, only with the merge permission, and only as reviewed (APIMergeFork)",
	"POST /api/v1/sandbox":                     "starts a sandbox: a new fork owned by the caller",
	"POST /api/v1/sandbox/end":                 "hands off or discards the caller's own sandbox fork",
}

// sandboxRefusal is the error a sandboxed caller gets for anything outside its forks.
const sandboxRefusal = "this connection is limited to its own forks. Start a sandbox (start_agent_sandbox, or POST /api/v1/sandbox) and make the change there for a person to review and merge"

// sandboxRefusalFor explains a refusal to the caller: a Full key or connection
// is held to its forks only while it has a sandbox open, and is told how to
// end it.
func sandboxRefusalFor(user *auth.SessionUser) string {
	if user != nil && !user.SandboxOnly && user.ActiveSandboxFork != "" {
		return "this connection has a sandbox open (fork " + user.ActiveSandboxFork + "), so it is limited to its own forks until the sandbox ends. Finish the work there, or end it with end_agent_sandbox (POST /api/v1/sandbox/end) to change the live site"
	}
	return sandboxRefusal
}

// SandboxGate refuses every state-changing request from a sandboxed caller
// except the routes in sandboxWriteRoutes, whichever tool or client sent it.
// The handlers behind those routes then hold the caller to its own forks.
// Reads pass, governed by the caller's permissions.
func SandboxGate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, _ := auth.UserFromAPIContext(r.Context())
		if !user.Sandboxed() || r.Method == http.MethodGet || r.Method == http.MethodHead || r.Method == http.MethodOptions {
			next.ServeHTTP(w, r)
			return
		}
		template := ""
		if route := mux.CurrentRoute(r); route != nil {
			template, _ = route.GetPathTemplate()
		}
		if _, ok := sandboxWriteRoutes[r.Method+" "+template]; !ok {
			writeJSONError(w, http.StatusForbidden, sandboxRefusalFor(user))
			return
		}
		next.ServeHTTP(w, r)
	})
}

func writeJSONError(w http.ResponseWriter, status int, message string) {
	(&APIHandler{}).jsonError(w, status, message)
}

// callerMayWriteFork reports whether the caller may change pages in fork,
// writing the refusal when not. A fork submitted for review can't be changed
// through an API key or connection; a person may still edit it, which
// changes its review digest. A sandboxed caller may write only to an active
// fork that it created.
func (a *APIHandler) callerMayWriteFork(w http.ResponseWriter, r *http.Request, fork *models.ContentFork) bool {
	user := a.getAPIUser(r)
	if fork.SubmittedAt != nil && fork.Status == "active" && user != nil && user.Credential != "" {
		a.jsonError(w, http.StatusConflict, "this fork was submitted for review and can't be changed any more; start a new sandbox for further changes")
		return false
	}
	if !user.Sandboxed() {
		return true
	}
	if fork.CreatedByCredential == "" || fork.CreatedByCredential != user.Credential {
		a.jsonError(w, http.StatusForbidden, "this connection can change only forks it created")
		return false
	}
	if fork.Status != "active" {
		a.jsonError(w, http.StatusConflict, "fork is "+fork.Status+"; it can't be changed")
		return false
	}
	return true
}

// callerMayWriteContent applies callerMayWriteFork to the fork a page belongs
// to; a sandboxed caller may not change a live page at all.
func (a *APIHandler) callerMayWriteContent(w http.ResponseWriter, r *http.Request, content *models.Content) bool {
	user := a.getAPIUser(r)
	if content.ForkID == nil {
		if user.Sandboxed() {
			a.jsonError(w, http.StatusForbidden, sandboxRefusalFor(user))
			return false
		}
		return true
	}
	fork, err := a.forkService.GetByID(r.Context(), *content.ForkID)
	if err != nil {
		a.jsonError(w, http.StatusNotFound, "fork not found")
		return false
	}
	return a.callerMayWriteFork(w, r, fork)
}

// sandboxUploadTypes are the file types a sandboxed caller may upload: ones a
// browser does not run. HTML, JavaScript, CSS, SVG, XML and JSON are left out.
var sandboxUploadTypes = map[string]bool{
	".jpg": true, ".jpeg": true, ".png": true, ".gif": true, ".webp": true,
	".avif": true, ".bmp": true, ".ico": true, ".pdf": true,
	".woff": true, ".woff2": true, ".ttf": true, ".otf": true, ".eot": true,
	".txt": true, ".csv": true,
}

// callerMayUpload holds a sandboxed caller to new files of a safe type: an
// upload goes live at its URL at once, and one at an existing path replaces
// the file the site is already serving.
func (a *APIHandler) callerMayUpload(w http.ResponseWriter, r *http.Request, filename, servePath string) bool {
	if !a.getAPIUser(r).Sandboxed() {
		return true
	}
	for _, name := range []string{filename, servePath} {
		if !sandboxUploadTypes[strings.ToLower(filepath.Ext(name))] {
			a.jsonError(w, http.StatusForbidden, "this connection can upload images, PDFs, fonts and text files only (not "+filepath.Ext(name)+")")
			return false
		}
	}
	exists, err := a.assetService.AssetExistsAt(r.Context(), servePath)
	if err != nil {
		a.jsonError(w, http.StatusInternalServerError, "could not check the asset path")
		return false
	}
	if exists {
		a.jsonError(w, http.StatusConflict, "an asset already exists at "+servePath+"; this connection can add new files but not replace them")
		return false
	}
	return true
}

// SetOAuthService wires in the OAuth service (sandbox state of connections).
func (a *APIHandler) SetOAuthService(s *services.OAuthService) { a.oauthService = s }

// setActiveSandbox records fork (nil: none) as the caller's sandbox, on the
// API key or OAuth connection it signed in with.
func (a *APIHandler) setActiveSandbox(ctx context.Context, user *auth.SessionUser, fork *primitive.ObjectID) error {
	kind, hexID, _ := strings.Cut(user.Credential, ":")
	id, err := primitive.ObjectIDFromHex(hexID)
	if err != nil {
		return fmt.Errorf("unrecognised credential %q", user.Credential)
	}
	switch kind {
	case "apikey":
		return a.apiKeyService.SetActiveSandbox(ctx, id, fork)
	case "oauth":
		if a.oauthService == nil {
			return fmt.Errorf("OAuth service unavailable")
		}
		return a.oauthService.SetActiveSandbox(ctx, id, fork)
	}
	return fmt.Errorf("unrecognised credential %q", user.Credential)
}

// claimSandbox records fork as the caller's sandbox only if none is open, and
// reports whether it did: of two requests racing to start a sandbox, one wins.
func (a *APIHandler) claimSandbox(ctx context.Context, user *auth.SessionUser, fork primitive.ObjectID) (bool, error) {
	kind, hexID, _ := strings.Cut(user.Credential, ":")
	id, err := primitive.ObjectIDFromHex(hexID)
	if err != nil {
		return false, fmt.Errorf("unrecognised credential %q", user.Credential)
	}
	switch kind {
	case "apikey":
		return a.apiKeyService.ClaimSandbox(ctx, id, fork)
	case "oauth":
		if a.oauthService == nil {
			return false, fmt.Errorf("OAuth service unavailable")
		}
		return a.oauthService.ClaimSandbox(ctx, id, fork)
	}
	return false, fmt.Errorf("unrecognised credential %q", user.Credential)
}

// activeSandbox returns the caller's open sandbox fork, or nil when none is
// open. A sandbox whose fork was deleted, or merged or archived, is closed
// here. Any other error is returned and the sandbox is left as it is: a
// passing database error must not lift it.
func (a *APIHandler) activeSandbox(ctx context.Context, user *auth.SessionUser) (*models.ContentFork, error) {
	if user.ActiveSandboxFork == "" {
		return nil, nil
	}
	if id, err := primitive.ObjectIDFromHex(user.ActiveSandboxFork); err == nil {
		fork, err := a.forkService.GetByID(ctx, id)
		if err == nil && fork.Status == "active" {
			return fork, nil
		}
		if err != nil && !errors.Is(err, mongo.ErrNoDocuments) {
			return nil, fmt.Errorf("reading the sandbox fork: %w", err)
		}
	}
	if err := a.setActiveSandbox(ctx, user, nil); err != nil {
		return nil, fmt.Errorf("closing a finished sandbox: %w", err)
	}
	user.ActiveSandboxFork = ""
	return nil, nil
}

// sandboxUnavailable answers when the sandbox state can't be read.
func (a *APIHandler) sandboxUnavailable(w http.ResponseWriter, err error) {
	fmt.Printf("Warning: sandbox state unavailable: %v\n", err)
	a.jsonError(w, http.StatusServiceUnavailable, "the sandbox state could not be read; try again")
}

func sandboxResponse(user *auth.SessionUser, fork *models.ContentFork) map[string]interface{} {
	resp := map[string]interface{}{
		"active": fork != nil,
		// restricted: the credential is set to Sandbox and stays confined
		// to its forks whether or not a sandbox is open.
		"restricted":          user.SandboxOnly,
		"can_merge_own_forks": user.CanMergeOwnForks,
	}
	if fork != nil {
		resp["fork_id"] = fork.ID.Hex()
		resp["name"] = fork.Name
		resp["review_url"] = "/cm/forks/" + fork.ID.Hex()
	}
	return resp
}

// APIGetSandbox reports the caller's sandbox.
// GET /api/v1/sandbox
func (a *APIHandler) APIGetSandbox(w http.ResponseWriter, r *http.Request) {
	if !a.requirePermission(w, r, auth.PermContentView) {
		return
	}
	user := a.getAPIUser(r)
	fork, err := a.activeSandbox(r.Context(), user)
	if err != nil {
		a.sandboxUnavailable(w, err)
		return
	}
	a.jsonResponse(w, http.StatusOK, sandboxResponse(user, fork))
}

// APIStartSandbox opens a sandbox: a new fork owned by the caller's key or
// connection, recorded on it so every later request is held to the fork.
// POST /api/v1/sandbox {"name": "...", "description": "..."}
func (a *APIHandler) APIStartSandbox(w http.ResponseWriter, r *http.Request) {
	if !a.requirePermission(w, r, auth.PermForkCreate) {
		return
	}
	user := a.getAPIUser(r)
	if user.Credential == "" {
		a.jsonError(w, http.StatusBadRequest, "a sandbox belongs to an API key or OAuth connection; a browser session has none")
		return
	}
	var body struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	if r.ContentLength != 0 {
		if err := a.decodeJSON(r, &body); err != nil {
			a.jsonError(w, http.StatusBadRequest, "invalid request body")
			return
		}
	}
	open, err := a.activeSandbox(r.Context(), user)
	if err != nil {
		a.sandboxUnavailable(w, err)
		return
	}
	if open != nil {
		a.jsonResponse(w, http.StatusConflict, map[string]interface{}{
			"error":   fmt.Sprintf("a sandbox (%q) is already open; end it first", open.Name),
			"fork_id": open.ID.Hex(),
		})
		return
	}
	if body.Name == "" {
		body.Name = "agent-session"
	}
	if body.Description == "" {
		body.Description = "Agent sandbox session"
	}

	userID, _ := primitive.ObjectIDFromHex(user.ID)
	fork, err := a.forkService.CreateWithCredential(r.Context(), body.Name, body.Description, userID, user.Email, user.Credential)
	if err != nil {
		a.jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	claimed, err := a.claimSandbox(r.Context(), user, fork.ID)
	if err != nil || !claimed {
		_ = a.forkService.Delete(r.Context(), fork.ID)
		if err != nil {
			a.jsonError(w, http.StatusInternalServerError, "could not open the sandbox: "+err.Error())
		} else {
			a.jsonError(w, http.StatusConflict, "another sandbox was opened at the same time; end it first")
		}
		return
	}
	user.ActiveSandboxFork = fork.ID.Hex()
	a.auditLog(r, "sandbox.start", "fork", fork.ID.Hex(), map[string]interface{}{"name": fork.Name})
	a.jsonResponse(w, http.StatusCreated, sandboxResponse(user, fork))
}

// APIEndSandbox closes the caller's sandbox. "submit" leaves the fork for a
// person to review; "discard" deletes it. A key or connection set to Sandbox
// stays confined to its forks afterwards.
// POST /api/v1/sandbox/end {"action": "submit"|"discard"}
func (a *APIHandler) APIEndSandbox(w http.ResponseWriter, r *http.Request) {
	if !a.requirePermission(w, r, auth.PermForkCreate) {
		return
	}
	user := a.getAPIUser(r)
	var body struct {
		Action string `json:"action"`
	}
	if err := a.decodeJSON(r, &body); err != nil || (body.Action != "submit" && body.Action != "discard") {
		a.jsonError(w, http.StatusBadRequest, `action must be "submit" or "discard"`)
		return
	}
	fork, err := a.activeSandbox(r.Context(), user)
	if err != nil {
		a.sandboxUnavailable(w, err)
		return
	}
	if fork == nil {
		a.jsonError(w, http.StatusConflict, "no sandbox is open")
		return
	}

	if body.Action == "discard" {
		if err := a.forkService.Delete(r.Context(), fork.ID); err != nil {
			a.jsonError(w, http.StatusInternalServerError, err.Error())
			return
		}
	} else if err := a.forkService.Submit(r.Context(), fork.ID, user.Email); err != nil {
		a.jsonError(w, http.StatusInternalServerError, "could not submit the fork: "+err.Error())
		return
	}
	if err := a.setActiveSandbox(r.Context(), user, nil); err != nil {
		a.jsonError(w, http.StatusInternalServerError, "could not close the sandbox: "+err.Error())
		return
	}
	user.ActiveSandboxFork = ""
	a.auditLog(r, "sandbox."+body.Action, "fork", fork.ID.Hex(), map[string]interface{}{"name": fork.Name})

	resp := map[string]interface{}{
		"action":     body.Action,
		"fork_id":    fork.ID.Hex(),
		"name":       fork.Name,
		"restricted": user.SandboxOnly,
	}
	if body.Action == "submit" {
		resp["review_url"] = "/cm/forks/" + fork.ID.Hex()
	}
	a.jsonResponse(w, http.StatusOK, resp)
}
