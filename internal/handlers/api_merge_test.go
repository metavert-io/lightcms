package handlers

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/metavert-io/lightcms/v7/internal/auth"
	"github.com/metavert-io/lightcms/v7/internal/middleware"
	"github.com/metavert-io/lightcms/v7/internal/models"
	"github.com/metavert-io/lightcms/v7/internal/services"

	"github.com/gorilla/mux"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// allowMerge lets the key merge forks it creates, as an admin does on the
// API Keys page.
func (e *sandboxEnv) allowMerge(t *testing.T, rawKey string, sandboxOnly bool) {
	t.Helper()
	key, err := e.keys.ValidateAPIKey(context.Background(), rawKey)
	if err != nil {
		t.Fatalf("ValidateAPIKey: %v", err)
	}
	if err := e.keys.UpdateAccess(context.Background(), key.ID, services.AccessSettings{SandboxOnly: sandboxOnly, CanMergeOwnForks: true}); err != nil {
		t.Fatalf("UpdateAccess: %v", err)
	}
}

// forkWithEdit creates a fork through the key, copies the live page into it
// and edits the copy's title. It returns the fork ID.
func (e *sandboxEnv) forkWithEdit(t *testing.T, key, title string) string {
	t.Helper()
	rr := e.send(key, "POST", "/api/v1/forks", `{"name":"work"}`)
	if rr.Code != http.StatusCreated {
		t.Fatalf("create fork: %d %s", rr.Code, rr.Body.String())
	}
	fork := decode(t, rr)["id"].(string)
	e.editInFork(t, key, fork, title)
	return fork
}

func (e *sandboxEnv) editInFork(t *testing.T, key, fork, title string) {
	t.Helper()
	rr := e.send(key, "POST", "/api/v1/forks/"+fork+"/fork-page", `{"path":"/live"}`)
	if rr.Code != http.StatusOK {
		t.Fatalf("fork-page: %d %s", rr.Code, rr.Body.String())
	}
	copyID := decode(t, rr)["id"].(string)
	if rr := e.send(key, "PUT", "/api/v1/content/"+copyID, fmt.Sprintf(`{"title":%q}`, title)); rr.Code != http.StatusOK {
		t.Fatalf("edit copy: %d %s", rr.Code, rr.Body.String())
	}
}

func (e *sandboxEnv) digest(t *testing.T, key, fork string) string {
	t.Helper()
	rr := e.send(key, "GET", "/api/v1/forks/"+fork+"/diff", ``)
	if rr.Code != http.StatusOK {
		t.Fatalf("diff: %d %s", rr.Code, rr.Body.String())
	}
	digest, _ := decode(t, rr)["digest"].(string)
	if digest == "" {
		t.Fatal("the diff has no digest")
	}
	return digest
}

func (e *sandboxEnv) merge(key, fork, digest string) *httptest.ResponseRecorder {
	return e.send(key, "POST", "/api/v1/forks/"+fork+"/merge", fmt.Sprintf(`{"digest":%q}`, digest))
}

func (e *sandboxEnv) liveTitle(t *testing.T) string {
	t.Helper()
	var live models.Content
	if err := e.db.FindOne(context.Background(), "content", bson.M{"_id": e.live}, &live); err != nil {
		t.Fatalf("live page: %v", err)
	}
	return live.Title
}

