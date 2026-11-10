package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/metavert-io/lightcms/v7/internal/database"
	"github.com/metavert-io/lightcms/v7/internal/models"

	"github.com/gorilla/mux"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// Tests for the 7.4.0 draft-safety changes: fork copies and held pages are
// never published in bulk, fork copies stay out of listings and search, and
// merges publish (optionally), clean up, and can be purged.

// seedPage inserts a content document directly and returns its ID.
func seedPage(t *testing.T, db *database.DB, c models.Content) primitive.ObjectID {
	t.Helper()
	c.ID = primitive.NewObjectID()
	if c.Slug == "" {
		c.Slug = strings.TrimPrefix(c.FullPath, "/")
	}
	now := time.Now()
	c.CreatedAt, c.UpdatedAt = now, now
	if _, err := db.InsertOne(context.Background(), "content", &c); err != nil {
		t.Fatalf("seedPage %s: %v", c.FullPath, err)
	}
	return c.ID
}

// loadPage reads a content document by ID.
func loadPage(t *testing.T, db *database.DB, id primitive.ObjectID) models.Content {
	t.Helper()
	var c models.Content
	if err := db.FindOne(context.Background(), "content", bson.M{"_id": id}, &c); err != nil {
		t.Fatalf("loadPage %s: %v", id.Hex(), err)
	}
	return c
}

// apiCall runs an APIHandler method with an admin API user and route vars,
// and decodes a JSON object response.
func apiCall(t *testing.T, h http.HandlerFunc, method, target, body string, vars map[string]string) (*httptest.ResponseRecorder, map[string]interface{}) {
	t.Helper()
	req := authReq(method, target, strings.NewReader(body))
	if vars != nil {
		req = mux.SetURLVars(req, vars)
	}
	rr := httptest.NewRecorder()
	h(rr, req)
	var out map[string]interface{}
	_ = json.Unmarshal(rr.Body.Bytes(), &out)
	return rr, out
}

// forkDigest reviews a fork, as a person or agent does before merging, and
// returns the digest the merge presents.
func forkDigest(t *testing.T, a *APIHandler, forkID primitive.ObjectID) string {
	t.Helper()
	review, err := a.forkService.Review(context.Background(), forkID)
	if err != nil {
		t.Fatalf("Review: %v", err)
	}
	return review.Digest
}

// idSet collects the "id" of each element of a JSON array of objects, or the
// elements themselves when they are strings.
func idSet(v interface{}) map[string]map[string]interface{} {
	out := map[string]map[string]interface{}{}
	items, _ := v.([]interface{})
	for _, it := range items {
		switch x := it.(type) {
		case string:
			out[x] = nil
		case map[string]interface{}:
			if id, ok := x["id"].(string); ok {
				out[id] = x
			}
		}
	}
	return out
}

