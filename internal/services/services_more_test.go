package services

import (
	"context"
	"testing"
	"time"

	"github.com/metavert-io/lightcms/v7/internal/models"
	"github.com/metavert-io/lightcms/v7/internal/services/importer"
	"github.com/metavert-io/lightcms/v7/internal/testutil"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

func TestScheduler_StartStop(t *testing.T) {
	db, cleanup := testutil.MustConnectTestDB(t)
	defer cleanup()
	sch := NewSchedulerService(db, NewContentService(db))
	ctx, cancel := context.WithCancel(context.Background())
	sch.Start(ctx)
	sch.Stop()
	cancel()
}

func TestRegenQueue_Worker(t *testing.T) {
	db, cleanup := testutil.MustConnectTestDB(t)
	defer cleanup()
	cs := NewContentService(db)
	tmplID := seedTmpl(t, cs, "Page", "page")
	// Published content for the template so processJob has something to regenerate.
	_ = cs.CreateContent(context.Background(), &models.Content{
		ID: primitive.NewObjectID(), TemplateID: tmplID, Title: "P", Slug: "p", FullPath: "/p",
		Published: true, Data: map[string]interface{}{"body": "x"},
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	})

	q := NewRegenQueue(db, cs)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	q.Start(ctx)
	q.Enqueue(ctx, tmplID, "Page")

	// Wait for the worker to process the job.
	for i := 0; i < 50; i++ {
		jobs, _ := q.ListRecentJobs(ctx, 5)
		if len(jobs) > 0 && jobs[0].Status != "pending" && jobs[0].Status != "running" {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func TestForkService_Merge(t *testing.T) {
	db, cleanup := testutil.MustConnectTestDB(t)
	defer cleanup()
	cs := NewContentService(db)
	fs := NewForkService(db, cs)
	ctx := context.Background()
	uid := primitive.NewObjectID()

	liveID := seedLiveContent(t, db, "Home", "/home")
	fork, err := fs.Create(ctx, "WS", "", uid, "e@x.com")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := fs.ForkPage(ctx, fork.ID, liveID); err != nil {
		t.Fatalf("ForkPage: %v", err)
	}
	if _, err := fs.Merge(ctx, fork.ID, uid, "e@x.com", false, reviewDigest(t, fs, fork.ID)); err != nil {
		t.Errorf("Merge: %v", err)
	}
}

func TestContentService_StreamsAndPagination(t *testing.T) {
	db, cleanup := testutil.MustConnectTestDB(t)
	defer cleanup()
	cs := NewContentService(db)
	tmplID := seedTmpl(t, cs, "Page", "page")
	ctx := context.Background()
	for _, s := range []string{"a", "b", "c"} {
		_ = cs.CreateContent(ctx, &models.Content{
			ID: primitive.NewObjectID(), TemplateID: tmplID, Title: s, Slug: s, FullPath: "/" + s,
			Data: map[string]interface{}{"body": "x"}, CreatedAt: time.Now(), UpdatedAt: time.Now(),
		})
	}

	cur, err := cs.StreamContent(ctx, false)
	if err != nil {
		t.Errorf("StreamContent: %v", err)
	} else {
		cur.Close(ctx)
	}
	if _, _, err := cs.ListContentPaginated(ctx, PaginationOpts{Limit: 2}); err != nil {
		t.Errorf("ListContentPaginated: %v", err)
	}
}

func TestSanitizeContentData(t *testing.T) {
	out := SanitizeContentData(map[string]interface{}{
		"body":   "<p>ok</p><script>alert(1)</script>",
		"number": 42,
	})
	if out == nil {
		t.Fatal("SanitizeContentData returned nil")
	}
}

func TestAPIKeyService_ForUser(t *testing.T) {
	db, cleanup := testutil.MustConnectTestDB(t)
	defer cleanup()
	svc := NewAPIKeyService(db)
	ctx := context.Background()
	uid := primitive.NewObjectID()

	_, key, err := svc.CreateAPIKeyForUser(ctx, "k", "d", &uid)
	if err != nil {
		t.Fatalf("CreateAPIKeyForUser: %v", err)
	}
	if err := svc.DeleteAPIKeyForUser(ctx, key.ID, uid); err != nil {
		t.Errorf("DeleteAPIKeyForUser: %v", err)
	}
}

func TestImportService_JobLogsAndCancel(t *testing.T) {
	db, cleanup := testutil.MustConnectTestDB(t)
	defer cleanup()
	cs := NewContentService(db)
	seedNamedTemplate(t, db, "JL")
	is := NewImportService(db, cs)
	ctx := context.Background()

	job, err := is.RunMarkdownImport(ctx, []importer.MarkdownPage{
		{Filename: "a.md", Frontmatter: map[string]string{"title": "A"}, Body: "x"},
	}, "JL", "", false, "tester")
	if err != nil {
		t.Fatalf("RunMarkdownImport: %v", err)
	}
	if _, err := is.GetJobLogs(ctx, job.ID, 0); err != nil {
		t.Errorf("GetJobLogs: %v", err)
	}
	_ = is.CancelJob(ctx, job.ID)
}
