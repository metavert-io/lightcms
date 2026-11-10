package handlers

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/metavert-io/lightcms/v7/internal/models"
)

// Tests for the 7.4.1 follow-ups: search-and-replace and by-path lookups
// never land on fork copies, and scoped operations report fork copies named
// in content_ids as skipped.

func TestAPISearchReplace_LeavesForkCopiesAlone(t *testing.T) {
	a, db, cleanup := newTestAPIHandler(t)
	defer cleanup()

	forkID := createTestFork(t, db, "sr")
	body := func(s string) map[string]interface{} { return map[string]interface{}{"body": s} }
	live := seedPage(t, db, models.Content{Title: "Live", FullPath: "/sr-page", Data: body("a wombat lives here")})
	forkCopy := seedPage(t, db, models.Content{Title: "Copy", FullPath: "/sr-page", ForkID: &forkID, Data: body("a wombat in the fork")})
	forkOnly := seedPage(t, db, models.Content{Title: "wombat draft", FullPath: "/sr-new", ForkID: &forkID, Data: body("new wombat page")})

	assertCopiesUntouched := func(when string) {
		t.Helper()
		if c := loadPage(t, db, forkCopy); c.Data["body"] != "a wombat in the fork" {
			t.Errorf("%s: fork copy rewritten: %v", when, c.Data["body"])
		}
		if c := loadPage(t, db, forkOnly); c.Data["body"] != "new wombat page" || c.Title != "wombat draft" {
			t.Errorf("%s: fork-only page rewritten: %q %v", when, c.Title, c.Data["body"])
		}
	}

	// Global preview and execute see the live page only.
	rr, out := apiCall(t, a.APISearchReplacePreview, "POST", "/x", `{"search":"wombat","replace":"quoll"}`, nil)
	if rr.Code != 200 {
		t.Fatalf("preview: %d %s", rr.Code, rr.Body.String())
	}
	if m := idSet(out["matches"]); len(m) != 1 || m[live.Hex()] == nil {
		t.Errorf("preview matches = %v, want only the live page", out["matches"])
	}
	rr, out = apiCall(t, a.APISearchReplaceExecute, "POST", "/x", `{"search":"wombat","replace":"quoll"}`, nil)
	if rr.Code != 200 {
		t.Fatalf("execute: %d %s", rr.Code, rr.Body.String())
	}
	if u := idSet(out["updated_pages"]); len(u) != 1 || u[live.Hex()] == nil {
		t.Errorf("execute updated_pages = %v, want only the live page", out["updated_pages"])
	}
	if c := loadPage(t, db, live); c.Data["body"] != "a quoll lives here" {
		t.Errorf("live page not rewritten: %v", c.Data["body"])
	}
	assertCopiesUntouched("global execute")

	// Scoped runs with the copies named explicitly: live page handled, the
	// copies reported as skipped rather than silently dropped.
	scope := `"scope":{"content_ids":["` + live.Hex() + `","` + forkCopy.Hex() + `","` + forkOnly.Hex() + `"]}`
	assertSkipped := func(when string, out map[string]interface{}) {
		t.Helper()
		sk := idSet(out["skipped"])
		for _, id := range []string{forkCopy.Hex(), forkOnly.Hex()} {
			if s := sk[id]; s == nil || !strings.Contains(s["reason"].(string), "fork copy") {
				t.Errorf("%s: %s not reported as a skipped fork copy: %v", when, id, out["skipped"])
			}
		}
		if len(sk) != 2 {
			t.Errorf("%s: skipped = %v, want exactly the two fork copies", when, out["skipped"])
		}
	}
	rr, out = apiCall(t, a.APIScopedSearchReplacePreview, "POST", "/x", `{"search":"quoll","replace":"numbat",`+scope+`}`, nil)
	if rr.Code != 200 {
		t.Fatalf("scoped preview: %d %s", rr.Code, rr.Body.String())
	}
	if m := idSet(out["matches"]); len(m) != 1 || m[live.Hex()] == nil {
		t.Errorf("scoped preview matches = %v, want only the live page", out["matches"])
	}
	assertSkipped("scoped preview", out)

	// "wombat" now exists only in the copies: a scoped execute must change nothing.
	rr, out = apiCall(t, a.APIScopedSearchReplaceExecute, "POST", "/x", `{"search":"wombat","replace":"numbat",`+scope+`}`, nil)
	if rr.Code != 200 {
		t.Fatalf("scoped execute: %d %s", rr.Code, rr.Body.String())
	}
	if out["pages_modified"] != float64(0) {
		t.Errorf("scoped execute modified %v pages, want 0", out["pages_modified"])
	}
	assertSkipped("scoped execute", out)
	assertCopiesUntouched("scoped execute")

	// A scope without fork copies reports an empty skipped list.
	rr, out = apiCall(t, a.APIScopedSearchReplacePreview, "POST", "/x", `{"search":"quoll","replace":"numbat","scope":{"content_ids":["`+live.Hex()+`"]}}`, nil)
	if sk, ok := out["skipped"].([]interface{}); rr.Code != 200 || !ok || len(sk) != 0 {
		t.Errorf("scoped preview without copies: %d skipped=%v, want []", rr.Code, out["skipped"])
	}
}

