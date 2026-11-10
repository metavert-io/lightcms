package services

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"html/template"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/metavert-io/lightcms/v7/internal/database"
	"github.com/metavert-io/lightcms/v7/internal/models"

	"github.com/microcosm-cc/bluemonday"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	goldmarkhtml "github.com/yuin/goldmark/renderer/html"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// editorContextKey is the unexported context key for the current editor's email.
type editorContextKey struct{}

// WithEditorEmail returns a copy of ctx with the editor's email stored for version history.
func WithEditorEmail(ctx context.Context, email string) context.Context {
	return context.WithValue(ctx, editorContextKey{}, email)
}

// EditorEmailFromContext extracts the editor email stored by WithEditorEmail.
func EditorEmailFromContext(ctx context.Context) string {
	v, _ := ctx.Value(editorContextKey{}).(string)
	return v
}

// Provenance records who or what authored a content change: a human in the
// admin UI, an API caller, an AI agent session, or the admin copilot.
type Provenance struct {
	Actor        string // "human" or "agent"
	Via          string // "ui", "api", "copilot"
	AgentSession string // agent session ID when Actor == "agent"
}

type provenanceContextKey struct{}

// WithProvenance returns a copy of ctx carrying change provenance, recorded
// on every content version saved during the request.
func WithProvenance(ctx context.Context, p Provenance) context.Context {
	return context.WithValue(ctx, provenanceContextKey{}, p)
}

// ProvenanceFromContext extracts provenance stored by WithProvenance.
func ProvenanceFromContext(ctx context.Context) (Provenance, bool) {
	p, ok := ctx.Value(provenanceContextKey{}).(Provenance)
	return p, ok
}

// ContentService centralizes all content operations with automatic versioning
type ContentService struct {
	db               *database.DB
	searchService    *SearchService
	webhookService   *WebhookService
	cfService        *CloudflareService
	indexNow         *IndexNowService
	indexRegenCh     chan struct{} // coalescing trigger for RegenerateIndexPages
	keywordRebuildCh chan struct{} // coalescing trigger for RebuildKeywords

	// Cached wikilink index (title→path / path→title).
	// Rebuilt at most once per wikilinkCacheTTL to avoid a full collection scan
	// on every page publish/regenerate.
	wikilinkCacheMu  sync.RWMutex
	wikilinkCache    *wikilinkIndex
	wikilinkCacheExp time.Time
}

const wikilinkCacheTTL = 60 * time.Second

// DB returns the underlying database handle (for internal use by API handlers).
func (s *ContentService) DB() *database.DB {
	return s.db
}

// NewContentService creates a new content service
func NewContentService(db *database.DB) *ContentService {
	s := &ContentService{
		db:               db,
		indexRegenCh:     make(chan struct{}, 1),
		keywordRebuildCh: make(chan struct{}, 1),
	}
	go s.indexRegenWorker()
	go s.keywordRebuildWorker()
	return s
}

// SetSearchService sets the search service for automatic embedding generation
func (s *ContentService) SetSearchService(ss *SearchService) {
	s.searchService = ss
}

// SetWebhookService sets the webhook service for firing content lifecycle events.
func (s *ContentService) SetWebhookService(ws *WebhookService) {
	s.webhookService = ws
}

// SetCloudflareService sets the Cloudflare service for cache purging.
func (s *ContentService) SetCloudflareService(cf *CloudflareService) {
	s.cfService = cf
}

// SetIndexNowService sets the IndexNow service for search-engine change notification.
func (s *ContentService) SetIndexNowService(in *IndexNowService) {
	s.indexNow = in
}

type indexNowSuppressKey struct{}

// WithoutIndexNow marks ctx so page regenerations under it are not submitted
// to IndexNow. Used for site-wide re-renders (template, theme, snippet
// changes) where the pages' content did not change.
func WithoutIndexNow(ctx context.Context) context.Context {
	return context.WithValue(ctx, indexNowSuppressKey{}, true)
}

// notifyIndexNow queues live paths for IndexNow submission (no-op if
// IndexNow isn't wired or ctx is a site-wide regeneration).
func (s *ContentService) notifyIndexNow(ctx context.Context, paths ...string) {
	if s.indexNow == nil {
		return
	}
	if suppressed, _ := ctx.Value(indexNowSuppressKey{}).(bool); suppressed {
		return
	}
	s.indexNow.Notify(paths...)
}

// NotifyLiveChange tells IndexNow which public URLs a change affected:
// a page going live, going away (after == nil means deleted), moving, or
// being hidden from / returned to search. Content edits themselves are
// notified from static generation when the rendered HTML changes. Exported
// for handlers that write to the database directly (the admin editor).
func (s *ContentService) NotifyLiveChange(ctx context.Context, before, after *models.Content) {
	if before == nil || before.ForkID != nil {
		return
	}
	wasLive := before.Published && !before.Deleted
	isLive := after != nil && after.Published && !after.Deleted
	switch {
	case isLive && !wasLive:
		s.notifyIndexNow(ctx, after.FullPath)
	case wasLive && !isLive:
		s.notifyIndexNow(ctx, before.FullPath)
	case wasLive && isLive:
		if before.FullPath != after.FullPath {
			s.notifyIndexNow(ctx, before.FullPath, after.FullPath)
		}
		if before.NoIndex != after.NoIndex {
			s.notifyIndexNow(ctx, after.FullPath)
		}
	}
}

// ErrPublishForkCopy is returned when a fork copy is published directly.
// Fork copies share their full_path with the live page and reach the live
// site only through ForkService.Merge.
var ErrPublishForkCopy = errors.New("cannot publish a fork copy; merge the fork instead")

// ErrContentHeld is returned when a held page would go from draft to
// published. Clearing the hold flag is the only way to publish it.
var ErrContentHeld = errors.New("content is on hold; clear the hold flag before publishing")

// liveOnly restricts a content filter to live pages. Fork copies carry a
// fork_id; live pages have none (missing or null).
func liveOnly(filter bson.M) {
	filter["fork_id"] = nil
}

// stampPublishedAt records the publish time on content that is published
// without one. Before 7.3.2 only PublishContent set it, so pages created
// already-published (API, MCP, bulk, import) had no published date.
func stampPublishedAt(c *models.Content) {
	if c.Published && (c.PublishedAt == nil || c.PublishedAt.IsZero()) {
		now := time.Now()
		c.PublishedAt = &now
	}
}

// BackfillPublishedDates sets published_at = created_at on live published
// pages that have no published date (created before 7.3.2). Metadata only:
// rendered pages don't change, so no versions, regeneration, or IndexNow.
// With dryRun it only counts.
func (s *ContentService) BackfillPublishedDates(ctx context.Context, dryRun bool) (int64, error) {
	filter := bson.M{
		"published": true,
		"deleted":   bson.M{"$ne": true},
		"fork_id":   nil,
		"$or":       bson.A{bson.M{"published_at": bson.M{"$exists": false}}, bson.M{"published_at": nil}},
	}
	col := s.db.Collection("content")
	if dryRun {
		return col.CountDocuments(ctx, filter)
	}
	res, err := col.UpdateMany(ctx, filter, mongo.Pipeline{
		{{Key: "$set", Value: bson.D{{Key: "published_at", Value: "$created_at"}}}},
	})
	if err != nil {
		return 0, fmt.Errorf("backfill published dates: %w", err)
	}
	return res.ModifiedCount, nil
}

// PurgeCloudflareURLs purges specific paths from the Cloudflare cache.
// No-op if Cloudflare is not configured.
func (s *ContentService) PurgeCloudflareURLs(paths []string) {
	if s.cfService != nil && len(paths) > 0 {
		go s.cfService.PurgeByURLs(context.Background(), paths)
	}
}

// PurgeCloudflareAll purges the entire Cloudflare cache for the zone.
// No-op if Cloudflare is not configured.
func (s *ContentService) PurgeCloudflareAll() {
	if s.cfService != nil {
		go s.cfService.PurgeEverything(context.Background())
	}
}

// indexRegenWorker drains indexRegenCh and runs RegenerateIndexPages, coalescing
// bursts so a bulk update of N items triggers only one regeneration pass.
func (s *ContentService) indexRegenWorker() {
	for range s.indexRegenCh {
		// Small debounce: let any remaining burst signals arrive
		time.Sleep(200 * time.Millisecond)
		// Drain any accumulated signals
		for len(s.indexRegenCh) > 0 {
			<-s.indexRegenCh
		}
		ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
		s.RegenerateIndexPages(ctx)
		cancel()
	}
}

// keywordRebuildWorker drains keywordRebuildCh and runs RebuildKeywords, coalescing
// bursts so a bulk update triggers only one rebuild.
func (s *ContentService) keywordRebuildWorker() {
	for range s.keywordRebuildCh {
		time.Sleep(200 * time.Millisecond)
		for len(s.keywordRebuildCh) > 0 {
			<-s.keywordRebuildCh
		}
		if s.searchService == nil {
			continue
		}
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		if err := s.searchService.RebuildKeywords(ctx); err != nil {
			fmt.Printf("Warning: failed to rebuild search keywords: %v\n", err)
		}
		cancel()
	}
}

// triggerEmbedding asynchronously generates an embedding for the given content
func (s *ContentService) triggerEmbedding(contentID primitive.ObjectID) {
	if s.searchService == nil || !s.searchService.EmbeddingsEnabled() {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := s.searchService.UpdateContentEmbedding(ctx, contentID); err != nil {
			fmt.Printf("Warning: failed to update embedding for %s: %v\n", contentID.Hex(), err)
		}
	}()
}

// triggerKeywordRebuild signals the coalescing worker to rebuild search keywords.
// Multiple rapid calls collapse into a single rebuild.
func (s *ContentService) triggerKeywordRebuild() {
	select {
	case s.keywordRebuildCh <- struct{}{}:
	default: // already pending
	}
}

// triggerIndexRegen signals the coalescing worker to regenerate lc:query index pages.
// Multiple rapid calls collapse into a single regeneration pass.
func (s *ContentService) triggerIndexRegen() {
	select {
	case s.indexRegenCh <- struct{}{}:
	default: // already pending
	}
}

// TriggerIndexRegen is the exported variant for callers outside this package.
func (s *ContentService) TriggerIndexRegen() {
	s.triggerIndexRegen()
}

