package apiclient

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// APIError is an error response from the REST API.
type APIError struct {
	Status  int
	Message string
}

func (e *APIError) Error() string { return e.Message }

// Client is an HTTP client for the LightCMS REST API
type Client struct {
	baseURL      string
	apiKey       string
	agentSession string
	httpClient   *http.Client
}

// SetAgentSession sets the agent session ID sent as X-Agent-Session on
// every request, letting the server group and audit this session's changes.
func (c *Client) SetAgentSession(id string) { c.agentSession = id }

// AgentSession returns the session ID configured via SetAgentSession.
func (c *Client) AgentSession() string { return c.agentSession }

// NewAgentSessionID returns a new ID for an agent session. The server groups a
// session's changes under it so they can be reviewed or rolled back as a unit.
func NewAgentSessionID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b) // never fails: crypto/rand aborts the process instead
	return "agent-" + time.Now().UTC().Format("20060102-150405") + "-" + hex.EncodeToString(b)
}

// New creates a new API client
func New(baseURL, apiKey string) *Client {
	return &Client{
		baseURL: baseURL,
		apiKey:  apiKey,
		httpClient: &http.Client{
			Timeout: 60 * time.Second,
		},
	}
}

// NewWithTransport creates a client that carries no key and sends its requests
// through transport, which is responsible for saying who the caller is.
func NewWithTransport(baseURL string, transport http.RoundTripper) *Client {
	return &Client{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout:   60 * time.Second,
			Transport: transport,
		},
	}
}

// do executes an HTTP request with auth and JSON handling
func (c *Client) do(ctx context.Context, method, path string, body interface{}, result interface{}) error {
	var bodyReader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("failed to marshal request: %w", err)
		}
		bodyReader = bytes.NewReader(data)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+"/api/v1"+path, bodyReader)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}
	if c.agentSession != "" {
		req.Header.Set("X-Agent-Session", c.agentSession)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode >= 400 {
		var errResp struct {
			Error string `json:"error"`
		}
		if json.Unmarshal(respBody, &errResp) == nil && errResp.Error != "" {
			return &APIError{Status: resp.StatusCode, Message: errResp.Error}
		}
		return &APIError{Status: resp.StatusCode, Message: fmt.Sprintf("API error (status %d): %s", resp.StatusCode, string(respBody))}
	}

	if result != nil {
		if err := json.Unmarshal(respBody, result); err != nil {
			return fmt.Errorf("failed to decode response: %w", err)
		}
	}

	return nil
}

// Content operations

