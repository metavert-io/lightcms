package handlers

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/metavert-io/lightcms/v7/internal/auth"
	"github.com/metavert-io/lightcms/v7/internal/build"
	"github.com/metavert-io/lightcms/v7/internal/database"
	"github.com/metavert-io/lightcms/v7/internal/errors"
	"github.com/metavert-io/lightcms/v7/internal/middleware"
	"github.com/metavert-io/lightcms/v7/internal/models"
	"github.com/metavert-io/lightcms/v7/internal/services"

	"github.com/gorilla/csrf"
	"github.com/gorilla/mux"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo/options"
	"golang.org/x/text/unicode/norm"
)

// adminTemplateFuncMap is built once and shared across all cached templates.
var adminTemplateFuncMap = template.FuncMap{
	"split":    func(s, sep string) []string { return strings.Split(s, sep) },
	"join":     func(items []string, sep string) string { return strings.Join(items, sep) },
	"multiply": func(a, b int) int { return a * b },
	"divide": func(a, b int64) int64 {
		if b == 0 {
			return 0
		}
		return a / b
	},
	"safeHTML":    func(s string) template.HTML { return template.HTML(s) },
	"subtract":    func(a, b interface{}) int64 { return templateToInt64(a) - templateToInt64(b) },
	"add":         func(a, b interface{}) int64 { return templateToInt64(a) + templateToInt64(b) },
	"formatBytes": func(n interface{}) string { return formatBytes(templateToInt64(n)) },
}

// adminTemplateCache holds pre-compiled admin templates. Built once on first use.
var (
	adminTemplateCache     map[string]*template.Template
	adminTemplateCacheOnce sync.Once
)

func initAdminTemplateCache() {
	adminTemplateCacheOnce.Do(func() {
		adminTemplateCache = make(map[string]*template.Template, len(adminTemplates))
		for name, src := range adminTemplates {
			adminTemplateCache[name] = template.Must(
				template.New("admin").Funcs(adminTemplateFuncMap).Parse(src),
			)
		}
	})
}

type Handler struct {
	db               *database.DB
	auth             *auth.Manager
	baseURL          string
	env              string
	errors           *errors.Handler
	apiKeyService    *services.APIKeyService
	searchService    *services.SearchService
	userService      *services.UserService
	auditService     *services.AuditService
	snippetService   *services.SnippetService
	contentService   *services.ContentService
	analyticsService *services.AnalyticsService
	forkService      *services.ForkService
	webhookService   *services.WebhookService
	importService    *services.ImportService
	cfService        *services.CloudflareService
	proxyConfig      *middleware.TrustedProxyConfig
	anthropicAPIKey  string
	// anthropicURLOverride redirects copilot API calls (tests only).
	anthropicURLOverride string
	commentService       *services.CommentService
	approvalService      *services.ApprovalService
	maintenanceService   *services.MaintenanceService
	agentService         *services.AgentService
	indexNowService      *services.IndexNowService
	seoService           *services.SEOService
	oauthService         *services.OAuthService
}

// SetMaintenanceService wires the maintenance scan service (used by the copilot).
func (h *Handler) SetMaintenanceService(m *services.MaintenanceService) {
	h.maintenanceService = m
}

// SetCloudflareService wires the Cloudflare service into the handler (used for health checks on the config page).
func (h *Handler) SetCloudflareService(cf *services.CloudflareService) {
	h.cfService = cf
}

// SetSearchService sets the search service for end-user search features
func (h *Handler) SetSearchService(ss *services.SearchService) {
	h.searchService = ss
}

// SetAnthropicAPIKey sets the Anthropic API key for chat widget answer synthesis
func (h *Handler) SetAnthropicAPIKey(key string) {
	h.anthropicAPIKey = key
}

// SetProxyConfig sets the trusted proxy configuration used for client IP extraction
func (h *Handler) SetProxyConfig(pc *middleware.TrustedProxyConfig) {
	h.proxyConfig = pc
}

// SetContentService sets the content service for lc:query index regeneration
func (h *Handler) SetContentService(cs *services.ContentService) {
	h.contentService = cs
}

// SetWebhookService sets the webhook service
func (h *Handler) SetWebhookService(ws *services.WebhookService) {
	h.webhookService = ws
}

// SetCommentService sets the comment service
func (h *Handler) SetCommentService(cs *services.CommentService) {
	h.commentService = cs
}

// SetApprovalService sets the approval service
func (h *Handler) SetApprovalService(as *services.ApprovalService) {
	h.approvalService = as
}

func New(db *database.DB, authManager *auth.Manager, baseURL string, env string, userService *services.UserService, auditService *services.AuditService, snippetService *services.SnippetService) *Handler {
	isDev := env == "development" || env == "dev"
	return &Handler{
		db:             db,
		auth:           authManager,
		baseURL:        baseURL,
		env:            env,
		errors:         errors.NewHandler(isDev),
		apiKeyService:  services.NewAPIKeyService(db),
		userService:    userService,
		auditService:   auditService,
		snippetService: snippetService,
	}
}

// IsDev returns true if running in development mode
func (h *Handler) IsDev() bool {
	return h.env == "development" || h.env == "dev"
}

// uploadMaxBytes returns the configured max upload size in bytes.
// Falls back to 1 MiB if the config is unavailable.
func (h *Handler) uploadMaxBytes(ctx context.Context) int64 {
	if cfg, err := h.db.GetSiteConfig(ctx); err == nil && cfg.MaxUploadBytes > 0 {
		return cfg.MaxUploadBytes
	}
	return 1 << 20 // 1 MiB fallback
}

// formatBytes formats a byte count as a human-readable string (e.g. "1.0 MB").
func formatBytes(n int64) string {
	switch {
	case n >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.1f KB", float64(n)/(1<<10))
	default:
		return fmt.Sprintf("%d B", n)
	}
}

// SeedDefaults creates default templates and hello world page if they don't exist
func (h *Handler) SeedDefaults(ctx context.Context) error {
	// Seed default templates
	for _, tmpl := range models.DefaultTemplates {
		count, err := h.db.Count(ctx, "templates", bson.M{"slug": tmpl.Slug})
		if err != nil {
			return err
		}
		if count == 0 {
			tmpl.CreatedAt = time.Now()
			tmpl.UpdatedAt = time.Now()
			if _, err := h.db.InsertOne(ctx, "templates", tmpl); err != nil {
				return err
			}
		}
	}

	// Create Hello World page if no content exists
	count, err := h.db.Count(ctx, "content", bson.M{})
	if err != nil {
		return err
	}
	if count == 0 {
		// Get the explanatory page template
		var tmpl models.Template
		err := h.db.FindOne(ctx, "templates", bson.M{"slug": "explanatory-page"}, &tmpl)
		if err != nil {
			return err
		}

		now := time.Now()
		helloWorld := models.Content{
			TemplateID:   tmpl.ID,
			TemplateName: tmpl.Name,
			Title:        "Welcome to LightCMS",
			Slug:         "",
			Category:     "pages",
			Published:    true,
			PublishedAt:  &now,
			UseHeader:    true,
			UseFooter:    true,
			CreatedAt:    now,
			UpdatedAt:    now,
			Data: map[string]interface{}{
				"title":    "Welcome to LightCMS",
				"subtitle": "A lightweight, modern content management system",
				"intro":    "<p>Your new website is ready to be customized. LightCMS makes it easy to create beautiful, fast websites with a powerful template system.</p>",
				"main_content": `<h2>Getting Started</h2>
<p>Welcome to your new LightCMS installation! Here's how to get started:</p>
<ol>
<li><strong>Access the Admin Panel</strong> - Visit <a href="/cm">/cm</a> to log in and manage your content.</li>
<li><strong>Create Content</strong> - Use templates to create blog posts, press releases, or custom pages.</li>
<li><strong>Customize Your Theme</strong> - Adjust colors, fonts, and styling to match your brand.</li>
<li><strong>Build Collections</strong> - Group your content into browsable collections.</li>
</ol>
<h2>Features</h2>
<ul>
<li>📝 <strong>Template System</strong> - Define reusable content structures</li>
<li>🎨 <strong>Theme Customization</strong> - Modern, sleek design with full control</li>
<li>⚡ <strong>Static Generation</strong> - Fast page loads from pre-rendered HTML</li>
<li>📱 <strong>Responsive Design</strong> - Beautiful on all devices</li>
<li>🔒 <strong>Secure Admin</strong> - Password-protected content management</li>
</ul>`,
				"cta_text": "Go to Admin Panel",
				"cta_link": "/cm",
			},
		}

		if _, err := h.db.InsertOne(ctx, "content", helloWorld); err != nil {
			return err
		}

		// Generate static page
		if err := h.generateStaticPage(ctx, &helloWorld, &tmpl); err != nil {
			return err
		}
	}

	// Create default blog collection
	collCount, err := h.db.Count(ctx, "collections", bson.M{})
	if err != nil {
		return err
	}
	if collCount == 0 {
		blogCollection := models.Collection{
			Name:         "Blog",
			Slug:         "blog",
			Description:  "All blog posts",
			Category:     "blog",
			SortField:    "published_at",
			SortOrder:    "desc",
			ItemsPerPage: 10,
			ItemTemplate: `<article class="collection-item">
	<a href="/{{.slug}}">
		{{if .featured_image}}<img src="{{.featured_image}}" alt="{{.title}}" class="item-image">{{end}}
		<h3>{{.title}}</h3>
		{{if .excerpt}}<p class="excerpt">{{.excerpt}}</p>{{end}}
		<time>{{.published_at}}</time>
	</a>
</article>`,
			PageTemplate: `<div class="collection-page">
	<header class="collection-header">
		<h1>{{.collection_name}}</h1>
		<p>{{.collection_description}}</p>
	</header>
	<div class="collection-items">
		{{.items}}
	</div>
	{{.pagination}}
</div>`,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}
		if _, err := h.db.InsertOne(ctx, "collections", blogCollection); err != nil {
			return err
		}
	}

	// Ensure default pages exist
	h.ensureDefaultPages(ctx)

	// Set default header/footer in theme if not set
	theme, _ := h.db.GetThemeSettings(ctx)
	if theme.FooterHTML == "" {
		theme.FooterHTML = `<p>Created with LightCMS, licensed under <a href="https://creativecommons.org/licenses/by/4.0/" target="_blank">CC BY 4.0</a></p>
<p>&copy; 2026 <a href="https://metavert.io" target="_blank">Metavert LLC</a></p>`
		h.db.SaveThemeSettings(ctx, theme)
	}

	return nil
}

// ensureDefaultPages checks for and creates default pages like 404
func (h *Handler) ensureDefaultPages(ctx context.Context) {
	// Check for 404 page in content
	var existing404 models.Content
	err := h.db.FindOne(ctx, "content", bson.M{"slug": "404"}, &existing404)
	if err != nil {
		// Get the explanatory page template
		var tmpl models.Template
		if err := h.db.FindOne(ctx, "templates", bson.M{"slug": "explanatory-page"}, &tmpl); err != nil {
			return
		}

		now := time.Now()
		page404 := models.Content{
			TemplateID:   tmpl.ID,
			TemplateName: tmpl.Name,
			Title:        "Page Not Found",
			Slug:         "404",
			Category:     "pages",
			Published:    true,
			PublishedAt:  &now,
			UseHeader:    true,
			UseFooter:    true,
			CreatedAt:    now,
			UpdatedAt:    now,
			Data: map[string]interface{}{
				"title":    "404",
				"subtitle": "Page Not Found",
				"intro":    "",
				"main_content": `<div class="error-page">
<p>The page you're looking for doesn't exist or has been moved.</p>
<a href="/" class="cta-button">Go Home</a>
</div>
<style>
.error-page {
	text-align: center;
	padding: 2rem 0;
}
.error-page p {
	color: rgba(241, 245, 249, 0.7);
	margin-bottom: 2rem;
	font-size: 1.2rem;
}
</style>`,
				"cta_text": "",
				"cta_link": "",
			},
		}

		if id, err := h.db.InsertOne(ctx, "content", page404); err == nil {
			page404.ID = id
			h.generateStaticPage(ctx, &page404, &tmpl)
		}
	}
}

// LoginPage renders the login form
func (h *Handler) LoginPage(w http.ResponseWriter, r *http.Request) {
	if h.auth.IsAuthenticated(r) {
		http.Redirect(w, r, "/cm", http.StatusSeeOther)
		return
	}

	ctx := r.Context()
	data := make(map[string]interface{})

	// Check if rate limited
	if locked, duration := h.auth.CheckRateLimit(ctx, r); locked {
		data["Error"] = fmt.Sprintf("Too many login attempts. Please try again in %s.", duration)
		data["RateLimited"] = true
	}

	h.renderAdmin(w, r, "login", data)
}

// LoginHandler processes login
func (h *Handler) LoginHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Check rate limit first
	if locked, duration := h.auth.CheckRateLimit(ctx, r); locked {
		h.renderAdmin(w, r, "login", map[string]interface{}{
			"Error":       fmt.Sprintf("Too many login attempts. Please try again in %s.", duration),
			"RateLimited": true,
		})
		return
	}

	email := strings.TrimSpace(r.FormValue("email"))
	password := r.FormValue("password")

	user, err := h.auth.ValidateCredentials(ctx, email, password)
	if err != nil {
		// Account disabled or other error
		h.renderAdmin(w, r, "login", map[string]interface{}{
			"Error": err.Error(),
			"Email": email,
		})
		return
	}
	if user != nil {
		h.auth.ClearRateLimit(ctx, r)
		if err := h.auth.LoginUser(w, r, user); err != nil {
			http.Error(w, "Login failed", http.StatusInternalServerError)
			return
		}

		// Audit log
		if h.auditService != nil {
			h.auditService.LogAsync(models.AuditLog{
				UserID:    user.ID,
				UserEmail: user.Email,
				Action:    "login.success",
				Resource:  "session",
			})
		}

		// Record activity for DAU/MAU (bounded goroutine with 5s timeout)
		if h.analyticsService != nil {
			userIDHex := user.ID.Hex()
			go func() {
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				h.analyticsService.RecordActivity(ctx, userIDHex)
			}()
		}

		// Force password change if needed
		if user.IsDefaultPassword {
			http.Redirect(w, r, "/cm/change-password", http.StatusSeeOther)
			return
		}
		http.Redirect(w, r, "/cm", http.StatusSeeOther)
		return
	}

	// Record failed attempt
	h.auth.RecordFailedLogin(ctx, r)

	// Audit log for failed login
	if h.auditService != nil {
		h.auditService.LogAsync(models.AuditLog{
			Action:   "login.failure",
			Resource: "session",
			Details:  map[string]interface{}{"email": email},
		})
	}

	// Check if now rate limited after this attempt
	errorMsg := "Invalid email or password"
	rateLimited := false
	if locked, duration := h.auth.CheckRateLimit(ctx, r); locked {
		errorMsg = fmt.Sprintf("Invalid credentials. Too many attempts - locked for %s.", duration)
		rateLimited = true
	}

	h.renderAdmin(w, r, "login", map[string]interface{}{
		"Error":       errorMsg,
		"Email":       email,
		"RateLimited": rateLimited,
	})
}

// LogoutHandler logs out the user
func (h *Handler) LogoutHandler(w http.ResponseWriter, r *http.Request) {
	h.auth.Logout(w, r)
	http.Redirect(w, r, "/cm/login", http.StatusSeeOther)
}

// AdminDashboard shows the main admin page
func (h *Handler) AdminDashboard(w http.ResponseWriter, r *http.Request) {
	if !h.auth.IsAuthenticated(r) {
		http.Redirect(w, r, "/cm/login", http.StatusSeeOther)
		return
	}

	// Check if the user must change their password
	if h.auth.MustChangePassword(r) {
		http.Redirect(w, r, "/cm/change-password", http.StatusSeeOther)
		return
	}

	ctx := r.Context()
	contentCount, _ := h.db.Count(ctx, "content", bson.M{})
	templateCount, _ := h.db.Count(ctx, "templates", bson.M{})
	collectionCount, _ := h.db.Count(ctx, "collections", bson.M{})

	// Get recent content
	cursor, _ := h.db.FindMany(ctx, "content", bson.M{}, options.Find().SetSort(bson.D{{Key: "updated_at", Value: -1}}).SetLimit(5))
	var recentContent []models.Content
	if cursor != nil {
		cursor.All(ctx, &recentContent)
	}

	// Analytics KPIs
	var dau, mau, contentToday int64
	if h.analyticsService != nil {
		dau = h.analyticsService.GetDAU(ctx)
		mau = h.analyticsService.GetMAU(ctx)
		contentToday = h.analyticsService.GetContentCreatedToday(ctx)
	}

	// Recent comments (for dashboard)
	var recentComments []services.ContentCommentWithContent
	if h.commentService != nil {
		recentComments, _ = h.commentService.ListRecent(ctx, 5)
	}

	// Pending approvals (for dashboard, only show when > 0)
	var pendingApprovals []models.ApprovalRequest
	if h.approvalService != nil {
		pendingApprovals, _ = h.approvalService.ListPending(ctx)
	}

	h.renderAdmin(w, r, "dashboard", map[string]interface{}{
		"ContentCount":        contentCount,
		"TemplateCount":       templateCount,
		"CollectionCount":     collectionCount,
		"RecentContent":       recentContent,
		"DAU":                 dau,
		"MAU":                 mau,
		"ContentCreatedToday": contentToday,
		"RecentComments":      recentComments,
		"PendingApprovals":    pendingApprovals,
	})
}

// Template handlers
func (h *Handler) ListTemplates(w http.ResponseWriter, r *http.Request) {
	if !h.auth.IsAuthenticated(r) {
		http.Redirect(w, r, "/cm/login", http.StatusSeeOther)
		return
	}

	ctx := r.Context()
	cursor, err := h.db.FindMany(ctx, "templates", bson.M{}, options.Find().SetSort(bson.D{{Key: "name", Value: 1}}))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	var templates []models.Template
	if err := cursor.All(ctx, &templates); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	h.renderAdmin(w, r, "templates_list", map[string]interface{}{"Templates": templates})
}

func (h *Handler) NewTemplate(w http.ResponseWriter, r *http.Request) {
	if !h.auth.IsAuthenticated(r) {
		http.Redirect(w, r, "/cm/login", http.StatusSeeOther)
		return
	}
	h.renderAdmin(w, r, "template_form", map[string]interface{}{"IsNew": true})
}

func (h *Handler) CreateTemplate(w http.ResponseWriter, r *http.Request) {
	if !h.auth.IsAuthenticated(r) {
		http.Redirect(w, r, "/cm/login", http.StatusSeeOther)
		return
	}

	tmpl := h.parseTemplateForm(r)
	tmpl.CreatedAt = time.Now()
	tmpl.UpdatedAt = time.Now()

	ctx := r.Context()
	newID, err := h.db.InsertOne(ctx, "templates", tmpl)
	if err != nil {
		h.renderAdmin(w, r, "template_form", map[string]interface{}{"IsNew": true, "Error": err.Error(), "Template": tmpl})
		return
	}
	auditResource(r, newID.Hex())

	http.Redirect(w, r, "/cm/templates", http.StatusSeeOther)
}

func (h *Handler) EditTemplate(w http.ResponseWriter, r *http.Request) {
	if !h.auth.IsAuthenticated(r) {
		http.Redirect(w, r, "/cm/login", http.StatusSeeOther)
		return
	}

	vars := mux.Vars(r)
	id, err := primitive.ObjectIDFromHex(vars["id"])
	if err != nil {
		http.Error(w, "Invalid ID", http.StatusBadRequest)
		return
	}

	var tmpl models.Template
	if err := h.db.FindOne(r.Context(), "templates", bson.M{"_id": id}, &tmpl); err != nil {
		http.Error(w, "Template not found", http.StatusNotFound)
		return
	}

	h.renderAdmin(w, r, "template_form", map[string]interface{}{"IsNew": false, "Template": tmpl})
}

func (h *Handler) UpdateTemplate(w http.ResponseWriter, r *http.Request) {
	if !h.auth.IsAuthenticated(r) {
		http.Redirect(w, r, "/cm/login", http.StatusSeeOther)
		return
	}

	vars := mux.Vars(r)
	id, err := primitive.ObjectIDFromHex(vars["id"])
	if err != nil {
		http.Error(w, "Invalid ID", http.StatusBadRequest)
		return
	}

	tmpl := h.parseTemplateForm(r)
	tmpl.UpdatedAt = time.Now()

	ctx := r.Context()
	if err := h.db.UpdateOne(ctx, "templates", bson.M{"_id": id}, bson.M{"$set": tmpl}); err != nil {
		h.renderAdmin(w, r, "template_form", map[string]interface{}{"IsNew": false, "Error": err.Error(), "Template": tmpl})
		return
	}

	// Regenerate all content using this template
	h.regenerateContentByTemplate(ctx, id)

	http.Redirect(w, r, "/cm/templates", http.StatusSeeOther)
}

func (h *Handler) DeleteTemplate(w http.ResponseWriter, r *http.Request) {
	if !h.auth.IsAuthenticated(r) {
		http.Redirect(w, r, "/cm/login", http.StatusSeeOther)
		return
	}

	vars := mux.Vars(r)
	id, err := primitive.ObjectIDFromHex(vars["id"])
	if err != nil {
		http.Error(w, "Invalid ID", http.StatusBadRequest)
		return
	}

	// Check if it's a system template
	var tmpl models.Template
	if err := h.db.FindOne(r.Context(), "templates", bson.M{"_id": id}, &tmpl); err == nil && tmpl.IsSystem {
		http.Error(w, "Cannot delete system templates", http.StatusForbidden)
		return
	}

	if err := h.db.DeleteOne(r.Context(), "templates", bson.M{"_id": id}); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/cm/templates", http.StatusSeeOther)
}

func (h *Handler) parseTemplateForm(r *http.Request) models.Template {
	r.ParseForm()

	var fields []models.TemplateField
	fieldNames := r.Form["field_name[]"]
	fieldLabels := r.Form["field_label[]"]
	fieldTypes := r.Form["field_type[]"]
	fieldRequired := r.Form["field_required[]"]
	fieldPlaceholders := r.Form["field_placeholder[]"]
	fieldOptions := r.Form["field_options[]"]

	for i := range fieldNames {
		if fieldNames[i] == "" {
			continue
		}
		required := false
		if i < len(fieldRequired) && fieldRequired[i] == "on" {
			required = true
		}
		placeholder := ""
		if i < len(fieldPlaceholders) {
			placeholder = fieldPlaceholders[i]
		}
		opts := ""
		if i < len(fieldOptions) {
			opts = fieldOptions[i]
		}
		fields = append(fields, models.TemplateField{
			Name:        fieldNames[i],
			Label:       fieldLabels[i],
			Type:        fieldTypes[i],
			Required:    required,
			Placeholder: placeholder,
			Options:     opts,
		})
	}

	return models.Template{
		Name:        r.FormValue("name"),
		Slug:        slugify(r.FormValue("name")),
		Description: r.FormValue("description"),
		Category:    r.FormValue("category"),
		HTMLLayout:  r.FormValue("html_layout"),
		Fields:      fields,
	}
}

// Content handlers
func (h *Handler) ListContent(w http.ResponseWriter, r *http.Request) {
	if !h.auth.IsAuthenticated(r) {
		http.Redirect(w, r, "/cm/login", http.StatusSeeOther)
		return
	}

	ctx := r.Context()

	// Ensure default pages exist
	h.ensureDefaultPages(ctx)

	// Get filter parameters
	showDeleted := r.URL.Query().Get("deleted") == "true"
	folderFilter := r.URL.Query().Get("folder")

	// Build query filter using $and to combine multiple conditions
	var conditions []bson.M

	// Deleted filter
	if showDeleted {
		conditions = append(conditions, bson.M{"deleted": true})
	} else {
		// Match documents where deleted is false, null, or missing
		// Using $or explicitly for compatibility with documents created before the deleted field existed
		conditions = append(conditions, bson.M{"$or": []bson.M{
			{"deleted": false},
			{"deleted": nil},
			{"deleted": bson.M{"$exists": false}},
		}})
	}

	// Folder filter
	if folderFilter != "" && folderFilter != "all" {
		if folderFilter == "root" {
			// Root level: no folder or empty folder path
			conditions = append(conditions, bson.M{"$or": []bson.M{
				{"folder_path": ""},
				{"folder_path": bson.M{"$exists": false}},
			}})
		} else {
			conditions = append(conditions, bson.M{"folder_path": folderFilter})
		}
	}

	// Always exclude fork copies from the main content list
	conditions = append(conditions, bson.M{"fork_id": nil})

	// Combine all conditions with $and
	filter := bson.M{}
	if len(conditions) == 1 {
		filter = conditions[0]
	} else if len(conditions) > 1 {
		filter["$and"] = conditions
	}

	// Pagination
	const pageSize = 100
	page := 1
	if p, err := strconv.Atoi(r.URL.Query().Get("page")); err == nil && p > 1 {
		page = p
	}
	skip := int64((page - 1) * pageSize)

	total, _ := h.db.Count(ctx, "content", filter)
	totalPages := int((total + pageSize - 1) / pageSize)
	if totalPages < 1 {
		totalPages = 1
	}

	cursor, err := h.db.FindMany(ctx, "content", filter, options.Find().
		SetSort(bson.D{{Key: "updated_at", Value: -1}}).
		SetSkip(skip).
		SetLimit(pageSize))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	var content []models.Content
	if err := cursor.All(ctx, &content); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Pin the homepage to the top of the list (first page only).
	if page == 1 {
		for i, c := range content {
			if c.FullPath == "/" || (c.FullPath == "" && c.Slug == "") {
				content = append([]models.Content{content[i]}, append(content[:i], content[i+1:]...)...)
				break
			}
		}
	}

	// Get all folders for the filter dropdown
	folderCursor, err := h.db.FindMany(ctx, "folders", bson.M{}, options.Find().SetSort(bson.D{{Key: "path", Value: 1}}))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	var folders []models.Folder
	folderCursor.All(ctx, &folders)

	h.renderAdmin(w, r, "content_list", map[string]interface{}{
		"Content":      content,
		"Folders":      folders,
		"ShowDeleted":  showDeleted,
		"FolderFilter": folderFilter,
		"CurrentPage":  page,
		"TotalPages":   totalPages,
		"Total":        total,
	})
}

func (h *Handler) NewContent(w http.ResponseWriter, r *http.Request) {
	if !h.auth.IsAuthenticated(r) {
		http.Redirect(w, r, "/cm/login", http.StatusSeeOther)
		return
	}

	ctx := r.Context()
	cursor, err := h.db.FindMany(ctx, "templates", bson.M{}, options.Find().SetSort(bson.D{{Key: "name", Value: 1}}))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	var templates []models.Template
	cursor.All(ctx, &templates)

	h.renderAdmin(w, r, "content_select_template", map[string]interface{}{"Templates": templates})
}