// CreateContent creates new content and saves the initial version
func (s *ContentService) CreateContent(ctx context.Context, content *models.Content, versionComment ...string) error {
	// A held page cannot be created already-published.
	if content.Hold && content.Published {
		return ErrContentHeld
	}

	now := time.Now()
	content.CreatedAt = now
	content.UpdatedAt = now
	stampPublishedAt(content)

	// Build full path
	if content.FolderPath != "" && content.FolderPath != "/" {
		content.FullPath = content.FolderPath + "/" + content.Slug
	} else if content.Slug != "" {
		content.FullPath = "/" + content.Slug
	} else {
		content.FullPath = "/"
	}

	// Reject case-variant duplicates (paths are case-insensitive).
	if conflict := s.findPathCaseConflict(ctx, content.FullPath, primitive.NilObjectID); conflict != nil {
		return fmt.Errorf("path %s conflicts with existing page %s — paths are case-insensitive", content.FullPath, conflict.FullPath)
	}

	// Merge inline #tags from data fields into content.Tags
	mergeInlineTags(content)

	// Extract internal links
	content.InternalLinks = s.extractInternalLinks(content)

	// Insert content
	id, err := s.db.InsertOne(ctx, "content", content)
	if err != nil {
		return fmt.Errorf("failed to insert content: %w", err)
	}
	content.ID = id

	// Save initial version with optional comment
	comment := ""
	if len(versionComment) > 0 {
		comment = versionComment[0]
	}
	if err := s.saveVersion(ctx, content, nil, comment); err != nil {
		return fmt.Errorf("failed to save initial version: %w", err)
	}

	// Generate static page if published (fork copies never touch static
	// files or the embedding index — they go live only on merge)
	if content.Published && content.ForkID == nil {
		if err := s.GenerateStaticPage(ctx, content); err != nil {
			// Log but don't fail
			fmt.Printf("Warning: failed to generate static page: %v\n", err)
		}
		s.triggerEmbedding(content.ID)
	}

	// Fire webhook event. A fork copy is not a page on the site: creating
	// one announces nothing (the merge that brings it live does).
	if s.webhookService != nil && content.ForkID == nil {
		s.webhookService.FireEvent(ctx, "content.create", map[string]interface{}{
			"id":    content.ID.Hex(),
			"title": content.Title,
			"path":  content.FullPath,
		})
	}

	return nil
}

// UpsertContent creates content if it doesn't exist at the given path, or updates it if it does.
// Returns true if the content was created (false = updated).
func (s *ContentService) UpsertContent(ctx context.Context, content *models.Content, versionComment string) (created bool, err error) {
	// Build the full path first so we can look up existing content
	if content.FolderPath != "" && content.FolderPath != "/" {
		content.FullPath = content.FolderPath + "/" + content.Slug
	} else if content.Slug != "" {
		content.FullPath = "/" + content.Slug
	} else {
		content.FullPath = "/"
	}

	existing, err := s.GetContentByPath(ctx, content.FullPath)
	if err == nil && existing != nil {
		// Update existing: merge fields from the incoming content onto the existing one
		existing.Title = content.Title
		existing.Slug = content.Slug
		existing.FolderPath = content.FolderPath
		existing.Category = content.Category
		existing.Tags = content.Tags
		existing.MetaDescription = content.MetaDescription
		existing.OGImage = content.OGImage
		existing.Data = content.Data
		existing.UseHeader = content.UseHeader
		existing.UseFooter = content.UseFooter
		existing.UseTheme = content.UseTheme
		existing.RawMode = content.RawMode
		existing.NoIndex = content.NoIndex
		existing.AuthorName = content.AuthorName
		existing.AuthorURL = content.AuthorURL
		if content.Published {
			existing.Published = true
		}
		// Like published, an upsert can set hold but never silently clears it.
		if content.Hold {
			existing.Hold = true
		}
		if err := s.UpdateContent(ctx, existing, versionComment); err != nil {
			return false, err
		}
		*content = *existing
		return false, nil
	}

	// Create new
	if err := s.CreateContent(ctx, content, versionComment); err != nil {
		return false, err
	}
	return true, nil
}

// BulkCreateResult holds the outcome for a single item in a bulk create batch.
type BulkCreateResult struct {
	Index    int    `json:"index"`
	ID       string `json:"id,omitempty"`
	FullPath string `json:"full_path,omitempty"`
	Success  bool   `json:"success"`
	Error    string `json:"error,omitempty"`
}

// BulkCreateContent creates multiple content items in a single batch.
// Uses unordered InsertMany so one failure doesn't abort the rest.
// HTML generation for published items runs in parallel (bounded to 10 goroutines).
func (s *ContentService) BulkCreateContent(ctx context.Context, items []*models.Content, versionComment string) []BulkCreateResult {
	results := make([]BulkCreateResult, len(items))
	now := time.Now()

	// Pre-process all items: build paths, merge tags, extract links, assign IDs
	docs := make([]interface{}, 0, len(items))
	validIndices := make([]int, 0, len(items))
	for i, c := range items {
		c.CreatedAt = now
		c.UpdatedAt = now
		stampPublishedAt(c)
		c.ID = primitive.NewObjectID()

		if c.FolderPath != "" && c.FolderPath != "/" {
			c.FullPath = c.FolderPath + "/" + c.Slug
		} else if c.Slug != "" {
			c.FullPath = "/" + c.Slug
		} else {
			c.FullPath = "/"
		}

		mergeInlineTags(c)
		c.InternalLinks = s.extractInternalLinks(c)

		docs = append(docs, c)
		validIndices = append(validIndices, i)
		results[i] = BulkCreateResult{Index: i, ID: c.ID.Hex(), FullPath: c.FullPath, Success: true}
	}

	// Insert all at once (unordered — partial success is OK)
	_, err := s.db.InsertManyUnordered(ctx, "content", docs)
	if err != nil {
		// Check for bulk write errors (partial failures)
		if bwe, ok := err.(mongo.BulkWriteException); ok {
			failedIndices := make(map[int]string)
			for _, we := range bwe.WriteErrors {
				failedIndices[we.Index] = we.Message
			}
			for docIdx, origIdx := range validIndices {
				if msg, failed := failedIndices[docIdx]; failed {
					results[origIdx].Success = false
					results[origIdx].Error = msg
					results[origIdx].ID = ""
				}
			}
		} else {
			// Total failure
			for i := range results {
				results[i].Success = false
				results[i].Error = err.Error()
				results[i].ID = ""
			}
			return results
		}
	}

	// Batch-insert initial versions for successful items
	var versionDocs []interface{}
	for i, c := range items {
		if !results[i].Success {
			continue
		}
		editorEmail := EditorEmailFromContext(ctx)
		v := models.ContentVersion{
			ContentID:       c.ID,
			Version:         1,
			Title:           c.Title,
			Slug:            c.Slug,
			Data:            c.Data,
			ModifiedByEmail: editorEmail,
			Comment:         versionComment,
			CreatedAt:       now,
		}
		versionDocs = append(versionDocs, v)
	}
	if len(versionDocs) > 0 {
		s.db.InsertMany(ctx, "content_versions", versionDocs)
	}

	// Parallel HTML generation for published items (bounded concurrency)
	sem := make(chan struct{}, 10)
	var wg sync.WaitGroup
	for i, c := range items {
		if !results[i].Success || !c.Published || c.ForkID != nil {
			continue
		}
		wg.Add(1)
		sem <- struct{}{}
		go func(content *models.Content) {
			defer wg.Done()
			defer func() { <-sem }()
			if err := s.GenerateStaticPage(ctx, content); err != nil {
				fmt.Printf("Warning: bulk create static page gen failed for %s: %v\n", content.FullPath, err)
			}
			s.triggerEmbedding(content.ID)
		}(c)
	}
	wg.Wait()

	// Fire webhook events and trigger index rebuild once
	if s.webhookService != nil {
		for i, c := range items {
			if results[i].Success && c.ForkID == nil {
				s.webhookService.FireEvent(ctx, "content.create", map[string]interface{}{
					"id": c.ID.Hex(), "title": c.Title, "path": c.FullPath,
				})
			}
		}
	}
	s.triggerIndexRegen()
	s.triggerKeywordRebuild()

	return results
}

