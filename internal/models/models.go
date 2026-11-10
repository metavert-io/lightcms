package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// TemplateField defines a dynamic field in a content template
type TemplateField struct {
	Name        string `bson:"name" json:"name"`
	Label       string `bson:"label" json:"label"`
	Type        string `bson:"type" json:"type"` // text, textarea, richtext, date, image, select
	Required    bool   `bson:"required" json:"required"`
	Placeholder string `bson:"placeholder,omitempty" json:"placeholder,omitempty"`
	Options     string `bson:"options,omitempty" json:"options,omitempty"` // comma-separated for select type
	Default     string `bson:"default,omitempty" json:"default,omitempty"`
}

// Template defines a content structure template
type Template struct {
	ID          primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	Name        string             `bson:"name" json:"name"`
	Slug        string             `bson:"slug" json:"slug"`
	Description string             `bson:"description" json:"description"`
	Fields      []TemplateField    `bson:"fields" json:"fields"`
	HTMLLayout  string             `bson:"html_layout" json:"html_layout"` // HTML template with {{.FieldName}} placeholders
	Category    string             `bson:"category" json:"category"`       // For grouping content
	IsSystem    bool               `bson:"is_system" json:"is_system"`     // Built-in templates can't be deleted
	CreatedAt   time.Time          `bson:"created_at" json:"created_at"`
	UpdatedAt   time.Time          `bson:"updated_at" json:"updated_at"`
}

// Content represents a content item created from a template
type Content struct {
	ID              primitive.ObjectID     `bson:"_id,omitempty" json:"id"`
	TemplateID      primitive.ObjectID     `bson:"template_id" json:"template_id"`
	TemplateName    string                 `bson:"template_name" json:"template_name"`
	Title           string                 `bson:"title" json:"title"`
	Slug            string                 `bson:"slug" json:"slug"`                               // Just the page slug, not including folder path
	FolderID        *primitive.ObjectID    `bson:"folder_id,omitempty" json:"folder_id,omitempty"` // nil for root level
	FolderPath      string                 `bson:"folder_path" json:"folder_path"`                 // Cached folder path for quick lookups
	FullPath        string                 `bson:"full_path" json:"full_path"`                     // Complete URL path (folder + slug)
	Category        string                 `bson:"category" json:"category"`
	Tags            []string               `bson:"tags,omitempty" json:"tags,omitempty"`     // Multi-value tags for lc:query filtering
	MetaDescription string                 `bson:"meta_description" json:"meta_description"` // SEO meta description
	OGImage         string                 `bson:"og_image" json:"og_image"`                 // Open Graph image URL
	Data            map[string]interface{} `bson:"data" json:"data"`                         // Dynamic field values
	Published       bool                   `bson:"published" json:"published"`
	PublishedAt     *time.Time             `bson:"published_at,omitempty" json:"published_at,omitempty"`
	UseHeader       bool                   `bson:"use_header" json:"use_header"`
	UseFooter       bool                   `bson:"use_footer" json:"use_footer"`
	UseTheme        bool                   `bson:"use_theme" json:"use_theme"`                               // Whether to wrap in site theme/layout
	RawMode         bool                   `bson:"raw_mode" json:"raw_mode"`                                 // True = raw HTML, False = rich editor
	InternalLinks   []string               `bson:"internal_links,omitempty" json:"internal_links,omitempty"` // Tracks internal links (slugs) in this content
	ContentHash     string                 `bson:"content_hash,omitempty" json:"-"`                          // SHA-256 of last generated static HTML
	Embedding       []float32              `bson:"embedding,omitempty" json:"-"`                             // Voyage AI vector embedding (1024 dims)
	EmbeddingAt     *time.Time             `bson:"embedding_at,omitempty" json:"-"`                          // When embedding was last generated
	PlainText       string                 `bson:"plain_text,omitempty" json:"-"`                            // Cached stripped-HTML text for search snippets
	PublishAt       *time.Time             `bson:"publish_at,omitempty" json:"publish_at,omitempty"`         // Scheduled publish time
	Deleted         bool                   `bson:"deleted" json:"deleted"`                                   // Soft delete flag
	DeletedAt       *time.Time             `bson:"deleted_at,omitempty" json:"deleted_at,omitempty"`
	// Set by the service's DeleteContent when it unpublishes a published page, so RestoreContent can publish it again (v7.4.4)
	PublishedBeforeDelete bool   `bson:"published_before_delete,omitempty" json:"-"`
	SourceURL             string `bson:"source_url,omitempty" json:"source_url,omitempty"` // Original URL for imported content (RSS dedup key)
	// SEO (v7.3): per-page search/AI visibility and authorship
	NoIndex           bool       `bson:"noindex,omitempty" json:"noindex,omitempty"`                         // Hide from search engines & AI: noindex meta, excluded from sitemap/llms.txt/feeds/IndexNow
	AuthorName        string     `bson:"author_name,omitempty" json:"author_name,omitempty"`                 // Overrides the site default author in structured data and feeds
	AuthorURL         string     `bson:"author_url,omitempty" json:"author_url,omitempty"`                   // Author profile URL
	ContentModifiedAt *time.Time `bson:"content_modified_at,omitempty" json:"content_modified_at,omitempty"` // Last time the rendered page actually changed (falls back to UpdatedAt)
	// Hold (v7.4): a held draft cannot be published by any path until the flag is cleared
	Hold bool `bson:"hold,omitempty" json:"hold,omitempty"`
	// Fork fields — set when this content item belongs to a fork workspace
	ForkID        *primitive.ObjectID `bson:"fork_id,omitempty" json:"fork_id,omitempty"`                 // nil for live content
	BaseUpdatedAt *time.Time          `bson:"base_updated_at,omitempty" json:"base_updated_at,omitempty"` // updated_at of the live page at fork time (for conflict detection)
	// Approval fields — set when a Contributor submits content for review
	PendingApproval bool      `bson:"pending_approval,omitempty" json:"pending_approval,omitempty"`
	CreatedAt       time.Time `bson:"created_at" json:"created_at"`
	UpdatedAt       time.Time `bson:"updated_at" json:"updated_at"`
}

