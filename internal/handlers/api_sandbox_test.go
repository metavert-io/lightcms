package handlers

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sort"
	"strings"
	"testing"

	"github.com/metavert-io/lightcms/v7/internal/auth"
	"github.com/metavert-io/lightcms/v7/internal/database"
	"github.com/metavert-io/lightcms/v7/internal/middleware"
	"github.com/metavert-io/lightcms/v7/internal/models"
	"github.com/metavert-io/lightcms/v7/internal/services"

	"github.com/gorilla/mux"
	"github.com/gorilla/sessions"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// sandboxRefusedRoutes lists every state-changing /api/v1 route a sandboxed
// caller may not use, with the reason. Together with sandboxWriteRoutes it
// must cover every write route the router has, so a new route needs a
// decision before this test passes.
var sandboxRefusedRoutes = map[string]string{
	"DELETE /api/v1/api-keys/{id}":                        "manages credentials",
	"POST /api/v1/api-keys":                               "manages credentials",
	"DELETE /api/v1/approval-workflows/{id}":              "changes the approval process",
	"POST /api/v1/approval-workflows":                     "changes the approval process",
	"PUT /api/v1/approval-workflows/{id}":                 "changes the approval process",
	"POST /api/v1/approval-requests/{id}/approve":         "approving publishes live content",
	"POST /api/v1/approval-requests/{id}/reject":          "decides on someone else's change",
	"POST /api/v1/approval-requests/{id}/cancel":          "decides on someone else's change",
	"POST /api/v1/content/{id}/submit-approval":           "starts an approval of a live page",
	"DELETE /api/v1/assets/{id}":                          "removes a file the site serves",
	"DELETE /api/v1/collections/{id}":                     "site structure",
	"POST /api/v1/collections":                            "site structure",
	"PUT /api/v1/collections/{id}":                        "site structure",
	"DELETE /api/v1/folders/{id}":                         "site structure",
	"POST /api/v1/folders":                                "site structure",
	"DELETE /api/v1/redirects/{id}":                       "changes live URLs",
	"POST /api/v1/redirects":                              "changes live URLs",
	"PUT /api/v1/redirects/{id}":                          "changes live URLs",
	"DELETE /api/v1/snippets/{id}":                        "snippets render on live pages",
	"POST /api/v1/snippets":                               "snippets render on live pages",
	"PUT /api/v1/snippets/{id}":                           "snippets render on live pages",
	"DELETE /api/v1/templates/{id}":                       "templates render live pages",
	"POST /api/v1/templates":                              "templates render live pages",
	"PUT /api/v1/templates/{id}":                          "templates render live pages",
	"PUT /api/v1/theme":                                   "the theme renders every live page",
	"POST /api/v1/theme/versions/{version}/revert":        "the theme renders every live page",
	"POST /api/v1/theme/versions/{version}/pin":           "theme history",
	"POST /api/v1/theme/versions/{version}/unpin":         "theme history",
	"PUT /api/v1/config":                                  "site configuration",
	"PUT /api/v1/seo":                                     "site configuration (robots.txt, feeds)",
	"PUT /api/v1/indexnow":                                "site configuration",
	"POST /api/v1/indexnow/submit":                        "contacts search engines",
	"DELETE /api/v1/webhooks/{id}":                        "sends data off the site",
	"POST /api/v1/webhooks":                               "sends data off the site",
	"PUT /api/v1/webhooks/{id}":                           "sends data off the site",
	"POST /api/v1/webhooks/{id}/regenerate-secret":        "webhook credentials",
	"DELETE /api/v1/content/{id}":                         "deletes a live page (a fork copy is removed through its fork)",
	"POST /api/v1/content/{id}/restore":                   "brings back a live page",
	"POST /api/v1/content/{id}/publish":                   "publishes",
	"POST /api/v1/content/{id}/unpublish":                 "takes a live page down",
	"POST /api/v1/content/{id}/schedule":                  "publishes later",
	"POST /api/v1/content/{id}/versions/{version}/revert": "rewrites a page's history",
	"POST /api/v1/content/batch-publish":                  "publishes",
	"POST /api/v1/content/bulk-create":                    "writes live pages in bulk",
	"POST /api/v1/content/bulk-update":                    "writes live pages in bulk",
	"POST /api/v1/content/bulk-field-op":                  "writes live pages in bulk",
	"POST /api/v1/content/{id}/lock":                      "locks a live page against its editors",
	"DELETE /api/v1/content/{id}/lock":                    "locks a live page against its editors",
	"POST /api/v1/content/{id}/lock/force":                "breaks someone else's lock",
	"DELETE /api/v1/content/{id}/comments/{cid}":          "removes someone else's comment",
	"POST /api/v1/search-replace/execute":                 "writes live pages in bulk",
	"POST /api/v1/search-replace/scoped/execute":          "writes live pages in bulk",
	"POST /api/v1/search-replace/preview":                 "needs search_replace.execute, which a sandbox never has",
	"POST /api/v1/search-replace/scoped/preview":          "needs search_replace.execute, which a sandbox never has",
	"POST /api/v1/regenerate":                             "rewrites every live page",
	"POST /api/v1/reindex-embeddings":                     "rebuilds the search index",
	"POST /api/v1/link-check":                             "fetches every outbound link",
	"POST /api/v1/maintenance/scan":                       "site maintenance",
	"POST /api/v1/maintenance/backfill-published-dates":   "changes live pages' dates",
	"POST /api/v1/maintenance/repair-fork-damage":         "rewrites live pages",
	"POST /api/v1/agent-sessions/{id}/rollback":           "rewrites live pages",
	"POST /api/v1/imports/csv":                            "creates pages outside any fork",
	"POST /api/v1/imports/markdown":                       "creates pages outside any fork",
	"POST /api/v1/imports/sources":                        "creates pages outside any fork, on a schedule",
	"PUT /api/v1/imports/sources/{id}":                    "creates pages outside any fork, on a schedule",
	"DELETE /api/v1/imports/sources/{id}":                 "import configuration",
	"POST /api/v1/imports/sources/{id}/trigger":           "creates pages outside any fork",
	"POST /api/v1/imports/jobs/{id}/cancel":               "stops someone else's import",
	"POST /api/v1/forks/{id}/merge":                       "merging writes the live site",
	"POST /api/v1/forks/{id}/archive":                     "ends a fork without review",
	"POST /api/v1/forks/{id}/purge-copies":                "fork housekeeping",
}

