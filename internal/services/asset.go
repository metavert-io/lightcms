package services

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/metavert-io/lightcms/v7/internal/database"
	"github.com/metavert-io/lightcms/v7/internal/middleware"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// AssetService handles asset operations
type AssetService struct {
	db *database.DB
}

// NewAssetService creates a new asset service
func NewAssetService(db *database.DB) *AssetService {
	return &AssetService{db: db}
}

// UploadAsset uploads a new asset
func (s *AssetService) UploadAsset(ctx context.Context, data []byte, filename, servePath, description string) (*database.Asset, error) {
	// Validate file extension
	ext := strings.ToLower(filepath.Ext(filename))
	if !middleware.IsAllowedFileType(ext) {
		return nil, fmt.Errorf("file type not allowed: %s", ext)
	}

	// Validate MIME type
	detectedMIME := http.DetectContentType(data)
	if !middleware.ValidateMIMEType(ext, detectedMIME) {
		return nil, fmt.Errorf("file content does not match extension")
	}

	// Clean and validate serve path
	if servePath == "" {
		servePath = "/" + filename
	}
	if !strings.HasPrefix(servePath, "/") {
		servePath = "/" + servePath
	}

	// Validate path doesn't contain traversal
	if !middleware.ValidateFilePath(servePath) {
		return nil, fmt.Errorf("invalid file path")
	}

	servePath = filepath.Clean(servePath)
	if !strings.HasPrefix(servePath, "/") {
		servePath = "/" + servePath
	}

	// Double-check for path traversal
	if strings.Contains(servePath, "..") {
		return nil, fmt.Errorf("invalid file path")
	}

	// Strip /assets prefix if present — the /assets URL prefix is a routing
	// concern; internally we store paths without it so ServeAsset can look
	// them up after stripping the prefix from the request URL.
	if strings.HasPrefix(servePath, "/assets/") {
		servePath = strings.TrimPrefix(servePath, "/assets")
	}

	// Get filename and folder from serve path
	fname := filepath.Base(servePath)
	folder := filepath.Dir(servePath)
	if folder == "." {
		folder = "/"
	}

	// Determine MIME type
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

	// Verify path is within allowed directory
	baseDir, _ := filepath.Abs("content/generated")
	staticPath := filepath.Join("content/generated", servePath)
	absStaticPath, err := filepath.Abs(staticPath)
	if err != nil {
		return nil, fmt.Errorf("invalid path: %w", err)
	}
	if !strings.HasPrefix(absStaticPath, baseDir) {
		return nil, fmt.Errorf("path traversal not allowed")
	}

	// Create directory and write file
	staticDir := filepath.Dir(staticPath)
	if err := os.MkdirAll(staticDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create directory: %w", err)
	}
	if err := os.WriteFile(staticPath, data, 0644); err != nil {
		return nil, fmt.Errorf("failed to write file: %w", err)
	}

	// Create asset record
	now := time.Now()
	asset := &database.Asset{
		Filename:    fname,
		Folder:      folder,
		FullPath:    servePath,
		ServePath:   servePath,
		MimeType:    mimeType,
		Size:        int64(len(data)),
		Data:        nil, // Don't store in DB anymore
		Description: description,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	if err := s.db.SaveAsset(ctx, asset); err != nil {
		// Clean up file if DB save fails
		os.Remove(staticPath)
		return nil, fmt.Errorf("failed to save asset: %w", err)
	}

	return asset, nil
}

// DeleteAsset deletes an asset
func (s *AssetService) DeleteAsset(ctx context.Context, id primitive.ObjectID) error {
	// Get asset to find file path
	asset, err := s.db.GetAsset(ctx, id)
	if err != nil {
		return fmt.Errorf("asset not found: %w", err)
	}
	if asset == nil {
		return fmt.Errorf("asset not found")
	}

	// Delete file from filesystem
	staticPath := filepath.Join("content/generated", asset.ServePath)
	os.Remove(staticPath)

	// Delete from database
	if err := s.db.DeleteAsset(ctx, id); err != nil {
		return fmt.Errorf("failed to delete asset: %w", err)
	}

	return nil
}

// GetAsset retrieves an asset by ID
func (s *AssetService) GetAsset(ctx context.Context, id primitive.ObjectID) (*database.Asset, error) {
	asset, err := s.db.GetAsset(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("asset not found: %w", err)
	}
	return asset, nil
}

// AssetExistsAt reports whether an asset is already stored at the serve path an
// upload names, after the same normalisation UploadAsset applies.
func (s *AssetService) AssetExistsAt(ctx context.Context, servePath string) (bool, error) {
	p := filepath.Clean("/" + strings.TrimPrefix(servePath, "/"))
	if strings.HasPrefix(p, "/assets/") {
		p = strings.TrimPrefix(p, "/assets")
	}
	asset, err := s.db.GetAssetByPath(ctx, p)
	if err != nil {
		return false, err
	}
	return asset != nil, nil
}

// GetAssetByPath retrieves an asset by serve path
func (s *AssetService) GetAssetByPath(ctx context.Context, path string) (*database.Asset, error) {
	return s.db.GetAssetByPath(ctx, path)
}

// ListAssets lists all assets
func (s *AssetService) ListAssets(ctx context.Context, folder string) ([]database.Asset, error) {
	filter := bson.M{}
	if folder != "" {
		filter["folder"] = folder
	}

	cursor, err := s.db.FindMany(ctx, "assets", filter,
		options.Find().SetSort(bson.D{{Key: "created_at", Value: -1}}))
	if err != nil {
		return nil, fmt.Errorf("failed to list assets: %w", err)
	}

	var assets []database.Asset
	if err := cursor.All(ctx, &assets); err != nil {
		return nil, fmt.Errorf("failed to decode assets: %w", err)
	}

	return assets, nil
}

// ListFolders lists all unique asset folders
func (s *AssetService) ListFolders(ctx context.Context) ([]string, error) {
	return s.db.GetAssetFolders(ctx)
}