func TestAPIBatchPublish_SkipsForkCopiesAndHeld(t *testing.T) {
	a, db, cleanup := newTestAPIHandler(t)
	defer cleanup()

	forkID := createTestFork(t, db, "bp")
	draft := seedPage(t, db, models.Content{Title: "Draft", FullPath: "/bp-draft"})
	held := seedPage(t, db, models.Content{Title: "Held", FullPath: "/bp-held", Hold: true})
	live := seedPage(t, db, models.Content{Title: "Live", FullPath: "/bp-live", Published: true})
	forkCopy := seedPage(t, db, models.Content{Title: "Live (fork)", FullPath: "/bp-live", ForkID: &forkID})

	// publish_all_drafts: publishes the draft, reports the held page as
	// skipped, and never considers the fork copy.
	rr, out := apiCall(t, a.APIBatchPublishContent, "POST", "/api/v1/content/batch-publish", `{"publish_all_drafts":true}`, nil)
	if rr.Code != 200 {
		t.Fatalf("status = %d, body: %s", rr.Code, rr.Body.String())
	}
	published, skipped := idSet(out["published"]), idSet(out["skipped"])
	if _, ok := published[draft.Hex()]; !ok {
		t.Errorf("draft not in published: %v", out["published"])
	}
	if _, ok := published[forkCopy.Hex()]; ok {
		t.Error("fork copy listed as published")
	}
	if s, ok := skipped[held.Hex()]; !ok || s["reason"] != "on hold" {
		t.Errorf("held page not skipped with reason: %v", out["skipped"])
	}
	if _, ok := skipped[forkCopy.Hex()]; ok {
		t.Error("publish_all_drafts should not consider fork copies at all")
	}
	if !loadPage(t, db, draft).Published {
		t.Error("draft was not published")
	}
	if loadPage(t, db, held).Published {
		t.Error("held page was published")
	}
	if loadPage(t, db, forkCopy).Published {
		t.Error("fork copy was published")
	}
	if !loadPage(t, db, live).Published {
		t.Error("live page lost its published state")
	}

	// Explicit IDs: held pages and fork copies come back in skipped.
	draft2 := seedPage(t, db, models.Content{Title: "Draft 2", FullPath: "/bp-draft-2"})
	body := `{"ids":["` + draft2.Hex() + `","` + held.Hex() + `","` + forkCopy.Hex() + `"]}`
	rr, out = apiCall(t, a.APIBatchPublishContent, "POST", "/api/v1/content/batch-publish", body, nil)
	if rr.Code != 200 {
		t.Fatalf("explicit ids: status = %d, body: %s", rr.Code, rr.Body.String())
	}
	published, skipped = idSet(out["published"]), idSet(out["skipped"])
	if _, ok := published[draft2.Hex()]; !ok || len(published) != 1 {
		t.Errorf("published = %v, want only draft2", out["published"])
	}
	if s, ok := skipped[held.Hex()]; !ok || s["reason"] != "on hold" {
		t.Errorf("held page not skipped: %v", out["skipped"])
	}
	if s, ok := skipped[forkCopy.Hex()]; !ok || !strings.Contains(s["reason"].(string), "fork copy") {
		t.Errorf("fork copy not skipped: %v", out["skipped"])
	}
	if loadPage(t, db, held).Published || loadPage(t, db, forkCopy).Published {
		t.Error("held page or fork copy was published by explicit-id batch")
	}
}

func TestAPIPublishContent_Guards(t *testing.T) {
	a, db, cleanup := newTestAPIHandler(t)
	defer cleanup()

	forkID := createTestFork(t, db, "pg")
	held := seedPage(t, db, models.Content{Title: "Held", FullPath: "/pg-held", Hold: true})
	forkCopy := seedPage(t, db, models.Content{Title: "Copy", FullPath: "/pg-copy", ForkID: &forkID})

	rr, out := apiCall(t, a.APIPublishContent, "POST", "/x", "", map[string]string{"id": held.Hex()})
	if rr.Code != http.StatusConflict || !strings.Contains(out["error"].(string), "on hold") {
		t.Errorf("publish held: %d %s", rr.Code, rr.Body.String())
	}
	rr, out = apiCall(t, a.APIPublishContent, "POST", "/x", "", map[string]string{"id": forkCopy.Hex()})
	if rr.Code != http.StatusConflict || !strings.Contains(out["error"].(string), "merge the fork") {
		t.Errorf("publish fork copy: %d %s", rr.Code, rr.Body.String())
	}
	if loadPage(t, db, held).Published || loadPage(t, db, forkCopy).Published {
		t.Error("a guarded page was published")
	}
}

