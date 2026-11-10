package apiclient

import "time"

// Content types

type Content struct {
	ID              string                 `json:"id"`
	TemplateID      string                 `json:"template_id"`
	TemplateName    string                 `json:"template_name"`
	Title           string                 `json:"title"`
	Slug            string                 `json:"slug"`
	FolderID        *string                `json:"folder_id,omitempty"`
	FolderPath      string                 `json:"folder_path"`
	FullPath        string                 `json:"full_path"`
	Category        string                 `json:"category"`
	Tags            []string               `json:"tags,omitempty"`
	MetaDescription string                 `json:"meta_description"`
	OGImage         string                 `json:"og_image"`
	NoIndex         bool                   `json:"noindex,omitempty"`
	Hold            bool                   `json:"hold,omitempty"`
	AuthorName      string                 `json:"author_name,omitempty"`
	AuthorURL       string                 `json:"author_url,omitempty"`
	Data            map[string]interface{} `json:"data"`
	Published       bool                   `json:"published"`
	PublishedAt     *time.Time             `json:"published_at,omitempty"`
	UseHeader       bool                   `json:"use_header"`
	UseFooter       bool                   `json:"use_footer"`
	UseTheme        bool                   `json:"use_theme"`
	RawMode         bool                   `json:"raw_mode"`
	Deleted         bool                   `json:"deleted"`
	DeletedAt       *time.Time             `json:"deleted_at,omitempty"`
	CreatedAt       time.Time              `json:"created_at"`
	UpdatedAt       time.Time              `json:"updated_at"`
	RenderedHTML    string                 `json:"rendered_html,omitempty"`
	ForkID          string                 `json:"fork_id,omitempty"`
}

// Fork diff types

type ForkFieldDiff struct {
	Name string `json:"name"`
	Live string `json:"live"`
	Fork string `json:"fork"`
}

type ForkPageDiff struct {
	Path     string          `json:"path"`
	Title    string          `json:"title"`
	Status   string          `json:"status"`
	Conflict bool            `json:"conflict"`
	Fields   []ForkFieldDiff `json:"fields,omitempty"`
}

type ForkDiff struct {
	ForkID string         `json:"fork_id"`
	Pages  []ForkPageDiff `json:"pages"`
	Status string         `json:"status,omitempty"`
	// Set on merged forks: their copies are gone, so Pages is empty.
	MergedCreated int `json:"merged_created,omitempty"`
	MergedUpdated int `json:"merged_updated,omitempty"`
}

type ContentVersion struct {
	ID              string                 `json:"id"`
	ContentID       string                 `json:"content_id"`
	Version         int                    `json:"version"`
	Comment         string                 `json:"comment,omitempty"`
	Actor           string                 `json:"actor,omitempty"`         // "human" | "agent"
	Via             string                 `json:"via,omitempty"`           // "ui" | "api" | "copilot"
	AgentSession    string                 `json:"agent_session,omitempty"` // agent session that authored this version
	TemplateID      string                 `json:"template_id"`
	TemplateName    string                 `json:"template_name"`
	Title           string                 `json:"title"`
	Slug            string                 `json:"slug"`
	FolderID        *string                `json:"folder_id,omitempty"`
	FolderPath      string                 `json:"folder_path"`
	FullPath        string                 `json:"full_path"`
	Category        string                 `json:"category"`
	MetaDescription string                 `json:"meta_description"`
	OGImage         string                 `json:"og_image"`
	NoIndex         bool                   `json:"noindex,omitempty"`
	AuthorName      string                 `json:"author_name,omitempty"`
	AuthorURL       string                 `json:"author_url,omitempty"`
	Data            map[string]interface{} `json:"data"`
	Published       bool                   `json:"published"`
	PublishedAt     *time.Time             `json:"published_at,omitempty"`
	UseHeader       bool                   `json:"use_header"`
	UseFooter       bool                   `json:"use_footer"`
	UseTheme        bool                   `json:"use_theme"`
	RawMode         bool                   `json:"raw_mode"`
	CreatedAt       time.Time              `json:"created_at"`
}

