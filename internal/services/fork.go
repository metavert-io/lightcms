package services

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"sort"
	"time"

	"github.com/metavert-io/lightcms/v7/internal/database"
	"github.com/metavert-io/lightcms/v7/internal/models"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// ForkService manages content forks — isolated workspaces for staging site changes.
type ForkService struct {
	db             *database.DB
	contentService *ContentService
}

// NewForkService creates a ForkService.
func NewForkService(db *database.DB, cs *ContentService) *ForkService {
	return &ForkService{db: db, contentService: cs}
}

// MergeResult summarises what happened when a fork was merged into live.
type MergeResult struct {
	Updated    int
	Created    int
	CreatedIDs []primitive.ObjectID // live pages created by the merge
	UpdatedIDs []primitive.ObjectID // live pages updated by the merge
	Conflicts  []ForkConflict
	// NotPublished lists created pages that publishNew left as drafts
	// (held pages, or pages whose publish failed).
	NotPublished []MergeNotPublished
}

// MergeNotPublished records a page created by a merge that stayed a draft
// even though the merge was asked to publish new pages.
type MergeNotPublished struct {
	ID     primitive.ObjectID
	Path   string
	Reason string
}

// ForkCopyRef identifies one page copy belonging to a fork.
type ForkCopyRef struct {
	ID       primitive.ObjectID `bson:"_id" json:"id"`
	FullPath string             `bson:"full_path" json:"full_path"`
}

// ForkConflict records a page that was modified on the live site after the fork was made.
type ForkConflict struct {
	ForkItem  models.Content
	LiveTitle string
	LivePath  string
}

// generatePreviewToken returns a random 32-byte hex string.
func generatePreviewToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// Create creates a new fork workspace.
func (s *ForkService) Create(ctx context.Context, name, description string, userID primitive.ObjectID, userEmail string) (*models.ContentFork, error) {
	return s.CreateWithCredential(ctx, name, description, userID, userEmail, "")
}

// CreateWithCredential creates a fork and records the API key or OAuth
// connection that made it, which a sandboxed caller needs to write to it.
func (s *ForkService) CreateWithCredential(ctx context.Context, name, description string, userID primitive.ObjectID, userEmail, credential string) (*models.ContentFork, error) {
	token, err := generatePreviewToken()
	if err != nil {
		return nil, fmt.Errorf("generate preview token: %w", err)
	}
	now := time.Now()
	fork := &models.ContentFork{
		ID:             primitive.NewObjectID(),
		Name:           name,
		Description:    description,
		Status:         "active",
		PreviewToken:   token,
		CreatedBy:      userID,
		CreatedByEmail: userEmail,
		CreatedAt:      now,

		CreatedByCredential: credential,
	}
	if _, err := s.db.InsertOne(ctx, "content_forks", fork); err != nil {
		return nil, fmt.Errorf("insert fork: %w", err)
	}
	return fork, nil
}

// GetByID returns a fork by its ObjectID.
func (s *ForkService) GetByID(ctx context.Context, id primitive.ObjectID) (*models.ContentFork, error) {
	var fork models.ContentFork
	if err := s.db.FindOne(ctx, "content_forks", bson.M{"_id": id}, &fork); err != nil {
		return nil, err
	}
	return &fork, nil
}

// GetByPreviewToken returns an active fork by its preview token.
func (s *ForkService) GetByPreviewToken(ctx context.Context, token string) (*models.ContentFork, error) {
	if token == "" {
		return nil, fmt.Errorf("empty token")
	}
	var fork models.ContentFork
	if err := s.db.FindOne(ctx, "content_forks", bson.M{"preview_token": token, "status": "active"}, &fork); err != nil {
		return nil, err
	}
	return &fork, nil
}

// List returns all forks, newest first.
func (s *ForkService) List(ctx context.Context) ([]models.ContentFork, error) {
	opts := options.Find().SetSort(bson.D{{Key: "created_at", Value: -1}})
	cursor, err := s.db.Collection("content_forks").Find(ctx, bson.M{}, opts)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)
	var forks []models.ContentFork
	if err := cursor.All(ctx, &forks); err != nil {
		return nil, err
	}
	return forks, nil
}

