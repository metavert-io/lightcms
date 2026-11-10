package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/metavert-io/lightcms/v7/internal/auth"
	"github.com/metavert-io/lightcms/v7/internal/database"
	lightmcp "github.com/metavert-io/lightcms/v7/internal/mcp"
	"github.com/metavert-io/lightcms/v7/internal/middleware"
	"github.com/metavert-io/lightcms/v7/internal/models"
	"github.com/metavert-io/lightcms/v7/internal/services"

	"github.com/gorilla/mux"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// apiRouterWithKeys mounts the real /api/v1 routes and middleware, with API
// keys that resolve to the given callers.
func apiRouterWithKeys(a *APIHandler, callers map[string]*auth.SessionUser) http.Handler {
	apiAuth := middleware.NewAPIAuth(func(ctx context.Context, key string) (interface{}, error) {
		if u, ok := callers[key]; ok {
			return u, nil
		}
		return nil, fmt.Errorf("unknown key")
	})
	r := mux.NewRouter()
	a.MountAPI(r, apiAuth)
	return r
}

func sendJSON(router http.Handler, key, method, target, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	return rr
}

func keyCaller(name, role string, scopes ...string) *auth.SessionUser {
	return &auth.SessionUser{
		ID:         primitive.NewObjectID().Hex(),
		Email:      name + "@example.com",
		Role:       role,
		ViaAPIKey:  true,
		Credential: "apikey:" + name,
		Scopes:     scopes,
	}
}

// publishCallers: a contributor, a key scoped to writing but not publishing,
// and an editor.
func publishCallers() map[string]*auth.SessionUser {
	return map[string]*auth.SessionUser{
		"lc_contributor": keyCaller("contributor", models.RoleContributor),
		"lc_writer":      keyCaller("writer", models.RoleEditor, auth.PermContentView, auth.PermContentCreate, auth.PermContentEdit),
		"lc_editor":      keyCaller("editor", models.RoleEditor),
	}
}

func pagePublished(t *testing.T, db *database.DB, slug string) (found, published bool) {
	t.Helper()
	var c models.Content
	if err := db.FindOne(context.Background(), "content", bson.M{"slug": slug}, &c); err != nil {
		return false, false
	}
	return true, c.Published
}

func TestPublishPermission_CreateRequiresIt(t *testing.T) {
	a, db, cleanup := newTestAPIHandler(t)
	defer cleanup()
	router := apiRouterWithKeys(a, publishCallers())
	tmpl := seedTemplate(t, db, "Page", "page").Hex()

	body := func(slug string, published bool) string {
		return fmt.Sprintf(`{"template_id":%q,"title":"T","slug":%q,"data":{},"published":%t}`, tmpl, slug, published)
	}

	for _, key := range []string{"lc_contributor", "lc_writer"} {
		if rr := sendJSON(router, key, "POST", "/api/v1/content", body("live-"+key, true)); rr.Code != http.StatusForbidden {
			t.Errorf("%s publishing on create: %d %s", key, rr.Code, rr.Body.String())
		}
		if found, _ := pagePublished(t, db, "live-"+key); found {
			t.Errorf("%s: the refused page was created", key)
		}
		if rr := sendJSON(router, key, "POST", "/api/v1/content", body("draft-"+key, false)); rr.Code != http.StatusCreated {
			t.Errorf("%s creating a draft: %d %s", key, rr.Code, rr.Body.String())
		}
	}

	if rr := sendJSON(router, "lc_editor", "POST", "/api/v1/content", body("by-editor", true)); rr.Code != http.StatusCreated {
		t.Fatalf("editor: %d %s", rr.Code, rr.Body.String())
	}
	if _, published := pagePublished(t, db, "by-editor"); !published {
		t.Error("the editor's page should be published")
	}
}

