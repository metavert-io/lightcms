package handlers

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// Guards for the admin UI's side of the 7.4.2 changes: every state-changing
// fetch sends the CSRF token (a session-authenticated /api/v1 request without
// it is refused), the @mention dropdown is built without inline handlers, and
// the delete/revert confirmations show their message as text.

var fetchCallRe = regexp.MustCompile(`\bfetch\(`)

// jsCallArgs returns the argument text of every call matched by re in src
// (re ends with the opening parenthesis).
func jsCallArgs(src string, re *regexp.Regexp) []string {
	var calls []string
	for _, loc := range re.FindAllStringIndex(src, -1) {
		depth, quote := 0, byte(0)
		i := loc[1]
	scan:
		for ; i < len(src); i++ {
			c := src[i]
			switch {
			case quote != 0:
				if c == '\\' {
					i++
				} else if c == quote {
					quote = 0
				}
			case c == '\\':
				i++ // an escaped quote in a call that is itself inside a JS string
			case c == '\'' || c == '"':
				quote = c
			case c == '(' || c == '[' || c == '{':
				depth++
			case c == ')' || c == ']' || c == '}':
				if depth == 0 {
					break scan
				}
				depth--
			}
		}
		calls = append(calls, src[loc[1]:i])
	}
	return calls
}

var (
	unsafeMethodRe = regexp.MustCompile(`(?i)\bmethod\s*:\s*['"](POST|PUT|PATCH|DELETE)['"]`)
	csrfHelperRe   = regexp.MustCompile(`\bheaders\s*:\s*csrfHeaders\(`)
	csrfLiteralRe  = regexp.MustCompile(`\bheaders\s*:\s*\{[^}]*'X-CSRF-Token'\s*:`)
)

// fetchCSRFProblem says what is wrong with one fetch call's arguments, ""
// when it is fine. A call that changes state must send the token: through
// csrfHeaders() for /api/v1 (the documented helper), and through it or an
// explicit X-CSRF-Token header for the /cm endpoints that predate it.
func fetchCSRFProblem(args string) string {
	if !unsafeMethodRe.MatchString(args) {
		return ""
	}
	if csrfHelperRe.MatchString(args) {
		return ""
	}
	if strings.Contains(args, "/api/v1") {
		return "calls /api/v1 with an unsafe method without headers: csrfHeaders(...)"
	}
	if csrfLiteralRe.MatchString(args) {
		return ""
	}
	return "changes state without the CSRF token: add headers: csrfHeaders(...)"
}