// ForkPage copies an existing live content item into a fork workspace.
// If the page is already in this fork it returns the existing copy.
func (s *ForkService) ForkPage(ctx context.Context, forkID primitive.ObjectID, contentID primitive.ObjectID) (*models.Content, error) {
	// Check not already forked
	existing, err := s.GetForkPageByContentID(ctx, forkID, contentID)
	if err == nil && existing != nil {
		return existing, nil
	}

	// Load live content
	var live models.Content
	if err := s.db.FindOne(ctx, "content", bson.M{"_id": contentID, "deleted": bson.M{"$ne": true}, "fork_id": nil}, &live); err != nil {
		return nil, fmt.Errorf("content not found: %w", err)
	}

	// Create fork copy
	now := time.Now()
	copy := live
	copy.ID = primitive.NewObjectID()
	copy.ForkID = &forkID
	copy.BaseUpdatedAt = &live.UpdatedAt
	copy.Published = false // fork pages are never live-published
	copy.CreatedAt = now
	copy.UpdatedAt = now

	if _, err := s.db.InsertOne(ctx, "content", &copy); err != nil {
		return nil, fmt.Errorf("insert fork page: %w", err)
	}
	return &copy, nil
}

// GetForkPageByContentID finds the fork copy of a specific live content item.
func (s *ForkService) GetForkPageByContentID(ctx context.Context, forkID primitive.ObjectID, contentID primitive.ObjectID) (*models.Content, error) {
	// The fork copy was made from the live page — we match by full_path within the fork since we don't store the source ID.
	// Instead, look by _id which was freshly generated. Use full_path + fork_id.
	// Actually we need a reliable way — store the source content ID. Let's query by fork_id + original content_id via full_path.
	// Simpler: just check fork_id + matching original content (_id was re-generated, but full_path is the same).
	// We'll use fork_id + full_path by first fetching the live item's full_path.
	var live models.Content
	if err := s.db.FindOne(ctx, "content", bson.M{"_id": contentID}, &live); err != nil {
		return nil, err
	}
	return s.GetForkPageByPath(ctx, forkID, live.FullPath)
}

// GetForkPageByPath finds the fork version of a page by path.
func (s *ForkService) GetForkPageByPath(ctx context.Context, forkID primitive.ObjectID, fullPath string) (*models.Content, error) {
	var item models.Content
	if err := s.db.FindOne(ctx, "content", bson.M{"fork_id": forkID, "full_path": fullPath, "deleted": bson.M{"$ne": true}}, &item); err != nil {
		return nil, err
	}
	return &item, nil
}

// ListPages returns all content items belonging to a fork.
func (s *ForkService) ListPages(ctx context.Context, forkID primitive.ObjectID) ([]models.Content, error) {
	opts := options.Find().SetSort(bson.D{{Key: "full_path", Value: 1}})
	cursor, err := s.db.Collection("content").Find(ctx, bson.M{"fork_id": forkID, "deleted": bson.M{"$ne": true}}, opts)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)
	var pages []models.Content
	if err := cursor.All(ctx, &pages); err != nil {
		return nil, err
	}
	return pages, nil
}

// RemovePage removes a page from a fork (deletes the fork copy).
func (s *ForkService) RemovePage(ctx context.Context, forkID primitive.ObjectID, pageID primitive.ObjectID) error {
	filter := bson.M{"_id": pageID, "fork_id": forkID}
	result, err := s.db.Collection("content").DeleteOne(ctx, filter)
	if err != nil {
		return fmt.Errorf("delete fork page: %w", err)
	}
	if result.DeletedCount == 0 {
		return fmt.Errorf("page not found in fork")
	}
	return nil
}