func (h *Handler) NewContentWithTemplate(w http.ResponseWriter, r *http.Request) {
	if !h.auth.IsAuthenticated(r) {
		http.Redirect(w, r, "/cm/login", http.StatusSeeOther)
		return
	}

	vars := mux.Vars(r)
	templateID, err := primitive.ObjectIDFromHex(vars["templateID"])
	if err != nil {
		http.Error(w, "Invalid template ID", http.StatusBadRequest)
		return
	}

	ctx := r.Context()
	var tmpl models.Template
	if err := h.db.FindOne(ctx, "templates", bson.M{"_id": templateID}, &tmpl); err != nil {
		http.Error(w, "Template not found", http.StatusNotFound)
		return
	}

	folders := h.getAllFolders(ctx)

	h.renderAdmin(w, r, "content_form", map[string]interface{}{
		"IsNew":    true,
		"Template": tmpl,
		"Folders":  folders,
	})
}

func (h *Handler) CreateContent(w http.ResponseWriter, r *http.Request) {
	if !h.auth.IsAuthenticated(r) {
		http.Redirect(w, r, "/cm/login", http.StatusSeeOther)
		return
	}

	r.ParseMultipartForm(32 << 20)

	templateID, err := primitive.ObjectIDFromHex(r.FormValue("template_id"))
	if err != nil {
		http.Error(w, "Invalid template ID", http.StatusBadRequest)
		return
	}

	ctx := r.Context()
	var tmpl models.Template
	if err := h.db.FindOne(ctx, "templates", bson.M{"_id": templateID}, &tmpl); err != nil {
		http.Error(w, "Template not found", http.StatusNotFound)
		return
	}

	// Parse field data
	data := make(map[string]interface{})
	for _, field := range tmpl.Fields {
		if field.Type == "image" {
			// Handle file upload
			file, header, err := r.FormFile("field_" + field.Name)
			if err == nil {
				defer file.Close()
				if uploaded, err := saveUpload(file, "", header.Filename); err == nil {
					data[field.Name] = uploaded
				} else {
					log.Printf("Failed to save upload for field %s: %v", field.Name, err)
				}
			}
		} else {
			data[field.Name] = r.FormValue("field_" + field.Name)
		}
	}

	title := r.FormValue("title")
	slug := r.FormValue("slug")
	if slug == "" {
		slug = slugify(title)
	}

	// Handle folder selection
	folderIDStr := r.FormValue("folder_id")
	var folderID *primitive.ObjectID
	folderPath := ""
	fullPath := "/" + slug
	if slug == "" {
		fullPath = "/"
	}

	if folderIDStr != "" && folderIDStr != "root" {
		fid, err := primitive.ObjectIDFromHex(folderIDStr)
		if err == nil {
			folderID = &fid
			var folder models.Folder
			if err := h.db.FindOne(ctx, "folders", bson.M{"_id": fid}, &folder); err == nil {
				folderPath = folder.Path
				if slug == "" {
					fullPath = folderPath
				} else {
					fullPath = folderPath + "/" + slug
				}
			}
		}
	}

	published := r.FormValue("published") == "on"

	// A held page is always created as a draft: hold wins over the publish
	// checkbox (and over a contributor's submit-for-approval).
	hold := r.FormValue("hold") == "on"
	if hold {
		published = false
	}

	// Contributor intercept: if a contributor attempts to create published content,
	// create it as a draft and auto-submit for approval.
	createContributorApproval := false
	if published {
		if u, ok := h.auth.GetCurrentUser(r); ok && u.Role == models.RoleContributor {
			published = false
			createContributorApproval = true
		}
	}

	var publishedAt *time.Time
	if published {
		now := time.Now()
		publishedAt = &now
	}

	// For blank pages, check if raw mode is enabled
	// Check if checkboxes are checked
	// Note: With hidden+checkbox pattern, we need to check if "on" is in the values
	rawMode := false
	useHeader := false
	useFooter := false
	useTheme := false
	if values, ok := r.Form["raw_mode"]; ok {
		for _, v := range values {
			if v == "on" {
				rawMode = true
				break
			}
		}
	}
	if values, ok := r.Form["use_header"]; ok {
		for _, v := range values {
			if v == "on" {
				useHeader = true
				break
			}
		}
	}
	if values, ok := r.Form["use_footer"]; ok {
		for _, v := range values {
			if v == "on" {
				useFooter = true
				break
			}
		}
	}
	if values, ok := r.Form["use_theme"]; ok {
		for _, v := range values {
			if v == "on" {
				useTheme = true
				break
			}
		}
	}

	// Handle SEO fields
	metaDescription := r.FormValue("meta_description")
	noIndex := r.FormValue("noindex") == "on"
	authorName := strings.TrimSpace(r.FormValue("author_name"))
	authorURL := strings.TrimSpace(r.FormValue("author_url"))
	ogImage := ""
	// Handle OG image upload
	ogFile, ogHeader, err := r.FormFile("og_image")
	if err == nil {
		defer ogFile.Close()
		if uploaded, err := saveUpload(ogFile, "og_", ogHeader.Filename); err == nil {
			ogImage = uploaded
		} else {
			log.Printf("Failed to save OG image upload: %v", err)
		}
	}

	// Parse tags (comma-separated input)
	var contentTags []string
	if tagsStr := r.FormValue("content_tags"); tagsStr != "" {
		for _, t := range strings.Split(tagsStr, ",") {
			if trimmed := strings.TrimSpace(t); trimmed != "" {
				contentTags = append(contentTags, trimmed)
			}
		}
	}

	content := models.Content{
		TemplateID:      templateID,
		TemplateName:    tmpl.Name,
		Title:           title,
		Slug:            slug,
		FolderID:        folderID,
		FolderPath:      folderPath,
		FullPath:        fullPath,
		Category:        tmpl.Category,
		Tags:            contentTags,
		MetaDescription: metaDescription,
		OGImage:         ogImage,
		NoIndex:         noIndex,
		Hold:            hold,
		AuthorName:      authorName,
		AuthorURL:       authorURL,
		Data:            data,
		Published:       published,
		PublishedAt:     publishedAt,
		PendingApproval: createContributorApproval,
		UseHeader:       useHeader,
		UseFooter:       useFooter,
		UseTheme:        useTheme,
		RawMode:         rawMode,
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
	}

	// Extract and track internal links
	content.InternalLinks = h.extractInternalLinksFromContent(&content)

	id, err := h.db.InsertOne(ctx, "content", content)
	if err != nil {
		h.renderAdmin(w, r, "content_form", map[string]interface{}{
			"IsNew":    true,
			"Template": tmpl,
			"Content":  content,
			"Error":    err.Error(),
		})
		return
	}

	content.ID = id
	auditResource(r, id.Hex())

	// Save the initial version (v1)
	if err := h.saveContentVersion(ctx, &content); err != nil {
		fmt.Printf("Warning: Failed to save initial content version: %v\n", err)
	}

	// Generate static page
	if published {
		h.generateStaticPage(ctx, &content, &tmpl)
	}

	// Submit for approval if contributor attempted to publish
	if createContributorApproval && h.approvalService != nil {
		if u, ok := h.auth.GetCurrentUser(r); ok {
			submitterID, _ := primitive.ObjectIDFromHex(u.ID)
			go func() {
				if _, err := h.approvalService.SubmitContentForApproval(context.Background(), &content, submitterID, u.Email); err != nil {
					fmt.Printf("Warning: Failed to submit for approval: %v\n", err)
				}
			}()
		}
	}

	// Regenerate sitemap after content creation
	go h.RegenerateSitemap(context.Background())

	http.Redirect(w, r, "/cm/content", http.StatusSeeOther)
}

func (h *Handler) EditContent(w http.ResponseWriter, r *http.Request) {
	if !h.auth.IsAuthenticated(r) {
		http.Redirect(w, r, "/cm/login", http.StatusSeeOther)
		return
	}

	vars := mux.Vars(r)
	id, err := primitive.ObjectIDFromHex(vars["id"])
	if err != nil {
		http.Error(w, "Invalid ID", http.StatusBadRequest)
		return
	}

	ctx := r.Context()
	var content models.Content
	if err := h.db.FindOne(ctx, "content", bson.M{"_id": id}, &content); err != nil {
		http.Error(w, "Content not found", http.StatusNotFound)
		return
	}

	var tmpl models.Template
	if err := h.db.FindOne(ctx, "templates", bson.M{"_id": content.TemplateID}, &tmpl); err != nil {
		http.Error(w, "Template not found", http.StatusNotFound)
		return
	}

	folders := h.getAllFolders(ctx)

	// Get version history for this content
	versionCursor, err := h.db.FindMany(ctx, "content_versions", bson.M{"content_id": id}, options.Find().SetSort(bson.D{{Key: "version", Value: -1}}).SetLimit(20))
	var versions []models.ContentVersion
	if err == nil {
		versionCursor.All(ctx, &versions)
	}

	// Get same-slug historical pages (different document IDs but same slug, including deleted)
	var sameSlugPages []models.Content
	if content.Slug != "" {
		sameSlugCursor, err := h.db.FindMany(ctx, "content", bson.M{
			"slug": content.Slug,
			"_id":  bson.M{"$ne": id},
		}, options.Find().SetSort(bson.D{{Key: "updated_at", Value: -1}}))
		if err == nil {
			sameSlugCursor.All(ctx, &sameSlugPages)
		}
	}

	// Check for error query param (e.g., path conflict on undelete or slug exists)
	errorMsg := ""
	if r.URL.Query().Get("error") == "path_conflict" {
		errorMsg = "Cannot restore: another page already exists at this URL path. Change the slug first or delete the conflicting page."
	} else if r.URL.Query().Get("error") == "slug_exists" {
		conflictPath := r.URL.Query().Get("path")
		if conflictPath != "" {
			errorMsg = fmt.Sprintf("A page already exists at %q. Please choose a different slug.", conflictPath)
		} else {
			errorMsg = "A page already exists at that URL path. Please choose a different slug."
		}
	} else if r.URL.Query().Get("error") == "held" {
		errorMsg = "This page is on hold and was not saved. Uncheck Hold to publish it, or uncheck Published to save it as a held draft."
	}

	// Get all templates for template change feature
	var allTemplates []models.Template
	templateCursor, err := h.db.FindMany(ctx, "templates", bson.M{}, nil)
	if err == nil {
		templateCursor.All(ctx, &allTemplates)
	}

	// If this is a live page (no fork), provide the ID so the template can offer "Fork to workspace"
	forkPageID := ""
	if content.ForkID == nil {
		forkPageID = content.ID.Hex()
	}

	// Load comments for discussion tab
	var comments []models.ContentComment
	if h.commentService != nil {
		comments, _ = h.commentService.ListForContent(ctx, content.ID)
	}

	// Current user role for permission checks in template
	currentUser, _ := h.auth.GetCurrentUser(r)
	currentUserRole := ""
	currentUserEmail := ""
	if currentUser != nil {
		currentUserRole = currentUser.Role
		currentUserEmail = currentUser.Email
	}

	// Page analytics for the analytics tab (last 30 days)
	pageViews30d := 0
	pageViews7d := 0
	pageReferrersJSON := "[]"
	if h.analyticsService != nil && content.FullPath != "" {
		now := time.Now().UTC()
		pageViews30d = h.analyticsService.GetPageViews(ctx, now.Add(-30*24*time.Hour), now, content.FullPath)
		pageViews7d = h.analyticsService.GetPageViews(ctx, now.Add(-7*24*time.Hour), now, content.FullPath)
		refs, _ := h.analyticsService.GetPageReferrers(ctx, now.Add(-30*24*time.Hour), now, content.FullPath, 10, services.BotFilterHuman)
		if b, err := json.Marshal(refs); err == nil {
			pageReferrersJSON = string(b)
		}
	}

	h.renderAdmin(w, r, "content_form", map[string]interface{}{
		"IsNew":             false,
		"Template":          tmpl,
		"Content":           content,
		"Folders":           folders,
		"Versions":          versions,
		"SameSlugPages":     sameSlugPages,
		"AllTemplates":      allTemplates,
		"Error":             errorMsg,
		"ForkPageID":        forkPageID,
		"Comments":          comments,
		"CurrentUserRole":   currentUserRole,
		"CurrentUserEmail":  currentUserEmail,
		"PageViews30d":      pageViews30d,
		"PageViews7d":       pageViews7d,
		"PageReferrersJSON": pageReferrersJSON,
	})
}

func (h *Handler) UpdateContent(w http.ResponseWriter, r *http.Request) {
	if !h.auth.IsAuthenticated(r) {
		http.Redirect(w, r, "/cm/login", http.StatusSeeOther)
		return
	}

	vars := mux.Vars(r)
	id, err := primitive.ObjectIDFromHex(vars["id"])
	if err != nil {
		http.Error(w, "Invalid ID", http.StatusBadRequest)
		return
	}

	r.ParseMultipartForm(32 << 20)

	ctx := r.Context()
	// Inject editor identity + provenance into context for version history
	if user, ok := h.auth.GetCurrentUser(r); ok {
		ctx = services.WithEditorEmail(ctx, user.Email)
	}
	ctx = services.WithProvenance(ctx, services.Provenance{Actor: "human", Via: "ui"})
	var existingContent models.Content
	if err := h.db.FindOne(ctx, "content", bson.M{"_id": id}, &existingContent); err != nil {
		http.Error(w, "Content not found", http.StatusNotFound)
		return
	}

	// Saving a page needs content.edit, as PUT /api/v1/content/{id} does. A
	// contributor (content.create without content.edit) writes and revises a
	// submission here, so may save a live draft — but not a published page
	// (the save would take it off the site), a fork copy or a deleted page.
	if user, ok := h.auth.GetCurrentUser(r); ok && !auth.HasPermission(user.Role, auth.PermContentEdit) {
		if !auth.HasPermission(user.Role, auth.PermContentCreate) {
			h.refuseAdmin(w, r, false, []string{auth.PermContentEdit})
			return
		}
		if existingContent.Published || existingContent.ForkID != nil || existingContent.Deleted {
			h.refuseAdminMessage(w, r, false, "Your role can edit drafts only. This page is published, deleted or part of a fork, and changing it needs content.edit. Nothing was changed.")
			return
		}
	}

	// Make a copy of the original content before modifications (for versioning)
	originalContent := existingContent
	// Deep copy the Data map
	originalContent.Data = make(map[string]interface{})
	for k, v := range existingContent.Data {
		originalContent.Data[k] = v
	}

	var tmpl models.Template
	if err := h.db.FindOne(ctx, "templates", bson.M{"_id": existingContent.TemplateID}, &tmpl); err != nil {
		http.Error(w, "Template not found", http.StatusNotFound)
		return
	}

	// Parse field data
	data := make(map[string]interface{})
	for _, field := range tmpl.Fields {
		if field.Type == "image" {
			file, header, err := r.FormFile("field_" + field.Name)
			if err == nil {
				defer file.Close()
				if uploaded, err := saveUpload(file, "", header.Filename); err == nil {
					data[field.Name] = uploaded
				} else {
					log.Printf("Failed to save upload for field %s: %v", field.Name, err)
				}
			} else if existingData, ok := existingContent.Data[field.Name]; ok {
				data[field.Name] = existingData
			}
		} else {
			data[field.Name] = r.FormValue("field_" + field.Name)
		}
	}

	title := r.FormValue("title")
	slug := r.FormValue("slug")
	slugRenameEnabled := r.FormValue("slug_rename_enabled") == "yes"

	// For existing content, preserve the existing slug ONLY if rename wasn't explicitly enabled
	// If rename was enabled, accept whatever value was submitted (including empty for homepage)
	if !slugRenameEnabled && slug == "" && existingContent.Slug != "" {
		// Preserve existing slug if form submitted empty and rename wasn't enabled
		slug = existingContent.Slug
	}
	// Otherwise use the submitted slug value (empty is valid for root page "/")

	// Handle folder selection
	folderIDStr := r.FormValue("folder_id")
	var folderID *primitive.ObjectID
	folderPath := ""
	fullPath := "/" + slug
	if slug == "" {
		fullPath = "/"
	}

	if folderIDStr != "" && folderIDStr != "root" {
		fid, err := primitive.ObjectIDFromHex(folderIDStr)
		if err == nil {
			folderID = &fid
			var folder models.Folder
			if err := h.db.FindOne(ctx, "folders", bson.M{"_id": fid}, &folder); err == nil {
				folderPath = folder.Path
				if slug == "" {
					fullPath = folderPath
				} else {
					fullPath = folderPath + "/" + slug
				}
			}
		}
	}

	published := r.FormValue("published") == "on"

	// A fork copy shares its full_path with the live page. It is never
	// published on its own (it reaches the site by merging the fork), and
	// saving it must not touch the live page's static file, redirects, links
	// in other pages or wikilinks.
	isForkCopy := existingContent.ForkID != nil
	if isForkCopy {
		published = false
	}

	// A held draft cannot be published: refuse before anything is written.
	// Holding a page that is already published leaves it published.
	hold := r.FormValue("hold") == "on"
	if hold && published && !existingContent.Published {
		http.Redirect(w, r, fmt.Sprintf("/cm/content/%s?error=held", existingContent.ID.Hex()), http.StatusSeeOther)
		return
	}

	// Contributor intercept: contributors cannot publish directly.
	// If they attempt to publish, save as draft and submit for approval instead.
	contributorSubmittedForApproval := false
	if published {
		if u, ok := h.auth.GetCurrentUser(r); ok && u.Role == models.RoleContributor {
			published = false // keep as draft
			contributorSubmittedForApproval = true
		}
	}

	var publishedAt *time.Time
	if published && existingContent.PublishedAt == nil {
		now := time.Now()
		publishedAt = &now
	} else {
		publishedAt = existingContent.PublishedAt
	}

	// Track old full path for dependency updates and file cleanup
	oldFullPath := existingContent.FullPath
	if oldFullPath == "" {
		// Legacy content without full_path
		oldFullPath = "/" + existingContent.Slug
		if existingContent.Slug == "" {
			oldFullPath = "/"
		}
	}

	// Check for duplicate path if path is changing
	createRedirectFromOld := false
	if oldFullPath != fullPath {
		var existingAtPath models.Content
		err := h.db.FindOne(ctx, "content", bson.M{
			"full_path": fullPath,
			"_id":       bson.M{"$ne": existingContent.ID},
			"deleted":   bson.M{"$ne": true},
			"fork_id":   pathScope(existingContent.ForkID),
		}, &existingAtPath)
		if err == nil {
			// Found existing content at this path - redirect back with error
			http.Redirect(w, r, fmt.Sprintf("/cm/content/%s?error=slug_exists&path=%s", existingContent.ID.Hex(), url.QueryEscape(fullPath)), http.StatusSeeOther)
			return
		}

		// Check if user wants to create a redirect from old path to new
		if r.FormValue("create_redirect") == "yes" {
			createRedirectFromOld = true
		}

		// Delete old static file if path changed (the old path of a fork
		// copy is the live page's path: leave its file alone)
		if !isForkCopy {
			oldStaticPath := h.getStaticFilePath(oldFullPath)
			os.Remove(oldStaticPath)
		}
	}

	// Check if checkboxes are checked
	// Note: With hidden+checkbox pattern, we need to check if "on" is in the values
	rawMode := false
	useHeader := false
	useFooter := false
	useTheme := false
	if values, ok := r.Form["raw_mode"]; ok {
		for _, v := range values {
			if v == "on" {
				rawMode = true
				break
			}
		}
	}
	if values, ok := r.Form["use_header"]; ok {
		for _, v := range values {
			if v == "on" {
				useHeader = true
				break
			}
		}
	}
	if values, ok := r.Form["use_footer"]; ok {
		for _, v := range values {
			if v == "on" {
				useFooter = true
				break
			}
		}
	}
	if values, ok := r.Form["use_theme"]; ok {
		for _, v := range values {
			if v == "on" {
				useTheme = true
				break
			}
		}
	}

	// Handle version comment (optional)
	versionComment := r.FormValue("version_comment")

	// Parse tags (comma-separated input)
	var updatedTags []string
	if tagsStr := r.FormValue("content_tags"); tagsStr != "" {
		for _, t := range strings.Split(tagsStr, ",") {
			if trimmed := strings.TrimSpace(t); trimmed != "" {
				updatedTags = append(updatedTags, trimmed)
			}
		}
	}

	// Handle SEO fields
	metaDescription := r.FormValue("meta_description")
	noIndex := r.FormValue("noindex") == "on"
	authorName := strings.TrimSpace(r.FormValue("author_name"))
	authorURL := strings.TrimSpace(r.FormValue("author_url"))
	ogImage := existingContent.OGImage // Keep existing if not uploading new
	// Handle OG image upload
	ogFile, ogHeader, err := r.FormFile("og_image")
	if err == nil {
		defer ogFile.Close()
		if uploaded, err := saveUpload(ogFile, "og_", ogHeader.Filename); err == nil {
			ogImage = uploaded
		} else {
			log.Printf("Failed to save OG image upload: %v", err)
		}
	}

	// Extract internal links from updated data
	tempContent := &models.Content{Data: data}
	internalLinks := h.extractInternalLinksFromContent(tempContent)

	update := bson.M{
		"$set": bson.M{
			"title":            title,
			"slug":             slug,
			"folder_id":        folderID,
			"folder_path":      folderPath,
			"full_path":        fullPath,
			"tags":             updatedTags,
			"meta_description": metaDescription,
			"og_image":         ogImage,
			"noindex":          noIndex,
			"hold":             hold,
			"author_name":      authorName,
			"author_url":       authorURL,
			"data":             data,
			"published":        published,
			"published_at":     publishedAt,
			"pending_approval": contributorSubmittedForApproval,
			"use_header":       useHeader,
			"use_footer":       useFooter,
			"use_theme":        useTheme,
			"raw_mode":         rawMode,
			"internal_links":   internalLinks,
			"updated_at":       time.Now(),
		},
	}

	if err := h.db.UpdateOne(ctx, "content", bson.M{"_id": id}, update); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Submit for approval if contributor tried to publish
	if contributorSubmittedForApproval && h.approvalService != nil {
		updatedContent := existingContent
		updatedContent.Title = title
		updatedContent.FullPath = fullPath
		updatedContent.FolderPath = folderPath
		updatedContent.Tags = updatedTags
		if u, ok := h.auth.GetCurrentUser(r); ok {
			submitterID, _ := primitive.ObjectIDFromHex(u.ID)
			go func() {
				if _, err := h.approvalService.SubmitContentForApproval(context.Background(), &updatedContent, submitterID, u.Email); err != nil {
					fmt.Printf("Warning: Failed to submit for approval: %v\n", err)
				}
			}()
		}
	}

	// If full path changed, update all dependent content links (not for a
	// fork copy: the live page has not moved)
	if oldFullPath != fullPath && !isForkCopy {
		if err := h.updateDependentContentByPath(ctx, oldFullPath, fullPath); err != nil {
			// Log but don't fail the request
			fmt.Printf("Warning: Failed to update dependent content: %v\n", err)
		}

		// Create redirect from old path to new if user requested
		if createRedirectFromOld {
			redirect := models.Redirect{
				FromPath:    oldFullPath,
				ToPath:      fullPath,
				StatusCode:  301, // Permanent redirect
				Description: fmt.Sprintf("Auto-created when page was renamed from %s", oldFullPath),
				CreatedAt:   time.Now(),
				UpdatedAt:   time.Now(),
			}
			if _, err := h.db.InsertOne(ctx, "redirects", redirect); err != nil {
				fmt.Printf("Warning: Failed to create redirect from %s to %s: %v\n", oldFullPath, fullPath, err)
			}
		}
	}

	// Capture pre-update title for wikilink rename (must be before struct update)
	oldTitleForWikilinks := existingContent.Title

	// Update content struct with new values for static generation and version saving
	existingContent.Title = title
	existingContent.Slug = slug
	existingContent.FolderID = folderID
	existingContent.FolderPath = folderPath
	existingContent.FullPath = fullPath
	existingContent.MetaDescription = metaDescription
	existingContent.OGImage = ogImage
	existingContent.NoIndex = noIndex
	existingContent.Hold = hold
	existingContent.AuthorName = authorName
	existingContent.AuthorURL = authorURL
	existingContent.Data = data
	existingContent.Published = published
	existingContent.PublishedAt = publishedAt
	existingContent.UseTheme = useTheme
	existingContent.RawMode = rawMode
	existingContent.UseHeader = useHeader
	existingContent.UseFooter = useFooter

	// Save this version after the update succeeds
	// Pass the original content so we can save it as v1 if no versions exist yet
	if err := h.saveContentVersionWithOriginal(ctx, &existingContent, &originalContent, versionComment); err != nil {
		fmt.Printf("Warning: Failed to save content version: %v\n", err)
	}

	if isForkCopy {
		// Nothing on the live site changed: no static file, IndexNow,
		// sitemap, index-page or wikilink work.
		http.Redirect(w, r, "/cm/content", http.StatusSeeOther)
		return
	}

	if published {
		h.generateStaticPage(ctx, &existingContent, &tmpl)
	} else {
		// Remove the static file if unpublished — by full path: a slug is
		// only unique inside its folder, so /docs/about must not take
		// /about's file with it.
		h.removeStaticPage(fullPath)
	}
	// This handler writes to the DB directly; tell IndexNow about URL
	// transitions (content edits reach it via the change-stream regen).
	if h.contentService != nil {
		h.contentService.NotifyLiveChange(ctx, &originalContent, &existingContent)
	}

	// Regenerate sitemap after content update
	go h.RegenerateSitemap(context.Background())

	// Regenerate index pages that may reference this content via lc:query
	if h.contentService != nil {
		h.contentService.TriggerIndexRegen()
	}

	// Rewrite [[wikilinks]] across all content if title or path changed
	if h.contentService != nil && (oldTitleForWikilinks != title || oldFullPath != fullPath) {
		go func(oldT, newT, oldP, newP string) {
			wCtx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
			defer cancel()
			h.contentService.UpdateWikilinksOnRename(wCtx, oldT, newT, oldP, newP)
		}(oldTitleForWikilinks, title, oldFullPath, fullPath)
	}

	http.Redirect(w, r, "/cm/content", http.StatusSeeOther)
}

