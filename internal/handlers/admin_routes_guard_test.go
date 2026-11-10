package handlers

import (
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/gorilla/mux"
	"github.com/gorilla/sessions"

	"github.com/metavert-io/lightcms/v7/internal/auth"
)

// Guards for the /cm route table (7.4.2). Before it, 34 state-changing admin
// routes only checked that someone was signed in, so a viewer could create
// templates, save the theme or revoke anyone's API key. Now every /cm route
// is declared in AdminRoutes with the access it needs and registered with
// the check in front of the handler; these tests keep that true. They need
// no database.

// routesTestHandler is a Handler with only what the access check uses.
func routesTestHandler() *Handler {
	store := sessions.NewCookieStore([]byte(testSessionSecret))
	return &Handler{auth: auth.NewManager(store, nil, nil)}
}

// writeRoutesWithoutPermission is the complete list of non-GET /cm routes
// that are reachable without a permission, each with the reason. Anything
// else that changes state must name a permission in AdminRoutes.
var writeRoutesWithoutPermission = map[string]string{
	"POST /cm/login":           "public: signing in is what creates the session; rate-limited by IP",
	"POST /cm/logout":          "public: only clears the caller's own session cookie",
	"POST /cm/change-password": "session: the forced change of the signed-in user's own password",
	"POST /cm/security":        "session: changes the signed-in user's own password and requires the current one",

	"POST /cm/connected-apps/{id}/access": "session: a viewer can connect an app too; the handler refuses another person's connection below apikey.manage_all, and merging needs fork.merge",
	"POST /cm/connected-apps/{id}/revoke": "session: anyone may disconnect their own app; the handler refuses another person's below apikey.manage_all",
}

var routeVarRe = regexp.MustCompile(`\{[^}]+\}`)

func routeKey(method, path string) string { return method + " /cm" + path }