// UpdateContent updates content and saves a new version with an optional comment
func (s *ContentService) UpdateContent(ctx context.Context, content *models.Content, versionComment ...string) error {
	// Get the original content for versioning
	var original models.Content
	if err := s.db.FindOne(ctx, "content", bson.M{"_id": content.ID}, &original); err != nil {
		return fmt.Errorf("failed to get original content: %w", err)
	}

	// A held draft cannot go live through an update either. Holding a page
	// that is already published is allowed and does not unpublish it.
	if content.Hold && content.Published && !original.Published {
		return ErrContentHeld
	}

	content.UpdatedAt = time.Now()
	stampPublishedAt(content)

	// Build full path
	if content.FolderPath != "" && content.FolderPath != "/" {
		content.FullPath = content.FolderPath + "/" + content.Slug
	} else if content.Slug != "" {
		content.FullPath = "/" + content.Slug
	} else {
		content.FullPath = "/"
	}

	// Reject case-variant duplicates (paths are case-insensitive).
	if content.FullPath != original.FullPath {
		if conflict := s.findPathCaseConflict(ctx, content.FullPath, content.ID); conflict != nil {
			return fmt.Errorf("path %s conflicts with existing page %s — paths are case-insensitive", content.FullPath, conflict.FullPath)
		}
	}

	// Merge inline #tags from data fields into content.Tags
	mergeInlineTags(content)

	// Extract internal links
	content.InternalLinks = s.extractInternalLinks(content)

	// Update content
	update := bson.M{
		"$set": bson.M{
			"template_id":      content.TemplateID,
			"template_name":    content.TemplateName,
			"title":            content.Title,
			"slug":             content.Slug,
			"folder_id":        content.FolderID,
			"folder_path":      content.FolderPath,
			"full_path":        content.FullPath,
			"category":         content.Category,
			"tags":             content.Tags,
			"meta_description": content.MetaDescription,
			"og_image":         content.OGImage,
			"data":             content.Data,
			"published":        content.Published,
			"published_at":     content.PublishedAt,
			"publish_at":       content.PublishAt,
			"use_header":       content.UseHeader,
			"use_footer":       content.UseFooter,
			"use_theme":        content.UseTheme,
			"raw_mode":         content.RawMode,
			"noindex":          content.NoIndex,
			"hold":             content.Hold,
			"author_name":      content.AuthorName,
			"author_url":       content.AuthorURL,
			"internal_links":   content.InternalLinks,
			"updated_at":       content.UpdatedAt,
		},
	}

	if err := s.db.UpdateOne(ctx, "content", bson.M{"_id": content.ID}, update); err != nil {
		return fmt.Errorf("failed to update content: %w", err)
	}

	// Save version with original for first-time versioning
	comment := ""
	if len(versionComment) > 0 {
		comment = versionComment[0]
	}
	if err := s.saveVersion(ctx, content, &original, comment); err != nil {
		return fmt.Errorf("failed to save version: %w", err)
	}

	// Invalidate wikilink cache: title/path/publish-status changes all affect the index.
	if original.Title != content.Title || original.FullPath != content.FullPath || original.Published != content.Published {
		s.invalidateWikilinkCache()
	}

	// A fork copy shares its full_path with the live page and exists only
	// inside its fork. Saving one has no effect outside the fork: no static
	// file, embedding, IndexNow ping, keyword rebuild, wikilink rewrite of
	// other pages, index-page regeneration or content.update webhook. The
	// decision is made on the stored row as well as on what the caller
	// passed, so a caller that dropped ForkID cannot turn a copy live.
	if original.ForkID != nil || content.ForkID != nil {
		return nil
	}

	// Generate or remove static page based on publish status.
	if content.Published {
		if err := s.GenerateStaticPage(ctx, content); err != nil {
			fmt.Printf("Warning: failed to generate static page: %v\n", err)
		}
		s.triggerEmbedding(content.ID)
	} else {
		// Remove static page if unpublished
		s.removeStaticPage(content.FullPath)
	}

	// Tell search engines about live-URL transitions. Edits to published
	// pages are notified from static generation, only when the HTML changed.
	s.NotifyLiveChange(ctx, &original, content)

	// Rebuild search keyword cache when content changes
	s.triggerKeywordRebuild()

	// Rewrite [[wikilinks]] across all content if title or path changed
	if original.Title != content.Title || original.FullPath != content.FullPath {
		go func(oldTitle, newTitle, oldPath, newPath string) {
			wCtx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
			defer cancel()
			s.UpdateWikilinksOnRename(wCtx, oldTitle, newTitle, oldPath, newPath)
		}(original.Title, content.Title, original.FullPath, content.FullPath)
	}

	// Regenerate index pages that may reference this content via lc:query
	s.triggerIndexRegen()

	// Fire webhook event
	if s.webhookService != nil {
		s.webhookService.FireEvent(ctx, "content.update", map[string]interface{}{
			"id":    content.ID.Hex(),
			"title": content.Title,
			"path":  content.FullPath,
		})
	}

	return nil
}

// PublishContent publishes content and generates static page
func (s *ContentService) PublishContent(ctx context.Context, id primitive.ObjectID) error {
	var content models.Content
	if err := s.db.FindOne(ctx, "content", bson.M{"_id": id}, &content); err != nil {
		return fmt.Errorf("content not found: %w", err)
	}
	if content.ForkID != nil {
		return ErrPublishForkCopy
	}
	if content.Hold {
		return ErrContentHeld
	}

	now := time.Now()
	content.Published = true
	content.PublishedAt = &now

	if err := s.UpdateContent(ctx, &content); err != nil {
		return err
	}

	// Fire webhook event
	if s.webhookService != nil {
		s.webhookService.FireEvent(ctx, "content.publish", map[string]interface{}{
			"id":    content.ID.Hex(),
			"title": content.Title,
			"path":  content.FullPath,
		})
	}
	// Purge Cloudflare cache
	if s.cfService != nil {
		go s.cfService.PurgeByURLs(context.Background(), []string{content.FullPath})
	}
	return nil
}

// UnpublishContent unpublishes content and removes static page
func (s *ContentService) UnpublishContent(ctx context.Context, id primitive.ObjectID) error {
	var content models.Content
	if err := s.db.FindOne(ctx, "content", bson.M{"_id": id}, &content); err != nil {
		return fmt.Errorf("content not found: %w", err)
	}

	content.Published = false
	content.PublishedAt = nil

	if err := s.UpdateContent(ctx, &content); err != nil {
		return err
	}

	// A fork copy was never on the live site: clearing its flag is not an
	// unpublish of the live page at that path (no webhook, no cache purge).
	if content.ForkID != nil {
		return nil
	}

	// Fire webhook event
	if s.webhookService != nil {
		s.webhookService.FireEvent(ctx, "content.unpublish", map[string]interface{}{
			"id":    content.ID.Hex(),
			"title": content.Title,
			"path":  content.FullPath,
		})
	}
	// Purge Cloudflare cache
	if s.cfService != nil {
		go s.cfService.PurgeByURLs(context.Background(), []string{content.FullPath})
	}
	return nil
}

// DeleteContent soft-deletes content
func (s *ContentService) DeleteContent(ctx context.Context, id primitive.ObjectID) error {
	var content models.Content
	if err := s.db.FindOne(ctx, "content", bson.M{"_id": id}, &content); err != nil {
		return fmt.Errorf("content not found: %w", err)
	}

	// A deleted row is never left flagged published: a query that forgets
	// the deleted filter must not find it. RestoreContent publishes it again.
	now := time.Now()
	update := bson.M{
		"$set": bson.M{
			"deleted":                 true,
			"deleted_at":              now,
			"updated_at":              now,
			"published":               false,
			"published_before_delete": content.Published,
		},
	}

	if err := s.db.UpdateOne(ctx, "content", bson.M{"_id": id}, update); err != nil {
		return fmt.Errorf("failed to delete content: %w", err)
	}

	// A fork copy shares its full_path with the live page. Deleting the copy
	// changes nothing on the live site: the live page's static file stays,
	// and no content.delete webhook, Cloudflare purge, IndexNow ping, keyword
	// rebuild or index regeneration is issued for the live path.
	if content.ForkID != nil {
		return nil
	}

	// Remove static page
	s.removeStaticPage(content.FullPath)
	s.NotifyLiveChange(ctx, &content, nil)

	// Rebuild search keyword cache
	s.triggerKeywordRebuild()

	// Regenerate index pages that referenced this content
	s.triggerIndexRegen()

	// Fire webhook event
	if s.webhookService != nil {
		s.webhookService.FireEvent(ctx, "content.delete", map[string]interface{}{
			"id":    content.ID.Hex(),
			"title": content.Title,
			"path":  content.FullPath,
		})
	}
	// Purge Cloudflare cache
	if s.cfService != nil {
		go s.cfService.PurgeByURLs(context.Background(), []string{content.FullPath})
	}

	return nil
}

// RestoreContent restores soft-deleted content
func (s *ContentService) RestoreContent(ctx context.Context, id primitive.ObjectID) error {
	set := bson.M{
		"deleted":    false,
		"updated_at": time.Now(),
	}
	// A page DeleteContent unpublished comes back published, unless it is a
	// fork copy or was put on hold in the meantime.
	var deleted models.Content
	if err := s.db.FindOne(ctx, "content", bson.M{"_id": id}, &deleted); err == nil &&
		deleted.PublishedBeforeDelete && deleted.ForkID == nil && !deleted.Hold {
		set["published"] = true
	}
	update := bson.M{
		"$set": set,
		"$unset": bson.M{
			"deleted_at":              "",
			"published_before_delete": "",
		},
	}

	if err := s.db.UpdateOne(ctx, "content", bson.M{"_id": id}, update); err != nil {
		return fmt.Errorf("failed to restore content: %w", err)
	}

	// Regenerate static page if published
	var content models.Content
	if err := s.db.FindOne(ctx, "content", bson.M{"_id": id}, &content); err != nil {
		return nil // Content restored, just can't regenerate
	}

	if content.Published {
		s.GenerateStaticPage(ctx, &content)
		if content.ForkID == nil {
			s.notifyIndexNow(ctx, content.FullPath)
		}
	}

	// Rebuild search keyword cache
	s.triggerKeywordRebuild()

	return nil
}

// GetContent retrieves content by ID
func (s *ContentService) GetContent(ctx context.Context, id primitive.ObjectID) (*models.Content, error) {
	var content models.Content
	if err := s.db.FindOne(ctx, "content", bson.M{"_id": id}, &content); err != nil {
		return nil, fmt.Errorf("content not found: %w", err)
	}
	return &content, nil
}

// GetContentByPath retrieves content by full path
func (s *ContentService) GetContentByPath(ctx context.Context, path string) (*models.Content, error) {
	var content models.Content
	// Live pages only: a fork copy shares its full_path with the live page
	// (or, for a page created inside a fork, has no live page at all) and
	// must never be what a path resolves to. Fork-aware callers use
	// ForkService.GetForkPageByPath.
	exact := bson.M{"full_path": path, "deleted": bson.M{"$ne": true}}
	liveOnly(exact)
	if err := s.db.FindOne(ctx, "content", exact, &content); err == nil {
		return &content, nil
	}
	// Paths are case-insensitive: fall back to a case-insensitive exact
	// match so /claude.md resolves the page stored as /CLAUDE.md. The
	// canonical (stored) casing is preserved in the returned content.
	ciFilter := bson.M{
		"full_path": caseInsensitivePathRegex(path),
		"deleted":   bson.M{"$ne": true},
		"fork_id":   nil,
	}
	if err := s.db.FindOne(ctx, "content", ciFilter, &content); err != nil {
		return nil, fmt.Errorf("content not found: %w", err)
	}
	return &content, nil
}

// caseInsensitivePathRegex builds an anchored, case-insensitive exact-match
// filter for a full_path (QuoteMeta handles dots and other specials).
func caseInsensitivePathRegex(path string) bson.M {
	return bson.M{"$regex": "^" + regexp.QuoteMeta(path) + "$", "$options": "i"}
}