func (h *Handler) DeleteContent(w http.ResponseWriter, r *http.Request) {
	user, ok := h.auth.GetCurrentUser(r)
	if !ok {
		http.Redirect(w, r, "/cm/login", http.StatusSeeOther)
		return
	}

	vars := mux.Vars(r)
	id, err := primitive.ObjectIDFromHex(vars["id"])
	if err != nil {
		http.Error(w, "Invalid ID", http.StatusBadRequest)
		return
	}

	ctx := r.Context()
	var content models.Content
	if err := h.db.FindOne(ctx, "content", bson.M{"_id": id}, &content); err != nil {
		http.Error(w, "Content not found", http.StatusNotFound)
		return
	}

	// A fork copy shares its full_path with the live page: deleting it must
	// never touch the live page's static file, redirects, sitemap or search
	// index. Remove it from its fork instead, exactly as "Remove" on the fork
	// page does (same permission, same service call, same landing page).
	if content.ForkID != nil {
		if !auth.HasPermission(user.Role, auth.PermForkCreate) {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}
		if err := h.forkService.RemovePage(ctx, *content.ForkID, id); err != nil {
			http.Error(w, fmt.Sprintf("Failed to remove page: %v", err), http.StatusInternalServerError)
			return
		}
		http.Redirect(w, r, "/cm/forks/"+content.ForkID.Hex(), http.StatusSeeOther)
		return
	}

	if !auth.HasPermission(user.Role, auth.PermContentDelete) {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	// Delete static file using full path
	h.removeStaticPage(content.FullPath)

	// Remove any redirects that point to this page
	// (no point keeping redirects to a deleted page)
	if content.FullPath != "" {
		deleteResult, err := h.db.DeleteMany(ctx, "redirects", bson.M{"to_path": content.FullPath})
		if err != nil {
			fmt.Printf("Warning: Failed to delete redirects pointing to %s: %v\n", content.FullPath, err)
		} else if deleteResult > 0 {
			fmt.Printf("Deleted %d redirect(s) pointing to deleted page %s\n", deleteResult, content.FullPath)
		}
	}

	// Soft delete: mark as deleted instead of removing from database
	// Use a unique deleted path to avoid unique index conflicts
	now := time.Now()
	deletedPath := fmt.Sprintf("__deleted__/%s/%d", id.Hex(), now.UnixNano())
	update := bson.M{
		"$set": bson.M{
			"deleted":    true,
			"deleted_at": now,
			"published":  false,       // Unpublish when deleting
			"full_path":  deletedPath, // Unique path for deleted items
			"updated_at": now,
		},
	}

	if err := h.db.UpdateOne(ctx, "content", bson.M{"_id": id}, update); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Regenerate sitemap after content deletion
	go h.RegenerateSitemap(context.Background())

	// Regenerate index pages — the deleted item may have appeared in lc:query results
	if h.contentService != nil {
		h.contentService.TriggerIndexRegen()
		h.contentService.NotifyLiveChange(ctx, &content, nil)
	}

	http.Redirect(w, r, "/cm/content", http.StatusSeeOther)
}

func (h *Handler) UndeleteContent(w http.ResponseWriter, r *http.Request) {
	if !h.auth.IsAuthenticated(r) {
		http.Redirect(w, r, "/cm/login", http.StatusSeeOther)
		return
	}

	vars := mux.Vars(r)
	id, err := primitive.ObjectIDFromHex(vars["id"])
	if err != nil {
		http.Error(w, "Invalid ID", http.StatusBadRequest)
		return
	}

	ctx := r.Context()
	var content models.Content
	if err := h.db.FindOne(ctx, "content", bson.M{"_id": id}, &content); err != nil {
		http.Error(w, "Content not found", http.StatusNotFound)
		return
	}

	// Reconstruct full_path
	fullPath := "/" + content.Slug
	if content.Slug == "" {
		fullPath = "/"
	}
	if content.FolderPath != "" {
		if content.Slug == "" {
			fullPath = content.FolderPath
		} else {
			fullPath = content.FolderPath + "/" + content.Slug
		}
	}

	// Check if another content exists at this path
	var existingContent models.Content
	// (among live pages for a live page, within its fork for a fork copy)
	err = h.db.FindOne(ctx, "content", bson.M{
		"full_path": fullPath,
		"deleted":   bson.M{"$ne": true},
		"_id":       bson.M{"$ne": id},
		"fork_id":   pathScope(content.ForkID),
	}, &existingContent)
	if err == nil {
		// Another content exists at this path - redirect with error
		http.Redirect(w, r, "/cm/content/"+id.Hex()+"?error=path_conflict", http.StatusSeeOther)
		return
	}

	// Restore content
	update := bson.M{
		"$set": bson.M{
			"deleted":    false,
			"full_path":  fullPath,
			"updated_at": time.Now(),
		},
		"$unset": bson.M{
			"deleted_at":              "",
			"published_before_delete": "", // an undeleted page comes back as a draft here
		},
	}

	if err := h.db.UpdateOne(ctx, "content", bson.M{"_id": id}, update); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Regenerate index pages — the restored item may now appear in lc:query results
	if h.contentService != nil {
		h.contentService.TriggerIndexRegen()
	}

	http.Redirect(w, r, "/cm/content/"+id.Hex(), http.StatusSeeOther)
}

// FieldMapping represents a field that will be mapped between templates
type FieldMapping struct {
	Name    string
	OldType string
	NewType string
	Value   string
}

// FieldInfo represents a field with its name and type
type FieldInfo struct {
	Name string
	Type string
}

func (h *Handler) ChangeTemplatePreview(w http.ResponseWriter, r *http.Request) {
	if !h.auth.IsAuthenticated(r) {
		http.Redirect(w, r, "/cm/login", http.StatusSeeOther)
		return
	}

	vars := mux.Vars(r)
	contentID, err := primitive.ObjectIDFromHex(vars["id"])
	if err != nil {
		http.Error(w, "Invalid content ID", http.StatusBadRequest)
		return
	}

	newTemplateID, err := primitive.ObjectIDFromHex(vars["template_id"])
	if err != nil {
		http.Error(w, "Invalid template ID", http.StatusBadRequest)
		return
	}

	ctx := r.Context()

	// Get content
	var content models.Content
	if err := h.db.FindOne(ctx, "content", bson.M{"_id": contentID}, &content); err != nil {
		http.Error(w, "Content not found", http.StatusNotFound)
		return
	}

	// Get old template
	var oldTemplate models.Template
	if err := h.db.FindOne(ctx, "templates", bson.M{"_id": content.TemplateID}, &oldTemplate); err != nil {
		http.Error(w, "Old template not found", http.StatusNotFound)
		return
	}

	// Get new template
	var newTemplate models.Template
	if err := h.db.FindOne(ctx, "templates", bson.M{"_id": newTemplateID}, &newTemplate); err != nil {
		http.Error(w, "New template not found", http.StatusNotFound)
		return
	}

	// Build field mappings
	oldFieldMap := make(map[string]models.TemplateField)
	for _, f := range oldTemplate.Fields {
		oldFieldMap[f.Name] = f
	}

	newFieldMap := make(map[string]models.TemplateField)
	for _, f := range newTemplate.Fields {
		newFieldMap[f.Name] = f
	}

	var mappedFields []FieldMapping
	var lostFields []FieldMapping
	var newFields []FieldInfo

	// Check which old fields map to new fields
	for _, oldField := range oldTemplate.Fields {
		value := ""
		if content.Data != nil {
			if v, ok := content.Data[oldField.Name]; ok {
				if s, ok := v.(string); ok {
					value = s
					// Truncate long values for display
					if len(value) > 100 {
						value = value[:100] + "..."
					}
				}
			}
		}

		if newField, exists := newFieldMap[oldField.Name]; exists {
			mappedFields = append(mappedFields, FieldMapping{
				Name:    oldField.Name,
				OldType: oldField.Type,
				NewType: newField.Type,
				Value:   value,
			})
		} else {
			lostFields = append(lostFields, FieldMapping{
				Name:    oldField.Name,
				OldType: oldField.Type,
				Value:   value,
			})
		}
	}

	// Check for new fields that don't exist in old template
	for _, newField := range newTemplate.Fields {
		if _, exists := oldFieldMap[newField.Name]; !exists {
			newFields = append(newFields, FieldInfo{
				Name: newField.Name,
				Type: newField.Type,
			})
		}
	}

	h.renderAdmin(w, r, "change_template_preview", map[string]interface{}{
		"Content":      content,
		"OldTemplate":  oldTemplate,
		"NewTemplate":  newTemplate,
		"MappedFields": mappedFields,
		"LostFields":   lostFields,
		"NewFields":    newFields,
	})
}

func (h *Handler) ConfirmChangeTemplate(w http.ResponseWriter, r *http.Request) {
	if !h.auth.IsAuthenticated(r) {
		http.Redirect(w, r, "/cm/login", http.StatusSeeOther)
		return
	}

	vars := mux.Vars(r)
	contentID, err := primitive.ObjectIDFromHex(vars["id"])
	if err != nil {
		http.Error(w, "Invalid content ID", http.StatusBadRequest)
		return
	}

	newTemplateID, err := primitive.ObjectIDFromHex(vars["template_id"])
	if err != nil {
		http.Error(w, "Invalid template ID", http.StatusBadRequest)
		return
	}

	ctx := r.Context()

	// Get content
	var content models.Content
	if err := h.db.FindOne(ctx, "content", bson.M{"_id": contentID}, &content); err != nil {
		http.Error(w, "Content not found", http.StatusNotFound)
		return
	}

	// Get old template (for version history)
	var oldTemplate models.Template
	if err := h.db.FindOne(ctx, "templates", bson.M{"_id": content.TemplateID}, &oldTemplate); err != nil {
		http.Error(w, "Old template not found", http.StatusNotFound)
		return
	}

	// Get new template
	var newTemplate models.Template
	if err := h.db.FindOne(ctx, "templates", bson.M{"_id": newTemplateID}, &newTemplate); err != nil {
		http.Error(w, "New template not found", http.StatusNotFound)
		return
	}

	// Save current version before making changes
	cursor, err := h.db.FindMany(ctx, "content_versions", bson.M{"content_id": contentID}, options.Find().SetSort(bson.D{{Key: "version", Value: -1}}).SetLimit(1))
	nextVersion := 1
	if err == nil {
		var versions []models.ContentVersion
		if cursor.All(ctx, &versions) == nil && len(versions) > 0 {
			nextVersion = versions[0].Version + 1
		}
	}

	version := models.ContentVersion{
		ContentID:  contentID,
		Version:    nextVersion,
		Title:      content.Title,
		Slug:       content.Slug,
		Data:       content.Data,
		Published:  content.Published,
		TemplateID: content.TemplateID,
		CreatedAt:  time.Now(),
	}
	if _, err := h.db.InsertOne(ctx, "content_versions", version); err != nil {
		fmt.Printf("Warning: Failed to save content version: %v\n", err)
	}

	// Build new field map
	newFieldMap := make(map[string]bool)
	for _, f := range newTemplate.Fields {
		newFieldMap[f.Name] = true
	}

	// Filter data to only include fields in new template
	newData := make(map[string]interface{})
	if content.Data != nil {
		for key, value := range content.Data {
			if newFieldMap[key] {
				newData[key] = value
			}
		}
	}

	// Update content with new template
	update := bson.M{
		"$set": bson.M{
			"template_id": newTemplateID,
			"data":        newData,
			"updated_at":  time.Now(),
		},
	}

	if err := h.db.UpdateOne(ctx, "content", bson.M{"_id": contentID}, update); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Regenerate static file if published
	if content.Published {
		content.TemplateID = newTemplateID
		content.Data = newData
		h.generateStaticPage(ctx, &content, &newTemplate)
	}

	http.Redirect(w, r, "/cm/content/"+contentID.Hex(), http.StatusSeeOther)
}

func (h *Handler) ListContentVersions(w http.ResponseWriter, r *http.Request) {
	if !h.auth.IsAuthenticated(r) {
		http.Redirect(w, r, "/cm/login", http.StatusSeeOther)
		return
	}

	vars := mux.Vars(r)
	id, err := primitive.ObjectIDFromHex(vars["id"])
	if err != nil {
		http.Error(w, "Invalid ID", http.StatusBadRequest)
		return
	}

	ctx := r.Context()

	cursor, err := h.db.FindMany(ctx, "content_versions", bson.M{"content_id": id}, options.Find().SetSort(bson.D{{Key: "version", Value: -1}}))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	var versions []models.ContentVersion
	if err := cursor.All(ctx, &versions); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(versions)
}

func (h *Handler) ViewContentVersion(w http.ResponseWriter, r *http.Request) {
	if !h.auth.IsAuthenticated(r) {
		http.Redirect(w, r, "/cm/login", http.StatusSeeOther)
		return
	}

	vars := mux.Vars(r)
	contentID, err := primitive.ObjectIDFromHex(vars["id"])
	if err != nil {
		http.Error(w, "Invalid ID", http.StatusBadRequest)
		return
	}

	ctx := r.Context()

	var version models.ContentVersion
	versionStr := vars["version"]

	if versionStr == "latest" {
		// Get the latest version
		cursor, err := h.db.FindMany(ctx, "content_versions", bson.M{"content_id": contentID}, options.Find().SetSort(bson.D{{Key: "version", Value: -1}}).SetLimit(1))
		if err != nil {
			http.Error(w, "Error finding versions", http.StatusInternalServerError)
			return
		}
		var versions []models.ContentVersion
		if err := cursor.All(ctx, &versions); err != nil || len(versions) == 0 {
			http.Error(w, "No versions found", http.StatusNotFound)
			return
		}
		version = versions[0]
	} else {
		versionNum, err := strconv.Atoi(versionStr)
		if err != nil {
			http.Error(w, "Invalid version number", http.StatusBadRequest)
			return
		}
		if err := h.db.FindOne(ctx, "content_versions", bson.M{"content_id": contentID, "version": versionNum}, &version); err != nil {
			http.Error(w, "Version not found", http.StatusNotFound)
			return
		}
	}

	// Get the template to render the content
	var tmpl models.Template
	if err := h.db.FindOne(ctx, "templates", bson.M{"_id": version.TemplateID}, &tmpl); err != nil {
		http.Error(w, "Template not found", http.StatusNotFound)
		return
	}

	// Create a temporary content object for rendering
	tempContent := models.Content{
		ID:           contentID,
		TemplateID:   version.TemplateID,
		TemplateName: version.TemplateName,
		Title:        version.Title,
		Slug:         version.Slug,
		Data:         version.Data,
		UseHeader:    version.UseHeader,
		UseFooter:    version.UseFooter,
		UseTheme:     version.UseTheme,
	}

	// Render the content using the template
	rendered := h.renderContent(&tempContent, &tmpl)

	// Render with preview banner
	theme, _ := h.db.GetThemeSettings(ctx)

	previewBanner := fmt.Sprintf(`<div style="background: #f59e0b; color: #000; padding: 1rem; text-align: center; position: fixed; top: 0; left: 0; right: 0; z-index: 9999;">
		<strong>Version Preview</strong> - Viewing version %d of "%s" (saved %s)
		<a href="/cm/content/%s" style="margin-left: 1rem; color: #000; text-decoration: underline;">← Back to Editor</a>
	</div><div style="padding-top: 50px;">`, version.Version, version.Title, version.CreatedAt.Format("Jan 2, 2006 3:04 PM"), contentID.Hex())

	h.renderPublicWithOptions(w, r, theme, previewBanner+rendered+"</div>", version.UseHeader, version.UseFooter)
}

func (h *Handler) DiffContentVersion(w http.ResponseWriter, r *http.Request) {
	if !h.auth.IsAuthenticated(r) {
		http.Redirect(w, r, "/cm/login", http.StatusSeeOther)
		return
	}

	vars := mux.Vars(r)
	contentID, err := primitive.ObjectIDFromHex(vars["id"])
	if err != nil {
		http.Error(w, "Invalid ID", http.StatusBadRequest)
		return
	}

	ctx := r.Context()

	// Get the version to compare
	var version models.ContentVersion
	versionStr := vars["version"]

	if versionStr == "latest" {
		cursor, err := h.db.FindMany(ctx, "content_versions", bson.M{"content_id": contentID}, options.Find().SetSort(bson.D{{Key: "version", Value: -1}}).SetLimit(1))
		if err != nil {
			http.Error(w, "Error finding versions", http.StatusInternalServerError)
			return
		}
		var versions []models.ContentVersion
		if err := cursor.All(ctx, &versions); err != nil || len(versions) == 0 {
			http.Error(w, "No versions found", http.StatusNotFound)
			return
		}
		version = versions[0]
	} else {
		versionNum, err := strconv.Atoi(versionStr)
		if err != nil {
			http.Error(w, "Invalid version number", http.StatusBadRequest)
			return
		}
		if err := h.db.FindOne(ctx, "content_versions", bson.M{"content_id": contentID, "version": versionNum}, &version); err != nil {
			http.Error(w, "Version not found", http.StatusNotFound)
			return
		}
	}

	// Get current content
	var currentContent models.Content
	if err := h.db.FindOne(ctx, "content", bson.M{"_id": contentID}, &currentContent); err != nil {
		http.Error(w, "Content not found", http.StatusNotFound)
		return
	}

	// Render the diff page
	h.renderAdmin(w, r, "version_diff", map[string]interface{}{
		"Version": version,
		"Current": currentContent,
	})
}

func (h *Handler) RevertContentVersion(w http.ResponseWriter, r *http.Request) {
	if !h.auth.IsAuthenticated(r) {
		http.Redirect(w, r, "/cm/login", http.StatusSeeOther)
		return
	}

	vars := mux.Vars(r)
	contentID, err := primitive.ObjectIDFromHex(vars["id"])
	if err != nil {
		http.Error(w, "Invalid ID", http.StatusBadRequest)
		return
	}

	versionNum, err := strconv.Atoi(vars["version"])
	if err != nil {
		http.Error(w, "Invalid version number", http.StatusBadRequest)
		return
	}

	ctx := r.Context()

	// Get the version to revert to
	var version models.ContentVersion
	if err := h.db.FindOne(ctx, "content_versions", bson.M{"content_id": contentID, "version": versionNum}, &version); err != nil {
		http.Error(w, "Version not found", http.StatusNotFound)
		return
	}

	// Revert to the selected version
	update := bson.M{
		"$set": bson.M{
			"template_id":      version.TemplateID,
			"template_name":    version.TemplateName,
			"title":            version.Title,
			"slug":             version.Slug,
			"folder_id":        version.FolderID,
			"folder_path":      version.FolderPath,
			"full_path":        version.FullPath,
			"category":         version.Category,
			"meta_description": version.MetaDescription,
			"og_image":         version.OGImage,
			"data":             version.Data,
			"use_header":       version.UseHeader,
			"use_footer":       version.UseFooter,
			"use_theme":        version.UseTheme,
			"raw_mode":         version.RawMode,
			"updated_at":       time.Now(),
		},
	}

	if err := h.db.UpdateOne(ctx, "content", bson.M{"_id": contentID}, update); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Fetch the reverted content and save it as a new version
	var revertedContent models.Content
	h.db.FindOne(ctx, "content", bson.M{"_id": contentID}, &revertedContent)
	if err := h.saveContentVersion(ctx, &revertedContent); err != nil {
		fmt.Printf("Warning: Failed to save content version after revert: %v\n", err)
	}

	// Regenerate static file if published
	if revertedContent.Published {
		var tmpl models.Template
		if err := h.db.FindOne(ctx, "templates", bson.M{"_id": revertedContent.TemplateID}, &tmpl); err == nil {
			h.generateStaticPage(ctx, &revertedContent, &tmpl)
		}
	}

	http.Redirect(w, r, "/cm/content/"+contentID.Hex(), http.StatusSeeOther)
}

func (h *Handler) RegenerateContent(w http.ResponseWriter, r *http.Request) {
	if !h.auth.IsAuthenticated(r) {
		http.Redirect(w, r, "/cm/login", http.StatusSeeOther)
		return
	}

	vars := mux.Vars(r)
	id, err := primitive.ObjectIDFromHex(vars["id"])
	if err != nil {
		http.Error(w, "Invalid ID", http.StatusBadRequest)
		return
	}

	ctx := r.Context()
	var content models.Content
	if err := h.db.FindOne(ctx, "content", bson.M{"_id": id}, &content); err != nil {
		http.Error(w, "Content not found", http.StatusNotFound)
		return
	}

	var tmpl models.Template
	if err := h.db.FindOne(ctx, "templates", bson.M{"_id": content.TemplateID}, &tmpl); err != nil {
		http.Error(w, "Template not found", http.StatusNotFound)
		return
	}

	if content.Published {
		h.generateStaticPage(ctx, &content, &tmpl)
	}

	http.Redirect(w, r, "/cm/content", http.StatusSeeOther)
}

// Collection handlers
func (h *Handler) ListCollections(w http.ResponseWriter, r *http.Request) {
	if !h.auth.IsAuthenticated(r) {
		http.Redirect(w, r, "/cm/login", http.StatusSeeOther)
		return
	}

	ctx := r.Context()
	cursor, err := h.db.FindMany(ctx, "collections", bson.M{})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	var collections []models.Collection
	cursor.All(ctx, &collections)

	h.renderAdmin(w, r, "collections_list", map[string]interface{}{"Collections": collections})
}

func (h *Handler) NewCollection(w http.ResponseWriter, r *http.Request) {
	if !h.auth.IsAuthenticated(r) {
		http.Redirect(w, r, "/cm/login", http.StatusSeeOther)
		return
	}
	h.renderAdmin(w, r, "collection_form", map[string]interface{}{"IsNew": true})
}

func (h *Handler) CreateCollection(w http.ResponseWriter, r *http.Request) {
	if !h.auth.IsAuthenticated(r) {
		http.Redirect(w, r, "/cm/login", http.StatusSeeOther)
		return
	}

	r.ParseForm()
	itemsPerPage := 10
	fmt.Sscanf(r.FormValue("items_per_page"), "%d", &itemsPerPage)

	collection := models.Collection{
		Name:         r.FormValue("name"),
		Slug:         slugify(r.FormValue("name")),
		Description:  r.FormValue("description"),
		Category:     r.FormValue("category"),
		SortField:    r.FormValue("sort_field"),
		SortOrder:    r.FormValue("sort_order"),
		ItemTemplate: r.FormValue("item_template"),
		PageTemplate: r.FormValue("page_template"),
		ItemsPerPage: itemsPerPage,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}

	ctx := r.Context()
	newID, err := h.db.InsertOne(ctx, "collections", collection)
	if err != nil {
		h.renderAdmin(w, r, "collection_form", map[string]interface{}{
			"IsNew":      true,
			"Collection": collection,
			"Error":      err.Error(),
		})
		return
	}
	auditResource(r, newID.Hex())

	http.Redirect(w, r, "/cm/collections", http.StatusSeeOther)
}

func (h *Handler) EditCollection(w http.ResponseWriter, r *http.Request) {
	if !h.auth.IsAuthenticated(r) {
		http.Redirect(w, r, "/cm/login", http.StatusSeeOther)
		return
	}

	vars := mux.Vars(r)
	id, err := primitive.ObjectIDFromHex(vars["id"])
	if err != nil {
		http.Error(w, "Invalid ID", http.StatusBadRequest)
		return
	}

	var collection models.Collection
	if err := h.db.FindOne(r.Context(), "collections", bson.M{"_id": id}, &collection); err != nil {
		http.Error(w, "Collection not found", http.StatusNotFound)
		return
	}

	h.renderAdmin(w, r, "collection_form", map[string]interface{}{"IsNew": false, "Collection": collection})
}

func (h *Handler) UpdateCollection(w http.ResponseWriter, r *http.Request) {
	if !h.auth.IsAuthenticated(r) {
		http.Redirect(w, r, "/cm/login", http.StatusSeeOther)
		return
	}

	vars := mux.Vars(r)
	id, err := primitive.ObjectIDFromHex(vars["id"])
	if err != nil {
		http.Error(w, "Invalid ID", http.StatusBadRequest)
		return
	}

	r.ParseForm()
	itemsPerPage := 10
	fmt.Sscanf(r.FormValue("items_per_page"), "%d", &itemsPerPage)

	update := bson.M{
		"$set": bson.M{
			"name":           r.FormValue("name"),
			"slug":           slugify(r.FormValue("name")),
			"description":    r.FormValue("description"),
			"category":       r.FormValue("category"),
			"sort_field":     r.FormValue("sort_field"),
			"sort_order":     r.FormValue("sort_order"),
			"item_template":  r.FormValue("item_template"),
			"page_template":  r.FormValue("page_template"),
			"items_per_page": itemsPerPage,
			"updated_at":     time.Now(),
		},
	}

	if err := h.db.UpdateOne(r.Context(), "collections", bson.M{"_id": id}, update); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/cm/collections", http.StatusSeeOther)
}

func (h *Handler) DeleteCollection(w http.ResponseWriter, r *http.Request) {
	if !h.auth.IsAuthenticated(r) {
		http.Redirect(w, r, "/cm/login", http.StatusSeeOther)
		return
	}

	vars := mux.Vars(r)
	id, err := primitive.ObjectIDFromHex(vars["id"])
	if err != nil {
		http.Error(w, "Invalid ID", http.StatusBadRequest)
		return
	}

	if err := h.db.DeleteOne(r.Context(), "collections", bson.M{"_id": id}); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/cm/collections", http.StatusSeeOther)
}

// Folder handlers
func (h *Handler) ListFolders(w http.ResponseWriter, r *http.Request) {
	if !h.auth.IsAuthenticated(r) {
		http.Redirect(w, r, "/cm/login", http.StatusSeeOther)
		return
	}

	ctx := r.Context()
	cursor, err := h.db.FindMany(ctx, "folders", bson.M{}, options.Find().SetSort(bson.D{{Key: "path", Value: 1}}))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	var folders []models.Folder
	cursor.All(ctx, &folders)

	// Build folder tree for display
	folderTree := h.buildFolderTree(folders)

	h.renderAdmin(w, r, "folders_list", map[string]interface{}{
		"Folders":    folders,
		"FolderTree": folderTree,
	})
}

func (h *Handler) NewFolder(w http.ResponseWriter, r *http.Request) {
	if !h.auth.IsAuthenticated(r) {
		http.Redirect(w, r, "/cm/login", http.StatusSeeOther)
		return
	}

	ctx := r.Context()
	folders := h.getAllFolders(ctx)

	h.renderAdmin(w, r, "folder_form", map[string]interface{}{
		"IsNew":   true,
		"Folders": folders,
	})
}

func (h *Handler) CreateFolder(w http.ResponseWriter, r *http.Request) {
	if !h.auth.IsAuthenticated(r) {
		http.Redirect(w, r, "/cm/login", http.StatusSeeOther)
		return
	}

	r.ParseForm()
	ctx := r.Context()

	name := r.FormValue("name")
	slug := r.FormValue("slug")
	if slug == "" {
		slug = slugify(name)
	}

	parentIDStr := r.FormValue("parent_id")
	var parentID *primitive.ObjectID
	parentPath := ""

	if parentIDStr != "" && parentIDStr != "root" {
		pid, err := primitive.ObjectIDFromHex(parentIDStr)
		if err == nil {
			parentID = &pid
			// Get parent folder path
			var parentFolder models.Folder
			if err := h.db.FindOne(ctx, "folders", bson.M{"_id": pid}, &parentFolder); err == nil {
				parentPath = parentFolder.Path
			}
		}
	}

	// Build full path
	fullPath := "/" + slug
	if parentPath != "" {
		fullPath = parentPath + "/" + slug
	}

	folder := models.Folder{
		Name:      name,
		Slug:      slug,
		ParentID:  parentID,
		Path:      fullPath,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	newID, err := h.db.InsertOne(ctx, "folders", folder)
	if err != nil {
		folders := h.getAllFolders(ctx)
		h.renderAdmin(w, r, "folder_form", map[string]interface{}{
			"IsNew":   true,
			"Folder":  folder,
			"Folders": folders,
			"Error":   err.Error(),
		})
		return
	}
	auditResource(r, newID.Hex())

	http.Redirect(w, r, "/cm/folders", http.StatusSeeOther)
}

func (h *Handler) EditFolder(w http.ResponseWriter, r *http.Request) {
	if !h.auth.IsAuthenticated(r) {
		http.Redirect(w, r, "/cm/login", http.StatusSeeOther)
		return
	}

	vars := mux.Vars(r)
	id, err := primitive.ObjectIDFromHex(vars["id"])
	if err != nil {
		http.Error(w, "Invalid ID", http.StatusBadRequest)
		return
	}

	ctx := r.Context()
	var folder models.Folder
	if err := h.db.FindOne(ctx, "folders", bson.M{"_id": id}, &folder); err != nil {
		http.Error(w, "Folder not found", http.StatusNotFound)
		return
	}

	folders := h.getAllFolders(ctx)

	h.renderAdmin(w, r, "folder_form", map[string]interface{}{
		"IsNew":   false,
		"Folder":  folder,
		"Folders": folders,
	})
}

func (h *Handler) UpdateFolder(w http.ResponseWriter, r *http.Request) {
	if !h.auth.IsAuthenticated(r) {
		http.Redirect(w, r, "/cm/login", http.StatusSeeOther)
		return
	}

	vars := mux.Vars(r)
	id, err := primitive.ObjectIDFromHex(vars["id"])
	if err != nil {
		http.Error(w, "Invalid ID", http.StatusBadRequest)
		return
	}

	r.ParseForm()
	ctx := r.Context()

	var existingFolder models.Folder
	if err := h.db.FindOne(ctx, "folders", bson.M{"_id": id}, &existingFolder); err != nil {
		http.Error(w, "Folder not found", http.StatusNotFound)
		return
	}

	oldPath := existingFolder.Path

	name := r.FormValue("name")
	slug := r.FormValue("slug")
	if slug == "" {
		slug = slugify(name)
	}

	parentIDStr := r.FormValue("parent_id")
	var parentID *primitive.ObjectID
	parentPath := ""

	if parentIDStr != "" && parentIDStr != "root" {
		pid, err := primitive.ObjectIDFromHex(parentIDStr)
		if err == nil {
			parentID = &pid
			var parentFolder models.Folder
			if err := h.db.FindOne(ctx, "folders", bson.M{"_id": pid}, &parentFolder); err == nil {
				parentPath = parentFolder.Path
			}
		}
	}

	// Build new full path
	newPath := "/" + slug
	if parentPath != "" {
		newPath = parentPath + "/" + slug
	}

	update := bson.M{
		"$set": bson.M{
			"name":       name,
			"slug":       slug,
			"parent_id":  parentID,
			"path":       newPath,
			"updated_at": time.Now(),
		},
	}

	if err := h.db.UpdateOne(ctx, "folders", bson.M{"_id": id}, update); err != nil {
		folders := h.getAllFolders(ctx)
		h.renderAdmin(w, r, "folder_form", map[string]interface{}{
			"IsNew":   false,
			"Folder":  existingFolder,
			"Folders": folders,
			"Error":   err.Error(),
		})
		return
	}

	// If path changed, update all child folders and content
	if oldPath != newPath {
		h.updateFolderPaths(ctx, oldPath, newPath)
	}

	http.Redirect(w, r, "/cm/folders", http.StatusSeeOther)
}

func (h *Handler) DeleteFolder(w http.ResponseWriter, r *http.Request) {
	if !h.auth.IsAuthenticated(r) {
		http.Redirect(w, r, "/cm/login", http.StatusSeeOther)
		return
	}

	vars := mux.Vars(r)
	id, err := primitive.ObjectIDFromHex(vars["id"])
	if err != nil {
		http.Error(w, "Invalid ID", http.StatusBadRequest)
		return
	}

	ctx := r.Context()

	// Check if folder has children
	childCount, _ := h.db.Count(ctx, "folders", bson.M{"parent_id": id})
	if childCount > 0 {
		http.Error(w, "Cannot delete folder with subfolders", http.StatusBadRequest)
		return
	}

	// Check if folder has content
	contentCount, _ := h.db.Count(ctx, "content", bson.M{"folder_id": id})
	if contentCount > 0 {
		http.Error(w, "Cannot delete folder containing content. Move or delete content first.", http.StatusBadRequest)
		return
	}

	if err := h.db.DeleteOne(ctx, "folders", bson.M{"_id": id}); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/cm/folders", http.StatusSeeOther)
}

// GetAllFolders returns all folders as JSON for the content form dropdown
func (h *Handler) GetAllFoldersAPI(w http.ResponseWriter, r *http.Request) {
	if !h.auth.IsAuthenticated(r) {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	ctx := r.Context()
	folders := h.getAllFolders(ctx)

	w.Header().Set("Content-Type", "application/json")
	fmt.Fprint(w, `[{"path":"/","name":"/ (root)"}`)
	for _, f := range folders {
		name := strings.ReplaceAll(f.Name, `"`, `\"`)
		fmt.Fprintf(w, `,{"id":%q,"path":%q,"name":%q}`, f.ID.Hex(), f.Path, f.Path+" ("+name+")")
	}
	fmt.Fprint(w, "]")
}

// Helper to get all folders sorted by path
func (h *Handler) getAllFolders(ctx context.Context) []models.Folder {
	cursor, err := h.db.FindMany(ctx, "folders", bson.M{}, options.Find().SetSort(bson.D{{Key: "path", Value: 1}}))
	if err != nil {
		return nil
	}
	var folders []models.Folder
	cursor.All(ctx, &folders)
	return folders
}

// Build folder tree structure for display
type FolderTreeItem struct {
	Folder   models.Folder
	Children []FolderTreeItem
	Depth    int
}

func (h *Handler) buildFolderTree(folders []models.Folder) []FolderTreeItem {
	// Create map for quick lookup
	folderMap := make(map[primitive.ObjectID]models.Folder)
	for _, f := range folders {
		folderMap[f.ID] = f
	}

	// Build flat list with depth
	var result []FolderTreeItem
	for _, f := range folders {
		depth := strings.Count(f.Path, "/") - 1
		result = append(result, FolderTreeItem{
			Folder: f,
			Depth:  depth,
		})
	}
	return result
}

// Update all folder paths when a parent folder path changes
func (h *Handler) updateFolderPaths(ctx context.Context, oldPath, newPath string) {
	// Find all folders under the old path
	cursor, _ := h.db.FindMany(ctx, "folders", bson.M{
		"path": bson.M{"$regex": "^" + regexp.QuoteMeta(oldPath) + "/"},
	})

	var childFolders []models.Folder
	cursor.All(ctx, &childFolders)

	for _, cf := range childFolders {
		updatedPath := strings.Replace(cf.Path, oldPath, newPath, 1)
		h.db.UpdateOne(ctx, "folders", bson.M{"_id": cf.ID}, bson.M{
			"$set": bson.M{"path": updatedPath, "updated_at": time.Now()},
		})
	}

	// Update content in these folders
	h.updateContentFolderPaths(ctx, oldPath, newPath)
}

// Update content folder paths and full paths when folder path changes
func (h *Handler) updateContentFolderPaths(ctx context.Context, oldFolderPath, newFolderPath string) {
	// Find all content in the folder or below it: the folder's own path or
	// one that continues with "/" (renaming /blog must not touch /blogging).
	cursor, _ := h.db.FindMany(ctx, "content", bson.M{
		"folder_path": bson.M{"$regex": "^" + regexp.QuoteMeta(oldFolderPath) + "(/|$)"},
	})

	var contents []models.Content
	cursor.All(ctx, &contents)

	for _, c := range contents {
		updatedFolderPath := newFolderPath + strings.TrimPrefix(c.FolderPath, oldFolderPath)
		updatedFullPath := updatedFolderPath
		if c.Slug != "" {
			updatedFullPath = updatedFolderPath + "/" + c.Slug
		}

		// Delete the old static file: a live page's only (a fork copy has
		// none, and its path is the live page's), and never for an empty
		// full_path, which getStaticFilePath maps to the homepage's file.
		if c.ForkID == nil {
			h.removeStaticPage(c.FullPath)
		}

		h.db.UpdateOne(ctx, "content", bson.M{"_id": c.ID}, bson.M{
			"$set": bson.M{
				"folder_path": updatedFolderPath,
				"full_path":   updatedFullPath,
				"updated_at":  time.Now(),
			},
		})

		// Regenerate static file at new location if published
		if c.Published {
			c.FolderPath = updatedFolderPath
			c.FullPath = updatedFullPath
			var tmpl models.Template
			if err := h.db.FindOne(ctx, "templates", bson.M{"_id": c.TemplateID}, &tmpl); err == nil {
				h.generateStaticPage(ctx, &c, &tmpl)
			}
		}
	}
}

// removeStaticPage removes a live page's generated HTML by its full path,
// through the service's removal function: one definition of where a page's
// file lives, and a legacy row with an empty full_path removes nothing
// (getStaticFilePath maps "" to the homepage's file).
func (h *Handler) removeStaticPage(fullPath string) {
	if h.contentService != nil {
		h.contentService.RemoveStaticPage(fullPath)
		return
	}
	if fullPath == "" {
		return
	}
	os.Remove(h.getStaticFilePath(fullPath))
}

// Get the static file path for a content item's full path
func (h *Handler) getStaticFilePath(fullPath string) string {
	if fullPath == "" || fullPath == "/" {
		return "content/generated/index.html"
	}
	// Remove leading slash and add .html
	path := strings.TrimPrefix(fullPath, "/")
	return filepath.Join("content/generated", path+".html")
}

// Theme handlers
func (h *Handler) ThemeSettings(w http.ResponseWriter, r *http.Request) {
	if !h.auth.IsAuthenticated(r) {
		http.Redirect(w, r, "/cm/login", http.StatusSeeOther)
		return
	}

	ctx := r.Context()
	settings, err := h.db.GetThemeSettings(ctx)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	versions, _ := h.db.GetThemeVersions(ctx)

	h.renderAdmin(w, r, "theme", map[string]interface{}{
		"Settings": settings,
		"Versions": versions,
	})
}

func (h *Handler) UpdateTheme(w http.ResponseWriter, r *http.Request) {
	if !h.auth.IsAuthenticated(r) {
		http.Redirect(w, r, "/cm/login", http.StatusSeeOther)
		return
	}

	r.ParseForm()
	ctx := r.Context()

	// Get old settings to check if header/footer changed and for versioning
	oldSettings, _ := h.db.GetThemeSettings(ctx)
	headerChanged := oldSettings == nil || oldSettings.HeaderHTML != r.FormValue("header_html")
	footerChanged := oldSettings == nil || oldSettings.FooterHTML != r.FormValue("footer_html")

	settings := &database.ThemeSettings{
		PrimaryColor:    r.FormValue("primary_color"),
		SecondaryColor:  r.FormValue("secondary_color"),
		AccentColor:     r.FormValue("accent_color"),
		BackgroundColor: r.FormValue("background_color"),
		TextColor:       r.FormValue("text_color"),
		FontFamily:      r.FormValue("font_family"),
		HeadingFont:     r.FormValue("heading_font"),
		BorderRadius:    r.FormValue("border_radius"),
		CustomCSS:       r.FormValue("custom_css"),
		SiteName:        r.FormValue("site_name"),
		SiteTagline:     r.FormValue("site_tagline"),
		LogoURL:         r.FormValue("logo_url"),
		HeadHTML:        r.FormValue("head_html"),
		HeaderHTML:      r.FormValue("header_html"),
		FooterHTML:      r.FormValue("footer_html"),
	}

	if err := h.db.SaveThemeSettings(ctx, settings); err != nil {
		h.renderAdmin(w, r, "theme", map[string]interface{}{
			"Settings": settings,
			"Error":    err.Error(),
		})
		return
	}

	// Save theme version
	h.saveThemeVersion(ctx, settings, oldSettings)

	// Regenerate CSS file
	h.generateThemeCSS(settings)

	// If header or footer changed, regenerate all published content
	if headerChanged || footerChanged {
		h.regenerateAllContent(ctx)
	}

	h.renderAdmin(w, r, "theme", map[string]interface{}{
		"Settings": settings,
		"Success":  "Theme updated successfully!",
	})
}

// saveThemeVersion saves a new version of the theme settings
func (h *Handler) saveThemeVersion(ctx context.Context, theme *database.ThemeSettings, original *database.ThemeSettings) {
	// Get the current version count
	count, err := h.db.GetThemeVersionCount(ctx)
	if err != nil {
		return
	}

	// If no versions exist and we have original theme, save it as v1 first
	if count == 0 && original != nil {
		v1 := &database.ThemeVersion{
			Version:         1,
			PrimaryColor:    original.PrimaryColor,
			SecondaryColor:  original.SecondaryColor,
			AccentColor:     original.AccentColor,
			BackgroundColor: original.BackgroundColor,
			TextColor:       original.TextColor,
			FontFamily:      original.FontFamily,
			HeadingFont:     original.HeadingFont,
			BorderRadius:    original.BorderRadius,
			CustomCSS:       original.CustomCSS,
			SiteName:        original.SiteName,
			SiteTagline:     original.SiteTagline,
			LogoURL:         original.LogoURL,
			HeadHTML:        original.HeadHTML,
			HeaderHTML:      original.HeaderHTML,
			FooterHTML:      original.FooterHTML,
		}
		h.db.SaveThemeVersion(ctx, v1)
		count = 1
	}

	version := int(count) + 1

	themeVersion := &database.ThemeVersion{
		Version:         version,
		PrimaryColor:    theme.PrimaryColor,
		SecondaryColor:  theme.SecondaryColor,
		AccentColor:     theme.AccentColor,
		BackgroundColor: theme.BackgroundColor,
		TextColor:       theme.TextColor,
		FontFamily:      theme.FontFamily,
		HeadingFont:     theme.HeadingFont,
		BorderRadius:    theme.BorderRadius,
		CustomCSS:       theme.CustomCSS,
		SiteName:        theme.SiteName,
		SiteTagline:     theme.SiteTagline,
		LogoURL:         theme.LogoURL,
		HeadHTML:        theme.HeadHTML,
		HeaderHTML:      theme.HeaderHTML,
		FooterHTML:      theme.FooterHTML,
	}

	h.db.SaveThemeVersion(ctx, themeVersion)
}

func (h *Handler) generateThemeCSS(settings *database.ThemeSettings) {
	css := fmt.Sprintf(`:root {
    --primary: %s;
    --secondary: %s;
    --accent: %s;
    --background: %s;
    --text: %s;
    --font-family: %s;
    --heading-font: %s;
    --border-radius: %s;
}

%s`, settings.PrimaryColor, settings.SecondaryColor, settings.AccentColor,
		settings.BackgroundColor, settings.TextColor, settings.FontFamily,
		settings.HeadingFont, settings.BorderRadius, settings.CustomCSS)

	os.WriteFile(services.ThemeCSSFile, []byte(css), 0644)
}

// ThemeVersions shows the theme version history
func (h *Handler) ThemeVersions(w http.ResponseWriter, r *http.Request) {
	if !h.auth.IsAuthenticated(r) {
		http.Redirect(w, r, "/cm/login", http.StatusSeeOther)
		return
	}

	ctx := r.Context()
	versions, err := h.db.GetThemeVersions(ctx)
	if err != nil {
		h.renderAdmin(w, r, "theme_versions", map[string]interface{}{
			"Error": "Failed to load theme versions",
		})
		return
	}

	h.renderAdmin(w, r, "theme_versions", map[string]interface{}{
		"Versions": versions,
	})
}

// ThemeVersionDiff shows the diff between a version and current theme
func (h *Handler) ThemeVersionDiff(w http.ResponseWriter, r *http.Request) {
	if !h.auth.IsAuthenticated(r) {
		http.Redirect(w, r, "/cm/login", http.StatusSeeOther)
		return
	}

	vars := mux.Vars(r)
	versionNum, err := strconv.Atoi(vars["version"])
	if err != nil {
		http.Error(w, "Invalid version number", http.StatusBadRequest)
		return
	}

	ctx := r.Context()
	version, err := h.db.GetThemeVersion(ctx, versionNum)
	if err != nil {
		h.renderAdmin(w, r, "theme_version_diff", map[string]interface{}{
			"Error": "Version not found",
		})
		return
	}

	current, err := h.db.GetThemeSettings(ctx)
	if err != nil {
		h.renderAdmin(w, r, "theme_version_diff", map[string]interface{}{
			"Error": "Failed to load current theme",
		})
		return
	}

	h.renderAdmin(w, r, "theme_version_diff", map[string]interface{}{
		"Version": version,
		"Current": current,
	})
}

// RevertThemeVersion reverts the theme to a previous version
func (h *Handler) RevertThemeVersion(w http.ResponseWriter, r *http.Request) {
	if !h.auth.IsAuthenticated(r) {
		http.Redirect(w, r, "/cm/login", http.StatusSeeOther)
		return
	}

	vars := mux.Vars(r)
	versionNum, err := strconv.Atoi(vars["version"])
	if err != nil {
		http.Error(w, "Invalid version number", http.StatusBadRequest)
		return
	}

	ctx := r.Context()
	version, err := h.db.GetThemeVersion(ctx, versionNum)
	if err != nil {
		auditSkip(r) // no such version: nothing was reverted
		http.Redirect(w, r, "/cm/theme/versions", http.StatusSeeOther)
		return
	}

	// Create theme settings from version
	theme := &database.ThemeSettings{
		PrimaryColor:    version.PrimaryColor,
		SecondaryColor:  version.SecondaryColor,
		AccentColor:     version.AccentColor,
		BackgroundColor: version.BackgroundColor,
		TextColor:       version.TextColor,
		FontFamily:      version.FontFamily,
		HeadingFont:     version.HeadingFont,
		BorderRadius:    version.BorderRadius,
		CustomCSS:       version.CustomCSS,
		SiteName:        version.SiteName,
		SiteTagline:     version.SiteTagline,
		LogoURL:         version.LogoURL,
		HeadHTML:        version.HeadHTML,
		HeaderHTML:      version.HeaderHTML,
		FooterHTML:      version.FooterHTML,
	}

	// Get old settings to check if header/footer changed
	oldSettings, _ := h.db.GetThemeSettings(ctx)
	headerChanged := oldSettings == nil || oldSettings.HeaderHTML != theme.HeaderHTML
	footerChanged := oldSettings == nil || oldSettings.FooterHTML != theme.FooterHTML

	if err := h.db.SaveThemeSettings(ctx, theme); err != nil {
		http.Redirect(w, r, "/cm/theme/versions", http.StatusSeeOther)
		return
	}

	// Save as new version with revert comment
	count, _ := h.db.GetThemeVersionCount(ctx)
	newVersion := &database.ThemeVersion{
		Version:         int(count) + 1,
		Comment:         fmt.Sprintf("Reverted to version %d", versionNum),
		PrimaryColor:    theme.PrimaryColor,
		SecondaryColor:  theme.SecondaryColor,
		AccentColor:     theme.AccentColor,
		BackgroundColor: theme.BackgroundColor,
		TextColor:       theme.TextColor,
		FontFamily:      theme.FontFamily,
		HeadingFont:     theme.HeadingFont,
		BorderRadius:    theme.BorderRadius,
		CustomCSS:       theme.CustomCSS,
		SiteName:        theme.SiteName,
		SiteTagline:     theme.SiteTagline,
		LogoURL:         theme.LogoURL,
		HeadHTML:        theme.HeadHTML,
		HeaderHTML:      theme.HeaderHTML,
		FooterHTML:      theme.FooterHTML,
	}
	h.db.SaveThemeVersion(ctx, newVersion)

	// Regenerate CSS
	h.generateThemeCSS(theme)

	// If header or footer changed, regenerate all published content
	if headerChanged || footerChanged {
		h.regenerateAllContent(ctx)
	}

	http.Redirect(w, r, "/cm/theme/versions", http.StatusSeeOther)
}

// SecuritySettings shows the password change form
func (h *Handler) SecuritySettings(w http.ResponseWriter, r *http.Request) {
	user, ok := h.auth.GetCurrentUser(r)
	if !ok {
		http.Redirect(w, r, "/cm/login", http.StatusSeeOther)
		return
	}

	h.renderAdmin(w, r, "security", map[string]interface{}{
		"IsDefaultPassword": h.auth.MustChangePassword(r),
		"CurrentUser":       user,
	})
}

// UpdatePassword handles password change
func (h *Handler) UpdatePassword(w http.ResponseWriter, r *http.Request) {
	user, ok := h.auth.GetCurrentUser(r)
	if !ok {
		http.Redirect(w, r, "/cm/login", http.StatusSeeOther)
		return
	}

	r.ParseForm()
	currentPassword := r.FormValue("current_password")
	newPassword := r.FormValue("new_password")
	confirmPassword := r.FormValue("confirm_password")

	ctx := r.Context()

	// Validate confirm password matches
	if newPassword != confirmPassword {
		h.renderAdmin(w, r, "security", map[string]interface{}{
			"Error":             "New passwords do not match",
			"IsDefaultPassword": h.auth.MustChangePassword(r),
			"CurrentUser":       user,
		})
		return
	}

	userID, _ := primitive.ObjectIDFromHex(user.ID)

	// Attempt to change password
	if err := h.auth.ChangePassword(ctx, userID, currentPassword, newPassword); err != nil {
		h.renderAdmin(w, r, "security", map[string]interface{}{
			"Error":             err.Error(),
			"IsDefaultPassword": h.auth.MustChangePassword(r),
			"CurrentUser":       user,
		})
		return
	}

	// Clear force password change flag
	h.auth.ClearForcePasswordChange(w, r)

	// Audit log
	if h.auditService != nil {
		h.auditService.LogAsync(models.AuditLog{
			UserID:     userID,
			UserEmail:  user.Email,
			Action:     "password.change",
			Resource:   "user",
			ResourceID: user.ID,
		})
	}

	h.renderAdmin(w, r, "security", map[string]interface{}{
		"Success":           "Password changed successfully!",
		"IsDefaultPassword": false,
		"CurrentUser":       user,
	})
}

// ForceChangePasswordPage shows the mandatory password change page
func (h *Handler) ForceChangePasswordPage(w http.ResponseWriter, r *http.Request) {
	if !h.auth.IsAuthenticated(r) {
		http.Redirect(w, r, "/cm/login", http.StatusSeeOther)
		return
	}
	if !h.auth.MustChangePassword(r) {
		http.Redirect(w, r, "/cm", http.StatusSeeOther)
		return
	}
	h.renderAdmin(w, r, "force_change_password", nil)
}

// ForceChangePasswordHandler processes the mandatory password change
func (h *Handler) ForceChangePasswordHandler(w http.ResponseWriter, r *http.Request) {
	user, ok := h.auth.GetCurrentUser(r)
	if !ok {
		http.Redirect(w, r, "/cm/login", http.StatusSeeOther)
		return
	}

	r.ParseForm()
	currentPassword := r.FormValue("current_password")
	newPassword := r.FormValue("new_password")
	confirmPassword := r.FormValue("confirm_password")

	if newPassword != confirmPassword {
		h.renderAdmin(w, r, "force_change_password", map[string]interface{}{
			"Error": "New passwords do not match",
		})
		return
	}

	userID, _ := primitive.ObjectIDFromHex(user.ID)
	if err := h.auth.ChangePassword(r.Context(), userID, currentPassword, newPassword); err != nil {
		h.renderAdmin(w, r, "force_change_password", map[string]interface{}{
			"Error": err.Error(),
		})
		return
	}

	h.auth.ClearForcePasswordChange(w, r)

	if h.auditService != nil {
		h.auditService.LogAsync(models.AuditLog{
			UserID:     userID,
			UserEmail:  user.Email,
			Action:     "password.change",
			Resource:   "user",
			ResourceID: user.ID,
		})
	}

	http.Redirect(w, r, "/cm", http.StatusSeeOther)
}

// APIKeysPage lists API keys — admins see all, others see only their own
func (h *Handler) APIKeysPage(w http.ResponseWriter, r *http.Request) {
	user, ok := h.auth.GetCurrentUser(r)
	if !ok {
		http.Redirect(w, r, "/cm/login", http.StatusSeeOther)
		return
	}

	var keys []models.APIKey
	var err error
	seeAll := auth.HasPermission(user.Role, auth.PermAPIKeyManageAll)
	if seeAll {
		keys, err = h.apiKeyService.ListAPIKeys(r.Context())
	} else {
		userID, _ := primitive.ObjectIDFromHex(user.ID)
		keys, err = h.apiKeyService.ListAPIKeysForUser(r.Context(), userID)
	}
	if err != nil {
		log.Printf("Failed to list API keys: %v", err)
	}

	cache := map[primitive.ObjectID]*models.User{}
	rows := make([]credentialRow, 0, len(keys))
	for i := range keys {
		row := credentialRow{Key: &keys[i], OwnerEmail: "(no owner)"}
		if keys[i].UserID != nil {
			email, ownerCanMerge := h.ownerInfo(r, cache, *keys[i].UserID)
			row.OwnerEmail = email
			row.MergeEditable = ownerCanMerge && auth.HasPermission(user.Role, auth.PermForkMerge)
		}
		rows = append(rows, row)
	}

	h.renderAdmin(w, r, "api_keys", map[string]interface{}{
		"APIKeys": rows,
		"SeeAll":  seeAll,
		"Success": r.URL.Query().Get("success"),
		"Error":   r.URL.Query().Get("error"),
	})
}

// NewAPIKeyPage shows the form to create a new API key
func (h *Handler) NewAPIKeyPage(w http.ResponseWriter, r *http.Request) {
	if !h.auth.IsAuthenticated(r) {
		http.Redirect(w, r, "/cm/login", http.StatusSeeOther)
		return
	}

	h.renderAdmin(w, r, "api_key_new", nil)
}

// CreateAPIKey creates a new API key owned by the current user and shows it once
func (h *Handler) CreateAPIKey(w http.ResponseWriter, r *http.Request) {
	user, ok := h.auth.GetCurrentUser(r)
	if !ok {
		http.Redirect(w, r, "/cm/login", http.StatusSeeOther)
		return
	}

	name := strings.TrimSpace(r.FormValue("name"))
	description := strings.TrimSpace(r.FormValue("description"))

	if name == "" {
		h.renderAdmin(w, r, "api_key_new", map[string]interface{}{
			"Error": "Name is required",
		})
		return
	}

	// New keys are Sandbox unless Full is chosen. Merging its own forks
	// needs a role that can merge.
	access := services.AccessSettings{
		SandboxOnly:      r.FormValue("access") != "full",
		CanMergeOwnForks: r.FormValue("can_merge_own_forks") == "1" && auth.HasPermission(user.Role, auth.PermForkMerge),
	}
	userID, _ := primitive.ObjectIDFromHex(user.ID)
	rawKey, apiKey, err := h.apiKeyService.CreateScopedAPIKey(r.Context(), name, description, &userID, nil, access.SandboxOnly)
	if err == nil && access.CanMergeOwnForks {
		if err = h.apiKeyService.UpdateAccess(r.Context(), apiKey.ID, access); err == nil {
			apiKey.CanMergeOwnForks = true
		}
	}
	if err != nil {
		h.renderAdmin(w, r, "api_key_new", map[string]interface{}{
			"Error": "Failed to create API key: " + err.Error(),
		})
		return
	}
	auditDetail(r, "sandbox_only", access.SandboxOnly)
	auditDetail(r, "can_merge_own_forks", access.CanMergeOwnForks)

	auditResource(r, apiKey.ID.Hex())
	h.renderAdmin(w, r, "api_key_created", map[string]interface{}{
		"RawKey": rawKey,
		"APIKey": apiKey,
	})
}

// DeleteAPIKey deletes an API key
func (h *Handler) DeleteAPIKey(w http.ResponseWriter, r *http.Request) {
	if !h.auth.IsAuthenticated(r) {
		http.Redirect(w, r, "/cm/login", http.StatusSeeOther)
		return
	}

	vars := mux.Vars(r)
	id, err := primitive.ObjectIDFromHex(vars["id"])
	if err != nil {
		auditSkip(r) // nothing was revoked
		http.Redirect(w, r, "/cm/api-keys", http.StatusSeeOther)
		return
	}

	// As DELETE /api/v1/api-keys/{id}: apikey.manage_all (admin) revokes any
	// key, everyone else only a key they own.
	user, _ := h.auth.GetCurrentUser(r)
	if user != nil && auth.HasPermission(user.Role, auth.PermAPIKeyManageAll) {
		if err := h.apiKeyService.DeleteAPIKey(r.Context(), id); err != nil {
			log.Printf("Failed to delete API key: %v", err)
			auditSkip(r) // nothing was revoked
		}
	} else {
		// DeleteAPIKeyForUser deletes nothing for a key that is not the
		// caller's, without saying so: check first, so the user is told.
		ownerID, err := primitive.ObjectIDFromHex(userIDOf(user))
		owned := int64(0)
		if err == nil {
			owned, err = h.db.Count(r.Context(), "api_keys", bson.M{"_id": id, "user_id": ownerID})
		}
		if err != nil || owned == 0 || h.apiKeyService.DeleteAPIKeyForUser(r.Context(), id, ownerID) != nil {
			h.refuseAdminMessage(w, r, false, "That API key was not found or is not yours. Nothing was changed.")
			return
		}
	}

	http.Redirect(w, r, "/cm/api-keys", http.StatusSeeOther)
}

// userIDOf returns the session user's ID, "" for no user.
func userIDOf(u *auth.SessionUser) string {
	if u == nil {
		return ""
	}
	return u.ID
}

// SiteConfiguration shows the site configuration page
func (h *Handler) SiteConfiguration(w http.ResponseWriter, r *http.Request) {
	if !h.auth.IsAuthenticated(r) {
		http.Redirect(w, r, "/cm/login", http.StatusSeeOther)
		return
	}

	ctx := r.Context()
	config, err := h.db.GetSiteConfig(ctx)
	if err != nil {
		http.Error(w, "Error loading configuration", http.StatusInternalServerError)
		return
	}

	theme, _ := h.db.GetThemeSettings(ctx)
	dbVersion, _ := h.db.GetDatabaseVersion(ctx)

	// Import build package for software version
	softwareVersion := build.GetVersion()

	// Determine environment label
	envLabel := "Development"
	if h.env == "production" || h.env == "prod" {
		envLabel = "Production"
	}

	// Run Cloudflare health check if credentials are configured.
	var cfStatus, cfError string
	if config.CloudflareZoneID != "" && config.CloudflareAPIToken != "" && h.cfService != nil {
		if err := h.cfService.TestConnection(ctx); err != nil {
			cfStatus = "error"
			cfError = err.Error()
		} else {
			cfStatus = "ok"
		}
	}

	// The form carries the Cloudflare API token. Only a role that can save
	// the form (settings.edit) gets it; settings.view sees the page without it.
	if user, ok := h.auth.GetCurrentUser(r); !ok || !auth.HasPermission(user.Role, auth.PermSettingsEdit) {
		redacted := *config
		redacted.CloudflareAPIToken = ""
		config = &redacted
	}

	h.renderAdmin(w, r, "config", map[string]interface{}{
		"Config":          config,
		"SiteName":        theme.SiteName,
		"SoftwareVersion": softwareVersion,
		"DatabaseVersion": dbVersion,
		"EnvLabel":        envLabel,
		"CFStatus":        cfStatus,
		"CFError":         cfError,
	})
}

// UpdateSiteConfiguration handles site configuration updates
func (h *Handler) UpdateSiteConfiguration(w http.ResponseWriter, r *http.Request) {
	if !h.auth.IsAuthenticated(r) {
		http.Redirect(w, r, "/cm/login", http.StatusSeeOther)
		return
	}

	r.ParseForm()
	ctx := r.Context()

	config, _ := h.db.GetSiteConfig(ctx)
	config.TitleTemplate = r.FormValue("title_template")
	config.TitleTemplateNoTitle = r.FormValue("title_template_no_title")

	if s := strings.TrimSpace(r.FormValue("max_upload_bytes")); s != "" {
		if v, err := strconv.ParseInt(s, 10, 64); err == nil && v > 0 {
			config.MaxUploadBytes = v
		}
	}

	config.CloudflareZoneID = strings.TrimSpace(r.FormValue("cloudflare_zone_id"))
	config.CloudflareAPIToken = strings.TrimSpace(r.FormValue("cloudflare_api_token"))
	config.CFCacheEnabled = r.FormValue("cf_cache_enabled") == "true"

	if err := h.db.SaveSiteConfig(ctx, config); err != nil {
		theme, _ := h.db.GetThemeSettings(ctx)
		h.renderAdmin(w, r, "config", map[string]interface{}{
			"Config":   config,
			"SiteName": theme.SiteName,
			"Error":    "Failed to save configuration",
		})
		return
	}

	theme, _ := h.db.GetThemeSettings(ctx)
	h.renderAdmin(w, r, "config", map[string]interface{}{
		"Config":   config,
		"SiteName": theme.SiteName,
		"Success":  "Configuration saved successfully!",
	})
}

// API handler for template fields
func (h *Handler) GetTemplateFields(w http.ResponseWriter, r *http.Request) {
	// Require authentication to prevent information disclosure
	if !h.auth.IsAuthenticated(r) {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	vars := mux.Vars(r)
	id, err := primitive.ObjectIDFromHex(vars["id"])
	if err != nil {
		http.Error(w, "Invalid ID", http.StatusBadRequest)
		return
	}

	var tmpl models.Template
	if err := h.db.FindOne(r.Context(), "templates", bson.M{"_id": id}, &tmpl); err != nil {
		http.Error(w, "Template not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{"fields": %v}`, tmpl.Fields)
}

// GetAllSlugs returns all content slugs for link autocomplete
func (h *Handler) GetAllSlugs(w http.ResponseWriter, r *http.Request) {
	if !h.auth.IsAuthenticated(r) {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	ctx := r.Context()
	cursor, err := h.db.FindMany(ctx, "content", bson.M{}, options.Find().SetProjection(bson.M{"slug": 1, "full_path": 1, "title": 1}))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	var contents []models.Content
	if err := cursor.All(ctx, &contents); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Build JSON response
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprint(w, "[")
	for i, c := range contents {
		if i > 0 {
			fmt.Fprint(w, ",")
		}
		// Use full_path if available, fall back to slug for legacy content
		path := c.FullPath
		if path == "" {
			path = "/" + c.Slug
			if c.Slug == "" {
				path = "/"
			}
		}
		// Escape JSON strings properly
		title := strings.ReplaceAll(c.Title, `"`, `\"`)
		title = strings.ReplaceAll(title, "\n", " ")
		fmt.Fprintf(w, `{"slug":%q,"title":%q}`, path, title)
	}
	fmt.Fprint(w, "]")
}

// ServePage serves public content
func (h *Handler) ServePage(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	slug := vars["slug"]

	// Build full path from URL
	fullPath := "/" + slug
	if slug == "" {
		fullPath = "/"
	}

	// Check for static asset first (for files uploaded via Asset Library)
	// These are served from content/generated at any path
	if slug != "" && strings.Contains(slug, ".") {
		// Looks like a file (has extension) - check if it exists as a static asset
		assetPath := filepath.Join("content/generated", fullPath)
		if info, err := os.Stat(assetPath); err == nil && !info.IsDir() {
			// Serve the static file with proper MIME type
			ext := strings.ToLower(filepath.Ext(slug))
			mimeType := "application/octet-stream"
			switch ext {
			case ".png":
				mimeType = "image/png"
			case ".jpg", ".jpeg":
				mimeType = "image/jpeg"
			case ".gif":
				mimeType = "image/gif"
			case ".svg":
				mimeType = "image/svg+xml"
			case ".webp":
				mimeType = "image/webp"
			case ".ico":
				mimeType = "image/x-icon"
			case ".css":
				mimeType = "text/css"
			case ".js":
				mimeType = "application/javascript"
			case ".json":
				mimeType = "application/json"
			case ".woff":
				mimeType = "font/woff"
			case ".woff2":
				mimeType = "font/woff2"
			case ".ttf":
				mimeType = "font/ttf"
			case ".pdf":
				mimeType = "application/pdf"
			case ".xml":
				mimeType = "application/xml"
			case ".txt":
				mimeType = "text/plain"
			}
			w.Header().Set("Content-Type", mimeType)
			w.Header().Set("Cache-Control", "public, max-age=31536000")
			http.ServeFile(w, r, assetPath)
			return
		}
	}

	ctx := r.Context()
	theme, _ := h.db.GetThemeSettings(ctx)

	// Check for redirect first
	redirect, err := h.db.GetRedirect(ctx, fullPath)
	if err == nil && redirect != nil {
		statusCode := redirect.StatusCode
		if statusCode == 0 {
			statusCode = 301
		}
		// Do not cache redirects — they may be deleted or changed at any time
		w.Header().Set("Cache-Control", "no-store")
		http.Redirect(w, r, redirect.ToPath, statusCode)
		return
	}

	// Check for collection first (collections are still top-level)
	var collection models.Collection
	if err := h.db.FindOne(ctx, "collections", bson.M{"slug": slug}, &collection); err == nil {
		h.serveCollection(w, r, &collection, theme)
		return
	}

	// Check for fork preview mode — if active and fork has a copy of this page, serve it
	activeFork := h.getActiveForkPreview(r)
	if activeFork != nil {
		if forkPage, err := h.forkService.GetForkPageByPath(ctx, activeFork.ID, fullPath); err == nil && forkPage != nil {
			h.servePageContent(w, r, forkPage, theme, activeFork)
			return
		}
	}

	// Look up content from database - try full_path first, fall back to slug for legacy
	var content models.Content
	// Live pages only: never a fork copy, never a soft-deleted page (a page
	// deleted through the API keeps its published flag and its path).
	filter := bson.M{"published": true, "full_path": fullPath, "fork_id": nil, "deleted": bson.M{"$ne": true}}
	if err := h.db.FindOne(ctx, "content", filter, &content); err != nil {
		// Fall back to legacy slug lookup
		legacyFilter := bson.M{"published": true, "slug": slug, "fork_id": nil, "deleted": bson.M{"$ne": true}}
		if err := h.db.FindOne(ctx, "content", legacyFilter, &content); err != nil {
			// Paths are case-insensitive: try a case-insensitive match and
			// 301 to the canonical casing (e.g. /claude.md -> /CLAUDE.md).
			ciFilter := bson.M{
				"published": true,
				"full_path": bson.M{"$regex": "^" + regexp.QuoteMeta(fullPath) + "$", "$options": "i"},
				"fork_id":   nil,
				"deleted":   bson.M{"$ne": true},
			}
			if err := h.db.FindOne(ctx, "content", ciFilter, &content); err == nil && content.FullPath != fullPath {
				w.Header().Set("Cache-Control", "public, max-age=3600")
				http.Redirect(w, r, content.FullPath, http.StatusMovedPermanently)
				return
			}
			// /<page>.md → Markdown copy of <page> (real pages ending in .md won above)
			if h.serveMarkdownAlternate(w, r, fullPath) {
				return
			}
			h.serve404(w, r, theme)
			return
		}
	}

	// Record visitor activity for DAU/MAU metrics, hourly stats, and page views (bounded goroutine with 5s timeout)
	if h.analyticsService != nil {
		visitorIP := middleware.GetClientIP(r, h.proxyConfig)
		ipHash := services.HashIP(visitorIP)
		pagePath := content.FullPath
		referrer := r.Referer()
		if referrer == "" {
			// AI assistants often strip the referrer but tag links (?utm_source=chatgpt.com).
			referrer = services.AIReferrerFromQuery(r.URL.RawQuery)
		}
		userAgent := r.UserAgent()
		h.analyticsService.RecordCrawlerHit(pagePath, userAgent)
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			h.analyticsService.RecordActivity(ctx, ipHash)
			h.analyticsService.RecordHourlyVisitor(ctx, ipHash, userAgent)
			h.analyticsService.RecordPageView(ctx, pagePath, referrer, userAgent)
		}()
	}

	h.servePageContent(w, r, &content, theme, activeFork)
}

// servePageContent renders a content item to the response.
// activeFork is non-nil when serving a fork preview; the preview bar is injected into the page.
func (h *Handler) servePageContent(w http.ResponseWriter, r *http.Request, content *models.Content, theme *database.ThemeSettings, activeFork *models.ContentFork) {
	ctx := r.Context()
	fullPath := content.FullPath

	// For blank pages with raw mode and no theme, serve raw HTML directly
	if !content.UseTheme && content.TemplateName == "Blank Page" {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		htmlContent := ""
		if v, ok := content.Data["content"].(string); ok {
			htmlContent = v
		}
		// Raw pages bypass the themed layout, so inject structured data, robots /
		// Markdown tags, and feed links into the authored <head> directly
		// (skipping any JSON-LD types the page already declares).
		if activeFork == nil {
			if content.NoIndex {
				w.Header().Set("X-Robots-Tag", "noindex")
			}
			head := h.pageSEOHead(r, content, nil, content.OGImage, htmlContent) + h.feedLinks(r, h.seoConfig(ctx), theme.SiteName)
			htmlContent = injectHead(htmlContent, head)
		}
		if activeFork != nil {
			w.Header().Set("Cache-Control", "private, no-store")
			htmlContent = forkPreviewBar(activeFork) + htmlContent
		}
		w.Write([]byte(htmlContent))
		return
	}

	// Load template — needed for both rendering and OG image inference.
	var tmpl models.Template
	if err := h.db.FindOne(ctx, "templates", bson.M{"_id": content.TemplateID}, &tmpl); err != nil {
		http.Error(w, "Template not found", http.StatusInternalServerError)
		return
	}

	ogImage := inferOGImage(content, &tmpl)
	canonical := h.resolveBaseURL(r) + fullPath
	if content.NoIndex {
		w.Header().Set("X-Robots-Tag", "noindex")
	}

	// For live pages (no active fork), try static file first
	if activeFork == nil {
		staticPath := h.getStaticFilePath(fullPath)
		if _, err := os.Stat(staticPath); err == nil {
			staticContent, _ := os.ReadFile(staticPath)
			head := h.pageSEOHead(r, content, &tmpl, ogImage, string(staticContent))
			h.renderPublicWithSEO(w, r, theme, string(staticContent), content.UseHeader, content.UseFooter,
				content.Title, content.MetaDescription, ogImage, canonical, head)
			return
		}
	}

	rendered := h.renderContent(content, &tmpl)
	head := h.pageSEOHead(r, content, &tmpl, ogImage, rendered)
	if activeFork != nil {
		rendered = forkPreviewBar(activeFork) + rendered
	}
	h.renderPublicWithSEO(w, r, theme, rendered, content.UseHeader, content.UseFooter,
		content.Title, content.MetaDescription, ogImage, canonical, head)
}

// forkPreviewBar returns the HTML for the floating preview bar injected during fork preview.
func forkPreviewBar(fork *models.ContentFork) string {
	return `<div id="lc-fork-preview-bar" style="position:fixed;top:0;left:0;right:0;z-index:99999;background:#6366f1;color:white;padding:10px 20px;display:flex;align-items:center;gap:16px;font-family:system-ui,sans-serif;font-size:13px;font-weight:500;box-shadow:0 2px 8px rgba(0,0,0,0.3);">` +
		`<span>🌿 Previewing fork: <strong>` + template.HTMLEscapeString(fork.Name) + `</strong></span>` +
		`<span style="flex:1"></span>` +
		`<a href="/cm/forks/` + fork.ID.Hex() + `" style="color:white;opacity:0.85;text-decoration:underline;">Manage</a>` +
		`<a href="/cm/forks/exit-preview" style="background:rgba(255,255,255,0.2);padding:5px 14px;border-radius:6px;color:white;text-decoration:none;">Exit Preview</a>` +
		`</div><div style="height:44px"></div>`
}

func (h *Handler) serveCollection(w http.ResponseWriter, r *http.Request, collection *models.Collection, theme *database.ThemeSettings) {
	ctx := r.Context()

	sortOrder := 1
	if collection.SortOrder == "desc" {
		sortOrder = -1
	}
	sortField := collection.SortField
	if sortField == "" {
		sortField = "created_at"
	}

	opts := options.Find().SetSort(bson.D{{Key: sortField, Value: sortOrder}})
	// Live pages only: a fork copy flagged published and a soft-deleted page
	// are not on the site and must not be listed on a public collection page.
	cursor, err := h.db.FindMany(ctx, "content", bson.M{
		"category":  collection.Category,
		"published": true,
		"deleted":   bson.M{"$ne": true},
		"fork_id":   nil,
	}, opts)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	var items []models.Content
	cursor.All(ctx, &items)

	// Render each item using item template
	var itemsHTML strings.Builder
	for _, item := range items {
		itemData := make(map[string]interface{})
		for k, v := range item.Data {
			itemData[k] = v
		}
		itemData["slug"] = item.Slug
		itemData["title"] = item.Title
		if item.PublishedAt != nil {
			itemData["published_at"] = item.PublishedAt.Format("January 2, 2006")
		}

		tmpl, err := template.New("item").Parse(collection.ItemTemplate)
		if err != nil {
			continue
		}
		var buf bytes.Buffer
		tmpl.Execute(&buf, itemData)
		itemsHTML.WriteString(buf.String())
	}

	// Render page template
	pageData := map[string]interface{}{
		"collection_name":        collection.Name,
		"collection_description": collection.Description,
		"items":                  template.HTML(itemsHTML.String()),
		"pagination":             "",
	}

	pageTmpl, _ := template.New("page").Parse(collection.PageTemplate)
	var pageHTML bytes.Buffer
	pageTmpl.Execute(&pageHTML, pageData)

	h.renderPublic(w, r, theme, pageHTML.String())
}

func (h *Handler) serve404(w http.ResponseWriter, r *http.Request, theme *database.ThemeSettings) {
	w.WriteHeader(http.StatusNotFound)
	ctx := r.Context()

	// Try to serve 404 content page
	var content404 models.Content
	if err := h.db.FindOne(ctx, "content", bson.M{"slug": "404", "published": true, "deleted": bson.M{"$ne": true}, "fork_id": nil}, &content404); err == nil {
		// Try static file first
		staticPath := "content/generated/404.html"
		if _, err := os.Stat(staticPath); err == nil {
			staticContent, _ := os.ReadFile(staticPath)
			h.renderPublicWithOptions(w, r, theme, string(staticContent), content404.UseHeader, content404.UseFooter)
			return
		}

		// Fall back to rendering from DB
		var tmpl models.Template
		if err := h.db.FindOne(ctx, "templates", bson.M{"_id": content404.TemplateID}, &tmpl); err == nil {
			rendered := h.renderContent(&content404, &tmpl)
			h.renderPublicWithOptions(w, r, theme, rendered, content404.UseHeader, content404.UseFooter)
			return
		}
	}

	// Fallback to simple 404
	h.renderPublicWithOptions(w, r, theme, `<div style="text-align:center;padding:4rem"><h1>404</h1><p>Page not found</p></div>`, true, true)
}

func (h *Handler) renderContent(content *models.Content, tmpl *models.Template) string {
	data := make(map[string]interface{})
	for k, v := range content.Data {
		// Convert string values to template.HTML so they're not escaped
		if str, ok := v.(string); ok {
			data[k] = template.HTML(str)
		} else {
			data[k] = v
		}
	}
	data["title"] = content.Title
	data["slug"] = content.Slug
	if content.PublishedAt != nil {
		data["published_at"] = content.PublishedAt.Format("January 2, 2006")
	}

	t, err := template.New("content").Parse(tmpl.HTMLLayout)
	if err != nil {
		return fmt.Sprintf("<p>Error rendering content: %v</p>", err)
	}

	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return fmt.Sprintf("<p>Error rendering content: %v</p>", err)
	}

	return buf.String()
}

func (h *Handler) generateStaticPage(ctx context.Context, content *models.Content, tmpl *models.Template) error {
	// Fork copies share full_path with their live counterpart — generating
	// them would overwrite the live page's static file with fork content.
	if content.ForkID != nil {
		return nil
	}
	rendered := h.renderContent(content, tmpl)

	// Use full path for file location, supporting folders
	staticPath := h.getStaticFilePath(content.FullPath)
	if staticPath == "" {
		// Fallback for legacy content without full_path
		filename := content.Slug + ".html"
		if content.Slug == "" {
			filename = "index.html"
		}
		staticPath = filepath.Join("content/generated", filename)
	}

	// Ensure directory exists
	dir := filepath.Dir(staticPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	return os.WriteFile(staticPath, []byte(rendered), 0644)
}

// regenerateAllContent regenerates all published content (used when header/footer/templates change)
func (h *Handler) regenerateAllContent(ctx context.Context) {
	// Regenerate all published content items
	cursor, err := h.db.FindMany(ctx, "content", bson.M{"published": true, "deleted": bson.M{"$ne": true}, "fork_id": nil})
	if err != nil {
		return
	}

	var contents []models.Content
	if err := cursor.All(ctx, &contents); err != nil {
		return
	}

	for _, content := range contents {
		var tmpl models.Template
		if err := h.db.FindOne(ctx, "templates", bson.M{"_id": content.TemplateID}, &tmpl); err != nil {
			continue
		}
		h.generateStaticPage(ctx, &content, &tmpl)
	}
}

// regenerateContentByTemplate regenerates all content using a specific template
func (h *Handler) regenerateContentByTemplate(ctx context.Context, templateID primitive.ObjectID) {
	cursor, err := h.db.FindMany(ctx, "content", bson.M{"template_id": templateID, "published": true, "deleted": bson.M{"$ne": true}, "fork_id": nil})
	if err != nil {
		return
	}

	var tmpl models.Template
	if err := h.db.FindOne(ctx, "templates", bson.M{"_id": templateID}, &tmpl); err != nil {
		return
	}

	var contents []models.Content
	if err := cursor.All(ctx, &contents); err != nil {
		return
	}

	for _, content := range contents {
		h.generateStaticPage(ctx, &content, &tmpl)
	}
}

func (h *Handler) renderPublic(w http.ResponseWriter, r *http.Request, theme *database.ThemeSettings, content string) {
	h.renderPublicWithOptions(w, r, theme, content, true, true)
}

func (h *Handler) renderPublicWithOptions(w http.ResponseWriter, r *http.Request, theme *database.ThemeSettings, content string, useHeader, useFooter bool) {
	// The homepage renders through this generic path; give it a schema.org
	// WebSite node (the appropriate homepage structured data) so it isn't
	// the one page without JSON-LD.
	jsonLD := ""
	if r.URL.Path == "/" {
		jsonLD = buildWebsiteJSONLD(theme.SiteName, theme.SiteTagline, h.resolveBaseURL(r))
	}
	h.renderPublicWithSEO(w, r, theme, content, useHeader, useFooter, "", "", "", "", jsonLD)
}

func (h *Handler) renderPublicWithSEO(w http.ResponseWriter, r *http.Request, theme *database.ThemeSettings, content string, useHeader, useFooter bool, title, metaDescription, ogImage, canonicalURL string, structuredData ...string) {
	tmpl := template.Must(template.New("layout").Parse(publicLayout))

	// Get site config for title template
	ctx := r.Context()
	config, _ := h.db.GetSiteConfig(ctx)

	// Apply title template
	pageTitle := h.applyTitleTemplate(config, title, theme.SiteName)

	// OG image must be an absolute URL for social media scrapers.
	// Relative paths (e.g. /assets/img.jpg) are resolved against the site base URL.
	if ogImage != "" && strings.HasPrefix(ogImage, "/") {
		base := h.baseURL
		if base == "" {
			scheme := "https"
			if r.TLS == nil && r.Header.Get("X-Forwarded-Proto") != "https" {
				scheme = "http"
			}
			base = scheme + "://" + r.Host
		}
		ogImage = strings.TrimRight(base, "/") + ogImage
	}

	data := map[string]interface{}{
		"Theme":           theme,
		"Content":         template.HTML(content),
		"UseHeader":       useHeader,
		"UseFooter":       useFooter,
		"HeadHTML":        template.HTML(theme.HeadHTML),
		"HeaderHTML":      template.HTML(theme.HeaderHTML),
		"FooterHTML":      template.HTML(theme.FooterHTML),
		"PageTitle":       pageTitle,
		"MetaDescription": metaDescription,
		"OGImage":         ogImage,
		"CanonicalURL":    canonicalURL,
	}
	head := ""
	if len(structuredData) > 0 {
		head = structuredData[0]
	}
	head += h.feedLinks(r, h.seoConfig(ctx), theme.SiteName)
	if head != "" {
		data["StructuredData"] = template.HTML(head)
	}

	// A fork preview is one editor's view of unpublished work: it is never
	// cached, by the browser (Exit Preview would keep showing the copy) or by
	// a CDN (which would serve it to everyone).
	if h.getActiveForkPreview(r) != nil {
		w.Header().Set("Cache-Control", "private, no-store")
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		tmpl.Execute(w, data)
		return
	}

	// ETag and caching headers for public pages
	hash := sha256.Sum256([]byte(content))
	etag := fmt.Sprintf(`"%x"`, hash)
	w.Header().Set("ETag", etag)
	w.Header().Set("Cache-Control", "public, max-age=300, s-maxage=3600")
	w.Header().Set("Vary", "Accept-Encoding")
	if theme.UpdatedAt.After(time.Time{}) {
		w.Header().Set("Last-Modified", theme.UpdatedAt.UTC().Format(http.TimeFormat))
	}

	// Handle conditional requests
	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	tmpl.Execute(w, data)
}

// inferOGImage returns the best OG image for a content item.
// Explicit og_image always wins. If unset, walks the template's field
// definitions in order and returns the first non-empty image-type field value.
// Using the template's field order (a slice, not a map) guarantees we always
// pick the designer-intended primary image (e.g. featured_image before any
// secondary images) rather than random map iteration order.
func inferOGImage(content *models.Content, tmpl *models.Template) string {
	if content.OGImage != "" {
		return content.OGImage
	}
	if tmpl == nil {
		return ""
	}
	for _, field := range tmpl.Fields {
		if field.Type != "image" {
			continue
		}
		if v, ok := content.Data[field.Name]; ok {
			if s, ok := v.(string); ok && s != "" {
				return s
			}
		}
	}
	return ""
}

// applyTitleTemplate applies the configured title template
func (h *Handler) applyTitleTemplate(config *database.SiteConfig, title, siteName string) string {
	var templateStr string
	if title == "" {
		templateStr = config.TitleTemplateNoTitle
		if templateStr == "" {
			templateStr = "{{site_name}}"
		}
	} else {
		templateStr = config.TitleTemplate
		if templateStr == "" {
			templateStr = "{{title}} - {{site_name}}"
		}
	}

	// Replace template variables
	result := strings.ReplaceAll(templateStr, "{{title}}", title)
	result = strings.ReplaceAll(result, "{{site_name}}", siteName)

	return result
}

func templateToInt64(v interface{}) int64 {
	switch n := v.(type) {
	case int:
		return int64(n)
	case int64:
		return n
	case int32:
		return int64(n)
	default:
		return 0
	}
}

func (h *Handler) renderAdmin(w http.ResponseWriter, r *http.Request, name string, data map[string]interface{}) {
	h.renderAdminStatus(w, r, http.StatusOK, name, data)
}

// renderAdminStatus is renderAdmin with an explicit HTTP status (the styled
// 403 page).
func (h *Handler) renderAdminStatus(w http.ResponseWriter, r *http.Request, status int, name string, data map[string]interface{}) {
	if data == nil {
		data = make(map[string]interface{})
	}
	// A page rendered with an Error is a request that did not go through:
	// an audited route writes no audit entry for it (admin_audit_wrap.go).
	if e, ok := data["Error"]; ok && e != nil && e != "" {
		auditSkip(r)
	}
	data["IsAuthenticated"] = h.auth.IsAuthenticated(r)

	// Inject current user into template data for sidebar/nav, and what the
	// role may do so templates can hide controls it cannot use:
	// {{if index .Can "template.edit"}}
	can := map[string]bool{}
	if user, ok := h.auth.GetCurrentUser(r); ok {
		if _, exists := data["CurrentUser"]; !exists {
			data["CurrentUser"] = user
		}
		for _, p := range auth.RolePermissions[user.Role] {
			can[p] = true
		}
	}
	data["Can"] = can

	// Add CSRF token to template data
	data["CSRFToken"] = csrf.Token(r)
	data["CSRFField"] = csrf.TemplateField(r)

	// Running software version, shown under the sidebar logo
	data["AppVersion"] = build.GetVersion()

	// Copilot drawer: available on every admin page for editors+ when the
	// Anthropic key is configured
	if user, okU := h.auth.GetCurrentUser(r); okU {
		data["CopilotEnabled"] = h.anthropicAPIKey != "" && auth.HasPermission(user.Role, auth.PermContentEdit)
	}

	ctx := r.Context()
	theme, _ := h.db.GetThemeSettings(ctx)
	data["Theme"] = theme

	// Get unread message count for sidebar badge
	unreadCount, _ := h.db.Count(ctx, "contact_messages", bson.M{"read": false})
	data["UnreadMessageCount"] = unreadCount

	// Get pending approval count for sidebar badge (use CountPending, not ListPending,
	// to avoid loading the full result set on every admin page render)
	if h.approvalService != nil {
		data["PendingApprovalCount"] = h.approvalService.CountPending(ctx)
	}

	initAdminTemplateCache()
	tmpl := adminTemplateCache[name]
	if tmpl == nil {
		http.Error(w, "unknown template: "+name, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	// Prevent caching of admin pages
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Expires", "0")
	// Note: Security headers are now applied globally via middleware
	if status != http.StatusOK {
		w.WriteHeader(status)
	}
	tmpl.Execute(w, data)
}

func slugify(s string) string {
	// Normalize unicode (NFKD decomposition)
	s = norm.NFKD.String(s)

	// Remove non-ASCII characters (accents, etc.)
	var result strings.Builder
	for _, r := range s {
		if r < 128 {
			result.WriteRune(r)
		}
	}
	s = result.String()

	// Convert to lowercase
	s = strings.ToLower(s)

	// Replace non-alphanumeric characters with hyphens
	reg := regexp.MustCompile(`[^a-z0-9]+`)
	s = reg.ReplaceAllString(s, "-")

	// Trim leading/trailing hyphens
	s = strings.Trim(s, "-")

	// Enforce maximum length (255 chars for URL safety)
	if len(s) > 255 {
		s = s[:255]
		// Don't end with a hyphen after truncation
		s = strings.TrimRight(s, "-")
	}

	return s
}

// slugifyStrict is like slugify but also validates the result isn't empty
func slugifyStrict(s string) (string, error) {
	slug := slugify(s)
	if slug == "" {
		return "", fmt.Errorf("invalid input: results in empty slug")
	}
	return slug, nil
}

// extractInternalLinks finds all internal links (starting with /) in HTML content
func extractInternalLinks(htmlContent string) []string {
	linkRegex := regexp.MustCompile(`href=["'](/[^"'#?]*)["']`)
	matches := linkRegex.FindAllStringSubmatch(htmlContent, -1)

	linkSet := make(map[string]bool)
	for _, match := range matches {
		if len(match) > 1 {
			link := match[1]
			// Normalize: remove trailing slash, convert to slug
			link = strings.TrimSuffix(link, "/")
			if link == "" {
				link = "/" // homepage
			}
			linkSet[link] = true
		}
	}

	links := make([]string, 0, len(linkSet))
	for link := range linkSet {
		links = append(links, link)
	}
	return links
}

// saveContentVersion saves the current state of content as a new version
// If originalContent is provided and no versions exist yet, it saves the original as v1 first
func (h *Handler) saveContentVersion(ctx context.Context, content *models.Content) error {
	return h.saveContentVersionWithOriginal(ctx, content, nil, "")
}

// saveContentVersionWithOriginal saves versions with optional original state for first-time versioning
func (h *Handler) saveContentVersionWithOriginal(ctx context.Context, content *models.Content, originalContent *models.Content, comment string) error {
	// Get the current version count
	count, err := h.db.Count(ctx, "content_versions", bson.M{"content_id": content.ID})
	if err != nil {
		return err
	}

	// If no versions exist and we have the original content, save it as v1 first
	if count == 0 && originalContent != nil {
		v1 := models.ContentVersion{
			ContentID:       originalContent.ID,
			Version:         1,
			TemplateID:      originalContent.TemplateID,
			TemplateName:    originalContent.TemplateName,
			Title:           originalContent.Title,
			Slug:            originalContent.Slug,
			FolderID:        originalContent.FolderID,
			FolderPath:      originalContent.FolderPath,
			FullPath:        originalContent.FullPath,
			Category:        originalContent.Category,
			MetaDescription: originalContent.MetaDescription,
			OGImage:         originalContent.OGImage,
			Data:            originalContent.Data,
			Published:       originalContent.Published,
			PublishedAt:     originalContent.PublishedAt,
			UseHeader:       originalContent.UseHeader,
			UseFooter:       originalContent.UseFooter,
			UseTheme:        originalContent.UseTheme,
			RawMode:         originalContent.RawMode,
			CreatedAt:       originalContent.CreatedAt, // Use original creation time
		}
		if _, err := h.db.InsertOne(ctx, "content_versions", v1); err != nil {
			return err
		}
		count = 1
	}

	version := int(count) + 1

	modifiedByEmail := services.EditorEmailFromContext(ctx)
	contentVersion := models.ContentVersion{
		ContentID:       content.ID,
		Version:         version,
		Comment:         comment,
		ModifiedByEmail: modifiedByEmail,
		TemplateID:      content.TemplateID,
		TemplateName:    content.TemplateName,
		Title:           content.Title,
		Slug:            content.Slug,
		FolderID:        content.FolderID,
		FolderPath:      content.FolderPath,
		FullPath:        content.FullPath,
		Category:        content.Category,
		MetaDescription: content.MetaDescription,
		OGImage:         content.OGImage,
		Data:            content.Data,
		Published:       content.Published,
		PublishedAt:     content.PublishedAt,
		UseHeader:       content.UseHeader,
		UseFooter:       content.UseFooter,
		UseTheme:        content.UseTheme,
		RawMode:         content.RawMode,
		CreatedAt:       time.Now(),
	}

	_, err = h.db.InsertOne(ctx, "content_versions", contentVersion)
	return err
}

// extractInternalLinksFromContent extracts all internal links from a content item's data
func (h *Handler) extractInternalLinksFromContent(content *models.Content) []string {
	linkSet := make(map[string]bool)

	for _, value := range content.Data {
		if strVal, ok := value.(string); ok {
			for _, link := range extractInternalLinks(strVal) {
				linkSet[link] = true
			}
		}
	}

	links := make([]string, 0, len(linkSet))
	for link := range linkSet {
		links = append(links, link)
	}
	return links
}

// updateLinksInHTML replaces old slug references with new slug in HTML
func updateLinksInHTML(html, oldSlug, newSlug string) string {
	oldPath := "/" + oldSlug
	newPath := "/" + newSlug
	if oldSlug == "" {
		oldPath = "/"
	}
	if newSlug == "" {
		newPath = "/"
	}

	// Replace href="oldPath" with href="newPath"
	// Handle various quote styles and edge cases
	patterns := []struct {
		old string
		new string
	}{
		{fmt.Sprintf(`href="%s"`, oldPath), fmt.Sprintf(`href="%s"`, newPath)},
		{fmt.Sprintf(`href='%s'`, oldPath), fmt.Sprintf(`href='%s'`, newPath)},
		{fmt.Sprintf(`href="%s#`, oldPath), fmt.Sprintf(`href="%s#`, newPath)},
		{fmt.Sprintf(`href='%s#`, oldPath), fmt.Sprintf(`href='%s#`, newPath)},
		{fmt.Sprintf(`href="%s?`, oldPath), fmt.Sprintf(`href="%s?`, newPath)},
		{fmt.Sprintf(`href='%s?`, oldPath), fmt.Sprintf(`href='%s?`, newPath)},
	}

	result := html
	for _, p := range patterns {
		result = strings.ReplaceAll(result, p.old, p.new)
	}
	return result
}

// updateDependentContent updates all content that links to oldSlug to point to newSlug (legacy)
func (h *Handler) updateDependentContent(ctx context.Context, oldSlug, newSlug string) error {
	oldPath := "/" + oldSlug
	if oldSlug == "" {
		oldPath = "/"
	}
	newPath := "/" + newSlug
	if newSlug == "" {
		newPath = "/"
	}
	return h.updateDependentContentByPath(ctx, oldPath, newPath)
}

// updateDependentContentByPath updates all content that links to oldPath to point to newPath
func (h *Handler) updateDependentContentByPath(ctx context.Context, oldPath, newPath string) error {
	if oldPath == newPath {
		return nil
	}

	// Find all content that has this internal link
	cursor, err := h.db.FindMany(ctx, "content", bson.M{"internal_links": oldPath})
	if err != nil {
		return err
	}

	var contents []models.Content
	if err := cursor.All(ctx, &contents); err != nil {
		return err
	}

	for _, content := range contents {
		updated := false
		// Update links in all data fields
		for key, value := range content.Data {
			if strVal, ok := value.(string); ok {
				newVal := updateLinksInHTMLByPath(strVal, oldPath, newPath)
				if newVal != strVal {
					content.Data[key] = newVal
					updated = true
				}
			}
		}

		if updated {
			// Re-extract internal links after update
			content.InternalLinks = h.extractInternalLinksFromContent(&content)

			// Update in database
			if err := h.db.UpdateOne(ctx, "content", bson.M{"_id": content.ID}, bson.M{
				"$set": bson.M{
					"data":           content.Data,
					"internal_links": content.InternalLinks,
					"updated_at":     time.Now(),
				},
			}); err != nil {
				return err
			}

			// Regenerate static page if published
			if content.Published {
				var tmpl models.Template
				if err := h.db.FindOne(ctx, "templates", bson.M{"_id": content.TemplateID}, &tmpl); err == nil {
					h.generateStaticPage(ctx, &content, &tmpl)
				}
			}
		}
	}

	// Also update header and footer in theme settings
	theme, err := h.db.GetThemeSettings(ctx)
	if err == nil && theme != nil {
		themeUpdated := false

		newHeader := updateLinksInHTMLByPath(theme.HeaderHTML, oldPath, newPath)
		if newHeader != theme.HeaderHTML {
			theme.HeaderHTML = newHeader
			themeUpdated = true
		}

		newFooter := updateLinksInHTMLByPath(theme.FooterHTML, oldPath, newPath)
		if newFooter != theme.FooterHTML {
			theme.FooterHTML = newFooter
			themeUpdated = true
		}

		if themeUpdated {
			h.db.SaveThemeSettings(ctx, theme)
			// Regenerate all content since header/footer changed
			h.regenerateAllContent(ctx)
		}
	}

	return nil
}

// updateLinksInHTMLByPath replaces old path references with new path in HTML
func updateLinksInHTMLByPath(html, oldPath, newPath string) string {
	// Replace href="oldPath" with href="newPath"
	// Handle various quote styles and edge cases
	patterns := []struct {
		old string
		new string
	}{
		{fmt.Sprintf(`href="%s"`, oldPath), fmt.Sprintf(`href="%s"`, newPath)},
		{fmt.Sprintf(`href='%s'`, oldPath), fmt.Sprintf(`href='%s'`, newPath)},
		{fmt.Sprintf(`href="%s#`, oldPath), fmt.Sprintf(`href="%s#`, newPath)},
		{fmt.Sprintf(`href='%s#`, oldPath), fmt.Sprintf(`href='%s#`, newPath)},
		{fmt.Sprintf(`href="%s?`, oldPath), fmt.Sprintf(`href="%s?`, newPath)},
		{fmt.Sprintf(`href='%s?`, oldPath), fmt.Sprintf(`href='%s?`, newPath)},
	}

	result := html
	for _, p := range patterns {
		result = strings.ReplaceAll(result, p.old, p.new)
	}
	return result
}

// Public layout template
const publicLayout = `<!DOCTYPE html>
<html lang="en">
<head>
{{if .HeadHTML}}{{.HeadHTML}}{{end}}
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>{{if .PageTitle}}{{.PageTitle}}{{else}}{{.Theme.SiteName}}{{end}}</title>
    {{if .MetaDescription}}<meta name="description" content="{{.MetaDescription}}">{{end}}
    {{if .CanonicalURL}}<link rel="canonical" href="{{.CanonicalURL}}">{{end}}
    {{if .StructuredData}}{{.StructuredData}}{{end}}
    <!-- Open Graph / Social Media -->
    <meta property="og:type" content="website">
    <meta property="og:title" content="{{if .PageTitle}}{{.PageTitle}}{{else}}{{.Theme.SiteName}}{{end}}">
    {{if .MetaDescription}}<meta property="og:description" content="{{.MetaDescription}}">{{end}}
    {{if .OGImage}}<meta property="og:image" content="{{.OGImage}}">{{end}}
    <meta name="twitter:card" content="summary_large_image">
    <link rel="preconnect" href="https://fonts.googleapis.com">
    <link rel="preconnect" href="https://fonts.gstatic.com" crossorigin>
    <link href="https://fonts.googleapis.com/css2?family=Inter:wght@300;400;500;600;700&family=Space+Grotesk:wght@400;500;600;700&display=swap" rel="stylesheet">
    <link rel="stylesheet" href="/static/css/theme-vars.css">
    <link rel="stylesheet" href="/static/css/main.css">
</head>
<body>
    {{if .UseHeader}}
    <header class="site-header">
        {{if .HeaderHTML}}
        {{.HeaderHTML}}
        {{else}}
        <nav class="main-nav">
            <div class="nav-container">
                <a href="/" class="logo">{{if .Theme.LogoURL}}<img src="{{.Theme.LogoURL}}" alt="{{.Theme.SiteName}}" class="site-logo">{{else}}{{.Theme.SiteName}}{{end}}</a>
                <div class="nav-links">
                    <a href="/">Home</a>
                    <a href="/blog">Blog</a>
                </div>
            </div>
        </nav>
        {{end}}
    </header>
    {{end}}
    <main class="main-content">
        <div class="container">
            {{.Content}}
        </div>
    </main>
    {{if .UseFooter}}
    <footer class="main-footer">
        {{if .FooterHTML}}
        <div class="container">
            {{.FooterHTML}}
        </div>
        {{else}}
        <div class="container">
            <p>{{.Theme.SiteTagline}}</p>
            <p class="copyright">&copy; 2026 {{.Theme.SiteName}}. Powered by LightCMS.</p>
        </div>
        {{end}}
    </footer>
    {{end}}
</body>
</html>`

// ============================================
// Redirect Management Handlers
// ============================================

// ListRedirects shows all redirects
func (h *Handler) ListRedirects(w http.ResponseWriter, r *http.Request) {
	if !h.auth.IsAuthenticated(r) {
		http.Redirect(w, r, "/cm/login", http.StatusSeeOther)
		return
	}

	ctx := r.Context()
	cursor, err := h.db.FindMany(ctx, "redirects", bson.M{}, options.Find().SetSort(bson.D{{Key: "from_path", Value: 1}}))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	var redirects []models.Redirect
	cursor.All(ctx, &redirects)

	h.renderAdmin(w, r, "redirects_list", map[string]interface{}{
		"Redirects": redirects,
	})
}

// NewRedirect shows form for new redirect
func (h *Handler) NewRedirect(w http.ResponseWriter, r *http.Request) {
	if !h.auth.IsAuthenticated(r) {
		http.Redirect(w, r, "/cm/login", http.StatusSeeOther)
		return
	}
	h.renderAdmin(w, r, "redirect_form", map[string]interface{}{
		"IsNew": true,
	})
}

// CreateRedirect creates a new redirect
func (h *Handler) CreateRedirect(w http.ResponseWriter, r *http.Request) {
	if !h.auth.IsAuthenticated(r) {
		http.Redirect(w, r, "/cm/login", http.StatusSeeOther)
		return
	}

	fromPath := r.FormValue("from_path")
	toPath := r.FormValue("to_path")
	statusCode := 301
	if r.FormValue("status_code") == "302" {
		statusCode = 302
	}
	description := r.FormValue("description")

	// Ensure paths start with /
	if !strings.HasPrefix(fromPath, "/") {
		fromPath = "/" + fromPath
	}
	if !strings.HasPrefix(toPath, "/") && !strings.HasPrefix(toPath, "http") {
		toPath = "/" + toPath
	}

	redirect := models.Redirect{
		FromPath:    fromPath,
		ToPath:      toPath,
		StatusCode:  statusCode,
		Description: description,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	ctx := r.Context()
	newID, err := h.db.InsertOne(ctx, "redirects", redirect)
	if err != nil {
		h.renderAdmin(w, r, "redirect_form", map[string]interface{}{
			"IsNew":    true,
			"Error":    "Failed to create redirect: " + err.Error(),
			"Redirect": redirect,
		})
		return
	}
	auditResource(r, newID.Hex())

	http.Redirect(w, r, "/cm/redirects", http.StatusSeeOther)
}

// EditRedirect shows form to edit a redirect
func (h *Handler) EditRedirect(w http.ResponseWriter, r *http.Request) {
	if !h.auth.IsAuthenticated(r) {
		http.Redirect(w, r, "/cm/login", http.StatusSeeOther)
		return
	}

	vars := mux.Vars(r)
	id, err := primitive.ObjectIDFromHex(vars["id"])
	if err != nil {
		http.Error(w, "Invalid ID", http.StatusBadRequest)
		return
	}

	var redirect models.Redirect
	ctx := r.Context()
	if err := h.db.FindOne(ctx, "redirects", bson.M{"_id": id}, &redirect); err != nil {
		http.Error(w, "Redirect not found", http.StatusNotFound)
		return
	}

	h.renderAdmin(w, r, "redirect_form", map[string]interface{}{
		"IsNew":    false,
		"Redirect": redirect,
	})
}

// UpdateRedirect updates an existing redirect
func (h *Handler) UpdateRedirect(w http.ResponseWriter, r *http.Request) {
	if !h.auth.IsAuthenticated(r) {
		http.Redirect(w, r, "/cm/login", http.StatusSeeOther)
		return
	}

	vars := mux.Vars(r)
	id, err := primitive.ObjectIDFromHex(vars["id"])
	if err != nil {
		http.Error(w, "Invalid ID", http.StatusBadRequest)
		return
	}

	fromPath := r.FormValue("from_path")
	toPath := r.FormValue("to_path")
	statusCode := 301
	if r.FormValue("status_code") == "302" {
		statusCode = 302
	}
	description := r.FormValue("description")

	// Ensure paths start with /
	if !strings.HasPrefix(fromPath, "/") {
		fromPath = "/" + fromPath
	}
	if !strings.HasPrefix(toPath, "/") && !strings.HasPrefix(toPath, "http") {
		toPath = "/" + toPath
	}

	ctx := r.Context()
	update := bson.M{
		"$set": bson.M{
			"from_path":   fromPath,
			"to_path":     toPath,
			"status_code": statusCode,
			"description": description,
			"updated_at":  time.Now(),
		},
	}

	if err := h.db.UpdateOne(ctx, "redirects", bson.M{"_id": id}, update); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/cm/redirects", http.StatusSeeOther)
}

// DeleteRedirect removes a redirect
func (h *Handler) DeleteRedirect(w http.ResponseWriter, r *http.Request) {
	if !h.auth.IsAuthenticated(r) {
		http.Redirect(w, r, "/cm/login", http.StatusSeeOther)
		return
	}

	vars := mux.Vars(r)
	id, err := primitive.ObjectIDFromHex(vars["id"])
	if err != nil {
		http.Error(w, "Invalid ID", http.StatusBadRequest)
		return
	}

	ctx := r.Context()
	if err := h.db.DeleteOne(ctx, "redirects", bson.M{"_id": id}); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/cm/redirects", http.StatusSeeOther)
}

// ============================================
// Contact Form Handlers
// ============================================

// ContactFormSubmitWithConfig returns a handler that uses the provided proxy config for rate limiting
func (h *Handler) ContactFormSubmitWithConfig(proxyConfig *middleware.TrustedProxyConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		h.contactFormSubmitInternal(w, r, proxyConfig)
	}
}

// ContactFormSubmit handles public contact form submissions (legacy, uses no proxy config)
func (h *Handler) ContactFormSubmit(w http.ResponseWriter, r *http.Request) {
	h.contactFormSubmitInternal(w, r, nil)
}

// contactFormSubmitInternal is the internal implementation of contact form submission
func (h *Handler) contactFormSubmitInternal(w http.ResponseWriter, r *http.Request, proxyConfig *middleware.TrustedProxyConfig) {
	ctx := r.Context()

	// Get real client IP using trusted proxy config
	ip := middleware.GetClientIP(r, proxyConfig)

	// Check rate limit
	isLimited, err := h.db.IsContactRateLimited(ctx, ip)
	if err != nil {
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	if isLimited {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusTooManyRequests)
		w.Write([]byte(`{"success":false,"error":"Too many submissions. Please try again later."}`))
		return
	}

	// Parse form
	name := strings.TrimSpace(r.FormValue("name"))
	email := strings.TrimSpace(r.FormValue("email"))
	message := strings.TrimSpace(r.FormValue("message"))

	// Validate required fields
	if name == "" || email == "" || message == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"success":false,"error":"All fields are required."}`))
		return
	}

	// Sanitize inputs to prevent injection attacks
	// Remove any characters that could be used for mailto: injection or phishing
	email = sanitizeEmail(email)
	name = sanitizeContactInput(name)
	message = sanitizeContactInput(message)

	// Validate email format more strictly
	if !isValidEmail(email) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"success":false,"error":"Please enter a valid email address."}`))
		return
	}

	// Length limits to prevent abuse
	if len(name) > 200 {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"success":false,"error":"Name is too long (max 200 characters)."}`))
		return
	}
	if len(message) > 10000 {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"success":false,"error":"Message is too long (max 10000 characters)."}`))
		return
	}

	// Create contact message
	contact := models.ContactMessage{
		Name:      name,
		Email:     email,
		Message:   message,
		IPAddress: ip,
		UserAgent: r.UserAgent(),
		Read:      false,
		CreatedAt: time.Now(),
	}

	if _, err := h.db.InsertOne(ctx, "contact_messages", contact); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"success":false,"error":"Failed to submit message. Please try again."}`))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"success":true,"message":"Thank you! Your message has been sent."}`))
}