func TestPublishPermission_BulkCreateRequiresIt(t *testing.T) {
	a, db, cleanup := newTestAPIHandler(t)
	defer cleanup()
	router := apiRouterWithKeys(a, publishCallers())
	tmpl := seedTemplate(t, db, "Page", "page").Hex()

	body := fmt.Sprintf(`{"items":[
		{"template_id":%q,"title":"Draft","slug":"bulk-draft","data":{}},
		{"template_id":%q,"title":"Live","slug":"bulk-live","data":{},"published":true}
	]}`, tmpl, tmpl)
	if rr := sendJSON(router, "lc_contributor", "POST", "/api/v1/content/bulk-create", body); rr.Code != http.StatusForbidden {
		t.Errorf("bulk create with a published item: %d %s", rr.Code, rr.Body.String())
	}
	if found, _ := pagePublished(t, db, "bulk-live"); found {
		t.Error("nothing in a refused batch should be created")
	}
}

func TestPublishPermission_UpdateRequiresItOnlyToChangeState(t *testing.T) {
	a, db, cleanup := newTestAPIHandler(t)
	defer cleanup()
	router := apiRouterWithKeys(a, publishCallers())
	tmpl := seedTemplate(t, db, "Page", "page")
	id := seedContent(t, db, tmpl, "Draft", "update-draft", "/update-draft").Hex()

	if rr := sendJSON(router, "lc_writer", "PUT", "/api/v1/content/"+id, `{"published":true}`); rr.Code != http.StatusForbidden {
		t.Errorf("publishing by update: %d %s", rr.Code, rr.Body.String())
	}
	if _, published := pagePublished(t, db, "update-draft"); published {
		t.Fatal("the page went live")
	}
	if rr := sendJSON(router, "lc_writer", "PUT", "/api/v1/content/by-path?path=/update-draft", `{"published":true}`); rr.Code != http.StatusForbidden {
		t.Errorf("publishing by path: %d %s", rr.Code, rr.Body.String())
	}

	// Sending the page's current state is not publishing.
	if rr := sendJSON(router, "lc_writer", "PUT", "/api/v1/content/"+id, `{"title":"New title","published":false}`); rr.Code != http.StatusOK {
		t.Errorf("same state: %d %s", rr.Code, rr.Body.String())
	}

	if rr := sendJSON(router, "lc_editor", "PUT", "/api/v1/content/"+id, `{"published":true}`); rr.Code != http.StatusOK {
		t.Errorf("editor: %d %s", rr.Code, rr.Body.String())
	}
	if _, published := pagePublished(t, db, "update-draft"); !published {
		t.Error("the editor should have published the page")
	}

	// Taking it down is also a change of state.
	if rr := sendJSON(router, "lc_writer", "PUT", "/api/v1/content/"+id, `{"published":false}`); rr.Code != http.StatusForbidden {
		t.Errorf("unpublishing by update: %d %s", rr.Code, rr.Body.String())
	}
}

func TestPublishPermission_RevertAndRestoreRequireItToChangeWhatIsLive(t *testing.T) {
	a, db, cleanup := newTestAPIHandler(t)
	defer cleanup()
	router := apiRouterWithKeys(a, publishCallers())
	tmpl := seedTemplate(t, db, "Page", "page").Hex()

	// Version 1 is a published page; version 2 takes it down.
	rr := sendJSON(router, "lc_editor", "POST", "/api/v1/content", fmt.Sprintf(`{"template_id":%q,"title":"T","slug":"revert-me","data":{},"published":true}`, tmpl))
	if rr.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", rr.Code, rr.Body.String())
	}
	var page models.Content
	json.Unmarshal(rr.Body.Bytes(), &page)
	id := page.ID.Hex()
	if rr := sendJSON(router, "lc_editor", "PUT", "/api/v1/content/"+id, `{"published":false}`); rr.Code != http.StatusOK {
		t.Fatalf("unpublish: %d %s", rr.Code, rr.Body.String())
	}

	if rr := sendJSON(router, "lc_writer", "POST", "/api/v1/content/"+id+"/versions/1/revert", `{}`); rr.Code != http.StatusForbidden {
		t.Errorf("reverting to a published version: %d %s", rr.Code, rr.Body.String())
	}
	if _, published := pagePublished(t, db, "revert-me"); published {
		t.Fatal("the revert published the page")
	}
	// Reverting to a version in the same state is an ordinary edit.
	if rr := sendJSON(router, "lc_writer", "POST", "/api/v1/content/"+id+"/versions/2/revert", `{}`); rr.Code != http.StatusOK {
		t.Errorf("reverting within the same state: %d %s", rr.Code, rr.Body.String())
	}

	// A page that was live when it was deleted comes back live on restore.
	if rr := sendJSON(router, "lc_editor", "PUT", "/api/v1/content/"+id, `{"published":true}`); rr.Code != http.StatusOK {
		t.Fatalf("republish: %d %s", rr.Code, rr.Body.String())
	}
	if err := a.contentService.DeleteContent(context.Background(), page.ID); err != nil {
		t.Fatalf("DeleteContent: %v", err)
	}
	if rr := sendJSON(router, "lc_writer", "POST", "/api/v1/content/"+id+"/restore", ``); rr.Code != http.StatusForbidden {
		t.Errorf("restoring a page that was live: %d %s", rr.Code, rr.Body.String())
	}
	if rr := sendJSON(router, "lc_editor", "POST", "/api/v1/content/"+id+"/restore", ``); rr.Code != http.StatusOK {
		t.Errorf("editor restoring: %d %s", rr.Code, rr.Body.String())
	}
}