type CreateContentRequest struct {
	TemplateID      string                 `json:"template_id"`
	Title           string                 `json:"title"`
	Slug            string                 `json:"slug"`
	FolderPath      string                 `json:"folder_path,omitempty"`
	Category        string                 `json:"category,omitempty"`
	Tags            []string               `json:"tags,omitempty"`
	MetaDescription string                 `json:"meta_description,omitempty"`
	OGImage         string                 `json:"og_image,omitempty"`
	NoIndex         bool                   `json:"noindex,omitempty"`
	Hold            bool                   `json:"hold,omitempty"`
	AuthorName      string                 `json:"author_name,omitempty"`
	AuthorURL       string                 `json:"author_url,omitempty"`
	Data            map[string]interface{} `json:"data"`
	Published       bool                   `json:"published,omitempty"`
	UseHeader       bool                   `json:"use_header,omitempty"`
	UseFooter       bool                   `json:"use_footer,omitempty"`
	UseTheme        bool                   `json:"use_theme,omitempty"`
	RawMode         bool                   `json:"raw_mode,omitempty"`
	VersionComment  string                 `json:"version_comment,omitempty"`
	Upsert          bool                   `json:"upsert,omitempty"`
	ForkID          string                 `json:"fork_id,omitempty"`
}

// Snippet types

type Snippet struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	HTML      string    `json:"html"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type CreateSnippetRequest struct {
	Name string `json:"name"`
	HTML string `json:"html"`
}

type UpdateSnippetRequest struct {
	Name string `json:"name"`
	HTML string `json:"html"`
}

type UpdateContentRequest map[string]interface{}

// Template types

type TemplateField struct {
	Name        string `json:"name"`
	Label       string `json:"label"`
	Type        string `json:"type"`
	Required    bool   `json:"required"`
	Placeholder string `json:"placeholder,omitempty"`
	Options     string `json:"options,omitempty"`
	Default     string `json:"default,omitempty"`
}

type Template struct {
	ID          string          `json:"id"`
	Name        string          `json:"name"`
	Slug        string          `json:"slug"`
	Description string          `json:"description"`
	Fields      []TemplateField `json:"fields"`
	HTMLLayout  string          `json:"html_layout"`
	Category    string          `json:"category"`
	IsSystem    bool            `json:"is_system"`
	CreatedAt   time.Time       `json:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at"`
}

type CreateTemplateRequest struct {
	Name        string          `json:"name"`
	Slug        string          `json:"slug"`
	Description string          `json:"description,omitempty"`
	Fields      []TemplateField `json:"fields"`
	HTMLLayout  string          `json:"html_layout"`
	Category    string          `json:"category,omitempty"`
}

type UpdateTemplateRequest map[string]interface{}

// Asset types

type AssetSummary struct {
	ID          string `json:"id"`
	Filename    string `json:"filename"`
	Folder      string `json:"folder"`
	FullPath    string `json:"full_path"`
	ServePath   string `json:"serve_path"`
	MimeType    string `json:"mime_type"`
	Size        int64  `json:"size"`
	Description string `json:"description"`
	CreatedAt   string `json:"created_at"`
}

type UploadAssetRequest struct {
	Filename    string `json:"filename"`
	ServePath   string `json:"serve_path"`
	DataBase64  string `json:"data_base64,omitempty"`
	Description string `json:"description,omitempty"`
}

// Theme types

type ThemeSettings struct {
	PrimaryColor    string `json:"primary_color"`
	SecondaryColor  string `json:"secondary_color"`
	AccentColor     string `json:"accent_color"`
	BackgroundColor string `json:"background_color"`
	TextColor       string `json:"text_color"`
	FontFamily      string `json:"font_family"`
	HeadingFont     string `json:"heading_font"`
	BorderRadius    string `json:"border_radius"`
	CustomCSS       string `json:"custom_css"`
	SiteName        string `json:"site_name"`
	SiteTagline     string `json:"site_tagline"`
	LogoURL         string `json:"logo_url"`
	HeadHTML        string `json:"head_html"`
	HeaderHTML      string `json:"header_html"`
	FooterHTML      string `json:"footer_html"`
}