// ModifiedAt returns when the page's rendered content last actually changed:
// content_modified_at when known (set only when the generated HTML changes),
// otherwise updated_at.
func (c *Content) ModifiedAt() time.Time {
	if c.ContentModifiedAt != nil && !c.ContentModifiedAt.IsZero() {
		return *c.ContentModifiedAt
	}
	return c.UpdatedAt
}

// ContentFork represents a named workspace for staging site changes before merging to live
type ContentFork struct {
	ID             primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	Name           string             `bson:"name" json:"name"`
	Description    string             `bson:"description" json:"description"`
	Status         string             `bson:"status" json:"status"`               // "active" | "merged" | "archived"
	PreviewToken   string             `bson:"preview_token" json:"preview_token"` // random token for cookie-based preview
	CreatedBy      primitive.ObjectID `bson:"created_by" json:"created_by"`
	CreatedByEmail string             `bson:"created_by_email" json:"created_by_email"`
	// CreatedByCredential is the API key or OAuth connection that created the
	// fork ("apikey:<id>" / "oauth:<grant id>"); empty for a person in the
	// admin UI and for forks made before 7.5.0.
	CreatedByCredential string              `bson:"created_by_credential,omitempty" json:"created_by_credential,omitempty"`
	CreatedAt           time.Time           `bson:"created_at" json:"created_at"`
	MergedAt            *time.Time          `bson:"merged_at,omitempty" json:"merged_at,omitempty"`
	MergedBy            *primitive.ObjectID `bson:"merged_by,omitempty" json:"merged_by,omitempty"`
	MergedByEmail       string              `bson:"merged_by_email,omitempty" json:"merged_by_email,omitempty"`
	ArchivedAt          *time.Time          `bson:"archived_at,omitempty" json:"archived_at,omitempty"`
	// SubmittedAt is when the fork was handed off for review (ending an
	// agent sandbox with "submit"). From then on no API key or connection
	// can change it; a person reviews, merges or archives it.
	SubmittedAt      *time.Time `bson:"submitted_at,omitempty" json:"submitted_at,omitempty"`
	SubmittedByEmail string     `bson:"submitted_by_email,omitempty" json:"submitted_by_email,omitempty"`
	// Page counts recorded at merge time (v7.4). The fork's page copies are
	// deleted after a successful merge, so these are the history of what it did.
	MergedCreated int `bson:"merged_created,omitempty" json:"merged_created,omitempty"`
	MergedUpdated int `bson:"merged_updated,omitempty" json:"merged_updated,omitempty"`
}

