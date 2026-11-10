package services

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/metavert-io/lightcms/v7/internal/database"
	"github.com/metavert-io/lightcms/v7/internal/models"
	"github.com/metavert-io/lightcms/v7/internal/testutil"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

type reviewFixture struct {
	db   *database.DB
	cs   *ContentService
	fs   *ForkService
	fork *models.ContentFork
	live primitive.ObjectID
	copy primitive.ObjectID
}

// newReviewFixture makes a live page with a long body, and a fork holding a
// copy of it.
func newReviewFixture(t *testing.T) *reviewFixture {
	t.Helper()
	db, cleanup := testutil.MustConnectTestDB(t)
	t.Cleanup(cleanup)
	ctx := context.Background()
	f := &reviewFixture{db: db, cs: NewContentService(db)}
	f.fs = NewForkService(db, f.cs)

	f.live = seedLiveContent(t, db, "Live", "/reviewed")
	if err := db.UpdateOne(ctx, "content", bson.M{"_id": f.live}, bson.M{"$set": bson.M{
		"data": bson.M{"body": strings.Repeat("a", 3000)},
		"tags": []string{"one"},
	}}); err != nil {
		t.Fatalf("seeding body: %v", err)
	}
	fork, err := f.fs.Create(ctx, "review", "", primitive.NewObjectID(), "agent@example.com")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	f.fork = fork
	page, err := f.fs.ForkPage(ctx, fork.ID, f.live)
	if err != nil {
		t.Fatalf("ForkPage: %v", err)
	}
	f.copy = page.ID
	return f
}

func (f *reviewFixture) editCopy(t *testing.T, set bson.M) {
	t.Helper()
	if err := f.db.UpdateOne(context.Background(), "content", bson.M{"_id": f.copy}, bson.M{"$set": set}); err != nil {
		t.Fatalf("editing the copy: %v", err)
	}
}

func (f *reviewFixture) review(t *testing.T) *ForkReview {
	t.Helper()
	review, err := f.fs.Review(context.Background(), f.fork.ID)
	if err != nil {
		t.Fatalf("Review: %v", err)
	}
	return review
}

func fieldNamed(page ForkPageDiff, name string) (FieldDiff, bool) {
	for _, f := range page.Fields {
		if f.Name == name {
			return f, true
		}
	}
	return FieldDiff{}, false
}

func TestReview_ComparesTheWholeValueOfEveryFieldAMergeWrites(t *testing.T) {
	f := newReviewFixture(t)

	// A change past the first 2,000 characters, which the old diff cut off
	// before comparing, and fields it never compared.
	body := strings.Repeat("a", 2500) + "INJECTED" + strings.Repeat("a", 492)
	f.editCopy(t, bson.M{
		"data.body": body, "tags": []string{"one", "two"}, "category": "news",
		"author_url": "https://elsewhere.example", "raw_mode": true, "use_theme": true,
	})

	review := f.review(t)
	if len(review.Pages) != 1 || review.Pages[0].Status != "modified" {
		t.Fatalf("pages: %+v", review.Pages)
	}
	page := review.Pages[0]
	got, ok := fieldNamed(page, "body")
	if !ok {
		t.Fatal("a change after 2,000 characters is not in the diff")
	}
	if got.Fork != body || len(got.Live) != 3000 {
		t.Errorf("values are not complete: live %d chars, fork %d chars", len(got.Live), len(got.Fork))
	}
	for _, name := range []string{"tags", "category", "author_url", "raw_mode", "use_theme"} {
		if _, ok := fieldNamed(page, name); !ok {
			t.Errorf("%s is not compared", name)
		}
	}
	if _, ok := fieldNamed(page, "title"); ok {
		t.Error("an unchanged field is listed")
	}
}