// ListContactMessages shows all contact messages in admin
func (h *Handler) ListContactMessages(w http.ResponseWriter, r *http.Request) {
	if !h.auth.IsAuthenticated(r) {
		http.Redirect(w, r, "/cm/login", http.StatusSeeOther)
		return
	}

	ctx := r.Context()
	cursor, err := h.db.FindMany(ctx, "contact_messages", bson.M{}, options.Find().SetSort(bson.D{{Key: "created_at", Value: -1}}))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	var messages []models.ContactMessage
	cursor.All(ctx, &messages)

	// Count unread
	unreadCount, _ := h.db.Count(ctx, "contact_messages", bson.M{"read": false})

	h.renderAdmin(w, r, "contact_messages_list", map[string]interface{}{
		"Messages":    messages,
		"UnreadCount": unreadCount,
	})
}

// ViewContactMessage shows a single contact message
func (h *Handler) ViewContactMessage(w http.ResponseWriter, r *http.Request) {
	if !h.auth.IsAuthenticated(r) {
		http.Redirect(w, r, "/cm/login", http.StatusSeeOther)
		return
	}

	vars := mux.Vars(r)
	id, err := primitive.ObjectIDFromHex(vars["id"])
	if err != nil {
		http.Error(w, "Invalid ID", http.StatusBadRequest)
		return
	}

	var message models.ContactMessage
	ctx := r.Context()
	if err := h.db.FindOne(ctx, "contact_messages", bson.M{"_id": id}, &message); err != nil {
		http.Error(w, "Message not found", http.StatusNotFound)
		return
	}

	// Mark as read
	if !message.Read {
		h.db.UpdateOne(ctx, "contact_messages", bson.M{"_id": id}, bson.M{"$set": bson.M{"read": true}})
		message.Read = true
	}

	h.renderAdmin(w, r, "contact_message_view", map[string]interface{}{
		"Message": message,
	})
}