func TestPublishedField_IgnoredOnForkCopies(t *testing.T) {
	a, db, cleanup := newTestAPIHandler(t)
	defer cleanup()
	router := apiRouterWithKeys(a, publishCallers())
	tmpl := seedTemplate(t, db, "Page", "page").Hex()
	fork := createTestFork(t, db, "copies").Hex()

	body := fmt.Sprintf(`{"template_id":%q,"title":"Copy","slug":"fork-copy","data":{},"published":true,"fork_id":%q}`, tmpl, fork)
	rr := sendJSON(router, "lc_writer", "POST", "/api/v1/content", body)
	if rr.Code != http.StatusCreated {
		t.Fatalf("create in fork: %d %s", rr.Code, rr.Body.String())
	}
	var created models.Content
	json.Unmarshal(rr.Body.Bytes(), &created)
	if created.Published {
		t.Error("a fork copy was created published")
	}

	if rr := sendJSON(router, "lc_writer", "PUT", "/api/v1/content/"+created.ID.Hex(), `{"published":true}`); rr.Code != http.StatusOK {
		t.Fatalf("update copy: %d %s", rr.Code, rr.Body.String())
	}
	if _, published := pagePublished(t, db, "fork-copy"); published {
		t.Error("a fork copy was flagged published")
	}
}

func TestPublishPermission_ImportsRequireIt(t *testing.T) {
	a, _, cleanup := newTestAPIHandler(t)
	defer cleanup()
	router := apiRouterWithKeys(a, publishCallers())

	cases := map[string]string{
		"/api/v1/imports/markdown": `{"pages":[{"filename":"a.md","content":"# A"}],"auto_publish":true}`,
		"/api/v1/imports/csv":      `{"csv_data":"title\nA\n","title_column":"title","auto_publish":true}`,
		"/api/v1/imports/sources":  `{"name":"Feed","url":"https://example.org/feed","auto_publish":true}`,
	}
	for target, body := range cases {
		if rr := sendJSON(router, "lc_writer", "POST", target, body); rr.Code != http.StatusForbidden {
			t.Errorf("%s: %d %s", target, rr.Code, rr.Body.String())
		}
	}

	ctx := context.Background()
	draftSource := &models.ImportSource{Name: "Drafts", URL: "https://example.org/a", Schedule: "daily", Active: true}
	if err := a.importService.CreateSource(ctx, draftSource); err != nil {
		t.Fatalf("CreateSource: %v", err)
	}
	if rr := sendJSON(router, "lc_writer", "PUT", "/api/v1/imports/sources/"+draftSource.ID.Hex(), `{"auto_publish":true}`); rr.Code != http.StatusForbidden {
		t.Errorf("turning on auto-publish: %d %s", rr.Code, rr.Body.String())
	}

	publishing := &models.ImportSource{Name: "Live", URL: "https://example.org/b", Schedule: "daily", Active: true, AutoPublish: true}
	if err := a.importService.CreateSource(ctx, publishing); err != nil {
		t.Fatalf("CreateSource: %v", err)
	}
	if rr := sendJSON(router, "lc_writer", "POST", "/api/v1/imports/sources/"+publishing.ID.Hex()+"/trigger", ``); rr.Code != http.StatusForbidden {
		t.Errorf("triggering an auto-publishing source: %d %s", rr.Code, rr.Body.String())
	}
}