// apiWriteRoutes walks the real /api/v1 router and returns every route that
// is not a read, as "METHOD /api/v1/template".
func apiWriteRoutes(t *testing.T) []string {
	t.Helper()
	r := mux.NewRouter()
	(&APIHandler{}).MountAPI(r, middleware.NewAPIAuth(nil))
	var routes []string
	err := r.Walk(func(route *mux.Route, _ *mux.Router, _ []*mux.Route) error {
		tpl, err := route.GetPathTemplate()
		if err != nil {
			return nil
		}
		methods, err := route.GetMethods()
		if err != nil {
			if tpl != "/api/v1" {
				t.Errorf("route %s is registered for every method", tpl)
			}
			return nil
		}
		for _, m := range methods {
			if m != http.MethodGet {
				routes = append(routes, m+" "+tpl)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk: %v", err)
	}
	sort.Strings(routes)
	return routes
}

func TestSandboxRoutes_EveryWriteRouteIsDecided(t *testing.T) {
	routes := apiWriteRoutes(t)
	if len(routes) < 80 {
		t.Fatalf("only %d write routes found; the walk looks wrong", len(routes))
	}
	seen := map[string]bool{}
	for _, key := range routes {
		seen[key] = true
		_, allowed := sandboxWriteRoutes[key]
		_, refused := sandboxRefusedRoutes[key]
		switch {
		case allowed && refused:
			t.Errorf("%s is both allowed and refused for a sandbox", key)
		case !allowed && !refused:
			t.Errorf("%s: decide whether a sandboxed connection may call it (sandboxWriteRoutes, with why it can't reach the live site) or not (sandboxRefusedRoutes)", key)
		}
	}
	for key := range sandboxWriteRoutes {
		if !seen[key] {
			t.Errorf("sandboxWriteRoutes lists %s, which is not a route", key)
		}
	}
	for key := range sandboxRefusedRoutes {
		if !seen[key] {
			t.Errorf("sandboxRefusedRoutes lists %s, which is not a route", key)
		}
	}
}

// Every write route outside the table is refused to a sandboxed caller
// before its handler runs: nothing reaches the database.
func TestSandboxGate_RefusesEveryRouteOutsideTheTable(t *testing.T) {
	a, db, cleanup := newTestAPIHandler(t)
	defer cleanup()
	tmpl := seedTemplate(t, db, "Page", "page")
	live := seedContent(t, db, tmpl, "Live", "live", "/live")

	// Each route gets its own pair of keys, so the per-key rate limits
	// don't get in the way of sending every route at once.
	routes := apiWriteRoutes(t)
	callers := map[string]*auth.SessionUser{}
	for i := range routes {
		sandboxed := keyCaller(fmt.Sprintf("agent-%d", i), models.RoleAdmin)
		sandboxed.SandboxOnly = true
		inSandbox := keyCaller(fmt.Sprintf("helper-%d", i), models.RoleAdmin) // Full access, with a sandbox open
		inSandbox.ActiveSandboxFork = primitive.NewObjectID().Hex()
		callers[fmt.Sprintf("lc_sandboxed_%d", i)] = sandboxed
		callers[fmt.Sprintf("lc_insandbox_%d", i)] = inSandbox
	}
	router := apiRouterWithKeys(a, callers)

	for i, key := range routes {
		if _, allowed := sandboxWriteRoutes[key]; allowed {
			continue
		}
		method, tpl, _ := strings.Cut(key, " ")
		target := strings.NewReplacer("{id}", live.Hex(), "{version}", "1", "{pageID}", live.Hex(), "{cid}", live.Hex()).Replace(tpl)
		for _, credential := range []string{fmt.Sprintf("lc_sandboxed_%d", i), fmt.Sprintf("lc_insandbox_%d", i)} {
			rr := sendJSON(router, credential, method, target, `{}`)
			if rr.Code != http.StatusForbidden || !strings.Contains(rr.Body.String(), "limited to its own forks") {
				t.Errorf("%s as %s: %d %s", key, credential, rr.Code, rr.Body.String())
			}
		}
	}

	var page models.Content
	if err := db.FindOne(context.Background(), "content", bson.M{"_id": live}, &page); err != nil || page.Deleted || page.Title != "Live" {
		t.Errorf("the live page changed: %+v, %v", page, err)
	}
}

// --- sandbox state and fork ownership, with real keys -----------------------

type sandboxEnv struct {
	a      *APIHandler
	db     *database.DB
	router http.Handler
	keys   *services.APIKeyService
	tmpl   string
	live   primitive.ObjectID
	owner  *models.User
}

func newSandboxEnv(t *testing.T) *sandboxEnv {
	t.Helper()
	a, db, cleanup := newTestAPIHandler(t)
	t.Cleanup(cleanup)
	users := services.NewUserService(db)
	owner, err := users.CreateUserWithHash(context.Background(), "editor@example.com", "Editor", models.RoleEditor, "unused", false)
	if err != nil {
		t.Fatalf("CreateUserWithHash: %v", err)
	}
	keys := services.NewAPIKeyService(db)
	resolver := auth.NewCredentialResolver(users, keys, services.NewOAuthService(db), "https://example.com/mcp", nil)
	r := mux.NewRouter()
	a.MountAPI(r, middleware.NewAPIAuth(resolver.ResolveAPIKey))
	tmpl := seedTemplate(t, db, "Page", "page")
	return &sandboxEnv{
		a: a, db: db, router: r, keys: keys, owner: owner,
		tmpl: tmpl.Hex(),
		live: seedContent(t, db, tmpl, "Live", "live", "/live"),
	}
}

// key creates an API key for the editor with the given access.
func (e *sandboxEnv) key(t *testing.T, name string, sandboxOnly bool) string {
	t.Helper()
	raw, _, err := e.keys.CreateScopedAPIKey(context.Background(), name, "", &e.owner.ID, nil, sandboxOnly)
	if err != nil {
		t.Fatalf("CreateScopedAPIKey: %v", err)
	}
	return raw
}

func (e *sandboxEnv) send(key, method, target, body string) *httptest.ResponseRecorder {
	return sendJSON(e.router, key, method, target, body)
}

func decode(t *testing.T, rr *httptest.ResponseRecorder) map[string]interface{} {
	t.Helper()
	var out map[string]interface{}
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatalf("decoding %q: %v", rr.Body.String(), err)
	}
	return out
}

func (e *sandboxEnv) startSandbox(t *testing.T, key string) string {
	t.Helper()
	rr := e.send(key, "POST", "/api/v1/sandbox", `{"name":"work"}`)
	if rr.Code != http.StatusCreated {
		t.Fatalf("start sandbox: %d %s", rr.Code, rr.Body.String())
	}
	return decode(t, rr)["fork_id"].(string)
}

func TestSandbox_ConnectionsWriteOnlyToTheirOwnForks(t *testing.T) {
	e := newSandboxEnv(t)
	mine := e.key(t, "Mine", true)
	other := e.key(t, "Other agent", true)
	full := e.key(t, "Full", false)

	fork := e.startSandbox(t, mine)
	create := func(key, forkID, slug string) *httptest.ResponseRecorder {
		body := fmt.Sprintf(`{"template_id":%q,"title":"T","slug":%q,"data":{}`, e.tmpl, slug)
		if forkID != "" {
			body += fmt.Sprintf(`,"fork_id":%q`, forkID)
		}
		return e.send(key, "POST", "/api/v1/content", body+"}")
	}

	if rr := create(mine, "", "outside"); rr.Code != http.StatusForbidden {
		t.Errorf("sandboxed create outside a fork: %d %s", rr.Code, rr.Body.String())
	}
	rr := create(mine, fork, "inside")
	if rr.Code != http.StatusCreated {
		t.Fatalf("create in own fork: %d %s", rr.Code, rr.Body.String())
	}
	copyID := decode(t, rr)["id"].(string)

	if rr := create(other, fork, "intruder"); rr.Code != http.StatusForbidden {
		t.Errorf("another sandboxed key creating in the fork: %d %s", rr.Code, rr.Body.String())
	}
	if rr := e.send(other, "PUT", "/api/v1/content/"+copyID, `{"title":"Hijacked"}`); rr.Code != http.StatusForbidden {
		t.Errorf("another sandboxed key editing the copy: %d %s", rr.Code, rr.Body.String())
	}
	if rr := e.send(other, "POST", "/api/v1/forks/"+fork+"/fork-page", `{"path":"/live"}`); rr.Code != http.StatusForbidden {
		t.Errorf("another sandboxed key adding a page: %d %s", rr.Code, rr.Body.String())
	}
	if rr := e.send(other, "DELETE", "/api/v1/forks/"+fork, ``); rr.Code != http.StatusForbidden {
		t.Errorf("another sandboxed key discarding the fork: %d %s", rr.Code, rr.Body.String())
	}

	if rr := e.send(mine, "PUT", "/api/v1/content/"+e.live.Hex(), `{"title":"Live change"}`); rr.Code != http.StatusForbidden {
		t.Errorf("sandboxed edit of a live page: %d %s", rr.Code, rr.Body.String())
	}
	if rr := e.send(mine, "PUT", "/api/v1/content/by-path?path=/live", `{"title":"Live change"}`); rr.Code != http.StatusForbidden {
		t.Errorf("sandboxed edit of a live page by path: %d %s", rr.Code, rr.Body.String())
	}
	if rr := e.send(mine, "PUT", "/api/v1/content/"+copyID, `{"title":"Edited in sandbox"}`); rr.Code != http.StatusOK {
		t.Errorf("own copy: %d %s", rr.Code, rr.Body.String())
	}
	if rr := e.send(mine, "POST", "/api/v1/forks/"+fork+"/fork-page", `{"path":"/live"}`); rr.Code != http.StatusOK {
		t.Errorf("adding a live page to own fork: %d %s", rr.Code, rr.Body.String())
	}

	// A Full key is not held to forks.
	if rr := e.send(full, "PUT", "/api/v1/content/"+copyID, `{"title":"Edited by a person's tool"}`); rr.Code != http.StatusOK {
		t.Errorf("full key on the copy: %d %s", rr.Code, rr.Body.String())
	}

	var live models.Content
	e.db.FindOne(context.Background(), "content", bson.M{"_id": e.live}, &live)
	if live.Title != "Live" {
		t.Errorf("live page title is %q", live.Title)
	}

	// Ending the sandbox doesn't lift a Sandbox key's restriction.
	if rr := e.send(mine, "POST", "/api/v1/sandbox/end", `{"action":"submit"}`); rr.Code != http.StatusOK {
		t.Fatalf("submit: %d %s", rr.Code, rr.Body.String())
	}
	state := decode(t, e.send(mine, "GET", "/api/v1/sandbox", ``))
	if state["active"] != false || state["restricted"] != true {
		t.Errorf("after submit: %+v", state)
	}
	if rr := create(mine, "", "after-submit"); rr.Code != http.StatusForbidden {
		t.Errorf("sandbox key after submit: %d %s", rr.Code, rr.Body.String())
	}
	if rr := e.send(mine, "PUT", "/api/v1/content/"+e.live.Hex(), `{"title":"Live change"}`); rr.Code != http.StatusForbidden {
		t.Errorf("sandbox key editing live after submit: %d %s", rr.Code, rr.Body.String())
	}

	// It can still discard the fork it made; nobody else's.
	if rr := e.send(mine, "DELETE", "/api/v1/forks/"+fork, ``); rr.Code != http.StatusOK {
		t.Errorf("discarding own fork: %d %s", rr.Code, rr.Body.String())
	}
}

func TestSandbox_FullKeyIsHeldToItsSandboxUntilItEnds(t *testing.T) {
	e := newSandboxEnv(t)
	full := e.key(t, "Full", false)

	fork := e.startSandbox(t, full)
	if rr := e.send(full, "POST", "/api/v1/sandbox", `{}`); rr.Code != http.StatusConflict {
		t.Errorf("second sandbox: %d %s", rr.Code, rr.Body.String())
	}
	if rr := e.send(full, "PUT", "/api/v1/content/"+e.live.Hex(), `{"title":"Live change"}`); rr.Code != http.StatusForbidden {
		t.Errorf("live edit with a sandbox open: %d %s", rr.Code, rr.Body.String())
	}
	if rr := e.send(full, "POST", "/api/v1/content/"+e.live.Hex()+"/publish", ``); rr.Code != http.StatusForbidden {
		t.Errorf("publish with a sandbox open: %d %s", rr.Code, rr.Body.String())
	}

	rr := e.send(full, "POST", "/api/v1/sandbox/end", `{"action":"discard"}`)
	if rr.Code != http.StatusOK {
		t.Fatalf("discard: %d %s", rr.Code, rr.Body.String())
	}
	if _, err := e.a.forkService.GetByID(context.Background(), mustOID(t, fork)); err == nil {
		t.Error("the discarded fork still exists")
	}
	if rr := e.send(full, "PUT", "/api/v1/content/"+e.live.Hex(), `{"title":"Live change"}`); rr.Code != http.StatusOK {
		t.Errorf("live edit after the sandbox ended: %d %s", rr.Code, rr.Body.String())
	}
}

func TestSandbox_EndsWhenItsForkIsMergedArchivedOrDeleted(t *testing.T) {
	e := newSandboxEnv(t)
	ctx := context.Background()
	full := e.key(t, "Full", false)

	fork := e.startSandbox(t, full)
	if err := e.a.forkService.Delete(ctx, mustOID(t, fork)); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if rr := e.send(full, "PUT", "/api/v1/content/"+e.live.Hex(), `{"title":"After delete"}`); rr.Code != http.StatusOK {
		t.Errorf("a person deleted the fork, but the key is still confined: %d %s", rr.Code, rr.Body.String())
	}

	fork = e.startSandbox(t, full)
	if err := e.a.forkService.Archive(ctx, mustOID(t, fork)); err != nil {
		t.Fatalf("Archive: %v", err)
	}
	if state := decode(t, e.send(full, "GET", "/api/v1/sandbox", ``)); state["active"] != false {
		t.Errorf("after archive: %+v", state)
	}

	// A sandbox left pointing at a fork that is no longer active is closed
	// the next time it is looked at.
	fork = e.startSandbox(t, full)
	if err := e.db.UpdateOne(ctx, "content_forks", bson.M{"_id": mustOID(t, fork)}, bson.M{"$set": bson.M{"status": "merged"}}); err != nil {
		t.Fatalf("marking merged: %v", err)
	}
	if state := decode(t, e.send(full, "GET", "/api/v1/sandbox", ``)); state["active"] != false {
		t.Errorf("stale sandbox: %+v", state)
	}
	if rr := e.send(full, "PUT", "/api/v1/content/"+e.live.Hex(), `{"title":"After stale"}`); rr.Code != http.StatusOK {
		t.Errorf("live edit after a stale sandbox was closed: %d %s", rr.Code, rr.Body.String())
	}
}

func TestSandbox_BrowserSessionsHaveNoSandbox(t *testing.T) {
	a, _, cleanup := newTestAPIHandler(t)
	defer cleanup()
	req := httptest.NewRequest("POST", "/api/v1/sandbox", strings.NewReader(`{}`))
	req = req.WithContext(middleware.InjectAPIUser(req.Context(), &auth.SessionUser{ID: primitive.NewObjectID().Hex(), Email: "a@example.com", Role: models.RoleAdmin}))
	rr := httptest.NewRecorder()
	a.APIStartSandbox(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("got %d %s", rr.Code, rr.Body.String())
	}
}

func mustOID(t *testing.T, hex string) primitive.ObjectID {
	t.Helper()
	id, err := primitive.ObjectIDFromHex(hex)
	if err != nil {
		t.Fatalf("bad id %q", hex)
	}
	return id
}

func TestSandbox_UploadsAreNewAndInert(t *testing.T) {
	e := newSandboxEnv(t)
	t.Chdir(t.TempDir()) // uploads are written under content/generated
	sandboxed := e.key(t, "Agent", true)
	full := e.key(t, "Full", false)

	png := base64.StdEncoding.EncodeToString([]byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR\x00\x00\x00\x01\x00\x00\x00\x01\x08\x06\x00\x00\x00\x1f\x15\xc4\x89"))
	svg := base64.StdEncoding.EncodeToString([]byte(`<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>`))
	html := base64.StdEncoding.EncodeToString([]byte(`<html><script>alert(1)</script></html>`))
	upload := func(key, filename, servePath, data string) int {
		return e.send(key, "POST", "/api/v1/assets", fmt.Sprintf(`{"filename":%q,"serve_path":%q,"data_base64":%q}`, filename, servePath, data)).Code
	}

	if code := upload(sandboxed, "a.png", "/assets/a.png", png); code != http.StatusCreated {
		t.Errorf("new png: %d", code)
	}
	if code := upload(sandboxed, "a.png", "/assets/a.png", png); code != http.StatusConflict {
		t.Errorf("replacing a file: %d", code)
	}
	if code := upload(sandboxed, "x.svg", "/assets/x.svg", svg); code != http.StatusForbidden {
		t.Errorf("svg: %d", code)
	}
	if code := upload(sandboxed, "x.html", "/assets/x.html", html); code != http.StatusForbidden {
		t.Errorf("html: %d", code)
	}
	if code := upload(sandboxed, "x.png", "/assets/x.html", png); code != http.StatusForbidden {
		t.Errorf("png named into an html path: %d", code)
	}
	if rr := e.send(sandboxed, "POST", "/api/v1/assets/from-url", `{"url":"https://example.invalid/x.js"}`); rr.Code != http.StatusForbidden {
		t.Errorf("script by URL: %d %s", rr.Code, rr.Body.String())
	}

	if code := upload(full, "x.svg", "/assets/x.svg", svg); code != http.StatusCreated {
		t.Errorf("full key svg: %d", code)
	}
}

// --- admin pages ---------------------------------------------------------

// userReq is a signed-in admin-page request as the given account.
func userReq(user *models.User, method, target string, form url.Values) *http.Request {
	store := sessions.NewCookieStore([]byte(testSessionSecret))
	req := httptest.NewRequest(method, target, strings.NewReader(form.Encode()))
	if method != http.MethodGet {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	rec := httptest.NewRecorder()
	sess, _ := store.New(req, "lightcms-session")
	sess.Values["authenticated"] = true
	sess.Values["user_id"] = user.ID.Hex()
	sess.Values["user_email"] = user.Email
	sess.Values["user_role"] = user.Role
	_ = sess.Save(req, rec)
	for _, c := range rec.Result().Cookies() {
		req.AddCookie(c)
	}
	return req
}

func serveAs(r http.Handler, user *models.User, method, target string, form url.Values) *httptest.ResponseRecorder {
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, userReq(user, method, target, form))
	return rr
}

type adminEnv struct {
	h      *Handler
	router http.Handler
	db     *database.DB
	users  *services.UserService
	oauth  *services.OAuthService
	keys   *services.APIKeyService
}

func newAdminEnv(t *testing.T) *adminEnv {
	t.Helper()
	h, cleanup := newTestHandler(t)
	t.Cleanup(cleanup)
	return &adminEnv{h: h, router: adminRouter(h), db: h.db, users: services.NewUserService(h.db), oauth: services.NewOAuthService(h.db), keys: services.NewAPIKeyService(h.db)}
}

func (e *adminEnv) user(t *testing.T, email, role string) *models.User {
	t.Helper()
	u, err := e.users.CreateUserWithHash(context.Background(), email, email, role, "unused", false)
	if err != nil {
		t.Fatalf("CreateUserWithHash: %v", err)
	}
	return u
}

func (e *adminEnv) connection(t *testing.T, owner *models.User, name string) *models.OAuthGrant {
	t.Helper()
	ctx := context.Background()
	client, _, err := e.oauth.RegisterClient(ctx, name, []string{"https://app.example/cb"})
	if err != nil {
		t.Fatalf("RegisterClient: %v", err)
	}
	grant, err := e.oauth.SaveGrant(ctx, client, owner.ID, "https://example.com/mcp", services.AccessSettings{SandboxOnly: true})
	if err != nil {
		t.Fatalf("SaveGrant: %v", err)
	}
	return grant
}

func TestConnectedApps_PeopleSeeAndChangeOnlyTheirOwn(t *testing.T) {
	e := newAdminEnv(t)
	ctx := context.Background()
	admin := e.user(t, "admin@example.com", models.RoleAdmin)
	editor := e.user(t, "editor@example.com", models.RoleEditor)
	viewer := e.user(t, "viewer@example.com", models.RoleViewer)
	editorsApp := e.connection(t, editor, "Editor's Assistant")
	viewersApp := e.connection(t, viewer, "Viewer's Reader")

	page := serveAs(e.router, viewer, "GET", "/cm/connected-apps", nil).Body.String()
	if !strings.Contains(page, "Viewer&#39;s Reader") || strings.Contains(page, "Editor&#39;s Assistant") {
		t.Errorf("a viewer should see only their own connection:\n%s", page)
	}
	if page := serveAs(e.router, admin, "GET", "/cm/connected-apps", nil).Body.String(); !strings.Contains(page, "Editor&#39;s Assistant") || !strings.Contains(page, "viewer@example.com") {
		t.Error("an admin should see every connection and who made it")
	}

	// Someone else's connection is refused.
	rr := serveAs(e.router, viewer, "POST", "/cm/connected-apps/"+editorsApp.ID.Hex()+"/access", url.Values{"access": {"full"}})
	if rr.Code != http.StatusForbidden {
		t.Errorf("changing another person's connection: %d", rr.Code)
	}
	rr = serveAs(e.router, viewer, "POST", "/cm/connected-apps/"+editorsApp.ID.Hex()+"/revoke", nil)
	if rr.Code != http.StatusForbidden {
		t.Errorf("revoking another person's connection: %d", rr.Code)
	}

	// Their own works.
	if rr := serveAs(e.router, viewer, "POST", "/cm/connected-apps/"+viewersApp.ID.Hex()+"/access", url.Values{"access": {"full"}}); rr.Code != http.StatusSeeOther {
		t.Errorf("changing own connection: %d", rr.Code)
	}
	if grant, _ := e.oauth.GetGrant(ctx, viewersApp.ID); grant.SandboxOnly {
		t.Error("access was not changed")
	}
	if rr := serveAs(e.router, viewer, "POST", "/cm/connected-apps/"+viewersApp.ID.Hex()+"/revoke", nil); rr.Code != http.StatusSeeOther {
		t.Errorf("revoking own connection: %d", rr.Code)
	}
	if grant, _ := e.oauth.GetGrant(ctx, viewersApp.ID); grant.RevokedAt == nil {
		t.Error("the connection was not revoked")
	}
}

func TestConnectedApps_OnlyAnAdminCanAllowSelfMerge(t *testing.T) {
	e := newAdminEnv(t)
	ctx := context.Background()
	admin := e.user(t, "admin@example.com", models.RoleAdmin)
	editor := e.user(t, "editor@example.com", models.RoleEditor)
	adminsApp := e.connection(t, admin, "Admin's Assistant")
	editorsApp := e.connection(t, editor, "Editor's Assistant")
	merge := url.Values{"access": {"sandbox"}, "merge_shown": {"1"}, "can_merge_own_forks": {"1"}}

	// An editor can't merge, so can't allow an app of theirs to.
	serveAs(e.router, editor, "POST", "/cm/connected-apps/"+editorsApp.ID.Hex()+"/access", merge)
	if grant, _ := e.oauth.GetGrant(ctx, editorsApp.ID); grant.CanMergeOwnForks {
		t.Error("an editor let their app merge")
	}
	// Nor can an admin, for an account that can't merge.
	serveAs(e.router, admin, "POST", "/cm/connected-apps/"+editorsApp.ID.Hex()+"/access", merge)
	if grant, _ := e.oauth.GetGrant(ctx, editorsApp.ID); grant.CanMergeOwnForks {
		t.Error("an admin let an editor's app merge")
	}

	if rr := serveAs(e.router, admin, "POST", "/cm/connected-apps/"+adminsApp.ID.Hex()+"/access", merge); rr.Code != http.StatusSeeOther {
		t.Fatalf("admin allowing merge: %d", rr.Code)
	}
	grant, _ := e.oauth.GetGrant(ctx, adminsApp.ID)
	if !grant.CanMergeOwnForks || !grant.SandboxOnly {
		t.Errorf("after allowing merge: %+v", grant)
	}

	// A form without the checkbox leaves the setting alone.
	serveAs(e.router, admin, "POST", "/cm/connected-apps/"+adminsApp.ID.Hex()+"/access", url.Values{"access": {"full"}})
	if grant, _ := e.oauth.GetGrant(ctx, adminsApp.ID); !grant.CanMergeOwnForks || grant.SandboxOnly {
		t.Errorf("after an access-only change: %+v", grant)
	}
	// Unticking it turns merging off.
	serveAs(e.router, admin, "POST", "/cm/connected-apps/"+adminsApp.ID.Hex()+"/access", url.Values{"access": {"full"}, "merge_shown": {"1"}})
	if grant, _ := e.oauth.GetGrant(ctx, adminsApp.ID); grant.CanMergeOwnForks {
		t.Error("merging is still allowed")
	}
}

func TestAPIKeys_NewKeysAreSandboxAndAccessCanBeChanged(t *testing.T) {
	e := newAdminEnv(t)
	ctx := context.Background()
	admin := e.user(t, "admin@example.com", models.RoleAdmin)
	editor := e.user(t, "editor@example.com", models.RoleEditor)

	if rr := serveAs(e.router, editor, "POST", "/cm/api-keys/new", url.Values{"name": {"Agent"}, "can_merge_own_forks": {"1"}}); rr.Code != http.StatusOK {
		t.Fatalf("create: %d", rr.Code)
	}
	keys, _ := e.keys.ListAPIKeysForUser(ctx, editor.ID)
	if len(keys) != 1 || !keys[0].SandboxOnly || keys[0].CanMergeOwnForks {
		t.Fatalf("new key: %+v", keys)
	}

	// The owner can switch it to Full; another non-admin can't touch it.
	other := e.user(t, "other@example.com", models.RoleEditor)
	if rr := serveAs(e.router, other, "POST", "/cm/api-keys/"+keys[0].ID.Hex()+"/access", url.Values{"access": {"full"}}); rr.Code != http.StatusForbidden {
		t.Errorf("another editor: %d", rr.Code)
	}
	serveAs(e.router, editor, "POST", "/cm/api-keys/"+keys[0].ID.Hex()+"/access", url.Values{"access": {"full"}})
	if key, _ := e.keys.GetAPIKey(ctx, keys[0].ID); key.SandboxOnly {
		t.Error("the owner could not switch the key to Full")
	}

	// An admin's own key can be allowed to merge.
	if rr := serveAs(e.router, admin, "POST", "/cm/api-keys/new", url.Values{"name": {"Publisher"}, "access": {"full"}, "can_merge_own_forks": {"1"}}); rr.Code != http.StatusOK {
		t.Fatalf("admin create: %d", rr.Code)
	}
	adminKeys, _ := e.keys.ListAPIKeysForUser(ctx, admin.ID)
	if len(adminKeys) != 1 || adminKeys[0].SandboxOnly || !adminKeys[0].CanMergeOwnForks {
		t.Errorf("admin key: %+v", adminKeys)
	}

	page := serveAs(e.router, admin, "GET", "/cm/api-keys", nil).Body.String()
	for _, want := range []string{"editor@example.com", "Can merge its own forks", `value="sandbox"`} {
		if !strings.Contains(page, want) {
			t.Errorf("API keys page lacks %q", want)
		}
	}
}
