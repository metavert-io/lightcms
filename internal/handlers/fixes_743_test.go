package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/mux"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"

	"github.com/metavert-io/lightcms/v7/internal/database"
	"github.com/metavert-io/lightcms/v7/internal/models"
)

// Tests for the 7.4.3 fixes in the handlers: live-only duplicate-path checks
// in the admin editor, the public queries (collection pages, page serving,
// the 404 page), API key deletion, audit logging of admin-UI mutations, the
// exit-preview redirect, folder renames and the removed routes.

// ---------------------------------------------------------------- item 2

// A fork copy shares its live page's path. It must not make the editor say
// "a page already exists" to a live page, and a live page must not block a
// copy: live pages are compared with live pages, a copy with its own fork.
func TestAdminDuplicatePathChecks_AreScopedToLiveOrFork(t *testing.T) {
	h, cleanup := newTestHandler(t)
	defer cleanup()
	ctx := context.Background()

	tmplID := seedTemplate(t, h.db, "Page", "page")
	forkID := createTestFork(t, h.db, "f2")
	otherFork := createTestFork(t, h.db, "f2-other")

	liveA := seedPage(t, h.db, models.Content{TemplateID: tmplID, Title: "A", Slug: "f2-a", FullPath: "/f2-a"})
	seedPage(t, h.db, models.Content{TemplateID: tmplID, Title: "Live taken", Slug: "f2-live-taken", FullPath: "/f2-live-taken"})
	// Paths that exist only as fork copies
	copyOnly := seedPage(t, h.db, models.Content{TemplateID: tmplID, Title: "Fork only", Slug: "f2-fork-only", FullPath: "/f2-fork-only", ForkID: &forkID})
	copy2 := seedPage(t, h.db, models.Content{TemplateID: tmplID, Title: "Fork two", Slug: "f2-fork-two", FullPath: "/f2-fork-two", ForkID: &forkID})
	seedPage(t, h.db, models.Content{TemplateID: tmplID, Title: "Other fork", Slug: "f2-other-fork", FullPath: "/f2-other-fork", ForkID: &otherFork})

	check := func(path, exclude string) bool {
		t.Helper()
		q := "path=" + url.QueryEscape(path)
		if exclude != "" {
			q += "&exclude=" + exclude
		}
		rr := getPageQ(t, h.CheckSlug, q, nil)
		if rr.Code != http.StatusOK {
			t.Fatalf("CheckSlug %s: %d", path, rr.Code)
		}
		return strings.Contains(rr.Body.String(), `"exists":true`)
	}

	// CheckSlug, asked for a new or live page
	if check("/f2-fork-only", "") {
		t.Error("CheckSlug: a fork copy makes its path look taken to a new live page")
	}
	if check("/f2-fork-only", liveA.Hex()) {
		t.Error("CheckSlug: a fork copy makes its path look taken to a live page being edited")
	}
	if !check("/f2-live-taken", "") || !check("/f2-live-taken", liveA.Hex()) {
		t.Error("CheckSlug: a live page's path is reported free")
	}
	if check("/f2-a", liveA.Hex()) {
		t.Error("CheckSlug: a page conflicts with itself")
	}
	// CheckSlug, asked for a fork copy: within its own fork
	if !check("/f2-fork-two", copyOnly.Hex()) {
		t.Error("CheckSlug: another copy in the same fork is not a conflict for a fork copy")
	}
	if check("/f2-live-taken", copyOnly.Hex()) {
		t.Error("CheckSlug: a live page is a conflict for a fork copy")
	}
	if check("/f2-other-fork", copyOnly.Hex()) {
		t.Error("CheckSlug: a copy in a different fork is a conflict")
	}

	// UpdateContent: the live page takes a path only a fork copy has
	rr := postForm(t, h.UpdateContent, url.Values{"title": {"A"}, "slug": {"f2-fork-only"}, "slug_rename_enabled": {"yes"}}, map[string]string{"id": liveA.Hex()})
	if loc := rr.Header().Get("Location"); rr.Code != http.StatusSeeOther || strings.Contains(loc, "error=") {
		t.Errorf("live page renamed onto a fork-only path: %d -> %q, want it saved", rr.Code, loc)
	}
	if c := loadPage(t, h.db, liveA); c.FullPath != "/f2-fork-only" {
		t.Errorf("live page path = %q, want /f2-fork-only", c.FullPath)
	}
	// ... but not a path another live page has
	rr = postForm(t, h.UpdateContent, url.Values{"title": {"A"}, "slug": {"f2-live-taken"}, "slug_rename_enabled": {"yes"}}, map[string]string{"id": liveA.Hex()})
	if loc := rr.Header().Get("Location"); !strings.Contains(loc, "error=slug_exists") {
		t.Errorf("live page renamed onto another live page's path: %d -> %q, want error=slug_exists", rr.Code, loc)
	}
	// A fork copy cannot take the path of another copy in its fork
	rr = postForm(t, h.UpdateContent, url.Values{"title": {"Fork only"}, "slug": {"f2-fork-two"}, "slug_rename_enabled": {"yes"}}, map[string]string{"id": copyOnly.Hex()})
	if loc := rr.Header().Get("Location"); !strings.Contains(loc, "error=slug_exists") {
		t.Errorf("fork copy renamed onto a sibling copy's path: %d -> %q, want error=slug_exists", rr.Code, loc)
	}
	if c := loadPage(t, h.db, copy2); c.FullPath != "/f2-fork-two" {
		t.Errorf("sibling copy changed: %q", c.FullPath)
	}

	// UndeleteContent: a deleted live page whose path a fork copy also has
	deleted := seedPage(t, h.db, models.Content{TemplateID: tmplID, Title: "Was deleted", Slug: "f2-undel", FullPath: "__deleted__/x/1", Deleted: true})
	seedPage(t, h.db, models.Content{TemplateID: tmplID, Title: "Copy of deleted", Slug: "f2-undel", FullPath: "/f2-undel", ForkID: &forkID})
	rr = postForm(t, h.UndeleteContent, nil, map[string]string{"id": deleted.Hex()})
	if loc := rr.Header().Get("Location"); strings.Contains(loc, "error=") {
		t.Errorf("restoring a page whose path only a fork copy has: -> %q, want it restored", loc)
	}
	if c := loadPage(t, h.db, deleted); c.Deleted || c.FullPath != "/f2-undel" {
		t.Errorf("page not restored: deleted=%v path=%q", c.Deleted, c.FullPath)
	}
	// ... and a real live conflict still refuses
	deleted2 := seedPage(t, h.db, models.Content{TemplateID: tmplID, Title: "Was deleted 2", Slug: "f2-live-taken", FullPath: "__deleted__/x/2", Deleted: true})
	rr = postForm(t, h.UndeleteContent, nil, map[string]string{"id": deleted2.Hex()})
	if loc := rr.Header().Get("Location"); !strings.Contains(loc, "error=path_conflict") {
		t.Errorf("restoring onto a live page's path: -> %q, want error=path_conflict", loc)
	}
	_ = ctx
}