// DeleteContactMessage removes a contact message
func (h *Handler) DeleteContactMessage(w http.ResponseWriter, r *http.Request) {
	if !h.auth.IsAuthenticated(r) {
		http.Redirect(w, r, "/cm/login", http.StatusSeeOther)
		return
	}

	vars := mux.Vars(r)
	id, err := primitive.ObjectIDFromHex(vars["id"])
	if err != nil {
		http.Error(w, "Invalid ID", http.StatusBadRequest)
		return
	}

	ctx := r.Context()
	if err := h.db.DeleteOne(ctx, "contact_messages", bson.M{"_id": id}); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/cm/messages", http.StatusSeeOther)
}

// MarkAllMessagesRead marks all contact messages as read
func (h *Handler) MarkAllMessagesRead(w http.ResponseWriter, r *http.Request) {
	if !h.auth.IsAuthenticated(r) {
		http.Redirect(w, r, "/cm/login", http.StatusSeeOther)
		return
	}
	h.db.Collection("contact_messages").UpdateMany(r.Context(), bson.M{"read": false}, bson.M{"$set": bson.M{"read": true}}) //nolint:errcheck
	http.Redirect(w, r, "/cm/messages", http.StatusSeeOther)
}

// GetUnreadMessageCount returns the number of unread contact messages (for dashboard)
func (h *Handler) GetUnreadMessageCount(ctx context.Context) int64 {
	count, _ := h.db.Count(ctx, "contact_messages", bson.M{"read": false})
	return count
}