type ThemeVersion struct {
	ID              string    `json:"id"`
	Version         int       `json:"version"`
	Comment         string    `json:"comment,omitempty"`
	PrimaryColor    string    `json:"primary_color"`
	SecondaryColor  string    `json:"secondary_color"`
	AccentColor     string    `json:"accent_color"`
	BackgroundColor string    `json:"background_color"`
	TextColor       string    `json:"text_color"`
	FontFamily      string    `json:"font_family"`
	HeadingFont     string    `json:"heading_font"`
	BorderRadius    string    `json:"border_radius"`
	CustomCSS       string    `json:"custom_css"`
	SiteName        string    `json:"site_name"`
	SiteTagline     string    `json:"site_tagline"`
	LogoURL         string    `json:"logo_url"`
	HeadHTML        string    `json:"head_html"`
	HeaderHTML      string    `json:"header_html"`
	FooterHTML      string    `json:"footer_html"`
	CreatedAt       time.Time `json:"created_at"`
}

// Site Config

type SiteConfig struct {
	TitleTemplate        string `json:"title_template"`
	TitleTemplateNoTitle string `json:"title_template_no_title"`
	MarkdownScriptPolicy string `json:"markdown_script_policy"`
}

// Redirect

type Redirect struct {
	ID          string    `json:"id"`
	FromPath    string    `json:"from_path"`
	ToPath      string    `json:"to_path"`
	StatusCode  int       `json:"status_code"`
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type CreateRedirectRequest struct {
	FromPath    string `json:"from_path"`
	ToPath      string `json:"to_path"`
	StatusCode  int    `json:"status_code,omitempty"`
	Description string `json:"description,omitempty"`
}

// Folder

type Folder struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Slug      string    `json:"slug"`
	ParentID  *string   `json:"parent_id,omitempty"`
	Path      string    `json:"path"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type CreateFolderRequest struct {
	Name     string `json:"name"`
	Slug     string `json:"slug"`
	ParentID string `json:"parent_id,omitempty"`
}

// Collection

type Collection struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	Slug         string    `json:"slug"`
	Description  string    `json:"description"`
	Category     string    `json:"category"`
	SortField    string    `json:"sort_field"`
	SortOrder    string    `json:"sort_order"`
	ItemTemplate string    `json:"item_template"`
	PageTemplate string    `json:"page_template"`
	ItemsPerPage int       `json:"items_per_page"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// Search

type SearchMatch struct {
	ID           string   `json:"id"`
	Title        string   `json:"title"`
	FullPath     string   `json:"full_path"`
	TemplateName string   `json:"template_name"`
	Published    bool     `json:"published"`
	MatchedIn    []string `json:"matched_in"`
}

type SearchResult struct {
	Query      string        `json:"query"`
	SearchType string        `json:"search_type"`
	Total      int           `json:"total"`
	Matches    []SearchMatch `json:"matches"`
}

type SearchReplaceMatch struct {
	ID            string         `json:"id"`
	Title         string         `json:"title"`
	FullPath      string         `json:"full_path"`
	Published     bool           `json:"published"`
	MatchCount    int            `json:"match_count"`
	FieldMatches  map[string]int `json:"field_matches,omitempty"`
	FieldsUpdated []string       `json:"fields_updated,omitempty"`
}

type SearchReplaceResult struct {
	Success           bool                 `json:"success,omitempty"`
	Search            string               `json:"search"`
	Replace           string               `json:"replace"`
	TotalMatches      int                  `json:"total_matches,omitempty"`
	TotalReplacements int                  `json:"total_replacements,omitempty"`
	AffectedPages     int                  `json:"affected_pages,omitempty"`
	PagesUpdated      int                  `json:"pages_updated,omitempty"`
	Matches           []SearchReplaceMatch `json:"matches,omitempty"`
	UpdatedPages      []SearchReplaceMatch `json:"updated_pages,omitempty"`
	// Skipped lists content_ids left out of a scoped run as {id, reason}
	// (fork copies: search-and-replace works on live pages only).
	Skipped []map[string]string `json:"skipped,omitempty"`
}

// API Key

type APIKey struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	Description string     `json:"description"`
	Prefix      string     `json:"prefix"`
	LastUsedAt  *time.Time `json:"last_used_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
}

type APIKeyCreated struct {
	Key         string    `json:"key"`
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Prefix      string    `json:"prefix"`
	CreatedAt   time.Time `json:"created_at"`
}

// Fork types

type Fork struct {
	ID             string  `json:"id"`
	Name           string  `json:"name"`
	Description    string  `json:"description"`
	Status         string  `json:"status"`
	PageCount      int64   `json:"page_count"`
	CreatedByEmail string  `json:"created_by_email"`
	CreatedAt      string  `json:"created_at"`
	MergedAt       *string `json:"merged_at,omitempty"`
	MergedByEmail  string  `json:"merged_by_email,omitempty"`
	MergedCreated  int     `json:"merged_created,omitempty"`
	MergedUpdated  int     `json:"merged_updated,omitempty"`
	ArchivedAt     *string `json:"archived_at,omitempty"`
}

type ForkDetail struct {
	ID             string        `json:"id"`
	Name           string        `json:"name"`
	Description    string        `json:"description"`
	Status         string        `json:"status"`
	CreatedByEmail string        `json:"created_by_email"`
	CreatedAt      string        `json:"created_at"`
	PageCount      int           `json:"page_count"`
	Pages          []ForkPageRef `json:"pages"`
	// Set on merged forks, whose page copies are deleted by the merge.
	MergedAt      string `json:"merged_at,omitempty"`
	MergedByEmail string `json:"merged_by_email,omitempty"`
	MergedCreated int    `json:"merged_created,omitempty"`
	MergedUpdated int    `json:"merged_updated,omitempty"`
}

type ForkPageRef struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	FullPath  string `json:"full_path"`
	UpdatedAt string `json:"updated_at"`
}

type ForkPageResult struct {
	ID       string `json:"id"`
	FullPath string `json:"full_path"`
	Title    string `json:"title"`
	Message  string `json:"message"`
}

type ForkMergeResult struct {
	Success      bool                    `json:"success"`
	Updated      int                     `json:"updated"`
	Created      int                     `json:"created"`
	CreatedIDs   []string                `json:"created_ids"`
	UpdatedIDs   []string                `json:"updated_ids"`
	PublishNew   bool                    `json:"publish_new"`
	NotPublished []ForkMergeNotPublished `json:"not_published"`
	Conflicts    []ForkMergeConflict     `json:"conflicts"`
	Message      string                  `json:"message"`
}

// ForkMergeNotPublished is a page a merge created that stayed a draft even
// though publish_new was set (held, or its publish failed).
type ForkMergeNotPublished struct {
	ID     string `json:"id"`
	Path   string `json:"path"`
	Reason string `json:"reason"`
}

// ForkPurgeResult is returned by PurgeForkCopies. Copies is filled on a dry
// run; Deleted on a real one.
type ForkPurgeResult struct {
	ForkID  string        `json:"fork_id"`
	DryRun  bool          `json:"dry_run"`
	Count   int           `json:"count"`
	Deleted int           `json:"deleted,omitempty"`
	Copies  []ForkCopyRef `json:"copies,omitempty"`
}

type ForkCopyRef struct {
	ID       string `json:"id"`
	FullPath string `json:"full_path"`
}

type ForkMergeConflict struct {
	Path      string `json:"path"`
	ForkTitle string `json:"fork_title"`
	LiveTitle string `json:"live_title"`
}

// Import types

type ImportSource struct {
	ID            string     `json:"id,omitempty"`
	Name          string     `json:"name"`
	URL           string     `json:"url"`
	TemplateName  string     `json:"template_name,omitempty"`
	FolderPath    string     `json:"folder_path,omitempty"`
	AutoPublish   bool       `json:"auto_publish"`
	Schedule      string     `json:"schedule"`
	Active        bool       `json:"active"`
	LastRunAt     *time.Time `json:"last_run_at,omitempty"`
	LastRunStatus string     `json:"last_run_status,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
}