// ---------------------------------------------------------------- item 3

func servePublic(h *Handler, path string) *httptest.ResponseRecorder {
	req := httptest.NewRequest("GET", path, nil)
	req = mux.SetURLVars(req, map[string]string{"slug": strings.TrimPrefix(path, "/")})
	rr := httptest.NewRecorder()
	h.ServePage(rr, req)
	return rr
}

// A public collection page lists live published pages only: not a fork copy
// flagged published, not a soft-deleted page, not a draft.
func TestServeCollection_LiveOnly(t *testing.T) {
	h, cleanup := newTestHandler(t)
	defer cleanup()
	ctx := context.Background()

	tmplID := seedTemplate(t, h.db, "Page", "page")
	forkID := createTestFork(t, h.db, "f3")
	if _, err := h.db.InsertOne(ctx, "collections", &models.Collection{
		Name: "F3 News", Slug: "f3-news", Category: "f3news", SortField: "title", SortOrder: "asc",
		ItemTemplate: `<li class="f3-item">{{.title}}</li>`, PageTemplate: `<ul id="f3-list">{{.items}}</ul>`,
	}); err != nil {
		t.Fatalf("seed collection: %v", err)
	}
	seedPage(t, h.db, models.Content{TemplateID: tmplID, Title: "F3 Live Item", FullPath: "/f3-live", Category: "f3news", Published: true})
	seedPage(t, h.db, models.Content{TemplateID: tmplID, Title: "F3 Fork Copy", FullPath: "/f3-copy", Category: "f3news", Published: true, ForkID: &forkID})
	seedPage(t, h.db, models.Content{TemplateID: tmplID, Title: "F3 Deleted Item", FullPath: "/f3-deleted", Category: "f3news", Published: true, Deleted: true})
	seedPage(t, h.db, models.Content{TemplateID: tmplID, Title: "F3 Draft Item", FullPath: "/f3-draft", Category: "f3news"})

	rr := servePublic(h, "/f3-news")
	body := rr.Body.String()
	if rr.Code != http.StatusOK || !strings.Contains(body, "F3 Live Item") {
		t.Fatalf("collection page: %d, live item present=%v", rr.Code, strings.Contains(body, "F3 Live Item"))
	}
	for _, leaked := range []string{"F3 Fork Copy", "F3 Deleted Item", "F3 Draft Item"} {
		if strings.Contains(body, leaked) {
			t.Errorf("public collection page lists %q", leaked)
		}
	}
	if n := strings.Count(body, `class="f3-item"`); n != 1 {
		t.Errorf("collection page has %d items, want 1", n)
	}
}