// findPathCaseConflict returns a live page whose full_path equals fullPath
// case-insensitively but differs in casing (paths are case-insensitive, so
// /CLAUDE.md and /claude.md cannot coexist). excludeID skips the page being
// updated; pass primitive.NilObjectID for creates.
func (s *ContentService) findPathCaseConflict(ctx context.Context, fullPath string, excludeID primitive.ObjectID) *models.Content {
	filter := bson.M{
		"full_path": caseInsensitivePathRegex(fullPath),
		"deleted":   bson.M{"$ne": true},
		"fork_id":   nil,
	}
	if !excludeID.IsZero() {
		filter["_id"] = bson.M{"$ne": excludeID}
	}
	var existing models.Content
	if err := s.db.FindOne(ctx, "content", filter, &existing); err != nil {
		return nil
	}
	if existing.FullPath == fullPath {
		return nil // byte-identical path: the unique index handles true duplicates
	}
	return &existing
}

// GetBacklinks finds all published content that links to the given path via internal_links.
func (s *ContentService) GetBacklinks(ctx context.Context, targetPath string) ([]models.Content, error) {
	filter := bson.M{
		"internal_links": targetPath,
		"deleted":        bson.M{"$ne": true},
		"published":      true,
		"fork_id":        nil, // live pages only
	}
	cursor, err := s.db.FindMany(ctx, "content", filter, options.Find().SetSort(bson.D{{Key: "title", Value: 1}}))
	if err != nil {
		return nil, fmt.Errorf("GetBacklinks: %w", err)
	}
	var results []models.Content
	if err := cursor.All(ctx, &results); err != nil {
		return nil, fmt.Errorf("GetBacklinks decode: %w", err)
	}
	return results, nil
}

// ContentScope describes optional MongoDB-level filters for bulk operations.
// All fields are optional; unset fields are ignored.
type ContentScope struct {
	TemplateName   string
	Category       string
	FolderPath     string // prefix match: full_path starts with FolderPath+"/"
	ContentIDs     []primitive.ObjectID
	IncludeDeleted bool
	IncludeForks   bool // also return fork copies (left out by default)
}

// ListContentScoped fetches content with all filters pushed down to MongoDB,
// avoiding the full-collection-load + application-level filter pattern.
// Fork copies are left out unless scope.IncludeForks is set.
func (s *ContentService) ListContentScoped(ctx context.Context, scope ContentScope) ([]models.Content, error) {
	filter := bson.M{}
	if !scope.IncludeDeleted {
		filter["deleted"] = bson.M{"$ne": true}
	}
	if !scope.IncludeForks {
		liveOnly(filter)
	}
	if scope.TemplateName != "" {
		filter["template_name"] = scope.TemplateName
	}
	if scope.Category != "" {
		filter["category"] = scope.Category
	}
	if scope.FolderPath != "" {
		// Match pages whose full_path starts with the given folder path
		filter["full_path"] = bson.M{"$regex": "^" + regexp.QuoteMeta(scope.FolderPath) + "(/|$)"}
	}
	if len(scope.ContentIDs) > 0 {
		filter["_id"] = bson.M{"$in": scope.ContentIDs}
	}

	cursor, err := s.db.FindMany(ctx, "content", filter, options.Find().SetSort(bson.D{{Key: "updated_at", Value: -1}}))
	if err != nil {
		return nil, fmt.Errorf("failed to list content (scoped): %w", err)
	}
	var contents []models.Content
	if err := cursor.All(ctx, &contents); err != nil {
		return nil, fmt.Errorf("failed to decode content: %w", err)
	}
	return contents, nil
}

// StreamContent returns a raw MongoDB cursor over all non-deleted content,
// sorted by updated_at desc. The caller is responsible for closing the cursor.
// Use this instead of ListContent when processing large result sets to avoid
// loading the entire collection into memory. Fork copies are left out unless
// includeForks is passed as true (search-and-replace must not rewrite them).
func (s *ContentService) StreamContent(ctx context.Context, includeDeleted bool, includeForks ...bool) (*mongo.Cursor, error) {
	filter := bson.M{}
	if !includeDeleted {
		filter["deleted"] = bson.M{"$ne": true}
	}
	if len(includeForks) == 0 || !includeForks[0] {
		liveOnly(filter)
	}
	return s.db.FindMany(ctx, "content", filter, options.Find().SetSort(bson.D{{Key: "updated_at", Value: -1}}))
}

// StreamContentScoped returns a raw MongoDB cursor with scope filters pushed down
// to MongoDB. The caller is responsible for closing the cursor. Fork copies are
// left out unless scope.IncludeForks is set.
func (s *ContentService) StreamContentScoped(ctx context.Context, scope ContentScope) (*mongo.Cursor, error) {
	filter := bson.M{}
	if !scope.IncludeDeleted {
		filter["deleted"] = bson.M{"$ne": true}
	}
	if !scope.IncludeForks {
		liveOnly(filter)
	}
	if scope.TemplateName != "" {
		filter["template_name"] = scope.TemplateName
	}
	if scope.Category != "" {
		filter["category"] = scope.Category
	}
	if scope.FolderPath != "" {
		filter["full_path"] = bson.M{"$regex": "^" + regexp.QuoteMeta(scope.FolderPath) + "(/|$)"}
	}
	if len(scope.ContentIDs) > 0 {
		filter["_id"] = bson.M{"$in": scope.ContentIDs}
	}
	return s.db.FindMany(ctx, "content", filter, options.Find().SetSort(bson.D{{Key: "updated_at", Value: -1}}))
}

// ForkCopyIDs returns which of the given IDs are fork copies, in the order
// given. Scoped bulk operations work on live pages only; callers use this to
// report explicitly requested fork copies as skipped instead of silently
// matching nothing.
func (s *ContentService) ForkCopyIDs(ctx context.Context, ids []primitive.ObjectID) ([]primitive.ObjectID, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	filter := bson.M{"_id": bson.M{"$in": ids}, "fork_id": bson.M{"$ne": nil}}
	cursor, err := s.db.FindMany(ctx, "content", filter, options.Find().SetProjection(bson.M{"_id": 1}))
	if err != nil {
		return nil, fmt.Errorf("failed to check fork copies: %w", err)
	}
	var docs []struct {
		ID primitive.ObjectID `bson:"_id"`
	}
	if err := cursor.All(ctx, &docs); err != nil {
		return nil, fmt.Errorf("failed to decode fork copies: %w", err)
	}
	isFork := make(map[primitive.ObjectID]bool, len(docs))
	for _, d := range docs {
		isFork[d.ID] = true
	}
	var out []primitive.ObjectID
	for _, id := range ids {
		if isFork[id] {
			out = append(out, id)
			isFork[id] = false // report a repeated id once
		}
	}
	return out, nil
}

// GetContentByIDs fetches multiple content items in a single query.
// Returns a map keyed by ObjectID for O(1) lookup.
func (s *ContentService) GetContentByIDs(ctx context.Context, ids []primitive.ObjectID) (map[primitive.ObjectID]*models.Content, error) {
	if len(ids) == 0 {
		return map[primitive.ObjectID]*models.Content{}, nil
	}
	cursor, err := s.db.FindMany(ctx, "content", bson.M{"_id": bson.M{"$in": ids}})
	if err != nil {
		return nil, fmt.Errorf("failed to batch fetch content: %w", err)
	}
	var items []models.Content
	if err := cursor.All(ctx, &items); err != nil {
		return nil, fmt.Errorf("failed to decode content: %w", err)
	}
	m := make(map[primitive.ObjectID]*models.Content, len(items))
	for i := range items {
		m[items[i].ID] = &items[i]
	}
	return m, nil
}

// ListContent lists all content with optional filters. Fork copies are left
// out unless includeForks is passed as true.
func (s *ContentService) ListContent(ctx context.Context, includeDeleted bool, category string, folderID *primitive.ObjectID, includeForks ...bool) ([]models.Content, error) {
	filter := bson.M{}
	if !includeDeleted {
		filter["deleted"] = bson.M{"$ne": true}
	}
	if len(includeForks) == 0 || !includeForks[0] {
		liveOnly(filter)
	}
	if category != "" {
		filter["category"] = category
	}
	if folderID != nil {
		filter["folder_id"] = folderID
	}

	cursor, err := s.db.FindMany(ctx, "content", filter, options.Find().SetSort(bson.D{{Key: "updated_at", Value: -1}}))
	if err != nil {
		return nil, fmt.Errorf("failed to list content: %w", err)
	}

	var contents []models.Content
	if err := cursor.All(ctx, &contents); err != nil {
		return nil, fmt.Errorf("failed to decode content: %w", err)
	}

	return contents, nil
}

// PaginationOpts configures paginated listing.
type PaginationOpts struct {
	Limit          int
	Offset         int
	IncludeDeleted bool
	IncludeForks   bool // also return fork copies (left out by default)
	Category       string
	FolderID       *primitive.ObjectID
}

// PaginationResult carries pagination metadata alongside results.
type PaginationResult struct {
	Total   int64 `json:"total"`
	Limit   int   `json:"limit"`
	Offset  int   `json:"offset"`
	HasMore bool  `json:"has_more"`
}

// ListContentPaginated returns a page of content with total count.
func (s *ContentService) ListContentPaginated(ctx context.Context, opts PaginationOpts) ([]models.Content, PaginationResult, error) {
	filter := bson.M{}
	if !opts.IncludeDeleted {
		filter["deleted"] = bson.M{"$ne": true}
	}
	if !opts.IncludeForks {
		liveOnly(filter)
	}
	if opts.Category != "" {
		filter["category"] = opts.Category
	}
	if opts.FolderID != nil {
		filter["folder_id"] = opts.FolderID
	}

	total, err := s.db.Count(ctx, "content", filter)
	if err != nil {
		return nil, PaginationResult{}, fmt.Errorf("failed to count content: %w", err)
	}

	findOpts := options.Find().
		SetSort(bson.D{{Key: "updated_at", Value: -1}}).
		SetLimit(int64(opts.Limit)).
		SetSkip(int64(opts.Offset))

	cursor, err := s.db.FindMany(ctx, "content", filter, findOpts)
	if err != nil {
		return nil, PaginationResult{}, fmt.Errorf("failed to list content: %w", err)
	}

	var contents []models.Content
	if err := cursor.All(ctx, &contents); err != nil {
		return nil, PaginationResult{}, fmt.Errorf("failed to decode content: %w", err)
	}

	pr := PaginationResult{
		Total:   total,
		Limit:   opts.Limit,
		Offset:  opts.Offset,
		HasMore: int64(opts.Offset+len(contents)) < total,
	}
	return contents, pr, nil
}