// ==================== Asset Library Handlers ====================

// AssetLibrary displays the asset library listing
func (h *Handler) AssetLibrary(w http.ResponseWriter, r *http.Request) {
	if !h.auth.IsAuthenticated(r) {
		http.Redirect(w, r, "/cm/login", http.StatusSeeOther)
		return
	}

	ctx := r.Context()
	folder := r.URL.Query().Get("folder")

	assets, err := h.db.ListAssets(ctx, folder)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	folders, err := h.db.GetAssetFolders(ctx)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	h.renderAdmin(w, r, "asset_library", map[string]interface{}{
		"Assets":        assets,
		"Folders":       folders,
		"CurrentFolder": folder,
	})
}

// AssetUploadForm shows the upload form
func (h *Handler) AssetUploadForm(w http.ResponseWriter, r *http.Request) {
	if !h.auth.IsAuthenticated(r) {
		http.Redirect(w, r, "/cm/login", http.StatusSeeOther)
		return
	}

	ctx := r.Context()
	folders, err := h.db.GetAssetFolders(ctx)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	h.renderAdmin(w, r, "asset_upload", map[string]interface{}{
		"Folders": folders,
	})
}

// AssetUpload handles file upload
func (h *Handler) AssetUpload(w http.ResponseWriter, r *http.Request) {
	if !h.auth.IsAuthenticated(r) {
		http.Redirect(w, r, "/cm/login", http.StatusSeeOther)
		return
	}

	maxBytes := h.uploadMaxBytes(r.Context())
	r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
	if err := r.ParseMultipartForm(maxBytes); err != nil {
		http.Error(w, fmt.Sprintf("File too large (max %s)", formatBytes(maxBytes)), http.StatusBadRequest)
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "No file uploaded", http.StatusBadRequest)
		return
	}
	defer file.Close()

	// Validate file extension first (before reading the entire file)
	ext := strings.ToLower(filepath.Ext(header.Filename))
	if !middleware.IsAllowedFileType(ext) {
		http.Error(w, "File type not allowed. Allowed types: images, PDFs, web assets (CSS, JS, fonts)", http.StatusBadRequest)
		return
	}

	// Read file data (bounded to configured max upload size)
	data, err := io.ReadAll(io.LimitReader(file, maxBytes))
	if err != nil {
		h.errors.HTTPError(w, err, http.StatusInternalServerError)
		return
	}

	// Detect MIME type and validate it matches the extension
	detectedMIME := http.DetectContentType(data)
	if !middleware.ValidateMIMEType(ext, detectedMIME) {
		log.Printf("MIME type mismatch: extension=%s, detected=%s", ext, detectedMIME)
		http.Error(w, "File content does not match its extension", http.StatusBadRequest)
		return
	}

	// Get serve path from form
	servePath := r.FormValue("serve_path")
	if servePath == "" {
		servePath = "/" + header.Filename
	}
	// Ensure serve path starts with /
	if !strings.HasPrefix(servePath, "/") {
		servePath = "/" + servePath
	}

	// SECURITY: Validate path doesn't contain traversal attempts
	if !middleware.ValidateFilePath(servePath) {
		http.Error(w, "Invalid file path", http.StatusBadRequest)
		return
	}

	// Clean the path
	servePath = filepath.Clean(servePath)
	if !strings.HasPrefix(servePath, "/") {
		servePath = "/" + servePath
	}

	// SECURITY: Double-check after cleaning that we're still safe
	if strings.Contains(servePath, "..") {
		http.Error(w, "Invalid file path", http.StatusBadRequest)
		return
	}

	// Get filename from serve path
	filename := filepath.Base(servePath)

	// Get folder from serve path
	folder := filepath.Dir(servePath)
	if folder == "." {
		folder = "/"
	}

	// Determine MIME type - use detected type but override for known extensions
	mimeType := detectedMIME
	switch ext {
	case ".svg":
		mimeType = "image/svg+xml"
	case ".css":
		mimeType = "text/css"
	case ".js":
		mimeType = "application/javascript"
	case ".json":
		mimeType = "application/json"
	case ".ico":
		mimeType = "image/x-icon"
	case ".webp":
		mimeType = "image/webp"
	case ".woff":
		mimeType = "font/woff"
	case ".woff2":
		mimeType = "font/woff2"
	case ".ttf":
		mimeType = "font/ttf"
	}

	description := r.FormValue("description")

	// SECURITY: Build the static path and verify it's within the allowed directory
	baseDir, _ := filepath.Abs("content/generated")
	staticPath := filepath.Join("content/generated", servePath)
	absStaticPath, err := filepath.Abs(staticPath)
	if err != nil {
		h.errors.HTTPError(w, err, http.StatusInternalServerError)
		return
	}
	// Ensure the resolved path is within the base directory
	if !strings.HasPrefix(absStaticPath, baseDir) {
		log.Printf("Path traversal attempt blocked: %s not within %s", absStaticPath, baseDir)
		http.Error(w, "Invalid file path", http.StatusBadRequest)
		return
	}

	staticDir := filepath.Dir(staticPath)
	if err := os.MkdirAll(staticDir, 0755); err != nil {
		h.errors.HTTPError(w, err, http.StatusInternalServerError)
		return
	}
	if err := os.WriteFile(staticPath, data, 0644); err != nil {
		h.errors.HTTPError(w, err, http.StatusInternalServerError)
		return
	}

	ctx := r.Context()
	asset := &database.Asset{
		Filename:    filename,
		Folder:      folder,
		FullPath:    servePath, // Keep for backwards compat
		ServePath:   servePath,
		MimeType:    mimeType,
		Size:        int64(len(data)),
		Data:        nil, // Don't store binary data in DB anymore
		Description: description,
	}

	if err := h.db.SaveAsset(ctx, asset); err != nil {
		// Clean up the file if DB save fails
		os.Remove(staticPath)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	// SaveAsset upserts by path and does not return the id: the path
	// identifies the asset in the audit entry.
	auditDetail(r, "serve_path", servePath)

	http.Redirect(w, r, "/cm/assets", http.StatusSeeOther)
}