func TestAPIContentByPath_ResolvesLivePagesOnly(t *testing.T) {
	a, db, cleanup := newTestAPIHandler(t)
	defer cleanup()

	tmplID := seedTemplate(t, db, "Page", "page")
	forkID := createTestFork(t, db, "bp")
	ownedBySandboxKey(t, db, forkID)
	otherFork := createTestFork(t, db, "bp-other")
	// Copies are seeded first so an unfiltered lookup could return them.
	forkCopy := seedPage(t, db, models.Content{TemplateID: tmplID, Title: "Copy", FullPath: "/bp-page", ForkID: &forkID})
	live := seedPage(t, db, models.Content{TemplateID: tmplID, Title: "Live", FullPath: "/bp-page"})
	forkOnly := seedPage(t, db, models.Content{TemplateID: tmplID, Title: "Fork Only", FullPath: "/bp-new", ForkID: &forkID})

	// Live page wins on read and write.
	rr, out := apiCall(t, a.APIGetContentByPath, "GET", "/api/v1/content/by-path?path=/bp-page", "", nil)
	if rr.Code != 200 || out["id"] != live.Hex() {
		t.Errorf("get by path: %d id=%v, want live %s", rr.Code, out["id"], live.Hex())
	}
	rr, _ = apiCall(t, a.APIUpdateContentByPath, "PUT", "/api/v1/content/by-path?path=/bp-page", `{"title":"Live Edited"}`, nil)
	if rr.Code != 200 {
		t.Fatalf("update by path: %d %s", rr.Code, rr.Body.String())
	}
	if c := loadPage(t, db, live); c.Title != "Live Edited" {
		t.Errorf("live title = %q, want Live Edited", c.Title)
	}
	if c := loadPage(t, db, forkCopy); c.Title != "Copy" {
		t.Errorf("fork copy title = %q, want it untouched", c.Title)
	}

	// A path that exists only inside a fork is not found without fork_id.
	rr, _ = apiCall(t, a.APIGetContentByPath, "GET", "/api/v1/content/by-path?path=/bp-new", "", nil)
	if rr.Code != 404 {
		t.Errorf("get fork-only path: %d, want 404 (%s)", rr.Code, rr.Body.String())
	}
	rr, _ = apiCall(t, a.APIUpdateContentByPath, "PUT", "/api/v1/content/by-path?path=/bp-new", `{"title":"Hijacked"}`, nil)
	if rr.Code != 404 {
		t.Errorf("update fork-only path: %d, want 404 (%s)", rr.Code, rr.Body.String())
	}
	if c := loadPage(t, db, forkOnly); c.Title != "Fork Only" {
		t.Errorf("fork-only page was edited through a bare path: %q", c.Title)
	}

	// Naming the fork reaches its copies.
	q := "&fork_id=" + forkID.Hex()
	rr, out = apiCall(t, a.APIGetContentByPath, "GET", "/api/v1/content/by-path?path=/bp-page"+q, "", nil)
	if rr.Code != 200 || out["id"] != forkCopy.Hex() {
		t.Errorf("get by path in fork: %d id=%v, want copy %s", rr.Code, out["id"], forkCopy.Hex())
	}
	rr, _ = apiCall(t, a.APIUpdateContentByPath, "PUT", "/api/v1/content/by-path?path=/bp-new"+q, `{"title":"Fork Edited"}`, nil)
	if rr.Code != 200 {
		t.Errorf("update by path in fork: %d %s", rr.Code, rr.Body.String())
	}
	if c := loadPage(t, db, forkOnly); c.Title != "Fork Edited" {
		t.Errorf("fork-only title = %q, want Fork Edited", c.Title)
	}
	// Wrong fork and malformed fork_id.
	rr, _ = apiCall(t, a.APIGetContentByPath, "GET", "/api/v1/content/by-path?path=/bp-new&fork_id="+otherFork.Hex(), "", nil)
	if rr.Code != 404 {
		t.Errorf("get by path in the wrong fork: %d, want 404", rr.Code)
	}
	rr, _ = apiCall(t, a.APIGetContentByPath, "GET", "/api/v1/content/by-path?path=/bp-new&fork_id=nope", "", nil)
	if rr.Code != 400 {
		t.Errorf("get by path with bad fork_id: %d, want 400", rr.Code)
	}

	// Sandbox-only keys: live by path stays forbidden, a bare fork-only path
	// is not found, and the fork's copy is editable when the fork is named.
	rec := httptest.NewRecorder()
	a.APIUpdateContentByPath(rec, sandboxOnlyReq("PUT", "/api/v1/content/by-path?path=/bp-page", strings.NewReader(`{"title":"Hack"}`)))
	if rec.Code != 403 {
		t.Errorf("sandbox-only live by path: %d, want 403", rec.Code)
	}
	rec = httptest.NewRecorder()
	a.APIUpdateContentByPath(rec, sandboxOnlyReq("PUT", "/api/v1/content/by-path?path=/bp-new", strings.NewReader(`{"title":"Hack"}`)))
	if rec.Code != 404 {
		t.Errorf("sandbox-only bare fork-only path: %d, want 404", rec.Code)
	}
	rec = httptest.NewRecorder()
	a.APIUpdateContentByPath(rec, sandboxOnlyReq("PUT", "/api/v1/content/by-path?path=/bp-page"+q, strings.NewReader(`{"title":"Copy Edited"}`)))
	if rec.Code != 200 {
		t.Errorf("sandbox-only copy by path: %d %s", rec.Code, rec.Body.String())
	}
	if c := loadPage(t, db, forkCopy); c.Title != "Copy Edited" {
		t.Errorf("fork copy title = %q, want Copy Edited", c.Title)
	}
	if c := loadPage(t, db, live); c.Title != "Live Edited" {
		t.Errorf("live page changed by a sandbox-only key: %q", c.Title)
	}
}