// GetVersions retrieves all versions of a content item
func (s *ContentService) GetVersions(ctx context.Context, contentID primitive.ObjectID) ([]models.ContentVersion, error) {
	cursor, err := s.db.FindMany(ctx, "content_versions",
		bson.M{"content_id": contentID},
		options.Find().SetSort(bson.D{{Key: "version", Value: -1}}))
	if err != nil {
		return nil, fmt.Errorf("failed to get versions: %w", err)
	}

	var versions []models.ContentVersion
	if err := cursor.All(ctx, &versions); err != nil {
		return nil, fmt.Errorf("failed to decode versions: %w", err)
	}

	return versions, nil
}

// GetVersion retrieves a specific version
func (s *ContentService) GetVersion(ctx context.Context, contentID primitive.ObjectID, version int) (*models.ContentVersion, error) {
	var v models.ContentVersion
	if err := s.db.FindOne(ctx, "content_versions",
		bson.M{"content_id": contentID, "version": version}, &v); err != nil {
		return nil, fmt.Errorf("version not found: %w", err)
	}
	return &v, nil
}

// RevertToVersion reverts content to a previous version with an optional comment
func (s *ContentService) RevertToVersion(ctx context.Context, contentID primitive.ObjectID, version int, versionComment ...string) error {
	// Get the version to revert to
	v, err := s.GetVersion(ctx, contentID, version)
	if err != nil {
		return err
	}

	// Get current content
	var content models.Content
	if err := s.db.FindOne(ctx, "content", bson.M{"_id": contentID}, &content); err != nil {
		return fmt.Errorf("content not found: %w", err)
	}

	// Update content with version data
	content.TemplateID = v.TemplateID
	content.TemplateName = v.TemplateName
	content.Title = v.Title
	content.Slug = v.Slug
	content.FolderID = v.FolderID
	content.FolderPath = v.FolderPath
	content.FullPath = v.FullPath
	content.Category = v.Category
	content.Tags = v.Tags
	content.MetaDescription = v.MetaDescription
	content.OGImage = v.OGImage
	content.Data = v.Data
	content.Published = v.Published
	content.PublishedAt = v.PublishedAt
	content.UseHeader = v.UseHeader
	content.UseFooter = v.UseFooter
	content.UseTheme = v.UseTheme
	content.RawMode = v.RawMode
	// Versions saved before 7.5 don't have these; the page keeps its values.
	if v.NoIndex != nil {
		content.NoIndex = *v.NoIndex
	}
	if v.AuthorName != nil {
		content.AuthorName = *v.AuthorName
	}
	if v.AuthorURL != nil {
		content.AuthorURL = *v.AuthorURL
	}

	// Pass through the version comment if provided
	return s.UpdateContent(ctx, &content, versionComment...)
}

// saveVersion saves a new version of the content with an optional comment
func (s *ContentService) saveVersion(ctx context.Context, content *models.Content, original *models.Content, comment string) error {
	// Get the current version count
	count, err := s.db.Count(ctx, "content_versions", bson.M{"content_id": content.ID})
	if err != nil {
		return err
	}

	// If no versions exist and we have the original content, save it as v1 first
	if count == 0 && original != nil {
		v1 := models.ContentVersion{
			ContentID:       original.ID,
			Version:         1,
			TemplateID:      original.TemplateID,
			TemplateName:    original.TemplateName,
			Title:           original.Title,
			Slug:            original.Slug,
			FolderID:        original.FolderID,
			FolderPath:      original.FolderPath,
			FullPath:        original.FullPath,
			Category:        original.Category,
			Tags:            original.Tags,
			MetaDescription: original.MetaDescription,
			OGImage:         original.OGImage,
			Data:            original.Data,
			Published:       original.Published,
			PublishedAt:     original.PublishedAt,
			UseHeader:       original.UseHeader,
			UseFooter:       original.UseFooter,
			UseTheme:        original.UseTheme,
			RawMode:         original.RawMode,
			NoIndex:         boolPtr(original.NoIndex),
			AuthorName:      stringPtr(original.AuthorName),
			AuthorURL:       stringPtr(original.AuthorURL),
			CreatedAt:       original.CreatedAt,
		}
		if _, err := s.db.InsertOne(ctx, "content_versions", v1); err != nil {
			return err
		}
		count = 1
	}

	version := int(count) + 1

	modifiedByEmail := EditorEmailFromContext(ctx)
	prov, _ := ProvenanceFromContext(ctx)
	if prov.Actor == "" {
		prov.Actor = "human" // default for legacy paths that set no provenance
	}
	contentVersion := models.ContentVersion{
		ContentID:       content.ID,
		Version:         version,
		Comment:         comment,
		ModifiedByEmail: modifiedByEmail,
		Actor:           prov.Actor,
		Via:             prov.Via,
		AgentSession:    prov.AgentSession,
		TemplateID:      content.TemplateID,
		TemplateName:    content.TemplateName,
		Title:           content.Title,
		Slug:            content.Slug,
		FolderID:        content.FolderID,
		FolderPath:      content.FolderPath,
		FullPath:        content.FullPath,
		Category:        content.Category,
		Tags:            content.Tags,
		MetaDescription: content.MetaDescription,
		OGImage:         content.OGImage,
		Data:            content.Data,
		Published:       content.Published,
		PublishedAt:     content.PublishedAt,
		UseHeader:       content.UseHeader,
		UseFooter:       content.UseFooter,
		UseTheme:        content.UseTheme,
		RawMode:         content.RawMode,
		NoIndex:         boolPtr(content.NoIndex),
		AuthorName:      stringPtr(content.AuthorName),
		AuthorURL:       stringPtr(content.AuthorURL),
		CreatedAt:       time.Now(),
	}

	_, err = s.db.InsertOne(ctx, "content_versions", contentVersion)
	return err
}

// getAuthorRole looks up the role of a user by ID.
// Returns "admin" for zero IDs (legacy content), "editor" on any error.
func (s *ContentService) getAuthorRole(ctx context.Context, userID primitive.ObjectID) string {
	if userID.IsZero() {
		return "admin" // legacy content created before multi-user system
	}
	var user struct {
		Role string `bson:"role"`
	}
	if err := s.db.Collection("users").FindOne(ctx, bson.M{"_id": userID}).Decode(&user); err != nil || user.Role == "" {
		return "editor" // default to restrictive on error
	}
	return user.Role
}

// getContentAuthorRole looks up the role of the user who last modified the content
// by finding the most recent content version with a ModifiedBy field.
// Returns "admin" for legacy content (no modifier recorded), "editor" on error.
func (s *ContentService) getContentAuthorRole(ctx context.Context, content *models.Content) string {
	var latestVersion struct {
		ModifiedBy *primitive.ObjectID `bson:"modified_by"`
	}
	opts := options.FindOne().SetSort(bson.D{{Key: "version", Value: -1}})
	err := s.db.Collection("content_versions").FindOne(ctx,
		bson.M{"content_id": content.ID, "modified_by": bson.M{"$exists": true}},
		opts,
	).Decode(&latestVersion)
	if err != nil || latestVersion.ModifiedBy == nil {
		return "admin" // no modifier recorded — treat as legacy/admin content
	}
	return s.getAuthorRole(ctx, *latestVersion.ModifiedBy)
}

// GenerateStaticPage renders and saves the content as a static HTML file
func (s *ContentService) GenerateStaticPage(ctx context.Context, content *models.Content) error {
	return s.generateStaticPageWithWikilinkIndex(ctx, content, nil)
}

// generateStaticPageWithWikilinkIndex renders and saves the content as a static HTML file,
// optionally using a pre-built wikilink index (pass nil to build one on demand).
func (s *ContentService) generateStaticPageWithWikilinkIndex(ctx context.Context, content *models.Content, prebuiltIdx *wikilinkIndex) error {
	// Fork copies share full_path with their live counterpart — generating
	// them would overwrite the live page's static file with sandbox content.
	if content.ForkID != nil {
		return nil
	}
	// Get template
	var tmpl models.Template
	if err := s.db.FindOne(ctx, "templates", bson.M{"_id": content.TemplateID}, &tmpl); err != nil {
		return fmt.Errorf("template not found: %w", err)
	}

	// Determine script policy from site config
	siteConfig, _ := s.db.GetSiteConfig(ctx)
	policy := "all"
	if siteConfig != nil && siteConfig.MarkdownScriptPolicy != "" {
		policy = siteConfig.MarkdownScriptPolicy
	}

	var allowUnsafeScripts bool
	switch policy {
	case "none":
		allowUnsafeScripts = false
	case "admin_only":
		// Look up the role of the content's last modifier via content versions
		// Since Content doesn't store ModifiedBy directly, we use the most recent version's modifier.
		// Fall back to checking the content's creator via the versions collection.
		authorRole := s.getContentAuthorRole(ctx, content)
		allowUnsafeScripts = (authorRole == "admin")
	default: // "all" or ""
		allowUnsafeScripts = true
	}

	// Process lc:query directives in the raw template layout BEFORE html/template rendering,
	// because html/template strips HTML comments (which is how lc:query directives are expressed).
	layout := tmpl.HTMLLayout
	if strings.Contains(layout, "lc:query") {
		var lcErr error
		layout, lcErr = s.processQueryDirectives(ctx, layout)
		if lcErr != nil {
			log.Printf("Warning: lc:query processing error for %s: %v", content.FullPath, lcErr)
		}
	}

	// Build processed data map: handle markdown fields, snippet includes, and lc_toc placeholder.
	processedData := buildProcessedData(ctx, s.db, content, tmpl.Fields, allowUnsafeScripts)

	// Render content using the processed data map.
	html, err := s.renderContentFromLayoutWithData(processedData, layout)
	if err != nil {
		return fmt.Errorf("failed to render content: %w", err)
	}

	// Inject id= attributes into heading tags that don't already have one.
	html = injectHeadingIDs(html)

	// Build and inject the table of contents (replaces tocPlaceholder).
	html = buildAndInjectTOC(html)

	// Resolve [[wikilinks]] in the rendered output
	if strings.Contains(html, "[[") {
		idx := prebuiltIdx
		if idx == nil {
			idx = s.buildWikilinkIndex(ctx)
		}
		html = processWikiLinks(idx, html)
	}

	// Compute content hash — skip write if unchanged (conditional republish)
	hash := fmt.Sprintf("%x", sha256.Sum256([]byte(html)))
	if content.ContentHash == hash {
		return nil // HTML hasn't changed, skip write
	}

	// Determine file path
	filePath := content.FullPath
	if filePath == "" || filePath == "/" {
		filePath = "/index"
	}
	filePath = "content/generated" + filePath + ".html"

	// Create directory if needed
	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	// Write file
	if err := os.WriteFile(filePath, []byte(html), 0644); err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	// Update content hash in DB
	// content_modified_at records when the rendered page actually changed —
	// accurate dateModified / lastmod / feed dates, unlike updated_at, which
	// moves on no-op saves. A site-wide re-render (WithoutIndexNow) changes
	// layout, not content, so it doesn't count.
	content.ContentHash = hash
	set := bson.M{"content_hash": hash}
	if suppressed, _ := ctx.Value(indexNowSuppressKey{}).(bool); !suppressed {
		now := time.Now()
		content.ContentModifiedAt = &now
		set["content_modified_at"] = now
	}
	s.db.UpdateOne(ctx, "content", bson.M{"_id": content.ID}, bson.M{"$set": set})

	// The rendered page actually changed — tell search engines.
	s.notifyIndexNow(ctx, content.FullPath)

	return nil
}