// ServeAsset serves an asset file by path (legacy /assets/ path support)
func (h *Handler) ServeAsset(w http.ResponseWriter, r *http.Request) {
	// Get full path from URL
	fullPath := r.URL.Path
	if !strings.HasPrefix(fullPath, "/assets") {
		http.NotFound(w, r)
		return
	}
	// Remove /assets prefix to get the actual path
	assetPath := strings.TrimPrefix(fullPath, "/assets")
	if assetPath == "" {
		assetPath = "/"
	}

	// First check for static file (new style)
	// Try without /assets prefix first, then with it (handles assets
	// uploaded before the prefix-stripping normalization was added)
	staticFilePath := filepath.Join("content/generated", assetPath)
	if info, err := os.Stat(staticFilePath); err == nil && !info.IsDir() {
		w.Header().Set("Cache-Control", "no-cache")
		http.ServeFile(w, r, staticFilePath)
		return
	}
	staticFilePathFull := filepath.Join("content/generated", fullPath)
	if info, err := os.Stat(staticFilePathFull); err == nil && !info.IsDir() {
		w.Header().Set("Cache-Control", "no-cache")
		http.ServeFile(w, r, staticFilePathFull)
		return
	}

	// Fall back to database (legacy: assets stored with binary data in DB)
	ctx := r.Context()
	// Try lookup with stripped path first, then with full URL path
	// (handles both old assets stored without /assets prefix and
	// newer assets that may have been stored with it)
	asset, err := h.db.GetAssetByPath(ctx, assetPath)
	if (err != nil || asset == nil) && assetPath != fullPath {
		asset, err = h.db.GetAssetByPath(ctx, fullPath)
	}
	if err != nil || asset == nil {
		http.NotFound(w, r)
		return
	}

	// Legacy assets stored binary data in the DB; modern assets are on disk only.
	if len(asset.Data) == 0 {
		http.NotFound(w, r)
		return
	}

	// Set headers
	w.Header().Set("Content-Type", asset.MimeType)
	w.Header().Set("Content-Length", fmt.Sprintf("%d", asset.Size))
	w.Header().Set("Cache-Control", "no-cache")

	w.Write(asset.Data)
}

// DeleteAsset removes an asset
func (h *Handler) DeleteAsset(w http.ResponseWriter, r *http.Request) {
	if !h.auth.IsAuthenticated(r) {
		http.Redirect(w, r, "/cm/login", http.StatusSeeOther)
		return
	}

	vars := mux.Vars(r)
	id, err := primitive.ObjectIDFromHex(vars["id"])
	if err != nil {
		http.Error(w, "Invalid ID", http.StatusBadRequest)
		return
	}

	ctx := r.Context()

	// Get asset first to know the file path
	asset, err := h.db.GetAsset(ctx, id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Delete the static file if it exists
	if asset != nil && asset.ServePath != "" {
		staticPath := filepath.Join("content/generated", asset.ServePath)
		os.Remove(staticPath) // Ignore errors, file may not exist
	}

	if err := h.db.DeleteAsset(ctx, id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/cm/assets", http.StatusSeeOther)
}

// ==================== Sitemap Generation ====================

// sitemapFile is where the generated sitemap is written and read, relative
// to the working directory. A variable so the tests point it at a temp
// directory instead of rewriting a tracked file.
var sitemapFile = "static/sitemap.xml"

// GenerateSitemap creates/updates the sitemap.xml file
func (h *Handler) GenerateSitemap(ctx context.Context, baseURL string) error {
	// Get all published content
	// Live pages only: soft-deleted pages and fork copies (which share their
	// live page's path) don't belong in the sitemap.
	cursor, err := h.db.FindMany(ctx, "content", bson.M{"published": true, "deleted": bson.M{"$ne": true}, "fork_id": nil, "noindex": bson.M{"$ne": true}},
		options.Find().SetProjection(bson.M{"full_path": 1, "slug": 1, "category": 1, "updated_at": 1, "content_modified_at": 1}))
	if err != nil {
		return err
	}
	defer cursor.Close(ctx)

	var contents []models.Content
	if err := cursor.All(ctx, &contents); err != nil {
		return err
	}

	// Get all collections
	collCursor, err := h.db.FindMany(ctx, "collections", bson.M{})
	if err != nil {
		return err
	}
	defer collCursor.Close(ctx)

	var collections []models.Collection
	if err := collCursor.All(ctx, &collections); err != nil {
		return err
	}

	// Build sitemap XML following Google guidelines
	var sb strings.Builder
	sb.WriteString(`<?xml version="1.0" encoding="UTF-8"?>
<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
`)

	// Add homepage
	sb.WriteString(fmt.Sprintf(`  <url>
    <loc>%s/</loc>
    <changefreq>daily</changefreq>
    <priority>1.0</priority>
  </url>
`, baseURL))

	// Add content pages
	for _, content := range contents {
		path := content.FullPath
		if path == "" {
			path = "/" + content.Slug
		}
		if path == "/" {
			continue // Skip homepage, already added
		}

		// Determine priority based on content type
		priority := "0.7"
		changefreq := "weekly"
		if content.Category == "blog" {
			priority = "0.6"
			changefreq = "monthly"
		}

		lastmod := content.ModifiedAt().Format("2006-01-02")

		sb.WriteString(fmt.Sprintf(`  <url>
    <loc>%s%s</loc>
    <lastmod>%s</lastmod>
    <changefreq>%s</changefreq>
    <priority>%s</priority>
  </url>
`, baseURL, path, lastmod, changefreq, priority))
	}

	// Add collection pages
	for _, coll := range collections {
		sb.WriteString(fmt.Sprintf(`  <url>
    <loc>%s/%s</loc>
    <changefreq>weekly</changefreq>
    <priority>0.8</priority>
  </url>
`, baseURL, coll.Slug))
	}

	sb.WriteString(`</urlset>
`)

	// Write to static file
	return os.WriteFile(sitemapFile, []byte(sb.String()), 0644)
}

// ==================== Tools ====================

// BrokenLinkFinder scans all content for broken internal links
func (h *Handler) BrokenLinkFinder(w http.ResponseWriter, r *http.Request) {
	if !h.auth.IsAuthenticated(r) {
		http.Redirect(w, r, "/cm/login", http.StatusSeeOther)
		return
	}

	// Render the page - scanning happens via SSE
	h.renderAdmin(w, r, "broken_links", nil)
}

// BrokenLinkScan performs the actual scan with Server-Sent Events for real-time progress
func (h *Handler) BrokenLinkScan(w http.ResponseWriter, r *http.Request) {
	user, ok := h.auth.GetCurrentUser(r)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	// The scan fetches every external link in published content from this
	// server: it needs the permission the broken-link fix needs.
	if !auth.HasPermission(user.Role, auth.PermContentEdit) {
		adminJSONError(w, http.StatusForbidden, forbiddenMessage([]string{auth.PermContentEdit}))
		return
	}

	// Set headers for SSE
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no") // Disable nginx buffering

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming not supported", http.StatusInternalServerError)
		return
	}

	// Use background context for database operations to avoid cancellation
	ctx := context.Background()
	clientCtx := r.Context()

	// Helper to check if client is still connected
	clientDisconnected := func() bool {
		select {
		case <-clientCtx.Done():
			return true
		default:
			return false
		}
	}

	// Helper to send SSE event
	sendEvent := func(eventType string, data string) bool {
		if clientDisconnected() {
			return false
		}
		fmt.Fprintf(w, "event: %s\ndata: %s\n\n", eventType, data)
		flusher.Flush()
		return true
	}

	// Start a goroutine to send keep-alive comments every 5 seconds
	// This prevents Fly.io proxy from timing out on the SSE connection
	done := make(chan bool)
	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				// Send SSE comment (lines starting with : are comments)
				fmt.Fprintf(w, ": keepalive\n\n")
				flusher.Flush()
			}
		}
	}()
	defer func() {
		select {
		case done <- true:
		default:
		}
	}()

	// Get all published, non-deleted content
	cursor, err := h.db.FindMany(ctx, "content", bson.M{
		"published": true,
		"deleted":   bson.M{"$ne": true},
	}, nil)
	if err != nil {
		sendEvent("error", err.Error())
		return
	}

	var allContent []models.Content
	if err := cursor.All(ctx, &allContent); err != nil {
		sendEvent("error", err.Error())
		return
	}

	totalPages := len(allContent)
	sendEvent("total", fmt.Sprintf(`{"total": %d}`, totalPages))

	// Build a set of valid paths (published content and collections)
	validPaths := make(map[string]bool)
	validPaths["/"] = true

	for _, c := range allContent {
		path := c.FullPath
		if path == "" {
			path = "/" + c.Slug
		}
		validPaths[path] = true
	}

	// Also add collection paths
	collCursor, err := h.db.FindMany(ctx, "collections", bson.M{}, nil)
	if err == nil {
		var collections []models.Collection
		if collCursor.All(ctx, &collections) == nil {
			for _, coll := range collections {
				validPaths["/"+coll.Slug] = true
			}
		}
	}

	// Load all redirects to check if internal paths have redirects
	redirectPaths := make(map[string]bool)
	redirectCursor, err := h.db.FindMany(ctx, "redirects", bson.M{}, nil)
	if err == nil {
		var redirects []models.Redirect
		if redirectCursor.All(ctx, &redirects) == nil {
			for _, redir := range redirects {
				redirectPaths[redir.FromPath] = true
			}
		}
	}

	// Extract links from href attributes
	linkPattern := regexp.MustCompile(`href=["']([^"']+)["']`)

	type BrokenLinkResult struct {
		URL    string `json:"url"`
		Field  string `json:"field"`
		Status int    `json:"status,omitempty"`
		Error  string `json:"error,omitempty"`
	}

	type PageResult struct {
		ID          string             `json:"id"`
		Title       string             `json:"title"`
		Path        string             `json:"path"`
		BrokenLinks []BrokenLinkResult `json:"brokenLinks"`
	}

	// HTTP client with short timeout for external link checking
	// Use 3 seconds to prevent blocking the SSE stream too long.
	// The URLs come from page content, so the client dials through the SSRF
	// guard: a link (or a redirect) that resolves to a loopback, private or
	// link-local address is never contacted.
	httpClient := &http.Client{
		Transport: ssrfSafeClient.Transport,
		Timeout:   5 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			// Allow up to 10 redirects (sites like anchor.fm can have 4+ redirects)
			if len(via) >= 10 {
				return fmt.Errorf("too many redirects")
			}
			return nil
		},
	}

	// Cache for external link results to avoid checking same URL multiple times
	externalLinkCache := make(map[string]*BrokenLinkResult)

	// Browser-like User-Agent (many sites block non-browser UAs)
	browserUA := "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"

	// Helper to check if an external URL is broken
	checkExternalURL := func(urlStr string) *BrokenLinkResult {
		// Check if client disconnected
		if clientDisconnected() {
			return nil
		}

		// Check cache first
		if cached, ok := externalLinkCache[urlStr]; ok {
			return cached
		}

		// Use GET directly - many sites (Substack, Medium) return 404 for HEAD but work with GET
		req, err := http.NewRequest("GET", urlStr, nil)
		if err != nil {
			result := &BrokenLinkResult{URL: urlStr, Error: "invalid URL"}
			externalLinkCache[urlStr] = result
			return result
		}
		req.Header.Set("User-Agent", browserUA)
		req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
		req.Header.Set("Accept-Language", "en-US,en;q=0.5")

		resp, err := httpClient.Do(req)
		if err != nil {
			// Simplify error message
			errMsg := err.Error()
			if strings.Contains(errMsg, "timeout") || strings.Contains(errMsg, "deadline") {
				errMsg = "timeout"
			} else if strings.Contains(errMsg, "no such host") {
				errMsg = "host not found"
			} else if strings.Contains(errMsg, "connection refused") {
				errMsg = "connection refused"
			} else if strings.Contains(errMsg, "certificate") {
				errMsg = "SSL certificate error"
			} else if len(errMsg) > 50 {
				errMsg = errMsg[:50] + "..."
			}
			result := &BrokenLinkResult{URL: urlStr, Error: errMsg}
			externalLinkCache[urlStr] = result
			return result
		}

		// Read and discard only first 1KB to avoid blocking on large responses
		// We only care about the status code
		io.CopyN(io.Discard, resp.Body, 1024)
		resp.Body.Close()

		// Consider 4xx and 5xx as broken (except 403 which might be blocking bots)
		// Also accept 401 as valid (requires auth but page exists)
		if resp.StatusCode >= 400 && resp.StatusCode != 401 && resp.StatusCode != 403 {
			result := &BrokenLinkResult{URL: urlStr, Status: resp.StatusCode}
			externalLinkCache[urlStr] = result
			return result
		}

		// Link is OK
		externalLinkCache[urlStr] = nil
		return nil
	}

	var results []PageResult
	totalBrokenLinks := 0
	totalLinksChecked := 0

	for i, content := range allContent {
		// Check if client disconnected
		if clientDisconnected() {
			return
		}

		path := content.FullPath
		if path == "" {
			path = "/" + content.Slug
		}

		// Send progress update
		progressData := fmt.Sprintf(`{"current": %d, "total": %d, "path": %q, "title": %q, "linksChecked": %d}`,
			i+1, totalPages, path, content.Title, totalLinksChecked)
		if !sendEvent("progress", progressData) {
			return // Client disconnected
		}

		var brokenLinks []BrokenLinkResult

		// Check all data fields
		for fieldName, value := range content.Data {
			if strVal, ok := value.(string); ok {
				matches := linkPattern.FindAllStringSubmatch(strVal, -1)
				for _, match := range matches {
					if len(match) >= 2 {
						href := match[1]

						// Skip mailto, tel, javascript, and anchor-only links
						if strings.HasPrefix(href, "mailto:") ||
							strings.HasPrefix(href, "tel:") ||
							strings.HasPrefix(href, "javascript:") ||
							strings.HasPrefix(href, "#") {
							continue
						}

						// Check internal links (starting with /)
						if strings.HasPrefix(href, "/") && !strings.HasPrefix(href, "//") {
							// Remove query string and fragment
							cleanPath := href
							if idx := strings.Index(cleanPath, "?"); idx != -1 {
								cleanPath = cleanPath[:idx]
							}
							if idx := strings.Index(cleanPath, "#"); idx != -1 {
								cleanPath = cleanPath[:idx]
							}

							// Skip static assets and special paths
							if strings.HasPrefix(cleanPath, "/static/") ||
								strings.HasPrefix(cleanPath, "/uploads/") ||
								strings.HasPrefix(cleanPath, "/assets/") ||
								strings.HasPrefix(cleanPath, "/cm/") ||
								strings.HasPrefix(cleanPath, "/api/") ||
								cleanPath == "/sitemap.xml" ||
								cleanPath == "/robots.txt" {
								continue
							}

							totalLinksChecked++

							// Check if path exists in content OR has a redirect
							if !validPaths[cleanPath] && !redirectPaths[cleanPath] {
								brokenLinks = append(brokenLinks, BrokenLinkResult{
									URL:   href,
									Field: fieldName,
								})
							}
						} else if strings.HasPrefix(href, "http://") || strings.HasPrefix(href, "https://") || strings.HasPrefix(href, "//") {
							// External link - check with HTTP request
							totalLinksChecked++

							// Normalize // to https://
							checkURL := href
							if strings.HasPrefix(href, "//") {
								checkURL = "https:" + href
							}

							// Send checking status
							checkData := fmt.Sprintf(`{"current": %d, "total": %d, "path": %q, "title": %q, "checking": %q, "linksChecked": %d}`,
								i+1, totalPages, path, content.Title, checkURL, totalLinksChecked)
							if !sendEvent("progress", checkData) {
								return // Client disconnected
							}

							if result := checkExternalURL(checkURL); result != nil {
								result.Field = fieldName
								brokenLinks = append(brokenLinks, *result)
							}
						}
					}
				}
			}
		}

		if len(brokenLinks) > 0 {
			pageResult := PageResult{
				ID:          content.ID.Hex(),
				Title:       content.Title,
				Path:        path,
				BrokenLinks: brokenLinks,
			}
			results = append(results, pageResult)
			totalBrokenLinks += len(brokenLinks)

			// Send found broken links for this page immediately
			resultJSON, _ := json.Marshal(pageResult)
			if !sendEvent("result", string(resultJSON)) {
				return // Client disconnected
			}
		}
	}

	// Send completion event
	completeData := fmt.Sprintf(`{"totalPages": %d, "pagesWithBrokenLinks": %d, "totalBrokenLinks": %d, "totalLinksChecked": %d}`,
		totalPages, len(results), totalBrokenLinks, totalLinksChecked)
	sendEvent("complete", completeData)
}

// FixBrokenLink handles fixing a broken link in content
func (h *Handler) FixBrokenLink(w http.ResponseWriter, r *http.Request) {
	user, ok := h.auth.GetCurrentUser(r)
	if !ok {
		adminJSONError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}
	if !auth.HasPermission(user.Role, auth.PermContentEdit) {
		adminJSONError(w, http.StatusForbidden, "Fixing links requires permission to edit content.")
		return
	}

	var req struct {
		ContentID string `json:"contentId"`
		Field     string `json:"field"`
		OldURL    string `json:"oldUrl"`
		NewURL    string `json:"newUrl"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "Invalid request body"})
		return
	}

	ctx := r.Context()

	// Get the content
	objectID, err := primitive.ObjectIDFromHex(req.ContentID)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "Invalid content ID"})
		return
	}

	var content models.Content
	if err := h.db.FindOne(ctx, "content", bson.M{"_id": objectID}, &content); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "Content not found"})
		return
	}
	auditResource(r, objectID.Hex())

	// Store original content for versioning
	originalContent := content

	// Find and replace the link in the specified field
	fieldValue, ok := content.Data[req.Field]
	if !ok {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "Field not found"})
		return
	}

	fieldStr, ok := fieldValue.(string)
	if !ok {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "Field is not a string"})
		return
	}

	// Replace the old URL with the new one in href attributes
	// We need to be careful to only replace in href="..." or href='...'
	var newFieldStr string
	if req.NewURL == "" {
		// Remove the link entirely - find <a href="oldUrl">...</a> and replace with just the inner content
		// This is a simplified approach - for complex cases, user should use the full editor
		linkPattern := regexp.MustCompile(`<a[^>]*href=["']` + regexp.QuoteMeta(req.OldURL) + `["'][^>]*>(.*?)</a>`)
		newFieldStr = linkPattern.ReplaceAllString(fieldStr, "$1")
	} else {
		// Replace the URL
		oldPattern := regexp.MustCompile(`(href=["'])` + regexp.QuoteMeta(req.OldURL) + `(["'])`)
		newFieldStr = oldPattern.ReplaceAllString(fieldStr, "${1}"+req.NewURL+"${2}")
	}

	if newFieldStr == fieldStr {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "Link not found in field"})
		return
	}

	// Update the content
	content.Data[req.Field] = newFieldStr
	content.UpdatedAt = time.Now()

	// Save version first (using the same logic as UpdateContent)
	if err := h.saveContentVersionWithOriginal(ctx, &content, &originalContent, "Link updated via bulk update"); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "Failed to save version: " + err.Error()})
		return
	}

	// Update the content in the database
	if err := h.db.UpdateOne(ctx, "content", bson.M{"_id": objectID}, bson.M{
		"$set": bson.M{
			"data":       content.Data,
			"updated_at": content.UpdatedAt,
		},
	}); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "Failed to update content"})
		return
	}

	// Get the version number for the response
	versionCount, _ := h.db.Count(ctx, "content_versions", bson.M{"content_id": content.ID})

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"version": versionCount,
	})
}