func TestAPIContent_HoldFlag(t *testing.T) {
	a, db, cleanup := newTestAPIHandler(t)
	defer cleanup()

	tmplID := seedTemplate(t, db, "Page", "page")
	base := `"template_id":"` + tmplID.Hex() + `","data":{}`

	// Create held.
	rr, out := apiCall(t, a.APICreateContent, "POST", "/api/v1/content", `{`+base+`,"title":"H","slug":"hf-held","hold":true}`, nil)
	if rr.Code != 201 {
		t.Fatalf("create held: %d %s", rr.Code, rr.Body.String())
	}
	if out["hold"] != true {
		t.Errorf("created content hold = %v, want true", out["hold"])
	}
	id, _ := primitive.ObjectIDFromHex(out["id"].(string))
	vars := map[string]string{"id": id.Hex()}

	// Held + published in one create is refused.
	rr, _ = apiCall(t, a.APICreateContent, "POST", "/api/v1/content", `{`+base+`,"title":"HP","slug":"hf-held-pub","hold":true,"published":true}`, nil)
	if rr.Code != http.StatusConflict {
		t.Errorf("create held+published: %d, want 409", rr.Code)
	}

	// An update cannot publish a held page...
	rr, _ = apiCall(t, a.APIUpdateContent, "PUT", "/x", `{"published":true}`, vars)
	if rr.Code != http.StatusConflict {
		t.Errorf("update held to published: %d, want 409", rr.Code)
	}
	if c := loadPage(t, db, id); c.Published || !c.Hold {
		t.Errorf("after refused update: published=%v hold=%v", c.Published, c.Hold)
	}
	// ...nor can a by-path update.
	req := authReq("PUT", "/api/v1/content/by-path?path=/hf-held", strings.NewReader(`{"published":true}`))
	rec := httptest.NewRecorder()
	a.APIUpdateContentByPath(rec, req)
	if rec.Code != http.StatusConflict {
		t.Errorf("by-path update held to published: %d, want 409", rec.Code)
	}

	// Other edits to a held page are fine and keep the hold.
	rr, _ = apiCall(t, a.APIUpdateContent, "PUT", "/x", `{"title":"H2"}`, vars)
	if rr.Code != 200 {
		t.Errorf("edit held: %d %s", rr.Code, rr.Body.String())
	}
	if c := loadPage(t, db, id); c.Title != "H2" || !c.Hold {
		t.Errorf("after edit: title=%q hold=%v", c.Title, c.Hold)
	}

	// Clear the hold, then publish.
	rr, _ = apiCall(t, a.APIUpdateContent, "PUT", "/x", `{"hold":false}`, vars)
	if rr.Code != 200 || loadPage(t, db, id).Hold {
		t.Errorf("clear hold: %d, hold=%v", rr.Code, loadPage(t, db, id).Hold)
	}
	rr, _ = apiCall(t, a.APIPublishContent, "POST", "/x", "", vars)
	if rr.Code != 200 || !loadPage(t, db, id).Published {
		t.Errorf("publish after clearing hold: %d %s", rr.Code, rr.Body.String())
	}

	// Holding a published page does not unpublish it.
	rr, _ = apiCall(t, a.APIUpdateContent, "PUT", "/x", `{"hold":true}`, vars)
	if c := loadPage(t, db, id); rr.Code != 200 || !c.Hold || !c.Published {
		t.Errorf("hold a published page: %d hold=%v published=%v", rr.Code, c.Hold, c.Published)
	}
}