// Merge merges all fork pages into live content, marks the fork as merged
// (recording the created/updated counts on it) and deletes the fork's page
// copies. Returns a MergeResult summarising what was created, updated, and
// any conflicts.
//
// New pages are created with the fork copy's published flag. With publishNew,
// new pages that would be drafts are then published through the normal
// publish path. Held pages always stay drafts.
//
// reviewedDigest is the Digest of the review the merge was approved from (see
// Review). The merge is refused with ErrForkChanged when the fork, or a live
// page it would change, differs from what that review showed.
//
// Every live page the merge creates or changes gets a version, recorded with
// the provenance and editor in ctx, so the merge shows in page history and an
// agent session's merge can be rolled back.
func (s *ForkService) Merge(ctx context.Context, forkID primitive.ObjectID, mergedByID primitive.ObjectID, mergedByEmail string, publishNew bool, reviewedDigest string) (*MergeResult, error) {
	fork, err := s.GetByID(ctx, forkID)
	if err != nil {
		return nil, fmt.Errorf("fork not found: %w", err)
	}
	// "merging" is let through: claimForMerge decides whether that claim is
	// live or left behind by a merge that never finished.
	if fork.Status != "active" && fork.Status != "merging" {
		return nil, fmt.Errorf("fork is %s, cannot merge", fork.Status)
	}
	if reviewedDigest == "" {
		return nil, ErrReviewRequired
	}

	// Claim the fork, so a second merge (an agent retrying, two people at
	// once) finds it busy and API writes to it are refused meanwhile. A claim
	// left behind by a crash expires.
	claimed, err := s.claimForMerge(ctx, forkID)
	if err != nil {
		return nil, fmt.Errorf("claim fork: %w", err)
	}
	if !claimed {
		return nil, fmt.Errorf("fork is already being merged")
	}
	merged := false
	defer func() {
		if !merged {
			s.releaseMergeClaim(ctx, forkID)
		}
	}()

	// The digest is taken from exactly the pages this merge applies.
	pages, err := s.ListPages(ctx, forkID)
	if err != nil {
		return nil, fmt.Errorf("list fork pages: %w", err)
	}
	review, err := s.reviewOf(ctx, pages)
	if err != nil {
		return nil, fmt.Errorf("review fork: %w", err)
	}
	if review.Digest != reviewedDigest {
		return nil, ErrForkChanged
	}

	result := &MergeResult{}
	comment := fmt.Sprintf("Merged fork %q", fork.Name)

	for _, forkPage := range pages {
		// Look for existing live page with the same full_path
		var livePage models.Content
		liveErr := s.db.FindOne(ctx, "content", bson.M{
			"full_path": forkPage.FullPath,
			"fork_id":   nil,
			"deleted":   bson.M{"$ne": true},
		}, &livePage)

		now := time.Now()

		if liveErr != nil {
			// No live page — create new
			newPage := forkPage
			newPage.ID = primitive.NewObjectID()
			newPage.ForkID = nil
			newPage.BaseUpdatedAt = nil
			newPage.CreatedAt = now
			newPage.UpdatedAt = now
			// A new page goes live only as the merge says (publish_new),
			// never later on a schedule set in the fork.
			newPage.PublishAt = nil
			if newPage.Hold {
				// A held page never goes live through a merge.
				newPage.Published = false
				newPage.PublishedAt = nil
			}
			if _, err := s.db.InsertOne(ctx, "content", &newPage); err != nil {
				return nil, fmt.Errorf("create live page %s: %w", forkPage.FullPath, err)
			}
			s.recordMergeVersion(ctx, &newPage, nil, comment)
			// Regenerate static file if published
			if newPage.Published && s.contentService != nil {
				_ = s.contentService.GenerateStaticPage(ctx, &newPage)
			}
			if publishNew && !newPage.Published {
				if reason := s.publishMerged(ctx, &newPage); reason != "" {
					result.NotPublished = append(result.NotPublished, MergeNotPublished{
						ID: newPage.ID, Path: newPage.FullPath, Reason: reason,
					})
				}
			}
			result.Created++
			result.CreatedIDs = append(result.CreatedIDs, newPage.ID)
		} else {
			// Live page exists — check for conflict
			if forkPage.BaseUpdatedAt != nil && livePage.UpdatedAt.After(*forkPage.BaseUpdatedAt) {
				result.Conflicts = append(result.Conflicts, ForkConflict{
					ForkItem:  forkPage,
					LiveTitle: livePage.Title,
					LivePath:  livePage.FullPath,
				})
				// Still merge (fork wins), just record the conflict
			}

			// Update live page fields from fork
			merged := livePage
			applyMergedFields(&merged, &forkPage)
			merged.UpdatedAt = now
			update := bson.M{"$set": bson.M{
				"title":            merged.Title,
				"slug":             merged.Slug,
				"folder_id":        merged.FolderID,
				"folder_path":      merged.FolderPath,
				"full_path":        merged.FullPath,
				"category":         merged.Category,
				"tags":             merged.Tags,
				"meta_description": merged.MetaDescription,
				"og_image":         merged.OGImage,
				"data":             merged.Data,
				"use_header":       merged.UseHeader,
				"use_footer":       merged.UseFooter,
				"use_theme":        merged.UseTheme,
				"raw_mode":         merged.RawMode,
				"noindex":          merged.NoIndex,
				"author_name":      merged.AuthorName,
				"author_url":       merged.AuthorURL,
				"template_id":      merged.TemplateID,
				"template_name":    merged.TemplateName,
				"updated_at":       now,
			}}
			if err := s.db.UpdateOne(ctx, "content", bson.M{"_id": livePage.ID}, update); err != nil {
				return nil, fmt.Errorf("update live page %s: %w", forkPage.FullPath, err)
			}
			s.recordMergeVersion(ctx, &merged, &livePage, comment)
			// Regenerate the static file from the whole merged page, so its
			// meta description, tags and author render as merged too.
			if merged.Published && s.contentService != nil {
				_ = s.contentService.GenerateStaticPage(ctx, &merged)
			}
			result.Updated++
			result.UpdatedIDs = append(result.UpdatedIDs, livePage.ID)
		}
	}

	// Mark fork as merged, keeping the page counts as its history.
	now := time.Now()
	if err := s.db.UpdateOne(ctx, "content_forks", bson.M{"_id": forkID}, bson.M{
		"$set": bson.M{
			"status":          "merged",
			"merged_at":       now,
			"merged_by":       mergedByID,
			"merged_by_email": mergedByEmail,
			"merged_created":  result.Created,
			"merged_updated":  result.Updated,
		},
		"$unset": bson.M{"merging_at": ""},
	}); err != nil {
		return nil, fmt.Errorf("mark fork merged: %w", err)
	}
	merged = true
	s.releaseSandboxes(ctx, forkID)

	// The copies have served their purpose: left behind they would only be
	// stale duplicates of live pages. A failure here does not undo the merge —
	// PurgeCopies removes whatever remains.
	if _, err := s.db.Collection("content").DeleteMany(ctx, bson.M{"fork_id": forkID}); err != nil {
		fmt.Printf("Warning: failed to delete page copies of merged fork %s: %v\n", forkID.Hex(), err)
	}

	return result, nil
}