type ImportJob struct {
	ID         string     `json:"id,omitempty"`
	SourceName string     `json:"source_name,omitempty"`
	Type       string     `json:"type"`
	Status     string     `json:"status"`
	TotalPages int        `json:"total_pages"`
	Created    int        `json:"created"`
	Updated    int        `json:"updated"`
	Failed     int        `json:"failed"`
	Skipped    int        `json:"skipped"`
	ErrorMsg   string     `json:"error_msg,omitempty"`
	StartedAt  time.Time  `json:"started_at"`
	FinishedAt *time.Time `json:"finished_at,omitempty"`
	CreatedBy  string     `json:"created_by,omitempty"`
}

type ImportLog struct {
	ID        string    `json:"id,omitempty"`
	JobID     string    `json:"job_id"`
	Seq       int       `json:"seq"`
	Level     string    `json:"level"`
	Message   string    `json:"message"`
	Path      string    `json:"path,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

type ImportJobDetail struct {
	Job  *ImportJob  `json:"job"`
	Logs []ImportLog `json:"logs,omitempty"`
}

type CreateImportSourceRequest struct {
	Name         string `json:"name"`
	URL          string `json:"url"`
	TemplateName string `json:"template_name,omitempty"`
	FolderPath   string `json:"folder_path,omitempty"`
	AutoPublish  bool   `json:"auto_publish,omitempty"`
	Schedule     string `json:"schedule,omitempty"`
	Active       *bool  `json:"active,omitempty"`
}

type UpdateImportSourceRequest struct {
	Name         *string `json:"name,omitempty"`
	URL          *string `json:"url,omitempty"`
	TemplateName *string `json:"template_name,omitempty"`
	FolderPath   *string `json:"folder_path,omitempty"`
	AutoPublish  *bool   `json:"auto_publish,omitempty"`
	Schedule     *string `json:"schedule,omitempty"`
	Active       *bool   `json:"active,omitempty"`
}

type ImportMarkdownPage struct {
	Filename string `json:"filename,omitempty"`
	Content  string `json:"content"`
}

type ImportMarkdownRequest struct {
	Pages           []ImportMarkdownPage `json:"pages"`
	DefaultTemplate string               `json:"default_template,omitempty"`
	DefaultFolder   string               `json:"default_folder,omitempty"`
	AutoPublish     bool                 `json:"auto_publish,omitempty"`
}

type ImportCSVRequest struct {
	CSVData      string `json:"csv_data"`
	TitleColumn  string `json:"title_column"`
	TemplateName string `json:"template_name,omitempty"`
	FolderPath   string `json:"folder_path,omitempty"`
	AutoPublish  bool   `json:"auto_publish,omitempty"`
	SlugColumn   string `json:"slug_column,omitempty"`
}

type ImportJobResponse struct {
	JobID   string `json:"job_id"`
	Message string `json:"message"`
}

// Webhook types

type Webhook struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	URL       string   `json:"url"`
	Events    []string `json:"events"`
	Active    bool     `json:"active"`
	CreatedAt string   `json:"created_at"`
	UpdatedAt string   `json:"updated_at"`
}

type WebhookCreated struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	URL       string   `json:"url"`
	Events    []string `json:"events"`
	Active    bool     `json:"active"`
	Secret    string   `json:"secret"`
	CreatedAt string   `json:"created_at"`
}

type WebhookDelivery struct {
	ID          string     `json:"id"`
	WebhookID   string     `json:"webhook_id"`
	Event       string     `json:"event"`
	Attempt     int        `json:"attempt"`
	StatusCode  int        `json:"status_code"`
	Success     bool       `json:"success"`
	Error       string     `json:"error,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	DeliveredAt *time.Time `json:"delivered_at,omitempty"`
}