func TestAPIListAndSearch_IncludeForks(t *testing.T) {
	a, db, cleanup := newTestAPIHandler(t)
	defer cleanup()

	forkID := createTestFork(t, db, "lf")
	live := seedPage(t, db, models.Content{Title: "Quokka Live", FullPath: "/lf-page", Published: true})
	forkCopy := seedPage(t, db, models.Content{Title: "Quokka Fork", FullPath: "/lf-page", ForkID: &forkID})

	listIDs := func(target string) map[string]map[string]interface{} {
		t.Helper()
		rr := httptest.NewRecorder()
		a.APIListContent(rr, authReq("GET", target, nil))
		if rr.Code != 200 {
			t.Fatalf("%s: %d %s", target, rr.Code, rr.Body.String())
		}
		var items []interface{}
		if err := json.Unmarshal(rr.Body.Bytes(), &items); err != nil {
			var env map[string]interface{}
			if err := json.Unmarshal(rr.Body.Bytes(), &env); err != nil {
				t.Fatalf("%s: decode: %v", target, err)
			}
			items, _ = env["items"].([]interface{})
		}
		return idSet(items)
	}

	for _, target := range []string{"/api/v1/content", "/api/v1/content?limit=50", "/api/v1/content?include_data=true"} {
		ids := listIDs(target)
		if _, ok := ids[live.Hex()]; !ok {
			t.Errorf("%s: live page missing", target)
		}
		if _, ok := ids[forkCopy.Hex()]; ok {
			t.Errorf("%s: fork copy listed by default", target)
		}
		sep := "?"
		if strings.Contains(target, "?") {
			sep = "&"
		}
		ids = listIDs(target + sep + "include_forks=true")
		if _, ok := ids[forkCopy.Hex()]; !ok {
			t.Errorf("%s with include_forks: fork copy missing", target)
		}
	}
	// The slim list format marks fork copies.
	if c := listIDs("/api/v1/content?limit=50&include_forks=true")[forkCopy.Hex()]; c == nil || c["fork_id"] != forkID.Hex() {
		t.Errorf("fork copy in paginated list lacks fork_id: %v", c)
	}

	search := func(target string) map[string]map[string]interface{} {
		t.Helper()
		rr, out := apiCall(t, a.APISearchContent, "GET", target, "", nil)
		if rr.Code != 200 {
			t.Fatalf("%s: %d %s", target, rr.Code, rr.Body.String())
		}
		return idSet(out["matches"])
	}
	m := search("/api/v1/search?q=quokka&type=name")
	if _, ok := m[live.Hex()]; !ok || len(m) != 1 {
		t.Errorf("search default = %v, want only the live page", m)
	}
	m = search("/api/v1/search?q=quokka&type=name&include_forks=true")
	if c, ok := m[forkCopy.Hex()]; !ok || c["fork_id"] != forkID.Hex() {
		t.Errorf("search include_forks: fork copy missing or unmarked: %v", m)
	}

	// Fork-specific endpoints and get-by-id still see the copy.
	rr, out := apiCall(t, a.APIGetFork, "GET", "/x", "", map[string]string{"id": forkID.Hex()})
	if rr.Code != 200 || out["page_count"] != float64(1) {
		t.Errorf("get fork: %d %s", rr.Code, rr.Body.String())
	}
	rr, out = apiCall(t, a.APIGetContent, "GET", "/x", "", map[string]string{"id": forkCopy.Hex()})
	if rr.Code != 200 || out["fork_id"] != forkID.Hex() {
		t.Errorf("get fork copy by id: %d %s", rr.Code, rr.Body.String())
	}
}