// mergeClaimTTL is how long a merge's claim on a fork lasts if the merge never
// finishes (a crash part way).
const mergeClaimTTL = 10 * time.Minute

// claimForMerge moves an active fork to "merging", and reports whether this
// call did.
func (s *ForkService) claimForMerge(ctx context.Context, forkID primitive.ObjectID) (bool, error) {
	now := time.Now()
	result, err := s.db.Collection("content_forks").UpdateOne(ctx,
		bson.M{"_id": forkID, "$or": bson.A{
			bson.M{"status": "active"},
			bson.M{"status": "merging", "merging_at": bson.M{"$lt": now.Add(-mergeClaimTTL)}},
		}},
		bson.M{"$set": bson.M{"status": "merging", "merging_at": now}})
	if err != nil {
		return false, err
	}
	return result.ModifiedCount == 1, nil
}

// releaseMergeClaim returns a fork whose merge did not complete to "active".
func (s *ForkService) releaseMergeClaim(ctx context.Context, forkID primitive.ObjectID) {
	if err := s.db.UpdateOne(ctx, "content_forks",
		bson.M{"_id": forkID, "status": "merging"},
		bson.M{"$set": bson.M{"status": "active"}, "$unset": bson.M{"merging_at": ""}}); err != nil {
		log.Printf("Warning: failed to release the merge claim on fork %s: %v", forkID.Hex(), err)
	}
}

// ErrReviewRequired means a merge was attempted without naming the review it
// was approved from.
var ErrReviewRequired = errors.New("review the fork first: pass the digest from its diff")

// ErrForkChanged means the fork, or a live page it would change, was edited
// after the review the merge was approved from.
var ErrForkChanged = errors.New("the fork or a page it changes was edited after it was reviewed; review it again before merging")

