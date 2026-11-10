package handlers

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/metavert-io/lightcms/v7/internal/auth"
	"github.com/metavert-io/lightcms/v7/internal/database"
	"github.com/metavert-io/lightcms/v7/internal/middleware"
	"github.com/metavert-io/lightcms/v7/internal/models"

	"github.com/gorilla/mux"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// sandboxTestCredential is the key sandboxOnlyReq signs in with.
const sandboxTestCredential = "apikey:sandbox-test"

// ownedBySandboxKey records that the sandboxOnlyReq key created the fork, so
// it may write to it.
func ownedBySandboxKey(t *testing.T, db *database.DB, forkID primitive.ObjectID) {
	t.Helper()
	if err := db.UpdateOne(context.Background(), "content_forks", bson.M{"_id": forkID}, bson.M{"$set": bson.M{"created_by_credential": sandboxTestCredential}}); err != nil {
		t.Fatalf("ownedBySandboxKey: %v", err)
	}
}

// sandboxOnlyReq builds a request authenticated as an admin whose API key is
// sandbox-only.
func sandboxOnlyReq(method, target string, body io.Reader) *http.Request {
	req := httptest.NewRequest(method, target, body)
	u := &auth.SessionUser{
		ID: "000000000000000000000001", Email: "agent@localhost",
		Role: "admin", ViaAPIKey: true, SandboxOnly: true,
		Credential: sandboxTestCredential,
	}
	return req.WithContext(middleware.InjectAPIUser(req.Context(), u))
}

func TestSandboxOnlyKey_Enforcement(t *testing.T) {
	a, db, cleanup := newTestAPIHandler(t)
	defer cleanup()

	tmplID := seedTemplate(t, db, "Page", "page")
	forkID := createTestFork(t, db, "gov")
	ownedBySandboxKey(t, db, forkID)

	// Live create without fork_id → 403.
	req := sandboxOnlyReq("POST", "/api/v1/content", strings.NewReader(
		`{"template_id":"`+tmplID.Hex()+`","title":"Live","slug":"live-x","data":{}}`))
	rr := httptest.NewRecorder()
	a.APICreateContent(rr, req)
	if rr.Code != 403 {
		t.Errorf("live create: status = %d, want 403 (body: %s)", rr.Code, rr.Body.String())
	}

	// Create inside a fork → allowed.
	req = sandboxOnlyReq("POST", "/api/v1/content", strings.NewReader(
		`{"template_id":"`+tmplID.Hex()+`","title":"Sandboxed","slug":"sb-x","data":{},"fork_id":"`+forkID.Hex()+`"}`))
	rr = httptest.NewRecorder()
	a.APICreateContent(rr, req)
	if rr.Code != 201 {
		t.Errorf("fork create: status = %d, want 201 (body: %s)", rr.Code, rr.Body.String())
	}

	// Update a live page → 403.
	liveID := seedContent(t, db, tmplID, "Live Page", "lp", "/lp")
	req = sandboxOnlyReq("PUT", "/api/v1/content/"+liveID.Hex(), strings.NewReader(`{"title":"Hack"}`))
	req = mux.SetURLVars(req, map[string]string{"id": liveID.Hex()})
	rr = httptest.NewRecorder()
	a.APIUpdateContent(rr, req)
	if rr.Code != 403 {
		t.Errorf("live update: status = %d, want 403", rr.Code)
	}

	// Update a fork copy → allowed.
	forkCopyID := primitive.NewObjectID()
	now := time.Now()
	_, _ = db.InsertOne(context.Background(), "content", &models.Content{
		ID: forkCopyID, TemplateID: tmplID, Title: "Copy", Slug: "lp", FullPath: "/lp",
		ForkID: &forkID, CreatedAt: now, UpdatedAt: now,
	})
	req = sandboxOnlyReq("PUT", "/api/v1/content/"+forkCopyID.Hex(), strings.NewReader(`{"title":"Edited"}`))
	req = mux.SetURLVars(req, map[string]string{"id": forkCopyID.Hex()})
	rr = httptest.NewRecorder()
	a.APIUpdateContent(rr, req)
	if rr.Code != 200 {
		t.Errorf("fork update: status = %d, want 200 (body: %s)", rr.Code, rr.Body.String())
	}

	// Update-by-path resolves to live → 403.
	req = sandboxOnlyReq("PUT", "/api/v1/content/by-path?path=/lp", strings.NewReader(`{"title":"Hack"}`))
	rr = httptest.NewRecorder()
	a.APIUpdateContentByPath(rr, req)
	if rr.Code != 403 {
		t.Errorf("by-path live update: status = %d, want 403", rr.Code)
	}

	// Publish → 403 via permission gate.
	req = sandboxOnlyReq("POST", "/api/v1/content/"+liveID.Hex()+"/publish", nil)
	req = mux.SetURLVars(req, map[string]string{"id": liveID.Hex()})
	rr = httptest.NewRecorder()
	a.APIPublishContent(rr, req)
	if rr.Code != 403 {
		t.Errorf("publish: status = %d, want 403", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "limited to its own forks") {
		t.Errorf("publish denial should explain the sandbox, got: %s", rr.Body.String())
	}

	// Delete → 403.
	req = sandboxOnlyReq("DELETE", "/api/v1/content/"+liveID.Hex(), nil)
	req = mux.SetURLVars(req, map[string]string{"id": liveID.Hex()})
	rr = httptest.NewRecorder()
	a.APIDeleteContent(rr, req)
	if rr.Code != 403 {
		t.Errorf("delete: status = %d, want 403", rr.Code)
	}
}

func TestAPICreateAPIKey_ScopesValidation(t *testing.T) {
	a, _, cleanup := newTestAPIHandler(t)
	defer cleanup()

	// Unknown scope rejected.
	req := authReq("POST", "/api/v1/apikeys", strings.NewReader(
		`{"name":"agent-key","scopes":["content.view","not.a.perm"]}`))
	rr := httptest.NewRecorder()
	a.APICreateAPIKey(rr, req)
	if rr.Code != 400 {
		t.Errorf("unknown scope: status = %d, want 400 (body: %s)", rr.Code, rr.Body.String())
	}

	// Valid scoped sandbox key created and persisted with flags.
	req = authReq("POST", "/api/v1/apikeys", strings.NewReader(
		`{"name":"agent-key","scopes":["content.view","content.edit"],"sandbox_only":true}`))
	rr = httptest.NewRecorder()
	a.APICreateAPIKey(rr, req)
	if rr.Code != 201 {
		t.Fatalf("create: status = %d (body: %s)", rr.Code, rr.Body.String())
	}
	var resp struct {
		ID  string `json:"id"`
		Key string `json:"key"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !strings.HasPrefix(resp.Key, "lc_") {
		t.Errorf("key format: %q", resp.Key)
	}

	stored, err := a.apiKeyService.ValidateAPIKey(context.Background(), resp.Key)
	if err != nil {
		t.Fatalf("ValidateAPIKey: %v", err)
	}
	if !stored.SandboxOnly {
		t.Error("sandbox_only not persisted")
	}
	if len(stored.Scopes) != 2 || stored.Scopes[1] != "content.edit" {
		t.Errorf("scopes not persisted: %v", stored.Scopes)
	}
}