// ContentVersion represents a historical version of content
type ContentVersion struct {
	ID              primitive.ObjectID  `bson:"_id,omitempty" json:"id"`
	ContentID       primitive.ObjectID  `bson:"content_id" json:"content_id"`                                   // Reference to the content item
	Version         int                 `bson:"version" json:"version"`                                         // Version number (1, 2, 3...)
	Comment         string              `bson:"comment,omitempty" json:"comment,omitempty"`                     // Optional version comment
	ModifiedBy      *primitive.ObjectID `bson:"modified_by,omitempty" json:"modified_by,omitempty"`             // User who made this version
	ModifiedByEmail string              `bson:"modified_by_email,omitempty" json:"modified_by_email,omitempty"` // Denormalized email
	// Provenance: who or what authored this version
	Actor           string                 `bson:"actor,omitempty" json:"actor,omitempty"`                 // "human" | "agent"
	Via             string                 `bson:"via,omitempty" json:"via,omitempty"`                     // "ui" | "api" | "copilot"
	AgentSession    string                 `bson:"agent_session,omitempty" json:"agent_session,omitempty"` // agent session ID when Actor == "agent"
	TemplateID      primitive.ObjectID     `bson:"template_id" json:"template_id"`
	TemplateName    string                 `bson:"template_name" json:"template_name"`
	Title           string                 `bson:"title" json:"title"`
	Slug            string                 `bson:"slug" json:"slug"`
	FolderID        *primitive.ObjectID    `bson:"folder_id,omitempty" json:"folder_id,omitempty"`
	FolderPath      string                 `bson:"folder_path" json:"folder_path"`
	FullPath        string                 `bson:"full_path" json:"full_path"`
	Category        string                 `bson:"category" json:"category"`
	Tags            []string               `bson:"tags,omitempty" json:"tags,omitempty"`
	MetaDescription string                 `bson:"meta_description" json:"meta_description"`
	OGImage         string                 `bson:"og_image" json:"og_image"`
	Data            map[string]interface{} `bson:"data" json:"data"`
	Published       bool                   `bson:"published" json:"published"`
	PublishedAt     *time.Time             `bson:"published_at,omitempty" json:"published_at,omitempty"`
	UseHeader       bool                   `bson:"use_header" json:"use_header"`
	UseFooter       bool                   `bson:"use_footer" json:"use_footer"`
	UseTheme        bool                   `bson:"use_theme" json:"use_theme"`
	RawMode         bool                   `bson:"raw_mode" json:"raw_mode"`
	// Kept since 7.5 (nil in older versions, which leave the page's value
	// alone when reverted to).
	NoIndex    *bool     `bson:"noindex,omitempty" json:"noindex,omitempty"`
	AuthorName *string   `bson:"author_name,omitempty" json:"author_name,omitempty"`
	AuthorURL  *string   `bson:"author_url,omitempty" json:"author_url,omitempty"`
	CreatedAt  time.Time `bson:"created_at" json:"created_at"` // When this version was created (saved)
}

// Folder represents a directory in the site structure
type Folder struct {
	ID        primitive.ObjectID  `bson:"_id,omitempty" json:"id"`
	Name      string              `bson:"name" json:"name"`                               // Display name
	Slug      string              `bson:"slug" json:"slug"`                               // URL segment (just the folder name, not full path)
	ParentID  *primitive.ObjectID `bson:"parent_id,omitempty" json:"parent_id,omitempty"` // nil for root-level folders
	Path      string              `bson:"path" json:"path"`                               // Full path like /blog/2024
	CreatedAt time.Time           `bson:"created_at" json:"created_at"`
	UpdatedAt time.Time           `bson:"updated_at" json:"updated_at"`
}

// Collection represents a content collection (e.g., all blog posts)
type Collection struct {
	ID           primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	Name         string             `bson:"name" json:"name"`
	Slug         string             `bson:"slug" json:"slug"`
	Description  string             `bson:"description" json:"description"`
	Category     string             `bson:"category" json:"category"`           // Filter content by this category
	SortField    string             `bson:"sort_field" json:"sort_field"`       // Field to sort by
	SortOrder    string             `bson:"sort_order" json:"sort_order"`       // asc or desc
	ItemTemplate string             `bson:"item_template" json:"item_template"` // HTML template for each item
	PageTemplate string             `bson:"page_template" json:"page_template"` // HTML template for the collection page
	ItemsPerPage int                `bson:"items_per_page" json:"items_per_page"`
	CreatedAt    time.Time          `bson:"created_at" json:"created_at"`
	UpdatedAt    time.Time          `bson:"updated_at" json:"updated_at"`
}