// The agent sandbox maps a path to its fork copy through fork-page (MCP
// sandboxTargetForPath): that mapping must keep working for pages forked from
// live and for pages created inside the sandbox, which have no live page.
func TestAPIForkPage_ByPath_SandboxMapping(t *testing.T) {
	a, db, cleanup := newTestAPIHandler(t)
	defer cleanup()

	forkID := createTestFork(t, db, "sbx")
	otherFork := createTestFork(t, db, "sbx-other")
	live := seedPage(t, db, models.Content{Title: "Live", FullPath: "/sbx-page", Published: true})
	forkOnly := seedPage(t, db, models.Content{Title: "New In Sandbox", FullPath: "/sbx-new", ForkID: &forkID})
	seedPage(t, db, models.Content{Title: "Elsewhere", FullPath: "/sbx-elsewhere", ForkID: &otherFork})
	vars := map[string]string{"id": forkID.Hex()}

	// Live page: copy-on-write, idempotent on repeat.
	rr, out := apiCall(t, a.APIForkPage, "POST", "/x", `{"path":"/sbx-page"}`, vars)
	if rr.Code != 200 {
		t.Fatalf("fork-page live path: %d %s", rr.Code, rr.Body.String())
	}
	copyID, _ := out["id"].(string)
	if copyID == "" || copyID == live.Hex() {
		t.Fatalf("fork-page returned %q, want a new copy", copyID)
	}
	rr, out = apiCall(t, a.APIForkPage, "POST", "/x", `{"path":"/sbx-page"}`, vars)
	if rr.Code != 200 || out["id"] != copyID {
		t.Errorf("fork-page repeat: %d id=%v, want the same copy %s", rr.Code, out["id"], copyID)
	}

	// Page created inside the sandbox: the path maps to that copy.
	rr, out = apiCall(t, a.APIForkPage, "POST", "/x", `{"path":"/sbx-new"}`, vars)
	if rr.Code != 200 || out["id"] != forkOnly.Hex() {
		t.Errorf("fork-page sandbox-created path: %d id=%v, want %s (%s)", rr.Code, out["id"], forkOnly.Hex(), rr.Body.String())
	}

	// A path that exists only in another fork is not reachable.
	rr, _ = apiCall(t, a.APIForkPage, "POST", "/x", `{"path":"/sbx-elsewhere"}`, vars)
	if rr.Code != 404 {
		t.Errorf("fork-page path from another fork: %d, want 404 (%s)", rr.Code, rr.Body.String())
	}
}