func TestMergePublishNew_RequiresPublish(t *testing.T) {
	a, db, cleanup := newTestAPIHandler(t)
	defer cleanup()
	router := apiRouterWithKeys(a, map[string]*auth.SessionUser{
		"lc_merger": keyCaller("merger", models.RoleAdmin, auth.PermForkMerge),
	})
	fork := createTestFork(t, db, "publish-new").Hex()

	if rr := sendJSON(router, "lc_merger", "POST", "/api/v1/forks/"+fork+"/merge", `{"publish_new":true}`); rr.Code != http.StatusForbidden {
		t.Errorf("publish_new without content.publish: %d %s", rr.Code, rr.Body.String())
	}
}

func TestUploadAsset_NeverReadsTheServersDisk(t *testing.T) {
	a, _, cleanup := newTestAPIHandler(t)
	defer cleanup()
	router := apiRouterWithKeys(a, publishCallers())
	secret := filepath.Join(t.TempDir(), "secrets.txt")
	if err := os.WriteFile(secret, []byte("do not leak"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	body := fmt.Sprintf(`{"filename":"x.txt","serve_path":"/assets/x.txt","file_path":%q}`, secret)
	if rr := sendJSON(router, "lc_editor", "POST", "/api/v1/assets", body); rr.Code != http.StatusBadRequest {
		t.Errorf("file_path upload: %d %s", rr.Code, rr.Body.String())
	}
}

func TestStampProvenance(t *testing.T) {
	run := func(user *auth.SessionUser, session string) (services.Provenance, string) {
		var prov services.Provenance
		var email string
		handler := StampProvenance(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			prov, _ = services.ProvenanceFromContext(r.Context())
			email = services.EditorEmailFromContext(r.Context())
		}))
		req := httptest.NewRequest("POST", "/api/v1/content", nil)
		if session != "" {
			req.Header.Set("X-Agent-Session", session)
		}
		if user != nil {
			req = req.WithContext(middleware.InjectAPIUser(req.Context(), user))
		}
		handler.ServeHTTP(httptest.NewRecorder(), req)
		return prov, email
	}

	if prov, _ := run(&auth.SessionUser{Email: "a@x", ViaAPIKey: true}, ""); prov.Actor != "human" {
		t.Errorf("plain API key: actor %q", prov.Actor)
	}
	if prov, email := run(&auth.SessionUser{Email: "a@x", ViaAPIKey: true}, "agent-1"); prov.Actor != "agent" || prov.AgentSession != "agent-1" || email != "a@x" {
		t.Errorf("session header: %+v, %q", prov, email)
	}
	if prov, _ := run(&auth.SessionUser{Email: "a@x", ViaOAuth: true}, ""); prov.Actor != "agent" {
		t.Errorf("OAuth connection: actor %q", prov.Actor)
	}
}

// --- end to end: a real OAuth connection through /mcp to the real REST API ---

type mcpEnvironment struct {
	url   string
	db    *database.DB
	users *services.UserService
	oauth *services.OAuthService
}

const mcpTestBaseURL = "https://cms.example.com"

