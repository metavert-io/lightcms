package handlers

import (
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// Guards for two admin UI bugs fixed in 7.4.3: delete buttons whose
// confirmation text sat in an attribute nothing reads, and HTML built in
// JavaScript with an inline event handler that had data concatenated into it.

// deadConfirmAttrRe matches attributes that look like a confirmation message
// but that no script reads. The styled confirm reads data-confirm (and
// data-confirm-title) on the <form>.
var deadConfirmAttrRe = regexp.MustCompile(`\bdata-(message|confirm-message|confirm-text|prompt)=`)

// The API key and snippet Delete buttons carried data-message="Are you
// sure...", which nothing read: they deleted on the first click.
func TestAdminTemplates_NoDeadConfirmAttributes(t *testing.T) {
	for _, name := range sortedAdminTemplateNames() {
		body := templateBody(adminTemplates[name])
		for _, loc := range deadConfirmAttrRe.FindAllStringIndex(body, -1) {
			line := 1 + strings.Count(body[:loc[0]], "\n")
			t.Errorf("template %q line %d: %s is read by nothing — put the message in data-confirm on the <form>", name, line, body[loc[0]:loc[1]])
		}
	}
	// data-confirm belongs on a form (the listener is on submit)
	onNonForm := regexp.MustCompile(`<(button|a|input|div|span)\b[^>]*\bdata-confirm=`)
	for _, name := range sortedAdminTemplateNames() {
		body := templateBody(adminTemplates[name])
		for _, loc := range onNonForm.FindAllStringIndex(body, -1) {
			t.Errorf("template %q line %d: data-confirm on a non-form element is never read", name, 1+strings.Count(body[:loc[0]], "\n"))
		}
	}
}

var postFormRe = regexp.MustCompile(`<form\b[^>]*method="POST"[^>]*>`)

// Every form that deletes something asks first: through the styled confirm
// (data-confirm) or the delete modal (confirmDelete).
func TestAdminTemplates_DeleteFormsAskFirst(t *testing.T) {
	checked := 0
	for _, name := range sortedAdminTemplateNames() {
		body := templateBody(adminTemplates[name])
		for _, loc := range postFormRe.FindAllStringIndex(body, -1) {
			tag := body[loc[0]:loc[1]]
			if !regexp.MustCompile(`action="[^"]*/(delete|remove)"`).MatchString(tag) {
				continue
			}
			checked++
			if !strings.Contains(tag, "data-confirm=") && !strings.Contains(tag, "confirmDelete(") {
				t.Errorf("template %q line %d: this form deletes without asking: %s", name, 1+strings.Count(body[:loc[0]], "\n"), tag)
			}
		}
	}
	if checked < 12 {
		t.Fatalf("only %d delete forms found; the scan is not seeing the templates", checked)
	}
	for _, want := range []string{
		`action="/cm/api-keys/{{.Key.ID.Hex}}/delete" style="display:inline;" data-confirm="`,
		`action="/cm/snippets/{{.ID.Hex}}/delete" style="display:inline;" data-confirm="`,
	} {
		found := false
		for _, tpl := range adminTemplates {
			if strings.Contains(tpl, want) {
				found = true
			}
		}
		if !found {
			t.Errorf("no template has a delete form with %s", want)
		}
	}
}

// inlineHandlerWithDataRe matches a JavaScript string literal that contains
// an inline event handler attribute left open at the end of the literal and
// followed by a concatenation: '<div onclick="add(\” + value. Whatever is
// concatenated there is parsed as HTML and then run as script; an email
// address with a quote in it was enough (the approver picker, 7.4.3; the
// @mention dropdown, 7.4.2).
var inlineHandlerWithDataRe = regexp.MustCompile(`\bon[a-z]+=(\\?["'])[^"'\n]*\\'+\s*'\s*\+`)

func inlineHandlerProblems(src string) []string {
	var problems []string
	for _, loc := range inlineHandlerWithDataRe.FindAllStringIndex(src, -1) {
		lineStart := strings.LastIndex(src[:loc[0]], "\n") + 1
		lineEnd := strings.Index(src[loc[0]:], "\n")
		if lineEnd < 0 {
			lineEnd = len(src) - loc[0]
		}
		full := src[lineStart : loc[0]+lineEnd]
		// Documentation samples are HTML-escaped text inside <pre><code>
		if strings.Contains(full, "&lt;") {
			continue
		}
		problems = append(problems, "line "+strconv.Itoa(1+strings.Count(src[:loc[0]], "\n"))+": "+src[loc[0]:loc[1]])
	}
	return problems
}

func TestServedTemplatesAndJS_NoInlineHandlersBuiltFromData(t *testing.T) {
	// The checker itself
	bad := []string{
		`res.innerHTML = '<div onclick="addApprover(\''+u.id+'\',\''+u.email+'\')">'+u.email+'</div>';`,
		`var b = '<button onclick="deleteComment(\'' + contentId + '\',\'' + comment.id + '\',this)">Delete</button>';`,
		`html += '<span onmouseover="this.style.color=\'' + color + '\'">x</span>';`,
	}
	for _, src := range bad {
		if len(inlineHandlerProblems(src)) == 0 {
			t.Fatalf("checker missed: %s", src)
		}
	}
	good := []string{
		`html += '<form method="POST" action="/cm/content/' + item.id + '/delete" onsubmit="return confirmDelete(this, \'Are you sure?\')">';`,
		`<button type="button" onclick="deleteWorkflow('{{.ID.Hex}}')">Delete</button>`,
		`row.dataset.email = u.email; row.textContent = u.email;`,
		`'<button onclick="closeModal()">' + label + '</button>'`,
	}
	for _, src := range good {
		if p := inlineHandlerProblems(src); len(p) != 0 {
			t.Fatalf("checker flagged safe code %s: %v", src, p)
		}
	}

	for _, name := range sortedAdminTemplateNames() {
		for _, p := range inlineHandlerProblems(templateBody(adminTemplates[name])) {
			t.Errorf("template %q %s — build the element with DOM calls, put the values in data attributes and use addEventListener", name, p)
		}
	}
	for what, src := range map[string]string{"adminLayoutStart": adminLayoutStart, "adminLayoutEnd": adminLayoutEnd} {
		for _, p := range inlineHandlerProblems(src) {
			t.Errorf("%s %s", what, p)
		}
	}
	js, err := os.ReadFile("../../static/js/chat-widget.js")
	if err != nil {
		t.Fatalf("read chat-widget.js: %v", err)
	}
	for _, p := range inlineHandlerProblems(string(js)) {
		t.Errorf("static/js/chat-widget.js %s", p)
	}
}

// The approver picker on the approvals page is built from DOM nodes: the
// email is text and a data attribute, and one delegated listener handles it.
func TestApprovalsTemplate_ApproverPickerUsesDOMAndDataAttributes(t *testing.T) {
	tpl := templateBody(adminTemplates["approvals_page"])
	for _, want := range []string{
		"row.dataset.email = u.email",
		"row.textContent = u.email",
		"res.addEventListener('click', pick)",
		"chip.appendChild(document.createTextNode(a.email))",
		"list.addEventListener('click'",
	} {
		if !strings.Contains(tpl, want) {
			t.Errorf("approvals template lacks %q", want)
		}
	}
	for _, gone := range []string{`onclick="addApprover(`, `onclick="removeApprover(`, "+u.email+", "+a.email+"} {
		if strings.Contains(tpl, gone) {
			t.Errorf("approvals template still contains %q", gone)
		}
	}
}
