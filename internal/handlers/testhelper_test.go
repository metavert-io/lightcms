package handlers

import (
	"bufio"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/metavert-io/lightcms/v7/internal/auth"
	"github.com/metavert-io/lightcms/v7/internal/database"
	"github.com/metavert-io/lightcms/v7/internal/middleware"
	"github.com/metavert-io/lightcms/v7/internal/services"
	"github.com/metavert-io/lightcms/v7/internal/testutil"

	"github.com/gorilla/mux"
	"github.com/gorilla/sessions"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/writeconcern"
)

// adminAPIUser returns a SessionUser with the admin role, as produced by API-key
// authentication. The admin role carries every RBAC permission.
func adminAPIUser() *auth.SessionUser {
	return &auth.SessionUser{
		ID:        "000000000000000000000001",
		Email:     "admin@localhost",
		Role:      "admin",
		ViaAPIKey: true,
	}
}

// testSessionSecret must match the secret used to construct the auth manager's
// cookie store in newTestHandler, so forged session cookies validate.
const testSessionSecret = "test-session-secret"

// sessionReq builds an httptest request carrying a valid admin session cookie,
// as produced by a logged-in admin. It is the session-auth counterpart to
// authReq, for exercising the /cm admin UI handlers. CSRF is enforced by
// middleware (not the handlers themselves), so direct handler calls bypass it.
func sessionReq(method, target string, body io.Reader, vars map[string]string) *http.Request {
	store := sessions.NewCookieStore([]byte(testSessionSecret))
	req := httptest.NewRequest(method, target, body)
	rec := httptest.NewRecorder()
	sess, _ := store.New(req, "lightcms-session")
	sess.Values["authenticated"] = true
	sess.Values["user_id"] = "000000000000000000000001"
	sess.Values["user_email"] = "admin@localhost"
	sess.Values["user_role"] = "admin"
	_ = sess.Save(req, rec)
	for _, c := range rec.Result().Cookies() {
		req.AddCookie(c)
	}
	if vars != nil {
		req = mux.SetURLVars(req, vars)
	}
	return req
}

// authReq builds an httptest request with an authenticated admin API user
// injected into its context. It is a drop-in replacement for
// httptest.NewRequest for APIHandler tests: as of the API-key-requires-user
// security change, handlers reject requests with no user context (403).
func authReq(method, target string, body io.Reader) *http.Request {
	req := httptest.NewRequest(method, target, body)
	return req.WithContext(middleware.InjectAPIUser(req.Context(), adminAPIUser()))
}

var (
	sharedTestOnce sync.Once
	sharedTestDB   *database.DB
	sharedTestErr  error
)

