// Command server runs the LightCMS HTTP server: the admin dashboard, REST API, and public site.
package main

import (
	"context"
	"crypto/sha256"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"github.com/jonradoff/lightcms/v7/config"
	"github.com/jonradoff/lightcms/v7/internal/auth"
	"github.com/jonradoff/lightcms/v7/internal/build"
	"github.com/jonradoff/lightcms/v7/internal/database"
	"github.com/jonradoff/lightcms/v7/internal/handlers"
	lightmcp "github.com/jonradoff/lightcms/v7/internal/mcp"
	"github.com/jonradoff/lightcms/v7/internal/middleware"
	"github.com/jonradoff/lightcms/v7/internal/models"
	"github.com/jonradoff/lightcms/v7/internal/oauth"
	"github.com/jonradoff/lightcms/v7/internal/services"

	"github.com/gorilla/mux"
	"github.com/gorilla/sessions"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

func main() {
	// Load configuration from JSON config file
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	log.Printf("Starting LightCMS in %s mode", cfg.Env)

	// Validate required config
	if cfg.MongoURI == "" {
		log.Fatal("mongo_uri is required in config file")
	}
	if cfg.SessionSecret == "" {
		log.Fatal("session_secret is required in config file")
	}
	// Enforce minimum SESSION_SECRET entropy.
	// Production requires 32+ characters; development warns below 16.
	if cfg.Env == "production" || cfg.Env == "prod" {
		if len(cfg.SessionSecret) < 32 {
			log.Fatalf("session_secret is too short for production (minimum 32 characters, got %d)", len(cfg.SessionSecret))
		}
	} else {
		if len(cfg.SessionSecret) < 16 {
			log.Printf("WARNING: session_secret is too short (minimum 16 characters; 32+ recommended)")
		} else if len(cfg.SessionSecret) < 32 {
			log.Printf("WARNING: session_secret should be at least 32 characters for production use")
		}
	}

	// Connect to MongoDB
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	db, err := database.Connect(ctx, cfg.MongoURI, cfg.DatabaseName)
	if err != nil {
		log.Fatalf("Failed to connect to MongoDB: %v", err)
	}
	defer db.Disconnect(context.Background())

	log.Printf("Connected to MongoDB successfully (database %q)", cfg.DatabaseName)

	// Initialize session store with secure settings
	sessionStore := sessions.NewCookieStore([]byte(cfg.SessionSecret))
	sessionStore.Options = &sessions.Options{
		Path:     "/",
		MaxAge:   86400, // 24 hours (reduced from 7 days for security)
		HttpOnly: true,
		Secure:   cfg.SecureCookies,       // true in production (requires HTTPS)
		SameSite: http.SameSiteStrictMode, // Prevent CSRF via cookies
	}

	// Initialize user and audit services
	userService := services.NewUserService(db)
	auditService := services.NewAuditService(db)

	// Initialize auth manager with database connection
	authManager := auth.NewManager(sessionStore, db, userService)

	// Migrate to multi-user system (creates first admin user from legacy password if needed)
	if err := authManager.MigrateToMultiUser(context.Background()); err != nil {
		log.Printf("Warning: Failed to migrate to multi-user: %v", err)
	}

	// Initialize snippet service (needed by both handlers and API handler)
	snippetService := services.NewSnippetService(db)

	// Initialize analytics service (DAU/MAU tracking)
	analyticsService := services.NewAnalyticsService(context.Background(), db, cfg.BaseURL)

	// Initialize handlers with config
	h := handlers.New(db, authManager, cfg.BaseURL, cfg.Env, userService, auditService, snippetService)
	h.SetAnalyticsService(analyticsService)

	// Initialize trusted proxy config for rate limiting
	proxyConfig := middleware.DefaultCloudConfig()

	// Seed default data if needed
	if err := h.SeedDefaults(context.Background()); err != nil {
		log.Printf("Warning: Failed to seed defaults: %v", err)
	}

	// Check for version migration
	if err := checkVersionMigration(db); err != nil {
		log.Printf("Warning: Failed to check version migration: %v", err)
	}

	// Migrate legacy assets to have serve_path
	if err := db.MigrateAssetServePaths(context.Background()); err != nil {
		log.Printf("Warning: Failed to migrate asset serve paths: %v", err)
	}

	// Ensure theme version 1 exists (save current theme as first version if no versions exist)
	settingsService := services.NewSettingsService(db, services.NewContentService(db))
	if err := settingsService.EnsureThemeVersion1(context.Background()); err != nil {
		log.Printf("Warning: Failed to ensure theme version 1: %v", err)
	}

	// Regenerate theme CSS on startup — ensures static/css/theme-vars.css exists after deploy/restart
	if err := settingsService.EnsureThemeCSS(context.Background()); err != nil {
		log.Printf("Warning: Failed to regenerate theme CSS on startup: %v", err)
	}

	// Initialize services for API and change watcher
	contentService := services.NewContentService(db)
	h.SetContentService(contentService)
	templateService := services.NewTemplateService(db, contentService)
	assetService := services.NewAssetService(db)

	// Initialize v4.5 services
	webhookService := services.NewWebhookService(db)
	lockService := services.NewLockService(db)
	importService := services.NewImportService(db, contentService)
	cfService := services.NewCloudflareService(func() (zoneID, apiToken string, enabled bool) {
		cfg, err := db.GetSiteConfig(context.Background())
		if err != nil || cfg == nil {
			return "", "", false
		}
		return cfg.CloudflareZoneID, cfg.CloudflareAPIToken, cfg.CFCacheEnabled
	}, cfg.BaseURL)
	regenQueue := services.NewRegenQueue(db, contentService)
	schedulerService := services.NewSchedulerService(db, contentService)

	// Wire services together
	contentService.SetWebhookService(webhookService)
	contentService.SetCloudflareService(cfService)

	// IndexNow: notify search engines of changed URLs. Self-gating — inactive
	// in development or without a public BASE_URL, and verifies its own key
	// file through BASE_URL before submitting anything.
	indexNowService := services.NewIndexNowService(db, cfg.BaseURL, cfg.IsDev())
	seoService := services.NewSEOService(db)
	contentService.SetIndexNowService(indexNowService)
	templateService.SetRegenQueue(regenQueue)

	// Start background goroutines
	bgCtx, bgCancel := context.WithCancel(context.Background())
	defer bgCancel()
	go schedulerService.Start(bgCtx)
	go indexNowService.Start(bgCtx)
	regenQueue.Start(bgCtx)

	// Inject new services into handler
	h.SetWebhookService(webhookService)
	h.SetImportService(importService)
	h.SetCloudflareService(cfService)
	h.SetIndexNowService(indexNowService)
	h.SetSEOService(seoService)

	// Initialize search service (always available; semantic search requires Voyage API key)
	searchService := services.NewSearchService(db, cfg.VoyageAPIKey)
	contentService.SetSearchService(searchService) // Always set — needed for keyword cache rebuild on content changes
	if cfg.VoyageAPIKey != "" {
		log.Println("End-user search enabled (fulltext + semantic via Voyage AI)")
	} else {
		log.Println("End-user search enabled (fulltext only — set VOYAGE_API_KEY for semantic search)")
	}

	// Build search keyword cache on startup
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := searchService.RebuildKeywords(ctx); err != nil {
			log.Printf("Warning: failed to build search keywords: %v", err)
		}
	}()

	// Start content change watcher for real-time sync with database changes
	// This enables automatic static page regeneration when content is modified via MCP
	watchCtx, watchCancel := context.WithCancel(context.Background())
	defer watchCancel()
	go contentService.WatchForChanges(watchCtx)

	// Initialize fork service
	forkService := services.NewForkService(db, contentService)
	h.SetForkService(forkService)

	// Initialize comment and approval services (wired into handlers after apiHandler is created below)
	commentService := services.NewCommentService(db)
	commentService.SetWebhookService(webhookService)
	approvalService := services.NewApprovalService(db, contentService, commentService, webhookService)
	h.SetCommentService(commentService)
	h.SetApprovalService(approvalService)

	// Wire search service into handlers
	h.SetSearchService(searchService)
	h.SetProxyConfig(proxyConfig)
	h.SetAnthropicAPIKey(cfg.AnthropicAPIKey)

	// Setup router
	r := mux.NewRouter()

	// Apply security headers middleware to all routes
	// (HSTS follows configuration — secure cookies + an https base URL — not a request header)
	r.Use(middleware.SecurityHeadersWithHSTS(middleware.HSTSEnabled(cfg.SecureCookies, cfg.BaseURL)))

	// Static files
	r.PathPrefix("/static/").Handler(http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))
	r.PathPrefix("/uploads/").Handler(http.StripPrefix("/uploads/", http.FileServer(http.Dir("static/uploads"))))

	// CSRF protection for admin routes
	// Derive a 32-byte CSRF key by hashing the session secret with SHA-256.
	// This avoids zero-padding (which reduces entropy) while remaining deterministic.
	csrfHash := sha256.Sum256([]byte(cfg.SessionSecret))
	csrfKey := csrfHash[:]

	csrfMiddleware := middleware.CSRFProtect(csrfKey, cfg.SecureCookies)

	// gorilla/csrf v1.7.x defaults the assumed request scheme to "https" for
	// Origin/Referer checks. On plaintext HTTP the browser-sent Origin
	// (e.g. http://localhost:8082) won't match, so flag requests as plaintext.
	if !cfg.SecureCookies {
		inner := csrfMiddleware
		csrfMiddleware = func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				inner(next).ServeHTTP(w, csrf.PlaintextHTTPRequest(r))
			})
		}
	}

	// Admin routes (under /cm)
	admin := r.PathPrefix("/cm").Subrouter()
	admin.Use(csrfMiddleware)
	// Every /cm route and the permission it requires is declared in
	// handlers.AdminRoutes; registration puts the check in front of the handler.
	h.RegisterAdminRoutes(admin)

	// REST API v1 routes (API key authenticated, JSON only)
	apiKeyService := services.NewAPIKeyService(db)
	linkCheckerService := services.NewLinkCheckerService(db)
	maintenanceService := services.NewMaintenanceService(db, linkCheckerService)
	go maintenanceService.Start(bgCtx)
	h.SetMaintenanceService(maintenanceService)

	// CMS Agent: email digests of the daily analyses (Resend for delivery)
	emailService := services.NewEmailService(cfg.ResendAPIKey, cfg.EmailFrom)
	agentService := services.NewAgentService(db, emailService, maintenanceService,
		analyticsService, forkService, approvalService, cfg.BaseURL, cfg.AnthropicAPIKey)
	go agentService.Start(bgCtx)
	h.SetAgentService(agentService)
	apiHandler := handlers.NewAPIHandler(contentService, templateService, assetService, settingsService, apiKeyService, auditService, snippetService)
	apiHandler.SetSearchService(searchService)
	apiHandler.SetForkService(forkService)
	apiHandler.SetImportService(importService)
	apiHandler.SetWebhookServiceAPI(webhookService)
	apiHandler.SetLockServiceAPI(lockService)
	apiHandler.SetLinkCheckerService(linkCheckerService)
	apiHandler.SetCommentService(commentService)
	apiHandler.SetApprovalService(approvalService)
	apiHandler.SetUserService(userService)
	apiHandler.SetAgentSessionService(services.NewAgentSessionService(auditService, contentService))
	apiHandler.SetMaintenanceService(maintenanceService)
	apiHandler.SetIndexNowService(indexNowService)
	apiHandler.SetSEOService(seoService)
	apiHandler.SetAnalyticsService(analyticsService)
	apiHandler.SetBaseURL(cfg.BaseURL)
	apiAuthMiddleware := middleware.NewAPIAuth(func(ctx context.Context, rawKey string) (interface{}, error) {
		apiKey, err := apiKeyService.ValidateAPIKey(ctx, rawKey)
		if err != nil {
			return nil, err
		}
		// Resolve the user who owns this API key
		if apiKey.UserID != nil {
			user, err := userService.GetByID(ctx, *apiKey.UserID)
			if err == nil && user != nil {
				go analyticsService.RecordActivity(context.Background(), user.ID.Hex())
				return &auth.SessionUser{
					ID:          user.ID.Hex(),
					Email:       user.Email,
					Role:        user.Role,
					ViaAPIKey:   true,
					Scopes:      apiKey.Scopes,
					SandboxOnly: apiKey.SandboxOnly,
				}, nil
			}
		}
		// Legacy key without user association — auto-migrate by associating with the first admin user.
		log.Printf("[security] API key %s... has no user — attempting auto-migration to admin user", rawKey[:min(10, len(rawKey))])
		if adminUsers, err := userService.ListUsers(ctx); err == nil {
			for _, u := range adminUsers {
				if u.Role == models.RoleAdmin && !u.Disabled {
					uid := u.ID
					db.UpdateOne(ctx, "api_keys", bson.M{"_id": apiKey.ID}, bson.M{"$set": bson.M{"user_id": uid}})
					log.Printf("[security] Auto-migrated legacy API key to admin user %s", u.Email)
					go analyticsService.RecordActivity(context.Background(), u.ID.Hex())
					return &auth.SessionUser{
						ID:        u.ID.Hex(),
						Email:     u.Email,
						Role:      u.Role,
						ViaAPIKey: true,
					}, nil
				}
			}
		}
		// No admin user found — reject the key
		return nil, fmt.Errorf("legacy key has no user and no admin user available for migration")
	})

	// OAuth 2.1 setup for MCP Cowork connector support
	oauthService := services.NewOAuthService(db)
	// Find an admin user to associate with the system API key so OAuth-authenticated
	// MCP sessions resolve to admin-level permissions (not nil → permission bypass).
	var systemKeyAdminID *primitive.ObjectID
	if adminUsers, err := userService.ListUsers(context.Background()); err == nil {
		for _, u := range adminUsers {
			if u.Role == models.RoleAdmin && !u.Disabled {
				id := u.ID
				systemKeyAdminID = &id
				break
			}
		}
	}
	systemAPIKey, err := services.EnsureSystemAPIKey(context.Background(), db, apiKeyService, systemKeyAdminID)
	if err != nil {
		log.Fatalf("Failed to create system API key: %v", err)
	}
	resourceMetadataURL := cfg.BaseURL + "/.well-known/oauth-protected-resource"
	apiAuthMiddleware.SetOAuth(
		func(ctx context.Context, rawToken string) (interface{}, error) {
			_, err := oauthService.ValidateAccessToken(ctx, rawToken)
			if err != nil {
				return nil, err
			}
			// OAuth tokens go through the system API key path — no specific user context
			// (the system key will be resolved on the subsequent internal request)
			return nil, nil
		},
		systemAPIKey,
		resourceMetadataURL,
	)
	oauthHandler := oauth.NewHandler(oauthService, authManager, cfg.BaseURL, cfg.SessionSecret)

	// Allow browser-based admin UI calls to /api/v1/ using session cookies (no Bearer token required).
	// This enables admin pages to call API endpoints (e.g., post comments, approve requests)
	// without needing to embed an API key in the page.
	// A session-authenticated request that changes state must carry the CSRF
	// token the /cm pages are rendered with (X-CSRF-Token); API keys and OAuth
	// tokens are unaffected.
	apiAuthMiddleware.SetSessionAuth(func(r *http.Request) interface{} {
		user, ok := authManager.GetCurrentUser(r)
		if !ok {
			return nil
		}
		return user
	}, middleware.CSRFProtectAPI(csrfKey, cfg.SecureCookies))

	apiv1 := r.PathPrefix("/api/v1").Subrouter()
	apiv1.Use(apiAuthMiddleware.Middleware)
	apiv1.Use(middleware.APIBurstRateLimit) // per-token burst limit (20 req/s)
	apiv1.Use(middleware.APIRateLimit)      // per-token sliding-window rate limit (300 req/min)
	apiv1.Use(middleware.APIBodySizeLimit)  // cap request body at 10 MiB to prevent memory exhaustion
	// Stamp change provenance (and version attribution) on every API request:
	// agent sessions are identified by the X-Agent-Session header.
	apiv1.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			prov := services.Provenance{Actor: "human", Via: "api"}
			if sess := r.Header.Get("X-Agent-Session"); sess != "" {
				prov.Actor = "agent"
				prov.AgentSession = sess
			}
			ctx = services.WithProvenance(ctx, prov)
			if u, ok := auth.UserFromAPIContext(ctx); ok && u != nil {
				ctx = services.WithEditorEmail(ctx, u.Email)
			}
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	})

	// Content
	apiv1.HandleFunc("/content", apiHandler.APIListContent).Methods("GET")
	apiv1.HandleFunc("/content", apiHandler.APICreateContent).Methods("POST")
	apiv1.HandleFunc("/content/by-path", apiHandler.APIGetContentByPath).Methods("GET")
	apiv1.HandleFunc("/content/by-path", apiHandler.APIUpdateContentByPath).Methods("PUT")
	apiv1.HandleFunc("/content/backlinks", apiHandler.APIGetBacklinks).Methods("GET")
	apiv1.HandleFunc("/content/batch-publish", apiHandler.APIBatchPublishContent).Methods("POST")
	apiv1.Handle("/content/bulk-create", middleware.BulkUpdateLimiter()(http.HandlerFunc(apiHandler.APIBulkCreateContent))).Methods("POST")
	apiv1.Handle("/content/bulk-update", middleware.BulkUpdateLimiter()(http.HandlerFunc(apiHandler.APIBulkUpdateContent))).Methods("POST")
	apiv1.Handle("/content/bulk-field-op", middleware.BulkUpdateLimiter()(http.HandlerFunc(apiHandler.APIBulkFieldOperation))).Methods("POST")
	apiv1.Handle("/content/export", middleware.ExportLimiter()(http.HandlerFunc(apiHandler.APIExportContent))).Methods("POST")
	apiv1.HandleFunc("/content/{id}", apiHandler.APIGetContent).Methods("GET")
	apiv1.HandleFunc("/content/{id}", apiHandler.APIUpdateContent).Methods("PUT")
	apiv1.HandleFunc("/content/{id}", apiHandler.APIDeleteContent).Methods("DELETE")
	apiv1.HandleFunc("/content/{id}/restore", apiHandler.APIRestoreContent).Methods("POST")
	apiv1.HandleFunc("/content/{id}/publish", apiHandler.APIPublishContent).Methods("POST")
	apiv1.HandleFunc("/content/{id}/unpublish", apiHandler.APIUnpublishContent).Methods("POST")
	apiv1.HandleFunc("/content/{id}/preview", apiHandler.APIPreviewContent).Methods("GET", "POST")
	apiv1.HandleFunc("/content/{id}/versions", apiHandler.APIListContentVersions).Methods("GET")
	apiv1.HandleFunc("/content/{id}/versions/{version}", apiHandler.APIGetContentVersion).Methods("GET")
	apiv1.HandleFunc("/content/{id}/versions/{version}/revert", apiHandler.APIRevertContentVersion).Methods("POST")

	// Templates
	apiv1.HandleFunc("/templates", apiHandler.APIListTemplates).Methods("GET")
	apiv1.HandleFunc("/templates", apiHandler.APICreateTemplate).Methods("POST")
	apiv1.HandleFunc("/templates/{id}", apiHandler.APIGetTemplate).Methods("GET")
	apiv1.HandleFunc("/templates/{id}", apiHandler.APIUpdateTemplate).Methods("PUT")
	apiv1.HandleFunc("/templates/{id}", apiHandler.APIDeleteTemplate).Methods("DELETE")

	// Assets
	apiv1.HandleFunc("/assets", apiHandler.APIListAssets).Methods("GET")
	apiv1.HandleFunc("/assets", apiHandler.APIUploadAsset).Methods("POST")
	apiv1.Handle("/assets/from-url", middleware.AssetFromURLLimiter()(http.HandlerFunc(apiHandler.APIUploadAssetFromURL))).Methods("POST")
	apiv1.HandleFunc("/assets/folders", apiHandler.APIListAssetFolders).Methods("GET")
	apiv1.HandleFunc("/assets/by-path", apiHandler.APIGetAssetByPath).Methods("GET")
	apiv1.HandleFunc("/assets/{id}", apiHandler.APIGetAsset).Methods("GET")
	apiv1.HandleFunc("/assets/{id}", apiHandler.APIDeleteAsset).Methods("DELETE")

	// Theme
	apiv1.HandleFunc("/theme", apiHandler.APIGetTheme).Methods("GET")
	apiv1.HandleFunc("/theme", apiHandler.APIUpdateTheme).Methods("PUT")
	apiv1.HandleFunc("/theme/versions", apiHandler.APIListThemeVersions).Methods("GET")
	apiv1.HandleFunc("/theme/versions/{version}", apiHandler.APIGetThemeVersion).Methods("GET")
	apiv1.HandleFunc("/theme/versions/{version}/revert", apiHandler.APIRevertThemeVersion).Methods("POST")
	apiv1.HandleFunc("/theme/versions/{version}/pin", apiHandler.APIPinThemeVersion).Methods("POST")
	apiv1.HandleFunc("/theme/versions/{version}/unpin", apiHandler.APIPinThemeVersion).Methods("POST")

	// Site Config
	apiv1.HandleFunc("/config", apiHandler.APIGetSiteConfig).Methods("GET")
	apiv1.HandleFunc("/config", apiHandler.APIUpdateSiteConfig).Methods("PUT")

	// Redirects
	apiv1.HandleFunc("/redirects", apiHandler.APIListRedirects).Methods("GET")
	apiv1.HandleFunc("/redirects", apiHandler.APICreateRedirect).Methods("POST")
	apiv1.HandleFunc("/redirects/{id}", apiHandler.APIGetRedirect).Methods("GET")
	apiv1.HandleFunc("/redirects/{id}", apiHandler.APIUpdateRedirect).Methods("PUT")
	apiv1.HandleFunc("/redirects/{id}", apiHandler.APIDeleteRedirect).Methods("DELETE")

	// Folders
	apiv1.HandleFunc("/folders", apiHandler.APIListFolders).Methods("GET")
	apiv1.HandleFunc("/folders", apiHandler.APICreateFolder).Methods("POST")
	apiv1.HandleFunc("/folders/{id}", apiHandler.APIGetFolder).Methods("GET")
	apiv1.HandleFunc("/folders/{id}", apiHandler.APIDeleteFolder).Methods("DELETE")

	// Collections
	apiv1.HandleFunc("/collections", apiHandler.APIListCollections).Methods("GET")
	apiv1.HandleFunc("/collections", apiHandler.APICreateCollection).Methods("POST")
	apiv1.HandleFunc("/collections/{id}", apiHandler.APIGetCollection).Methods("GET")
	apiv1.HandleFunc("/collections/{id}", apiHandler.APIUpdateCollection).Methods("PUT")
	apiv1.HandleFunc("/collections/{id}", apiHandler.APIDeleteCollection).Methods("DELETE")

	// Search
	apiv1.HandleFunc("/search", apiHandler.APISearchContent).Methods("GET")
	apiv1.HandleFunc("/search-replace/preview", apiHandler.APISearchReplacePreview).Methods("POST")
	apiv1.Handle("/search-replace/execute", middleware.SearchReplaceExecuteLimiter()(http.HandlerFunc(apiHandler.APISearchReplaceExecute))).Methods("POST")
	apiv1.HandleFunc("/search-replace/scoped/preview", apiHandler.APIScopedSearchReplacePreview).Methods("POST")
	apiv1.Handle("/search-replace/scoped/execute", middleware.SearchReplaceExecuteLimiter()(http.HandlerFunc(apiHandler.APIScopedSearchReplaceExecute))).Methods("POST")

	// End-user search (authenticated API)
	apiv1.HandleFunc("/end-user-search", apiHandler.APIEndUserSearch).Methods("GET")
	apiv1.HandleFunc("/end-user-search/suggest", apiHandler.APIEndUserSearchSuggest).Methods("GET")
	apiv1.Handle("/reindex-embeddings", middleware.ReindexLimiter()(http.HandlerFunc(apiHandler.APIReindexEmbeddings))).Methods("POST")

	// API Keys
	apiv1.HandleFunc("/api-keys", apiHandler.APIListAPIKeys).Methods("GET")
	apiv1.HandleFunc("/api-keys", apiHandler.APICreateAPIKey).Methods("POST")
	apiv1.HandleFunc("/api-keys/{id}", apiHandler.APIDeleteAPIKey).Methods("DELETE")

	// Snippets
	apiv1.HandleFunc("/snippets", apiHandler.APIListSnippets).Methods("GET")
	apiv1.HandleFunc("/snippets", apiHandler.APICreateSnippet).Methods("POST")
	apiv1.HandleFunc("/snippets/{id}", apiHandler.APIGetSnippet).Methods("GET")
	apiv1.HandleFunc("/snippets/{id}", apiHandler.APIUpdateSnippet).Methods("PUT")
	apiv1.HandleFunc("/snippets/{id}", apiHandler.APIDeleteSnippet).Methods("DELETE")

	// Forks
	apiv1.HandleFunc("/forks", apiHandler.APIListForks).Methods("GET")
	apiv1.HandleFunc("/forks", apiHandler.APICreateFork).Methods("POST")
	apiv1.HandleFunc("/forks/{id}", apiHandler.APIGetFork).Methods("GET")
	apiv1.HandleFunc("/forks/{id}", apiHandler.APIDeleteFork).Methods("DELETE")
	apiv1.HandleFunc("/forks/{id}/fork-page", apiHandler.APIForkPage).Methods("POST")
	apiv1.HandleFunc("/forks/{id}/pages", apiHandler.APIListForkPages).Methods("GET")
	apiv1.HandleFunc("/forks/{id}/pages/{pageID}", apiHandler.APIRemoveForkPage).Methods("DELETE")
	apiv1.HandleFunc("/forks/{id}/diff", apiHandler.APIForkDiff).Methods("GET")

	// Agent session ledger & rollback
	apiv1.HandleFunc("/agent-sessions/{id}/changes", apiHandler.APIAgentSessionChanges).Methods("GET")
	apiv1.HandleFunc("/agent-sessions/{id}/rollback", apiHandler.APIAgentSessionRollback).Methods("POST")

	// Maintenance scans (self-maintaining site routines)
	apiv1.HandleFunc("/maintenance/report", apiHandler.APIMaintenanceReport).Methods("GET")
	apiv1.HandleFunc("/indexnow", apiHandler.APIIndexNowStatus).Methods("GET")
	apiv1.HandleFunc("/seo", apiHandler.APIGetSEO).Methods("GET")
	apiv1.HandleFunc("/seo", apiHandler.APIUpdateSEO).Methods("PUT")
	apiv1.HandleFunc("/analytics/ai", apiHandler.APIAITraffic).Methods("GET")
	apiv1.HandleFunc("/indexnow", apiHandler.APIIndexNowUpdate).Methods("PUT")
	apiv1.HandleFunc("/indexnow/submit", apiHandler.APIIndexNowSubmit).Methods("POST")
	apiv1.HandleFunc("/maintenance/scan", apiHandler.APIMaintenanceScan).Methods("POST")
	apiv1.HandleFunc("/maintenance/backfill-published-dates", apiHandler.APIBackfillPublishedDates).Methods("POST")
	apiv1.HandleFunc("/maintenance/repair-fork-damage", apiHandler.APIRepairForkDamage).Methods("POST")
	apiv1.HandleFunc("/forks/{id}/merge", apiHandler.APIMergeFork).Methods("POST")
	apiv1.HandleFunc("/forks/{id}/archive", apiHandler.APIArchiveFork).Methods("POST")
	apiv1.HandleFunc("/forks/{id}/purge-copies", apiHandler.APIPurgeForkCopies).Methods("POST")

	// Regenerate (rate-limited: 2/min — full rebuild is expensive)
	apiv1.Handle("/regenerate", middleware.RegenerateLimiter()(http.HandlerFunc(apiHandler.APIRegenerateAllContent))).Methods("POST")

	// Webhooks API
	apiv1.HandleFunc("/webhooks", apiHandler.APIListWebhooks).Methods("GET")
	apiv1.HandleFunc("/webhooks", apiHandler.APICreateWebhook).Methods("POST")
	apiv1.HandleFunc("/webhooks/{id}", apiHandler.APIUpdateWebhook).Methods("PUT")
	apiv1.HandleFunc("/webhooks/{id}", apiHandler.APIDeleteWebhook).Methods("DELETE")
	apiv1.HandleFunc("/webhooks/{id}/regenerate-secret", apiHandler.APIRegenerateWebhookSecret).Methods("POST")
	apiv1.HandleFunc("/webhooks/{id}/deliveries", apiHandler.APIListWebhookDeliveries).Methods("GET")

	// Content locks API
	apiv1.HandleFunc("/content/{id}/lock", apiHandler.APIGetContentLock).Methods("GET")
	apiv1.HandleFunc("/content/{id}/lock", apiHandler.APIAcquireContentLock).Methods("POST")
	apiv1.HandleFunc("/content/{id}/lock", apiHandler.APIReleaseContentLock).Methods("DELETE")
	apiv1.HandleFunc("/content/{id}/lock/force", apiHandler.APIForceUnlockContent).Methods("POST")

	// Scheduled publish API
	apiv1.HandleFunc("/content/scheduled", apiHandler.APIListScheduledContent).Methods("GET")
	apiv1.HandleFunc("/content/{id}/schedule", apiHandler.APIScheduleContentPublish).Methods("POST")

	// Audit log API
	apiv1.HandleFunc("/audit", apiHandler.APIListAuditLogs).Methods("GET")

	// Link check API
	apiv1.HandleFunc("/link-check", apiHandler.APIStartLinkCheck).Methods("POST")
	apiv1.HandleFunc("/link-check/{id}", apiHandler.APIGetLinkCheckJob).Methods("GET")

	// Import pipeline
	apiv1.HandleFunc("/imports/sources", apiHandler.APIListImportSources).Methods("GET")
	apiv1.HandleFunc("/imports/sources", apiHandler.APICreateImportSource).Methods("POST")
	apiv1.HandleFunc("/imports/sources/{id}", apiHandler.APIUpdateImportSource).Methods("PUT")
	apiv1.HandleFunc("/imports/sources/{id}", apiHandler.APIDeleteImportSource).Methods("DELETE")
	apiv1.HandleFunc("/imports/sources/{id}/trigger", apiHandler.APITriggerImportSource).Methods("POST")
	apiv1.HandleFunc("/imports/markdown", apiHandler.APIImportMarkdown).Methods("POST")
	apiv1.HandleFunc("/imports/csv", apiHandler.APIImportCSV).Methods("POST")
	apiv1.HandleFunc("/imports/jobs", apiHandler.APIListImportJobs).Methods("GET")
	apiv1.HandleFunc("/imports/jobs/{id}", apiHandler.APIGetImportJob).Methods("GET")
	apiv1.HandleFunc("/imports/jobs/{id}/cancel", apiHandler.APICancelImportJob).Methods("POST")

	// Comments API
	apiv1.HandleFunc("/content/{id}/comments", apiHandler.APIListComments).Methods("GET")
	apiv1.Handle("/content/{id}/comments", middleware.CommentCreateLimiter()(http.HandlerFunc(apiHandler.APICreateComment))).Methods("POST")
	apiv1.HandleFunc("/content/{id}/comments/{cid}", apiHandler.APIDeleteComment).Methods("DELETE")
	apiv1.HandleFunc("/content/{id}/submit-approval", apiHandler.APISubmitForApproval).Methods("POST")

	// Users list (for @mention and approver search in UI)
	apiv1.HandleFunc("/users", apiHandler.APIListUsers).Methods("GET")

	// Approval workflows API
	apiv1.HandleFunc("/approval-workflows", apiHandler.APIListApprovalWorkflows).Methods("GET")
	apiv1.HandleFunc("/approval-workflows", apiHandler.APICreateApprovalWorkflow).Methods("POST")
	apiv1.HandleFunc("/approval-workflows/{id}", apiHandler.APIGetApprovalWorkflow).Methods("GET")
	apiv1.HandleFunc("/approval-workflows/{id}", apiHandler.APIUpdateApprovalWorkflow).Methods("PUT")
	apiv1.HandleFunc("/approval-workflows/{id}", apiHandler.APIDeleteApprovalWorkflow).Methods("DELETE")

	// Approval requests API
	apiv1.HandleFunc("/approval-requests", apiHandler.APIListApprovalRequests).Methods("GET")
	apiv1.HandleFunc("/approval-requests/{id}", apiHandler.APIGetApprovalRequest).Methods("GET")
	apiv1.HandleFunc("/approval-requests/{id}/approve", apiHandler.APIApproveRequest).Methods("POST")
	apiv1.HandleFunc("/approval-requests/{id}/reject", apiHandler.APIRejectRequest).Methods("POST")
	apiv1.HandleFunc("/approval-requests/{id}/cancel", apiHandler.APICancelRequest).Methods("POST")

	// API routes for AJAX (admin panel internal use)
	// Note: Most API routes require authentication (checked in handlers)
	// The /api/contact route is public for contact form submissions
	// This subrouter has NO CSRF middleware: an admin-session route that
	// changes state belongs under /cm (see the search-and-replace and
	// broken-link fix routes there), never here.
	api := r.PathPrefix("/api").Subrouter()
	api.HandleFunc("/template/{id}/fields", h.GetTemplateFields).Methods("GET")            // Auth checked in handler
	api.HandleFunc("/slugs", h.GetAllSlugs).Methods("GET")                                 // Auth checked in handler
	api.HandleFunc("/folders", h.GetAllFoldersAPI).Methods("GET")                          // Auth checked in handler
	api.HandleFunc("/contact", h.ContactFormSubmitWithConfig(proxyConfig)).Methods("POST") // Public, uses trusted proxy config
	api.HandleFunc("/content/search", h.SearchContent).Methods("GET")                      // Auth checked in handler
	api.HandleFunc("/content/check-slug", h.CheckSlug).Methods("GET")                      // Auth checked in handler
	api.HandleFunc("/tools/broken-links/scan", h.BrokenLinkScan).Methods("GET")            // Auth checked in handler
	api.HandleFunc("/search", h.EndUserSearch).Methods("GET")                              // Public end-user search
	api.HandleFunc("/search/suggest", h.EndUserSearchSuggest).Methods("GET")               // Public typeahead suggestions
	api.HandleFunc("/chat", h.ChatWidgetQuery).Methods("GET", "OPTIONS")                   // Public chat widget query
	api.HandleFunc("/chat/config", h.ChatWidgetConfigPublic).Methods("GET")                // Public chat widget config (for JS widget)

	// OAuth 2.1 endpoints (no auth middleware — these implement their own auth)
	r.HandleFunc("/oauth/register", oauthHandler.Register).Methods("POST")
	r.HandleFunc("/oauth/authorize", oauthHandler.Authorize).Methods("GET")
	r.HandleFunc("/oauth/authorize", oauthHandler.AuthorizeSubmit).Methods("POST")
	r.HandleFunc("/oauth/token", oauthHandler.Token).Methods("POST")
	r.HandleFunc("/oauth/revoke", oauthHandler.TokenRevocation).Methods("POST")

	// MCP Streamable HTTP endpoint (API key + OAuth authenticated)
	mcpHandler := lightmcp.NewHTTPHandler(cfg.Port)
	mcpSubrouter := r.PathPrefix("/mcp").Subrouter()
	mcpSubrouter.Use(apiAuthMiddleware.Middleware)
	mcpSubrouter.Handle("", mcpHandler)

	// Public read-only MCP endpoint — no auth; lets visitors' agents search
	// and read the published site ("MCP for readers")
	publicMCP := lightmcp.NewPublicServer(db, searchService, cfg.BaseURL)
	r.Handle("/mcp-public", publicMCP.Handler())

	// Public asset serving
	r.PathPrefix("/assets/").HandlerFunc(h.ServeAsset).Methods("GET")

	// Health check for Fly.io — fails if the server is resource-exhausted
	// so Fly restarts the machine instead of routing traffic to a stuck instance.
	r.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		if n := runtime.NumGoroutine(); n > 10000 {
			log.Printf("[health] FAIL: goroutine count %d exceeds threshold", n)
			http.Error(w, "unhealthy: goroutine leak", http.StatusServiceUnavailable)
			return
		}
		// Quick DB ping to confirm we can actually serve requests.
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()
		if err := db.Collection("content").Database().Client().Ping(ctx, nil); err != nil {
			log.Printf("[health] FAIL: db ping: %v", err)
			http.Error(w, "unhealthy: db unreachable", http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	}).Methods("GET")

	// Structured healthz endpoint (vibectl VibeCtl Health Check Protocol)
	r.HandleFunc("/healthz", h.Healthz).Methods("GET")

	// OAuth well-known metadata endpoints (RFC 9728 + RFC 8414)
	r.HandleFunc("/.well-known/oauth-protected-resource", oauth.ProtectedResourceMetadataHandler(cfg.BaseURL)).Methods("GET", "OPTIONS")
	r.HandleFunc("/.well-known/oauth-authorization-server", oauth.AuthorizationServerMetadataHandler(cfg.BaseURL)).Methods("GET", "OPTIONS")
	r.HandleFunc("/oauth/jwks", oauth.JWKSHandler()).Methods("GET")

	// MCP well-known server card (dynamic, includes full tool schemas)
	serverCardJSON, err := lightmcp.ServerCard(cfg.BaseURL)
	if err != nil {
		log.Printf("Warning: could not generate MCP server card: %v", err)
	}
	r.HandleFunc("/.well-known/mcp/server-card.json", func(w http.ResponseWriter, rr *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if serverCardJSON != nil {
			w.Write(serverCardJSON)
		} else {
			http.ServeFile(w, rr, ".well-known/mcp/server-card.json")
		}
	}).Methods("GET")

	// Sitemap and robots.txt
	r.HandleFunc("/sitemap.xml", h.ServeSitemap).Methods("GET")
	r.HandleFunc("/robots.txt", h.ServeRobotsTxt).Methods("GET")

	// llms.txt for AI crawlers/agents (llmstxt.org proposal)
	r.HandleFunc("/llms.txt", h.ServeLlmsTxt).Methods("GET")
	r.HandleFunc("/llms-full.txt", h.ServeLlmsFullTxt).Methods("GET")

	// Public content routes - must be last
	// Feeds (RSS / Atom); unknown collections fall through to page serving
	r.HandleFunc("/feed.xml", h.ServeRSSFeed).Methods("GET")
	r.HandleFunc("/atom.xml", h.ServeAtomFeed).Methods("GET")
	r.HandleFunc("/{collection}/feed.xml", h.ServeRSSFeed).Methods("GET")

	// IndexNow key file (/{key}.txt); non-matching *.txt paths fall through to ServePage
	r.HandleFunc("/{indexnowkey:[A-Za-z0-9-]{8,128}}.txt", h.ServeIndexNowKey).Methods("GET")

	r.HandleFunc("/", h.ServePage).Methods("GET")
	r.HandleFunc("/{slug:.*}", h.ServePage).Methods("GET")

	// Create server
	// Note: WriteTimeout is set high (5 min) to support SSE streaming endpoints like broken link scanner
	// which can take several minutes to complete scanning all pages
	srv := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      r,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 5 * time.Minute,
		IdleTimeout:  5 * time.Minute,
	}

	// Start server in goroutine
	go func() {
		log.Printf("LightCMS starting on http://localhost:%s", cfg.Port)
		log.Printf("Admin panel available at http://localhost:%s/cm", cfg.Port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server error: %v", err)
		}
	}()

	// Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("Shutting down server...")

	ctx, cancel = context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalf("Server forced to shutdown: %v", err)
	}

	log.Println("Server stopped")
}