// applyMergedFields copies onto live every field a merge takes from a fork page.
func applyMergedFields(live, fork *models.Content) {
	live.Title = fork.Title
	live.Slug = fork.Slug
	live.FolderID = fork.FolderID
	live.FolderPath = fork.FolderPath
	live.FullPath = fork.FullPath
	live.Category = fork.Category
	live.Tags = fork.Tags
	live.MetaDescription = fork.MetaDescription
	live.OGImage = fork.OGImage
	live.Data = fork.Data
	live.UseHeader = fork.UseHeader
	live.UseFooter = fork.UseFooter
	live.UseTheme = fork.UseTheme
	live.RawMode = fork.RawMode
	live.NoIndex = fork.NoIndex
	live.AuthorName = fork.AuthorName
	live.AuthorURL = fork.AuthorURL
	live.TemplateID = fork.TemplateID
	live.TemplateName = fork.TemplateName
}

// recordMergeVersion writes the version for a live page a merge created
// (original nil) or changed. A page with no history yet gets its pre-merge
// state as version 1 first, so the merge can be undone.
func (s *ForkService) recordMergeVersion(ctx context.Context, page, original *models.Content, comment string) {
	if s.contentService == nil {
		return
	}
	if err := s.contentService.saveVersion(ctx, page, original, comment); err != nil {
		log.Printf("Warning: failed to record the merge in %s's history: %v", page.FullPath, err)
	}
}

// publishMerged publishes a page that a merge just created, through the
// normal publish path so versioning, static generation, embeddings, IndexNow
// and webhooks behave as for any other publish. It returns "" on success, or
// the reason the page stayed a draft.
func (s *ForkService) publishMerged(ctx context.Context, page *models.Content) string {
	if page.Hold {
		return "on hold"
	}
	if s.contentService == nil {
		return "no content service"
	}
	if err := s.contentService.PublishContent(ctx, page.ID); err != nil {
		return err.Error()
	}
	return ""
}

// Archive marks a fork as archived without merging.
func (s *ForkService) Archive(ctx context.Context, forkID primitive.ObjectID) error {
	now := time.Now()
	if err := s.db.UpdateOne(ctx, "content_forks", bson.M{"_id": forkID, "status": "active"}, bson.M{"$set": bson.M{
		"status":      "archived",
		"archived_at": now,
	}}); err != nil {
		return err
	}
	s.releaseSandboxes(ctx, forkID)
	return nil
}

// Submit hands a fork off for review: from now on no API key or connection
// can change it.
func (s *ForkService) Submit(ctx context.Context, forkID primitive.ObjectID, byEmail string) error {
	return s.db.UpdateOne(ctx, "content_forks", bson.M{"_id": forkID, "status": "active"}, bson.M{"$set": bson.M{
		"submitted_at":       time.Now(),
		"submitted_by_email": byEmail,
	}})
}

// releaseSandboxes ends any API key's or connection's sandbox that writes to
// a fork which is no longer open for work, so the credential is not left
// confined to it. Failures are logged: GET /api/v1/sandbox clears a stale
// sandbox too.
func (s *ForkService) releaseSandboxes(ctx context.Context, forkID primitive.ObjectID) {
	for _, collection := range []string{"api_keys", "oauth_grants"} {
		if _, err := s.db.Collection(collection).UpdateMany(ctx,
			bson.M{"active_sandbox_fork": forkID},
			bson.M{"$unset": bson.M{"active_sandbox_fork": ""}}); err != nil {
			log.Printf("Warning: failed to end sandboxes on fork %s in %s: %v", forkID.Hex(), collection, err)
		}
	}
}

// Delete permanently deletes a fork and all its pages. For a merged fork this
// removes the history record and any page copies left over from before
// merges cleaned up after themselves; the merged live pages are not touched.
func (s *ForkService) Delete(ctx context.Context, forkID primitive.ObjectID) error {
	if _, err := s.GetByID(ctx, forkID); err != nil {
		return err
	}
	// Delete all fork pages
	if _, err := s.db.Collection("content").DeleteMany(ctx, bson.M{"fork_id": forkID}); err != nil {
		return fmt.Errorf("delete fork pages: %w", err)
	}
	// Delete fork record
	if _, err := s.db.Collection("content_forks").DeleteOne(ctx, bson.M{"_id": forkID}); err != nil {
		return fmt.Errorf("delete fork: %w", err)
	}
	s.releaseSandboxes(ctx, forkID)
	return nil
}