// loadTestEnv walks up from the working directory looking for .env.test and
// loads it into the process environment.
func loadTestEnv(t *testing.T) {
	t.Helper()
	dir, _ := os.Getwd()
	for i := 0; i < 6; i++ {
		data, err := os.ReadFile(filepath.Join(dir, ".env.test"))
		if err == nil {
			scanner := bufio.NewScanner(strings.NewReader(string(data)))
			for scanner.Scan() {
				line := strings.TrimSpace(scanner.Text())
				if line == "" || strings.HasPrefix(line, "#") {
					continue
				}
				if idx := strings.IndexByte(line, '='); idx > 0 {
					os.Setenv(strings.TrimSpace(line[:idx]), strings.TrimSpace(line[idx+1:]))
				}
			}
			return
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
}

// testDB returns a shared test database connection, skipping if MONGODB_URI
// is not set. The connection is created once per package binary using sync.Once.
func testDB(t *testing.T) *database.DB {
	t.Helper()
	sharedTestOnce.Do(func() {
		loadTestEnv(t)
		uri := os.Getenv("MONGODB_URI")
		if uri == "" {
			return
		}
		dbName := os.Getenv("DATABASE_NAME")
		if dbName == "" {
			dbName = "lightcms-test"
		}
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		wc := writeconcern.New(writeconcern.WMajority())
		wcOpts := options.Client().SetWriteConcern(wc)
		db, err := database.Connect(ctx, uri, dbName, wcOpts)
		if err != nil {
			sharedTestErr = err
			return
		}
		sharedTestDB = db
	})
	if sharedTestErr != nil {
		t.Fatalf("testDB: %v", sharedTestErr)
	}
	if sharedTestDB == nil {
		t.Skip("skipping: MONGODB_URI not set")
	}
	return sharedTestDB
}

// cleanupCollections drops test collections for isolation.
// Dropping (rather than just deleting documents) ensures indexes are also cleared,
// preventing duplicate key errors on re-creation and avoiding Atlas collection
// accumulation when tests use unique indexes (e.g. users.email).
// Drops are issued in parallel to minimise Atlas round-trip overhead.
func cleanupCollections(t *testing.T, db *database.DB) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	collections := []string{
		"content", "content_versions", "templates", "folders", "collections",
		"settings", "theme_versions", "assets", "api_keys", "redirects",
		"snippets", "users", "audit_logs", "contact_messages", "login_attempts",
		"oauth_clients", "oauth_grants", "user_activity",
		// Forks carry a preview token and webhooks fire on content events:
		// leftovers from an earlier test (or run) change what a later one sees.
		"content_forks", "webhooks", "webhook_deliveries", "content_comments",
		"content_locks", "approval_workflows", "approval_requests",
		"import_sources", "import_jobs",
	}
	var wg sync.WaitGroup
	for _, name := range collections {
		wg.Add(1)
		go func(col string) {
			defer wg.Done()
			db.Collection(col).Drop(ctx) //nolint:errcheck
		}(name)
	}
	wg.Wait()
}

// newTestHandler returns a Handler wired to a real test DB and a cleanup function.
func newTestHandler(t *testing.T) (*Handler, func()) {
	t.Helper()
	db := testDB(t)
	cleanupCollections(t, db)

	// Services
	userService := services.NewUserService(db)
	auditService := services.NewAuditService(db)
	snippetService := services.NewSnippetService(db)
	contentService := services.NewContentService(db)
	searchService := services.NewSearchService(db, "") // no Voyage key in tests
	apiKeyService := services.NewAPIKeyService(db)
	_ = apiKeyService // used internally by New()

	// Session store
	store := sessions.NewCookieStore([]byte("test-session-secret"))

	// Auth manager
	authManager := auth.NewManager(store, db, userService)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := authManager.MigrateToMultiUser(ctx); err != nil {
		t.Fatalf("MigrateToMultiUser: %v", err)
	}

	// Handler
	h := New(db, authManager, "http://localhost:8082", "test", userService, auditService, snippetService)
	h.SetSearchService(searchService)
	h.SetContentService(contentService)
	h.SetProxyConfig(nil)

	// Wire the optional services used by the admin UI handlers (imports,
	// webhooks, analytics, locks, comments, approvals, forks).
	webhookService := services.NewWebhookService(db)
	commentService := services.NewCommentService(db)
	h.SetImportService(services.NewImportService(db, contentService))
	h.SetWebhookService(webhookService)
	h.SetCommentService(commentService)
	h.SetForkService(services.NewForkService(db, contentService))
	h.SetApprovalService(services.NewApprovalService(db, contentService, commentService, webhookService))
	h.SetAnalyticsService(services.NewAnalyticsService(context.Background(), db, "http://localhost:8082"))
	h.SetOAuthService(services.NewOAuthService(db))

	cleanup := func() {
		cleanupCollections(t, db)
	}
	return h, cleanup
}

// brokenDBOnce builds a single MongoDB connection that is immediately
// disconnected, so every subsequent operation returns an error. This lets tests
// exercise the (otherwise unreachable) "if err != nil { 500 }" database-error
// branches that pepper the handlers.
var (
	brokenDBOnce sync.Once
	brokenDBConn *database.DB
)

func getBrokenDB(t *testing.T) *database.DB {
	t.Helper()
	brokenDBOnce.Do(func() {
		loadTestEnv(t)
		uri := os.Getenv("MONGODB_URI")
		if uri == "" {
			return
		}
		dbName := os.Getenv("DATABASE_NAME")
		if dbName == "" {
			dbName = "lightcms-test"
		}
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		wc := writeconcern.New(writeconcern.WMajority())
		db, err := database.Connect(ctx, uri, dbName, options.Client().SetWriteConcern(wc))
		if err != nil {
			return
		}
		_ = db.Disconnect(context.Background()) // kill the client; ops now error
		brokenDBConn = db
	})
	if brokenDBConn == nil {
		t.Skip("skipping: MONGODB_URI not set")
	}
	return brokenDBConn
}

// newBrokenAPIHandler returns an APIHandler whose services are wired to a dead
// (disconnected) database, so handler DB calls fail and their error paths run.
func newBrokenAPIHandler(t *testing.T) *APIHandler {
	t.Helper()
	db := getBrokenDB(t)
	contentService := services.NewContentService(db)
	templateService := services.NewTemplateService(db, contentService)
	assetService := services.NewAssetService(db)
	settingsService := services.NewSettingsService(db, contentService)
	apiKeyService := services.NewAPIKeyService(db)
	auditService := services.NewAuditService(db)
	snippetService := services.NewSnippetService(db)
	webhookService := services.NewWebhookService(db)
	commentService := services.NewCommentService(db)

	ah := NewAPIHandler(contentService, templateService, assetService, settingsService, apiKeyService, auditService, snippetService)
	ah.SetSearchService(services.NewSearchService(db, ""))
	ah.SetForkService(services.NewForkService(db, contentService))
	ah.SetWebhookServiceAPI(webhookService)
	ah.SetCommentService(commentService)
	ah.SetLockServiceAPI(services.NewLockService(db))
	ah.SetLinkCheckerService(services.NewLinkCheckerService(db))
	ah.SetImportService(services.NewImportService(db, contentService))
	ah.SetUserService(services.NewUserService(db))
	ah.SetApprovalService(services.NewApprovalService(db, contentService, commentService, webhookService))
	return ah
}

// newBrokenHandler returns a session Handler wired to a dead database.
func newBrokenHandler(t *testing.T) *Handler {
	t.Helper()
	db := getBrokenDB(t)
	userService := services.NewUserService(db)
	auditService := services.NewAuditService(db)
	snippetService := services.NewSnippetService(db)
	contentService := services.NewContentService(db)
	webhookService := services.NewWebhookService(db)
	commentService := services.NewCommentService(db)
	store := sessions.NewCookieStore([]byte(testSessionSecret))
	authManager := auth.NewManager(store, db, userService)

	h := New(db, authManager, "http://localhost:8082", "test", userService, auditService, snippetService)
	h.SetSearchService(services.NewSearchService(db, ""))
	h.SetContentService(contentService)
	h.SetProxyConfig(nil)
	h.SetImportService(services.NewImportService(db, contentService))
	h.SetWebhookService(webhookService)
	h.SetCommentService(commentService)
	h.SetForkService(services.NewForkService(db, contentService))
	h.SetApprovalService(services.NewApprovalService(db, contentService, commentService, webhookService))
	h.SetAnalyticsService(services.NewAnalyticsService(context.Background(), db, "http://localhost:8082"))
	return h
}

// newFaultAPIHandler returns an APIHandler wired to a live fault-injectable DB
// (via testutil.MustConnectFaultDB) plus the DB so the test can install a fault
// hook with db.SetFaultHook to exercise handler write-error branches.
func newFaultAPIHandler(t *testing.T) (*APIHandler, *database.DB) {
	t.Helper()
	db := testutil.MustConnectFaultDB(t)
	cleanupCollections(t, db)
	contentService := services.NewContentService(db)
	templateService := services.NewTemplateService(db, contentService)
	assetService := services.NewAssetService(db)
	settingsService := services.NewSettingsService(db, contentService)
	apiKeyService := services.NewAPIKeyService(db)
	auditService := services.NewAuditService(db)
	snippetService := services.NewSnippetService(db)
	webhookService := services.NewWebhookService(db)
	commentService := services.NewCommentService(db)
	ah := NewAPIHandler(contentService, templateService, assetService, settingsService, apiKeyService, auditService, snippetService)
	ah.SetForkService(services.NewForkService(db, contentService))
	ah.SetWebhookServiceAPI(webhookService)
	ah.SetCommentService(commentService)
	ah.SetLockServiceAPI(services.NewLockService(db))
	ah.SetUserService(services.NewUserService(db))
	ah.SetApprovalService(services.NewApprovalService(db, contentService, commentService, webhookService))
	t.Cleanup(func() { db.SetFaultHook(nil); cleanupCollections(t, db) })
	return ah, db
}

// newTestAPIHandler returns an APIHandler wired to a real test DB, the DB itself
// (for seeding data), and a cleanup function.
func newTestAPIHandler(t *testing.T) (*APIHandler, *database.DB, func()) {
	t.Helper()
	db := testDB(t)
	cleanupCollections(t, db)

	// Services
	contentService := services.NewContentService(db)
	templateService := services.NewTemplateService(db, contentService)
	assetService := services.NewAssetService(db)
	settingsService := services.NewSettingsService(db, contentService)
	apiKeyService := services.NewAPIKeyService(db)
	auditService := services.NewAuditService(db)
	snippetService := services.NewSnippetService(db)
	searchService := services.NewSearchService(db, "")

	ah := NewAPIHandler(contentService, templateService, assetService, settingsService, apiKeyService, auditService, snippetService)
	ah.SetSearchService(searchService)

	// Wire the optional services exercised by the forks/webhooks/comments/
	// locks/imports/approvals/link-check API handlers.
	webhookService := services.NewWebhookService(db)
	commentService := services.NewCommentService(db)
	ah.SetForkService(services.NewForkService(db, contentService))
	ah.SetWebhookServiceAPI(webhookService)
	ah.SetCommentService(commentService)
	ah.SetLockServiceAPI(services.NewLockService(db))
	ah.SetLinkCheckerService(services.NewLinkCheckerService(db))
	ah.SetImportService(services.NewImportService(db, contentService))
	ah.SetUserService(services.NewUserService(db))
	ah.SetApprovalService(services.NewApprovalService(db, contentService, commentService, webhookService))
	ah.SetOAuthService(services.NewOAuthService(db))

	cleanup := func() {
		cleanupCollections(t, db)
	}
	return ah, db, cleanup
}

// seedTemplate inserts a minimal test template and returns its ID.
func seedTemplate(t *testing.T, db *database.DB, name, slug string) primitive.ObjectID {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	now := time.Now()
	id := primitive.NewObjectID()
	doc := bson.M{
		"_id":         id,
		"name":        name,
		"slug":        slug,
		"html_layout": "<html><body>{{.Body}}</body></html>",
		"fields":      bson.A{},
		"created_at":  now,
		"updated_at":  now,
	}
	_, err := db.Collection("templates").InsertOne(ctx, doc)
	if err != nil {
		t.Fatalf("seedTemplate: %v", err)
	}
	return id
}

// seedContent inserts a minimal test content item and returns its ID.
func seedContent(t *testing.T, db *database.DB, templateID primitive.ObjectID, title, slug, fullPath string) primitive.ObjectID {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	now := time.Now()
	id := primitive.NewObjectID()
	doc := bson.M{
		"_id":         id,
		"template_id": templateID,
		"title":       title,
		"slug":        slug,
		"full_path":   fullPath,
		"status":      "draft",
		"fields":      bson.M{},
		"created_at":  now,
		"updated_at":  now,
		"deleted":     false,
	}
	_, err := db.Collection("content").InsertOne(ctx, doc)
	if err != nil {
		t.Fatalf("seedContent: %v", err)
	}
	return id
}