func TestServedTemplates_UnsafeFetchSendsCSRFToken(t *testing.T) {
	// The checker itself: the pre-7.4.2 calls fail, the fixed ones pass
	for _, bad := range []string{
		`fetch('/api/v1/content/' + contentId + '/comments/' + commentId, {method: 'DELETE'});`,
		`fetch('/api/v1/approval-requests/'+id+'/approve', {method:'POST',headers:{'Content-Type':'application/json'},body:'{}'});`,
		`fetch('/api/v1/approval-workflows', {method:'POST',headers:{'Content-Type':'application/json','X-CSRF-Token':tok},body:JSON.stringify(payload)});`,
		`fetch('/cm/tools/search/reindex', { method: "post" });`,
		`fetch(url, {method: 'PUT', body: data, headers: {'Content-Type': 'application/json'}});`,
	} {
		args := jsCallArgs(bad, fetchCallRe)
		if len(args) != 1 || fetchCSRFProblem(args[0]) == "" {
			t.Fatalf("checker accepts %s", bad)
		}
	}
	for _, good := range []string{
		`fetch('/api/v1/users');`,
		`fetch('/api/v1/content/' + contentId + '/comments/' + commentId, {method: 'DELETE', headers: csrfHeaders()});`,
		`fetch('/api/v1/approval-workflows', {method:'POST',headers:csrfHeaders({'Content-Type':'application/json'}),body:JSON.stringify(payload)});`,
		`fetch('/cm/replace/execute', { method: 'POST', headers: { 'Content-Type': 'application/json', 'X-CSRF-Token': csrfToken }, body: JSON.stringify({ search: (a) }) })`,
		`fetch('/api/search?q=' + encodeURIComponent(q) + '&mode=hybrid')`,
		`fetch('/cm/tools/search/test?q=' + encodeURIComponent(query), {method: 'GET'})`,
	} {
		args := jsCallArgs(good, fetchCallRe)
		if len(args) != 1 || fetchCSRFProblem(args[0]) != "" {
			t.Fatalf("checker rejects %s: %s", good, fetchCSRFProblem(args[0]))
		}
	}

	unsafeCalls, apiCalls := 0, 0
	for _, path := range servedSources(t) {
		rel, _ := filepath.Rel(repoRoot, path)
		rel = filepath.ToSlash(rel)
		if rel == vendoredQuillJS {
			continue
		}
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", rel, err)
		}
		src := string(data)
		for _, args := range jsCallArgs(src, fetchCallRe) {
			if !unsafeMethodRe.MatchString(args) {
				continue
			}
			unsafeCalls++
			if strings.Contains(args, "/api/v1") {
				apiCalls++
			}
			if problem := fetchCSRFProblem(args); problem != "" {
				first := strings.SplitN(strings.TrimSpace(args), "\n", 2)[0]
				t.Errorf("%s: fetch(%s ... %s", rel, first, problem)
			}
		}
		// The token helper only reaches fetch(): other ways of sending a
		// request from a served page would bypass this check.
		for _, other := range []string{"XMLHttpRequest", "sendBeacon(", "$.ajax(", "axios."} {
			if strings.Contains(src, other) {
				t.Errorf("%s uses %s: send state-changing requests with fetch() and csrfHeaders()", rel, other)
			}
		}
		// A plain form cannot send the X-CSRF-Token header
		if regexp.MustCompile(`(?i)<form[^>]*action="/api/v1`).MatchString(src) {
			t.Errorf("%s posts a form to /api/v1: session-authenticated API writes need fetch() with csrfHeaders()", rel)
		}
	}
	if unsafeCalls < 9 || apiCalls < 6 {
		t.Fatalf("scan saw only %d state-changing fetch calls (%d to /api/v1); it is no longer reading the admin templates", unsafeCalls, apiCalls)
	}

	// The helper every admin page gets
	for _, want := range []string{
		"var lcCSRFToken = {{.CSRFToken}};",
		"function csrfHeaders(extra) {",
		"var headers = {'X-CSRF-Token': lcCSRFToken};",
	} {
		if !strings.Contains(adminLayoutStart, want) {
			t.Errorf("adminLayoutStart lacks %q", want)
		}
	}
}

// The @mention dropdown used to build each entry as markup with an inline
// onclick="insertMention('id','name'" — a missing parenthesis, so choosing a
// name did nothing, and a name with a quote in it could end the attribute.
// Entries are DOM nodes carrying data attributes, picked up by one listener.
func TestContentForm_MentionDropdownHasNoInlineHandlers(t *testing.T) {
	form := adminTemplates["content_form"]
	i := strings.Index(form, "// @mention autocomplete")
	j := strings.Index(form, "async function postComment(")
	if i < 0 || j < i {
		t.Fatal("the @mention script is no longer where this test looks for it")
	}
	mention := form[i:j]
	for _, bad := range []string{"onclick=", "insertMention(\\'", "mentionDropdown.innerHTML", "safeNameForAttr"} {
		if strings.Contains(mention, bad) {
			t.Errorf("@mention script still contains %q: build the entries with the DOM, not markup", bad)
		}
	}
	for _, want := range []string{
		"b.dataset.mentionId = ",
		"b.dataset.mentionName = ",
		"b.textContent = ",
		"mentionDropdown.addEventListener('click'",
		"insertMention(b.dataset.mentionId, b.dataset.mentionName)",
		// /api/v1/users answers {"users": [...]}, not a bare array
		"data.users || []",
	} {
		if !strings.Contains(mention, want) {
			t.Errorf("@mention script lacks %q", want)
		}
	}
}