// ServeSitemap serves the sitemap.xml file
// sitemapMaxAge bounds how stale the generated sitemap.xml may get.
const sitemapMaxAge = 10 * time.Minute

func (h *Handler) ServeSitemap(w http.ResponseWriter, r *http.Request) {
	h.recordCrawler(r, "/sitemap.xml")
	// Serve the generated file, regenerating it when missing or stale. Only
	// admin-UI edits trigger RegenerateSitemap, so API/MCP-driven changes
	// would otherwise never reach the sitemap.
	data, err := os.ReadFile(sitemapFile)
	if info, statErr := os.Stat(sitemapFile); statErr == nil && time.Since(info.ModTime()) > sitemapMaxAge {
		err = os.ErrNotExist
	}
	if err != nil {
		// Generate on the fly if file doesn't exist
		baseURL := h.baseURL
		if baseURL == "" {
			baseURL = "https://" + r.Host
			if r.TLS == nil {
				baseURL = "http://" + r.Host
			}
		}
		if err := h.GenerateSitemap(r.Context(), baseURL); err != nil {
			http.Error(w, "Failed to generate sitemap", http.StatusInternalServerError)
			return
		}
		data, _ = os.ReadFile(sitemapFile)
	}

	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=3600") // Cache for 1 hour
	w.Write(data)
}

// RegenerateSitemap is called after content changes to update sitemap
func (h *Handler) RegenerateSitemap(ctx context.Context) {
	baseURL := h.baseURL
	if baseURL == "" {
		baseURL = "http://localhost:8082" // Fallback for development
	}
	h.GenerateSitemap(ctx, baseURL)
}

// ==================== Content Search ====================

// SearchContent handles AJAX search requests for content
func (h *Handler) SearchContent(w http.ResponseWriter, r *http.Request) {
	if !h.auth.IsAuthenticated(r) {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	ctx := r.Context()
	query := r.URL.Query().Get("q")
	searchType := r.URL.Query().Get("type") // "name", "slug", or "fulltext"
	includeDeleted := r.URL.Query().Get("deleted") == "true"

	// Sanitize and limit query length
	if query != "" {
		query = sanitizeContactInput(query)
		if len(query) > 200 {
			query = query[:200]
		}
	}

	// Build the base filter
	var filter bson.M
	// Sort order: slug mode sorts alphabetically by slug; others sort by updated_at desc
	sortOpt := options.Find().SetLimit(100).SetSort(bson.D{{Key: "updated_at", Value: -1}})

	if query == "" {
		// Empty query - return all content
		filter = bson.M{}
	} else if searchType == "slug" {
		// Slug-only search: match slug or full_path by regex.
		// The homepage (empty slug) is always prepended after the query runs.
		orClauses := []bson.M{
			{"slug": bson.M{"$regex": query, "$options": "i"}},
			{"full_path": bson.M{"$regex": query, "$options": "i"}},
		}
		filter = bson.M{"$or": orClauses}
		sortOpt = options.Find().SetLimit(100).SetSort(bson.D{{Key: "slug", Value: 1}})
	} else if searchType == "fulltext" {
		// Search in title, slug, full_path, and all data fields using $or and $regex
		orClauses := []bson.M{
			{"title": bson.M{"$regex": query, "$options": "i"}},
			{"slug": bson.M{"$regex": query, "$options": "i"}},
			{"full_path": bson.M{"$regex": query, "$options": "i"}},
			{"data": bson.M{"$regex": query, "$options": "i"}},
		}
		// "/" query should also match the homepage (stored with empty full_path and slug)
		if strings.HasPrefix(query, "/") {
			orClauses = append(orClauses, bson.M{"full_path": "", "slug": ""})
		}
		filter = bson.M{"$or": orClauses}
	} else {
		// Default to name/title search — also match slug and full_path
		orClauses := []bson.M{
			{"title": bson.M{"$regex": query, "$options": "i"}},
			{"slug": bson.M{"$regex": query, "$options": "i"}},
			{"full_path": bson.M{"$regex": query, "$options": "i"}},
		}
		// "/" query should also match the homepage (stored with empty full_path and slug)
		if strings.HasPrefix(query, "/") {
			orClauses = append(orClauses, bson.M{"full_path": "", "slug": ""})
		}
		filter = bson.M{"$or": orClauses}
	}

	// Add deleted filter
	if !includeDeleted {
		filter["deleted"] = bson.M{"$ne": true}
	}

	// Always exclude fork copies
	filter["fork_id"] = bson.M{"$exists": false}

	cursor, err := h.db.FindMany(ctx, "content", filter, sortOpt)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	var results []models.Content
	if err := cursor.All(ctx, &results); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// If fulltext search didn't find in Data field using regex on the whole object,
	// we need to do a manual search through Data fields
	if query != "" && searchType == "fulltext" && len(results) == 0 {
		// Fallback: get all content and search manually
		fallbackFilter := bson.M{"fork_id": nil}
		if !includeDeleted {
			fallbackFilter["deleted"] = bson.M{"$ne": true}
		}
		allCursor, err := h.db.FindMany(ctx, "content", fallbackFilter, options.Find().SetSort(bson.D{{Key: "updated_at", Value: -1}}))
		if err == nil {
			var allContent []models.Content
			if err := allCursor.All(ctx, &allContent); err == nil {
				lowerQuery := strings.ToLower(query)
				for _, c := range allContent {
					// Check title, slug, full_path
					if strings.Contains(strings.ToLower(c.Title), lowerQuery) ||
						strings.Contains(strings.ToLower(c.Slug), lowerQuery) ||
						strings.Contains(strings.ToLower(c.FullPath), lowerQuery) {
						results = append(results, c)
						continue
					}
					// Check each data field
					for _, v := range c.Data {
						if strVal, ok := v.(string); ok {
							if strings.Contains(strings.ToLower(strVal), lowerQuery) {
								results = append(results, c)
								break
							}
						}
					}
					if len(results) >= 100 {
						break
					}
				}
			}
		}
	}

	// Always pin the homepage (full_path="" slug="") to position 0.
	// For slug-mode searches MongoDB null-slug pages can sort before it, so we
	// explicitly fetch and prepend the homepage if it isn't already first.
	isHomepage := func(c models.Content) bool {
		return c.FullPath == "/" || (c.FullPath == "" && c.Slug == "")
	}
	if len(results) == 0 || !isHomepage(results[0]) {
		// Check if homepage is already somewhere in results and move it, or fetch it.
		found := false
		for i, c := range results {
			if isHomepage(c) {
				results = append([]models.Content{results[i]}, append(results[:i], results[i+1:]...)...)
				found = true
				break
			}
		}
		if !found {
			// Homepage wasn't returned by the query — fetch it directly.
			var homepage models.Content
			hpFilter := bson.M{"full_path": "", "slug": "", "deleted": bson.M{"$ne": true}, "fork_id": nil}
			if err := h.db.FindOne(ctx, "content", hpFilter, &homepage); err == nil {
				results = append([]models.Content{homepage}, results...)
			}
		}
	}

	// Build JSON response
	type SearchResult struct {
		ID           string `json:"id"`
		Title        string `json:"title"`
		TemplateName string `json:"template_name"`
		FullPath     string `json:"full_path"`
		Slug         string `json:"slug"`
		Published    bool   `json:"published"`
		Hold         bool   `json:"hold,omitempty"`
		Deleted      bool   `json:"deleted"`
		UpdatedAt    string `json:"updated_at"`
	}

	searchResults := make([]SearchResult, 0, len(results))
	for _, c := range results {
		path := c.FullPath
		if path == "" {
			path = "/" + c.Slug
		}
		searchResults = append(searchResults, SearchResult{
			ID:           c.ID.Hex(),
			Title:        c.Title,
			TemplateName: c.TemplateName,
			FullPath:     path,
			Slug:         c.Slug,
			Published:    c.Published,
			Hold:         c.Hold,
			Deleted:      c.Deleted,
			UpdatedAt:    c.UpdatedAt.Format("Jan 2, 2006"),
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(searchResults)
}

// CheckSlug checks if a content path already exists (for slug validation)
func (h *Handler) CheckSlug(w http.ResponseWriter, r *http.Request) {
	if !h.auth.IsAuthenticated(r) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]string{"error": "Unauthorized"})
		return
	}

	ctx := r.Context()
	path := r.URL.Query().Get("path")
	excludeID := r.URL.Query().Get("exclude") // Optional: exclude current content by ID

	if path == "" {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]bool{"exists": false})
		return
	}

	// Build filter to find content at this path. Live pages only: a fork
	// copy shares its live page's path and is not "a page at this path".
	filter := bson.M{
		"full_path": path,
		"deleted":   bson.M{"$ne": true},
		"fork_id":   nil,
	}

	// Exclude specific content ID if provided (for editing). When the page
	// being edited is a fork copy, the question is asked inside its fork.
	if excludeID != "" {
		if oid, err := primitive.ObjectIDFromHex(excludeID); err == nil {
			filter["_id"] = bson.M{"$ne": oid}
			var editing models.Content
			if err := h.db.FindOne(ctx, "content", bson.M{"_id": oid}, &editing); err == nil {
				filter["fork_id"] = pathScope(editing.ForkID)
			}
		}
	}

	var existingContent models.Content
	err := h.db.FindOne(ctx, "content", filter, &existingContent)

	w.Header().Set("Content-Type", "application/json")
	if err != nil {
		// No content found at this path
		json.NewEncoder(w).Encode(map[string]bool{"exists": false})
	} else {
		// Content exists at this path
		json.NewEncoder(w).Encode(map[string]interface{}{
			"exists": true,
			"title":  existingContent.Title,
		})
	}
}

// pathScope is the fork_id condition for "is there another page at this
// path": among live pages when the page being saved is live (nil matches a
// missing or null fork_id), inside its own fork when it is a fork copy. A
// fork copy shares its live page's path, so an unscoped check lets a copy
// block a live page's slug and a live page block a copy's.
func pathScope(forkID *primitive.ObjectID) interface{} {
	if forkID == nil {
		return nil
	}
	return *forkID
}

// requireSearchReplace gates the admin search-and-replace endpoints on the
// permission the API endpoints require (search_replace.execute, admin only):
// 401 without a session, 403 with a JSON error for any other role.
func (h *Handler) requireSearchReplace(w http.ResponseWriter, r *http.Request) (*auth.SessionUser, bool) {
	user, ok := h.auth.GetCurrentUser(r)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return nil, false
	}
	if !auth.HasPermission(user.Role, auth.PermSearchReplace) {
		adminJSONError(w, http.StatusForbidden, "Search and replace is limited to administrators.")
		return nil, false
	}
	return user, true
}

// adminJSONError writes {"error": msg} with the given status, the shape the
// admin UI's fetch() callers read.
func adminJSONError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

// ReplacePreview returns a preview of search and replace results
func (h *Handler) ReplacePreview(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.requireSearchReplace(w, r); !ok {
		return
	}

	ctx := r.Context()
	searchQuery := r.URL.Query().Get("search")
	replaceQuery := r.URL.Query().Get("replace")

	if searchQuery == "" {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"matches": []interface{}{}})
		return
	}

	// Live, non-deleted pages only: fork copies are never searched or rewritten.
	cursor, err := h.contentService.StreamContent(ctx, false)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	var allContent []models.Content
	if err := cursor.All(ctx, &allContent); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	sort.SliceStable(allContent, func(i, j int) bool { return allContent[i].Title < allContent[j].Title })

	type MatchResult struct {
		ID         string   `json:"id"`
		Title      string   `json:"title"`
		FullPath   string   `json:"full_path"`
		MatchCount int      `json:"match_count"`
		Excerpts   []string `json:"excerpts"`
	}

	var matches []MatchResult

	for _, content := range allContent {
		matchCount := 0
		var excerpts []string

		// Search in all data fields
		for fieldName, value := range content.Data {
			if strVal, ok := value.(string); ok {
				count := strings.Count(strVal, searchQuery)
				if count > 0 {
					matchCount += count
					// Generate excerpts with highlighted replacements (max 3 per field)
					fieldExcerpts := generateReplaceExcerpts(strVal, searchQuery, replaceQuery, 3)
					for _, exc := range fieldExcerpts {
						excerpts = append(excerpts, fmt.Sprintf("<strong>%s:</strong> %s", fieldName, exc))
					}
				}
			}
		}

		// Also check title
		if strings.Contains(content.Title, searchQuery) {
			titleCount := strings.Count(content.Title, searchQuery)
			matchCount += titleCount
			excerpts = append([]string{fmt.Sprintf("<strong>title:</strong> %s",
				generateReplaceExcerpt(content.Title, searchQuery, replaceQuery))}, excerpts...)
		}

		if matchCount > 0 {
			path := content.FullPath
			if path == "" {
				path = "/" + content.Slug
			}
			// Limit excerpts to 5 total
			if len(excerpts) > 5 {
				excerpts = excerpts[:5]
			}
			matches = append(matches, MatchResult{
				ID:         content.ID.Hex(),
				Title:      content.Title,
				FullPath:   path,
				MatchCount: matchCount,
				Excerpts:   excerpts,
			})
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"matches": matches,
		"search":  searchQuery,
		"replace": replaceQuery,
	})
}

// generateReplaceExcerpts generates multiple excerpts showing before/after replacement
func generateReplaceExcerpts(text, search, replace string, maxExcerpts int) []string {
	var excerpts []string
	remaining := text
	offset := 0

	for i := 0; i < maxExcerpts; i++ {
		idx := strings.Index(remaining, search)
		if idx == -1 {
			break
		}

		// Calculate absolute position
		absIdx := offset + idx

		// Get context around the match (50 chars before and after)
		start := absIdx - 50
		if start < 0 {
			start = 0
		}
		end := absIdx + len(search) + 50
		if end > len(text) {
			end = len(text)
		}

		// Build excerpt with highlighting
		excerpt := ""
		if start > 0 {
			excerpt += "..."
		}

		// Text before match
		beforeMatch := text[start:absIdx]
		// The match itself
		matchText := text[absIdx : absIdx+len(search)]
		// Text after match
		afterMatch := text[absIdx+len(search) : end]

		excerpt += escapeHTMLForExcerpt(beforeMatch)
		excerpt += `<span class="replace-old">` + escapeHTMLForExcerpt(matchText) + `</span>`
		excerpt += `<span class="replace-new">` + escapeHTMLForExcerpt(replace) + `</span>`
		excerpt += escapeHTMLForExcerpt(afterMatch)

		if end < len(text) {
			excerpt += "..."
		}

		excerpts = append(excerpts, excerpt)

		// Move past this match
		offset = absIdx + len(search)
		remaining = text[offset:]
	}

	return excerpts
}

// generateReplaceExcerpt generates a single excerpt for short text like titles
func generateReplaceExcerpt(text, search, replace string) string {
	idx := strings.Index(text, search)
	if idx == -1 {
		return escapeHTMLForExcerpt(text)
	}

	before := text[:idx]
	after := text[idx+len(search):]

	return escapeHTMLForExcerpt(before) +
		`<span class="replace-old">` + escapeHTMLForExcerpt(search) + `</span>` +
		`<span class="replace-new">` + escapeHTMLForExcerpt(replace) + `</span>` +
		escapeHTMLForExcerpt(after)
}

// escapeHTMLForExcerpt escapes HTML special characters for display
func escapeHTMLForExcerpt(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, "\"", "&quot;")
	// Truncate very long strings
	if len(s) > 200 {
		s = s[:200] + "..."
	}
	return s
}

// ReplaceExecute performs the actual search and replace on content. It runs
// through the service layer like the API endpoint: live pages only, a saved
// version per page, change provenance, and static regeneration for published
// pages (which stay published).
func (h *Handler) ReplaceExecute(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireSearchReplace(w, r)
	if !ok {
		return
	}

	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var request struct {
		Search  string `json:"search"`
		Replace string `json:"replace"`
	}

	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"error": "Invalid request body"})
		return
	}

	if request.Search == "" {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"error": "Search query is required"})
		return
	}

	ctx := services.WithEditorEmail(r.Context(), user.Email)
	ctx = services.WithProvenance(ctx, services.Provenance{Actor: "human", Via: "ui"})

	// Live, non-deleted pages only: fork copies are never searched or rewritten.
	cursor, err := h.contentService.StreamContent(ctx, false)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	var allContent []models.Content
	if err := cursor.All(ctx, &allContent); err != nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	versionComment := fmt.Sprintf("Bulk search and replace: '%s' → '%s'", request.Search, request.Replace)
	updatedCount := 0
	totalReplacements := 0

	for _, content := range allContent {
		needsUpdate := false
		pageReplacements := 0

		newData := make(map[string]interface{})
		for k, v := range content.Data {
			newData[k] = v
		}

		// Replace in all data fields
		for fieldName, value := range content.Data {
			if strVal, ok := value.(string); ok {
				if strings.Contains(strVal, request.Search) {
					pageReplacements += strings.Count(strVal, request.Search)
					newData[fieldName] = strings.ReplaceAll(strVal, request.Search, request.Replace)
					needsUpdate = true
				}
			}
		}

		// Replace in title
		newTitle := content.Title
		if strings.Contains(content.Title, request.Search) {
			pageReplacements += strings.Count(content.Title, request.Search)
			newTitle = strings.ReplaceAll(content.Title, request.Search, request.Replace)
			needsUpdate = true
		}

		if needsUpdate {
			content.Title = newTitle
			content.Data = newData

			unlock := lockContent(content.ID.Hex())
			err := h.contentService.UpdateContent(ctx, &content, versionComment)
			unlock()
			if err != nil {
				fmt.Printf("Warning: search and replace skipped %s: %v\n", content.ID.Hex(), err)
				continue // Skip this one but continue with others
			}

			updatedCount++
			totalReplacements += pageReplacements
		}
	}

	// Regenerate sitemap if content was updated
	if updatedCount > 0 {
		go h.RegenerateSitemap(context.Background())
	}

	// Audit the run under the action the API endpoint logs
	if h.auditService != nil {
		uid, _ := primitive.ObjectIDFromHex(user.ID)
		h.auditService.LogAsync(models.AuditLog{
			UserID: uid, UserEmail: user.Email,
			Action: "content.search_replace", Resource: "content",
			Details: map[string]interface{}{
				"pairs_count":   1,
				"pages_updated": updatedCount, "total_replacements": totalReplacements,
			},
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":       true,
		"updated_count": updatedCount,
	})
}

// ==================== Security Helpers ====================

// sanitizeEmail removes characters that could be used for mailto: injection
// Only allows alphanumeric, @, ., -, _, +
func sanitizeEmail(email string) string {
	var result strings.Builder
	for _, r := range email {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') ||
			r == '@' || r == '.' || r == '-' || r == '_' || r == '+' {
			result.WriteRune(r)
		}
	}
	return result.String()
}

// isValidEmail performs stricter email validation
func isValidEmail(email string) bool {
	// Must have exactly one @
	atCount := strings.Count(email, "@")
	if atCount != 1 {
		return false
	}

	// Split into local and domain parts
	parts := strings.Split(email, "@")
	local, domain := parts[0], parts[1]

	// Both parts must be non-empty
	if len(local) == 0 || len(domain) == 0 {
		return false
	}

	// Domain must have at least one dot
	if !strings.Contains(domain, ".") {
		return false
	}

	// Domain cannot start or end with a dot
	if strings.HasPrefix(domain, ".") || strings.HasSuffix(domain, ".") {
		return false
	}

	// Check for double dots
	if strings.Contains(email, "..") {
		return false
	}

	// Length checks
	if len(local) > 64 || len(domain) > 253 || len(email) > 254 {
		return false
	}

	return true
}

// sanitizeContactInput removes potentially dangerous characters from user input
// Allows most text but removes control characters and null bytes
func sanitizeContactInput(input string) string {
	var result strings.Builder
	for _, r := range input {
		// Skip control characters (except newline, tab, carriage return)
		if r < 32 && r != '\n' && r != '\t' && r != '\r' {
			continue
		}
		// Skip null bytes and other problematic characters
		if r == 0 {
			continue
		}
		result.WriteRune(r)
	}
	return result.String()
}

// ── Snippets ──────────────────────────────────────────────────────────────────

func (h *Handler) ListSnippets(w http.ResponseWriter, r *http.Request) {
	if !h.auth.IsAuthenticated(r) {
		http.Redirect(w, r, "/cm/login", http.StatusFound)
		return
	}
	snippets, err := h.snippetService.ListSnippets(r.Context())
	if err != nil {
		h.errors.HTTPError(w, err, http.StatusInternalServerError)
		return
	}
	h.renderAdmin(w, r, "snippets_list", map[string]interface{}{
		"Title":    "Snippets",
		"Snippets": snippets,
	})
}

func (h *Handler) NewSnippet(w http.ResponseWriter, r *http.Request) {
	if !h.auth.IsAuthenticated(r) {
		http.Redirect(w, r, "/cm/login", http.StatusFound)
		return
	}
	h.renderAdmin(w, r, "snippet_form", map[string]interface{}{
		"Title": "New Snippet",
	})
}

func (h *Handler) CreateSnippet(w http.ResponseWriter, r *http.Request) {
	if !h.auth.IsAuthenticated(r) {
		http.Redirect(w, r, "/cm/login", http.StatusFound)
		return
	}
	name := strings.TrimSpace(r.FormValue("name"))
	html := r.FormValue("html")
	if name == "" {
		h.renderAdmin(w, r, "snippet_form", map[string]interface{}{
			"Title": "New Snippet",
			"Error": "Name is required",
		})
		return
	}
	snip, err := h.snippetService.CreateSnippet(r.Context(), name, html)
	if err != nil {
		h.renderAdmin(w, r, "snippet_form", map[string]interface{}{
			"Title": "New Snippet",
			"Error": err.Error(),
		})
		return
	}
	auditResource(r, snip.ID.Hex())
	http.Redirect(w, r, "/cm/snippets", http.StatusFound)
}

func (h *Handler) EditSnippet(w http.ResponseWriter, r *http.Request) {
	if !h.auth.IsAuthenticated(r) {
		http.Redirect(w, r, "/cm/login", http.StatusFound)
		return
	}
	vars := mux.Vars(r)
	id, err := primitive.ObjectIDFromHex(vars["id"])
	if err != nil {
		http.NotFound(w, r)
		return
	}
	snip, err := h.snippetService.GetSnippet(r.Context(), id)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	h.renderAdmin(w, r, "snippet_form", map[string]interface{}{
		"Title":   "Edit Snippet",
		"Snippet": snip,
	})
}

func (h *Handler) UpdateSnippet(w http.ResponseWriter, r *http.Request) {
	if !h.auth.IsAuthenticated(r) {
		http.Redirect(w, r, "/cm/login", http.StatusFound)
		return
	}
	vars := mux.Vars(r)
	id, err := primitive.ObjectIDFromHex(vars["id"])
	if err != nil {
		http.NotFound(w, r)
		return
	}
	name := strings.TrimSpace(r.FormValue("name"))
	html := r.FormValue("html")
	if name == "" {
		snip, _ := h.snippetService.GetSnippet(r.Context(), id)
		h.renderAdmin(w, r, "snippet_form", map[string]interface{}{
			"Title":   "Edit Snippet",
			"Snippet": snip,
			"Error":   "Name is required",
		})
		return
	}
	if _, err := h.snippetService.UpdateSnippet(r.Context(), id, name, html); err != nil {
		snip, _ := h.snippetService.GetSnippet(r.Context(), id)
		h.renderAdmin(w, r, "snippet_form", map[string]interface{}{
			"Title":   "Edit Snippet",
			"Snippet": snip,
			"Error":   err.Error(),
		})
		return
	}
	// Regenerate all published pages — any page may include this snippet.
	if h.contentService != nil {
		go h.contentService.RegenerateAllContent(context.Background())
	}
	http.Redirect(w, r, "/cm/snippets", http.StatusFound)
}

func (h *Handler) DeleteSnippet(w http.ResponseWriter, r *http.Request) {
	if !h.auth.IsAuthenticated(r) {
		http.Redirect(w, r, "/cm/login", http.StatusFound)
		return
	}
	vars := mux.Vars(r)
	id, err := primitive.ObjectIDFromHex(vars["id"])
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err := h.snippetService.DeleteSnippet(r.Context(), id); err != nil {
		h.errors.HTTPError(w, err, http.StatusInternalServerError)
		return
	}
	// Regenerate all published pages — any page may have included this snippet.
	if h.contentService != nil {
		go h.contentService.RegenerateAllContent(context.Background())
	}
	http.Redirect(w, r, "/cm/snippets", http.StatusFound)
}

// ApprovalsPage renders the approvals dashboard.
// GET /cm/approvals
func (h *Handler) ApprovalsPage(w http.ResponseWriter, r *http.Request) {
	if !h.auth.IsAuthenticated(r) {
		http.Redirect(w, r, "/cm/login", http.StatusSeeOther)
		return
	}
	if h.approvalService == nil {
		h.renderAdmin(w, r, "approvals_page", map[string]interface{}{
			"MyQueue":      nil,
			"OtherPending": nil,
			"Workflows":    nil,
		})
		return
	}
	ctx := r.Context()
	user, _ := h.auth.GetCurrentUser(r)

	var myQueue, otherPending []models.ApprovalRequest
	allPending, err := h.approvalService.ListPending(ctx)
	if err == nil && user != nil {
		userID, _ := primitive.ObjectIDFromHex(user.ID)
		mine, _ := h.approvalService.ListMyQueue(ctx, userID)
		myQueue = mine
		myIDs := make(map[string]bool)
		for _, req := range mine {
			myIDs[req.ID.Hex()] = true
		}
		for _, req := range allPending {
			if !myIDs[req.ID.Hex()] {
				otherPending = append(otherPending, req)
			}
		}
	} else {
		otherPending = allPending
	}

	workflows, _ := h.approvalService.ListWorkflows(ctx)

	h.renderAdmin(w, r, "approvals_page", map[string]interface{}{
		"MyQueue":      myQueue,
		"OtherPending": otherPending,
		"Workflows":    workflows,
	})
}