// RemoveStaticPage removes a live page's generated HTML by its full path. It
// is for callers that write content to MongoDB directly (the admin editor);
// the caller is responsible for never passing a fork copy's path.
func (s *ContentService) RemoveStaticPage(fullPath string) {
	s.removeStaticPage(fullPath)
}

// removeStaticPage removes the static HTML file for content
func (s *ContentService) removeStaticPage(fullPath string) {
	if fullPath == "" {
		// Legacy rows can have an empty full_path. Mapping that to /index
		// would delete the homepage's static file.
		return
	}
	if fullPath == "/" {
		fullPath = "/index"
	}
	filePath := "content/generated" + fullPath + ".html"
	os.Remove(filePath)
}

// renderContent renders content using its template's HTML layout.
func (s *ContentService) renderContent(content *models.Content, tmpl *models.Template) (string, error) {
	return s.renderContentFromLayout(content, tmpl.HTMLLayout)
}

// renderContentFromLayout renders content using the provided HTML layout string.
func (s *ContentService) renderContentFromLayout(content *models.Content, layout string) (string, error) {
	// Build data map with template.HTML for string values
	data := make(map[string]interface{})
	for k, v := range content.Data {
		if str, ok := v.(string); ok {
			data[k] = template.HTML(str)
		} else {
			data[k] = v
		}
	}

	// Add title
	data["title"] = content.Title

	return s.renderContentFromLayoutWithData(data, layout)
}

// renderContentFromLayoutWithData renders a layout using a pre-built data map.
func (s *ContentService) renderContentFromLayoutWithData(data map[string]interface{}, layout string) (string, error) {
	// Parse and execute template
	t, err := template.New("content").Parse(layout)
	if err != nil {
		return "", fmt.Errorf("failed to parse template: %w", err)
	}

	var buf strings.Builder
	if err := t.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("failed to execute template: %w", err)
	}

	return buf.String(), nil
}

// extractInternalLinks extracts all internal links from content data
func (s *ContentService) extractInternalLinks(content *models.Content) []string {
	linkSet := make(map[string]bool)
	linkRegex := regexp.MustCompile(`href=["'](/[^"'#?]*)["']`)

	for _, value := range content.Data {
		if strVal, ok := value.(string); ok {
			matches := linkRegex.FindAllStringSubmatch(strVal, -1)
			for _, match := range matches {
				if len(match) > 1 {
					link := strings.TrimSuffix(match[1], "/")
					if link == "" {
						link = "/"
					}
					linkSet[link] = true
				}
			}
		}
	}

	links := make([]string, 0, len(linkSet))
	for link := range linkSet {
		links = append(links, link)
	}
	return links
}

// QueryContentForDirective fetches published content matching the given filter criteria, sorted as specified.
// filter: map of field->value (supported keys: tag, category, template, folder)
// sortField: "title", "created_at", "published_at" — sortDir: "asc" or "desc"
func (s *ContentService) QueryContentForDirective(ctx context.Context, filter map[string]string, sortField, sortDir string) ([]models.Content, error) {
	// Live pages only: lc:query output is written into public index pages,
	// so a fork copy must never be a result, whatever its published flag says.
	q := bson.M{"published": true, "deleted": bson.M{"$ne": true}, "fork_id": nil}

	if v, ok := filter["tag"]; ok && v != "" {
		q["tags"] = v
	}
	if v, ok := filter["category"]; ok && v != "" {
		q["category"] = v
	}
	if v, ok := filter["template"]; ok && v != "" {
		q["template_name"] = v
	}
	if v, ok := filter["folder"]; ok && v != "" {
		q["folder_path"] = bson.M{"$regex": "^" + regexp.QuoteMeta(v)}
	}

	// Cap at 10 000 items — lc:query is for rendering index pages, not bulk exports.
	// Prevents a single directive from loading the entire content collection into memory.
	cursor, err := s.db.FindMany(ctx, "content", q, options.Find().SetLimit(10000))
	if err != nil {
		return nil, fmt.Errorf("lc:query failed: %w", err)
	}
	var contents []models.Content
	if err := cursor.All(ctx, &contents); err != nil {
		return nil, fmt.Errorf("lc:query decode: %w", err)
	}

	// Sort in Go (avoids mongo index requirements for arbitrary sort fields)
	dir := 1
	if sortDir == "desc" {
		dir = -1
	}
	switch sortField {
	case "title", "":
		sort.Slice(contents, func(i, j int) bool {
			if dir == 1 {
				return strings.ToLower(contents[i].Title) < strings.ToLower(contents[j].Title)
			}
			return strings.ToLower(contents[i].Title) > strings.ToLower(contents[j].Title)
		})
	case "created_at":
		sort.Slice(contents, func(i, j int) bool {
			if dir == 1 {
				return contents[i].CreatedAt.Before(contents[j].CreatedAt)
			}
			return contents[i].CreatedAt.After(contents[j].CreatedAt)
		})
	case "published_at":
		sort.Slice(contents, func(i, j int) bool {
			ai := contents[i].PublishedAt
			aj := contents[j].PublishedAt
			if ai == nil {
				return dir == -1
			}
			if aj == nil {
				return dir == 1
			}
			if dir == 1 {
				return ai.Before(*aj)
			}
			return ai.After(*aj)
		})
	}
	return contents, nil
}

// lcQueryDirectiveRE matches <!-- lc:query ... --> comment blocks (single-line or across multiple lines).
var lcQueryDirectiveRE = regexp.MustCompile(`(?s)<!--\s*lc:query\s+(.*?)-->`)

// wikilinkRE matches [[target]] or [[target|display text]] wiki-style internal links.
var wikilinkRE = regexp.MustCompile(`\[\[([^\]|]+?)(?:\|([^\]]+?))?\]\]`)

// tocPlaceholder is the internal marker replaced by the generated TOC HTML.
const tocPlaceholder = "__LCTOC__"

// snippetIncludeRE matches [[include:snippet-name]] inline snippet inclusions.
var snippetIncludeRE = regexp.MustCompile(`\[\[include:([a-zA-Z0-9_-]+)\]\]`)

// inlineTagRE matches #tagname patterns in text fields.
var inlineTagRE = regexp.MustCompile(`(?:^|[\s,;(])#([a-zA-Z][a-zA-Z0-9_-]{0,49})`)

// headingRE matches HTML heading tags (h1-h6) including content, single-line.
var headingRE = regexp.MustCompile(`(?is)<(h[1-6])([^>]*)>(.*?)</h[1-6]>`)

// headingWithIDRE matches heading tags that already have an id= attribute.
var headingWithIDRE = regexp.MustCompile(`(?is)<(h[1-6])[^>]*\sid="([^"]+)"[^>]*>(.*?)</h[1-6]>`)

// tagStripRE strips HTML tags for text extraction.
var tagStripRE = regexp.MustCompile(`<[^>]+>`)

// slugifyRE replaces non-alphanumeric runs with hyphens.
var slugifyRE = regexp.MustCompile(`[^a-z0-9]+`)

// editorSafePolicy strips script-execution vectors (script, iframe, event handlers,
// javascript: URIs) but allows all other HTML including tables, divs, custom classes,
// inline styles, etc.
var editorSafePolicy = func() *bluemonday.Policy {
	p := bluemonday.NewPolicy()
	// Allow all standard HTML elements except script-execution ones
	// (script, iframe, object, embed, form, input, base, link, meta, style are blocked by omission)
	p.AllowElements(
		"a", "abbr", "acronym", "address", "article", "aside", "audio",
		"b", "blockquote", "br", "caption", "cite", "code", "col", "colgroup",
		"dd", "del", "details", "dfn", "div", "dl", "dt",
		"em", "figcaption", "figure", "footer",
		"h1", "h2", "h3", "h4", "h5", "h6", "header", "hr",
		"i", "img", "ins", "kbd", "li", "main", "mark", "nav",
		"ol", "p", "picture", "pre", "q", "rp", "rt", "ruby",
		"s", "samp", "section", "small", "source", "span", "strong", "sub", "summary", "sup",
		"table", "tbody", "td", "tfoot", "th", "thead", "time", "tr", "track",
		"u", "ul", "var", "video", "wbr",
	)
	p.AllowAttrs("href").OnElements("a")
	p.AllowURLSchemes("http", "https", "mailto", "tel")
	p.AllowAttrs("target", "rel").OnElements("a")
	p.AllowAttrs("src", "alt", "width", "height", "loading", "decoding").OnElements("img")
	p.AllowAttrs("src", "type").OnElements("source", "track")
	p.AllowAttrs("src", "controls", "autoplay", "loop", "muted", "preload", "width", "height").OnElements("video", "audio")
	p.AllowAttrs("id", "class", "style", "title", "lang", "dir").Globally()
	p.AllowAttrs("colspan", "rowspan", "scope", "headers").OnElements("td", "th")
	p.AllowAttrs("span").OnElements("col", "colgroup")
	p.AllowAttrs("open").OnElements("details")
	p.AllowAttrs("datetime").OnElements("del", "ins", "time")
	p.AllowAttrs("start", "reversed", "type").OnElements("ol")
	p.AllowAttrs("type", "value").OnElements("li")
	p.AllowAttrs("cite").OnElements("blockquote", "q", "del", "ins")
	p.AllowAttrs("srcset", "media", "sizes").OnElements("source", "picture")
	return p
}()