// APIKey represents an API key for REST API access
type APIKey struct {
	ID          primitive.ObjectID  `bson:"_id,omitempty" json:"id"`
	Name        string              `bson:"name" json:"name"`
	Description string              `bson:"description" json:"description"`
	Prefix      string              `bson:"prefix" json:"prefix"`                                 // First 11 chars (e.g., "lc_a1b2c3d4") for identification
	KeyHash     string              `bson:"key_hash" json:"-"`                                    // SHA-256 hash of full key
	UserID      *primitive.ObjectID `bson:"user_id,omitempty" json:"user_id,omitempty"`           // Owning user
	Scopes      []string            `bson:"scopes,omitempty" json:"scopes,omitempty"`             // Permission allowlist; empty = full owner permissions
	SandboxOnly bool                `bson:"sandbox_only,omitempty" json:"sandbox_only,omitempty"` // Content writes must target a fork the key created; live mutations rejected
	// CanMergeOwnForks lets the key merge forks it created into the live
	// site. Only a signed-in admin can turn it on.
	CanMergeOwnForks bool `bson:"can_merge_own_forks,omitempty" json:"can_merge_own_forks,omitempty"`
	// ActiveSandboxFork is the fork the key's current sandbox writes to.
	ActiveSandboxFork *primitive.ObjectID `bson:"active_sandbox_fork,omitempty" json:"active_sandbox_fork,omitempty"`
	LastUsedAt        *time.Time          `bson:"last_used_at,omitempty" json:"last_used_at,omitempty"`
	CreatedAt         time.Time           `bson:"created_at" json:"created_at"`
}

// Snippet is a named, reusable HTML fragment used as an item renderer in lc:query directives.
// The HTML is a Go text/template with access to content fields (.Title, .FullPath, .Tags, .Data, etc.).
type Snippet struct {
	ID        primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	Name      string             `bson:"name" json:"name"` // Unique name referenced in <!-- lc:query snippet="name" -->
	HTML      string             `bson:"html" json:"html"` // Go template HTML rendered per result item
	CreatedAt time.Time          `bson:"created_at" json:"created_at"`
	UpdatedAt time.Time          `bson:"updated_at" json:"updated_at"`
}

// ContactMessage represents a contact form submission or system message
type ContactMessage struct {
	ID        primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	Name      string             `bson:"name" json:"name"`             // Sender name (or "LightCMS" for system messages)
	Email     string             `bson:"email" json:"email"`           // Sender email (empty for system messages)
	Subject   string             `bson:"subject" json:"subject"`       // Optional subject line
	Message   string             `bson:"message" json:"message"`       // Message body (can contain HTML for system messages)
	IPAddress string             `bson:"ip_address" json:"ip_address"` // Empty for system messages
	UserAgent string             `bson:"user_agent" json:"user_agent"` // Empty for system messages
	IsSystem  bool               `bson:"is_system" json:"is_system"`   // True for system-generated messages
	Read      bool               `bson:"read" json:"read"`
	CreatedAt time.Time          `bson:"created_at" json:"created_at"`
}

// Redirect represents a URL redirect rule
type Redirect struct {
	ID          primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	FromPath    string             `bson:"from_path" json:"from_path"`     // Source path (e.g., /old-page)
	ToPath      string             `bson:"to_path" json:"to_path"`         // Destination path (e.g., /new-page)
	StatusCode  int                `bson:"status_code" json:"status_code"` // 301 or 302
	Description string             `bson:"description" json:"description"` // Optional note
	CreatedAt   time.Time          `bson:"created_at" json:"created_at"`
	UpdatedAt   time.Time          `bson:"updated_at" json:"updated_at"`
}

// Asset represents a file stored in the asset library
type Asset struct {
	ID             primitive.ObjectID  `bson:"_id,omitempty" json:"id"`
	Filename       string              `bson:"filename" json:"filename"`       // Original filename
	Folder         string              `bson:"folder" json:"folder"`           // Folder path (e.g., /images, /documents)
	FullPath       string              `bson:"full_path" json:"full_path"`     // Complete path (folder + filename)
	MimeType       string              `bson:"mime_type" json:"mime_type"`     // MIME type (e.g., image/png)
	Size           int64               `bson:"size" json:"size"`               // File size in bytes
	Data           []byte              `bson:"data" json:"data"`               // File binary data
	Description    string              `bson:"description" json:"description"` // Optional description
	UploadedByID   *primitive.ObjectID `bson:"uploaded_by_id,omitempty" json:"uploaded_by_id,omitempty"`
	UploadedByRole string              `bson:"uploaded_by_role,omitempty" json:"uploaded_by_role,omitempty"`
	PendingReview  bool                `bson:"pending_review,omitempty" json:"pending_review,omitempty"` // true when uploaded by a contributor
	CreatedAt      time.Time           `bson:"created_at" json:"created_at"`
	UpdatedAt      time.Time           `bson:"updated_at" json:"updated_at"`
}