// PurgeCopies deletes the page copies still attached to a merged or archived
// fork (forks merged before v7.4 kept theirs). Active forks are refused:
// their copies are work in progress. With dryRun nothing is deleted. It
// returns the copies found (and, unless dryRun, deleted).
func (s *ForkService) PurgeCopies(ctx context.Context, forkID primitive.ObjectID, dryRun bool) ([]ForkCopyRef, error) {
	fork, err := s.GetByID(ctx, forkID)
	if err != nil {
		return nil, fmt.Errorf("fork not found: %w", err)
	}
	if fork.Status != "merged" && fork.Status != "archived" {
		return nil, fmt.Errorf("fork is %s; only merged or archived forks can be purged", fork.Status)
	}

	// Soft-deleted copies count too, so no "deleted" condition here.
	filter := bson.M{"fork_id": forkID}
	opts := options.Find().
		SetSort(bson.D{{Key: "full_path", Value: 1}}).
		SetProjection(bson.M{"full_path": 1})
	cursor, err := s.db.Collection("content").Find(ctx, filter, opts)
	if err != nil {
		return nil, fmt.Errorf("list fork copies: %w", err)
	}
	defer cursor.Close(ctx)
	copies := []ForkCopyRef{}
	if err := cursor.All(ctx, &copies); err != nil {
		return nil, fmt.Errorf("decode fork copies: %w", err)
	}
	if dryRun || len(copies) == 0 {
		return copies, nil
	}

	// Delete exactly the copies listed, so the result matches what was removed.
	ids := make([]primitive.ObjectID, len(copies))
	for i, c := range copies {
		ids[i] = c.ID
	}
	if _, err := s.db.Collection("content").DeleteMany(ctx, bson.M{"fork_id": forkID, "_id": bson.M{"$in": ids}}); err != nil {
		return nil, fmt.Errorf("delete fork copies: %w", err)
	}
	return copies, nil
}

// GetPageCount returns the number of pages in a fork.
func (s *ForkService) GetPageCount(ctx context.Context, forkID primitive.ObjectID) (int64, error) {
	return s.db.Count(ctx, "content", bson.M{"fork_id": forkID, "deleted": bson.M{"$ne": true}})
}

// FieldDiff describes one changed field between a live page and its fork
// copy. Live and Fork are complete values; shorten them for display only.
type FieldDiff struct {
	Name string `json:"name"`
	Live string `json:"live"`
	Fork string `json:"fork"`
}

// ForkPageDiff summarizes how one fork page differs from its live counterpart.
type ForkPageDiff struct {
	Path     string      `json:"path"`
	Title    string      `json:"title"`
	Status   string      `json:"status"` // "added" (no live counterpart) or "modified"
	Conflict bool        `json:"conflict"`
	Fields   []FieldDiff `json:"fields,omitempty"`
}

// ForkReview is what a person or agent reviews before merging: every change
// the merge would make, and a digest of the fork pages and live pages behind
// it. Merge takes the digest and refuses if anything has changed since.
type ForkReview struct {
	Pages  []ForkPageDiff `json:"pages"`
	Digest string         `json:"digest"`
}

// diffValue renders a field value for comparison and display, in full.
func diffValue(v interface{}) string {
	if v == nil {
		return ""
	}
	s, ok := v.(string)
	if !ok {
		b, _ := json.Marshal(v)
		s = string(b)
	}
	return s
}

// mergedFieldValues returns, in a fixed order, every field other than the
// template data that Merge copies from a fork page, rendered for comparison.
func mergedFieldValues(c *models.Content) [][2]string {
	folderID := ""
	if c.FolderID != nil {
		folderID = c.FolderID.Hex()
	}
	return [][2]string{
		{"title", c.Title},
		{"slug", c.Slug},
		{"folder_path", c.FolderPath},
		{"folder_id", folderID},
		{"full_path", c.FullPath},
		{"template", c.TemplateName},
		{"template_id", c.TemplateID.Hex()},
		{"category", c.Category},
		{"tags", diffValue(c.Tags)},
		{"meta_description", c.MetaDescription},
		{"og_image", c.OGImage},
		{"noindex", fmt.Sprint(c.NoIndex)},
		{"author_name", c.AuthorName},
		{"author_url", c.AuthorURL},
		{"use_header", fmt.Sprint(c.UseHeader)},
		{"use_footer", fmt.Sprint(c.UseFooter)},
		{"use_theme", fmt.Sprint(c.UseTheme)},
		{"raw_mode", fmt.Sprint(c.RawMode)},
	}
}