type CreateWebhookRequest struct {
	Name   string   `json:"name"`
	URL    string   `json:"url"`
	Events []string `json:"events"`
	Active bool     `json:"active"`
}

type UpdateWebhookRequest struct {
	Name   *string  `json:"name,omitempty"`
	URL    *string  `json:"url,omitempty"`
	Events []string `json:"events,omitempty"`
	Active *bool    `json:"active,omitempty"`
}

// Content lock types

type ContentLock struct {
	Locked     bool   `json:"locked"`
	UserEmail  string `json:"user_email,omitempty"`
	AcquiredAt string `json:"acquired_at,omitempty"`
	ExpiresAt  string `json:"expires_at,omitempty"`
	Message    string `json:"message,omitempty"`
}

// Scheduled content types

type ScheduledContent struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	FullPath  string `json:"full_path"`
	PublishAt string `json:"publish_at"`
}

// Audit log types

type AuditLog struct {
	ID         string                 `json:"id"`
	UserEmail  string                 `json:"user_email"`
	Action     string                 `json:"action"`
	Resource   string                 `json:"resource"`
	ResourceID string                 `json:"resource_id,omitempty"`
	Details    map[string]interface{} `json:"details,omitempty"`
	IPAddress  string                 `json:"ip_address,omitempty"`
	CreatedAt  time.Time              `json:"created_at"`
}

