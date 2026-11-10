package mcp

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"

	"github.com/metavert-io/lightcms/v7/internal/apiclient"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Asset tool input types
type ListAssetsInput struct {
	Folder string `json:"folder,omitempty" jsonschema:"Filter by folder path (e.g., /images)"`
}

type GetAssetInput struct {
	ID   string `json:"id,omitempty" jsonschema:"Asset ID (MongoDB ObjectID)"`
	Path string `json:"path,omitempty" jsonschema:"Asset serve path (e.g., /images/logo.png)"`
}

type UploadAssetInput struct {
	Filename    string `json:"filename" jsonschema:"Original filename with extension,required"`
	ServePath   string `json:"serve_path" jsonschema:"URL path where file will be served (e.g., /images/logo.png),required"`
	Description string `json:"description,omitempty" jsonschema:"Optional description of the asset"`
	FilePath    string `json:"file_path,omitempty" jsonschema:"Absolute local filesystem path to read the file from (only when this MCP server runs on your own machine). Preferred over data_base64 for large files — avoids MCP transport size limits."`
	DataBase64  string `json:"data_base64,omitempty" jsonschema:"Base64-encoded file content. Use for small files (<100KB). For larger files, prefer file_path."`
}

type AssetIDInput struct {
	ID string `json:"id" jsonschema:"Asset ID (MongoDB ObjectID),required"`
}

type UploadAssetFromURLInput struct {
	URL         string `json:"url" jsonschema:"Public URL of the file to fetch (must be http or https),required"`
	ServePath   string `json:"serve_path,omitempty" jsonschema:"URL path where asset will be served (e.g. /assets/logo.png). Auto-derived from URL filename if omitted."`
	Description string `json:"description,omitempty" jsonschema:"Optional description"`
}