func TestAPIBulkFieldOpAndExport_ReportForkCopies(t *testing.T) {
	a, db, cleanup := newTestAPIHandler(t)
	defer cleanup()

	forkID := createTestFork(t, db, "bf")
	data := map[string]interface{}{"body": "x"}
	live := seedPage(t, db, models.Content{Title: "Live", FullPath: "/bf-page", Data: data})
	forkCopy := seedPage(t, db, models.Content{Title: "Copy", FullPath: "/bf-page", ForkID: &forkID, Data: data})
	ids := `["` + live.Hex() + `","` + forkCopy.Hex() + `"]`

	assertSkipped := func(when string, out map[string]interface{}) {
		t.Helper()
		sk := idSet(out["skipped"])
		if s := sk[forkCopy.Hex()]; len(sk) != 1 || s == nil || !strings.Contains(s["reason"].(string), "fork copy") {
			t.Errorf("%s: skipped = %v, want the fork copy with a reason", when, out["skipped"])
		}
	}

	for _, dry := range []string{"true", "false"} {
		rr, out := apiCall(t, a.APIBulkFieldOperation, "POST", "/x",
			`{"operation":"append","field":"body","value":"y","dry_run":`+dry+`,"scope":{"content_ids":`+ids+`}}`, nil)
		if rr.Code != 200 {
			t.Fatalf("bulk field op dry_run=%s: %d %s", dry, rr.Code, rr.Body.String())
		}
		if res := idSet(out["results"]); len(res) != 1 || res[live.Hex()] == nil {
			t.Errorf("bulk field op dry_run=%s results = %v, want only the live page", dry, out["results"])
		}
		assertSkipped("bulk field op dry_run="+dry, out)
	}
	if c := loadPage(t, db, live); c.Data["body"] != "xy" {
		t.Errorf("live body = %v, want xy", c.Data["body"])
	}
	if c := loadPage(t, db, forkCopy); c.Data["body"] != "x" {
		t.Errorf("fork copy body = %v, want it untouched", c.Data["body"])
	}

	rr, out := apiCall(t, a.APIExportContent, "POST", "/x", `{"content_ids":`+ids+`}`, nil)
	if rr.Code != 200 {
		t.Fatalf("export: %d %s", rr.Code, rr.Body.String())
	}
	if items := idSet(out["items"]); len(items) != 1 || items[live.Hex()] == nil {
		t.Errorf("export items = %v, want only the live page", out["items"])
	}
	assertSkipped("export", out)

	// No fork copies requested → empty list, not null.
	rr, out = apiCall(t, a.APIExportContent, "POST", "/x", `{"content_ids":["`+live.Hex()+`"]}`, nil)
	if sk, ok := out["skipped"].([]interface{}); rr.Code != 200 || !ok || len(sk) != 0 {
		t.Errorf("export without copies: %d skipped=%v, want []", rr.Code, out["skipped"])
	}
}