// SanitizeContentData strips script-execution vectors from all string fields in
// content Data when the script policy requires it. Called at write time as
// defense-in-depth (the primary enforcement is at render/generation time).
func SanitizeContentData(data map[string]interface{}) map[string]interface{} {
	if data == nil {
		return data
	}
	for k, v := range data {
		if s, ok := v.(string); ok {
			data[k] = editorSafePolicy.Sanitize(s)
		}
	}
	return data
}

// markdownToHTML converts markdown text to HTML using goldmark with GFM extensions.
// If allowUnsafe is true, raw HTML pass-through is enabled (goldmark WithUnsafe).
// If allowUnsafe is false, the output is sanitized by editorSafePolicy.
func markdownToHTML(text string, allowUnsafe bool) string {
	var opts []goldmark.Option
	opts = append(opts, goldmark.WithExtensions(extension.GFM))
	if allowUnsafe {
		opts = append(opts, goldmark.WithRendererOptions(goldmarkhtml.WithUnsafe()))
	}
	md := goldmark.New(opts...)
	var buf bytes.Buffer
	if err := md.Convert([]byte(text), &buf); err != nil {
		return text
	}
	result := buf.String()
	if !allowUnsafe {
		result = editorSafePolicy.Sanitize(result)
	}
	return result
}

// expandSnippetIncludes replaces [[include:snippet-name]] patterns with the snippet's HTML.
func expandSnippetIncludes(ctx context.Context, db *database.DB, text string, content *models.Content) string {
	return expandSnippetIncludesDepth(ctx, db, text, content, 0, map[string]bool{})
}

func expandSnippetIncludesDepth(ctx context.Context, db *database.DB, text string, content *models.Content, depth int, visited map[string]bool) string {
	if depth >= 3 {
		return text // max recursion depth
	}
	if !strings.Contains(text, "[[include:") {
		return text
	}
	return snippetIncludeRE.ReplaceAllStringFunc(text, func(match string) string {
		sub := snippetIncludeRE.FindStringSubmatch(match)
		if len(sub) < 2 {
			return match
		}
		snippetName := sub[1]
		if visited[snippetName] {
			return "" // cycle detected — drop silently
		}
		var snip models.Snippet
		if err := db.FindOne(ctx, "snippets", bson.M{"name": snippetName}, &snip); err != nil {
			return match // snippet not found, leave in place
		}
		rendered, err := renderSnippet(snip.HTML, *content)
		if err != nil {
			return match
		}
		visited[snippetName] = true
		rendered = expandSnippetIncludesDepth(ctx, db, rendered, content, depth+1, visited)
		delete(visited, snippetName) // allow same snippet in different branches
		return rendered
	})
}

// buildProcessedData builds the template data map, pre-processing markdown fields and snippet includes.
// allowUnsafeScripts controls whether raw HTML/scripts are permitted in markdown and richtext fields.
func buildProcessedData(ctx context.Context, db *database.DB, content *models.Content, fields []models.TemplateField, allowUnsafeScripts bool) map[string]interface{} {
	// Build a lookup for field types
	fieldTypes := make(map[string]string, len(fields))
	for _, f := range fields {
		fieldTypes[f.Name] = f.Type
	}

	data := make(map[string]interface{})
	for k, v := range content.Data {
		strVal, ok := v.(string)
		if !ok {
			if v != nil {
				data[k] = v
			}
			continue
		}
		// Expand snippet includes first
		strVal = expandSnippetIncludes(ctx, db, strVal, content)
		// Convert markdown fields
		if fieldTypes[k] == "markdown" {
			strVal = markdownToHTML(strVal, allowUnsafeScripts)
		} else if !allowUnsafeScripts && fieldTypes[k] == "richtext" {
			// Sanitize richtext fields too when script policy requires it
			strVal = editorSafePolicy.Sanitize(strVal)
		}
		data[k] = template.HTML(strVal)
	}
	// Add title
	data["title"] = content.Title
	// Add TOC placeholder as unescaped HTML
	data["lc_toc"] = template.HTML(tocPlaceholder)
	return data
}

// headingSlug converts heading text content to a URL-safe ID slug.
func headingSlug(text string) string {
	text = tagStripRE.ReplaceAllString(text, "")
	text = strings.ToLower(strings.TrimSpace(text))
	text = slugifyRE.ReplaceAllString(text, "-")
	text = strings.Trim(text, "-")
	return text
}

// injectHeadingIDs adds id= attributes to heading tags that don't already have one.
func injectHeadingIDs(html string) string {
	return headingRE.ReplaceAllStringFunc(html, func(match string) string {
		parts := headingRE.FindStringSubmatch(match)
		if len(parts) < 4 {
			return match
		}
		tag, attrs, content := parts[1], parts[2], parts[3]
		if strings.Contains(strings.ToLower(attrs), "id=") {
			return match
		}
		id := headingSlug(content)
		if id == "" {
			return match
		}
		return fmt.Sprintf(`<%s id="%s"%s>%s</%s>`, tag, id, attrs, content, tag)
	})
}

// buildAndInjectTOC scans for headings with IDs, builds a nav TOC, and substitutes the tocPlaceholder.
func buildAndInjectTOC(html string) string {
	type tocEntry struct {
		level int
		id    string
		text  string
	}

	var entries []tocEntry
	headingWithIDRE.ReplaceAllStringFunc(html, func(match string) string {
		parts := headingWithIDRE.FindStringSubmatch(match)
		if len(parts) < 4 {
			return match
		}
		tag, id, content := parts[1], parts[2], parts[3]
		level := int(tag[1] - '0')
		text := tagStripRE.ReplaceAllString(content, "")
		entries = append(entries, tocEntry{level, id, strings.TrimSpace(text)})
		return match
	})

	if len(entries) == 0 {
		return strings.ReplaceAll(html, tocPlaceholder, "")
	}

	var toc strings.Builder
	toc.WriteString(`<nav class="lc-toc"><ul>`)
	for _, e := range entries {
		toc.WriteString(fmt.Sprintf(`<li class="toc-h%d"><a href="#%s">%s</a></li>`, e.level, e.id, template.HTMLEscapeString(e.text)))
	}
	toc.WriteString(`</ul></nav>`)

	return strings.ReplaceAll(html, tocPlaceholder, toc.String())
}

// mergeInlineTags scans content data fields for #tagname patterns and merges found tags into content.Tags.
func mergeInlineTags(content *models.Content) {
	found := make(map[string]bool)
	for _, tag := range content.Tags {
		found[strings.ToLower(tag)] = true
	}

	for _, val := range content.Data {
		strVal, ok := val.(string)
		if !ok {
			continue
		}
		matches := inlineTagRE.FindAllStringSubmatch(strVal, -1)
		for _, m := range matches {
			if len(m) > 1 {
				tag := strings.ToLower(m[1])
				if !found[tag] {
					found[tag] = true
					content.Tags = append(content.Tags, m[1])
				}
			}
		}
	}
}

// wikilinkIndex holds bidirectional lookup maps for wikilink resolution.
type wikilinkIndex struct {
	titleToPath map[string]string // lowercase title → full_path
	pathToTitle map[string]string // full_path → canonical title
}

// buildWikilinkIndex returns a cached wikilink index, rebuilding it at most
// once per wikilinkCacheTTL. This prevents a full collection scan on every
// page publish/regenerate when many pages are updated in sequence.
func (s *ContentService) buildWikilinkIndex(ctx context.Context) *wikilinkIndex {
	s.wikilinkCacheMu.RLock()
	if s.wikilinkCache != nil && time.Now().Before(s.wikilinkCacheExp) {
		cached := s.wikilinkCache
		s.wikilinkCacheMu.RUnlock()
		return cached
	}
	s.wikilinkCacheMu.RUnlock()

	// Rebuild the index
	idx := &wikilinkIndex{
		titleToPath: make(map[string]string),
		pathToTitle: make(map[string]string),
	}
	// Live pages only: a fork copy flagged published must not become the
	// target a [[wikilink]] on the live site resolves to.
	cursor, err := s.db.FindMany(ctx, "content", bson.M{
		"published": true,
		"deleted":   bson.M{"$ne": true},
		"fork_id":   nil,
	}, options.Find().SetProjection(bson.D{
		{Key: "title", Value: 1},
		{Key: "full_path", Value: 1},
	}))
	if err == nil {
		for cursor.Next(ctx) {
			var c struct {
				Title    string `bson:"title"`
				FullPath string `bson:"full_path"`
			}
			if cursor.Decode(&c) == nil && c.FullPath != "" {
				idx.titleToPath[strings.ToLower(c.Title)] = c.FullPath
				idx.pathToTitle[c.FullPath] = c.Title
			}
		}
		cursor.Close(ctx)
	}

	s.wikilinkCacheMu.Lock()
	s.wikilinkCache = idx
	s.wikilinkCacheExp = time.Now().Add(wikilinkCacheTTL)
	s.wikilinkCacheMu.Unlock()

	return idx
}

// invalidateWikilinkCache forces the next buildWikilinkIndex call to rebuild.
// Called after any publish/title/path change.
func (s *ContentService) invalidateWikilinkCache() {
	s.wikilinkCacheMu.Lock()
	s.wikilinkCacheExp = time.Time{}
	s.wikilinkCacheMu.Unlock()
}