type AuditLogList struct {
	Total int64      `json:"total"`
	Logs  []AuditLog `json:"logs"`
}

// Link check types

type LinkCheckJobResponse struct {
	JobID   string `json:"job_id"`
	Message string `json:"message"`
}

type BrokenLink struct {
	SourcePath string `json:"source_path"`
	TargetPath string `json:"target_path"`
	LinkText   string `json:"link_text"`
}

type LinkCheckJob struct {
	ID          string       `json:"id"`
	Status      string       `json:"status"`
	TotalPages  int          `json:"total_pages"`
	BrokenLinks []BrokenLink `json:"broken_links"`
	StartedAt   time.Time    `json:"started_at"`
	FinishedAt  *time.Time   `json:"finished_at,omitempty"`
}

// Comment & Approval types

type ContentComment struct {
	ID              string    `json:"id"`
	ContentID       string    `json:"content_id"`
	UserID          string    `json:"user_id"`
	UserEmail       string    `json:"user_email"`
	UserDisplayName string    `json:"user_display_name"`
	Text            string    `json:"text"`
	Mentions        []string  `json:"mentions,omitempty"`
	CreatedAt       time.Time `json:"created_at"`
}

type CreateCommentRequest struct {
	Text     string   `json:"text"`
	Mentions []string `json:"mentions,omitempty"`
}

type WorkflowApprover struct {
	UserID    string `json:"user_id"`
	UserEmail string `json:"user_email"`
	Order     int    `json:"order"`
}

type ApprovalWorkflow struct {
	ID           string             `json:"id"`
	Name         string             `json:"name"`
	Description  string             `json:"description,omitempty"`
	Trigger      string             `json:"trigger"`
	TriggerValue string             `json:"trigger_value,omitempty"`
	Approvers    []WorkflowApprover `json:"approvers"`
	Mode         string             `json:"mode"`
	CreatedAt    time.Time          `json:"created_at"`
}

type CreateWorkflowRequest struct {
	Name         string             `json:"name"`
	Description  string             `json:"description,omitempty"`
	Trigger      string             `json:"trigger"`
	TriggerValue string             `json:"trigger_value,omitempty"`
	Approvers    []WorkflowApprover `json:"approvers"`
	Mode         string             `json:"mode"`
}

type ApprovalDecision struct {
	UserID    string    `json:"user_id"`
	UserEmail string    `json:"user_email"`
	Decision  string    `json:"decision"`
	Comment   string    `json:"comment,omitempty"`
	DecidedAt time.Time `json:"decided_at"`
}

type ApprovalRequest struct {
	ID                string             `json:"id"`
	ContentID         string             `json:"content_id,omitempty"`
	ContentTitle      string             `json:"content_title"`
	ContentPath       string             `json:"content_path"`
	WorkflowID        *string            `json:"workflow_id,omitempty"`
	SubmittedByID     string             `json:"submitted_by_id"`
	SubmittedByEmail  string             `json:"submitted_by_email"`
	Status            string             `json:"status"`
	Decisions         []ApprovalDecision `json:"decisions,omitempty"`
	RequiredApprovals int                `json:"required_approvals"`
	CurrentStep       int                `json:"current_step"`
	AssetID           *string            `json:"asset_id,omitempty"`
	CreatedAt         time.Time          `json:"created_at"`
}

type ApproveRejectRequest struct {
	Comment string `json:"comment,omitempty"`
}