// confirmDelete and confirmRevert show their message as text: one definition
// each (in the layout), writing textContent, with callers passing plain text.
func TestAdminLayout_ConfirmDeleteAndRevertAreTextSafe(t *testing.T) {
	all := adminLayoutStart + adminLayoutEnd
	for name := range adminTemplates {
		all += templateBody(adminTemplates[name])
	}
	for _, fn := range []string{"confirmDelete", "confirmRevert"} {
		if n := strings.Count(all, "function "+fn+"("); n != 1 {
			t.Errorf("%s is defined %d times in the admin templates, want once (in adminLayoutEnd)", fn, n)
		}
		i := strings.Index(adminLayoutEnd, "function "+fn+"(")
		if i < 0 {
			t.Errorf("adminLayoutEnd no longer defines %s", fn)
			continue
		}
		body := adminLayoutEnd[i:]
		if j := strings.Index(body, "\n    }\n"); j > 0 {
			body = body[:j]
		}
		if strings.Contains(body, "innerHTML") || strings.Contains(body, "insertAdjacentHTML") || strings.Contains(body, "outerHTML") {
			t.Errorf("%s writes its message as HTML; use textContent", fn)
		}
		if !strings.Contains(body, "msgEl.textContent = ") {
			t.Errorf("%s no longer sets msgEl.textContent", fn)
		}
	}
	for _, id := range []string{"delete-modal-message", "revert-modal-message"} {
		if !strings.Contains(adminLayoutEnd, `<p id="`+id+`" style="margin: 0; white-space: pre-line;">`) {
			t.Errorf("#%s lost white-space: pre-line (line breaks in messages are \\n, not <br>)", id)
		}
	}

	// Callers: a message is text, so markup in one would be shown literally
	calls := 0
	for _, path := range servedSources(t) {
		rel, _ := filepath.Rel(repoRoot, path)
		if filepath.ToSlash(rel) == vendoredQuillJS {
			continue
		}
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", rel, err)
		}
		for _, args := range jsCallArgs(string(data), regexp.MustCompile(`\bconfirm(?:Delete|Revert)\(`)) {
			if strings.HasPrefix(args, "form, ") {
				continue // the definitions
			}
			calls++
			if regexp.MustCompile(`(?i)<\s*/?\s*[a-z]|&lt;|&quot;`).MatchString(args) {
				t.Errorf("%s: confirmDelete/confirmRevert(%s) passes markup; the message is shown as text (use \\n for a line break)", filepath.ToSlash(rel), args)
			}
		}
	}
	if calls < 12 {
		t.Fatalf("scan saw only %d confirmDelete/confirmRevert calls", calls)
	}
}

// Delete in the admin is a soft delete with Restore: no dialog may call it
// irreversible. Since 7.5 a merge keeps each page's previous version, so its
// confirmation says that instead of calling it irreversible.
func TestAdminTemplates_DeleteWordingMatchesSoftDelete(t *testing.T) {
	undone := regexp.MustCompile(`(?i)cannot be undone|can't be undone|permanently delet`)
	for _, name := range sortedAdminTemplateNames() {
		for i, line := range strings.Split(templateBody(adminTemplates[name]), "\n") {
			if !undone.MatchString(line) {
				continue
			}
			if strings.Contains(line, `/content/`) && strings.Contains(line, "/delete") {
				t.Errorf("%s line %d tells the user deleting a page cannot be undone; it is a soft delete with Restore", name, i+1)
			}
		}
	}
	form := adminTemplates["content_form"]
	if !strings.Contains(form, "can be restored") {
		t.Error("the Delete Page confirmation no longer says the page can be restored")
	}
	merge := adminTemplates["fork_detail"]
	if strings.Contains(merge, "cannot be undone") || !strings.Contains(merge, "previous version stays in its history") {
		t.Error("the fork merge confirmation should say each page's previous version is kept, not that a merge cannot be undone")
	}
}