// processWikiLinks resolves [[wikilink]] and [[wikilink|display]] patterns in html.
// Resolved links become <a href="...">text</a>.
// Unresolved links become <span class="broken-link" data-target="...">text</span>.
func processWikiLinks(idx *wikilinkIndex, html string) string {
	if !strings.Contains(html, "[[") {
		return html
	}
	return wikilinkRE.ReplaceAllStringFunc(html, func(match string) string {
		sub := wikilinkRE.FindStringSubmatch(match)
		if len(sub) < 2 {
			return match
		}
		target := strings.TrimSpace(sub[1])
		var display string
		if len(sub) > 2 {
			display = strings.TrimSpace(sub[2])
		}

		var href, label string
		if strings.HasPrefix(target, "/") {
			// Path-based lookup: [[/full/path]] or [[/full/path|display]]
			if title, ok := idx.pathToTitle[target]; ok {
				href = target
				label = display
				if label == "" {
					label = title
				}
			}
		} else {
			// Title-based lookup (case-insensitive): [[Page Title]] or [[Page Title|display]]
			if path, ok := idx.titleToPath[strings.ToLower(target)]; ok {
				href = path
				label = display
				if label == "" {
					label = target
				}
			}
		}

		if href == "" {
			dispText := display
			if dispText == "" {
				dispText = target
			}
			return `<span class="broken-link" data-target="` +
				template.HTMLEscapeString(target) + `">` +
				template.HTMLEscapeString(dispText) + `</span>`
		}
		return `<a href="` + template.HTMLEscapeString(href) + `">` +
			template.HTMLEscapeString(label) + `</a>`
	})
}

// UpdateWikilinksOnRename rewrites [[OldTitle]] → [[NewTitle]] and [[/old/path]] → [[/new/path]]
// across all content data fields. Safe to call from a goroutine.
func (s *ContentService) UpdateWikilinksOnRename(ctx context.Context, oldTitle, newTitle, oldPath, newPath string) {
	if oldTitle == newTitle && oldPath == newPath {
		return
	}

	filter := bson.M{"deleted": bson.M{"$ne": true}}

	// Stream documents one-by-one to avoid loading the entire collection into memory.
	cursor, err := s.db.FindMany(ctx, "content", filter)
	if err != nil {
		log.Printf("Warning: UpdateWikilinksOnRename: query error: %v", err)
		return
	}
	defer cursor.Close(ctx)

	for cursor.Next(ctx) {
		var c models.Content
		if err := cursor.Decode(&c); err != nil {
			continue
		}
		updated := false
		newData := make(map[string]interface{})
		for k, v := range c.Data {
			if str, ok := v.(string); ok {
				rewritten := rewriteWikilinksInText(str, oldTitle, newTitle, oldPath, newPath)
				newData[k] = rewritten
				if rewritten != str {
					updated = true
				}
			} else {
				newData[k] = v
			}
		}
		if updated {
			if err := s.db.UpdateOne(ctx, "content", bson.M{"_id": c.ID}, bson.M{"$set": bson.M{
				"data":       newData,
				"updated_at": time.Now(),
			}}); err != nil {
				log.Printf("Warning: UpdateWikilinksOnRename: update error for %s: %v", c.ID.Hex(), err)
			}
		}
	}
}

// rewriteWikilinksInText rewrites wikilink targets in a single text string.
func rewriteWikilinksInText(text, oldTitle, newTitle, oldPath, newPath string) string {
	if !strings.Contains(text, "[[") {
		return text
	}
	return wikilinkRE.ReplaceAllStringFunc(text, func(match string) string {
		sub := wikilinkRE.FindStringSubmatch(match)
		if len(sub) < 2 {
			return match
		}
		target := strings.TrimSpace(sub[1])
		var displayPart string
		if len(sub) > 2 && strings.TrimSpace(sub[2]) != "" {
			displayPart = "|" + sub[2]
		}
		if oldTitle != "" && newTitle != "" && oldTitle != newTitle && strings.EqualFold(target, oldTitle) {
			return "[[" + newTitle + displayPart + "]]"
		}
		if oldPath != "" && newPath != "" && oldPath != newPath && target == oldPath {
			return "[[" + newPath + displayPart + "]]"
		}
		return match
	})
}

// parseDirectiveAttrs parses key="value" pairs from a directive attribute string.
func parseDirectiveAttrs(s string) map[string]string {
	attrs := make(map[string]string)
	re := regexp.MustCompile(`(\w+)="([^"]*)"`)
	for _, m := range re.FindAllStringSubmatch(s, -1) {
		attrs[m[1]] = m[2]
	}
	return attrs
}

// processQueryDirectives scans html for <!-- lc:query ... --> directives, evaluates each
// against the database, renders each result through the named snippet, and returns the
// final HTML with directives replaced.
func (s *ContentService) processQueryDirectives(ctx context.Context, html string) (string, error) {
	var firstErr error
	result := lcQueryDirectiveRE.ReplaceAllStringFunc(html, func(match string) string {
		sub := lcQueryDirectiveRE.FindStringSubmatch(match)
		if len(sub) < 2 {
			return match
		}
		attrs := parseDirectiveAttrs(sub[1])

		// Resolve filter — supports both direct keys (tag="X") and compound filter="key:value"
		filterMap := make(map[string]string)
		for _, key := range []string{"tag", "category", "template", "folder"} {
			if v, ok := attrs[key]; ok {
				filterMap[key] = v
			}
		}
		if fv, ok := attrs["filter"]; ok {
			parts := strings.SplitN(fv, ":", 2)
			if len(parts) == 2 {
				filterMap[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
			}
		}

		// Resolve sort
		sortField, sortDir := "title", "asc"
		if sv, ok := attrs["sort"]; ok {
			parts := strings.SplitN(sv, ":", 2)
			sortField = parts[0]
			if len(parts) == 2 {
				sortDir = parts[1]
			}
		}

		// Query
		items, err := s.QueryContentForDirective(ctx, filterMap, sortField, sortDir)
		if err != nil {
			firstErr = err
			return "<!-- lc:query error: " + err.Error() + " -->"
		}

		// Get snippet HTML
		snippetName := attrs["snippet"]
		snippetHTML := ""
		if snippetName != "" {
			var snip models.Snippet
			if err := s.db.FindOne(ctx, "snippets", bson.M{"name": snippetName}, &snip); err == nil {
				snippetHTML = snip.HTML
			}
		}

		// Render items
		var rendered strings.Builder
		for _, item := range items {
			if snippetHTML != "" {
				itemHTML, err := renderSnippet(snippetHTML, item)
				if err != nil {
					rendered.WriteString("<!-- snippet render error: " + err.Error() + " -->")
				} else {
					rendered.WriteString(itemHTML)
				}
			} else {
				// Default: simple link — escape to prevent XSS from titles with HTML characters
				rendered.WriteString(`<a href="` + template.HTMLEscapeString(item.FullPath) + `">` + template.HTMLEscapeString(item.Title) + `</a>`)
			}
		}
		return rendered.String()
	})
	return result, firstErr
}

// renderSnippet renders a snippet HTML template with the given content item.
func renderSnippet(snippetHTML string, item models.Content) (string, error) {
	t, err := template.New("snippet").Parse(snippetHTML)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, item); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// RegenerateIndexPages regenerates all published pages whose templates contain lc:query directives.
// Called after any content mutation so index pages stay in sync.
func (s *ContentService) RegenerateIndexPages(ctx context.Context) {
	// Find all templates with lc:query directives
	var templates []models.Template
	if err := s.db.FindAll(ctx, "templates", bson.M{}, &templates); err != nil {
		return
	}

	var indexTemplateIDs []primitive.ObjectID
	for _, tmpl := range templates {
		if strings.Contains(tmpl.HTMLLayout, "lc:query") {
			indexTemplateIDs = append(indexTemplateIDs, tmpl.ID)
		}
	}
	if len(indexTemplateIDs) == 0 {
		return
	}

	// Find all published pages using index templates
	var pages []models.Content
	if err := s.db.FindAll(ctx, "content", bson.M{
		"template_id": bson.M{"$in": indexTemplateIDs},
		"published":   true,
		"deleted":     bson.M{"$ne": true},
		"fork_id":     nil,
	}, &pages); err != nil {
		return
	}

	var purgedPaths []string
	for i := range pages {
		if err := s.GenerateStaticPage(ctx, &pages[i]); err != nil {
			fmt.Printf("Warning: failed to regenerate index page %s: %v\n", pages[i].FullPath, err)
		} else {
			purgedPaths = append(purgedPaths, pages[i].FullPath)
		}
	}
	if s.cfService != nil && len(purgedPaths) > 0 {
		go s.cfService.PurgeByURLs(context.Background(), purgedPaths)
	}
}

// RegenerateAllContent regenerates all published content.
// Clears all content hashes first so every page is regenerated (needed when
// theme/template/snippet changes affect rendered output globally).
func (s *ContentService) RegenerateAllContent(ctx context.Context) error {
	ctx = WithoutIndexNow(ctx) // site-wide re-render, not a content change
	// Clear all content hashes to force full regeneration
	s.db.Collection("content").UpdateMany(ctx,
		bson.M{"content_hash": bson.M{"$exists": true}},
		bson.M{"$unset": bson.M{"content_hash": ""}},
	)

	cursor, err := s.db.FindMany(ctx, "content",
		bson.M{"published": true, "deleted": bson.M{"$ne": true}, "fork_id": nil}, nil)
	if err != nil {
		return fmt.Errorf("failed to list content: %w", err)
	}

	var contents []models.Content
	if err := cursor.All(ctx, &contents); err != nil {
		return fmt.Errorf("failed to decode content: %w", err)
	}

	// Build the wikilink index once for all pages rather than once per page.
	wikilinkIdx := s.buildWikilinkIndex(ctx)

	// Parallel regeneration with a bounded worker pool.
	const maxWorkers = 6
	sem := make(chan struct{}, maxWorkers)
	var wg sync.WaitGroup

	for i := range contents {
		wg.Add(1)
		sem <- struct{}{}
		go func(c models.Content) {
			defer wg.Done()
			defer func() { <-sem }()
			if err := s.generateStaticPageWithWikilinkIndex(ctx, &c, wikilinkIdx); err != nil {
				fmt.Printf("Warning: failed to generate page %s: %v\n", c.FullPath, err)
			}
		}(contents[i])
	}
	wg.Wait()

	// Purge entire Cloudflare cache — every page was just regenerated.
	if s.cfService != nil {
		go s.cfService.PurgeEverything(context.Background())
	}

	return nil
}

func stringPtr(s string) *string { return &s }

func boolPtr(b bool) *bool { return &b }