// A page soft-deleted through the service keeps its published flag and its
// path. It must not be served, by exact path, by slug or case-insensitively;
// and neither a deleted nor a fork-copy "404" page becomes the site's 404.
func TestServePage_NeverServesDeletedOrForkContent(t *testing.T) {
	h, cleanup := newTestHandler(t)
	defer cleanup()

	tmplID := seedTemplate(t, h.db, "Page", "page")
	forkID := createTestFork(t, h.db, "f3b")
	seedPage(t, h.db, models.Content{TemplateID: tmplID, Title: "F3B Deleted Secret", Slug: "f3b-deleted", FullPath: "/f3b-deleted", Published: true, Deleted: true,
		Data: map[string]interface{}{"Body": "deleted-body-marker"}})
	seedPage(t, h.db, models.Content{TemplateID: tmplID, Title: "F3B Fork Secret", Slug: "f3b-fork", FullPath: "/f3b-fork", Published: true, ForkID: &forkID})
	seedPage(t, h.db, models.Content{TemplateID: tmplID, Title: "F3B Live", Slug: "f3b-live", FullPath: "/f3b-live", Published: true})

	if rr := servePublic(h, "/f3b-live"); rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), "F3B Live") {
		t.Fatalf("live page: %d", rr.Code)
	}
	for _, p := range []string{"/f3b-deleted", "/F3B-DELETED", "/f3b-fork"} {
		rr := servePublic(h, p)
		if rr.Code != http.StatusNotFound {
			t.Errorf("GET %s: %d, want 404", p, rr.Code)
		}
		if b := rr.Body.String(); strings.Contains(b, "F3B Deleted Secret") || strings.Contains(b, "F3B Fork Secret") {
			t.Errorf("GET %s served content that is not on the site", p)
		}
		if loc := rr.Header().Get("Location"); loc != "" {
			t.Errorf("GET %s redirected to %q", p, loc)
		}
	}

	// The 404 page itself
	seedPage(t, h.db, models.Content{TemplateID: tmplID, Title: "F3B Deleted 404", Slug: "404", FullPath: "__deleted__/404", Published: true, Deleted: true})
	seedPage(t, h.db, models.Content{TemplateID: tmplID, Title: "F3B Fork 404", Slug: "404", FullPath: "/404", Published: true, ForkID: &forkID})
	rr := servePublic(h, "/f3b-no-such-page")
	if b := rr.Body.String(); rr.Code != http.StatusNotFound || strings.Contains(b, "F3B Deleted 404") || strings.Contains(b, "F3B Fork 404") {
		t.Errorf("404 response used a deleted or fork-copy 404 page (status %d)", rr.Code)
	}
}

// ---------------------------------------------------------------- item 6

// DELETE /api/v1/api-keys/{id} must not report success when nothing was
// deleted: 403 for a key that is not the caller's (as the admin UI route
// answers), 404 for an admin deleting a key that does not exist.
func TestAPIDeleteAPIKey_ReportsWhenNothingWasDeleted(t *testing.T) {
	ah, db, cleanup := newTestAPIHandler(t)
	defer cleanup()
	ctx := context.Background()

	self, _ := primitive.ObjectIDFromHex("000000000000000000000001")
	other := primitive.NewObjectID()
	_, mine, err := ah.apiKeyService.CreateAPIKeyForUser(ctx, "mine", "", &self)
	if err != nil {
		t.Fatalf("create key: %v", err)
	}
	_, theirs, _ := ah.apiKeyService.CreateAPIKeyForUser(ctx, "theirs", "", &other)
	has := func(id primitive.ObjectID) bool { return count(t, db, "api_keys", bson.M{"_id": id}) == 1 }

	del := func(role, id string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodDelete, "/api/v1/api-keys/"+id, nil)
		req = req.WithContext(injectRole(req.Context(), role))
		req = mux.SetURLVars(req, map[string]string{"id": id})
		rr := httptest.NewRecorder()
		ah.APIDeleteAPIKey(rr, req)
		return rr
	}

	rr := del("editor", theirs.ID.Hex())
	if rr.Code != http.StatusForbidden || strings.Contains(rr.Body.String(), `"success":true`) {
		t.Errorf("editor deleting another user's key: %d %s, want 403", rr.Code, rr.Body.String())
	}
	if !has(theirs.ID) {
		t.Fatal("editor deleted another user's key")
	}
	if rr := del("editor", primitive.NewObjectID().Hex()); rr.Code != http.StatusForbidden {
		t.Errorf("editor deleting a key that does not exist: %d, want 403", rr.Code)
	}
	time.Sleep(300 * time.Millisecond)
	if n := count(t, db, "audit_logs", bson.M{"action": "apikey.delete"}); n != 0 {
		t.Errorf("%d apikey.delete audit entries for deletes that deleted nothing", n)
	}

	if rr := del("editor", mine.ID.Hex()); rr.Code != http.StatusOK || has(mine.ID) {
		t.Errorf("editor deleting their own key: %d, still there=%v", rr.Code, has(mine.ID))
	}
	if rr := del("admin", primitive.NewObjectID().Hex()); rr.Code != http.StatusNotFound {
		t.Errorf("admin deleting a key that does not exist: %d %s, want 404", rr.Code, rr.Body.String())
	}
	if rr := del("admin", theirs.ID.Hex()); rr.Code != http.StatusOK || has(theirs.ID) {
		t.Errorf("admin deleting any key: %d, still there=%v", rr.Code, has(theirs.ID))
	}
}