// checkVersionMigration checks if the software version has changed and performs migration tasks
func checkVersionMigration(db *database.DB) error {
	ctx := context.Background()

	// Get current software version from build config
	softwareVersion := build.GetVersion()

	// Get database version
	dbVersion, err := db.GetDatabaseVersion(ctx)
	if err != nil {
		return fmt.Errorf("failed to get database version: %w", err)
	}

	// Check if we need to upgrade
	if dbVersion == softwareVersion {
		return nil // Already up to date
	}

	log.Printf("Version change detected: database=%q, software=%q", dbVersion, softwareVersion)

	// Update database version
	if err := db.SetDatabaseVersion(ctx, softwareVersion); err != nil {
		return fmt.Errorf("failed to set database version: %w", err)
	}

	// Create welcome message
	welcomeMsg := models.ContactMessage{
		Name:      "LightCMS",
		Email:     "",
		Subject:   fmt.Sprintf("Welcome to LightCMS v%s", softwareVersion),
		Message:   fmt.Sprintf(`Welcome to LightCMS v%s! This is a <a href="https://metavert.io">Metavert</a> project. I hope you enjoy using it to maintain your website. --Jon`, softwareVersion),
		IsSystem:  true,
		Read:      false,
		CreatedAt: time.Now(),
	}

	if _, err := db.InsertOne(ctx, "contact_messages", welcomeMsg); err != nil {
		return fmt.Errorf("failed to create welcome message: %w", err)
	}

	log.Printf("Database upgraded to version %s, welcome message created", softwareVersion)
	return nil
}