func TestAPIMergeFork_PublishNewAndCleanup(t *testing.T) {
	a, db, cleanup := newTestAPIHandler(t)
	defer cleanup()
	ctx := context.Background()

	// Default merge: new page stays a draft, copies are removed.
	forkID := createTestFork(t, db, "mg-default")
	live := seedPage(t, db, models.Content{Title: "Live", FullPath: "/mg-live", Published: true})
	seedPage(t, db, models.Content{Title: "Live Edited", FullPath: "/mg-live", ForkID: &forkID})
	seedPage(t, db, models.Content{Title: "New", FullPath: "/mg-new", ForkID: &forkID})
	vars := map[string]string{"id": forkID.Hex()}

	// Without the digest of a review, nothing is merged.
	if rr, _ := apiCall(t, a.APIMergeFork, "POST", "/x", "", vars); rr.Code != 400 {
		t.Fatalf("merge without a digest: %d %s", rr.Code, rr.Body.String())
	}

	rr, out := apiCall(t, a.APIMergeFork, "POST", "/x", `{"digest":"`+forkDigest(t, a, forkID)+`"}`, vars) // default behaviour
	if rr.Code != 200 {
		t.Fatalf("merge: %d %s", rr.Code, rr.Body.String())
	}
	created, updated := idSet(out["created_ids"]), idSet(out["updated_ids"])
	if len(created) != 1 || len(updated) != 1 {
		t.Fatalf("created_ids=%v updated_ids=%v, want one each", out["created_ids"], out["updated_ids"])
	}
	if _, ok := updated[live.Hex()]; !ok {
		t.Errorf("updated_ids = %v, want the live page", out["updated_ids"])
	}
	for id := range created {
		oid, _ := primitive.ObjectIDFromHex(id)
		if c := loadPage(t, db, oid); c.Published || c.ForkID != nil || c.FullPath != "/mg-new" {
			t.Errorf("created page: published=%v fork=%v path=%s", c.Published, c.ForkID, c.FullPath)
		}
	}
	if loadPage(t, db, live).Title != "Live Edited" {
		t.Error("live page not updated by merge")
	}
	if n, _ := db.Count(ctx, "content", bson.M{"fork_id": forkID}); n != 0 {
		t.Errorf("fork copies after merge = %d, want 0", n)
	}

	// Fork detail and diff of the merged fork degrade to the stored counts.
	rr, out = apiCall(t, a.APIGetFork, "GET", "/x", "", vars)
	if rr.Code != 200 || out["status"] != "merged" || out["page_count"] != float64(0) ||
		out["merged_created"] != float64(1) || out["merged_updated"] != float64(1) {
		t.Errorf("get merged fork: %d %s", rr.Code, rr.Body.String())
	}
	rr, out = apiCall(t, a.APIForkDiff, "GET", "/x", "", vars)
	if rr.Code != 200 || out["status"] != "merged" || out["merged_created"] != float64(1) {
		t.Errorf("diff merged fork: %d %s", rr.Code, rr.Body.String())
	}
	if pages, _ := out["pages"].([]interface{}); len(pages) != 0 {
		t.Errorf("diff pages of merged fork = %v, want empty", out["pages"])
	}

	// A merged fork can be deleted.
	rr, _ = apiCall(t, a.APIDeleteFork, "DELETE", "/x", "", vars)
	if rr.Code != 200 {
		t.Errorf("delete merged fork: %d %s", rr.Code, rr.Body.String())
	}
	if n, _ := db.Count(ctx, "content_forks", bson.M{"_id": forkID}); n != 0 {
		t.Error("merged fork record still present after delete")
	}

	// publish_new: new pages go live, except the held one.
	fork2 := createTestFork(t, db, "mg-publish")
	seedPage(t, db, models.Content{Title: "New Pub", FullPath: "/mg-new-pub", ForkID: &fork2})
	seedPage(t, db, models.Content{Title: "New Held", FullPath: "/mg-new-held", ForkID: &fork2, Hold: true})
	rr, out = apiCall(t, a.APIMergeFork, "POST", "/x", `{"publish_new":true,"digest":"`+forkDigest(t, a, fork2)+`"}`, map[string]string{"id": fork2.Hex()})
	if rr.Code != 200 || out["publish_new"] != true {
		t.Fatalf("merge publish_new: %d %s", rr.Code, rr.Body.String())
	}
	if len(idSet(out["created_ids"])) != 2 {
		t.Errorf("created_ids = %v, want 2", out["created_ids"])
	}
	var pub, heldPage models.Content
	if err := db.FindOne(ctx, "content", bson.M{"full_path": "/mg-new-pub", "fork_id": nil}, &pub); err != nil {
		t.Fatalf("published page not found: %v", err)
	}
	if err := db.FindOne(ctx, "content", bson.M{"full_path": "/mg-new-held", "fork_id": nil}, &heldPage); err != nil {
		t.Fatalf("held page not found: %v", err)
	}
	if !pub.Published || pub.PublishedAt == nil {
		t.Error("publish_new did not publish the new page")
	}
	if heldPage.Published || !heldPage.Hold {
		t.Errorf("held page after publish_new merge: published=%v hold=%v", heldPage.Published, heldPage.Hold)
	}
	if np := idSet(out["not_published"]); len(np) != 1 || np[heldPage.ID.Hex()] == nil {
		t.Errorf("not_published = %v, want the held page", out["not_published"])
	}

	// Malformed body is rejected before anything is merged.
	fork3 := createTestFork(t, db, "mg-bad")
	rr, _ = apiCall(t, a.APIMergeFork, "POST", "/x", `{not json`, map[string]string{"id": fork3.Hex()})
	if rr.Code != 400 {
		t.Errorf("bad merge body: %d, want 400", rr.Code)
	}
}