func TestReview_ANewPageListsWhatItSets(t *testing.T) {
	f := newReviewFixture(t)
	ctx := context.Background()
	newPage := &models.Content{
		ID: primitive.NewObjectID(), Title: "Brand new", Slug: "brand-new", FullPath: "/brand-new",
		MetaDescription: "About it", Data: map[string]interface{}{"body": "<p>Hello</p>"},
		ForkID: &f.fork.ID, CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	if _, err := f.db.InsertOne(ctx, "content", newPage); err != nil {
		t.Fatalf("insert: %v", err)
	}

	var added *ForkPageDiff
	for i, p := range f.review(t).Pages {
		if p.Path == "/brand-new" {
			added = &f.review(t).Pages[i]
		}
	}
	if added == nil || added.Status != "added" {
		t.Fatalf("new page: %+v", added)
	}
	for name, want := range map[string]string{"title": "Brand new", "meta_description": "About it", "body": "<p>Hello</p>"} {
		got, ok := fieldNamed(*added, name)
		if !ok || got.Fork != want || got.Live != "" {
			t.Errorf("%s: %+v", name, got)
		}
	}
	if _, ok := fieldNamed(*added, "raw_mode"); ok {
		t.Error("a new page lists fields it leaves empty")
	}
}

func TestMerge_AppliesOnlyWhatWasReviewed(t *testing.T) {
	ctx := context.Background()
	uid := primitive.NewObjectID()

	t.Run("no review", func(t *testing.T) {
		f := newReviewFixture(t)
		if _, err := f.fs.Merge(ctx, f.fork.ID, uid, "m@example.com", false, ""); !errors.Is(err, ErrReviewRequired) {
			t.Errorf("got %v", err)
		}
	})

	t.Run("the fork changed after review", func(t *testing.T) {
		f := newReviewFixture(t)
		f.editCopy(t, bson.M{"title": "Reviewed title"})
		digest := f.review(t).Digest
		f.editCopy(t, bson.M{"data.body": "swapped after review"})
		if _, err := f.fs.Merge(ctx, f.fork.ID, uid, "m@example.com", false, digest); !errors.Is(err, ErrForkChanged) {
			t.Fatalf("got %v", err)
		}
		var live models.Content
		f.db.FindOne(ctx, "content", bson.M{"_id": f.live}, &live)
		if live.Title != "Live" {
			t.Error("a refused merge changed the live page")
		}
	})

	t.Run("a page added after review", func(t *testing.T) {
		f := newReviewFixture(t)
		digest := f.review(t).Digest
		extra := &models.Content{ID: primitive.NewObjectID(), Title: "Late", Slug: "late", FullPath: "/late", ForkID: &f.fork.ID, CreatedAt: time.Now(), UpdatedAt: time.Now()}
		if _, err := f.db.InsertOne(ctx, "content", extra); err != nil {
			t.Fatalf("insert: %v", err)
		}
		if _, err := f.fs.Merge(ctx, f.fork.ID, uid, "m@example.com", false, digest); !errors.Is(err, ErrForkChanged) {
			t.Errorf("got %v", err)
		}
	})

	t.Run("the live page changed after review", func(t *testing.T) {
		f := newReviewFixture(t)
		f.editCopy(t, bson.M{"title": "Reviewed title"})
		digest := f.review(t).Digest
		if err := f.db.UpdateOne(ctx, "content", bson.M{"_id": f.live}, bson.M{"$set": bson.M{"meta_description": "edited live"}}); err != nil {
			t.Fatalf("editing live: %v", err)
		}
		if _, err := f.fs.Merge(ctx, f.fork.ID, uid, "m@example.com", false, digest); !errors.Is(err, ErrForkChanged) {
			t.Errorf("got %v", err)
		}
	})

	t.Run("unchanged since review", func(t *testing.T) {
		f := newReviewFixture(t)
		f.editCopy(t, bson.M{"title": "Reviewed title"})
		digest := f.review(t).Digest
		if digest != f.review(t).Digest {
			t.Fatal("reviewing twice gave different digests")
		}
		if _, err := f.fs.Merge(ctx, f.fork.ID, uid, "m@example.com", false, digest); err != nil {
			t.Fatalf("Merge: %v", err)
		}
		var live models.Content
		f.db.FindOne(ctx, "content", bson.M{"_id": f.live}, &live)
		if live.Title != "Reviewed title" {
			t.Errorf("live title %q", live.Title)
		}
	})
}

func TestMerge_RecordsAVersionForEveryPageItChanges(t *testing.T) {
	f := newReviewFixture(t)
	f.editCopy(t, bson.M{"title": "Merged title", "meta_description": "merged meta"})
	newPage := &models.Content{ID: primitive.NewObjectID(), Title: "New", Slug: "new-one", FullPath: "/new-one", ForkID: &f.fork.ID, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	if _, err := f.db.InsertOne(context.Background(), "content", newPage); err != nil {
		t.Fatalf("insert: %v", err)
	}

	ctx := WithEditorEmail(context.Background(), "agent-owner@example.com")
	ctx = WithProvenance(ctx, Provenance{Actor: "agent", Via: "api", AgentSession: "agent-merge-1"})
	result, err := f.fs.Merge(ctx, f.fork.ID, primitive.NewObjectID(), "agent-owner@example.com", false, f.review(t).Digest)
	if err != nil {
		t.Fatalf("Merge: %v", err)
	}

	versions, err := f.cs.GetVersions(context.Background(), f.live)
	if err != nil || len(versions) != 2 {
		t.Fatalf("live page versions: %d, %v", len(versions), err)
	}
	merged, before := versions[0], versions[1] // newest first
	if merged.Title != "Merged title" || merged.AgentSession != "agent-merge-1" || merged.Actor != "agent" || merged.ModifiedByEmail != "agent-owner@example.com" || !strings.Contains(merged.Comment, "review") {
		t.Errorf("merge version: %+v", merged)
	}
	if before.Title != "Live" || before.AgentSession != "" {
		t.Errorf("pre-merge version: %+v", before)
	}

	if len(result.CreatedIDs) != 1 {
		t.Fatalf("created: %v", result.CreatedIDs)
	}
	created, err := f.cs.GetVersions(context.Background(), result.CreatedIDs[0])
	if err != nil || len(created) != 1 || created[0].AgentSession != "agent-merge-1" {
		t.Errorf("new page versions: %+v, %v", created, err)
	}
}

func TestRollback_UndoesASessionsMerge(t *testing.T) {
	f := newReviewFixture(t)
	f.editCopy(t, bson.M{"title": "Merged title"})
	newPage := &models.Content{ID: primitive.NewObjectID(), Title: "New", Slug: "rolled-new", FullPath: "/rolled-new", ForkID: &f.fork.ID, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	if _, err := f.db.InsertOne(context.Background(), "content", newPage); err != nil {
		t.Fatalf("insert: %v", err)
	}

	const session = "agent-rollback-merge"
	ctx := WithProvenance(context.Background(), Provenance{Actor: "agent", Via: "api", AgentSession: session})
	result, err := f.fs.Merge(ctx, f.fork.ID, primitive.NewObjectID(), "a@example.com", false, f.review(t).Digest)
	if err != nil {
		t.Fatalf("Merge: %v", err)
	}
	audit := NewAuditService(f.db)
	// The session also created the page inside the fork; merging removed
	// that copy, so rollback reports it as gone rather than failing on it.
	audit.Log(context.Background(), models.AuditLog{
		Action: "content.create", Resource: "content", ResourceID: newPage.ID.Hex(), AgentSession: session,
		CreatedAt: time.Now().Add(-time.Minute),
	})
	audit.Log(context.Background(), models.AuditLog{
		Action: "fork.merge", Resource: "fork", ResourceID: f.fork.ID.Hex(), AgentSession: session,
		Details: map[string]interface{}{
			"created_ids": []string{result.CreatedIDs[0].Hex()},
			"updated_ids": []string{result.UpdatedIDs[0].Hex()},
		},
	})

	sessions := NewAgentSessionService(audit, f.cs)
	changes, err := sessions.Changes(context.Background(), session)
	if err != nil || len(changes.ContentItems) != 3 {
		t.Fatalf("ledger: %+v, %v", changes, err)
	}
	rollback, err := sessions.Rollback(context.Background(), session)
	if err != nil {
		t.Fatalf("Rollback: %v", err)
	}
	if len(rollback.Reverted) != 1 || len(rollback.Deleted) != 1 {
		t.Errorf("rollback: %+v", rollback)
	}
	if len(rollback.Skipped) != 1 || rollback.Skipped[0].ContentID != newPage.ID.Hex() || !strings.Contains(rollback.Skipped[0].Reason, "no longer exists") {
		t.Errorf("the merged-away copy: %+v", rollback.Skipped)
	}

	var live, created models.Content
	f.db.FindOne(context.Background(), "content", bson.M{"_id": f.live}, &live)
	f.db.FindOne(context.Background(), "content", bson.M{"_id": result.CreatedIDs[0]}, &created)
	if live.Title != "Live" {
		t.Errorf("the merged page was not reverted: %q", live.Title)
	}
	if !created.Deleted {
		t.Error("the page the merge created was not removed")
	}
}

func TestSubmit_RecordsTheHandOff(t *testing.T) {
	f := newReviewFixture(t)
	if err := f.fs.Submit(context.Background(), f.fork.ID, "agent@example.com"); err != nil {
		t.Fatalf("Submit: %v", err)
	}
	fork, _ := f.fs.GetByID(context.Background(), f.fork.ID)
	if fork.SubmittedAt == nil || fork.SubmittedByEmail != "agent@example.com" || fork.Status != "active" {
		t.Errorf("after submit: %+v", fork)
	}
}

func TestMerge_TwoMergesOfOneForkApplyOnce(t *testing.T) {
	f := newReviewFixture(t)
	f.editCopy(t, bson.M{"title": "Merged once"})
	digest := f.review(t).Digest

	var wg sync.WaitGroup
	var mu sync.Mutex
	succeeded := 0
	for i := 0; i < 6; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := f.fs.Merge(context.Background(), f.fork.ID, primitive.NewObjectID(), "m@example.com", false, digest); err == nil {
				mu.Lock()
				succeeded++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if succeeded != 1 {
		t.Errorf("%d merges applied, want 1", succeeded)
	}
	versions, _ := f.cs.GetVersions(context.Background(), f.live)
	if len(versions) != 2 {
		t.Errorf("%d versions after one merge, want 2", len(versions))
	}
}

func TestMerge_AStaleClaimFromACrashedMergeExpires(t *testing.T) {
	f := newReviewFixture(t)
	ctx := context.Background()
	f.editCopy(t, bson.M{"title": "After the crash"})
	digest := f.review(t).Digest

	// A merge that is still running holds the fork.
	if err := f.db.UpdateOne(ctx, "content_forks", bson.M{"_id": f.fork.ID}, bson.M{"$set": bson.M{"status": "merging", "merging_at": time.Now()}}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.fs.Merge(ctx, f.fork.ID, primitive.NewObjectID(), "m@example.com", false, digest); err == nil {
		t.Fatal("merged a fork another merge holds")
	}
	// One that crashed long ago doesn't.
	if err := f.db.UpdateOne(ctx, "content_forks", bson.M{"_id": f.fork.ID}, bson.M{"$set": bson.M{"merging_at": time.Now().Add(-time.Hour)}}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.fs.Merge(ctx, f.fork.ID, primitive.NewObjectID(), "m@example.com", false, digest); err != nil {
		t.Errorf("merging after a stale claim: %v", err)
	}
}

func TestMerge_RefusedMergeReleasesTheFork(t *testing.T) {
	f := newReviewFixture(t)
	ctx := context.Background()
	if _, err := f.fs.Merge(ctx, f.fork.ID, primitive.NewObjectID(), "m@example.com", false, "stale"); !errors.Is(err, ErrForkChanged) {
		t.Fatalf("got %v", err)
	}
	fork, _ := f.fs.GetByID(ctx, f.fork.ID)
	if fork.Status != "active" {
		t.Errorf("after a refused merge the fork is %q", fork.Status)
	}
}

func TestMerge_ANewPageGoesLiveOnlyAsTheMergeSays(t *testing.T) {
	f := newReviewFixture(t)
	ctx := context.Background()
	soon := time.Now().Add(time.Hour)
	scheduled := &models.Content{ID: primitive.NewObjectID(), Title: "Scheduled in fork", Slug: "sched", FullPath: "/sched", ForkID: &f.fork.ID, PublishAt: &soon, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	if _, err := f.db.InsertOne(ctx, "content", scheduled); err != nil {
		t.Fatal(err)
	}
	result, err := f.fs.Merge(ctx, f.fork.ID, primitive.NewObjectID(), "m@example.com", false, f.review(t).Digest)
	if err != nil {
		t.Fatalf("Merge: %v", err)
	}
	for _, id := range result.CreatedIDs {
		var page models.Content
		f.db.FindOne(ctx, "content", bson.M{"_id": id}, &page)
		if page.PublishAt != nil {
			t.Errorf("%s kept a schedule from the fork", page.FullPath)
		}
	}
}

func TestRevertToVersion_RestoresNoindexAndAuthor(t *testing.T) {
	f := newReviewFixture(t)
	ctx := context.Background()
	f.editCopy(t, bson.M{"noindex": true, "author_name": "Agent", "author_url": "https://agent.example"})
	if _, err := f.fs.Merge(ctx, f.fork.ID, primitive.NewObjectID(), "m@example.com", false, f.review(t).Digest); err != nil {
		t.Fatalf("Merge: %v", err)
	}
	if err := f.cs.RevertToVersion(ctx, f.live, 1, "undo"); err != nil {
		t.Fatalf("RevertToVersion: %v", err)
	}
	var live models.Content
	f.db.FindOne(ctx, "content", bson.M{"_id": f.live}, &live)
	if live.NoIndex || live.AuthorName != "" || live.AuthorURL != "" {
		t.Errorf("after reverting the merge: noindex=%v author=%q url=%q", live.NoIndex, live.AuthorName, live.AuthorURL)
	}

	// A version from before 7.5 has none of these and leaves them alone.
	if err := f.db.UpdateOne(ctx, "content", bson.M{"_id": f.live}, bson.M{"$set": bson.M{"author_name": "Kept"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.Collection("content_versions").UpdateOne(ctx, bson.M{"content_id": f.live, "version": 1}, bson.M{"$unset": bson.M{"noindex": "", "author_name": "", "author_url": ""}}); err != nil {
		t.Fatal(err)
	}
	if err := f.cs.RevertToVersion(ctx, f.live, 1, "undo again"); err != nil {
		t.Fatalf("RevertToVersion: %v", err)
	}
	f.db.FindOne(ctx, "content", bson.M{"_id": f.live}, &live)
	if live.AuthorName != "Kept" {
		t.Errorf("an old version cleared the author: %q", live.AuthorName)
	}
}