// A route cannot be registered without saying who may call it, and a route
// that changes state names a permission unless it is on the allowlist above.
func TestAdminRoutes_EveryWriteRouteIsGuarded(t *testing.T) {
	h := routesTestHandler()
	table := map[string]AdminRoute{}
	for _, rt := range h.AdminRoutes() {
		key := routeKey(rt.Method, rt.Path)
		if _, dup := table[key]; dup {
			t.Errorf("%s is declared twice", key)
		}
		table[key] = rt

		kinds := 0
		if len(rt.Perms) > 0 {
			kinds++
		}
		if rt.Session != "" {
			kinds++
		}
		if rt.Public != "" {
			kinds++
		}
		if kinds != 1 {
			t.Errorf("%s must declare exactly one of Perms, Session or Public (has %d)", key, kinds)
		}
		for _, p := range rt.Perms {
			if !auth.IsKnownPermission(p) {
				t.Errorf("%s names unknown permission %q", key, p)
			}
		}
		if rt.handler == nil {
			t.Errorf("%s has no handler", key)
		}

		if rt.Method == http.MethodGet {
			continue
		}
		reason, allowed := writeRoutesWithoutPermission[key]
		switch {
		case len(rt.Perms) == 0 && !allowed:
			t.Errorf("%s changes state without a permission check: give it a permission in AdminRoutes, or add it to writeRoutesWithoutPermission with the reason", key)
		case len(rt.Perms) > 0 && allowed:
			t.Errorf("%s now requires %v: remove it from writeRoutesWithoutPermission (%s)", key, rt.Perms, reason)
		}
	}
	for key := range writeRoutesWithoutPermission {
		if _, ok := table[key]; !ok {
			t.Errorf("writeRoutesWithoutPermission lists %s, which is not a route", key)
		}
	}

	// The registered router is exactly the table: walk it
	r := mux.NewRouter()
	h.RegisterAdminRoutes(r.PathPrefix("/cm").Subrouter())
	walked := map[string]bool{}
	err := r.Walk(func(route *mux.Route, _ *mux.Router, _ []*mux.Route) error {
		tpl, err := route.GetPathTemplate()
		if err != nil {
			return nil
		}
		methods, err := route.GetMethods()
		if err != nil {
			if tpl == "/cm" {
				return nil // the subrouter itself
			}
			t.Errorf("route %s is registered for every method", tpl)
			return nil
		}
		for _, m := range methods {
			walked[m+" "+tpl] = true
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk: %v", err)
	}
	for key := range walked {
		if _, ok := table[key]; !ok {
			t.Errorf("registered route %s is not in AdminRoutes", key)
		}
	}
	for key := range table {
		if !walked[key] {
			t.Errorf("AdminRoutes entry %s was not registered", key)
		}
	}
	// (139 routes as of 7.4.3, which removed /upload and the two lock routes)
	if len(table) < 135 {
		t.Fatalf("only %d admin routes declared; the table looks truncated", len(table))
	}

	// main.go registers /cm routes through the table and no other way
	src, err := os.ReadFile("../../cmd/server/main.go")
	if err != nil {
		t.Fatalf("read main.go: %v", err)
	}
	if !strings.Contains(string(src), "h.RegisterAdminRoutes(admin)") {
		t.Error("cmd/server/main.go no longer calls h.RegisterAdminRoutes(admin)")
	}
	if m := regexp.MustCompile(`\badmin\.(HandleFunc|Handle|Path|Methods|PathPrefix)\(`).FindString(string(src)); m != "" {
		t.Errorf("cmd/server/main.go registers an admin route directly (%s...): declare it in handlers.AdminRoutes", m)
	}
}

// adminRoleMatrix says, for every /cm route, which roles reach the handler:
// v(iewer) c(ontributor) e(ditor) a(dmin), "-" for refused. It is written
// out by hand, independently of the permissions in AdminRoutes, so loosening
// or tightening a route there fails here until the change is made on purpose.
// "*" marks the session/public routes (every signed-in role).
var adminRoleMatrix = map[string]string{
	"GET /cm/login":              "*",
	"POST /cm/login":             "*",
	"POST /cm/logout":            "*",
	"GET /cm/change-password":    "*",
	"POST /cm/change-password":   "*",
	"GET /cm":                    "*",
	"GET /cm/":                   "*",
	"GET /cm/security":           "*",
	"POST /cm/security":          "*",
	"GET /cm/forks/exit-preview": "*",

	"GET /cm/connected-apps":              "*",
	"POST /cm/connected-apps/{id}/access": "*",
	"POST /cm/connected-apps/{id}/revoke": "*",
	"POST /cm/api-keys/{id}/access":       "-cea",

	// Templates and snippets: everyone reads, only admin writes
	"GET /cm/templates":              "vcea",
	"GET /cm/templates/new":          "---a",
	"POST /cm/templates/new":         "---a",
	"GET /cm/templates/{id}":         "vcea",
	"POST /cm/templates/{id}":        "---a",
	"POST /cm/templates/{id}/delete": "---a",
	"GET /cm/snippets":               "vcea",
	"GET /cm/snippets/new":           "---a",
	"POST /cm/snippets/new":          "---a",
	"GET /cm/snippets/{id}":          "vcea",
	"POST /cm/snippets/{id}":         "---a",
	"POST /cm/snippets/{id}/delete":  "---a",

	// Content: contributors create and save drafts (UpdateContent narrows
	// "c" to drafts); editors and admins do the rest
	"GET /cm/content":                                             "vcea",
	"GET /cm/content/new":                                         "-cea",
	"GET /cm/content/new/{templateID}":                            "-cea",
	"POST /cm/content/create":                                     "-cea",
	"GET /cm/content/{id}":                                        "vcea",
	"POST /cm/content/{id}":                                       "-cea",
	"POST /cm/content/{id}/delete":                                "--ea",
	"POST /cm/content/{id}/undelete":                              "--ea",
	"POST /cm/content/{id}/regenerate":                            "--ea",
	"GET /cm/content/{id}/change-template/{template_id}":          "--ea",
	"POST /cm/content/{id}/change-template/{template_id}/confirm": "--ea",
	"GET /cm/content/{id}/versions":                               "vcea",
	"GET /cm/content/{id}/versions/{version}/view":                "vcea",
	"GET /cm/content/{id}/versions/{version}/diff":                "vcea",
	"POST /cm/content/{id}/versions/{version}/revert":             "--ea",

	// Settings: collections, theme, config, folders, redirects
	"GET /cm/collections":                      "vcea",
	"GET /cm/collections/new":                  "---a",
	"POST /cm/collections/new":                 "---a",
	"GET /cm/collections/{id}":                 "vcea",
	"POST /cm/collections/{id}":                "---a",
	"POST /cm/collections/{id}/delete":         "---a",
	"GET /cm/theme":                            "vcea",
	"POST /cm/theme":                           "---a",
	"GET /cm/theme/versions":                   "vcea",
	"GET /cm/theme/versions/{version}":         "vcea",
	"POST /cm/theme/versions/{version}/revert": "---a",
	"GET /cm/config":                           "vcea",
	"POST /cm/config":                          "---a",
	"GET /cm/folders":                          "vcea",
	"GET /cm/folders/new":                      "---a",
	"POST /cm/folders/new":                     "---a",
	"GET /cm/folders/{id}":                     "vcea",
	"POST /cm/folders/{id}":                    "---a",
	"POST /cm/folders/{id}/delete":             "---a",
	"GET /cm/redirects":                        "vcea",
	"GET /cm/redirects/new":                    "---a",
	"POST /cm/redirects/new":                   "---a",
	"GET /cm/redirects/{id}":                   "vcea",
	"POST /cm/redirects/{id}":                  "---a",
	"POST /cm/redirects/{id}/delete":           "---a",

	// Assets
	"GET /cm/assets":              "vcea",
	"GET /cm/assets/upload":       "-cea",
	"POST /cm/assets/upload":      "-cea",
	"POST /cm/assets/{id}/delete": "--ea",

	// Inbox
	"GET /cm/messages":                "vcea",
	"POST /cm/messages/mark-all-read": "--ea",
	"GET /cm/messages/{id}":           "vcea",
	"POST /cm/messages/{id}/delete":   "--ea",

	// API keys (own keys; DeleteAPIKey checks ownership), approvals
	"GET /cm/api-keys":              "-cea",
	"GET /cm/api-keys/new":          "-cea",
	"POST /cm/api-keys/new":         "-cea",
	"POST /cm/api-keys/{id}/delete": "-cea",
	"GET /cm/approvals":             "-cea",

	// Admin only: users, audit, analytics
	"GET /cm/users":                        "---a",
	"GET /cm/users/new":                    "---a",
	"POST /cm/users/new":                   "---a",
	"GET /cm/users/{id}":                   "---a",
	"POST /cm/users/{id}":                  "---a",
	"POST /cm/users/{id}/toggle-disabled":  "---a",
	"POST /cm/users/{id}/reset-password":   "---a",
	"GET /cm/audit":                        "---a",
	"POST /cm/audit/ratelimits/{ip}/clear": "---a",
	"GET /cm/analytics":                    "---a",
	"GET /cm/analytics/page":               "---a",
	"GET /cm/analytics/referrer":           "---a",
	"GET /cm/analytics/ai":                 "---a",

	// Webhooks and imports: editor and admin
	"GET /cm/webhooks/docs":                    "--ea",
	"GET /cm/webhooks/new":                     "--ea",
	"GET /cm/webhooks":                         "--ea",
	"POST /cm/webhooks":                        "--ea",
	"GET /cm/webhooks/{id}/edit":               "--ea",
	"GET /cm/webhooks/{id}/deliveries":         "--ea",
	"POST /cm/webhooks/{id}/regenerate-secret": "--ea",
	"POST /cm/webhooks/{id}":                   "--ea",
	"POST /cm/webhooks/{id}/delete":            "--ea",
	"GET /cm/imports":                          "--ea",
	"GET /cm/imports/sources/new":              "--ea",
	"POST /cm/imports/sources":                 "--ea",
	"GET /cm/imports/sources/{id}/edit":        "--ea",
	"POST /cm/imports/sources/{id}":            "--ea",
	"POST /cm/imports/sources/{id}/delete":     "--ea",
	"POST /cm/imports/sources/{id}/trigger":    "--ea",
	"GET /cm/imports/markdown":                 "--ea",
	"POST /cm/imports/markdown":                "--ea",
	"GET /cm/imports/csv":                      "--ea",
	"POST /cm/imports/csv":                     "--ea",
	"GET /cm/imports/{id}/stream":              "--ea",
	"GET /cm/imports/{id}":                     "--ea",

	// Tools
	"GET /cm/replace/preview":         "---a",
	"POST /cm/replace/execute":        "---a",
	"GET /cm/tools/broken-links":      "--ea",
	"POST /cm/tools/broken-links/fix": "--ea",
	"GET /cm/tools/search":            "vcea",
	"GET /cm/tools/search/test":       "vcea",
	"POST /cm/tools/search/reindex":   "---a",
	"POST /cm/tools/search/config":    "---a",
	"GET /cm/copilot":                 "--ea",
	"POST /cm/copilot/chat":           "--ea",
	"GET /cm/tools/agent":             "---a",
	"POST /cm/tools/agent/config":     "---a",
	"POST /cm/tools/agent/test":       "---a",
	"GET /cm/tools/indexnow":          "---a",
	"POST /cm/tools/indexnow":         "---a",
	"GET /cm/tools/seo":               "---a",
	"POST /cm/tools/seo":              "---a",
	"GET /cm/tools/chat":              "vcea",
	"POST /cm/tools/chat/config":      "---a",

	// Forks: editors work in them, admins merge
	"GET /cm/forks":                             "--ea",
	"GET /cm/forks/new":                         "--ea",
	"POST /cm/forks/new":                        "--ea",
	"GET /cm/forks/{id}":                        "--ea",
	"POST /cm/forks/{id}/fork-page":             "--ea",
	"POST /cm/forks/{id}/pages/{pageID}/remove": "--ea",
	"GET /cm/forks/{id}/preview":                "--ea",
	"POST /cm/forks/{id}/merge":                 "---a",
	"POST /cm/forks/{id}/archive":               "---a",
	"POST /cm/forks/{id}/delete":                "---a",
}

// Every route, every role: the access check lets a request through to the
// handler exactly when adminRoleMatrix says so, refuses the rest with 403
// before the handler runs, and never lets a request without a session reach
// a non-public handler.
func TestAdminRoutes_RoleMatrix(t *testing.T) {
	h := routesTestHandler()
	roles := []string{"viewer", "contributor", "editor", "admin"}

	// The real access check in front of a sentinel instead of each handler.
	// Refusals are forced to JSON: the styled page needs the database and
	// has its own test (TestAdminRoutes_StyledRefusal).
	reached := ""
	r := mux.NewRouter()
	admin := r.PathPrefix("/cm").Subrouter()
	routes := h.AdminRoutes()
	for _, rt := range routes {
		key := routeKey(rt.Method, rt.Path)
		rt.JSON = true
		rt.handler = func(w http.ResponseWriter, _ *http.Request) {
			reached = key
			w.WriteHeader(http.StatusNoContent)
		}
		admin.HandleFunc(rt.Path, h.guardAdminRoute(rt)).Methods(rt.Method)
	}

	seen := map[string]bool{}
	for _, rt := range routes {
		key := routeKey(rt.Method, rt.Path)
		seen[key] = true
		want, ok := adminRoleMatrix[key]
		if !ok {
			t.Errorf("%s has no entry in adminRoleMatrix: say which roles may call it", key)
			continue
		}
		if want != "*" && len(want) != 4 {
			t.Errorf("%s: matrix entry %q is not four characters", key, want)
			continue
		}
		if (want == "*") != (rt.Session != "" || rt.Public != "") {
			t.Errorf("%s: matrix says %q but the route is declared with Perms=%v Session=%q Public=%q", key, want, rt.Perms, rt.Session, rt.Public)
		}
		target := "/cm" + routeVarRe.ReplaceAllString(rt.Path, "000000000000000000000001")

		for i, role := range roles {
			allowed := want == "*" || want[i] != '-'
			reached = ""
			rr := httptest.NewRecorder()
			r.ServeHTTP(rr, roleReq(role, rt.Method, target, strings.NewReader(""), nil))
			if allowed {
				if reached != key {
					t.Errorf("%s as %s: refused (%d), want it to reach the handler", key, role, rr.Code)
				}
				continue
			}
			if reached != "" {
				t.Errorf("%s as %s: reached handler %s, want 403", key, role, reached)
			}
			if rr.Code != http.StatusForbidden || !strings.Contains(rr.Body.String(), `"error"`) {
				t.Errorf("%s as %s: %d %q, want a 403 with an error", key, role, rr.Code, rr.Body.String())
			}
		}

		// No session at all
		reached = ""
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, httptest.NewRequest(rt.Method, target, strings.NewReader("")))
		if rt.Public != "" {
			if reached != key {
				t.Errorf("%s without a session: %d, want the public handler", key, rr.Code)
			}
		} else if reached != "" || rr.Code != http.StatusUnauthorized {
			t.Errorf("%s without a session: reached=%q status=%d, want 401 and no handler", key, reached, rr.Code)
		}
	}
	var stale []string
	for key := range adminRoleMatrix {
		if !seen[key] {
			stale = append(stale, key)
		}
	}
	sort.Strings(stale)
	for _, key := range stale {
		t.Errorf("adminRoleMatrix lists %s, which is not a route", key)
	}
}

// A signed-out request to a page route goes to the login form; to a fetch
// endpoint it gets a JSON 401.
func TestAdminRoutes_NoSession(t *testing.T) {
	h := routesTestHandler()
	r := mux.NewRouter()
	h.RegisterAdminRoutes(r.PathPrefix("/cm").Subrouter())

	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, httptest.NewRequest("POST", "/cm/templates/new", strings.NewReader("name=x")))
	if rr.Code != http.StatusSeeOther || rr.Header().Get("Location") != "/cm/login" {
		t.Errorf("form route without a session: %d -> %q, want 303 to /cm/login", rr.Code, rr.Header().Get("Location"))
	}
	rr = httptest.NewRecorder()
	r.ServeHTTP(rr, httptest.NewRequest("POST", "/cm/replace/execute", strings.NewReader("{}")))
	if rr.Code != http.StatusUnauthorized || !strings.Contains(rr.Header().Get("Content-Type"), "application/json") {
		t.Errorf("fetch route without a session: %d %q, want a JSON 401", rr.Code, rr.Header().Get("Content-Type"))
	}
}