// Default templates
var DefaultTemplates = []Template{
	{
		Name:        "Blog Post",
		Slug:        "blog-post",
		Description: "Standard blog post with title, featured image, and content",
		Category:    "blog",
		IsSystem:    true,
		Fields: []TemplateField{
			{Name: "title", Label: "Title", Type: "text", Required: true, Placeholder: "Enter post title"},
			{Name: "excerpt", Label: "Excerpt", Type: "textarea", Required: false, Placeholder: "Brief summary of the post"},
			{Name: "featured_image", Label: "Featured Image", Type: "image", Required: false},
			{Name: "content", Label: "Content", Type: "richtext", Required: true},
			{Name: "author", Label: "Author", Type: "text", Required: false, Placeholder: "Author name"},
			{Name: "tags", Label: "Tags", Type: "text", Required: false, Placeholder: "Comma-separated tags"},
		},
		HTMLLayout: `<article class="blog-post">
	<header class="post-header">
		{{if .featured_image}}<img src="{{.featured_image}}" alt="{{.title}}" class="featured-image">{{end}}
		<h1 class="post-title">{{.title}}</h1>
		<div class="post-meta">
			{{if .author}}<span class="author">By {{.author}}</span>{{end}}
			<time class="date">{{.published_at}}</time>
		</div>
	</header>
	{{if .excerpt}}<p class="excerpt">{{.excerpt}}</p>{{end}}
	<div class="post-content">{{.content}}</div>
	{{if .tags}}<div class="tags">{{.tags}}</div>{{end}}
</article>`,
	},
	{
		Name:        "Press Release",
		Slug:        "press-release",
		Description: "Official press release format with headline, dateline, and body",
		Category:    "press",
		IsSystem:    true,
		Fields: []TemplateField{
			{Name: "headline", Label: "Headline", Type: "text", Required: true, Placeholder: "Press release headline"},
			{Name: "subheadline", Label: "Subheadline", Type: "text", Required: false, Placeholder: "Optional subheadline"},
			{Name: "dateline", Label: "Dateline", Type: "text", Required: true, Placeholder: "e.g., NEW YORK, NY"},
			{Name: "release_date", Label: "Release Date", Type: "date", Required: true},
			{Name: "body", Label: "Body", Type: "richtext", Required: true},
			{Name: "boilerplate", Label: "About/Boilerplate", Type: "richtext", Required: false},
			{Name: "contact_info", Label: "Media Contact", Type: "textarea", Required: false, Placeholder: "Contact information"},
		},
		HTMLLayout: `<article class="press-release">
	<header class="press-header">
		<div class="press-label">PRESS RELEASE</div>
		<h1 class="headline">{{.headline}}</h1>
		{{if .subheadline}}<h2 class="subheadline">{{.subheadline}}</h2>{{end}}
		<div class="dateline"><strong>{{.dateline}}</strong> — <time>{{.release_date}}</time></div>
	</header>
	<div class="press-body">{{.body}}</div>
	{{if .boilerplate}}
	<section class="boilerplate">
		<h3>About</h3>
		{{.boilerplate}}
	</section>
	{{end}}
	{{if .contact_info}}
	<section class="contact">
		<h3>Media Contact</h3>
		<pre>{{.contact_info}}</pre>
	</section>
	{{end}}
</article>`,
	},
	{
		Name:        "Explanatory Page",
		Slug:        "explanatory-page",
		Description: "Informational page with sections for explaining concepts or features",
		Category:    "pages",
		IsSystem:    true,
		Fields: []TemplateField{
			{Name: "title", Label: "Page Title", Type: "text", Required: true, Placeholder: "Page title"},
			{Name: "subtitle", Label: "Subtitle", Type: "text", Required: false, Placeholder: "Optional subtitle"},
			{Name: "hero_image", Label: "Hero Image", Type: "image", Required: false},
			{Name: "intro", Label: "Introduction", Type: "richtext", Required: false},
			{Name: "main_content", Label: "Main Content", Type: "richtext", Required: true},
			{Name: "sidebar", Label: "Sidebar Content", Type: "richtext", Required: false},
			{Name: "cta_text", Label: "Call to Action Text", Type: "text", Required: false, Placeholder: "e.g., Learn More"},
			{Name: "cta_link", Label: "Call to Action Link", Type: "text", Required: false, Placeholder: "/contact"},
		},
		HTMLLayout: `<article class="explanatory-page">
	{{if .hero_image}}<div class="hero" style="background-image: url('{{.hero_image}}')"></div>{{end}}
	<header class="page-header">
		<h1>{{.title}}</h1>
		{{if .subtitle}}<p class="subtitle">{{.subtitle}}</p>{{end}}
	</header>
	{{if .intro}}<div class="intro">{{.intro}}</div>{{end}}
	<div class="content-wrapper">
		<main class="main-content">{{.main_content}}</main>
		{{if .sidebar}}<aside class="sidebar">{{.sidebar}}</aside>{{end}}
	</div>
	{{if .cta_text}}
	<div class="cta-section">
		<a href="{{.cta_link}}" class="cta-button">{{.cta_text}}</a>
	</div>
	{{end}}
</article>`,
	},
	{
		Name:        "Blank Page",
		Slug:        "blank-page",
		Description: "Fully customizable blank page - use raw HTML or rich editor",
		Category:    "pages",
		IsSystem:    true,
		Fields: []TemplateField{
			{Name: "content", Label: "Content", Type: "richtext", Required: true},
		},
		HTMLLayout: `{{.content}}`,
	},
	{
		Name:        "Homepage",
		Slug:        "homepage",
		Description: "Landing page with hero section, introduction, feature sections, and quote",
		Category:    "pages",
		IsSystem:    true,
		Fields: []TemplateField{
			{Name: "hero_tagline", Label: "Hero Tagline", Type: "richtext", Required: true, Placeholder: "Main headline text"},
			{Name: "intro_content", Label: "Introduction", Type: "richtext", Required: false},
			{Name: "sections", Label: "Feature Sections", Type: "richtext", Required: false, Placeholder: "Areas of interest, features, etc."},
			{Name: "quote_text", Label: "Featured Quote", Type: "textarea", Required: false},
			{Name: "quote_author", Label: "Quote Author", Type: "text", Required: false},
		},
		HTMLLayout: `<div class="homepage">
	<section class="hero-section">
		{{.hero_tagline}}
	</section>
	{{if .intro_content}}
	<section class="intro-section">
		{{.intro_content}}
	</section>
	{{end}}
	{{if .sections}}
	<section class="features-section">
		{{.sections}}
	</section>
	{{end}}
	{{if .quote_text}}
	<section class="quote-section">
		<blockquote>
			<p>{{.quote_text}}</p>
			{{if .quote_author}}<cite>— {{.quote_author}}</cite>{{end}}
		</blockquote>
	</section>
	{{end}}
</div>`,
	},
	{
		Name:        "Concept Page",
		Slug:        "concept-page",
		Description: "Glossary/topic page with definition, related topics, and further reading",
		Category:    "glossary",
		IsSystem:    true,
		Fields: []TemplateField{
			{Name: "definition", Label: "Definition", Type: "richtext", Required: true, Placeholder: "Main explanation of the concept"},
			{Name: "topic_links", Label: "Related Topics", Type: "richtext", Required: false, Placeholder: "Links to related concepts"},
			{Name: "further_reading", Label: "Further Reading", Type: "richtext", Required: false, Placeholder: "External resources and articles"},
		},
		HTMLLayout: `<article class="concept-page">
	<h1>{{.title}}</h1>
	<div class="definition">
		{{.definition}}
	</div>
	{{if .topic_links}}
	<div class="related-topics">
		<h2>Related Topics</h2>
		{{.topic_links}}
	</div>
	{{end}}
	{{if .further_reading}}
	<div class="further-reading">
		<h2>Further Reading</h2>
		{{.further_reading}}
	</div>
	{{end}}
</article>`,
	},
	{
		Name:        "Standard Page",
		Slug:        "standard-page",
		Description: "General purpose page with rich text content",
		Category:    "pages",
		IsSystem:    true,
		Fields: []TemplateField{
			{Name: "content", Label: "Content", Type: "richtext", Required: true},
		},
		HTMLLayout: `<article class="standard-page">
	<h1>{{.title}}</h1>
	<div class="page-content">
		{{.content}}
	</div>
</article>`,
	},
}
