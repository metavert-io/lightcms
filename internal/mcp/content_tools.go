package mcp

import (
	"context"
	"fmt"

	"github.com/metavert-io/lightcms/v7/internal/apiclient"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Content tool input types
type ListContentInput struct {
	IncludeDeleted bool     `json:"include_deleted,omitempty" jsonschema:"Include soft-deleted content in results"`
	IncludeForks   bool     `json:"include_forks,omitempty" jsonschema:"Also list fork copies (working copies inside fork workspaces). Off by default; use get_fork to see one fork's pages"`
	Category       string   `json:"category,omitempty" jsonschema:"Filter by content category"`
	FolderID       string   `json:"folder_id,omitempty" jsonschema:"Filter by folder ID (MongoDB ObjectID)"`
	IncludeData    bool     `json:"include_data,omitempty" jsonschema:"If true, include all template field data in results (avoids per-item get_content calls)"`
	IncludeFields  []string `json:"include_fields,omitempty" jsonschema:"Include only these specific field names from the data object (more efficient than include_data for large content)"`
	Limit          int      `json:"limit,omitempty" jsonschema:"Maximum number of items to return (1-500). When set, returns paginated response with {items, total, limit, offset, has_more}"`
	Offset         int      `json:"offset,omitempty" jsonschema:"Number of items to skip (for pagination). Requires limit to be set"`
}

type GetContentInput struct {
	ID              string `json:"id,omitempty" jsonschema:"Content ID (MongoDB ObjectID)"`
	Path            string `json:"path,omitempty" jsonschema:"Content path (e.g., /about or /blog/my-post)"`
	IncludeRendered bool   `json:"include_rendered,omitempty" jsonschema:"If true, include the fully rendered HTML output in the response"`
}

type CreateContentInput struct {
	TemplateID      string                 `json:"template_id" jsonschema:"Template ID (MongoDB ObjectID),required"`
	Title           string                 `json:"title" jsonschema:"Content title,required"`
	Slug            string                 `json:"slug" jsonschema:"URL slug for the content,required"`
	FolderPath      string                 `json:"folder_path,omitempty" jsonschema:"Folder path (e.g., /blog)"`
	Category        string                 `json:"category,omitempty" jsonschema:"Content category for collections"`
	Tags            []string               `json:"tags,omitempty" jsonschema:"Tags for lc:query index pages (e.g. ['AI & Machine Intelligence', 'Generative AI'])"`
	MetaDescription string                 `json:"meta_description,omitempty" jsonschema:"SEO meta description"`
	OGImage         string                 `json:"og_image,omitempty" jsonschema:"Open Graph image URL"`
	NoIndex         bool                   `json:"noindex,omitempty" jsonschema:"Hide this page from search engines and AI (noindex; excluded from sitemap, llms.txt, feeds, IndexNow)"`
	Hold            bool                   `json:"hold,omitempty" jsonschema:"Put the page on hold: it cannot be published (directly, in bulk, on a schedule or by merging a fork) until hold is cleared. Cannot be combined with published=true"`
	AuthorName      string                 `json:"author_name,omitempty" jsonschema:"Author name for structured data and feeds (defaults to the site author)"`
	AuthorURL       string                 `json:"author_url,omitempty" jsonschema:"Author profile URL"`
	Data            map[string]interface{} `json:"data" jsonschema:"Template field values,required"`
	UseHeader       bool                   `json:"use_header,omitempty" jsonschema:"Include site header"`
	UseFooter       bool                   `json:"use_footer,omitempty" jsonschema:"Include site footer"`
	UseTheme        bool                   `json:"use_theme,omitempty" jsonschema:"Apply site theme/layout"`
	RawMode         bool                   `json:"raw_mode,omitempty" jsonschema:"Use raw HTML mode"`
	Published       bool                   `json:"published,omitempty" jsonschema:"Publish immediately"`
	VersionComment  string                 `json:"version_comment,omitempty" jsonschema:"Optional comment describing this version"`
	Upsert          bool                   `json:"upsert,omitempty" jsonschema:"If true, update existing content at the same path instead of returning a duplicate key error"`
}

type UpdateContentInput struct {
	ID              string                 `json:"id" jsonschema:"Content ID (MongoDB ObjectID),required"`
	TemplateID      string                 `json:"template_id,omitempty" jsonschema:"Template ID (MongoDB ObjectID)"`
	Title           string                 `json:"title,omitempty" jsonschema:"Content title"`
	Slug            string                 `json:"slug,omitempty" jsonschema:"URL slug"`
	FolderPath      string                 `json:"folder_path,omitempty" jsonschema:"Folder path"`
	Category        string                 `json:"category,omitempty" jsonschema:"Content category"`
	Tags            []string               `json:"tags,omitempty" jsonschema:"Tags for lc:query index pages"`
	MetaDescription string                 `json:"meta_description,omitempty" jsonschema:"SEO meta description"`
	OGImage         string                 `json:"og_image,omitempty" jsonschema:"Open Graph image URL"`
	NoIndex         *bool                  `json:"noindex,omitempty" jsonschema:"true hides the page from search engines and AI; false makes it visible again"`
	Hold            *bool                  `json:"hold,omitempty" jsonschema:"true puts the page on hold so it cannot be published; false clears the hold. Holding a published page does not unpublish it"`
	AuthorName      *string                `json:"author_name,omitempty" jsonschema:"Author name for structured data and feeds; empty string reverts to the site default"`
	AuthorURL       *string                `json:"author_url,omitempty" jsonschema:"Author profile URL; empty string clears it"`
	Data            map[string]interface{} `json:"data,omitempty" jsonschema:"Template field values"`
	UseHeader       bool                   `json:"use_header,omitempty" jsonschema:"Include site header"`
	UseFooter       bool                   `json:"use_footer,omitempty" jsonschema:"Include site footer"`
	UseTheme        bool                   `json:"use_theme,omitempty" jsonschema:"Apply site theme/layout"`
	RawMode         bool                   `json:"raw_mode,omitempty" jsonschema:"Use raw HTML mode"`
	SetUseHeader    bool                   `json:"set_use_header,omitempty" jsonschema:"Set to true to explicitly update use_header (needed to set it to false)"`
	SetUseFooter    bool                   `json:"set_use_footer,omitempty" jsonschema:"Set to true to explicitly update use_footer (needed to set it to false)"`
	SetUseTheme     bool                   `json:"set_use_theme,omitempty" jsonschema:"Set to true to explicitly update use_theme (needed to set it to false)"`
	SetRawMode      bool                   `json:"set_raw_mode,omitempty" jsonschema:"Set to true to explicitly update raw_mode (needed to set it to false)"`
	ClearFields     []string               `json:"clear_fields,omitempty" jsonschema:"Field names to clear to empty string (removes ambiguity about how to delete field content)"`
	DryRun          bool                   `json:"dry_run,omitempty" jsonschema:"If true, validate the update without saving"`
	VersionComment  string                 `json:"version_comment,omitempty" jsonschema:"Optional comment describing this version change"`
}

type ContentIDInput struct {
	ID string `json:"id" jsonschema:"Content ID (MongoDB ObjectID),required"`
}

type BulkCreateItem struct {
	TemplateID      string                 `json:"template_id" jsonschema:"Template ID (MongoDB ObjectID),required"`
	Title           string                 `json:"title" jsonschema:"Content title,required"`
	Slug            string                 `json:"slug" jsonschema:"URL slug,required"`
	FolderPath      string                 `json:"folder_path,omitempty" jsonschema:"Folder path"`
	Category        string                 `json:"category,omitempty" jsonschema:"Content category"`
	Tags            []string               `json:"tags,omitempty" jsonschema:"Tags"`
	MetaDescription string                 `json:"meta_description,omitempty" jsonschema:"SEO meta description"`
	OGImage         string                 `json:"og_image,omitempty" jsonschema:"Open Graph image URL"`
	Data            map[string]interface{} `json:"data" jsonschema:"Template field values,required"`
	Published       bool                   `json:"published,omitempty" jsonschema:"Publish immediately"`
	UseHeader       bool                   `json:"use_header,omitempty" jsonschema:"Include site header"`
	UseFooter       bool                   `json:"use_footer,omitempty" jsonschema:"Include site footer"`
	UseTheme        bool                   `json:"use_theme,omitempty" jsonschema:"Apply site theme"`
	RawMode         bool                   `json:"raw_mode,omitempty" jsonschema:"Raw HTML mode"`
}

type BulkCreateContentInput struct {
	Items          []BulkCreateItem `json:"items" jsonschema:"Array of content items to create (max 100),required"`
	VersionComment string           `json:"version_comment,omitempty" jsonschema:"Version comment for all created items"`
	Upsert         bool             `json:"upsert,omitempty" jsonschema:"If true, update existing content at the same path instead of failing on duplicates"`
}

type GetVersionsInput struct {
	ContentID string `json:"content_id" jsonschema:"Content ID (MongoDB ObjectID),required"`
}

type GetVersionInput struct {
	ContentID string `json:"content_id" jsonschema:"Content ID (MongoDB ObjectID),required"`
	Version   int    `json:"version" jsonschema:"Version number,required"`
}

type RevertToVersionInput struct {
	ContentID      string `json:"content_id" jsonschema:"Content ID (MongoDB ObjectID),required"`
	Version        int    `json:"version" jsonschema:"Version number to revert to,required"`
	VersionComment string `json:"version_comment,omitempty" jsonschema:"Optional comment for the revert (e.g., 'Reverted to v3')"`
}

type BatchPublishInput struct {
	IDs              []string `json:"ids,omitempty" jsonschema:"List of content IDs to publish. Mutually exclusive with publish_all_drafts."`
	PublishAllDrafts bool     `json:"publish_all_drafts,omitempty" jsonschema:"If true, publish every unpublished (draft) content item in the site"`
}

type PreviewContentInput struct {
	ID    string                 `json:"id" jsonschema:"Content ID (MongoDB ObjectID),required"`
	Title string                 `json:"title,omitempty" jsonschema:"Override title for the preview (not saved)"`
	Data  map[string]interface{} `json:"data,omitempty" jsonschema:"Override field data for the preview (not saved). Merged on top of existing data."`
}

type UpdateContentByPathInput struct {
	Path            string                 `json:"path" jsonschema:"URL path of the content to update (e.g. /about or /blog/my-post),required"`
	Title           string                 `json:"title,omitempty" jsonschema:"New title"`
	Data            map[string]interface{} `json:"data,omitempty" jsonschema:"Field values to update"`
	Category        string                 `json:"category,omitempty" jsonschema:"Content category"`
	Tags            []string               `json:"tags,omitempty" jsonschema:"Tags for lc:query index pages"`
	MetaDescription string                 `json:"meta_description,omitempty" jsonschema:"SEO meta description"`
	OGImage         string                 `json:"og_image,omitempty" jsonschema:"Open Graph image URL"`
	NoIndex         *bool                  `json:"noindex,omitempty" jsonschema:"true hides the page from search engines and AI; false makes it visible again"`
	Hold            *bool                  `json:"hold,omitempty" jsonschema:"true puts the page on hold so it cannot be published; false clears the hold. Holding a published page does not unpublish it"`
	AuthorName      *string                `json:"author_name,omitempty" jsonschema:"Author name for structured data and feeds; empty string reverts to the site default"`
	AuthorURL       *string                `json:"author_url,omitempty" jsonschema:"Author profile URL; empty string clears it"`
	Published       *bool                  `json:"published,omitempty" jsonschema:"Publish state"`
	VersionComment  string                 `json:"version_comment,omitempty" jsonschema:"Version comment"`
}

type GetBacklinksInput struct {
	Path string `json:"path" jsonschema:"URL path to find backlinks for (e.g. /about),required"`
}

type BulkUpdateItem struct {
	ID              string                 `json:"id" jsonschema:"Content ID (MongoDB ObjectID),required"`
	Title           string                 `json:"title,omitempty" jsonschema:"New title (optional)"`
	Tags            []string               `json:"tags,omitempty" jsonschema:"Replace tags (optional)"`
	Data            map[string]interface{} `json:"data,omitempty" jsonschema:"Fields to update (merge semantics)"`
	ClearFields     []string               `json:"clear_fields,omitempty" jsonschema:"Field names to clear to empty string"`
	MetaDescription string                 `json:"meta_description,omitempty" jsonschema:"SEO meta description"`
}

type BulkUpdateContentInput struct {
	Updates        []BulkUpdateItem `json:"updates" jsonschema:"Array of content updates (max 100),required"`
	VersionComment string           `json:"version_comment,omitempty" jsonschema:"Version comment applied to all updates"`
	DryRun         bool             `json:"dry_run,omitempty" jsonschema:"If true, validate all IDs exist without saving"`
}

type BulkFieldOperationInput struct {
	Operation      string   `json:"operation" jsonschema:"Operation: clear, set, prepend, append, or wrap,required"`
	Field          string   `json:"field" jsonschema:"Field name to operate on,required"`
	Value          string   `json:"value,omitempty" jsonschema:"Value for set/prepend/append operations"`
	Before         string   `json:"before,omitempty" jsonschema:"Prefix string for wrap operation"`
	After          string   `json:"after,omitempty" jsonschema:"Suffix string for wrap operation"`
	VersionComment string   `json:"version_comment,omitempty" jsonschema:"Version comment"`
	DryRun         bool     `json:"dry_run,omitempty" jsonschema:"Preview affected pages without saving"`
	ContentIDs     []string `json:"content_ids,omitempty" jsonschema:"Limit to specific content IDs"`
	FolderPath     string   `json:"folder_path,omitempty" jsonschema:"Limit to pages under this path"`
	TemplateName   string   `json:"template_name,omitempty" jsonschema:"Limit to pages using this template"`
	Category       string   `json:"category,omitempty" jsonschema:"Limit to pages in this category"`
}

type ExportContentInput struct {
	TemplateName string   `json:"template_name,omitempty" jsonschema:"Filter by template name"`
	Category     string   `json:"category,omitempty" jsonschema:"Filter by category"`
	FolderPath   string   `json:"folder_path,omitempty" jsonschema:"Filter by folder path prefix"`
	ContentIDs   []string `json:"content_ids,omitempty" jsonschema:"Export only these specific IDs"`
	Fields       []string `json:"fields,omitempty" jsonschema:"Only include these field names (empty = all fields)"`
}

type ScopedSearchReplaceInput struct {
	Search         string `json:"search" jsonschema:"Text to search for,required"`
	Replace        string `json:"replace" jsonschema:"Replacement text (empty string to delete)"`
	Regex          bool   `json:"regex,omitempty" jsonschema:"If true, treat search as a Go regular expression. Use $1, $2 for capture group references in replace."`
	VersionComment string `json:"version_comment,omitempty" jsonschema:"Version comment for updated pages"`
	AutoRepublish  bool   `json:"auto_republish,omitempty" jsonschema:"If true (execute only), re-publish all previously-published pages immediately after updating them (saves a separate publish_multiple call)"`
	// Scope filters — leave all empty to match everything (same as global S&R)
	ContentIDs   []string `json:"content_ids,omitempty" jsonschema:"Limit to specific content IDs"`
	FolderPath   string   `json:"folder_path,omitempty" jsonschema:"Limit to pages whose URL starts with this path (e.g. /blog)"`
	TemplateName string   `json:"template_name,omitempty" jsonschema:"Limit to pages using this template name (e.g. 'Concept Page')"`
	Category     string   `json:"category,omitempty" jsonschema:"Limit to pages in this category"`
}

func (s *Server) registerContentTools() {
	// List content
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:  "list_content",
		Title: "List Content",
		Description: `List all content items with optional filters. Returns content metadata including title, path, publish status, and timestamps.

Add include_data: true to get full field data for all items in one call, or include_fields: ["field1", "field2"] to fetch only specific fields — both avoid per-item get_content calls for bulk workflows.

Up to 20 concurrent update_content calls are safe. For larger batches, prefer bulk_update_content (up to 100 items per call).`,
		Annotations: &mcp.ToolAnnotations{
			Title:         "List Content",
			ReadOnlyHint:  true,
			OpenWorldHint: boolPtr(false),
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, args ListContentInput) (*mcp.CallToolResult, any, error) {
		contents, err := s.client.ListContentWithOptions(ctx, apiclient.ListContentOptions{
			IncludeDeleted: args.IncludeDeleted,
			IncludeForks:   args.IncludeForks,
			Category:       args.Category,
			FolderID:       args.FolderID,
			IncludeData:    args.IncludeData,
			IncludeFields:  args.IncludeFields,
			Limit:          args.Limit,
			Offset:         args.Offset,
		})
		if err != nil {
			return errorResult(err), nil, nil
		}

		// Return summary for each content
		type ContentSummary struct {
			ID          string                 `json:"id"`
			Title       string                 `json:"title"`
			Slug        string                 `json:"slug"`
			FullPath    string                 `json:"full_path"`
			Category    string                 `json:"category"`
			Published   bool                   `json:"published"`
			Hold        bool                   `json:"hold,omitempty"`
			Deleted     bool                   `json:"deleted"`
			UpdatedAt   string                 `json:"updated_at"`
			PublishedAt string                 `json:"published_at,omitempty"`
			ForkID      string                 `json:"fork_id,omitempty"`
			Data        map[string]interface{} `json:"data,omitempty"`
		}

		wantData := args.IncludeData || len(args.IncludeFields) > 0
		summaries := make([]ContentSummary, len(contents))
		publishedCount := 0
		deletedCount := 0
		draftCount := 0
		for i, c := range contents {
			summary := ContentSummary{
				ID:        c.ID,
				Title:     c.Title,
				Slug:      c.Slug,
				FullPath:  c.FullPath,
				Category:  c.Category,
				Published: c.Published,
				Hold:      c.Hold,
				Deleted:   c.Deleted,
				UpdatedAt: c.UpdatedAt.Format("2006-01-02 15:04:05"),
				ForkID:    c.ForkID,
			}
			if c.PublishedAt != nil {
				summary.PublishedAt = c.PublishedAt.Format("2006-01-02 15:04:05")
			}
			if wantData {
				summary.Data = c.Data
			}
			summaries[i] = summary

			if c.Deleted {
				deletedCount++
			} else if c.Published {
				publishedCount++
			} else {
				draftCount++
			}
		}

		type ListContentResponse struct {
			Total           int              `json:"total"`
			Published       int              `json:"published"`
			Drafts          int              `json:"drafts"`
			Deleted         int              `json:"deleted"`
			IncludesDeleted bool             `json:"includes_deleted"`
			Items           []ContentSummary `json:"items"`
		}

		response := ListContentResponse{
			Total:           len(contents),
			Published:       publishedCount,
			Drafts:          draftCount,
			Deleted:         deletedCount,
			IncludesDeleted: args.IncludeDeleted,
			Items:           summaries,
		}

		return jsonResult(response), nil, nil
	})

	// Get content
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:  "get_content",
		Title: "Get Content",
		Description: `Get a single content item by ID or path. Returns full content including all field data (title, slug, full_path, data fields, published state).

Prefer path when you know the URL: {"path": "/about"}
Use id when you have the MongoDB ObjectID: {"id": "abc123"}

Set include_rendered=true to also receive the fully rendered HTML output (template + theme header/footer applied). Useful for verifying what visitors see without publishing.

Tip: to preview unsaved edits before publishing, use preview_content instead.`,
		Annotations: &mcp.ToolAnnotations{
			Title:         "Get Content",
			ReadOnlyHint:  true,
			OpenWorldHint: boolPtr(false),
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, args GetContentInput) (*mcp.CallToolResult, any, error) {
		var content *apiclient.Content
		var err error

		if args.ID != "" {
			content, err = s.client.GetContent(ctx, args.ID, args.IncludeRendered)
		} else if args.Path != "" {
			content, err = s.client.GetContentByPath(ctx, args.Path, args.IncludeRendered)
		} else {
			return errorResult(fmt.Errorf("either id or path is required")), nil, nil
		}

		if err != nil {
			return errorResult(err), nil, nil
		}

		return jsonResult(content), nil, nil
	})

	// Create content
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:  "create_content",
		Title: "Create Content",
		Description: `Create a new content item. Requires a template_id, title, slug, and the data fields defined by the template.

Workflow:
1. Call list_templates to find the right template and its field names.
2. Create the content with data matching those fields.
3. Call publish_content to make it live (or set published=true here to do both in one step).

Set use_header=true, use_footer=true, use_theme=true for pages that should use the site layout.
Always include version_comment to make history readable.

Content data fields support rich markup features:
- [[Wikilinks]] and [[Page Title|display text]] — link to other pages by title or path; auto-update when paths change
- [[include:snippet-name]] — embed a named snippet inline (reusable callouts, CTAs, disclaimers)
- #hashtags — mention #tagname anywhere to automatically tag the page
- Markdown fields (type "markdown") — GitHub Flavored Markdown converted to HTML at publish time
Templates can use {{.lc_toc}} in their HTML layout to inject an auto-generated table of contents.`,
		Annotations: &mcp.ToolAnnotations{
			Title:           "Create Content",
			ReadOnlyHint:    false,
			DestructiveHint: boolPtr(false),
			IdempotentHint:  false,
			OpenWorldHint:   boolPtr(false),
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, args CreateContentInput) (*mcp.CallToolResult, any, error) {
		createReq := apiclient.CreateContentRequest{
			TemplateID:      args.TemplateID,
			Title:           args.Title,
			Slug:            args.Slug,
			FolderPath:      args.FolderPath,
			Category:        args.Category,
			Tags:            args.Tags,
			MetaDescription: args.MetaDescription,
			OGImage:         args.OGImage,
			NoIndex:         args.NoIndex,
			Hold:            args.Hold,
			AuthorName:      args.AuthorName,
			AuthorURL:       args.AuthorURL,
			Data:            args.Data,
			Published:       args.Published,
			UseHeader:       args.UseHeader,
			UseFooter:       args.UseFooter,
			UseTheme:        args.UseTheme,
			RawMode:         args.RawMode,
			VersionComment:  args.VersionComment,
			Upsert:          args.Upsert,
		}

		// Sandbox: create the page inside the sandbox fork instead of live.
		forkID, _, active, err := s.sandboxFork(ctx)
		if err != nil {
			return errorResult(err), nil, nil
		}
		if active {
			if args.Upsert {
				return textResult("upsert is not available while an agent sandbox is active — use update_content so the change stays in the sandbox fork"), nil, nil
			}
			createReq.ForkID = forkID
		}

		content, err := s.client.CreateContent(ctx, createReq)
		if err != nil {
			return errorResult(err), nil, nil
		}

		action := "created"
		if args.Upsert && content.UpdatedAt.After(content.CreatedAt) {
			action = "updated"
		}
		return jsonResult(map[string]interface{}{
			"success":   true,
			"id":        content.ID,
			"full_path": content.FullPath,
			"action":    action,
			"message":   fmt.Sprintf("Content '%s' %s successfully", content.Title, action),
		}), nil, nil
	})

	// Update content
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:  "update_content",
		Title: "Update Content",
		Description: `Update an existing content item by ID. Creates a new version automatically. Only send fields you want to change.

For partial data updates, only the keys you include in "data" are changed — existing keys are preserved (merge semantics).
Use clear_fields: ["field1", "field2"] to explicitly set fields to empty string.
Set dry_run: true to validate the update without saving.
To update by URL path instead of ID, use update_content_by_path.
Always include version_comment so the version history is useful.

Up to 20 concurrent update_content calls are safe. For larger batches (>20 items), prefer bulk_update_content instead.

Example: {"id": "abc123", "data": {"body": "<p>Updated text</p>"}, "version_comment": "Revised intro paragraph"}

Content data fields support rich markup features:
- [[Wikilinks]] and [[Page Title|display text]] — link to other pages by title or path; auto-update when paths change
- [[include:snippet-name]] — embed a named snippet inline (reusable callouts, CTAs, disclaimers)
- #hashtags — mention #tagname anywhere to automatically tag the page
- Markdown fields (type "markdown") — GitHub Flavored Markdown converted to HTML at publish time
Templates can use {{.lc_toc}} in their HTML layout to inject an auto-generated table of contents.`,
		Annotations: &mcp.ToolAnnotations{
			Title:           "Update Content",
			ReadOnlyHint:    false,
			DestructiveHint: boolPtr(true),
			IdempotentHint:  true,
			OpenWorldHint:   boolPtr(false),
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, args UpdateContentInput) (*mcp.CallToolResult, any, error) {
		updates := make(apiclient.UpdateContentRequest)

		if args.TemplateID != "" {
			updates["template_id"] = args.TemplateID
		}
		if args.Title != "" {
			updates["title"] = args.Title
		}
		if args.Slug != "" {
			updates["slug"] = args.Slug
		}
		if args.FolderPath != "" {
			updates["folder_path"] = args.FolderPath
		}
		if args.Category != "" {
			updates["category"] = args.Category
		}
		if args.Tags != nil {
			updates["tags"] = args.Tags
		}
		if args.MetaDescription != "" {
			updates["meta_description"] = args.MetaDescription
		}
		if args.OGImage != "" {
			updates["og_image"] = args.OGImage
		}
		if args.NoIndex != nil {
			updates["noindex"] = *args.NoIndex
		}
		if args.Hold != nil {
			updates["hold"] = *args.Hold
		}
		if args.AuthorName != nil {
			updates["author_name"] = *args.AuthorName
		}
		if args.AuthorURL != nil {
			updates["author_url"] = *args.AuthorURL
		}
		if args.UseHeader || args.SetUseHeader {
			updates["use_header"] = args.UseHeader
		}
		if args.UseFooter || args.SetUseFooter {
			updates["use_footer"] = args.UseFooter
		}
		if args.UseTheme || args.SetUseTheme {
			updates["use_theme"] = args.UseTheme
		}
		if args.RawMode || args.SetRawMode {
			updates["raw_mode"] = args.RawMode
		}
		if args.VersionComment != "" {
			updates["version_comment"] = args.VersionComment
		}

		// Sandbox: redirect the write to the fork copy (copy-on-write).
		targetID, err := s.sandboxTargetForContent(ctx, args.ID)
		if err != nil {
			return errorResult(err), nil, nil
		}

		// For data fields, merge with existing content data
		if args.Data != nil || len(args.ClearFields) > 0 {
			existing, err := s.client.GetContent(ctx, targetID, false)
			if err != nil {
				return errorResult(err), nil, nil
			}
			mergedData := existing.Data
			if mergedData == nil {
				mergedData = make(map[string]interface{})
			}
			for k, v := range args.Data {
				mergedData[k] = v
			}
			for _, field := range args.ClearFields {
				mergedData[field] = ""
			}
			updates["data"] = mergedData
		}

		if args.DryRun {
			fieldsChanged := make([]string, 0, len(updates))
			for k := range updates {
				if k != "version_comment" {
					fieldsChanged = append(fieldsChanged, k)
				}
			}
			return jsonResult(map[string]interface{}{
				"dry_run":        true,
				"valid":          true,
				"id":             targetID,
				"fields_changed": fieldsChanged,
				"warnings":       []string{},
			}), nil, nil
		}

		content, err := s.client.UpdateContent(ctx, targetID, updates)
		if err != nil {
			return errorResult(err), nil, nil
		}

		return jsonResult(map[string]interface{}{
			"success":   true,
			"id":        content.ID,
			"full_path": content.FullPath,
			"message":   fmt.Sprintf("Content '%s' updated successfully", content.Title),
		}), nil, nil
	})

	// Publish content
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "publish_content",
		Title:       "Publish Content",
		Description: "Publish a content item, making it visible on the public site. Generates the static HTML page.",
		Annotations: &mcp.ToolAnnotations{
			Title:           "Publish Content",
			ReadOnlyHint:    false,
			DestructiveHint: boolPtr(true),
			IdempotentHint:  true,
			OpenWorldHint:   boolPtr(false),
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, args ContentIDInput) (*mcp.CallToolResult, any, error) {
		if err := s.client.PublishContent(ctx, args.ID); err != nil {
			return errorResult(err), nil, nil
		}
		return textResult(fmt.Sprintf("Content %s published successfully", args.ID)), nil, nil
	})

	// Unpublish content
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "unpublish_content",
		Title:       "Unpublish Content",
		Description: "Unpublish a content item, removing it from the public site. Removes the static HTML page.",
		Annotations: &mcp.ToolAnnotations{
			Title:           "Unpublish Content",
			ReadOnlyHint:    false,
			DestructiveHint: boolPtr(true),
			IdempotentHint:  true,
			OpenWorldHint:   boolPtr(false),
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, args ContentIDInput) (*mcp.CallToolResult, any, error) {
		if err := s.client.UnpublishContent(ctx, args.ID); err != nil {
			return errorResult(err), nil, nil
		}
		return textResult(fmt.Sprintf("Content %s unpublished successfully", args.ID)), nil, nil
	})

	// Delete content (soft delete)
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "delete_content",
		Title:       "Delete Content",
		Description: "Soft-delete a content item. The content can be restored later. Removes the static HTML page.",
		Annotations: &mcp.ToolAnnotations{
			Title:           "Delete Content",
			ReadOnlyHint:    false,
			DestructiveHint: boolPtr(true),
			IdempotentHint:  true,
			OpenWorldHint:   boolPtr(false),
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, args ContentIDInput) (*mcp.CallToolResult, any, error) {
		if err := s.client.DeleteContent(ctx, args.ID); err != nil {
			return errorResult(err), nil, nil
		}
		return textResult(fmt.Sprintf("Content %s deleted successfully", args.ID)), nil, nil
	})

	// Restore content
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "restore_content",
		Title:       "Restore Content",
		Description: "Restore a soft-deleted content item. Regenerates static page if content was published.",
		Annotations: &mcp.ToolAnnotations{
			Title:           "Restore Content",
			ReadOnlyHint:    false,
			DestructiveHint: boolPtr(false),
			IdempotentHint:  true,
			OpenWorldHint:   boolPtr(false),
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, args ContentIDInput) (*mcp.CallToolResult, any, error) {
		if err := s.client.RestoreContent(ctx, args.ID); err != nil {
			return errorResult(err), nil, nil
		}
		return textResult(fmt.Sprintf("Content %s restored successfully", args.ID)), nil, nil
	})

	// Get versions
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "get_content_versions",
		Title:       "Get Content Versions",
		Description: "Get the version history for a content item. Returns list of versions with timestamps.",
		Annotations: &mcp.ToolAnnotations{
			Title:         "Get Content Versions",
			ReadOnlyHint:  true,
			OpenWorldHint: boolPtr(false),
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, args GetVersionsInput) (*mcp.CallToolResult, any, error) {
		versions, err := s.client.GetContentVersions(ctx, args.ContentID)
		if err != nil {
			return errorResult(err), nil, nil
		}

		type VersionSummary struct {
			Version   int    `json:"version"`
			Title     string `json:"title"`
			Slug      string `json:"slug"`
			FullPath  string `json:"full_path"`
			Published bool   `json:"published"`
			Comment   string `json:"comment,omitempty"`
			CreatedAt string `json:"created_at"`
		}

		summaries := make([]VersionSummary, len(versions))
		for i, v := range versions {
			summaries[i] = VersionSummary{
				Version:   v.Version,
				Title:     v.Title,
				Slug:      v.Slug,
				FullPath:  v.FullPath,
				Published: v.Published,
				Comment:   v.Comment,
				CreatedAt: v.CreatedAt.Format("2006-01-02 15:04:05"),
			}
		}

		return jsonResult(summaries), nil, nil
	})

	// Get specific version
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "get_content_version",
		Title:       "Get Content Version",
		Description: "Get a specific version of a content item with full field data.",
		Annotations: &mcp.ToolAnnotations{
			Title:         "Get Content Version",
			ReadOnlyHint:  true,
			OpenWorldHint: boolPtr(false),
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, args GetVersionInput) (*mcp.CallToolResult, any, error) {
		version, err := s.client.GetContentVersion(ctx, args.ContentID, args.Version)
		if err != nil {
			return errorResult(err), nil, nil
		}
		return jsonResult(version), nil, nil
	})

	// Revert to version
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "revert_to_version",
		Title:       "Revert to Version",
		Description: "Revert content to a previous version. Creates a new version with the old data.",
		Annotations: &mcp.ToolAnnotations{
			Title:           "Revert to Version",
			ReadOnlyHint:    false,
			DestructiveHint: boolPtr(true),
			IdempotentHint:  false,
			OpenWorldHint:   boolPtr(false),
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, args RevertToVersionInput) (*mcp.CallToolResult, any, error) {
		if err := s.client.RevertContentVersion(ctx, args.ContentID, args.Version, args.VersionComment); err != nil {
			return errorResult(err), nil, nil
		}
		return textResult(fmt.Sprintf("Content reverted to version %d successfully", args.Version)), nil, nil
	})

	// Batch publish
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:  "publish_multiple",
		Title: "Publish Multiple",
		Description: `Publish multiple content items in a single call. Use this instead of calling publish_content in a loop.

Examples:
- Publish specific pages: {"ids": ["abc123", "def456"]}
- Publish all drafts at once: {"publish_all_drafts": true}

Held pages and fork copies are never published by this tool: they come back in "skipped" as {id, reason}. publish_all_drafts ignores fork copies entirely.

Returns published IDs, skipped items, and any failures.`,
		Annotations: &mcp.ToolAnnotations{
			Title:           "Publish Multiple",
			ReadOnlyHint:    false,
			DestructiveHint: boolPtr(true),
			IdempotentHint:  true,
			OpenWorldHint:   boolPtr(false),
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, args BatchPublishInput) (*mcp.CallToolResult, any, error) {
		result, err := s.client.BatchPublishContent(ctx, args.IDs, args.PublishAllDrafts)
		if err != nil {
			return errorResult(err), nil, nil
		}
		return jsonResult(result), nil, nil
	})

	// Preview content
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:  "preview_content",
		Title: "Preview Content",
		Description: `Render a content item's HTML without saving or publishing. Use this to verify what a page will look like before publishing.

Also accepts optional title/data overrides to preview unsaved edits:
{"id": "abc123", "data": {"body": "<p>New text</p>"}}

Returns rendered_html and any warnings (missing required fields, unclosed tags, unresolved placeholders).`,
		Annotations: &mcp.ToolAnnotations{
			Title:         "Preview Content",
			ReadOnlyHint:  true,
			OpenWorldHint: boolPtr(false),
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, args PreviewContentInput) (*mcp.CallToolResult, any, error) {
		overrides := map[string]interface{}{}
		if args.Title != "" {
			overrides["title"] = args.Title
		}
		if args.Data != nil {
			overrides["data"] = args.Data
		}
		result, err := s.client.PreviewContent(ctx, args.ID, overrides)
		if err != nil {
			return errorResult(err), nil, nil
		}
		return jsonResult(result), nil, nil
	})

	// Update content by path
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:  "update_content_by_path",
		Title: "Update Content by Path",
		Description: `Update content identified by its URL path instead of its ID. Useful when you know the page URL but not the MongoDB ID.

Example: {"path": "/about", "title": "About Us", "data": {"body": "<p>Updated content</p>"}}

The path always resolves to the live page, never to a fork copy (inside an agent sandbox the write goes to the sandbox copy as usual).

Only the fields you provide are changed. Always include a version_comment describing what changed.`,
		Annotations: &mcp.ToolAnnotations{
			Title:           "Update Content by Path",
			ReadOnlyHint:    false,
			DestructiveHint: boolPtr(true),
			IdempotentHint:  true,
			OpenWorldHint:   boolPtr(false),
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, args UpdateContentByPathInput) (*mcp.CallToolResult, any, error) {
		// Sandbox: copy-on-write the page into the fork and update the copy.
		sandboxID, err := s.sandboxTargetForPath(ctx, args.Path)
		if err != nil {
			return errorResult(err), nil, nil
		}

		updates := map[string]interface{}{}
		if args.Title != "" {
			updates["title"] = args.Title
		}
		if args.Data != nil {
			// Merge data on top of existing content
			var existing *apiclient.Content
			var err error
			if sandboxID != "" {
				existing, err = s.client.GetContent(ctx, sandboxID, false)
			} else {
				existing, err = s.client.GetContentByPath(ctx, args.Path, false)
			}
			if err != nil {
				return errorResult(err), nil, nil
			}
			mergedData := existing.Data
			if mergedData == nil {
				mergedData = make(map[string]interface{})
			}
			for k, v := range args.Data {
				mergedData[k] = v
			}
			updates["data"] = mergedData
		}
		if args.Category != "" {
			updates["category"] = args.Category
		}
		if args.Tags != nil {
			updates["tags"] = args.Tags
		}
		if args.MetaDescription != "" {
			updates["meta_description"] = args.MetaDescription
		}
		if args.OGImage != "" {
			updates["og_image"] = args.OGImage
		}
		if args.NoIndex != nil {
			updates["noindex"] = *args.NoIndex
		}
		if args.Hold != nil {
			updates["hold"] = *args.Hold
		}
		if args.AuthorName != nil {
			updates["author_name"] = *args.AuthorName
		}
		if args.AuthorURL != nil {
			updates["author_url"] = *args.AuthorURL
		}
		if args.Published != nil {
			updates["published"] = *args.Published
		}
		if args.VersionComment != "" {
			updates["version_comment"] = args.VersionComment
		}
		var content *apiclient.Content
		if sandboxID != "" {
			content, err = s.client.UpdateContent(ctx, sandboxID, updates)
		} else {
			content, err = s.client.UpdateContentByPath(ctx, args.Path, updates)
		}
		if err != nil {
			return errorResult(err), nil, nil
		}
		return jsonResult(map[string]interface{}{
			"success":   true,
			"id":        content.ID,
			"full_path": content.FullPath,
			"message":   fmt.Sprintf("Content at '%s' updated successfully", args.Path),
		}), nil, nil
	})

	// Scoped search-and-replace preview
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:  "scoped_search_replace_preview",
		Title: "Scoped Search Replace Preview",
		Description: `Preview a search-and-replace limited to a subset of pages. Safer than site-wide replacement.

Scope options (all optional — leave blank to match all pages):
- content_ids: specific page IDs
- folder_path: pages under /blog, /docs, etc.
- template_name: pages using "Concept Page", "Blog Post", etc.
- category: pages with a matching category

Only live pages are searched: fork copies named in content_ids come back in "skipped" as {id, reason}.

Example: {"search": "old text", "replace": "new text", "folder_path": "/blog"}

Always run preview before execute.`,
		Annotations: &mcp.ToolAnnotations{
			Title:         "Scoped Search Replace Preview",
			ReadOnlyHint:  true,
			OpenWorldHint: boolPtr(false),
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, args ScopedSearchReplaceInput) (*mcp.CallToolResult, any, error) {
		scope := apiclient.ScopedSearchReplaceScope{
			ContentIDs:   args.ContentIDs,
			FolderPath:   args.FolderPath,
			TemplateName: args.TemplateName,
			Category:     args.Category,
		}
		result, err := s.client.ScopedSearchReplacePreview(ctx, args.Search, args.Replace, args.Regex, scope)
		if err != nil {
			return errorResult(err), nil, nil
		}
		return jsonResult(result), nil, nil
	})

	// Get backlinks
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:  "get_backlinks",
		Title: "Get Backlinks",
		Description: `Find all published pages that link to the given URL path. Links are tracked automatically whenever a page is published — both [[Wikilinks]] and ordinary <a href="..."> links in content fields are indexed.

Use this to discover which pages reference a given page (wiki-style backlink graph), assess the impact of deleting or renaming a page, or find orphaned pages with no inbound links.

Example: {"path": "/about"} returns every published page whose content contains a link to /about.`,
		Annotations: &mcp.ToolAnnotations{
			Title:         "Get Backlinks",
			ReadOnlyHint:  true,
			OpenWorldHint: boolPtr(false),
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, args GetBacklinksInput) (*mcp.CallToolResult, any, error) {
		if args.Path == "" {
			return errorResult(fmt.Errorf("path is required")), nil, nil
		}
		backlinks, err := s.client.GetBacklinks(ctx, args.Path)
		if err != nil {
			return errorResult(err), nil, nil
		}

		type BacklinkSummary struct {
			ID       string `json:"id"`
			Title    string `json:"title"`
			FullPath string `json:"full_path"`
		}
		summaries := make([]BacklinkSummary, len(backlinks))
		for i, c := range backlinks {
			summaries[i] = BacklinkSummary{
				ID:       c.ID,
				Title:    c.Title,
				FullPath: c.FullPath,
			}
		}
		return jsonResult(map[string]interface{}{
			"target":    args.Path,
			"count":     len(summaries),
			"backlinks": summaries,
		}), nil, nil
	})

	// Scoped search-and-replace execute
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:  "scoped_search_replace_execute",
		Title: "Scoped Search Replace Execute",
		Description: `Execute a search-and-replace limited to a subset of pages. ALWAYS run scoped_search_replace_preview first and show results to the user before executing.

Scope options (all optional):
- content_ids, folder_path, template_name, category

Set auto_republish: true to immediately re-publish all previously-published pages after updating them, collapsing the execute + publish_multiple flow into one call.

Only live pages are changed: fork copies named in content_ids come back in "skipped" as {id, reason}.

Example: {"search": "old text", "replace": "new text", "folder_path": "/blog", "auto_republish": true, "version_comment": "Updated old references"}`,
		Annotations: &mcp.ToolAnnotations{
			Title:           "Scoped Search Replace Execute",
			ReadOnlyHint:    false,
			DestructiveHint: boolPtr(true),
			IdempotentHint:  false,
			OpenWorldHint:   boolPtr(false),
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, args ScopedSearchReplaceInput) (*mcp.CallToolResult, any, error) {
		scope := apiclient.ScopedSearchReplaceScope{
			ContentIDs:   args.ContentIDs,
			FolderPath:   args.FolderPath,
			TemplateName: args.TemplateName,
			Category:     args.Category,
		}
		result, err := s.client.ScopedSearchReplaceExecute(ctx, args.Search, args.Replace, args.VersionComment, args.Regex, args.AutoRepublish, scope)
		if err != nil {
			return errorResult(err), nil, nil
		}
		return jsonResult(result), nil, nil
	})

	// Bulk create content
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:  "bulk_create_content",
		Title: "Bulk Create Content",
		Description: `Create up to 100 content items in a single call using efficient batch insert.

Items are inserted via MongoDB InsertMany for maximum throughput. If one item fails (e.g., duplicate path), the rest continue.
Published items get their static HTML generated in parallel (up to 10 concurrent).

Set upsert: true to update existing pages at the same path instead of failing on duplicates.

Returns: total attempted, succeeded, failed counts, and per-item {id, full_path, success, error} details.`,
		Annotations: &mcp.ToolAnnotations{
			Title:           "Bulk Create Content",
			ReadOnlyHint:    false,
			DestructiveHint: boolPtr(false),
			IdempotentHint:  false,
			OpenWorldHint:   boolPtr(false),
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, args BulkCreateContentInput) (*mcp.CallToolResult, any, error) {
		result, err := s.client.BulkCreateContent(ctx, args)
		if err != nil {
			return errorResult(err), nil, nil
		}
		return jsonResult(result), nil, nil
	})

	// Bulk update content
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:  "bulk_update_content",
		Title: "Bulk Update Content",
		Description: `Update up to 100 content items in a single call. Use instead of calling update_content in a loop.

Each update in the array specifies the content ID and only the fields you want to change (merge semantics on data).
Use clear_fields to explicitly clear field values to empty string.
Set dry_run: true to validate all IDs exist without committing changes.

Returns: total attempted, succeeded, failed counts, and per-item success/error details.

Tip: call list_content with include_data: true first to get IDs + current field values, transform as needed, then submit here.
Recommended batch size: up to 50 per call for optimal performance.`,
		Annotations: &mcp.ToolAnnotations{
			Title:           "Bulk Update Content",
			ReadOnlyHint:    false,
			DestructiveHint: boolPtr(true),
			IdempotentHint:  false,
			OpenWorldHint:   boolPtr(false),
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, args BulkUpdateContentInput) (*mcp.CallToolResult, any, error) {
		result, err := s.client.BulkUpdateContent(ctx, args)
		if err != nil {
			return errorResult(err), nil, nil
		}
		return jsonResult(result), nil, nil
	})

	// Bulk field operation
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:  "bulk_field_operation",
		Title: "Bulk Field Operation",
		Description: `Apply a single field operation to all matching content pages in one call.

Operations:
- clear: set field to empty string
- set: replace field value with a fixed string
- prepend: add text before existing field value
- append: add text after existing field value
- wrap: surround existing value with before/after strings

Use scope filters to limit which pages are affected (content_ids, folder_path, template_name, category).
Set dry_run: true to preview which pages would be changed without saving.
Only live pages are changed: fork copies named in content_ids come back in "skipped" as {id, reason}.

Example: {"operation": "prepend", "field": "disclaimer", "value": "<p>Note: </p>", "template_name": "Blog Post"}`,
		Annotations: &mcp.ToolAnnotations{
			Title:           "Bulk Field Operation",
			ReadOnlyHint:    false,
			DestructiveHint: boolPtr(true),
			IdempotentHint:  false,
			OpenWorldHint:   boolPtr(false),
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, args BulkFieldOperationInput) (*mcp.CallToolResult, any, error) {
		body := map[string]interface{}{
			"operation":       args.Operation,
			"field":           args.Field,
			"value":           args.Value,
			"before":          args.Before,
			"after":           args.After,
			"version_comment": args.VersionComment,
			"dry_run":         args.DryRun,
			"scope": map[string]interface{}{
				"content_ids":   args.ContentIDs,
				"folder_path":   args.FolderPath,
				"template_name": args.TemplateName,
				"category":      args.Category,
			},
		}
		result, err := s.client.BulkFieldOperation(ctx, body)
		if err != nil {
			return errorResult(err), nil, nil
		}
		return jsonResult(result), nil, nil
	})

	// Export content
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:  "export_content",
		Title: "Export Content",
		Description: `Export content items with their full field data as a structured JSON array.

Use for batch transformations: export → transform externally → re-import via bulk_update_content.

Scope filters (all optional):
- template_name: most useful for bulk workflows, e.g. "Concept Page" or "Blog Post"
- category, folder_path, content_ids: narrower scoping options

Use fields: ["field1", "field2"] to include only specific data fields instead of all fields.

Only live pages are exported: fork copies named in content_ids come back in "skipped" as {id, reason}.

Returns: total count and array of items with id, title, slug, full_path, template_name, published, and data.`,
		Annotations: &mcp.ToolAnnotations{
			Title:         "Export Content",
			ReadOnlyHint:  true,
			OpenWorldHint: boolPtr(false),
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, args ExportContentInput) (*mcp.CallToolResult, any, error) {
		result, err := s.client.ExportContent(ctx, args)
		if err != nil {
			return errorResult(err), nil, nil
		}
		return jsonResult(result), nil, nil
	})
}
