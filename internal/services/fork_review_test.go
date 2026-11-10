package services

import (
	"context"
	"errors"
	"strings"
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
	audit.Log(context.Background(), models.AuditLog{
		Action: "fork.merge", Resource: "fork", ResourceID: f.fork.ID.Hex(), AgentSession: session,
		Details: map[string]interface{}{
			"created_ids": []string{result.CreatedIDs[0].Hex()},
			"updated_ids": []string{result.UpdatedIDs[0].Hex()},
		},
	})

	sessions := NewAgentSessionService(audit, f.cs)
	changes, err := sessions.Changes(context.Background(), session)
	if err != nil || len(changes.ContentItems) != 2 {
		t.Fatalf("ledger: %+v, %v", changes, err)
	}
	rollback, err := sessions.Rollback(context.Background(), session)
	if err != nil {
		t.Fatalf("Rollback: %v", err)
	}
	if len(rollback.Reverted) != 1 || len(rollback.Deleted) != 1 {
		t.Errorf("rollback: %+v", rollback)
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