// ---------------------------------------------------------------- item 7

// writeRoutesNotAudited is the complete list of non-GET /cm routes that the
// route table does not audit-log, each with the reason. Every other route
// that changes state names its audit action in AdminRoutes.
var writeRoutesNotAudited = map[string]string{
	"POST /cm/login":                      "the handler logs login.success / login.failure itself (there is no session yet for the wrapper to read)",
	"POST /cm/logout":                     "ends the caller's own session; changes no data",
	"POST /cm/change-password":            "the handler logs password.change itself",
	"POST /cm/security":                   "the handler logs password.change itself",
	"POST /cm/users/new":                  "the handler logs user.create itself, with the new user's email and role",
	"POST /cm/users/{id}":                 "the handler logs user.update itself, with the old and new role",
	"POST /cm/users/{id}/toggle-disabled": "the handler logs user.disable / user.enable itself (the action depends on the outcome)",
	"POST /cm/users/{id}/reset-password":  "the handler logs user.password_reset itself",
	"POST /cm/replace/execute":            "the handler logs content.search_replace itself, with the page and replacement counts",
	"POST /cm/tools/seo":                  "the handler logs seo.config_update itself",
	"POST /cm/tools/indexnow":             "the handler logs indexnow.<action> itself (one route, several actions)",
	"POST /cm/tools/agent/config":         "the handler logs agent.config_update itself",
	"POST /cm/tools/agent/test":           "the handler logs agent.test_digest itself",
	"POST /cm/copilot/chat":               "a chat turn is not a change; each tool the copilot runs writes its own audit entry",
	"POST /cm/forks/{id}/merge":           "the handler logs fork.merge itself, with the merge result",
}

var auditActionRe = regexp.MustCompile(`^[a-z_]+\.[a-z_]+$`)

// Every route that changes state either names an audit action — which the
// route wrapper writes, so the handler cannot forget — or is on the list
// above with a reason.
func TestAdminRoutes_EveryWriteRouteIsAudited(t *testing.T) {
	h := routesTestHandler()
	seen := map[string]bool{}
	for _, rt := range h.AdminRoutes() {
		key := routeKey(rt.Method, rt.Path)
		seen[key] = true
		if rt.Method == http.MethodGet {
			if rt.Audit != "" {
				t.Errorf("%s is a GET route with an audit action: a GET must not change anything", key)
			}
			continue
		}
		reason, listed := writeRoutesNotAudited[key]
		switch {
		case rt.Audit == "" && !listed:
			t.Errorf("%s changes state without an audit entry: add .audited(\"resource.action\") in AdminRoutes, or list it in writeRoutesNotAudited with the reason", key)
		case rt.Audit != "" && listed:
			t.Errorf("%s is audited as %q and also listed in writeRoutesNotAudited: remove one", key, rt.Audit)
		case listed && strings.TrimSpace(reason) == "":
			t.Errorf("%s is in writeRoutesNotAudited without a reason", key)
		}
		if rt.Audit != "" {
			if !auditActionRe.MatchString(rt.Audit) {
				t.Errorf("%s: audit action %q is not resource.action", key, rt.Audit)
			}
			if rt.Public != "" {
				t.Errorf("%s is public and audited: there is no user for the entry", key)
			}
		}
	}
	for key := range writeRoutesNotAudited {
		if !seen[key] {
			t.Errorf("writeRoutesNotAudited lists %s, which is not a route", key)
		}
	}
}

func TestAuditSucceeded(t *testing.T) {
	cases := []struct {
		status int
		loc    string
		want   bool
	}{
		{0, "", true}, // handler wrote nothing: 200
		{200, "", true},
		{303, "/cm/templates", true},
		{302, "/cm/snippets", true},
		{303, "/cm/login", false},
		{303, "/cm/content/abc?error=slug_exists&path=/x", false},
		{400, "", false},
		{403, "", false},
		{404, "", false},
		{500, "", false},
	}
	for _, c := range cases {
		if got := auditSucceeded(c.status, c.loc); got != c.want {
			t.Errorf("auditSucceeded(%d, %q) = %v, want %v", c.status, c.loc, got, c.want)
		}
	}
}