func newMCPEnvironment(t *testing.T) *mcpEnvironment {
	t.Helper()
	a, db, cleanup := newTestAPIHandler(t)
	t.Cleanup(cleanup)

	users := services.NewUserService(db)
	oauth := services.NewOAuthService(db)
	resolver := auth.NewCredentialResolver(users, services.NewAPIKeyService(db), oauth, services.MCPResourceURL(mcpTestBaseURL), nil)
	apiAuth := middleware.NewAPIAuth(resolver.ResolveAPIKey)
	apiAuth.SetOAuth(resolver.ResolveOAuthToken, mcpTestBaseURL+"/.well-known/oauth-protected-resource")

	r := mux.NewRouter()
	apiv1 := a.MountAPI(r, apiAuth)
	mcpRoute := r.PathPrefix("/mcp").Subrouter()
	mcpRoute.Use(apiAuth.MCPMiddleware)
	mcpRoute.Handle("", lightmcp.NewHTTPHandler(apiv1, ""))

	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)
	return &mcpEnvironment{url: srv.URL, db: db, users: users, oauth: oauth}
}

// connect signs person in through the OAuth service and returns the access
// token and grant.
func (e *mcpEnvironment) connect(t *testing.T, person *models.User) (string, *models.OAuthGrant) {
	t.Helper()
	ctx := context.Background()
	client, _, err := e.oauth.RegisterClient(ctx, "Test App", []string{"http://localhost/cb"})
	if err != nil {
		t.Fatalf("RegisterClient: %v", err)
	}
	grant, err := e.oauth.SaveGrant(ctx, client, person.ID, services.MCPResourceURL(mcpTestBaseURL))
	if err != nil {
		t.Fatalf("SaveGrant: %v", err)
	}
	const (
		verifier  = "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"
		challenge = "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM" // S256 of verifier (RFC 7636 appendix B)
	)
	code, err := e.oauth.CreateAuthCode(ctx, grant, "http://localhost/cb", challenge, "S256")
	if err != nil {
		t.Fatalf("CreateAuthCode: %v", err)
	}
	access, _, _, err := e.oauth.ExchangeAuthCode(ctx, code, client.ClientID, "http://localhost/cb", verifier)
	if err != nil {
		t.Fatalf("ExchangeAuthCode: %v", err)
	}
	return access, grant
}

func (e *mcpEnvironment) person(t *testing.T, email, role string) *models.User {
	t.Helper()
	user, err := e.users.CreateUserWithHash(context.Background(), email, email, role, "unused", false)
	if err != nil {
		t.Fatalf("CreateUserWithHash: %v", err)
	}
	return user
}

type tokenTransport struct{ token string }

func (b tokenTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	r.Header.Set("Authorization", "Bearer "+b.token)
	return http.DefaultTransport.RoundTrip(r)
}

func (e *mcpEnvironment) session(t *testing.T, token string) *mcpsdk.ClientSession {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	client := mcpsdk.NewClient(&mcpsdk.Implementation{Name: "test", Version: "1"}, nil)
	session, err := client.Connect(ctx, &mcpsdk.StreamableClientTransport{
		Endpoint:   e.url + "/mcp",
		HTTPClient: &http.Client{Transport: tokenTransport{token: token}},
		MaxRetries: -1,
	}, nil)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { session.Close() })
	return session
}