func (c *Client) ListContent(ctx context.Context, includeDeleted bool, category, folderID string) ([]Content, error) {
	params := url.Values{}
	if includeDeleted {
		params.Set("include_deleted", "true")
	}
	if category != "" {
		params.Set("category", category)
	}
	if folderID != "" {
		params.Set("folder_id", folderID)
	}

	path := "/content"
	if len(params) > 0 {
		path += "?" + params.Encode()
	}

	var result []Content
	if err := c.do(ctx, "GET", path, nil, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (c *Client) GetContent(ctx context.Context, id string, includeRendered bool) (*Content, error) {
	var result Content
	apiPath := "/content/" + id
	if includeRendered {
		apiPath += "?include_rendered=true"
	}
	if err := c.do(ctx, "GET", apiPath, nil, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *Client) GetContentByPath(ctx context.Context, path string, includeRendered bool) (*Content, error) {
	var result Content
	apiPath := "/content/by-path?path=" + url.QueryEscape(path)
	if includeRendered {
		apiPath += "&include_rendered=true"
	}
	if err := c.do(ctx, "GET", apiPath, nil, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *Client) GetBacklinks(ctx context.Context, path string) ([]Content, error) {
	var result []Content
	if err := c.do(ctx, "GET", "/content/backlinks?path="+url.QueryEscape(path), nil, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (c *Client) CreateContent(ctx context.Context, req CreateContentRequest) (*Content, error) {
	var result Content
	if err := c.do(ctx, "POST", "/content", req, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *Client) UpdateContent(ctx context.Context, id string, req UpdateContentRequest) (*Content, error) {
	var result Content
	if err := c.do(ctx, "PUT", "/content/"+id, req, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *Client) DeleteContent(ctx context.Context, id string) error {
	return c.do(ctx, "DELETE", "/content/"+id, nil, nil)
}

func (c *Client) RestoreContent(ctx context.Context, id string) error {
	return c.do(ctx, "POST", "/content/"+id+"/restore", nil, nil)
}

func (c *Client) PublishContent(ctx context.Context, id string) error {
	return c.do(ctx, "POST", "/content/"+id+"/publish", nil, nil)
}

func (c *Client) UnpublishContent(ctx context.Context, id string) error {
	return c.do(ctx, "POST", "/content/"+id+"/unpublish", nil, nil)
}

func (c *Client) GetContentVersions(ctx context.Context, contentID string) ([]ContentVersion, error) {
	var result []ContentVersion
	if err := c.do(ctx, "GET", "/content/"+contentID+"/versions", nil, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (c *Client) GetContentVersion(ctx context.Context, contentID string, version int) (*ContentVersion, error) {
	var result ContentVersion
	if err := c.do(ctx, "GET", "/content/"+contentID+"/versions/"+strconv.Itoa(version), nil, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *Client) RevertContentVersion(ctx context.Context, contentID string, version int, comment string) error {
	req := map[string]string{}
	if comment != "" {
		req["version_comment"] = comment
	}
	return c.do(ctx, "POST", "/content/"+contentID+"/versions/"+strconv.Itoa(version)+"/revert", req, nil)
}

// Template operations

func (c *Client) ListTemplates(ctx context.Context) ([]Template, error) {
	var result []Template
	if err := c.do(ctx, "GET", "/templates", nil, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (c *Client) GetTemplate(ctx context.Context, id string) (*Template, error) {
	var result Template
	if err := c.do(ctx, "GET", "/templates/"+id, nil, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *Client) CreateTemplate(ctx context.Context, req CreateTemplateRequest) (*Template, error) {
	var result Template
	if err := c.do(ctx, "POST", "/templates", req, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *Client) UpdateTemplate(ctx context.Context, id string, req UpdateTemplateRequest) (*Template, error) {
	var result Template
	if err := c.do(ctx, "PUT", "/templates/"+id, req, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *Client) DeleteTemplate(ctx context.Context, id string) error {
	return c.do(ctx, "DELETE", "/templates/"+id, nil, nil)
}

// Asset operations

func (c *Client) ListAssets(ctx context.Context, folder string) ([]AssetSummary, error) {
	path := "/assets"
	if folder != "" {
		path += "?folder=" + url.QueryEscape(folder)
	}
	var result []AssetSummary
	if err := c.do(ctx, "GET", path, nil, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (c *Client) GetAsset(ctx context.Context, id string) (*AssetSummary, error) {
	var result AssetSummary
	if err := c.do(ctx, "GET", "/assets/"+id, nil, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *Client) GetAssetByPath(ctx context.Context, path string) (*AssetSummary, error) {
	var result AssetSummary
	if err := c.do(ctx, "GET", "/assets/by-path?path="+url.QueryEscape(path), nil, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *Client) UploadAsset(ctx context.Context, req UploadAssetRequest) (*AssetSummary, error) {
	var result AssetSummary
	if err := c.do(ctx, "POST", "/assets", req, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *Client) DeleteAsset(ctx context.Context, id string) error {
	return c.do(ctx, "DELETE", "/assets/"+id, nil, nil)
}

func (c *Client) ListAssetFolders(ctx context.Context) ([]string, error) {
	var result []string
	if err := c.do(ctx, "GET", "/assets/folders", nil, &result); err != nil {
		return nil, err
	}
	return result, nil
}

// Theme operations

func (c *Client) GetTheme(ctx context.Context) (*ThemeSettings, error) {
	var result ThemeSettings
	if err := c.do(ctx, "GET", "/theme", nil, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *Client) UpdateTheme(ctx context.Context, req map[string]interface{}) (*ThemeSettings, error) {
	var result ThemeSettings
	if err := c.do(ctx, "PUT", "/theme", req, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *Client) ListThemeVersions(ctx context.Context) ([]ThemeVersion, error) {
	var result []ThemeVersion
	if err := c.do(ctx, "GET", "/theme/versions", nil, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (c *Client) GetThemeVersion(ctx context.Context, version int) (*ThemeVersion, error) {
	var result ThemeVersion
	if err := c.do(ctx, "GET", "/theme/versions/"+strconv.Itoa(version), nil, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *Client) RevertThemeVersion(ctx context.Context, version int, comment string) error {
	req := map[string]string{}
	if comment != "" {
		req["version_comment"] = comment
	}
	return c.do(ctx, "POST", "/theme/versions/"+strconv.Itoa(version)+"/revert", req, nil)
}

// Site Config

func (c *Client) GetSiteConfig(ctx context.Context) (*SiteConfig, error) {
	var result SiteConfig
	if err := c.do(ctx, "GET", "/config", nil, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *Client) UpdateSiteConfig(ctx context.Context, req map[string]interface{}) (*SiteConfig, error) {
	var result SiteConfig
	if err := c.do(ctx, "PUT", "/config", req, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// Redirects

func (c *Client) ListRedirects(ctx context.Context) ([]Redirect, error) {
	var result []Redirect
	if err := c.do(ctx, "GET", "/redirects", nil, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (c *Client) GetRedirect(ctx context.Context, id string) (*Redirect, error) {
	var result Redirect
	if err := c.do(ctx, "GET", "/redirects/"+id, nil, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *Client) CreateRedirect(ctx context.Context, req CreateRedirectRequest) (*Redirect, error) {
	var result Redirect
	if err := c.do(ctx, "POST", "/redirects", req, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *Client) UpdateRedirect(ctx context.Context, id string, req map[string]interface{}) (*Redirect, error) {
	var result Redirect
	if err := c.do(ctx, "PUT", "/redirects/"+id, req, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *Client) DeleteRedirect(ctx context.Context, id string) error {
	return c.do(ctx, "DELETE", "/redirects/"+id, nil, nil)
}

// Folders

func (c *Client) ListFolders(ctx context.Context) ([]Folder, error) {
	var result []Folder
	if err := c.do(ctx, "GET", "/folders", nil, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (c *Client) GetFolder(ctx context.Context, id string) (*Folder, error) {
	var result Folder
	if err := c.do(ctx, "GET", "/folders/"+id, nil, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *Client) CreateFolder(ctx context.Context, req CreateFolderRequest) (*Folder, error) {
	var result Folder
	if err := c.do(ctx, "POST", "/folders", req, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *Client) DeleteFolder(ctx context.Context, id string) error {
	return c.do(ctx, "DELETE", "/folders/"+id, nil, nil)
}

// Collections

func (c *Client) ListCollections(ctx context.Context) ([]Collection, error) {
	var result []Collection
	if err := c.do(ctx, "GET", "/collections", nil, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (c *Client) GetCollection(ctx context.Context, id string) (*Collection, error) {
	var result Collection
	if err := c.do(ctx, "GET", "/collections/"+id, nil, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *Client) CreateCollection(ctx context.Context, req map[string]interface{}) (*Collection, error) {
	var result Collection
	if err := c.do(ctx, "POST", "/collections", req, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *Client) UpdateCollection(ctx context.Context, id string, req map[string]interface{}) (*Collection, error) {
	var result Collection
	if err := c.do(ctx, "PUT", "/collections/"+id, req, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *Client) DeleteCollection(ctx context.Context, id string) error {
	return c.do(ctx, "DELETE", "/collections/"+id, nil, nil)
}

// Search

// SearchContent searches content by title or full text. Fork copies are left
// out unless includeForks is passed as true.
func (c *Client) SearchContent(ctx context.Context, query, searchType string, includeDeleted bool, includeForks ...bool) (*SearchResult, error) {
	params := url.Values{"q": {query}}
	if searchType != "" {
		params.Set("type", searchType)
	}
	if includeDeleted {
		params.Set("include_deleted", "true")
	}
	if len(includeForks) > 0 && includeForks[0] {
		params.Set("include_forks", "true")
	}

	var result SearchResult
	if err := c.do(ctx, "GET", "/search?"+params.Encode(), nil, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *Client) SearchReplacePreview(ctx context.Context, search, replace string, regex bool) (*SearchReplaceResult, error) {
	var result SearchReplaceResult
	if err := c.do(ctx, "POST", "/search-replace/preview", map[string]interface{}{
		"search":  search,
		"replace": replace,
		"regex":   regex,
	}, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *Client) SearchReplaceExecute(ctx context.Context, search, replace, comment string, regex, autoRepublish bool) (*SearchReplaceResult, error) {
	req := map[string]interface{}{
		"search":         search,
		"replace":        replace,
		"regex":          regex,
		"auto_republish": autoRepublish,
	}
	if comment != "" {
		req["version_comment"] = comment
	}
	var result SearchReplaceResult
	if err := c.do(ctx, "POST", "/search-replace/execute", req, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// SearchReplacePreviewPairs previews a multi-pair search/replace across all content.
func (c *Client) SearchReplacePreviewPairs(ctx context.Context, pairs []map[string]interface{}) (*SearchReplaceResult, error) {
	var result SearchReplaceResult
	if err := c.do(ctx, "POST", "/search-replace/preview", map[string]interface{}{
		"pairs": pairs,
	}, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// SearchReplaceExecutePairs executes a multi-pair search/replace across all content.
func (c *Client) SearchReplaceExecutePairs(ctx context.Context, pairs []map[string]interface{}, comment string, autoRepublish bool) (*SearchReplaceResult, error) {
	req := map[string]interface{}{
		"pairs":          pairs,
		"auto_republish": autoRepublish,
	}
	if comment != "" {
		req["version_comment"] = comment
	}
	var result SearchReplaceResult
	if err := c.do(ctx, "POST", "/search-replace/execute", req, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// API Keys

func (c *Client) ListAPIKeys(ctx context.Context) ([]APIKey, error) {
	var result []APIKey
	if err := c.do(ctx, "GET", "/api-keys", nil, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (c *Client) CreateAPIKey(ctx context.Context, name, description string) (*APIKeyCreated, error) {
	var result APIKeyCreated
	if err := c.do(ctx, "POST", "/api-keys", map[string]string{
		"name":        name,
		"description": description,
	}, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *Client) DeleteAPIKey(ctx context.Context, id string) error {
	return c.do(ctx, "DELETE", "/api-keys/"+id, nil, nil)
}

// Regenerate

func (c *Client) RegenerateAllContent(ctx context.Context) error {
	return c.do(ctx, "POST", "/regenerate", nil, nil)
}

// End-user search

type EndUserSearchResult struct {
	Query   string        `json:"query"`
	Mode    string        `json:"mode"`
	Results []interface{} `json:"results"`
	Total   int           `json:"total"`
}

func (c *Client) EndUserSearch(ctx context.Context, query, mode string, limit int) (*EndUserSearchResult, error) {
	path := "/end-user-search?q=" + url.QueryEscape(query)
	if mode != "" {
		path += "&mode=" + url.QueryEscape(mode)
	}
	if limit > 0 {
		path += "&limit=" + strconv.Itoa(limit)
	}
	var result EndUserSearchResult
	if err := c.do(ctx, "GET", path, nil, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *Client) ReindexEmbeddings(ctx context.Context) (map[string]interface{}, error) {
	var result map[string]interface{}
	if err := c.do(ctx, "POST", "/reindex-embeddings", nil, &result); err != nil {
		return nil, err
	}
	return result, nil
}

// BatchPublishContent publishes multiple content items at once.
// Pass a list of IDs, or set PublishAllDrafts = true to publish every draft.
func (c *Client) BatchPublishContent(ctx context.Context, ids []string, publishAllDrafts bool) (map[string]interface{}, error) {
	req := map[string]interface{}{
		"ids":                ids,
		"publish_all_drafts": publishAllDrafts,
	}
	var result map[string]interface{}
	if err := c.do(ctx, "POST", "/content/batch-publish", req, &result); err != nil {
		return nil, err
	}
	return result, nil
}

// PreviewContent returns the rendered HTML for a content item without publishing.
// Pass optional overrides (title, data map) to preview unsaved edits.
type PreviewContentResult struct {
	ContentID    string   `json:"content_id"`
	RenderedHTML string   `json:"rendered_html"`
	Warnings     []string `json:"warnings"`
}

func (c *Client) PreviewContent(ctx context.Context, id string, overrides map[string]interface{}) (*PreviewContentResult, error) {
	var result PreviewContentResult
	if err := c.do(ctx, "POST", "/content/"+id+"/preview", overrides, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// UpdateContentByPath updates content identified by URL path.
func (c *Client) UpdateContentByPath(ctx context.Context, path string, req map[string]interface{}) (*Content, error) {
	var result Content
	if err := c.do(ctx, "PUT", "/content/by-path?path="+url.QueryEscape(path), req, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// UploadAssetFromURL fetches a remote URL and stores it as a LightCMS asset.
func (c *Client) UploadAssetFromURL(ctx context.Context, assetURL, servePath, description string) (*AssetSummary, error) {
	req := map[string]interface{}{
		"url":         assetURL,
		"serve_path":  servePath,
		"description": description,
	}
	var result AssetSummary
	if err := c.do(ctx, "POST", "/assets/from-url", req, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// PinThemeVersion locks a theme version so it is protected from auto-pruning.
func (c *Client) PinThemeVersion(ctx context.Context, version int) error {
	return c.do(ctx, "POST", fmt.Sprintf("/theme/versions/%d/pin", version), nil, nil)
}

// UnpinThemeVersion unlocks a previously pinned theme version.
func (c *Client) UnpinThemeVersion(ctx context.Context, version int) error {
	return c.do(ctx, "POST", fmt.Sprintf("/theme/versions/%d/unpin", version), nil, nil)
}

// ScopedSearchReplaceScope defines the optional scope filter for scoped S&R.
type ScopedSearchReplaceScope struct {
	ContentIDs   []string `json:"content_ids,omitempty"`
	FolderPath   string   `json:"folder_path,omitempty"`
	TemplateName string   `json:"template_name,omitempty"`
	Category     string   `json:"category,omitempty"`
}

// ScopedSearchReplacePreview previews a search-and-replace limited to a scope.
func (c *Client) ScopedSearchReplacePreview(ctx context.Context, search, replace string, regex bool, scope ScopedSearchReplaceScope) (*SearchReplaceResult, error) {
	req := map[string]interface{}{
		"search":  search,
		"replace": replace,
		"regex":   regex,
		"scope":   scope,
	}
	var result SearchReplaceResult
	if err := c.do(ctx, "POST", "/search-replace/scoped/preview", req, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// ScopedSearchReplaceExecute runs a scoped search-and-replace.
func (c *Client) ScopedSearchReplaceExecute(ctx context.Context, search, replace, comment string, regex, autoRepublish bool, scope ScopedSearchReplaceScope) (*SearchReplaceResult, error) {
	req := map[string]interface{}{
		"search":          search,
		"replace":         replace,
		"regex":           regex,
		"version_comment": comment,
		"auto_republish":  autoRepublish,
		"scope":           scope,
	}
	var result SearchReplaceResult
	if err := c.do(ctx, "POST", "/search-replace/scoped/execute", req, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// ListContentOptions provides optional parameters for listing content.
type ListContentOptions struct {
	IncludeDeleted bool
	IncludeForks   bool // also list fork copies (left out by default)
	Category       string
	FolderID       string
	IncludeData    bool
	IncludeFields  []string
	Limit          int
	Offset         int
}

// ListContentResponse is returned when pagination is used.
type ListContentResponse struct {
	Items   []Content `json:"items"`
	Total   int64     `json:"total"`
	Limit   int       `json:"limit"`
	Offset  int       `json:"offset"`
	HasMore bool      `json:"has_more"`
}

// ListContentWithOptions lists content with extended filter options.
// When Limit > 0, returns paginated results via ListContentPaginated.
func (c *Client) ListContentWithOptions(ctx context.Context, opts ListContentOptions) ([]Content, error) {
	params := url.Values{}
	if opts.IncludeDeleted {
		params.Set("include_deleted", "true")
	}
	if opts.IncludeForks {
		params.Set("include_forks", "true")
	}
	if opts.Category != "" {
		params.Set("category", opts.Category)
	}
	if opts.FolderID != "" {
		params.Set("folder_id", opts.FolderID)
	}
	if opts.IncludeData {
		params.Set("include_data", "true")
	}
	if len(opts.IncludeFields) > 0 {
		params.Set("include_fields", strings.Join(opts.IncludeFields, ","))
	}
	if opts.Limit > 0 {
		params.Set("limit", strconv.Itoa(opts.Limit))
		if opts.Offset > 0 {
			params.Set("offset", strconv.Itoa(opts.Offset))
		}
	}

	path := "/content"
	if len(params) > 0 {
		path += "?" + params.Encode()
	}

	// When paginated, the server returns {items, total, ...} envelope
	if opts.Limit > 0 {
		var result ListContentResponse
		if err := c.do(ctx, "GET", path, nil, &result); err != nil {
			return nil, err
		}
		return result.Items, nil
	}

	var result []Content
	if err := c.do(ctx, "GET", path, nil, &result); err != nil {
		return nil, err
	}
	return result, nil
}

// ListContentPaginated returns paginated content with metadata.
func (c *Client) ListContentPaginated(ctx context.Context, opts ListContentOptions) (*ListContentResponse, error) {
	if opts.Limit <= 0 {
		opts.Limit = 100
	}
	params := url.Values{}
	if opts.IncludeDeleted {
		params.Set("include_deleted", "true")
	}
	if opts.IncludeForks {
		params.Set("include_forks", "true")
	}
	if opts.Category != "" {
		params.Set("category", opts.Category)
	}
	if opts.FolderID != "" {
		params.Set("folder_id", opts.FolderID)
	}
	if opts.IncludeData {
		params.Set("include_data", "true")
	}
	if len(opts.IncludeFields) > 0 {
		params.Set("include_fields", strings.Join(opts.IncludeFields, ","))
	}
	params.Set("limit", strconv.Itoa(opts.Limit))
	if opts.Offset > 0 {
		params.Set("offset", strconv.Itoa(opts.Offset))
	}

	path := "/content?" + params.Encode()
	var result ListContentResponse
	if err := c.do(ctx, "GET", path, nil, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// BulkCreateContent creates multiple content items in a single call.
func (c *Client) BulkCreateContent(ctx context.Context, req interface{}) (map[string]interface{}, error) {
	var result map[string]interface{}
	if err := c.do(ctx, "POST", "/content/bulk-create", req, &result); err != nil {
		return nil, err
	}
	return result, nil
}

// BulkUpdateContent updates multiple content items in a single call.
func (c *Client) BulkUpdateContent(ctx context.Context, req interface{}) (map[string]interface{}, error) {
	var result map[string]interface{}
	if err := c.do(ctx, "POST", "/content/bulk-update", req, &result); err != nil {
		return nil, err
	}
	return result, nil
}

// BulkFieldOperation applies a field operation to all matching content.
func (c *Client) BulkFieldOperation(ctx context.Context, req interface{}) (map[string]interface{}, error) {
	var result map[string]interface{}
	if err := c.do(ctx, "POST", "/content/bulk-field-op", req, &result); err != nil {
		return nil, err
	}
	return result, nil
}

// ExportContent exports content items with their field data.
func (c *Client) ExportContent(ctx context.Context, req interface{}) (map[string]interface{}, error) {
	var result map[string]interface{}
	if err := c.do(ctx, "POST", "/content/export", req, &result); err != nil {
		return nil, err
	}
	return result, nil
}

// ListSnippets returns all snippets.
func (c *Client) ListSnippets(ctx context.Context) ([]Snippet, error) {
	var snippets []Snippet
	if err := c.do(ctx, "GET", "/snippets", nil, &snippets); err != nil {
		return nil, err
	}
	return snippets, nil
}

// GetSnippet returns a snippet by ID.
func (c *Client) GetSnippet(ctx context.Context, id string) (*Snippet, error) {
	var snip Snippet
	if err := c.do(ctx, "GET", "/snippets/"+id, nil, &snip); err != nil {
		return nil, err
	}
	return &snip, nil
}

// CreateSnippet creates a new snippet.
func (c *Client) CreateSnippet(ctx context.Context, req CreateSnippetRequest) (*Snippet, error) {
	var snip Snippet
	if err := c.do(ctx, "POST", "/snippets", req, &snip); err != nil {
		return nil, err
	}
	return &snip, nil
}

// UpdateSnippet updates an existing snippet.
func (c *Client) UpdateSnippet(ctx context.Context, id string, req UpdateSnippetRequest) (*Snippet, error) {
	var snip Snippet
	if err := c.do(ctx, "PUT", "/snippets/"+id, req, &snip); err != nil {
		return nil, err
	}
	return &snip, nil
}

// DeleteSnippet deletes a snippet by ID.
func (c *Client) DeleteSnippet(ctx context.Context, id string) error {
	return c.do(ctx, "DELETE", "/snippets/"+id, nil, nil)
}

// Fork operations

// ListForks returns all fork workspaces.
func (c *Client) ListForks(ctx context.Context) ([]Fork, error) {
	var forks []Fork
	if err := c.do(ctx, "GET", "/forks", nil, &forks); err != nil {
		return nil, err
	}
	return forks, nil
}

// CreateFork creates a new fork workspace.
func (c *Client) CreateFork(ctx context.Context, name, description string) (map[string]interface{}, error) {
	body := map[string]string{"name": name, "description": description}
	var result map[string]interface{}
	if err := c.do(ctx, "POST", "/forks", body, &result); err != nil {
		return nil, err
	}
	return result, nil
}

// GetFork returns a fork with its page list.
func (c *Client) GetFork(ctx context.Context, id string) (*ForkDetail, error) {
	var detail ForkDetail
	if err := c.do(ctx, "GET", "/forks/"+id, nil, &detail); err != nil {
		return nil, err
	}
	return &detail, nil
}

// ForkPage copies a live page into a fork by content ID.
func (c *Client) ForkPage(ctx context.Context, forkID, contentID, path string) (*ForkPageResult, error) {
	body := map[string]string{}
	if contentID != "" {
		body["content_id"] = contentID
	} else {
		body["path"] = path
	}
	var result ForkPageResult
	if err := c.do(ctx, "POST", "/forks/"+forkID+"/fork-page", body, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// ListForkPages returns all pages in a fork.
func (c *Client) ListForkPages(ctx context.Context, forkID string) ([]ForkPageRef, error) {
	var pages []ForkPageRef
	if err := c.do(ctx, "GET", "/forks/"+forkID+"/pages", nil, &pages); err != nil {
		return nil, err
	}
	return pages, nil
}

// RemoveForkPage removes a page from a fork.
func (c *Client) RemoveForkPage(ctx context.Context, forkID, pageID string) error {
	return c.do(ctx, "DELETE", "/forks/"+forkID+"/pages/"+pageID, nil, nil)
}

// MergeFork merges all fork pages into live content, exactly as reviewed:
// digest is the one GetForkDiff returned, and the merge is refused if the fork
// or a page it changes was edited since. With publishNew, pages the merge
// creates are also published (held pages stay drafts).
func (c *Client) MergeFork(ctx context.Context, forkID string, publishNew bool, digest string) (*ForkMergeResult, error) {
	body := map[string]interface{}{"digest": digest}
	if publishNew {
		body["publish_new"] = true
	}
	var result ForkMergeResult
	if err := c.do(ctx, "POST", "/forks/"+forkID+"/merge", body, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// PurgeForkCopies deletes the page copies still attached to a merged or
// archived fork. With dryRun it only lists them.
func (c *Client) PurgeForkCopies(ctx context.Context, forkID string, dryRun bool) (*ForkPurgeResult, error) {
	path := "/forks/" + forkID + "/purge-copies"
	if dryRun {
		path += "?dry_run=true"
	}
	var result ForkPurgeResult
	if err := c.do(ctx, "POST", path, nil, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// ArchiveFork archives a fork without merging.
func (c *Client) ArchiveFork(ctx context.Context, forkID string) error {
	return c.do(ctx, "POST", "/forks/"+forkID+"/archive", nil, nil)
}

// DeleteFork permanently deletes a fork and all its pages.
func (c *Client) DeleteFork(ctx context.Context, forkID string) error {
	return c.do(ctx, "DELETE", "/forks/"+forkID, nil, nil)
}

// GetForkDiff returns per-field differences between each fork page and its
// live counterpart.
func (c *Client) GetForkDiff(ctx context.Context, forkID string) (*ForkDiff, error) {
	var result ForkDiff
	if err := c.do(ctx, "GET", "/forks/"+forkID+"/diff", nil, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// SandboxState is the caller's agent sandbox, as the server keeps it on the
// API key or OAuth connection.
type SandboxState struct {
	Active           bool   `json:"active"`
	Restricted       bool   `json:"restricted"` // the key or connection is set to Sandbox access
	CanMergeOwnForks bool   `json:"can_merge_own_forks"`
	ForkID           string `json:"fork_id,omitempty"`
	Name             string `json:"name,omitempty"`
	ReviewURL        string `json:"review_url,omitempty"`
}

// GetSandbox returns the caller's sandbox.
func (c *Client) GetSandbox(ctx context.Context) (*SandboxState, error) {
	var result SandboxState
	if err := c.do(ctx, "GET", "/sandbox", nil, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// StartSandbox opens a sandbox: a new fork that every later request from this
// key or connection is held to until the sandbox ends.
func (c *Client) StartSandbox(ctx context.Context, name, description string) (*SandboxState, error) {
	var result SandboxState
	if err := c.do(ctx, "POST", "/sandbox", map[string]string{"name": name, "description": description}, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// EndSandbox closes the caller's sandbox: "submit" leaves the fork for
// review, "discard" deletes it.
func (c *Client) EndSandbox(ctx context.Context, action string) (map[string]interface{}, error) {
	var result map[string]interface{}
	if err := c.do(ctx, "POST", "/sandbox/end", map[string]string{"action": action}, &result); err != nil {
		return nil, err
	}
	return result, nil
}

// GetAgentSessionChanges returns the audit ledger for an agent session.
func (c *Client) GetAgentSessionChanges(ctx context.Context, sessionID string) (map[string]interface{}, error) {
	var result map[string]interface{}
	if err := c.do(ctx, "GET", "/agent-sessions/"+url.PathEscape(sessionID)+"/changes", nil, &result); err != nil {
		return nil, err
	}
	return result, nil
}

// RollbackAgentSession reverts all content changes made by an agent session.
func (c *Client) RollbackAgentSession(ctx context.Context, sessionID string) (map[string]interface{}, error) {
	var result map[string]interface{}
	if err := c.do(ctx, "POST", "/agent-sessions/"+url.PathEscape(sessionID)+"/rollback", nil, &result); err != nil {
		return nil, err
	}
	return result, nil
}

// ---------------------------------------------------------------------------
// Import operations
// ---------------------------------------------------------------------------

// ListImportSources returns all configured import sources.
func (c *Client) ListImportSources(ctx context.Context) ([]ImportSource, error) {
	var result []ImportSource
	if err := c.do(ctx, "GET", "/imports/sources", nil, &result); err != nil {
		return nil, err
	}
	return result, nil
}

// CreateImportSource creates a new RSS/Atom import source.
func (c *Client) CreateImportSource(ctx context.Context, req CreateImportSourceRequest) (*ImportSource, error) {
	var result ImportSource
	if err := c.do(ctx, "POST", "/imports/sources", req, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// UpdateImportSource updates an existing import source by ID.
func (c *Client) UpdateImportSource(ctx context.Context, id string, req UpdateImportSourceRequest) (*ImportSource, error) {
	var result ImportSource
	if err := c.do(ctx, "PUT", "/imports/sources/"+id, req, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// DeleteImportSource deletes an import source by ID.
func (c *Client) DeleteImportSource(ctx context.Context, id string) error {
	return c.do(ctx, "DELETE", "/imports/sources/"+id, nil, nil)
}

// TriggerImportSource manually triggers an RSS import source.
func (c *Client) TriggerImportSource(ctx context.Context, id string) (*ImportJobResponse, error) {
	var result ImportJobResponse
	if err := c.do(ctx, "POST", "/imports/sources/"+id+"/trigger", nil, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// ImportMarkdown submits one or more markdown pages for import.
func (c *Client) ImportMarkdown(ctx context.Context, req ImportMarkdownRequest) (*ImportJobResponse, error) {
	var result ImportJobResponse
	if err := c.do(ctx, "POST", "/imports/markdown", req, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// ImportCSV submits CSV data for import.
func (c *Client) ImportCSV(ctx context.Context, req ImportCSVRequest) (*ImportJobResponse, error) {
	var result ImportJobResponse
	if err := c.do(ctx, "POST", "/imports/csv", req, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// ListImportJobs returns recent import jobs.
func (c *Client) ListImportJobs(ctx context.Context, limit int) ([]ImportJob, error) {
	path := "/imports/jobs"
	if limit > 0 {
		path += "?limit=" + strconv.Itoa(limit)
	}
	var result []ImportJob
	if err := c.do(ctx, "GET", path, nil, &result); err != nil {
		return nil, err
	}
	return result, nil
}

// GetImportJob returns a specific import job with optional log lines.
func (c *Client) GetImportJob(ctx context.Context, id string, includeLogs bool) (*ImportJobDetail, error) {
	path := "/imports/jobs/" + id
	if !includeLogs {
		path += "?logs=false"
	}
	var result ImportJobDetail
	if err := c.do(ctx, "GET", path, nil, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// CancelImportJob cancels a running import job.
func (c *Client) CancelImportJob(ctx context.Context, id string) error {
	return c.do(ctx, "POST", "/imports/jobs/"+id+"/cancel", nil, nil)
}

// ---------------------------------------------------------------------------
// Webhook operations
// ---------------------------------------------------------------------------

func (c *Client) ListWebhooks(ctx context.Context) ([]Webhook, error) {
	var result []Webhook
	if err := c.do(ctx, "GET", "/webhooks", nil, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (c *Client) CreateWebhook(ctx context.Context, req CreateWebhookRequest) (*WebhookCreated, error) {
	var result WebhookCreated
	if err := c.do(ctx, "POST", "/webhooks", req, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *Client) UpdateWebhook(ctx context.Context, id string, req UpdateWebhookRequest) (map[string]interface{}, error) {
	var result map[string]interface{}
	if err := c.do(ctx, "PUT", "/webhooks/"+id, req, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (c *Client) DeleteWebhook(ctx context.Context, id string) error {
	return c.do(ctx, "DELETE", "/webhooks/"+id, nil, nil)
}

func (c *Client) RegenerateWebhookSecret(ctx context.Context, id string) (string, error) {
	var result struct {
		Secret string `json:"secret"`
	}
	if err := c.do(ctx, "POST", "/webhooks/"+id+"/regenerate-secret", nil, &result); err != nil {
		return "", err
	}
	return result.Secret, nil
}

func (c *Client) ListWebhookDeliveries(ctx context.Context, id string, limit int) ([]WebhookDelivery, error) {
	path := "/webhooks/" + id + "/deliveries"
	if limit > 0 {
		path += "?limit=" + strconv.Itoa(limit)
	}
	var result []WebhookDelivery
	if err := c.do(ctx, "GET", path, nil, &result); err != nil {
		return nil, err
	}
	return result, nil
}

// ---------------------------------------------------------------------------
// Content lock operations
// ---------------------------------------------------------------------------

func (c *Client) GetContentLock(ctx context.Context, contentID string) (*ContentLock, error) {
	var result ContentLock
	if err := c.do(ctx, "GET", "/content/"+contentID+"/lock", nil, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *Client) AcquireContentLock(ctx context.Context, contentID string) (*ContentLock, error) {
	var result ContentLock
	if err := c.do(ctx, "POST", "/content/"+contentID+"/lock", nil, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *Client) ReleaseContentLock(ctx context.Context, contentID string) error {
	return c.do(ctx, "DELETE", "/content/"+contentID+"/lock", nil, nil)
}

func (c *Client) ForceUnlockContent(ctx context.Context, contentID string) error {
	return c.do(ctx, "POST", "/content/"+contentID+"/lock/force", nil, nil)
}

// ---------------------------------------------------------------------------
// Scheduled publish operations
// ---------------------------------------------------------------------------

func (c *Client) ScheduleContentPublish(ctx context.Context, contentID string, publishAt *string) (map[string]interface{}, error) {
	req := map[string]interface{}{"publish_at": publishAt}
	var result map[string]interface{}
	if err := c.do(ctx, "POST", "/content/"+contentID+"/schedule", req, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (c *Client) ListScheduledContent(ctx context.Context, folder string) ([]ScheduledContent, error) {
	path := "/content/scheduled"
	if folder != "" {
		path += "?folder=" + url.QueryEscape(folder)
	}
	var result []ScheduledContent
	if err := c.do(ctx, "GET", path, nil, &result); err != nil {
		return nil, err
	}
	return result, nil
}

// ---------------------------------------------------------------------------
// Audit log operations
// ---------------------------------------------------------------------------

func (c *Client) ListAuditLogs(ctx context.Context, limit int, action, resource string) (*AuditLogList, error) {
	params := url.Values{}
	if limit > 0 {
		params.Set("limit", strconv.Itoa(limit))
	}
	if action != "" {
		params.Set("action", action)
	}
	if resource != "" {
		params.Set("resource", resource)
	}
	path := "/audit"
	if len(params) > 0 {
		path += "?" + params.Encode()
	}
	var result AuditLogList
	if err := c.do(ctx, "GET", path, nil, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// ---------------------------------------------------------------------------
// Link check operations
// ---------------------------------------------------------------------------

func (c *Client) StartLinkCheck(ctx context.Context) (*LinkCheckJobResponse, error) {
	var result LinkCheckJobResponse
	if err := c.do(ctx, "POST", "/link-check", nil, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *Client) GetLinkCheckJob(ctx context.Context, id string) (*LinkCheckJob, error) {
	var result LinkCheckJob
	if err := c.do(ctx, "GET", "/link-check/"+id, nil, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// ---------------------------------------------------------------------------
// Comments
// ---------------------------------------------------------------------------

func (c *Client) ListComments(ctx context.Context, contentID string) ([]ContentComment, error) {
	var result []ContentComment
	if err := c.do(ctx, "GET", "/content/"+contentID+"/comments", nil, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (c *Client) CreateComment(ctx context.Context, contentID string, req CreateCommentRequest) (*ContentComment, error) {
	var result ContentComment
	if err := c.do(ctx, "POST", "/content/"+contentID+"/comments", req, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *Client) DeleteComment(ctx context.Context, contentID, commentID string) error {
	return c.do(ctx, "DELETE", "/content/"+contentID+"/comments/"+commentID, nil, nil)
}

// ---------------------------------------------------------------------------
// Approval Workflows
// ---------------------------------------------------------------------------

func (c *Client) ListApprovalWorkflows(ctx context.Context) ([]ApprovalWorkflow, error) {
	var result []ApprovalWorkflow
	if err := c.do(ctx, "GET", "/approval-workflows", nil, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (c *Client) GetApprovalWorkflow(ctx context.Context, id string) (*ApprovalWorkflow, error) {
	var result ApprovalWorkflow
	if err := c.do(ctx, "GET", "/approval-workflows/"+id, nil, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *Client) CreateApprovalWorkflow(ctx context.Context, req CreateWorkflowRequest) (*ApprovalWorkflow, error) {
	var result ApprovalWorkflow
	if err := c.do(ctx, "POST", "/approval-workflows", req, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *Client) UpdateApprovalWorkflow(ctx context.Context, id string, req CreateWorkflowRequest) error {
	return c.do(ctx, "PUT", "/approval-workflows/"+id, req, nil)
}

func (c *Client) DeleteApprovalWorkflow(ctx context.Context, id string) error {
	return c.do(ctx, "DELETE", "/approval-workflows/"+id, nil, nil)
}

// ---------------------------------------------------------------------------
// Approval Requests
// ---------------------------------------------------------------------------

func (c *Client) ListApprovalRequests(ctx context.Context, filter string) ([]ApprovalRequest, error) {
	path := "/approval-requests"
	if filter != "" {
		path += "?filter=" + filter
	}
	var result []ApprovalRequest
	if err := c.do(ctx, "GET", path, nil, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (c *Client) GetApprovalRequest(ctx context.Context, id string) (*ApprovalRequest, error) {
	var result ApprovalRequest
	if err := c.do(ctx, "GET", "/approval-requests/"+id, nil, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *Client) SubmitForApproval(ctx context.Context, contentID string) (*ApprovalRequest, error) {
	var result ApprovalRequest
	if err := c.do(ctx, "POST", "/content/"+contentID+"/submit-approval", nil, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *Client) ApproveRequest(ctx context.Context, id string, req ApproveRejectRequest) error {
	return c.do(ctx, "POST", "/approval-requests/"+id+"/approve", req, nil)
}

func (c *Client) RejectRequest(ctx context.Context, id string, req ApproveRejectRequest) error {
	return c.do(ctx, "POST", "/approval-requests/"+id+"/reject", req, nil)
}

func (c *Client) CancelApprovalRequest(ctx context.Context, id string) error {
	return c.do(ctx, "POST", "/approval-requests/"+id+"/cancel", nil, nil)
}

// GetMaintenanceReport returns the latest maintenance scan report.
func (c *Client) GetMaintenanceReport(ctx context.Context) (map[string]interface{}, error) {
	var result map[string]interface{}
	if err := c.do(ctx, "GET", "/maintenance/report", nil, &result); err != nil {
		return nil, err
	}
	return result, nil
}

// RunMaintenanceScan runs a maintenance scan now (optionally with an async
// broken-link check) and returns the fresh report.
func (c *Client) RunMaintenanceScan(ctx context.Context, withLinkCheck bool) (map[string]interface{}, error) {
	path := "/maintenance/scan"
	if withLinkCheck {
		path += "?link_check=true"
	}
	var result map[string]interface{}
	if err := c.do(ctx, "POST", path, nil, &result); err != nil {
		return nil, err
	}
	return result, nil
}

// GetIndexNowStatus returns IndexNow status, key file URL, and recent submissions.
func (c *Client) GetIndexNowStatus(ctx context.Context) (map[string]interface{}, error) {
	var result map[string]interface{}
	if err := c.do(ctx, "GET", "/indexnow", nil, &result); err != nil {
		return nil, err
	}
	return result, nil
}

// SetIndexNowEnabled turns automatic IndexNow submission on or off.
func (c *Client) SetIndexNowEnabled(ctx context.Context, enabled bool) (map[string]interface{}, error) {
	var result map[string]interface{}
	if err := c.do(ctx, "PUT", "/indexnow", map[string]interface{}{"enabled": enabled}, &result); err != nil {
		return nil, err
	}
	return result, nil
}

// SubmitIndexNow submits every published page (all=true) or queues specific paths.
func (c *Client) SubmitIndexNow(ctx context.Context, all bool, paths []string) (map[string]interface{}, error) {
	body := map[string]interface{}{"all": all}
	if len(paths) > 0 {
		body["paths"] = paths
	}
	var result map[string]interface{}
	if err := c.do(ctx, "POST", "/indexnow/submit", body, &result); err != nil {
		return nil, err
	}
	return result, nil
}

// GetSEOSettings returns the SEO & AI settings, crawler registry, and robots.txt preview.
func (c *Client) GetSEOSettings(ctx context.Context) (map[string]interface{}, error) {
	var result map[string]interface{}
	if err := c.do(ctx, "GET", "/seo", nil, &result); err != nil {
		return nil, err
	}
	return result, nil
}

// UpdateSEOSettings applies a partial update (only the keys present change).
func (c *Client) UpdateSEOSettings(ctx context.Context, updates map[string]interface{}) (map[string]interface{}, error) {
	var result map[string]interface{}
	if err := c.do(ctx, "PUT", "/seo", updates, &result); err != nil {
		return nil, err
	}
	return result, nil
}

// GetAITraffic returns the AI traffic report for the last N days.
func (c *Client) GetAITraffic(ctx context.Context, days int) (map[string]interface{}, error) {
	path := "/analytics/ai"
	if days > 0 {
		path += fmt.Sprintf("?days=%d", days)
	}
	var result map[string]interface{}
	if err := c.do(ctx, "GET", path, nil, &result); err != nil {
		return nil, err
	}
	return result, nil
}

// RepairForkDamage lists (dryRun) or repairs fork copies flagged published
// and live published pages missing their generated HTML.
func (c *Client) RepairForkDamage(ctx context.Context, dryRun bool) (map[string]interface{}, error) {
	path := "/maintenance/repair-fork-damage"
	if dryRun {
		path += "?dry_run=true"
	}
	var result map[string]interface{}
	if err := c.do(ctx, "POST", path, nil, &result); err != nil {
		return nil, err
	}
	return result, nil
}

// BackfillPublishedDates sets published_at = created_at on published pages missing it.
func (c *Client) BackfillPublishedDates(ctx context.Context, dryRun bool) (map[string]interface{}, error) {
	path := "/maintenance/backfill-published-dates"
	if dryRun {
		path += "?dry_run=true"
	}
	var result map[string]interface{}
	if err := c.do(ctx, "POST", path, nil, &result); err != nil {
		return nil, err
	}
	return result, nil
}