// pageFingerprint is the part of a page that decides what a merge does.
func pageFingerprint(c *models.Content) map[string]interface{} {
	fields := map[string]interface{}{
		"id": c.ID.Hex(), "data": c.Data, "hold": c.Hold,
		"published": c.Published, "publish_at": c.PublishAt,
	}
	for _, f := range mergedFieldValues(c) {
		fields[f[0]] = f[1]
	}
	return fields
}

// Diff compares every page in a fork against its live counterpart and
// returns per-field changes. Pages with no live counterpart are "added".
func (s *ForkService) Diff(ctx context.Context, forkID primitive.ObjectID) ([]ForkPageDiff, error) {
	review, err := s.Review(ctx, forkID)
	if err != nil {
		return nil, err
	}
	return review.Pages, nil
}

// Review compares every page in a fork with the live page a merge would
// change, over every field a merge writes, and returns the changes with the
// digest a merge must present. A new page lists every field it sets.
func (s *ForkService) Review(ctx context.Context, forkID primitive.ObjectID) (*ForkReview, error) {
	pages, err := s.ListPages(ctx, forkID)
	if err != nil {
		return nil, err
	}
	return s.reviewOf(ctx, pages)
}

// reviewOf reviews the given fork pages against the live site.
func (s *ForkService) reviewOf(ctx context.Context, pages []models.Content) (*ForkReview, error) {
	pages = append([]models.Content(nil), pages...)
	sort.Slice(pages, func(i, j int) bool {
		if pages[i].FullPath != pages[j].FullPath {
			return pages[i].FullPath < pages[j].FullPath
		}
		return pages[i].ID.Hex() < pages[j].ID.Hex()
	})

	review := &ForkReview{Pages: make([]ForkPageDiff, 0, len(pages))}
	fingerprints := make([]interface{}, 0, len(pages))
	for i := range pages {
		fp := &pages[i]
		d := ForkPageDiff{Path: fp.FullPath, Title: fp.Title}

		var live models.Content
		err := s.db.FindOne(ctx, "content", bson.M{
			"full_path": fp.FullPath,
			"deleted":   bson.M{"$ne": true},
			"fork_id":   nil,
		}, &live)
		var liveFields [][2]string
		var liveData map[string]interface{}
		if err != nil {
			d.Status = "added"
			fingerprints = append(fingerprints, map[string]interface{}{"fork": pageFingerprint(fp)})
		} else {
			d.Status = "modified"
			if fp.BaseUpdatedAt != nil && live.UpdatedAt.After(*fp.BaseUpdatedAt) {
				d.Conflict = true
			}
			liveFields, liveData = mergedFieldValues(&live), live.Data
			fingerprints = append(fingerprints, map[string]interface{}{"fork": pageFingerprint(fp), "live": pageFingerprint(&live)})
		}

		for j, f := range mergedFieldValues(fp) {
			if liveFields == nil {
				// A new page: list what it sets, leaving out empty values.
				if f[1] != "" && f[1] != "false" && f[1] != "null" {
					d.Fields = append(d.Fields, FieldDiff{Name: f[0], Fork: f[1]})
				}
				continue
			}
			if f[1] != liveFields[j][1] {
				d.Fields = append(d.Fields, FieldDiff{Name: f[0], Live: liveFields[j][1], Fork: f[1]})
			}
		}

		// Union of data field names from both sides, in sorted order.
		names := map[string]bool{}
		for k := range liveData {
			names[k] = true
		}
		for k := range fp.Data {
			names[k] = true
		}
		sorted := make([]string, 0, len(names))
		for k := range names {
			sorted = append(sorted, k)
		}
		sort.Strings(sorted)
		for _, k := range sorted {
			lv, fv := diffValue(liveData[k]), diffValue(fp.Data[k])
			if lv != fv {
				d.Fields = append(d.Fields, FieldDiff{Name: k, Live: lv, Fork: fv})
			}
		}

		review.Pages = append(review.Pages, d)
	}

	encoded, err := json.Marshal(fingerprints)
	if err != nil {
		return nil, fmt.Errorf("fingerprint fork: %w", err)
	}
	sum := sha256.Sum256(encoded)
	review.Digest = hex.EncodeToString(sum[:])
	return review, nil
}