func TestMerge_AKeyNeedsTheGrantAndMergesOnlyItsOwnForks(t *testing.T) {
	e := newSandboxEnvAs(t, models.RoleAdmin)
	key := e.key(t, "Full, no grant", false)

	fork := e.forkWithEdit(t, key, "From the key")
	if rr := e.merge(key, fork, e.digest(t, key, fork)); rr.Code != http.StatusForbidden || !strings.Contains(rr.Body.String(), "may not merge") {
		t.Fatalf("merging without the grant: %d %s", rr.Code, rr.Body.String())
	}

	e.allowMerge(t, key, false)

	// A fork another key made, or a person made, is not this key's to merge.
	other := e.key(t, "Other", false)
	othersFork := e.forkWithEdit(t, other, "From the other key")
	if rr := e.merge(key, othersFork, e.digest(t, key, othersFork)); rr.Code != http.StatusForbidden || !strings.Contains(rr.Body.String(), "only forks it created") {
		t.Errorf("merging another key's fork: %d %s", rr.Code, rr.Body.String())
	}
	personsFork, err := e.a.forkService.Create(context.Background(), "by a person", "", e.owner.ID, e.owner.Email)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if rr := e.merge(key, personsFork.ID.Hex(), e.digest(t, key, personsFork.ID.Hex())); rr.Code != http.StatusForbidden {
		t.Errorf("merging a person's fork: %d %s", rr.Code, rr.Body.String())
	}

	// Its own fork: only as reviewed.
	if rr := e.send(key, "POST", "/api/v1/forks/"+fork+"/merge", `{}`); rr.Code != http.StatusBadRequest {
		t.Errorf("merging without a digest: %d %s", rr.Code, rr.Body.String())
	}
	if rr := e.merge(key, fork, strings.Repeat("0", 64)); rr.Code != http.StatusConflict {
		t.Errorf("merging with a stale digest: %d %s", rr.Code, rr.Body.String())
	}
	if e.liveTitle(t) != "Live" {
		t.Fatal("a refused merge changed the live page")
	}
	rr := e.merge(key, fork, e.digest(t, key, fork))
	if rr.Code != http.StatusOK {
		t.Fatalf("merging its own reviewed fork: %d %s", rr.Code, rr.Body.String())
	}
	if e.liveTitle(t) != "From the key" {
		t.Errorf("live title %q", e.liveTitle(t))
	}

	// The audit entry says it was a self-merge and what it touched.
	deadline := time.Now().Add(5 * time.Second)
	for {
		var entry models.AuditLog
		err := e.db.FindOne(context.Background(), "audit_logs", bson.M{"action": "fork.merge", "resource_id": fork}, &entry)
		if err == nil {
			if entry.Details["self_merge"] != true || entry.Details["updated_ids"] == nil {
				t.Errorf("audit details: %+v", entry.Details)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("no audit entry: %v", err)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func TestMerge_ASandboxKeyWithTheGrantMergesItsOwnSandbox(t *testing.T) {
	e := newSandboxEnvAs(t, models.RoleAdmin)
	key := e.key(t, "Agent", true)
	e.allowMerge(t, key, true)

	fork := e.startSandbox(t, key)
	e.editInFork(t, key, fork, "Merged by the agent")
	if rr := e.merge(key, fork, e.digest(t, key, fork)); rr.Code != http.StatusOK {
		t.Fatalf("merge: %d %s", rr.Code, rr.Body.String())
	}
	if e.liveTitle(t) != "Merged by the agent" {
		t.Errorf("live title %q", e.liveTitle(t))
	}
	state := decode(t, e.send(key, "GET", "/api/v1/sandbox", ``))
	if state["active"] != false || state["restricted"] != true {
		t.Errorf("after merging its sandbox: %+v", state)
	}
	// Still a Sandbox key: no live edits.
	if rr := e.send(key, "PUT", "/api/v1/content/"+e.live.Hex(), `{"title":"Direct"}`); rr.Code != http.StatusForbidden {
		t.Errorf("live edit after the merge: %d %s", rr.Code, rr.Body.String())
	}
}

func TestMerge_TheGrantNeedsARoleThatCanMerge(t *testing.T) {
	e := newSandboxEnvAs(t, models.RoleEditor)
	key := e.key(t, "Editor's agent", false)
	e.allowMerge(t, key, false) // set directly; the admin pages refuse this
	fork := e.forkWithEdit(t, key, "Editor's change")
	if rr := e.merge(key, fork, e.digest(t, key, fork)); rr.Code != http.StatusForbidden {
		t.Errorf("an editor's key merging: %d %s", rr.Code, rr.Body.String())
	}
}

func TestSubmit_FreezesTheForkForKeysButNotForPeople(t *testing.T) {
	e := newSandboxEnv(t)
	agent := e.key(t, "Agent", true)
	full := e.key(t, "Full", false)

	fork := e.startSandbox(t, agent)
	rr := e.send(agent, "POST", "/api/v1/forks/"+fork+"/fork-page", `{"path":"/live"}`)
	if rr.Code != http.StatusOK {
		t.Fatalf("fork-page: %d %s", rr.Code, rr.Body.String())
	}
	copyID := decode(t, rr)["id"].(string)
	if rr := e.send(agent, "PUT", "/api/v1/content/"+copyID, `{"title":"Before submit"}`); rr.Code != http.StatusOK {
		t.Fatalf("edit before submit: %d %s", rr.Code, rr.Body.String())
	}
	if rr := e.send(agent, "POST", "/api/v1/sandbox/end", `{"action":"submit"}`); rr.Code != http.StatusOK {
		t.Fatalf("submit: %d %s", rr.Code, rr.Body.String())
	}

	for name, key := range map[string]string{"the agent": agent, "a Full key": full} {
		if rr := e.send(key, "PUT", "/api/v1/content/"+copyID, `{"title":"After submit"}`); rr.Code != http.StatusConflict {
			t.Errorf("%s editing a submitted fork: %d %s", name, rr.Code, rr.Body.String())
		}
	}
	// Every other way a key changes a page copy is frozen too.
	if rr := e.send(full, "POST", "/api/v1/content/"+copyID+"/versions/1/revert", `{}`); rr.Code != http.StatusConflict {
		t.Errorf("reverting a copy in a submitted fork: %d %s", rr.Code, rr.Body.String())
	}
	if rr := e.send(full, "DELETE", "/api/v1/content/"+copyID, ``); rr.Code != http.StatusConflict {
		t.Errorf("deleting a copy in a submitted fork: %d %s", rr.Code, rr.Body.String())
	}
	if rr := e.send(full, "POST", "/api/v1/content/"+copyID+"/schedule", `{"publish_at":"2030-01-01T00:00:00Z"}`); rr.Code != http.StatusBadRequest {
		t.Errorf("scheduling a fork copy: %d %s", rr.Code, rr.Body.String())
	}
	if rr := e.send(agent, "DELETE", "/api/v1/forks/"+fork, ``); rr.Code != http.StatusConflict {
		t.Errorf("the agent discarding its submitted fork: %d %s", rr.Code, rr.Body.String())
	}

	// A person (browser session) can still edit; the review digest changes.
	req := httptest.NewRequest("PUT", "/api/v1/content/"+copyID, strings.NewReader(`{"title":"Reviewer's tweak"}`))
	req = mux.SetURLVars(req.WithContext(middleware.InjectAPIUser(req.Context(), &auth.SessionUser{ID: e.owner.ID.Hex(), Email: e.owner.Email, Role: models.RoleEditor})), map[string]string{"id": copyID})
	out := httptest.NewRecorder()
	e.a.APIUpdateContent(out, req)
	if out.Code != http.StatusOK {
		t.Errorf("a person editing a submitted fork: %d %s", out.Code, out.Body.String())
	}
}

// --- the review page -------------------------------------------------------

func TestForkPage_ShowsTheReviewAndMergesOnlyWhatWasShown(t *testing.T) {
	h, cleanup := newTestHandler(t)
	defer cleanup()
	ctx := context.Background()
	router := adminRouter(h)

	tmpl := seedTemplate(t, h.db, "Page", "page")
	live := seedContent(t, h.db, tmpl, "Live", "live-reviewed", "/live-reviewed")
	fork, err := h.forkService.Create(ctx, "reviewed", "", primitive.NewObjectID(), "agent@example.com")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	copyPage, err := h.forkService.ForkPage(ctx, fork.ID, live)
	if err != nil {
		t.Fatalf("ForkPage: %v", err)
	}
	longBody := strings.Repeat("x", 2400) + "LATE CHANGE"
	if err := h.db.UpdateOne(ctx, "content", bson.M{"_id": copyPage.ID}, bson.M{"$set": bson.M{"title": "Reviewed title", "data": bson.M{"body": longBody}}}); err != nil {
		t.Fatalf("edit copy: %v", err)
	}

	page := routed(router, "admin", "GET", "/cm/forks/"+fork.ID.Hex(), nil).Body.String()
	for _, want := range []string{"Changes to review", "Reviewed title", "LATE CHANGE", `name="digest"`, "Show all"} {
		if !strings.Contains(page, want) {
			t.Errorf("review page lacks %q", want)
		}
	}
	if strings.Contains(page, "cannot be undone") {
		t.Error("the merge confirmation still says it cannot be undone")
	}
	review, _ := h.forkService.Review(ctx, fork.ID)
	if !strings.Contains(page, review.Digest) {
		t.Error("the merge form does not carry the review's digest")
	}

	// Edited after the page was shown: nothing is merged.
	if err := h.db.UpdateOne(ctx, "content", bson.M{"_id": copyPage.ID}, bson.M{"$set": bson.M{"title": "Swapped"}}); err != nil {
		t.Fatalf("edit copy: %v", err)
	}
	rr := routed(router, "admin", "POST", "/cm/forks/"+fork.ID.Hex()+"/merge", url.Values{"digest": {review.Digest}})
	if !strings.Contains(rr.Body.String(), "was edited after you opened it") {
		t.Errorf("stale merge: %d %s", rr.Code, rr.Body.String())
	}
	var livePage models.Content
	h.db.FindOne(ctx, "content", bson.M{"_id": live}, &livePage)
	if livePage.Title != "Live" {
		t.Fatalf("a stale merge changed the live page to %q", livePage.Title)
	}

	review, _ = h.forkService.Review(ctx, fork.ID)
	routed(router, "admin", "POST", "/cm/forks/"+fork.ID.Hex()+"/merge", url.Values{"digest": {review.Digest}})
	h.db.FindOne(ctx, "content", bson.M{"_id": live}, &livePage)
	if livePage.Title != "Swapped" {
		t.Errorf("after merging as reviewed: %q", livePage.Title)
	}
}

func TestSubmit_HandsTheForkToAPerson(t *testing.T) {
	e := newSandboxEnvAs(t, models.RoleAdmin)
	agent := e.key(t, "Agent", true)
	e.allowMerge(t, agent, true)
	admin := e.key(t, "Admin's Full key", false)

	fork := e.startSandbox(t, agent)
	e.editInFork(t, agent, fork, "Submitted change")
	if rr := e.send(agent, "POST", "/api/v1/sandbox/end", `{"action":"submit"}`); rr.Code != http.StatusOK {
		t.Fatalf("submit: %d %s", rr.Code, rr.Body.String())
	}

	// The agent handed it off: it can't merge it any more, even with the permission.
	if rr := e.merge(agent, fork, e.digest(t, agent, fork)); rr.Code != http.StatusForbidden || !strings.Contains(rr.Body.String(), "submitted for review") {
		t.Errorf("self-merging a submitted fork: %d %s", rr.Code, rr.Body.String())
	}
	// No key archives or deletes it either; a person does.
	if rr := e.send(admin, "POST", "/api/v1/forks/"+fork+"/archive", ``); rr.Code != http.StatusConflict {
		t.Errorf("a key archiving a submitted fork: %d %s", rr.Code, rr.Body.String())
	}
	if rr := e.send(admin, "DELETE", "/api/v1/forks/"+fork, ``); rr.Code != http.StatusConflict {
		t.Errorf("a key deleting a submitted fork: %d %s", rr.Code, rr.Body.String())
	}
	if e.liveTitle(t) != "Live" {
		t.Errorf("live title %q", e.liveTitle(t))
	}
}