func TestAPIPurgeForkCopies(t *testing.T) {
	a, db, cleanup := newTestAPIHandler(t)
	defer cleanup()
	ctx := context.Background()

	forkID := createTestFork(t, db, "pc")
	live := seedPage(t, db, models.Content{Title: "Live", FullPath: "/pc-a", Published: true})
	seedPage(t, db, models.Content{Title: "A", FullPath: "/pc-a", ForkID: &forkID})
	seedPage(t, db, models.Content{Title: "B", FullPath: "/pc-b", ForkID: &forkID})
	vars := map[string]string{"id": forkID.Hex()}

	// Refused while the fork is active.
	rr, _ := apiCall(t, a.APIPurgeForkCopies, "POST", "/x", "", vars)
	if rr.Code != http.StatusConflict {
		t.Errorf("purge active fork: %d, want 409", rr.Code)
	}
	if n, _ := db.Count(ctx, "content", bson.M{"fork_id": forkID}); n != 2 {
		t.Fatalf("copies after refused purge = %d, want 2", n)
	}

	// Unknown and invalid fork IDs.
	rr, _ = apiCall(t, a.APIPurgeForkCopies, "POST", "/x", "", map[string]string{"id": primitive.NewObjectID().Hex()})
	if rr.Code != 404 {
		t.Errorf("purge unknown fork: %d, want 404", rr.Code)
	}
	rr, _ = apiCall(t, a.APIPurgeForkCopies, "POST", "/x", "", map[string]string{"id": "nothex"})
	if rr.Code != 400 {
		t.Errorf("purge invalid id: %d, want 400", rr.Code)
	}

	// A fork merged before 7.4 kept its copies.
	if err := db.UpdateOne(ctx, "content_forks", bson.M{"_id": forkID}, bson.M{"$set": bson.M{"status": "merged"}}); err != nil {
		t.Fatalf("mark merged: %v", err)
	}

	// Dry run: count and list, nothing deleted, no audit entry.
	rr, out := apiCall(t, a.APIPurgeForkCopies, "POST", "/x?dry_run=true", "", vars)
	if rr.Code != 200 || out["dry_run"] != true || out["count"] != float64(2) {
		t.Fatalf("dry run: %d %s", rr.Code, rr.Body.String())
	}
	copies, _ := out["copies"].([]interface{})
	if len(copies) != 2 {
		t.Fatalf("dry run copies = %v, want 2", out["copies"])
	}
	first, _ := copies[0].(map[string]interface{})
	if first["full_path"] != "/pc-a" || first["id"] == "" {
		t.Errorf("dry run copy entry = %v, want {id, full_path:/pc-a}", first)
	}
	if n, _ := db.Count(ctx, "content", bson.M{"fork_id": forkID}); n != 2 {
		t.Errorf("dry run deleted copies: %d left", n)
	}

	// Real purge.
	rr, out = apiCall(t, a.APIPurgeForkCopies, "POST", "/x", "", vars)
	if rr.Code != 200 || out["deleted"] != float64(2) || out["dry_run"] != false {
		t.Fatalf("purge: %d %s", rr.Code, rr.Body.String())
	}
	if n, _ := db.Count(ctx, "content", bson.M{"fork_id": forkID}); n != 0 {
		t.Errorf("copies after purge = %d, want 0", n)
	}
	if !loadPage(t, db, live).Published {
		t.Error("purge touched the live page")
	}

	// The audit entry is written asynchronously; exactly one (the dry run
	// wrote none).
	var n int64
	for i := 0; i < 50; i++ {
		n, _ = db.Count(ctx, "audit_logs", bson.M{"action": "fork.purge_copies", "resource_id": forkID.Hex()})
		if n > 0 {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if n != 1 {
		t.Errorf("fork.purge_copies audit entries = %d, want 1", n)
	}
}

func TestAdminForkMerge_PublishNewAndMergedView(t *testing.T) {
	h, cleanup := newTestHandler(t)
	defer cleanup()
	db := testDB(t)
	ctx := context.Background()

	forkID := createTestFork(t, db, "adm")
	seedPage(t, db, models.Content{Title: "Adm New", FullPath: "/adm-new", ForkID: &forkID})
	seedPage(t, db, models.Content{Title: "Adm Held", FullPath: "/adm-held", ForkID: &forkID, Hold: true})
	vars := map[string]string{"id": forkID.Hex()}

	// The active fork's page offers the publish-new checkbox.
	if rr := getPage(t, h.ViewFork, vars); rr.Code != 200 || !strings.Contains(rr.Body.String(), `name="publish_new"`) {
		t.Errorf("ViewFork (active): %d, publish_new checkbox present = %v", rr.Code, strings.Contains(rr.Body.String(), `name="publish_new"`))
	}

	rr := postForm(t, h.MergeFork, url.Values{"publish_new": {"on"}}, vars)
	if rr.Code != 200 {
		t.Fatalf("MergeFork: %d %s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "New pages left as drafts") || !strings.Contains(rr.Body.String(), "/adm-held") {
		t.Error("merge result does not list the held page as left a draft")
	}
	var pub, held models.Content
	if err := db.FindOne(ctx, "content", bson.M{"full_path": "/adm-new", "fork_id": nil}, &pub); err != nil || !pub.Published {
		t.Errorf("new page after admin merge with publish_new: published=%v err=%v", pub.Published, err)
	}
	if err := db.FindOne(ctx, "content", bson.M{"full_path": "/adm-held", "fork_id": nil}, &held); err != nil || held.Published {
		t.Errorf("held page after admin merge: published=%v err=%v", held.Published, err)
	}

	// The merged fork's page renders from the stored counts, with no copies.
	rr = getPage(t, h.ViewFork, vars)
	if rr.Code != 200 || !strings.Contains(rr.Body.String(), "This fork was merged: 0 updated, 2 created") {
		t.Errorf("ViewFork (merged): %d, stored counts shown = %v", rr.Code, strings.Contains(rr.Body.String(), "This fork was merged"))
	}
	if rr := getPage(t, h.ListForks, nil); rr.Code != 200 {
		t.Errorf("ListForks with a merged fork: %d", rr.Code)
	}

	// Deleting a merged fork from the admin UI now works.
	if rr := postForm(t, h.DeleteForkHandler, nil, vars); rr.Code != http.StatusSeeOther {
		t.Errorf("DeleteForkHandler (merged): %d, want 303", rr.Code)
	}
	if n, _ := db.Count(ctx, "content_forks", bson.M{"_id": forkID}); n != 0 {
		t.Error("merged fork record still present after admin delete")
	}
}

func TestAdminContent_HoldCheckboxAndBadge(t *testing.T) {
	h, cleanup := newTestHandler(t)
	defer cleanup()
	db := testDB(t)

	tmplID := seedTemplate(t, db, "Page", "page")
	id := seedPage(t, db, models.Content{Title: "Editor Page", FullPath: "/ed-page", TemplateID: tmplID, TemplateName: "Page"})
	vars := map[string]string{"id": id.Hex()}
	form := func(extra url.Values) url.Values {
		v := url.Values{"title": {"Editor Page"}, "slug": {"ed-page"}}
		for k, vals := range extra {
			v[k] = vals
		}
		return v
	}

	// The editor shows the Hold checkbox.
	if rr := getPage(t, h.EditContent, vars); rr.Code != 200 || !strings.Contains(rr.Body.String(), `name="hold"`) {
		t.Fatalf("EditContent: %d, hold checkbox present = %v", rr.Code, strings.Contains(rr.Body.String(), `name="hold"`))
	}

	// Hold + Published on a draft is refused and nothing is saved.
	rr := postForm(t, h.UpdateContent, form(url.Values{"hold": {"on"}, "published": {"on"}}), vars)
	if rr.Code != http.StatusSeeOther || !strings.Contains(rr.Header().Get("Location"), "error=held") {
		t.Errorf("hold+published: %d -> %q, want redirect with error=held", rr.Code, rr.Header().Get("Location"))
	}
	if c := loadPage(t, db, id); c.Hold || c.Published {
		t.Errorf("after refused save: hold=%v published=%v", c.Hold, c.Published)
	}

	// Hold alone is saved, and the content list shows the badge.
	if rr := postForm(t, h.UpdateContent, form(url.Values{"hold": {"on"}}), vars); rr.Code >= 400 {
		t.Fatalf("save hold: %d %s", rr.Code, rr.Body.String())
	}
	if c := loadPage(t, db, id); !c.Hold || c.Published {
		t.Errorf("after saving hold: hold=%v published=%v", c.Hold, c.Published)
	}
	if rr := getPage(t, h.ListContent, nil); rr.Code != 200 || !strings.Contains(rr.Body.String(), ">Held</span>") {
		t.Errorf("ListContent: %d, Held badge present = %v", rr.Code, strings.Contains(rr.Body.String(), ">Held</span>"))
	}

	// Unchecking Hold clears it.
	if rr := postForm(t, h.UpdateContent, form(nil), vars); rr.Code >= 400 {
		t.Fatalf("clear hold: %d", rr.Code)
	}
	if loadPage(t, db, id).Hold {
		t.Error("hold not cleared by saving with the box unchecked")
	}
}
