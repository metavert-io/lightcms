package services

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/metavert-io/lightcms/v7/internal/models"
	"github.com/metavert-io/lightcms/v7/internal/testutil"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

func TestForkService_Merge_CreateUpdateConflict(t *testing.T) {
	db, cleanup := testutil.MustConnectTestDB(t)
	defer cleanup()

	cs := NewContentService(db)
	fs := NewForkService(db, cs)
	ctx := context.Background()
	uid := primitive.NewObjectID()

	liveID := seedLiveContent(t, db, "Live Original", "/merge-live")

	fork, err := fs.Create(ctx, "merge-fork", "", uid, "ed@x.com")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Fork the live page and edit the copy.
	copyPage, err := fs.ForkPage(ctx, fork.ID, liveID)
	if err != nil {
		t.Fatalf("ForkPage: %v", err)
	}
	if err := db.UpdateOne(ctx, "content", bson.M{"_id": copyPage.ID},
		bson.M{"$set": bson.M{"title": "Fork Edit", "updated_at": time.Now()}}); err != nil {
		t.Fatalf("edit fork copy: %v", err)
	}

	// Simulate a live edit after the fork point → conflict on merge.
	if err := db.UpdateOne(ctx, "content", bson.M{"_id": liveID},
		bson.M{"$set": bson.M{"title": "Live Edited Meanwhile", "updated_at": time.Now().Add(time.Minute)}}); err != nil {
		t.Fatalf("edit live: %v", err)
	}

	// A page that exists only in the fork → created on merge. Published so the
	// static-generation branch runs too.
	forkOnly := &models.Content{
		ID: primitive.NewObjectID(), Title: "Brand New", Slug: "merge-new",
		FullPath: "/merge-new", Published: true, ForkID: &fork.ID,
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	if _, err := db.InsertOne(ctx, "content", forkOnly); err != nil {
		t.Fatalf("insert fork-only page: %v", err)
	}

	result, err := fs.Merge(ctx, fork.ID, uid, "merger@x.com", false, reviewDigest(t, fs, fork.ID))
	if err != nil {
		t.Fatalf("Merge: %v", err)
	}
	if result.Created != 1 || result.Updated != 1 {
		t.Errorf("created=%d updated=%d, want 1/1", result.Created, result.Updated)
	}
	if len(result.Conflicts) != 1 {
		t.Errorf("conflicts=%d, want 1", len(result.Conflicts))
	} else if result.Conflicts[0].LiveTitle != "Live Edited Meanwhile" {
		t.Errorf("conflict live title = %q", result.Conflicts[0].LiveTitle)
	}

	// Fork wins: live page updated with fork title.
	var live models.Content
	if err := db.FindOne(ctx, "content", bson.M{"_id": liveID}, &live); err != nil {
		t.Fatalf("reload live: %v", err)
	}
	if live.Title != "Fork Edit" {
		t.Errorf("live title after merge = %q, want Fork Edit", live.Title)
	}

	// The fork-only page now exists as a live page.
	var created models.Content
	if err := db.FindOne(ctx, "content", bson.M{
		"full_path": "/merge-new", "fork_id": nil,
	}, &created); err != nil {
		t.Fatalf("created live page not found: %v", err)
	}

	// Fork is marked merged.
	merged, err := fs.GetByID(ctx, fork.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if merged.Status != "merged" {
		t.Errorf("fork status = %q, want merged", merged.Status)
	}

	// Merging a non-active fork fails.
	if _, err := fs.Merge(ctx, fork.ID, uid, "merger@x.com", false, reviewDigest(t, fs, fork.ID)); err == nil {
		t.Error("expected error merging an already-merged fork")
	}

	// The result names the live pages it touched.
	if len(result.CreatedIDs) != 1 || result.CreatedIDs[0] != created.ID {
		t.Errorf("CreatedIDs = %v, want [%s]", result.CreatedIDs, created.ID.Hex())
	}
	if len(result.UpdatedIDs) != 1 || result.UpdatedIDs[0] != liveID {
		t.Errorf("UpdatedIDs = %v, want [%s]", result.UpdatedIDs, liveID.Hex())
	}

	// The merge cleaned up: no page copies remain, and the counts are kept
	// on the fork record as its history.
	if n, _ := db.Count(ctx, "content", bson.M{"fork_id": fork.ID}); n != 0 {
		t.Errorf("fork copies after merge = %d, want 0", n)
	}
	if merged.MergedCreated != 1 || merged.MergedUpdated != 1 {
		t.Errorf("stored counts created=%d updated=%d, want 1/1", merged.MergedCreated, merged.MergedUpdated)
	}
	// With no copies left, the diff of a merged fork is simply empty.
	if diffs, err := fs.Diff(ctx, fork.ID); err != nil || len(diffs) != 0 {
		t.Errorf("Diff of merged fork = %v, %v; want empty, nil", diffs, err)
	}

	// A merged fork can be deleted (7.4.0; before, this was refused). Only
	// the record goes — the merged live pages stay.
	if err := fs.Delete(ctx, fork.ID); err != nil {
		t.Errorf("Delete merged fork: %v", err)
	}
	if _, err := fs.GetByID(ctx, fork.ID); err == nil {
		t.Error("fork record still present after Delete")
	}
	if err := db.FindOne(ctx, "content", bson.M{"_id": liveID}, &live); err != nil {
		t.Errorf("live page gone after deleting the merged fork: %v", err)
	}
}

// insertForkOnly adds a page that exists only in the fork (created on merge).
func insertForkOnly(t *testing.T, fs *ForkService, forkID primitive.ObjectID, path string, hold bool) {
	t.Helper()
	page := &models.Content{
		ID: primitive.NewObjectID(), Title: "New " + path, Slug: path[1:],
		FullPath: path, ForkID: &forkID, Hold: hold,
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	if _, err := fs.db.InsertOne(context.Background(), "content", page); err != nil {
		t.Fatalf("insert fork-only page %s: %v", path, err)
	}
}

// livePageAt returns the live (non-fork) page at path.
func livePageAt(t *testing.T, fs *ForkService, path string) models.Content {
	t.Helper()
	var c models.Content
	if err := fs.db.FindOne(context.Background(), "content", bson.M{"full_path": path, "fork_id": nil}, &c); err != nil {
		t.Fatalf("live page %s not found: %v", path, err)
	}
	return c
}

func TestForkService_Merge_PublishNew(t *testing.T) {
	db, cleanup := testutil.MustConnectTestDB(t)
	defer cleanup()

	fs := NewForkService(db, NewContentService(db))
	ctx := context.Background()
	uid := primitive.NewObjectID()

	// Default (publishNew=false): a new draft stays a draft.
	fork, err := fs.Create(ctx, "pn-off", "", uid, "ed@x.com")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	insertForkOnly(t, fs, fork.ID, "/pn-off-new", false)
	res, err := fs.Merge(ctx, fork.ID, uid, "m@x.com", false, reviewDigest(t, fs, fork.ID))
	if err != nil {
		t.Fatalf("Merge: %v", err)
	}
	if got := livePageAt(t, fs, "/pn-off-new"); got.Published {
		t.Error("publishNew=false: new page was published")
	}
	if len(res.NotPublished) != 0 {
		t.Errorf("publishNew=false: NotPublished = %v, want empty", res.NotPublished)
	}

	// publishNew=true: the new page is published, the held one stays a draft.
	fork2, err := fs.Create(ctx, "pn-on", "", uid, "ed@x.com")
	if err != nil {
		t.Fatalf("Create 2: %v", err)
	}
	insertForkOnly(t, fs, fork2.ID, "/pn-on-new", false)
	insertForkOnly(t, fs, fork2.ID, "/pn-on-held", true)
	res, err = fs.Merge(ctx, fork2.ID, uid, "m@x.com", true, reviewDigest(t, fs, fork2.ID))
	if err != nil {
		t.Fatalf("Merge 2: %v", err)
	}
	if res.Created != 2 || len(res.CreatedIDs) != 2 {
		t.Errorf("created=%d ids=%d, want 2/2", res.Created, len(res.CreatedIDs))
	}
	pub := livePageAt(t, fs, "/pn-on-new")
	if !pub.Published || pub.PublishedAt == nil {
		t.Errorf("publishNew=true: new page published=%v published_at=%v", pub.Published, pub.PublishedAt)
	}
	held := livePageAt(t, fs, "/pn-on-held")
	if held.Published {
		t.Error("publishNew=true: held page was published")
	}
	if !held.Hold {
		t.Error("held page lost its hold flag in the merge")
	}
	if len(res.NotPublished) != 1 || res.NotPublished[0].ID != held.ID {
		t.Errorf("NotPublished = %+v, want just the held page", res.NotPublished)
	}

	// A held fork page marked published is still created as a draft.
	fork3, err := fs.Create(ctx, "pn-held-pub", "", uid, "ed@x.com")
	if err != nil {
		t.Fatalf("Create 3: %v", err)
	}
	if _, err := db.InsertOne(ctx, "content", &models.Content{
		ID: primitive.NewObjectID(), Title: "Held Pub", Slug: "pn-held-pub", FullPath: "/pn-held-pub",
		ForkID: &fork3.ID, Hold: true, Published: true, CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}); err != nil {
		t.Fatalf("insert: %v", err)
	}
	if _, err := fs.Merge(ctx, fork3.ID, uid, "m@x.com", false, reviewDigest(t, fs, fork3.ID)); err != nil {
		t.Fatalf("Merge 3: %v", err)
	}
	if got := livePageAt(t, fs, "/pn-held-pub"); got.Published {
		t.Error("held fork page marked published went live on merge")
	}
}

func TestForkService_PurgeCopies(t *testing.T) {
	db, cleanup := testutil.MustConnectTestDB(t)
	defer cleanup()

	fs := NewForkService(db, NewContentService(db))
	ctx := context.Background()
	uid := primitive.NewObjectID()

	// Unknown fork.
	if _, err := fs.PurgeCopies(ctx, primitive.NewObjectID(), true); err == nil {
		t.Error("expected error for unknown fork")
	}

	fork, err := fs.Create(ctx, "purge", "", uid, "ed@x.com")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	liveID := seedLiveContent(t, db, "Purge Live", "/purge-live")
	insertForkOnly(t, fs, fork.ID, "/purge-a", false)
	insertForkOnly(t, fs, fork.ID, "/purge-b", false)

	// Active forks are refused, dry run or not.
	if _, err := fs.PurgeCopies(ctx, fork.ID, true); err == nil {
		t.Error("expected dry-run purge of an active fork to be refused")
	}
	if _, err := fs.PurgeCopies(ctx, fork.ID, false); err == nil {
		t.Error("expected purge of an active fork to be refused")
	}
	if n, _ := db.Count(ctx, "content", bson.M{"fork_id": fork.ID}); n != 2 {
		t.Fatalf("copies after refused purge = %d, want 2", n)
	}

	// Simulate a fork merged before 7.4: status merged, copies left behind.
	if err := db.UpdateOne(ctx, "content_forks", bson.M{"_id": fork.ID}, bson.M{"$set": bson.M{"status": "merged"}}); err != nil {
		t.Fatalf("mark merged: %v", err)
	}

	// Dry run lists without deleting.
	copies, err := fs.PurgeCopies(ctx, fork.ID, true)
	if err != nil {
		t.Fatalf("dry run: %v", err)
	}
	if len(copies) != 2 || copies[0].FullPath != "/purge-a" || copies[1].FullPath != "/purge-b" {
		t.Errorf("dry run copies = %+v, want /purge-a and /purge-b", copies)
	}
	if n, _ := db.Count(ctx, "content", bson.M{"fork_id": fork.ID}); n != 2 {
		t.Errorf("dry run deleted copies: %d left, want 2", n)
	}

	// Real purge deletes the copies, keeps the fork record and live pages.
	copies, err = fs.PurgeCopies(ctx, fork.ID, false)
	if err != nil {
		t.Fatalf("purge: %v", err)
	}
	if len(copies) != 2 {
		t.Errorf("purged = %d, want 2", len(copies))
	}
	if n, _ := db.Count(ctx, "content", bson.M{"fork_id": fork.ID}); n != 0 {
		t.Errorf("copies after purge = %d, want 0", n)
	}
	if f, err := fs.GetByID(ctx, fork.ID); err != nil || f.Status != "merged" {
		t.Errorf("fork record after purge: %+v, %v", f, err)
	}
	if n, _ := db.Count(ctx, "content", bson.M{"_id": liveID}); n != 1 {
		t.Error("purge deleted a live page")
	}

	// Purging again is a no-op.
	if copies, err = fs.PurgeCopies(ctx, fork.ID, false); err != nil || len(copies) != 0 {
		t.Errorf("second purge = %v, %v; want empty, nil", copies, err)
	}

	// Archived forks may be purged too.
	arch, err := fs.Create(ctx, "purge-arch", "", uid, "ed@x.com")
	if err != nil {
		t.Fatalf("Create arch: %v", err)
	}
	insertForkOnly(t, fs, arch.ID, "/purge-arch", false)
	if err := fs.Archive(ctx, arch.ID); err != nil {
		t.Fatalf("Archive: %v", err)
	}
	if copies, err = fs.PurgeCopies(ctx, arch.ID, false); err != nil || len(copies) != 1 {
		t.Errorf("purge archived = %v, %v; want 1 copy", copies, err)
	}
}

// seedDraft inserts a live draft page and returns it.
func seedDraft(t *testing.T, cs *ContentService, path string, hold bool) *models.Content {
	t.Helper()
	c := &models.Content{Title: "Draft " + path, Slug: path[1:], Hold: hold}
	if err := cs.CreateContent(context.Background(), c); err != nil {
		t.Fatalf("seedDraft %s: %v", path, err)
	}
	return c
}

func TestContentService_PublishGuards(t *testing.T) {
	db, cleanup := testutil.MustConnectTestDB(t)
	defer cleanup()

	cs := NewContentService(db)
	fs := NewForkService(db, cs)
	ctx := context.Background()

	// Fork copies cannot be published directly.
	liveID := seedLiveContent(t, db, "Guard Live", "/guard-live")
	fork, err := fs.Create(ctx, "guard", "", primitive.NewObjectID(), "ed@x.com")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	copyPage, err := fs.ForkPage(ctx, fork.ID, liveID)
	if err != nil {
		t.Fatalf("ForkPage: %v", err)
	}
	if err := cs.PublishContent(ctx, copyPage.ID); !errors.Is(err, ErrPublishForkCopy) {
		t.Errorf("publish fork copy: err = %v, want ErrPublishForkCopy", err)
	}
	if c, _ := cs.GetContent(ctx, copyPage.ID); c == nil || c.Published {
		t.Error("fork copy was published despite the guard")
	}

	// Held drafts cannot be published; clearing the hold allows it.
	held := seedDraft(t, cs, "/guard-held", true)
	if err := cs.PublishContent(ctx, held.ID); !errors.Is(err, ErrContentHeld) {
		t.Errorf("publish held: err = %v, want ErrContentHeld", err)
	}
	// ...nor through an update that flips published.
	held.Published = true
	if err := cs.UpdateContent(ctx, held); !errors.Is(err, ErrContentHeld) {
		t.Errorf("update held to published: err = %v, want ErrContentHeld", err)
	}
	if c, _ := cs.GetContent(ctx, held.ID); c == nil || c.Published || !c.Hold {
		t.Errorf("held page state after refused publishes: %+v", c)
	}
	held.Published = false
	held.Hold = false
	if err := cs.UpdateContent(ctx, held); err != nil {
		t.Fatalf("clear hold: %v", err)
	}
	if err := cs.PublishContent(ctx, held.ID); err != nil {
		t.Errorf("publish after clearing hold: %v", err)
	}

	// Holding an already-published page does not unpublish it.
	pub, _ := cs.GetContent(ctx, held.ID)
	pub.Hold = true
	if err := cs.UpdateContent(ctx, pub); err != nil {
		t.Fatalf("hold a published page: %v", err)
	}
	if c, _ := cs.GetContent(ctx, held.ID); c == nil || !c.Published || !c.Hold {
		t.Errorf("published page after hold: %+v, want published and held", c)
	}

	// A held page cannot be created already-published.
	if err := cs.CreateContent(ctx, &models.Content{Title: "X", Slug: "guard-held-pub", Hold: true, Published: true}); !errors.Is(err, ErrContentHeld) {
		t.Errorf("create held+published: err = %v, want ErrContentHeld", err)
	}

	// An upsert never clears a hold.
	keep := seedDraft(t, cs, "/guard-upsert", true)
	if _, err := cs.UpsertContent(ctx, &models.Content{Title: "Upserted", Slug: "guard-upsert"}, ""); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if c, _ := cs.GetContent(ctx, keep.ID); c == nil || !c.Hold || c.Title != "Upserted" {
		t.Errorf("after upsert: %+v, want hold kept and title updated", c)
	}
}

func TestContentService_ListingsHideForkCopies(t *testing.T) {
	db, cleanup := testutil.MustConnectTestDB(t)
	defer cleanup()

	cs := NewContentService(db)
	fs := NewForkService(db, cs)
	ctx := context.Background()

	liveID := seedLiveContent(t, db, "List Live", "/list-live")
	fork, err := fs.Create(ctx, "list", "", primitive.NewObjectID(), "ed@x.com")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	copyPage, err := fs.ForkPage(ctx, fork.ID, liveID)
	if err != nil {
		t.Fatalf("ForkPage: %v", err)
	}

	has := func(items []models.Content, id primitive.ObjectID) bool {
		for _, c := range items {
			if c.ID == id {
				return true
			}
		}
		return false
	}
	check := func(name string, items []models.Content, wantCopy bool) {
		t.Helper()
		if !has(items, liveID) {
			t.Errorf("%s: live page missing", name)
		}
		if has(items, copyPage.ID) != wantCopy {
			t.Errorf("%s: fork copy present = %v, want %v", name, !wantCopy, wantCopy)
		}
	}

	items, err := cs.ListContent(ctx, false, "", nil)
	if err != nil {
		t.Fatalf("ListContent: %v", err)
	}
	check("ListContent", items, false)
	items, _ = cs.ListContent(ctx, false, "", nil, true)
	check("ListContent include forks", items, true)

	items, pr, err := cs.ListContentPaginated(ctx, PaginationOpts{Limit: 50})
	if err != nil {
		t.Fatalf("ListContentPaginated: %v", err)
	}
	check("ListContentPaginated", items, false)
	items, prAll, _ := cs.ListContentPaginated(ctx, PaginationOpts{Limit: 50, IncludeForks: true})
	check("ListContentPaginated include forks", items, true)
	if prAll.Total != pr.Total+1 {
		t.Errorf("paginated totals: default=%d include_forks=%d, want a difference of 1", pr.Total, prAll.Total)
	}

	scope := ContentScope{ContentIDs: []primitive.ObjectID{liveID, copyPage.ID}}
	items, err = cs.ListContentScoped(ctx, scope)
	if err != nil {
		t.Fatalf("ListContentScoped: %v", err)
	}
	check("ListContentScoped", items, false)
	scope.IncludeForks = true
	items, _ = cs.ListContentScoped(ctx, scope)
	check("ListContentScoped include forks", items, true)

	// Fork-specific reads still see the copy.
	if pages, err := fs.ListPages(ctx, fork.ID); err != nil || len(pages) != 1 {
		t.Errorf("ListPages = %d pages, %v; want 1", len(pages), err)
	}
	if c, err := cs.GetContent(ctx, copyPage.ID); err != nil || c.ForkID == nil {
		t.Errorf("GetContent of a fork copy: %+v, %v", c, err)
	}

	// Search: a fork copy is never a result, even one flagged published
	// with matching text (the published live page is).
	ss := NewSearchService(db, "")
	for _, id := range []primitive.ObjectID{liveID, copyPage.ID} {
		if err := db.UpdateOne(ctx, "content", bson.M{"_id": id},
			bson.M{"$set": bson.M{"published": true, "plain_text": "zebracorn sighting"}}); err != nil {
			t.Fatalf("seed search text: %v", err)
		}
	}
	results, err := ss.SearchFullText(ctx, "zebracorn", 10)
	if err != nil {
		t.Fatalf("SearchFullText: %v", err)
	}
	if len(results) != 1 || results[0].ContentID != liveID.Hex() {
		t.Errorf("search results = %+v, want only the live page", results)
	}
	sug, err := ss.Suggest(ctx, "List Live", 8)
	if err != nil {
		t.Fatalf("Suggest: %v", err)
	}
	if len(sug.Pages) != 1 {
		t.Errorf("suggest pages = %d, want 1 (fork copy excluded)", len(sug.Pages))
	}
}

func TestScheduler_SkipsHeldAndForkCopies(t *testing.T) {
	db, cleanup := testutil.MustConnectTestDB(t)
	defer cleanup()

	cs := NewContentService(db)
	sched := NewSchedulerService(db, cs)
	ctx := context.Background()
	due := time.Now().Add(-time.Minute)

	normal := seedDraft(t, cs, "/sched-normal", false)
	held := seedDraft(t, cs, "/sched-held", true)
	forkID := primitive.NewObjectID()
	forkCopy := &models.Content{
		ID: primitive.NewObjectID(), Title: "Sched Copy", Slug: "sched-normal", FullPath: "/sched-normal",
		ForkID: &forkID, PublishAt: &due, CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	if _, err := db.InsertOne(ctx, "content", forkCopy); err != nil {
		t.Fatalf("insert fork copy: %v", err)
	}
	for _, id := range []primitive.ObjectID{normal.ID, held.ID} {
		if err := db.UpdateOne(ctx, "content", bson.M{"_id": id}, bson.M{"$set": bson.M{"publish_at": due}}); err != nil {
			t.Fatalf("schedule: %v", err)
		}
	}

	sched.runOnce(ctx)
	sched.runOnce(ctx) // a second tick must not publish the held page either

	published := func(id primitive.ObjectID) bool {
		c, err := cs.GetContent(ctx, id)
		if err != nil {
			t.Fatalf("GetContent: %v", err)
		}
		return c.Published
	}
	if !published(normal.ID) {
		t.Error("due page was not published")
	}
	if published(held.ID) {
		t.Error("held page was published by the scheduler")
	}
	if published(forkCopy.ID) {
		t.Error("fork copy was published by the scheduler")
	}
	if !sched.heldLogged[held.ID] {
		t.Error("skipped held page was not recorded as logged")
	}
}

func TestForkService_Merge_Errors(t *testing.T) {
	db, cleanup := testutil.MustConnectTestDB(t)
	defer cleanup()

	fs := NewForkService(db, NewContentService(db))
	ctx := context.Background()
	uid := primitive.NewObjectID()

	// Unknown fork.
	if _, err := fs.Merge(ctx, primitive.NewObjectID(), uid, "x@x.com", false, reviewDigest(t, fs, primitive.NewObjectID())); err == nil {
		t.Error("expected error for unknown fork")
	}

	// InsertOne failure while creating a new live page.
	fork, err := fs.Create(ctx, "err-fork", "", uid, "ed@x.com")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	forkOnly := &models.Content{
		ID: primitive.NewObjectID(), Title: "Only Fork", Slug: "err-new",
		FullPath: "/err-new", ForkID: &fork.ID,
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	if _, err := db.InsertOne(ctx, "content", forkOnly); err != nil {
		t.Fatalf("insert fork-only page: %v", err)
	}
	db.SetFaultHook(testutil.FailOp("InsertOne"))
	_, mergeErr := fs.Merge(ctx, fork.ID, uid, "x@x.com", false, reviewDigest(t, fs, fork.ID))
	db.SetFaultHook(nil)
	if mergeErr == nil {
		t.Error("expected merge error when live-page insert fails")
	}

	// UpdateOne failure while updating an existing live page.
	liveID := seedLiveContent(t, db, "Live Err", "/err-live")
	fork2, err := fs.Create(ctx, "err-fork-2", "", uid, "ed@x.com")
	if err != nil {
		t.Fatalf("Create 2: %v", err)
	}
	if _, err := fs.ForkPage(ctx, fork2.ID, liveID); err != nil {
		t.Fatalf("ForkPage: %v", err)
	}
	db.SetFaultHook(testutil.FailOp("UpdateOne"))
	_, mergeErr = fs.Merge(ctx, fork2.ID, uid, "x@x.com", false, reviewDigest(t, fs, fork2.ID))
	db.SetFaultHook(nil)
	if mergeErr == nil {
		t.Error("expected merge error when live-page update fails")
	}
}

func TestForkService_DeleteErrors(t *testing.T) {
	db, cleanup := testutil.MustConnectTestDB(t)
	defer cleanup()

	fs := NewForkService(db, NewContentService(db))
	ctx := context.Background()

	// Unknown fork.
	if err := fs.Delete(ctx, primitive.NewObjectID()); err == nil {
		t.Error("expected error deleting unknown fork")
	}
}

func TestApproval_HeldContentStaysDraft(t *testing.T) {
	db, cleanup := testutil.MustConnectTestDB(t)
	defer cleanup()

	cs := NewContentService(db)
	svc := NewApprovalService(db, cs, NewCommentService(db), NewWebhookService(db))
	ctx := context.Background()
	approverID, submitterID := primitive.NewObjectID(), primitive.NewObjectID()

	for _, tc := range []struct {
		path          string
		hold          bool
		wantPublished bool
	}{
		{"/appr-held", true, false},
		{"/appr-normal", false, true},
	} {
		page := seedDraft(t, cs, tc.path, tc.hold)
		req, err := svc.SubmitContentForApproval(ctx, page, submitterID, "sub@x.com")
		if err != nil || req == nil {
			t.Fatalf("%s: Submit: %v", tc.path, err)
		}
		if err := svc.Approve(ctx, req.ID, approverID, "approver@x.com", "ok"); err != nil {
			t.Fatalf("%s: Approve: %v", tc.path, err)
		}
		got, err := cs.GetContent(ctx, page.ID)
		if err != nil {
			t.Fatalf("%s: GetContent: %v", tc.path, err)
		}
		if got.Published != tc.wantPublished {
			t.Errorf("%s: published after approval = %v, want %v", tc.path, got.Published, tc.wantPublished)
		}
		if got.PendingApproval {
			t.Errorf("%s: still pending approval after it was approved", tc.path)
		}
	}
}

// reviewDigest reviews a fork, as a person or agent does before merging, and
// returns the digest a merge presents.
func reviewDigest(t *testing.T, fs *ForkService, forkID primitive.ObjectID) string {
	t.Helper()
	review, err := fs.Review(context.Background(), forkID)
	if err != nil {
		return ""
	}
	return review.Digest
}