func callTool(t *testing.T, session *mcpsdk.ClientSession, name string, args map[string]interface{}) *mcpsdk.CallToolResult {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	result, err := session.CallTool(ctx, &mcpsdk.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	return result
}

func toolText(result *mcpsdk.CallToolResult) string {
	var b strings.Builder
	for _, c := range result.Content {
		if text, ok := c.(*mcpsdk.TextContent); ok {
			b.WriteString(text.Text)
		}
	}
	return b.String()
}

func TestEndToEnd_OAuthConnectionHasThePersonsRole(t *testing.T) {
	env := newMCPEnvironment(t)
	env.person(t, "admin@example.com", models.RoleAdmin) // an admin exists, but nobody here signs in as one
	tmpl := seedTemplate(t, env.db, "Page", "page").Hex()

	createArgs := func(slug string) map[string]interface{} {
		return map[string]interface{}{"template_id": tmpl, "title": "T", "slug": slug, "data": map[string]interface{}{}}
	}

	viewerToken, _ := env.connect(t, env.person(t, "viewer@example.com", models.RoleViewer))
	viewer := env.session(t, viewerToken)

	if result := callTool(t, viewer, "list_templates", map[string]interface{}{}); result.IsError {
		t.Fatalf("a viewer should be able to read: %s", toolText(result))
	}
	result := callTool(t, viewer, "create_content", createArgs("by-viewer"))
	if !result.IsError || !strings.Contains(toolText(result), "insufficient permissions") {
		t.Fatalf("a viewer wrote through OAuth: %s", toolText(result))
	}
	if found, _ := pagePublished(t, env.db, "by-viewer"); found {
		t.Fatal("the viewer's page exists")
	}

	editorToken, _ := env.connect(t, env.person(t, "editor@example.com", models.RoleEditor))
	editor := env.session(t, editorToken)
	if result := callTool(t, editor, "create_content", createArgs("by-editor")); result.IsError {
		t.Fatalf("an editor should be able to create: %s", toolText(result))
	}
	result = callTool(t, editor, "update_site_config", map[string]interface{}{"title_template": "Taken over"})
	if !result.IsError || !strings.Contains(toolText(result), "insufficient permissions") {
		t.Fatalf("an editor changed site configuration: %s", toolText(result))
	}
}

func TestEndToEnd_ChangesAreRecordedAsTheAgentAndTheConnection(t *testing.T) {
	env := newMCPEnvironment(t)
	token, grant := env.connect(t, env.person(t, "editor@example.com", models.RoleEditor))
	tmpl := seedTemplate(t, env.db, "Page", "page").Hex()
	session := env.session(t, token)

	result := callTool(t, session, "create_content", map[string]interface{}{
		"template_id": tmpl, "title": "T", "slug": "agent-made", "data": map[string]interface{}{},
	})
	if result.IsError {
		t.Fatalf("create_content: %s", toolText(result))
	}

	ctx := context.Background()
	var version models.ContentVersion
	var content models.Content
	if err := env.db.FindOne(ctx, "content", bson.M{"slug": "agent-made"}, &content); err != nil {
		t.Fatalf("finding page: %v", err)
	}
	if err := env.db.FindOne(ctx, "content_versions", bson.M{"content_id": content.ID}, &version); err != nil {
		t.Fatalf("finding version: %v", err)
	}
	if version.Actor != "agent" || !strings.HasPrefix(version.AgentSession, "agent-") {
		t.Errorf("version recorded actor %q, session %q", version.Actor, version.AgentSession)
	}

	// The audit entry is written asynchronously.
	deadline := time.Now().Add(5 * time.Second)
	for {
		var entry models.AuditLog
		err := env.db.FindOne(ctx, "audit_logs", bson.M{"action": "content.create", "resource_id": content.ID.Hex()}, &entry)
		if err == nil {
			if entry.Credential != "oauth:"+grant.ID.Hex() || entry.UserEmail != "editor@example.com" || entry.AgentSession == "" {
				t.Errorf("audit entry: %+v", entry)
			}
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("no audit entry: %v", err)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func TestEndToEnd_RevokingAConnectionStopsItAtOnce(t *testing.T) {
	env := newMCPEnvironment(t)
	token, grant := env.connect(t, env.person(t, "editor@example.com", models.RoleEditor))
	session := env.session(t, token)
	if result := callTool(t, session, "list_templates", map[string]interface{}{}); result.IsError {
		t.Fatalf("list_templates: %s", toolText(result))
	}

	if err := env.oauth.RevokeGrant(context.Background(), grant.ID); err != nil {
		t.Fatalf("RevokeGrant: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if _, err := session.CallTool(ctx, &mcpsdk.CallToolParams{Name: "list_templates", Arguments: map[string]interface{}{}}); err == nil {
		t.Error("a revoked connection's open session still works")
	}
}

func TestEndToEnd_OAuthTokenRefusedAtTheRESTAPI(t *testing.T) {
	env := newMCPEnvironment(t)
	token, _ := env.connect(t, env.person(t, "admin@example.com", models.RoleAdmin))

	req, _ := http.NewRequest("GET", env.url+"/api/v1/content", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("got %d, want 401", resp.StatusCode)
	}
}