func (s *Server) registerAssetTools() {
	// List assets
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "list_assets",
		Title:       "List Assets",
		Description: "List all assets in the asset library. Assets are files like images, documents, CSS, JS, etc.",
		Annotations: &mcp.ToolAnnotations{
			Title:         "List Assets",
			ReadOnlyHint:  true,
			OpenWorldHint: boolPtr(false),
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, args ListAssetsInput) (*mcp.CallToolResult, any, error) {
		assets, err := s.client.ListAssets(ctx, args.Folder)
		if err != nil {
			return errorResult(err), nil, nil
		}
		return jsonResult(assets), nil, nil
	})

	// List asset folders
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "list_asset_folders",
		Title:       "List Asset Folders",
		Description: "List all unique folder paths in the asset library.",
		Annotations: &mcp.ToolAnnotations{
			Title:         "List Asset Folders",
			ReadOnlyHint:  true,
			OpenWorldHint: boolPtr(false),
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct{}) (*mcp.CallToolResult, any, error) {
		folders, err := s.client.ListAssetFolders(ctx)
		if err != nil {
			return errorResult(err), nil, nil
		}
		return jsonResult(folders), nil, nil
	})

	// Get asset
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "get_asset",
		Title:       "Get Asset",
		Description: "Get asset metadata by ID or path. Does not return file content (use the serve path to access the file).",
		Annotations: &mcp.ToolAnnotations{
			Title:         "Get Asset",
			ReadOnlyHint:  true,
			OpenWorldHint: boolPtr(false),
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, args GetAssetInput) (*mcp.CallToolResult, any, error) {
		if args.ID != "" {
			asset, err := s.client.GetAsset(ctx, args.ID)
			if err != nil {
				return errorResult(err), nil, nil
			}
			return jsonResult(asset), nil, nil
		} else if args.Path != "" {
			asset, err := s.client.GetAssetByPath(ctx, args.Path)
			if err != nil {
				return errorResult(err), nil, nil
			}
			return jsonResult(asset), nil, nil
		}
		return errorResult(fmt.Errorf("either id or path is required")), nil, nil
	})

	// Upload asset
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:  "upload_asset",
		Title: "Upload Asset",
		Description: `Upload or replace an asset in the asset library. Re-uploading to the same serve_path replaces the existing file in place — no need to delete first.

Provide file content via one of:
- file_path: Absolute local path to the file (preferred for files >100KB — avoids MCP transport size limits). Only available when this MCP server runs on your own machine (the stdio binary), not over a hosted /mcp connection.
- data_base64: Base64-encoded file content (fine for small files; required over a hosted /mcp connection)

Validates file type and MIME type for security.`,
		Annotations: &mcp.ToolAnnotations{
			Title:           "Upload Asset",
			ReadOnlyHint:    false,
			DestructiveHint: boolPtr(false),
			IdempotentHint:  false,
			OpenWorldHint:   boolPtr(false),
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, args UploadAssetInput) (*mcp.CallToolResult, any, error) {
		if args.FilePath == "" && args.DataBase64 == "" {
			return errorResult(fmt.Errorf("either file_path or data_base64 is required")), nil, nil
		}

		uploadReq := apiclient.UploadAssetRequest{
			Filename:    args.Filename,
			ServePath:   args.ServePath,
			Description: args.Description,
		}

		if args.FilePath != "" {
			if !s.localFiles {
				return errorResult(fmt.Errorf("file_path is only available when the MCP server runs on your own machine; send the file as data_base64")), nil, nil
			}
			// Read file locally and base64-encode it — avoids MCP transport size limits
			fileData, err := os.ReadFile(args.FilePath)
			if err != nil {
				return errorResult(fmt.Errorf("failed to read file_path %q: %w", args.FilePath, err)), nil, nil
			}
			uploadReq.DataBase64 = base64.StdEncoding.EncodeToString(fileData)
			// Auto-derive filename from path if not provided
			if uploadReq.Filename == "" {
				uploadReq.Filename = filepath.Base(args.FilePath)
			}
		} else {
			uploadReq.DataBase64 = args.DataBase64
		}

		asset, err := s.client.UploadAsset(ctx, uploadReq)
		if err != nil {
			return errorResult(err), nil, nil
		}

		return jsonResult(map[string]interface{}{
			"success":    true,
			"id":         asset.ID,
			"serve_path": asset.ServePath,
			"mime_type":  asset.MimeType,
			"size":       asset.Size,
			"message":    fmt.Sprintf("Asset '%s' uploaded successfully", uploadReq.Filename),
		}), nil, nil
	})

	// Upload asset from URL
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:  "upload_asset_from_url",
		Title: "Upload Asset from URL",
		Description: `Fetch a public URL and store it as a LightCMS asset. Useful for importing images or files from the web without downloading them locally first.

Example: {"url": "https://example.com/logo.png", "serve_path": "/assets/logo.png", "description": "Site logo"}

If serve_path is omitted, the filename is derived from the URL. Returns id, serve_path, mime_type, and size.`,
		Annotations: &mcp.ToolAnnotations{
			Title:           "Upload Asset from URL",
			ReadOnlyHint:    false,
			DestructiveHint: boolPtr(false),
			IdempotentHint:  false,
			OpenWorldHint:   boolPtr(true),
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, args UploadAssetFromURLInput) (*mcp.CallToolResult, any, error) {
		asset, err := s.client.UploadAssetFromURL(ctx, args.URL, args.ServePath, args.Description)
		if err != nil {
			return errorResult(err), nil, nil
		}
		return jsonResult(map[string]interface{}{
			"success":    true,
			"id":         asset.ID,
			"serve_path": asset.ServePath,
			"mime_type":  asset.MimeType,
			"size":       asset.Size,
			"message":    fmt.Sprintf("Asset fetched from URL and stored at '%s'", asset.ServePath),
		}), nil, nil
	})

	// Delete asset
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "delete_asset",
		Title:       "Delete Asset",
		Description: "Delete an asset from the library. Removes both the file and database record.",
		Annotations: &mcp.ToolAnnotations{
			Title:           "Delete Asset",
			ReadOnlyHint:    false,
			DestructiveHint: boolPtr(true),
			IdempotentHint:  true,
			OpenWorldHint:   boolPtr(false),
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, args AssetIDInput) (*mcp.CallToolResult, any, error) {
		if err := s.client.DeleteAsset(ctx, args.ID); err != nil {
			return errorResult(err), nil, nil
		}
		return textResult(fmt.Sprintf("Asset %s deleted successfully", args.ID)), nil, nil
	})
}
