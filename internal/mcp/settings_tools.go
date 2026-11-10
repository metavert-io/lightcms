package mcp

import (
	"context"
	"fmt"

	"github.com/metavert-io/lightcms/v7/internal/apiclient"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Theme settings input types
type UpdateThemeInput struct {
	PrimaryColor    string `json:"primary_color,omitempty" jsonschema:"Primary theme color (hex)"`
	SecondaryColor  string `json:"secondary_color,omitempty" jsonschema:"Secondary theme color (hex)"`
	AccentColor     string `json:"accent_color,omitempty" jsonschema:"Accent theme color (hex)"`
	BackgroundColor string `json:"background_color,omitempty" jsonschema:"Background color (hex)"`
	TextColor       string `json:"text_color,omitempty" jsonschema:"Text color (hex)"`
	FontFamily      string `json:"font_family,omitempty" jsonschema:"Body font family CSS value"`
	HeadingFont     string `json:"heading_font,omitempty" jsonschema:"Heading font family CSS value"`
	BorderRadius    string `json:"border_radius,omitempty" jsonschema:"Border radius CSS value"`
	CustomCSS       string `json:"custom_css,omitempty" jsonschema:"Additional custom CSS"`
	SiteName        string `json:"site_name,omitempty" jsonschema:"Site name"`
	SiteTagline     string `json:"site_tagline,omitempty" jsonschema:"Site tagline"`
	LogoURL         string `json:"logo_url,omitempty" jsonschema:"Logo image URL"`
	HeadHTML        string `json:"head_html,omitempty" jsonschema:"Custom HTML for <head> section"`
	HeaderHTML      string `json:"header_html,omitempty" jsonschema:"Custom header HTML (changing regenerates all content)"`
	FooterHTML      string `json:"footer_html,omitempty" jsonschema:"Custom footer HTML (changing regenerates all content)"`
}

// Theme version input types
type GetThemeVersionInput struct {
	Version int `json:"version" jsonschema:"Version number,required"`
}

type RevertThemeVersionInput struct {
	Version        int    `json:"version" jsonschema:"Version number to revert to,required"`
	VersionComment string `json:"version_comment,omitempty" jsonschema:"Optional comment for the revert (e.g., 'Reverted to v3')"`
}

type PinThemeVersionInput struct {
	Version int `json:"version" jsonschema:"Theme version number to pin/unpin,required"`
}

// Site config input types
type UpdateSiteConfigInput struct {
	TitleTemplate        string `json:"title_template,omitempty" jsonschema:"Page title template with {{title}} and {{site_name}} placeholders"`
	TitleTemplateNoTitle string `json:"title_template_no_title,omitempty" jsonschema:"Title template when page has no title"`
}

// Redirect input types
type CreateRedirectInput struct {
	FromPath    string `json:"from_path" jsonschema:"Source path (e.g., /old-page),required"`
	ToPath      string `json:"to_path" jsonschema:"Destination path or URL (e.g., /new-page),required"`
	StatusCode  int    `json:"status_code,omitempty" jsonschema:"301 (permanent) or 302 (temporary), defaults to 301"`
	Description string `json:"description,omitempty" jsonschema:"Optional description/note"`
}

type UpdateRedirectInput struct {
	ID          string `json:"id" jsonschema:"Redirect ID (MongoDB ObjectID),required"`
	FromPath    string `json:"from_path,omitempty" jsonschema:"Source path"`
	ToPath      string `json:"to_path,omitempty" jsonschema:"Destination path or URL"`
	StatusCode  int    `json:"status_code,omitempty" jsonschema:"301 or 302"`
	Description string `json:"description,omitempty" jsonschema:"Optional description"`
}

type RedirectIDInput struct {
	ID string `json:"id" jsonschema:"Redirect ID (MongoDB ObjectID),required"`
}

// Folder input types
type CreateFolderInput struct {
	Name     string `json:"name" jsonschema:"Display name for the folder,required"`
	Slug     string `json:"slug" jsonschema:"URL segment for the folder,required"`
	ParentID string `json:"parent_id,omitempty" jsonschema:"Parent folder ID for nested folders"`
}

type FolderIDInput struct {
	ID string `json:"id" jsonschema:"Folder ID (MongoDB ObjectID),required"`
}

// Collection input types
type CreateCollectionInput struct {
	Name         string `json:"name" jsonschema:"Collection name,required"`
	Slug         string `json:"slug" jsonschema:"Collection URL slug,required"`
	Description  string `json:"description,omitempty" jsonschema:"Collection description"`
	Category     string `json:"category,omitempty" jsonschema:"Content category to include"`
	SortField    string `json:"sort_field,omitempty" jsonschema:"Field to sort by"`
	SortOrder    string `json:"sort_order,omitempty" jsonschema:"Sort order: asc or desc"`
	ItemTemplate string `json:"item_template,omitempty" jsonschema:"HTML template for each item"`
	PageTemplate string `json:"page_template,omitempty" jsonschema:"HTML template for collection page"`
	ItemsPerPage int    `json:"items_per_page,omitempty" jsonschema:"Items per page for pagination"`
}

type UpdateCollectionInput struct {
	ID           string `json:"id" jsonschema:"Collection ID (MongoDB ObjectID),required"`
	Name         string `json:"name,omitempty" jsonschema:"Collection name"`
	Slug         string `json:"slug,omitempty" jsonschema:"Collection URL slug"`
	Description  string `json:"description,omitempty" jsonschema:"Collection description"`
	Category     string `json:"category,omitempty" jsonschema:"Content category to include"`
	SortField    string `json:"sort_field,omitempty" jsonschema:"Field to sort by"`
	SortOrder    string `json:"sort_order,omitempty" jsonschema:"Sort order: asc or desc"`
	ItemTemplate string `json:"item_template,omitempty" jsonschema:"HTML template for each item"`
	PageTemplate string `json:"page_template,omitempty" jsonschema:"HTML template for collection page"`
	ItemsPerPage int    `json:"items_per_page,omitempty" jsonschema:"Items per page for pagination"`
}

type CollectionIDInput struct {
	ID string `json:"id" jsonschema:"Collection ID (MongoDB ObjectID),required"`
}

func (s *Server) registerSettingsTools() {
	// === Theme Settings ===

	// Get theme
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "get_theme",
		Title:       "Get Theme",
		Description: "Get current theme settings including colors, fonts, and custom HTML for header/footer.",
		Annotations: &mcp.ToolAnnotations{
			Title:         "Get Theme",
			ReadOnlyHint:  true,
			OpenWorldHint: boolPtr(false),
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct{}) (*mcp.CallToolResult, any, error) {
		theme, err := s.client.GetTheme(ctx)
		if err != nil {
			return errorResult(err), nil, nil
		}
		return jsonResult(theme), nil, nil
	})

	// Update theme
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:  "update_theme",
		Title: "Update Theme",
		Description: `Update theme settings. Only the fields you provide are changed — all other settings are preserved (partial update, safe to call without get_theme first).

Changing header_html or footer_html triggers background regeneration of all published pages.
Changing colors, fonts, or custom_css does NOT require content regeneration.

Use pin_theme_version to protect important milestones before making major changes.`,
		Annotations: &mcp.ToolAnnotations{
			Title:           "Update Theme",
			ReadOnlyHint:    false,
			DestructiveHint: boolPtr(true),
			IdempotentHint:  true,
			OpenWorldHint:   boolPtr(false),
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, args UpdateThemeInput) (*mcp.CallToolResult, any, error) {
		updates := make(map[string]interface{})

		if args.PrimaryColor != "" {
			updates["primary_color"] = args.PrimaryColor
		}
		if args.SecondaryColor != "" {
			updates["secondary_color"] = args.SecondaryColor
		}
		if args.AccentColor != "" {
			updates["accent_color"] = args.AccentColor
		}
		if args.BackgroundColor != "" {
			updates["background_color"] = args.BackgroundColor
		}
		if args.TextColor != "" {
			updates["text_color"] = args.TextColor
		}
		if args.FontFamily != "" {
			updates["font_family"] = args.FontFamily
		}
		if args.HeadingFont != "" {
			updates["heading_font"] = args.HeadingFont
		}
		if args.BorderRadius != "" {
			updates["border_radius"] = args.BorderRadius
		}
		if args.CustomCSS != "" {
			updates["custom_css"] = args.CustomCSS
		}
		if args.SiteName != "" {
			updates["site_name"] = args.SiteName
		}
		if args.SiteTagline != "" {
			updates["site_tagline"] = args.SiteTagline
		}
		if args.LogoURL != "" {
			updates["logo_url"] = args.LogoURL
		}
		if args.HeadHTML != "" {
			updates["head_html"] = args.HeadHTML
		}
		if args.HeaderHTML != "" {
			updates["header_html"] = args.HeaderHTML
		}
		if args.FooterHTML != "" {
			updates["footer_html"] = args.FooterHTML
		}

		if _, err := s.client.UpdateTheme(ctx, updates); err != nil {
			return errorResult(err), nil, nil
		}

		return textResult("Theme updated successfully"), nil, nil
	})

	// Get theme versions
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "get_theme_versions",
		Title:       "Get Theme Versions",
		Description: "Get the version history for theme settings. Returns list of versions with timestamps.",
		Annotations: &mcp.ToolAnnotations{
			Title:         "Get Theme Versions",
			ReadOnlyHint:  true,
			OpenWorldHint: boolPtr(false),
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct{}) (*mcp.CallToolResult, any, error) {
		versions, err := s.client.ListThemeVersions(ctx)
		if err != nil {
			return errorResult(err), nil, nil
		}

		type VersionSummary struct {
			Version   int    `json:"version"`
			SiteName  string `json:"site_name"`
			Comment   string `json:"comment,omitempty"`
			CreatedAt string `json:"created_at"`
		}

		summaries := make([]VersionSummary, len(versions))
		for i, v := range versions {
			summaries[i] = VersionSummary{
				Version:   v.Version,
				SiteName:  v.SiteName,
				Comment:   v.Comment,
				CreatedAt: v.CreatedAt.Format("2006-01-02 15:04:05"),
			}
		}

		return jsonResult(summaries), nil, nil
	})

	// Get specific theme version
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "get_theme_version",
		Title:       "Get Theme Version",
		Description: "Get a specific version of theme settings with full data.",
		Annotations: &mcp.ToolAnnotations{
			Title:         "Get Theme Version",
			ReadOnlyHint:  true,
			OpenWorldHint: boolPtr(false),
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, args GetThemeVersionInput) (*mcp.CallToolResult, any, error) {
		version, err := s.client.GetThemeVersion(ctx, args.Version)
		if err != nil {
			return errorResult(err), nil, nil
		}
		return jsonResult(version), nil, nil
	})

	// Revert to theme version
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "revert_theme_to_version",
		Title:       "Revert Theme to Version",
		Description: "Revert theme to a previous version. Creates a new version with the old data.",
		Annotations: &mcp.ToolAnnotations{
			Title:           "Revert Theme to Version",
			ReadOnlyHint:    false,
			DestructiveHint: boolPtr(true),
			IdempotentHint:  false,
			OpenWorldHint:   boolPtr(false),
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, args RevertThemeVersionInput) (*mcp.CallToolResult, any, error) {
		if err := s.client.RevertThemeVersion(ctx, args.Version, args.VersionComment); err != nil {
			return errorResult(err), nil, nil
		}
		return textResult(fmt.Sprintf("Theme reverted to version %d successfully", args.Version)), nil, nil
	})

	// Pin theme version
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:  "pin_theme_version",
		Title: "Pin Theme Version",
		Description: `Lock a theme version so it is protected from automatic pruning or accidental overwrite. Pinned versions are marked with locked=true in get_theme_versions.

Use this to preserve milestone theme states (e.g., after a major redesign) before making further changes.

Example: {"version": 5}`,
		Annotations: &mcp.ToolAnnotations{
			Title:           "Pin Theme Version",
			ReadOnlyHint:    false,
			DestructiveHint: boolPtr(false),
			IdempotentHint:  true,
			OpenWorldHint:   boolPtr(false),
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, args PinThemeVersionInput) (*mcp.CallToolResult, any, error) {
		if err := s.client.PinThemeVersion(ctx, args.Version); err != nil {
			return errorResult(err), nil, nil
		}
		return textResult(fmt.Sprintf("Theme version %d pinned (locked)", args.Version)), nil, nil
	})

	// Unpin theme version
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:  "unpin_theme_version",
		Title: "Unpin Theme Version",
		Description: `Remove the lock from a previously pinned theme version.

Example: {"version": 5}`,
		Annotations: &mcp.ToolAnnotations{
			Title:           "Unpin Theme Version",
			ReadOnlyHint:    false,
			DestructiveHint: boolPtr(false),
			IdempotentHint:  true,
			OpenWorldHint:   boolPtr(false),
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, args PinThemeVersionInput) (*mcp.CallToolResult, any, error) {
		if err := s.client.UnpinThemeVersion(ctx, args.Version); err != nil {
			return errorResult(err), nil, nil
		}
		return textResult(fmt.Sprintf("Theme version %d unpinned", args.Version)), nil, nil
	})

	// === Site Config ===

	// Get site config
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "get_site_config",
		Title:       "Get Site Config",
		Description: "Get site configuration including title templates.",
		Annotations: &mcp.ToolAnnotations{
			Title:         "Get Site Config",
			ReadOnlyHint:  true,
			OpenWorldHint: boolPtr(false),
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct{}) (*mcp.CallToolResult, any, error) {
		config, err := s.client.GetSiteConfig(ctx)
		if err != nil {
			return errorResult(err), nil, nil
		}
		return jsonResult(config), nil, nil
	})

	// Update site config
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "update_site_config",
		Title:       "Update Site Config",
		Description: "Update site configuration. Title templates support {{title}} and {{site_name}} placeholders.",
		Annotations: &mcp.ToolAnnotations{
			Title:           "Update Site Config",
			ReadOnlyHint:    false,
			DestructiveHint: boolPtr(true),
			IdempotentHint:  true,
			OpenWorldHint:   boolPtr(false),
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, args UpdateSiteConfigInput) (*mcp.CallToolResult, any, error) {
		updates := make(map[string]interface{})

		if args.TitleTemplate != "" {
			updates["title_template"] = args.TitleTemplate
		}
		if args.TitleTemplateNoTitle != "" {
			updates["title_template_no_title"] = args.TitleTemplateNoTitle
		}

		if _, err := s.client.UpdateSiteConfig(ctx, updates); err != nil {
			return errorResult(err), nil, nil
		}

		return textResult("Site config updated successfully"), nil, nil
	})

	// === Redirects ===

	// List redirects
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "list_redirects",
		Title:       "List Redirects",
		Description: "List all URL redirects configured for the site.",
		Annotations: &mcp.ToolAnnotations{
			Title:         "List Redirects",
			ReadOnlyHint:  true,
			OpenWorldHint: boolPtr(false),
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct{}) (*mcp.CallToolResult, any, error) {
		redirects, err := s.client.ListRedirects(ctx)
		if err != nil {
			return errorResult(err), nil, nil
		}
		return jsonResult(redirects), nil, nil
	})

	// Create redirect
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "create_redirect",
		Title:       "Create Redirect",
		Description: "Create a new URL redirect. Use 301 for permanent redirects, 302 for temporary.",
		Annotations: &mcp.ToolAnnotations{
			Title:           "Create Redirect",
			ReadOnlyHint:    false,
			DestructiveHint: boolPtr(false),
			IdempotentHint:  false,
			OpenWorldHint:   boolPtr(false),
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, args CreateRedirectInput) (*mcp.CallToolResult, any, error) {
		createReq := apiclient.CreateRedirectRequest{
			FromPath:    args.FromPath,
			ToPath:      args.ToPath,
			StatusCode:  args.StatusCode,
			Description: args.Description,
		}

		redirect, err := s.client.CreateRedirect(ctx, createReq)
		if err != nil {
			return errorResult(err), nil, nil
		}

		return jsonResult(map[string]interface{}{
			"success": true,
			"id":      redirect.ID,
			"message": fmt.Sprintf("Redirect from %s to %s created", args.FromPath, args.ToPath),
		}), nil, nil
	})

	// Update redirect
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "update_redirect",
		Title:       "Update Redirect",
		Description: "Update an existing redirect.",
		Annotations: &mcp.ToolAnnotations{
			Title:           "Update Redirect",
			ReadOnlyHint:    false,
			DestructiveHint: boolPtr(true),
			IdempotentHint:  true,
			OpenWorldHint:   boolPtr(false),
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, args UpdateRedirectInput) (*mcp.CallToolResult, any, error) {
		updates := make(map[string]interface{})

		if args.FromPath != "" {
			updates["from_path"] = args.FromPath
		}
		if args.ToPath != "" {
			updates["to_path"] = args.ToPath
		}
		if args.StatusCode != 0 {
			updates["status_code"] = args.StatusCode
		}
		if args.Description != "" {
			updates["description"] = args.Description
		}

		if _, err := s.client.UpdateRedirect(ctx, args.ID, updates); err != nil {
			return errorResult(err), nil, nil
		}

		return textResult("Redirect updated successfully"), nil, nil
	})

	// Delete redirect
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "delete_redirect",
		Title:       "Delete Redirect",
		Description: "Delete a redirect.",
		Annotations: &mcp.ToolAnnotations{
			Title:           "Delete Redirect",
			ReadOnlyHint:    false,
			DestructiveHint: boolPtr(true),
			IdempotentHint:  true,
			OpenWorldHint:   boolPtr(false),
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, args RedirectIDInput) (*mcp.CallToolResult, any, error) {
		if err := s.client.DeleteRedirect(ctx, args.ID); err != nil {
			return errorResult(err), nil, nil
		}
		return textResult("Redirect deleted successfully"), nil, nil
	})

	// === Folders ===

	// List folders
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "list_folders",
		Title:       "List Folders",
		Description: "List all content folders. Folders organize content into URL path segments.",
		Annotations: &mcp.ToolAnnotations{
			Title:         "List Folders",
			ReadOnlyHint:  true,
			OpenWorldHint: boolPtr(false),
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct{}) (*mcp.CallToolResult, any, error) {
		folders, err := s.client.ListFolders(ctx)
		if err != nil {
			return errorResult(err), nil, nil
		}
		return jsonResult(folders), nil, nil
	})

	// Create folder
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "create_folder",
		Title:       "Create Folder",
		Description: "Create a new content folder. Folders create URL path segments for organizing content.",
		Annotations: &mcp.ToolAnnotations{
			Title:           "Create Folder",
			ReadOnlyHint:    false,
			DestructiveHint: boolPtr(false),
			IdempotentHint:  false,
			OpenWorldHint:   boolPtr(false),
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, args CreateFolderInput) (*mcp.CallToolResult, any, error) {
		createReq := apiclient.CreateFolderRequest{
			Name:     args.Name,
			Slug:     args.Slug,
			ParentID: args.ParentID,
		}

		folder, err := s.client.CreateFolder(ctx, createReq)
		if err != nil {
			return errorResult(err), nil, nil
		}

		return jsonResult(map[string]interface{}{
			"success": true,
			"id":      folder.ID,
			"path":    folder.Path,
			"message": fmt.Sprintf("Folder '%s' created at %s", folder.Name, folder.Path),
		}), nil, nil
	})

	// Get folder
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "get_folder",
		Title:       "Get Folder",
		Description: "Get a folder by ID.",
		Annotations: &mcp.ToolAnnotations{
			Title:         "Get Folder",
			ReadOnlyHint:  true,
			OpenWorldHint: boolPtr(false),
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, args FolderIDInput) (*mcp.CallToolResult, any, error) {
		folder, err := s.client.GetFolder(ctx, args.ID)
		if err != nil {
			return errorResult(err), nil, nil
		}
		return jsonResult(folder), nil, nil
	})

	// Delete folder
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "delete_folder",
		Title:       "Delete Folder",
		Description: "Delete an empty folder. Cannot delete folders that contain content or subfolders.",
		Annotations: &mcp.ToolAnnotations{
			Title:           "Delete Folder",
			ReadOnlyHint:    false,
			DestructiveHint: boolPtr(true),
			IdempotentHint:  true,
			OpenWorldHint:   boolPtr(false),
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, args FolderIDInput) (*mcp.CallToolResult, any, error) {
		if err := s.client.DeleteFolder(ctx, args.ID); err != nil {
			return errorResult(err), nil, nil
		}
		return textResult("Folder deleted successfully"), nil, nil
	})

	// === Collections ===

	// List collections
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "list_collections",
		Title:       "List Collections",
		Description: "List all content collections. Collections group and display content by category.",
		Annotations: &mcp.ToolAnnotations{
			Title:         "List Collections",
			ReadOnlyHint:  true,
			OpenWorldHint: boolPtr(false),
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct{}) (*mcp.CallToolResult, any, error) {
		collections, err := s.client.ListCollections(ctx)
		if err != nil {
			return errorResult(err), nil, nil
		}
		return jsonResult(collections), nil, nil
	})

	// Create collection
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "create_collection",
		Title:       "Create Collection",
		Description: "Create a new content collection. Collections display grouped content with custom templates.",
		Annotations: &mcp.ToolAnnotations{
			Title:           "Create Collection",
			ReadOnlyHint:    false,
			DestructiveHint: boolPtr(false),
			IdempotentHint:  false,
			OpenWorldHint:   boolPtr(false),
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, args CreateCollectionInput) (*mcp.CallToolResult, any, error) {
		createReq := map[string]interface{}{
			"name": args.Name,
			"slug": args.Slug,
		}
		if args.Description != "" {
			createReq["description"] = args.Description
		}
		if args.Category != "" {
			createReq["category"] = args.Category
		}
		if args.SortField != "" {
			createReq["sort_field"] = args.SortField
		}
		if args.SortOrder != "" {
			createReq["sort_order"] = args.SortOrder
		}
		if args.ItemTemplate != "" {
			createReq["item_template"] = args.ItemTemplate
		}
		if args.PageTemplate != "" {
			createReq["page_template"] = args.PageTemplate
		}
		if args.ItemsPerPage != 0 {
			createReq["items_per_page"] = args.ItemsPerPage
		}

		collection, err := s.client.CreateCollection(ctx, createReq)
		if err != nil {
			return errorResult(err), nil, nil
		}

		return jsonResult(map[string]interface{}{
			"success": true,
			"id":      collection.ID,
			"message": fmt.Sprintf("Collection '%s' created", collection.Name),
		}), nil, nil
	})

	// Get collection
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "get_collection",
		Title:       "Get Collection",
		Description: "Get a collection by ID.",
		Annotations: &mcp.ToolAnnotations{
			Title:         "Get Collection",
			ReadOnlyHint:  true,
			OpenWorldHint: boolPtr(false),
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, args CollectionIDInput) (*mcp.CallToolResult, any, error) {
		collection, err := s.client.GetCollection(ctx, args.ID)
		if err != nil {
			return errorResult(err), nil, nil
		}
		return jsonResult(collection), nil, nil
	})

	// Update collection
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "update_collection",
		Title:       "Update Collection",
		Description: "Update a collection's settings.",
		Annotations: &mcp.ToolAnnotations{
			Title:           "Update Collection",
			ReadOnlyHint:    false,
			DestructiveHint: boolPtr(true),
			IdempotentHint:  true,
			OpenWorldHint:   boolPtr(false),
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, args UpdateCollectionInput) (*mcp.CallToolResult, any, error) {
		updates := make(map[string]interface{})

		if args.Name != "" {
			updates["name"] = args.Name
		}
		if args.Slug != "" {
			updates["slug"] = args.Slug
		}
		if args.Description != "" {
			updates["description"] = args.Description
		}
		if args.Category != "" {
			updates["category"] = args.Category
		}
		if args.SortField != "" {
			updates["sort_field"] = args.SortField
		}
		if args.SortOrder != "" {
			updates["sort_order"] = args.SortOrder
		}
		if args.ItemTemplate != "" {
			updates["item_template"] = args.ItemTemplate
		}
		if args.PageTemplate != "" {
			updates["page_template"] = args.PageTemplate
		}
		if args.ItemsPerPage != 0 {
			updates["items_per_page"] = args.ItemsPerPage
		}

		if _, err := s.client.UpdateCollection(ctx, args.ID, updates); err != nil {
			return errorResult(err), nil, nil
		}

		return textResult("Collection updated successfully"), nil, nil
	})

	// Delete collection
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "delete_collection",
		Title:       "Delete Collection",
		Description: "Delete a collection. This does not delete the content in the collection.",
		Annotations: &mcp.ToolAnnotations{
			Title:           "Delete Collection",
			ReadOnlyHint:    false,
			DestructiveHint: boolPtr(true),
			IdempotentHint:  true,
			OpenWorldHint:   boolPtr(false),
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, args CollectionIDInput) (*mcp.CallToolResult, any, error) {
		if err := s.client.DeleteCollection(ctx, args.ID); err != nil {
			return errorResult(err), nil, nil
		}
		return textResult("Collection deleted successfully"), nil, nil
	})

	// === Utility ===

	// Regenerate all content
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "regenerate_all_content",
		Title:       "Regenerate All Content",
		Description: "Regenerate all published static HTML pages. Use after major theme or template changes.",
		Annotations: &mcp.ToolAnnotations{
			Title:           "Regenerate All Content",
			ReadOnlyHint:    false,
			DestructiveHint: boolPtr(false),
			IdempotentHint:  true,
			OpenWorldHint:   boolPtr(false),
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct{}) (*mcp.CallToolResult, any, error) {
		if err := s.client.RegenerateAllContent(ctx); err != nil {
			return errorResult(err), nil, nil
		}
		return textResult("All published content regenerated successfully"), nil, nil
	})
}