// waitAudit returns the audit entry matching the filter, waiting for the
// asynchronous write.
func waitAudit(t *testing.T, db *database.DB, filter bson.M) (models.AuditLog, bool) {
	t.Helper()
	var entry models.AuditLog
	deadline := time.Now().Add(10 * time.Second)
	for {
		if err := db.FindOne(context.Background(), "audit_logs", filter, &entry); err == nil {
			return entry, true
		}
		if time.Now().After(deadline) {
			return entry, false
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// Admin-UI mutations are audit-logged through the real router with the
// action names the API uses, the acting user and the resource id; a refused
// request, a validation error and a no-op write nothing.
func TestAdminUI_MutationsAreAuditLogged(t *testing.T) {
	h, cleanup := newTestHandler(t)
	defer cleanup()
	r := adminRouter(h)
	ctx := context.Background()
	const adminID = "000000000000000000000001" // the test session's user

	expect := func(what, action, resourceID string) models.AuditLog {
		t.Helper()
		f := bson.M{"action": action}
		if resourceID != "" && resourceID != "-" { // "-": the resource has no id (theme, config)
			f["resource_id"] = resourceID
		}
		e, ok := waitAudit(t, h.db, f)
		if !ok {
			t.Errorf("%s: no %s audit entry (resource %q)", what, action, resourceID)
			return e
		}
		if e.UserEmail != "admin@localhost" || e.UserID.Hex() != adminID || e.ViaAPI {
			t.Errorf("%s: entry user = %q / %s (via_api=%v), want the signed-in admin", what, e.UserEmail, e.UserID.Hex(), e.ViaAPI)
		}
		if want := strings.SplitN(action, ".", 2)[0]; e.Resource != want {
			t.Errorf("%s: resource = %q, want %q", what, e.Resource, want)
		}
		if e.ResourceID == "" && resourceID != "-" {
			t.Errorf("%s: %s entry has no resource id", what, action)
		}
		return e
	}
	idOf := func(coll string, filter bson.M) string {
		t.Helper()
		var doc struct {
			ID primitive.ObjectID `bson:"_id"`
		}
		if err := h.db.FindOne(ctx, coll, filter, &doc); err != nil {
			t.Fatalf("find %s %v: %v", coll, filter, err)
		}
		return doc.ID.Hex()
	}
	ok := func(what string, rr *httptest.ResponseRecorder) {
		t.Helper()
		if rr.Code >= 400 {
			t.Fatalf("%s: %d %s", what, rr.Code, rr.Body.String())
		}
	}

	// Templates
	ok("create template", routed(r, "admin", "POST", "/cm/templates/new", url.Values{"name": {"Audit tpl"}, "slug": {"audit-tpl"}, "html_layout": {"<p>{{.Body}}</p>"}}))
	tplID := idOf("templates", bson.M{"name": "Audit tpl"})
	expect("create template", "template.create", tplID)
	ok("update template", routed(r, "admin", "POST", "/cm/templates/"+tplID, url.Values{"name": {"Audit tpl 2"}, "slug": {"audit-tpl"}, "html_layout": {"<p>{{.Body}}</p>"}}))
	expect("update template", "template.update", tplID)

	// Content: create, update, delete, restore, regenerate, revert
	ok("create content", routed(r, "admin", "POST", "/cm/content/create", url.Values{"template_id": {tplID}, "title": {"Audit page"}, "slug": {"audit-page"}}))
	pageID := idOf("content", bson.M{"slug": "audit-page"})
	expect("create content", "content.create", pageID)
	ok("update content", routed(r, "admin", "POST", "/cm/content/"+pageID, url.Values{"title": {"Audit page 2"}, "slug": {"audit-page"}}))
	expect("update content", "content.update", pageID)
	ok("regenerate content", routed(r, "admin", "POST", "/cm/content/"+pageID+"/regenerate", nil))
	expect("regenerate content", "content.regenerate", pageID)
	ok("revert content", routed(r, "admin", "POST", "/cm/content/"+pageID+"/versions/1/revert", nil))
	if e := expect("revert content", "content.revert", pageID); e.Details["version"] != "1" {
		t.Errorf("content.revert details = %v, want the version", e.Details)
	}
	ok("delete content", routed(r, "admin", "POST", "/cm/content/"+pageID+"/delete", nil))
	expect("delete content", "content.delete", pageID)
	ok("restore content", routed(r, "admin", "POST", "/cm/content/"+pageID+"/undelete", nil))
	expect("restore content", "content.restore", pageID)

	// Settings: theme, site config, redirect, folder, collection, snippet
	ok("theme", routed(r, "admin", "POST", "/cm/theme", url.Values{"site_name": {"Audit site"}}))
	expect("theme", "theme.update", "-")
	ok("config", routed(r, "admin", "POST", "/cm/config", url.Values{"title_template": {"{{.Title}}"}}))
	expect("config", "config.update", "-")
	ok("redirect", routed(r, "admin", "POST", "/cm/redirects/new", url.Values{"from_path": {"/audit-from"}, "to_path": {"/audit-page"}, "status_code": {"301"}}))
	redirectID := idOf("redirects", bson.M{"from_path": "/audit-from"})
	expect("redirect create", "redirect.create", redirectID)
	ok("redirect delete", routed(r, "admin", "POST", "/cm/redirects/"+redirectID+"/delete", nil))
	expect("redirect delete", "redirect.delete", redirectID)
	ok("folder", routed(r, "admin", "POST", "/cm/folders/new", url.Values{"name": {"Audit folder"}, "slug": {"audit-folder"}}))
	expect("folder create", "folder.create", idOf("folders", bson.M{"slug": "audit-folder"}))
	ok("collection", routed(r, "admin", "POST", "/cm/collections/new", url.Values{"name": {"Audit coll"}, "category": {"auditcat"}}))
	expect("collection create", "collection.create", idOf("collections", bson.M{"name": "Audit coll"}))
	ok("snippet", routed(r, "admin", "POST", "/cm/snippets/new", url.Values{"name": {"audit-snippet"}, "html": {"<b>x</b>"}}))
	snippetID := idOf("snippets", bson.M{"name": "audit-snippet"})
	expect("snippet create", "snippet.create", snippetID)
	ok("snippet delete", routed(r, "admin", "POST", "/cm/snippets/"+snippetID+"/delete", nil))
	expect("snippet delete", "snippet.delete", snippetID)

	// API keys
	ok("api key", routed(r, "admin", "POST", "/cm/api-keys/new", url.Values{"name": {"audit-key"}}))
	keyID := idOf("api_keys", bson.M{"name": "audit-key"})
	expect("api key create", "apikey.create", keyID)
	ok("api key delete", routed(r, "admin", "POST", "/cm/api-keys/"+keyID+"/delete", nil))
	expect("api key delete", "apikey.delete", keyID)

	// Nothing is logged for a request that changed nothing
	baseline := count(t, h.db, "audit_logs", bson.M{})
	// refused: wrong role
	routed(r, "editor", "POST", "/cm/templates/new", url.Values{"name": {"Refused tpl"}, "slug": {"refused"}})
	// validation error: the form is re-rendered with a message (200)
	if rr := routed(r, "admin", "POST", "/cm/snippets/new", url.Values{"name": {""}}); rr.Code != http.StatusOK {
		t.Errorf("snippet without a name: %d, want the form again", rr.Code)
	}
	// failure reported by redirect: duplicate path
	seedPage(t, h.db, models.Content{Title: "Taken", Slug: "audit-taken", FullPath: "/audit-taken"})
	if rr := routed(r, "admin", "POST", "/cm/content/"+pageID, url.Values{"title": {"x"}, "slug": {"audit-taken"}, "slug_rename_enabled": {"yes"}}); !strings.Contains(rr.Header().Get("Location"), "error=slug_exists") {
		t.Errorf("duplicate slug: -> %q", rr.Header().Get("Location"))
	}
	// no-op: revoking a key that does not exist; reverting to a theme version that does not exist
	routed(r, "admin", "POST", "/cm/api-keys/"+primitive.NewObjectID().Hex()+"/delete", nil)
	routed(r, "admin", "POST", "/cm/theme/versions/99999/revert", nil)
	// error status
	routed(r, "admin", "POST", "/cm/content/"+primitive.NewObjectID().Hex()+"/delete", nil)
	time.Sleep(1500 * time.Millisecond)
	if n := count(t, h.db, "audit_logs", bson.M{}); n != baseline {
		var extra []models.AuditLog
		cur, _ := h.db.FindMany(ctx, "audit_logs", bson.M{})
		_ = cur.All(ctx, &extra)
		var actions []string
		for _, e := range extra {
			actions = append(actions, e.Action+":"+e.ResourceID)
		}
		t.Errorf("%d audit entries were written for requests that changed nothing (all entries: %v)", n-baseline, actions)
	}

	// Template delete last (the page above used it)
	ok("delete template", routed(r, "admin", "POST", "/cm/templates/"+tplID+"/delete", nil))
	expect("delete template", "template.delete", tplID)
}

// ---------------------------------------------------------------- item 9

func TestSameSitePath(t *testing.T) {
	req := httptest.NewRequest("GET", "http://cms.example/cm/forks/exit-preview", nil) // r.Host = cms.example
	const fb = "/cm/forks"
	cases := map[string]string{
		"":                                          fb,
		"/about":                                    "/about",
		"/blog/post?x=1":                            "/blog/post?x=1",
		"http://cms.example/blog/post?x=1#frag":     "/blog/post?x=1",
		"https://cms.example/":                      "/",
		"http://CMS.example/a":                      "/a",
		"https://evil.example/":                     fb,
		"https://evil.example/cm/forks":             fb,
		"//evil.example/":                           fb,
		"///evil.example/":                          fb,
		"/\\evil.example/":                          fb,
		"\\\\evil.example/":                         fb,
		"http://cms.example.evil.example/":          fb,
		"http://cms.example@evil.example/":          fb,
		"http://evil.example/?u=http://cms.example": fb,
		"javascript:alert(1)":                       fb,
		"data:text/html,x":                          fb,
		"ftp://cms.example/x":                       fb,
		"about":                                     fb,
		"http://cms.example//evil.example":          fb,
		"/a\r\nSet-Cookie: x=1":                     fb,
		"http://cms.example/cm/forks/exit-preview":  fb,
		"http://[::1":                               fb,
	}
	for raw, want := range cases {
		if got := sameSitePath(req, raw, fb); got != want {
			t.Errorf("sameSitePath(%q) = %q, want %q", raw, got, want)
		}
	}
}

// Leaving fork preview returns to the page the visitor was on only when it
// is a page of this site; a Referer from anywhere else is not followed.
func TestExitForkPreview_NoOpenRedirect(t *testing.T) {
	h := routesTestHandler()
	exit := func(referer string) *httptest.ResponseRecorder {
		req := httptest.NewRequest("GET", "http://cms.example/cm/forks/exit-preview", nil)
		if referer != "" {
			req.Header.Set("Referer", referer)
		}
		rr := httptest.NewRecorder()
		h.ExitForkPreview(rr, req)
		return rr
	}
	for referer, want := range map[string]string{
		"":                                 "/cm/forks",
		"https://evil.example/phish":       "/cm/forks",
		"//evil.example/phish":             "/cm/forks",
		"http://cms.example/blog/post?a=1": "/blog/post?a=1",
		"https://cms.example/":             "/",
	} {
		rr := exit(referer)
		if loc := rr.Header().Get("Location"); rr.Code != http.StatusSeeOther || loc != want {
			t.Errorf("Referer %q: %d -> %q, want 303 -> %q", referer, rr.Code, loc, want)
		}
		cleared := false
		for _, c := range rr.Result().Cookies() {
			if c.Name == forkPreviewCookie && c.MaxAge < 0 {
				cleared = true
			}
		}
		if !cleared {
			t.Errorf("Referer %q: the preview cookie was not cleared", referer)
		}
	}
}

// --------------------------------------------------------------- item 10

// Renaming a folder moves its pages' static files. A legacy row with an
// empty full_path must not take the homepage's file with it, and a fork
// copy in the folder must not take a live page's.
func TestUpdateContentFolderPaths_LeavesHomepageAndLiveFilesAlone(t *testing.T) {
	h, cleanup := newTestHandler(t)
	defer cleanup()
	ctx := context.Background()

	tmplID := seedTemplate(t, h.db, "Page", "page")
	forkID := createTestFork(t, h.db, "f10")
	const home = "<p>HOMEPAGE</p>"
	const live = "<p>LIVE ELSEWHERE</p>"
	writeStatic(t, "/index", home) // content/generated/index.html: the homepage
	writeStatic(t, "/f10-elsewhere", live)
	t.Cleanup(func() {
		os.RemoveAll("content/generated/f10-old")
		os.RemoveAll("content/generated/f10-new")
	})

	// A legacy row in the folder with no full_path
	legacy := primitive.NewObjectID()
	if _, err := h.db.Collection("content").InsertOne(ctx, bson.M{
		"_id": legacy, "template_id": tmplID, "title": "Legacy", "slug": "f10-legacy", "folder_path": "/f10-old", "full_path": "",
		"created_at": time.Now(), "updated_at": time.Now(),
	}); err != nil {
		t.Fatalf("seed legacy row: %v", err)
	}
	// A fork copy filed in the folder whose path is a live page's elsewhere
	copyID := seedPage(t, h.db, models.Content{TemplateID: tmplID, Title: "Copy", Slug: "f10-elsewhere", FolderPath: "/f10-old", FullPath: "/f10-elsewhere", ForkID: &forkID})
	// A normal published page in the folder
	writeStatic(t, "/f10-old/page", "<p>OLD</p>")
	page := seedPage(t, h.db, models.Content{TemplateID: tmplID, Title: "Page", Slug: "page", FolderPath: "/f10-old", FullPath: "/f10-old/page", Published: true})
	// Pages below the folder move with it; a sibling folder whose name
	// merely starts the same way does not.
	nested := seedPage(t, h.db, models.Content{TemplateID: tmplID, Title: "Nested", Slug: "deep", FolderPath: "/f10-old/sub", FullPath: "/f10-old/sub/deep"})
	sibling := seedPage(t, h.db, models.Content{TemplateID: tmplID, Title: "Sibling", Slug: "page", FolderPath: "/f10-older", FullPath: "/f10-older/page"})

	h.updateContentFolderPaths(ctx, "/f10-old", "/f10-new")

	if c := loadPage(t, h.db, nested); c.FullPath != "/f10-new/sub/deep" || c.FolderPath != "/f10-new/sub" {
		t.Errorf("nested page not moved: %q in %q", c.FullPath, c.FolderPath)
	}
	if c := loadPage(t, h.db, sibling); c.FullPath != "/f10-older/page" || c.FolderPath != "/f10-older" {
		t.Errorf("page in sibling folder /f10-older was moved: %q in %q", c.FullPath, c.FolderPath)
	}

	assertStatic(t, "folder rename with a legacy empty-path row", "/index", home)
	assertStatic(t, "folder rename with a fork copy in the folder", "/f10-elsewhere", live)
	if _, err := os.Stat(staticFile("/f10-old/page")); err == nil {
		t.Error("the moved page's old static file is still there")
	}
	if c := loadPage(t, h.db, page); c.FullPath != "/f10-new/page" || c.FolderPath != "/f10-new" {
		t.Errorf("page not moved: %q in %q", c.FullPath, c.FolderPath)
	}
	if c := loadPage(t, h.db, legacy); c.FullPath != "/f10-new/f10-legacy" {
		t.Errorf("legacy row path = %q, want /f10-new/f10-legacy", c.FullPath)
	}
	if c := loadPage(t, h.db, copyID); c.FolderPath != "/f10-new" {
		t.Errorf("fork copy folder = %q, want /f10-new", c.FolderPath)
	}
}

// --------------------------------------------------------------- item 12

// /cm/upload and the two /cm lock routes had no caller and are gone; the
// lock API they duplicated is still there.
func TestRemovedAdminRoutes(t *testing.T) {
	h := routesTestHandler()
	r := adminRouter(h)
	id := "000000000000000000000001"
	for _, target := range []string{"/cm/upload", "/cm/content/" + id + "/lock/refresh", "/cm/content/" + id + "/lock/force"} {
		rr := routed(r, "admin", "POST", target, nil)
		if rr.Code != http.StatusNotFound && rr.Code != http.StatusMethodNotAllowed {
			t.Errorf("POST %s: %d, want 404/405 (the route was removed)", target, rr.Code)
		}
	}
	src, err := os.ReadFile("api_routes.go")
	if err != nil {
		t.Fatalf("read api_routes.go: %v", err)
	}
	for _, route := range []string{`"/content/{id}/lock"`, `"/content/{id}/lock/force"`} {
		if !strings.Contains(string(src), route) {
			t.Errorf("api_routes.go no longer registers the lock API route %s", route)
		}
	}
}

// A page served in fork preview is one editor's view of unpublished work:
// the response must not be cacheable (a browser would keep showing the copy
// after Exit Preview; a CDN would serve it to everyone). The same page
// without the preview cookie keeps its public cache headers.
func TestForkPreview_ResponsesAreNotCacheable(t *testing.T) {
	h, cleanup := newTestHandler(t)
	defer cleanup()

	tmplID := seedTemplate(t, h.db, "Page", "page")
	forkName := "fpc-" + primitive.NewObjectID().Hex() // the preview token is derived from the name
	forkID := createTestFork(t, h.db, forkName)
	seedPage(t, h.db, models.Content{TemplateID: tmplID, Title: "FPC Live", Slug: "fpc-page", FullPath: "/fpc-page", Published: true})
	seedPage(t, h.db, models.Content{TemplateID: tmplID, Title: "FPC Fork Copy", Slug: "fpc-page", FullPath: "/fpc-page", ForkID: &forkID})

	get := func(preview bool) *httptest.ResponseRecorder {
		req := httptest.NewRequest("GET", "/fpc-page", nil)
		req = mux.SetURLVars(req, map[string]string{"slug": "fpc-page"})
		if preview {
			req.AddCookie(&http.Cookie{Name: forkPreviewCookie, Value: "tok-" + forkName})
		}
		rr := httptest.NewRecorder()
		h.ServePage(rr, req)
		return rr
	}

	rr := get(true)
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), "FPC Fork Copy") || !strings.Contains(rr.Body.String(), "lc-fork-preview-bar") {
		t.Fatalf("preview: status %d, fork copy or preview bar missing", rr.Code)
	}
	if cc := rr.Header().Get("Cache-Control"); cc != "private, no-store" {
		t.Errorf("preview: Cache-Control %q, want \"private, no-store\"", cc)
	}
	if etag := rr.Header().Get("ETag"); etag != "" {
		t.Errorf("preview: ETag %q set on an uncacheable response", etag)
	}

	rr = get(false)
	if rr.Code != http.StatusOK || strings.Contains(rr.Body.String(), "FPC Fork Copy") || strings.Contains(rr.Body.String(), "lc-fork-preview-bar") {
		t.Fatalf("live: status %d, or fork content served without the preview cookie", rr.Code)
	}
	if cc := rr.Header().Get("Cache-Control"); !strings.Contains(cc, "public") {
		t.Errorf("live: Cache-Control %q, want the public cache headers", cc)
	}
}

// Webhook changes made through the API are audit-logged under the action
// names the admin UI routes use.
func TestAPIWebhooks_AreAuditLogged(t *testing.T) {
	ah, db, cleanup := newTestAPIHandler(t)
	defer cleanup()

	rr := doJSON(t, ah.APICreateWebhook, http.MethodPost, map[string]interface{}{
		"name": "audited-hook", "url": "https://example.com/h", "events": []string{"content.create"}, "active": true,
	}, nil)
	if rr.Code != http.StatusCreated {
		t.Fatalf("APICreateWebhook: %d (%s)", rr.Code, rr.Body.String())
	}
	var created map[string]interface{}
	json.Unmarshal(rr.Body.Bytes(), &created)
	id, _ := created["id"].(string)
	vars := map[string]string{"id": id}

	doJSON(t, ah.APIUpdateWebhook, http.MethodPut, map[string]interface{}{"name": "audited-hook-2"}, vars)
	doJSON(t, ah.APIRegenerateWebhookSecret, http.MethodPost, nil, vars)
	doJSON(t, ah.APIDeleteWebhook, http.MethodDelete, nil, vars)

	for _, action := range []string{"webhook.create", "webhook.update", "webhook.regenerate_secret", "webhook.delete"} {
		if _, ok := waitAudit(t, db, bson.M{"action": action, "resource_id": id}); !ok {
			t.Errorf("no %s audit entry for webhook %s", action, id)
		}
	}
}
