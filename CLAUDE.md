Read VIBECTL.md for current project status, deployment details, and issue context before starting work.

# LightCMS - Claude Code Memory

## Project Overview

LightCMS is a lightweight, self-hosted content management system built in Go. It uses MongoDB for data storage and generates static HTML pages for public content serving.

**Key URLs:**
- Admin Dashboard: `/cm`
- Public Site: `/`
- MCP Server: `bin/lightcms-mcp` (stdio transport)

## MCP Server Integration

**IMPORTANT:** All website content operations MUST go through the MCP server. Do NOT:
- Write scripts to directly modify the database
- Use Go code to create/edit/delete content
- Bypass the MCP server for any content management tasks

If the MCP server is not available or not working, ASK the user for permission before attempting any content changes through other means.

### Starting the MCP Server

If the user requests a content operation and the MCP server is not yet running, start it using the convenience script:

```bash
./bin/lightcms-mcp-wrapper.sh
```

This wrapper script sets up the required environment variables (LIGHTCMS_CONFIG_DIR) and launches the MCP server. The MCP server uses stdio transport, so it will be connected automatically once started.

### Content Operations (REQUIRE MCP or explicit user permission):
- Creating, editing, publishing, or deleting content
- Managing templates and their HTML layouts
- Uploading or managing assets (images, CSS, JS, documents)
- Updating theme settings (colors, fonts, header/footer HTML)
- Managing redirects, folders, and collections
- Viewing or reverting content versions
- Site configuration changes

### Code Changes (allowed without MCP):
- Adding new features to LightCMS itself
- Fixing bugs in the application
- Changing application behavior or logic
- Adding new MCP tools
- Modifying database schemas or indexes
- Security improvements

### ⚠️ IMPORTANT: Rebuild MCP binary after adding tools

The Claude Desktop MCP integration uses a **local binary** (`bin/lightcms-mcp`) that connects to the remote server. The binary encodes all tool definitions — if you add or remove MCP tools without rebuilding it, Claude Desktop will see stale/missing tools.

**After any change to `cmd/mcp/` or `internal/mcp/`:**
```bash
go build -o bin/lightcms-mcp ./cmd/mcp
```

Then **restart Claude Desktop** to reload the MCP server. Until it's restarted, Cowork will still use the old binary.

### Setting Up MCP with Claude Code

Run the setup script from the lightcms directory:

```bash
./setup-mcp.sh
```

This builds the MCP server, creates the wrapper script, and registers it with Claude Code.

**Manual setup** (if needed):
```bash
# Build the MCP server
go build -o bin/lightcms-mcp ./cmd/mcp

# Register with Claude Code (use the wrapper script, not the binary directly)
claude mcp add --transport stdio lightcms-mcp -- /path/to/lightcms/lightcms-mcp-wrapper.sh
```

After registering, restart Claude Code. You can verify the server is connected:
- Run `/mcp` in Claude Code to check status
- Run `claude mcp list` in terminal to see registered servers

Once connected, you can ask Claude to manage your content naturally:
- "Create a new blog post about AI"
- "List all my published content"
- "Update the homepage hero image"
- "Delete the /random page"

### MCP Server Location
Binary: `bin/lightcms-mcp`
Config: Uses same `config.dev.json` or environment variables as main server

### Available MCP Tools (130 total):

**Content (23 tools):** list_content, get_content, create_content, update_content, update_content_by_path, publish_content, publish_multiple, unpublish_content, delete_content, restore_content, preview_content, get_content_versions, get_content_version, revert_to_version, bulk_create_content, bulk_update_content, bulk_field_operation, export_content, get_backlinks

**Templates (5 tools):** list_templates, get_template, create_template, update_template, delete_template

**Snippets (5 tools):** list_snippets, get_snippet, create_snippet, update_snippet, delete_snippet

**Assets (6 tools):** list_assets, list_asset_folders, get_asset, upload_asset, upload_asset_from_url, delete_asset

**Search (7 tools):** search_content, search_replace_preview, search_replace_execute, scoped_search_replace_preview, scoped_search_replace_execute, end_user_search, reindex_embeddings

**Settings (18 tools):** get_theme, update_theme, get_theme_versions, get_theme_version, revert_theme_to_version, pin_theme_version, unpin_theme_version, get_site_config, update_site_config, list_redirects, create_redirect, update_redirect, delete_redirect, list_folders, create_folder, get_folder, delete_folder, list_collections, create_collection, get_collection, update_collection, delete_collection, regenerate_all_content

**Forks (9 tools):** list_forks, create_fork, get_fork, fork_page, remove_fork_page, merge_fork, archive_fork, delete_fork, purge_fork_copies

**Comments (3 tools, v6.0+):** list_comments, create_comment, delete_comment

**Agent Sandbox & Governance (10 tools, v7.0+):** start_agent_sandbox, get_agent_sandbox, end_agent_sandbox, get_fork_diff, get_agent_session_changes, rollback_agent_session, get_maintenance_report, run_maintenance_scan, backfill_published_dates, repair_fork_damage

**IndexNow (3 tools, v7.2.3+):** get_indexnow_status, set_indexnow_enabled, submit_indexnow

**SEO & AI (3 tools, v7.3+):** get_seo_settings, update_seo_settings, get_ai_traffic

**Approvals (11 tools, v6.0+):** list_approval_workflows, get_approval_workflow, create_approval_workflow, update_approval_workflow, delete_approval_workflow, list_approval_requests, get_approval_request, submit_for_approval, approve_request, reject_request, cancel_approval_request

### ⚠️ CRITICAL: Search & Replace Safety

The `search_replace_execute` tool is **destructive** and modifies content permanently. Before using it, you MUST:

1. **ALWAYS run `search_replace_preview` first** to see exactly what will be changed
2. **Show the user the preview results**, including:
   - Number of pages affected
   - Which pages are published vs drafts
   - Sample excerpts showing what will change
3. **Explicitly ask for user confirmation** before running `search_replace_execute`
4. **Never execute search/replace without user consent**, even if the user asked for a "quick fix"

Example interaction:
```
User: "Update all links from http to https"
Assistant: "Let me preview what would change..."
[Run search_replace_preview]
Assistant: "This would affect 15 pages (12 published, 3 drafts) with 47 total replacements. Here are the affected pages: [list]. Should I proceed with these changes?"
User: "Yes, go ahead"
[Run search_replace_execute]
```

## Content Authoring: Wiki-Like Markup

When creating or updating content via MCP or admin UI, the following markup features are processed at page generation time:

### Wikilinks
- `[[Page Title]]` — links to a page by its title (case-insensitive lookup)
- `[[Page Title|display text]]` — link with custom label
- `[[/full/path]]` — links to a page by its URL path
- `[[/full/path|display text]]` — path link with custom label
- Broken links render as `<span class="broken-link">text</span>`
- Links auto-update when a page's title or path changes (via `UpdateWikilinksOnRename`)

### Snippet Includes
- `[[include:snippet-name]]` — embeds a named snippet inline
- Snippet name must match the `name` field in the snippets collection
- Recursion depth limit: 3 levels; cycles are detected and dropped

### Table of Contents
- Add `{{.lc_toc}}` in a template's HTML layout where the TOC should appear
- Auto-generates `<nav class="lc-toc"><ul>...</ul></nav>` from page headings
- All headings automatically get `id=` attributes for anchor navigation

### Markdown Fields
- Set a template field type to `markdown` for GFM rendering at publish time
- Supports tables, strikethrough, task lists, autolinks
- Script policy controls whether raw HTML/scripts are allowed (see Script Policy below)

### Inline Tag Detection
- Mention `#tagname` in any content field to automatically tag the page
- Tag names: start with a letter, alphanumeric/underscore/hyphen, max 50 chars
- Tags feed into `lc:query` index pages

### Snippet Best Practices (for MCP agents)
- Use snippets for reusable UI components: callout boxes, CTA sections, disclaimers, badge patterns
- Snippet variables: `{{.Title}}`, `{{.FullPath}}`, `{{.Slug}}`, `{{.MetaDescription}}`, `{{.PublishedAt}}`
- Reference snippets in content with `[[include:snippet-name]]` or in template layouts via `lc:query` directives

## Agentic Safety Features (v7.0+)

- **Credentials act as a person (7.5)**: an API key acts as its owner; an OAuth connection (`oauth_grants`) acts as the person who approved it. `auth.CredentialResolver` loads that person on every request (current role, disabled refused, revoked connection refused). There is no shared system key. OAuth tokens are bound to `BASE_URL/mcp` and refused at `/api/v1`; `/mcp` refuses cookies.
- **`/mcp` calls the REST API in-process** (`internal/mcp/http_handler.go`): the caller travels in the request context (`middleware.WithInternalCaller`), never as a credential, and the SDK binds each MCP session to the credential that opened it (`TokenInfo.UserID`). Only that file may call `WithInternalCaller`. A tool or resource handler must pass its `ctx` to the API client: a call without the caller fails closed. A panic in a REST handler is recovered in the transport.
- **Access: Sandbox or Full (7.5)**: every key and connection has it (`SandboxOnly`), plus `CanMergeOwnForks`, which only a signed-in admin can turn on for an account that can merge (consent page, `/cm/api-keys`, `/cm/connected-apps`; never through the API). New OAuth connections and keys made in the UI default to Sandbox.
- **The sandbox is enforced in the REST layer**: `auth.SessionUser.Sandboxed()` (Sandbox access, or an open sandbox) → `handlers.SandboxGate` refuses every write route not in `sandboxWriteRoutes` (`internal/handlers/api_sandbox.go`); the handlers behind allowed routes hold the caller to forks its credential created (`ContentFork.CreatedByCredential`) and to new non-executable uploads. Adding a write route to `/api/v1` fails `TestSandboxRoutes_EveryWriteRouteIsDecided` until it is listed as allowed (with why it can't reach the live site) or refused. MCP tools don't block anything themselves.
- **Agent sandbox sessions**: `start_agent_sandbox` (REST `POST /api/v1/sandbox`) records the open fork on the key or connection (`active_sandbox_fork`), so it holds in every session and across restarts; writes go into the fork (copy-on-write). `end_agent_sandbox` submit freezes the fork (`SubmittedAt`: no key or connection can change it) and never lifts Sandbox access. Merging, archiving or deleting the fork closes the sandbox (`ForkService.releaseSandboxes`). PREFER sandbox mode for multi-page or risky edits.
- **Merging is bound to a review**: `ForkService.Review` diffs every field a merge writes, in full, and returns a digest of the fork and live pages; `Merge` takes it and returns `ErrForkChanged` if anything changed. Through a key or connection, a merge needs `CanMergeOwnForks`, a role that can merge, and a fork that credential created and has not submitted (`APIMergeFork`). `Merge` claims the fork (status `merging`, expiring after 10 minutes) and takes the digest from exactly the pages it applies. A submitted fork is frozen for every key and connection on every route that changes it (edit, delete, revert, restore, archive, delete fork). Merges write a version per page and list `created_ids`/`updated_ids` in the `fork.merge` audit entry, which session rollback reads.
- **Sandbox-only API keys**: keys created with `sandbox_only: true` are the same Sandbox access. Keys can also carry a `scopes` permission allowlist.
- **Session ledger & rollback**: each MCP session has an agent-session ID recorded in the audit log (the HTTP endpoint generates it; the stdio binary sends its own); `get_agent_session_changes` shows everything the session touched, `rollback_agent_session` undoes it as a unit, merges included.
- **Provenance**: every content version records actor (human|agent), via (ui|api|copilot), and the agent session; the audit log also names the key or connection (`credential`).
- **Maintenance reports**: a daily scan surfaces stale pages, missing meta descriptions, and drafts via `get_maintenance_report` — treat it as a work queue.
- **Public read-only MCP**: `/mcp-public` (no auth) exposes search_site/get_page/list_pages/get_site_info to visitors' agents; `/llms.txt` and `/llms-full.txt` serve AI crawlers.
- **Admin copilot**: `/cm/copilot` lets editors drive content operations in natural language (requires ANTHROPIC_API_KEY; model via LIGHTCMS_COPILOT_MODEL).

## Engineering Invariants & Lessons (v7.x) — READ BEFORE CHANGING CODE

Hard-won rules from building v7. Violating these has bitten us before:

### Module path & forking
- The module is `github.com/metavert-io/lightcms/v7`. Go's semantic import versioning means a major-version bump (v8.x tags) requires renaming the module path to `/v8` across every import. If you FORK this repo, rename the module to your own path first:
  `sed -i '' 's|github.com/metavert-io/lightcms/v7|github.com/YOU/yourcms/v7|g' go.mod $(find . -name '*.go' -not -path './bin/*')`
- pkg.go.dev only indexes modules whose declared path matches the repo host path.

### Release process
- Every release: bump `build.json` (the admin sidebar displays this version), add a CHANGELOG.md entry, tag `vX.Y.Z`, and rebuild `bin/lightcms-mcp`.

### Template/JS escaping (caused a production bug)
- The admin UI is Go `html/template` const strings in `internal/handlers/admin_templates.go` (`adminLayoutStart`/`adminLayoutEnd`; the copilot drawer lives in adminLayoutEnd).
- html/template ALREADY quotes and escapes values in `<script>` context. NEVER pre-quote with `printf "%q"` — `{{.CSRFToken}}` bare is correct; pre-quoting double-wraps the value in literal quotes (this broke copilot CSRF in production).

### Fork-content safety invariant
- Content with `ForkID != nil` shares its `full_path` with the live page. It must NEVER generate or remove static files, or touch the embedding index. `GenerateStaticPage` and `UpdateContent` guard this — preserve those guards in any new content-mutation path.
- The same holds outside the service layer. The **admin editor handlers** write to MongoDB directly and each one guards fork copies itself: `UpdateContent` forces `published=false` on a copy and skips the static file, redirect, link-rewrite, wikilink, sitemap and IndexNow work; `DeleteContent` removes a copy from its fork (`ForkService.RemovePage`) instead of soft-deleting; `RegenerateContent`, `ConfirmChangeTemplate` and `RevertContentVersion` never write a copy's file (`Handler.generateStaticPage` returns early for a copy). The **change-stream watcher** (`content_watcher.go`) skips fork copies before it generates or removes anything. A new admin handler or background job that touches content needs the same check.
- A fork copy has no side effects outside its fork, whichever service method writes it. `DeleteContent` and `UnpublishContent` return before the webhook, Cloudflare purge, IndexNow, keyword-rebuild and index-regen steps for a copy (7.4.2); `CreateContent` and `BulkCreateContent` fire no `content.create` webhook for one, and `UpdateContent` returns right after saving the copy and its version — before the static file, embedding, IndexNow, keyword rebuild, wikilink rename, index regen and `content.update` webhook (7.4.3; it checks the stored row as well as the argument). Anything added to these methods after the save goes below that return, or needs its own `ForkID` check. Remove a live page's file through `ContentService.RemoveStaticPage(fullPath)` — by full path, never by slug (two pages in different folders share a slug), and never for an empty path (it maps to the homepage's file).
- A path lookup that means "the page at this URL" must be live-only: add `"fork_id": nil` (or `liveOnly(filter)`) to any `full_path` / `source_url` query. Import lookups and the analytics edit links were fixed in 7.4.2. The admin editor's duplicate-path checks (`UpdateContent`, `UndeleteContent`, `CheckSlug`) use `pathScope(forkID)` (7.4.3): a live page is compared with live pages, a fork copy with the copies in its own fork (`CheckSlug` learns which from its `exclude` id). `ForkService.Merge` matches a copy to its live page by path, so a copy saved under another path becomes a new page — or an edit of the live page already at that path — when the fork is merged.
- **Every query that puts content on a public page needs both filters:** `"fork_id": nil` and `"deleted": {"$ne": true}`, next to `"published": true`. The service's `DeleteContent` (API, MCP) leaves `full_path` as it was, and until 7.4.4 left `published` too, so `published: true` alone can still match a deleted page. Since 7.4.4 it clears `published` and sets `published_before_delete`, which `RestoreContent` uses to publish the page again (not a held page, not a fork copy) — keep that pair together: agent-session rollback restores deleted pages through it. `ServePage` (exact, legacy-slug and case-insensitive lookups), `serve404`, `serveCollection`, `QueryContentForDirective` (`lc:query`) and the wikilink index were each missing one of the two until 7.4.3. Use `"fork_id": nil` (matches a missing or null field) rather than `{"$exists": false}` in new code.
- Admin handlers derive a static file from a path through `Handler.removeStaticPage(fullPath)` (never `os.Remove(h.getStaticFilePath(...))`): `getStaticFilePath("")` is the homepage's file. `updateContentFolderPaths` was the last direct caller (7.4.3), and skips fork copies.
- `ContentService.RepairForkDamage(ctx, dryRun)` (API `POST /api/v1/maintenance/repair-fork-damage`, MCP `repair_fork_damage`) repairs pre-7.4.1 damage: it clears `published` on fork copies and regenerates live published pages whose static file is missing. Regeneration there must clear the in-memory `ContentHash` first — `GenerateStaticPage` skips the write when the hash matches, even if the file is gone — and runs under `WithoutIndexNow`.
- Fork copies are never published directly (`PublishContent` returns `ErrPublishForkCopy`) and are hidden from `ListContent`, `ListContentPaginated`, `ListContentScoped` and search unless include-forks is passed (v7.4). Any new listing must filter `"fork_id": nil` unless it is fork-specific. `ForkService.Merge` deletes a fork's copies after a successful merge. `StreamContent`/`StreamContentScoped` (search-and-replace) and `GetContentByPath` are live-only too (v7.4.1); a path reaches a fork copy only through `ForkService.GetForkPageByPath` (by-path API with `fork_id`, the fork-page endpoint the agent sandbox uses, fork preview). Scoped bulk operations report fork copies named in `content_ids` as `skipped` (`ContentService.ForkCopyIDs`).
- `Hold` (v7.4): a held draft cannot become published. `PublishContent`, `CreateContent` and `UpdateContent` return `ErrContentHeld`; batch publish, the scheduler, approvals and fork merges skip held pages. Paths that write `published` straight to MongoDB (the admin editor, approvals) must check hold themselves.

### Provenance & agent sessions
- Every content mutation path must stamp `services.WithEditorEmail` and `services.WithProvenance` on the context (the /api/v1 middleware does this for API calls; the admin UI and copilot do it explicitly). Session rollback (`/api/v1/agent-sessions/{id}/rollback`) selects revert targets BY PROVENANCE — a session's own versions are never rollback targets. Timestamps race (a session's version write can land before its async audit entry); do not reintroduce timestamp-based selection.
- The MCP server sends `X-Agent-Session` on every request; audit logs record it. From other API clients the header is only a label. OAuth callers are always recorded as agents (`handlers.StampProvenance`).

### Performance: content queries need projections
- `models.Content` carries a ~4KB embedding vector and full data map per document. List-style endpoints (llms.txt, feeds, indexes) MUST use MongoDB projections — loading full documents made /llms.txt take 40+ seconds and starve the 1GB production VM.
- Analytics hour-bucket queries use an EXCLUSIVE upper bound (`$lt untilKey`); pass `until = now + 1h` to include the current hour.

### Testing
- DB tests need `.env.test` with MONGODB_URI; the database name MUST contain "test" (safety guard). Run DB packages with `-p 1`.
- The handlers package has its OWN cleanup list (`cleanupCollections` in `internal/handlers/testhelper_test.go`), separate from `testutil.CleanupCollections`: add a new collection to both. It did not drop `content_forks` until 7.4.3, and `createTestFork` derives the preview token from the fork name, so a test that looks a fork up by token should use a unique name.
- When the Atlas test cluster is flaky a full `go test ./...` can fail a different handful of tests on each run (stalls of 15–100s, "i/o timeout", "server selection"). Read the failure text before debugging; a test that fails fast and repeatably on its own is real, the rest are the network.
- New MongoDB collections MUST be added to `CleanupCollections` in `internal/testutil/testutil.go`, or leftover data makes tests flake across runs (bit us with `maintenance_reports`).
- The `/api/v1` routes and their middleware are registered in `internal/handlers/api_routes.go` (`APIHandler.MountAPI`), so tests can mount and walk the real router; `cmd/server/main.go` only calls it.
- Tests must not write tracked files. The handlers and services write `static/sitemap.xml`, `static/css/theme-vars.css` and the admin editor's uploads relative to the working directory, which under `go test` is the package directory, where copies of the first two are checked in. `TestMain` in `internal/handlers` and `internal/services` points them at a temp directory (`sitemapFile`, `services.ThemeCSSFile`, `UploadsDir`) and fails the run if any tracked file under the package changed (`testutil.SnapshotTrackedFiles`). Give any new file the server writes the same kind of variable.
- Tests asserting on collection contents should match by seeded paths/IDs, not exact counts — background goroutines (index regen, keyword rebuild) can insert content mid-test.
- Coverage gate: Codecov ≥80% (target margin ~85%); codecov.yml excludes cmd/, testutil, internal/handlers, content_watcher.go. Codecov's line metric reads ~2 points BELOW `go tool cover` statement totals.
- Transient Atlas DNS failures ("no such host", "server selection timeout") are environmental — rerun before investigating.

### IndexNow (v7.2.3)
- `services.IndexNowService` queues paths via `ContentService.notifyIndexNow`. Edits to published pages notify from static generation ONLY when the rendered HTML hash changed. Publish/unpublish/rename/delete/restore transitions notify explicitly in the service methods.
- Site-wide re-renders MUST run under `services.WithoutIndexNow(ctx)` (RegenerateAllContent, template regen, regen queue already do). Any new bulk re-render path needs it too, or every page gets pinged to search engines.
- Self-gating: inactive in dev or with a non-public BASE_URL; verifies its own `/{key}.txt` via BASE_URL before every submission window. Config lives in settings `type: indexnow_config`, not SiteConfig (SaveSiteConfig $sets the whole struct).

### SEO & AI (v7.3)
- One crawler registry, `services.KnownCrawlers` (aicrawlers.go), drives robots.txt, analytics classification, and the admin UI. To support a new crawler, add it there (put more specific UA substrings first). AI-assistant referrer hosts live in the same file.
- Settings live in settings `type: seo_config` (`services.SEOService`, 30s in-memory cache), not SiteConfig. Zero values must mean "pre-7.3 behavior" (allow all crawlers, default feed selection).
- `content_modified_at` is stamped in `GenerateStaticPage` only when the HTML hash changes and the ctx is NOT `WithoutIndexNow` (site-wide re-renders don't count). Read dates via `Content.ModifiedAt()`, which falls back to `updated_at`.
- `noindex` pages must be excluded from every discovery surface: sitemap, llms.txt, feeds, Markdown copies, and IndexNow full submissions. Any new listing endpoint needs the `"noindex": {"$ne": true}` filter.
- Markdown copies are resolved in ServePage only AFTER exact, legacy and case-insensitive page lookups fail, so a real page whose path ends in `.md` wins. They are generated per request from the static HTML (no stored copies, nothing to invalidate).
- Dedicated public routes that fall back to page serving (feeds, IndexNow key) must use `servePagePath` / set the `slug` route var — `ServePage` with no slug serves the homepage (this bit us in 7.2.4).
- The admin editor's update/delete handlers write to MongoDB directly; they call `ContentService.NotifyLiveChange` for URL transitions. Content edits reach IndexNow via the change-stream regen.

### Copilot architecture
- The admin copilot (`/cm/copilot/chat`) is an Anthropic tool-use loop in `internal/handlers/copilot.go` executing directly against the service layer (NOT via the MCP client). To add a copilot tool: add its schema to `copilotToolDefs()` and its execution to `executeCopilotTool()` with an explicit `auth.HasPermission` check, and audit-log writes. Model: `LIGHTCMS_COPILOT_MODEL` env (default claude-sonnet-4-6); requires `ANTHROPIC_API_KEY`.

### Environment variables added in v7
- `ANTHROPIC_API_KEY` — copilot + chat widget answer synthesis + CMS Agent AI commentary
- `RESEND_API_KEY`, `EMAIL_FROM` — outbound email for CMS Agent digests (Resend; config keys resend_api_key/email_from)
- `LIGHTCMS_COPILOT_MODEL` — copilot model override
- `LIGHTCMS_EMBEDDINGS_PROVIDER=ollama`, `OLLAMA_URL`, `OLLAMA_EMBED_MODEL` — local embeddings (Atlas vector index dims must match the model: voyage-4-lite 1024, nomic-embed-text 768)

### Public agent-facing endpoints (no auth)
- `/llms.txt`, `/llms-full.txt` — AI-crawler site index (generated on the fly, projected queries)
- `/mcp-public` — read-only MCP server for visitors' agents (search_site, get_page, list_pages, get_site_info); drafts/forks structurally excluded

### Deploy quirks
- A file the server writes at runtime that must outlast a deploy goes under `content/`, the Fly volume mounted at `/app/content`. `static/` is part of the image: `./deploy.sh` builds it from the deploying machine's checkout and every deploy replaces it. Admin editor uploads (image fields, OG images) moved from `static/uploads` to `handlers.UploadsDir` (`content/uploads`, still served at `/uploads/`) in 7.4.6; `MigrateLegacyUploads` copies files an image still carries in `static/uploads` at startup. Files the server regenerates (`static/sitemap.xml` when missing, `static/css/theme-vars.css` at startup) can stay in `static/`.
- The `fly deploy --detach` step inside `./deploy.sh` (never run `fly deploy` yourself) uploads a ~185MB build context (content/ + static/) with no visible progress — deploys take ~8 minutes and are NOT hung.
- `fly secrets set` does NOT restart the legacy machine (it is outside Fly release management, same reason deploy.sh exists). Secrets stay staged until `fly machines update d890122a371528 -a metavert-cms --yes` or the next `./deploy.sh` — a plain `fly machines restart` reuses the old machine config and does NOT inject new secrets (verify with `fly ssh console -C 'sh -c env'`).

## Script Policy

Site-wide setting `markdown_script_policy` (configurable via `update_site_config`):
- `"all"` — default; all roles may use raw HTML including `<script>` in markdown fields
- `"admin_only"` — editors' content is sanitized; admin content passes through unchanged
- `"none"` — all content sanitized regardless of author role

The sanitizer (bluemonday) strips: `<script>`, `<iframe>`, `<form>`, `<input>`, event handlers (`onclick=`, etc.), `javascript:` URIs. All other HTML is preserved.

## Build Commands

```bash
# Build main HTTP server
go build -o bin/lightcms ./cmd/server

# Build MCP server
go build -o bin/lightcms-mcp ./cmd/mcp

# Run main server (requires config.dev.json or env vars)
./bin/lightcms

# Run MCP server (stdio transport)
./bin/lightcms-mcp
```

## Project Structure

```
lightcms/
├── cmd/
│   ├── server/main.go      # Main HTTP server entry point
│   └── mcp/main.go         # MCP server entry point
├── config/config.go        # Configuration (env vars or JSON)
├── internal/
│   ├── auth/               # Session-based authentication
│   ├── database/mongo.go   # MongoDB connection & helpers
│   ├── errors/             # Environment-aware error handling
│   ├── handlers/           # HTTP request handlers (~5000 lines)
│   ├── mcp/                # MCP tool implementations
│   ├── middleware/         # Security headers, file validation
│   ├── models/models.go    # Data models & default templates
│   └── services/           # Business logic layer
├── templates/              # Admin UI HTML templates
├── static/                 # CSS, JS, images
└── content/generated/      # Static HTML output
```

## Key Architecture Patterns

### Service Layer
All business logic goes through services in `internal/services/`:
- **ContentService**: CRUD with automatic versioning, static page generation
- **TemplateService**: Template management with content regeneration
- **AssetService**: File uploads with validation
- **SettingsService**: Theme, config, redirects, folders, collections
- **UserService**: User CRUD, password management, credential validation
- **AuditService**: Sync/async audit log writes, filtered listing
- **APIKeyService**: API key management with user ownership

### Content Versioning
Every content update automatically creates a version. Versions are stored in `content_versions` collection. Use `revert_to_version` to restore previous versions.

**IMPORTANT:** When updating content via MCP, ALWAYS include a `version_comment` parameter with a concise description of what changed, even if the user doesn't explicitly provide one. This makes version history useful for tracking changes over time.

Examples:
- "Updated page title"
- "Revised introduction paragraph"
- "Added new section on security"
- "Fixed typo in heading"
- "Replaced hero image"

### Static Page Generation
Published content is rendered to `content/generated/{path}.html` using template HTML + theme header/footer. Regeneration happens automatically on:
- Content publish/update
- Template layout change
- Theme header/footer change

### Database Collections
- `content` - Content items with full_path (unique index)
- `content_versions` - Version history (includes modified_by, modified_by_email)
- `templates` - Content templates with fields + HTML layout
- `folders` - Content organization hierarchy
- `collections` - Content grouping by category
- `assets` - File metadata (binary stored on filesystem)
- `redirects` - URL redirect rules
- `settings` - Theme, config settings
- `users` - User accounts with email/password/role (unique email index)
- `audit_logs` - Audit trail (who did what, when; 365-day TTL)
- `api_keys` - API keys with optional user_id ownership
- `contact_messages` - Form submissions
- `login_attempts` - Rate limiting data

## Code Conventions

### Naming
- Services: `ContentService`, `TemplateService`
- Handlers: `CreateContent`, `UpdateTemplate`
- DB helpers: `FindOne`, `UpdateOne`, `InsertOne`
- Receivers: `h` (Handler), `s` (Service), `db` (DB)

### Error Handling
```go
return fmt.Errorf("context description: %w", err)
```

### MongoDB Patterns
```go
// Filters use bson.M
filter := bson.M{"_id": id, "deleted": bson.M{"$ne": true}}

// Updates use bson.M with operators
update := bson.M{"$set": bson.M{"title": title, "updated_at": time.Now()}}

// Sorting uses bson.D for order
opts := options.Find().SetSort(bson.D{{Key: "created_at", Value: -1}})
```

### Context
Always pass context as first parameter for DB/service methods.

## Multiuser RBAC System (v2.0+)

### Roles
- **admin**: Full access — manage users, templates, settings, audit logs, all API keys
- **editor**: Create/edit/delete/publish content, upload/delete assets, forks, imports, approvals, manage own API keys. Not templates, snippets, collections, folders, redirects, theme or site configuration (admin only, in the UI as in the API)
- **contributor**: Create content and save drafts (publishing submits for approval), upload assets (pending review), comment, manage own API keys
- **viewer**: Read-only access to content, templates, assets, settings

### Auth Flow
- Login with email + password (migrated from single-admin password)
- Session stores: user_id, user_email, user_role
- Force password change on first login with temporary password
- API keys carry the permissions of their owning user

### Migration
On first startup with empty `users` collection, creates admin user from existing `settings.admin` password hash. Set `LIGHTCMS_ADMIN_EMAIL` env var (default: `admin@localhost`).

### Password Reset
```bash
go run cmd/resetpw/main.go [email]  # Reset specific user, or first admin if no email given
```

## Security Notes

- CSRF protection on all `/cm` routes (Gorilla CSRF, built in `middleware.CSRFProtect`). gorilla/csrf assumes HTTPS for its Origin/Referer check; requests are marked plaintext only when `secure_cookies` is false AND the connection has no TLS. Never key this on a request header.
- **Session CSRF rule for `/api/v1` and `/mcp` (7.4.2):** a request authenticated by the session cookie (no `Authorization` header) with an unsafe method must pass `middleware.CSRFProtectAPI` — the same gorilla token the `/cm` pages are rendered with, sent as `X-CSRF-Token`, same cookie (`lightcms_csrf`, path `/`), same key, same Origin/Referer check. API keys and OAuth tokens never reach the check; GETs are exempt. In admin templates every state-changing `fetch()` passes `headers: csrfHeaders({...})` (defined in `adminLayoutStart`); `TestServedTemplates_UnsafeFetchSendsCSRFToken` fails on one that does not. Do not send state-changing requests any other way (XHR, `sendBeacon`, a form posting to `/api/v1`).
- HSTS is decided by configuration (`middleware.HSTSEnabled`: secure cookies and an `https://` base URL), never by `X-Forwarded-Proto`.
- Admin CSP (`middleware.SecurityHeaders`) is `script-src 'self' 'unsafe-inline'`: a script or stylesheet from another origin is silently blocked. Vendor third-party admin assets under `static/admin/` with a README recording version, source URL and SHA-256 (see `static/admin/quill/`); `TestAdminTemplates_ExternalAssetsAllowedByCSP` enforces it.
- No native browser dialogs anywhere (`alert`/`confirm`/`prompt`): use `data-confirm` (+ `data-confirm-title`) on a form, or `showConfirm()` / `showAlert()` from `adminLayoutStart`. Both take HTML — wrap anything that is not a fixed string in `dialogText()`. `TestServedTemplatesAndJS_NoNativeDialogs` scans every served template and JS file.
- HTML forms cannot nest: the parser drops the inner `<form>` and its `</form>` closes the outer one. Put the second form outside and point the button at it with `form="id"` (see the webhook edit page).
- Session cookies: SameSite=Strict, 24-hour expiry, Secure in production
- File uploads: Extension whitelist + MIME validation
- Path traversal protection on all file operations
- Login rate limiting: Escalating lockout (10→1min, 15→5min, 20+→15min)
- Passwords: bcrypt with cost=12
- A request that changes what is published (create or update with `published`, bulk create, imports and auto-publishing import sources, revert, restore, `publish_new`) needs `content.publish`, whatever path it takes.
- RBAC: every REST endpoint calls `requirePermission`, and every `/cm` route is declared in `handlers.AdminRoutes` (`internal/handlers/admin_routes.go`) with the permission it needs — the one its `/api/v1` equivalent requires. `RegisterAdminRoutes` is the only place `/cm` routes are registered and puts the check in front of the handler (styled 403 page for forms and pages, JSON 403 for routes marked `JSON`). To add an admin route, add a line to that table; do not call `admin.HandleFunc` in `main.go`.
  - `TestAdminRoutes_EveryWriteRouteIsGuarded` walks the registered router and fails if a non-GET `/cm` route has no permission and is not in `writeRoutesWithoutPermission` (login, logout, the two own-password routes, each with its reason). `TestAdminRoutes_RoleMatrix` sends every route as viewer, contributor, editor and admin against a hand-written expectation (`adminRoleMatrix`): changing who may call a route means changing that table on purpose.
  - Rules the route table cannot express live in the handler: `UpdateContent` lets a contributor (`content.create` without `content.edit`) save a live draft only; `DeleteContent` needs `content.delete`, or `fork.create` for a fork copy; `DeleteAPIKey` only revokes the caller's own key below `apikey.manage_all` (the API route answers 403 for someone else's key and 404 for an admin deleting one that does not exist — `APIKeyService` deletes return `ErrAPIKeyNotFound` when nothing matched); `SiteConfiguration` leaves the Cloudflare token out of the page below `settings.edit`.
  - Templates get `.Can` (the role's permissions) from `renderAdmin`: hide a control with `{{if index $.Can "settings.edit"}}`.
  - Known difference: the admin webhook pages require `content.edit` while the webhook API requires `settings.edit`.
- Audit logging on all mutations (async, 365-day TTL auto-cleanup). For `/cm` routes it is declared, not hand-written (7.4.3): a route that changes state carries `.audited("resource.action")` in `AdminRoutes` — the action name the `/api/v1` equivalent logs — and `withAdminAudit` (`internal/handlers/admin_audit_wrap.go`) writes the entry after the handler returns, with the session user and the route's `{id}` (other route variables become details).
  - The wrapper decides success from the response: status below 400, not a redirect to `/cm/login` or to a URL with `error=`, and no page rendered with an `"Error"` in its data (`renderAdmin` marks that). A handler that returns a plain success response without changing anything must call `auditSkip(r)`; a create handler passes the new id with `auditResource(r, id.Hex())`; `auditDetail(r, k, v)` adds details.
  - `TestAdminRoutes_EveryWriteRouteIsAudited` fails for a non-GET route with neither an action nor an entry (with its reason) in `writeRoutesNotAudited` — the handlers that log their own entry because the action or details depend on the outcome (sign-in, passwords, users, search-and-replace, SEO/IndexNow/agent tools, fork merge, copilot).
- Outbound requests to a URL that a user configured or that comes from content (webhook targets, links in pages, assets by URL) go through `internal/netguard` (`netguard.NewClient` / `netguard.Transport`): the host is resolved at dial time, any loopback, link-local, private or reserved address refuses the request with `netguard.ErrBlockedAddress`, and the checked address is the one dialled. Never give such a request a plain `http.Client` or `http.DefaultClient`. The services package reaches it through `outboundTransport`, which its `TestMain` swaps so the tests' receivers on 127.0.0.1 work; a test of the guard calls `useGuardedTransport(t)`. RSS import fetches through `importer.FeedClient`, guarded the same way and swapped in the same `TestMain`.
- A goroutine started from a request handler must not use the request's context for work that outlives the response: it is cancelled when the handler returns. `WebhookService.FireEvent` uses `context.WithoutCancel` plus its own timeout (7.4.3; before that most webhooks were silently dropped).
- A response rendered for a fork preview (`getActiveForkPreview(r) != nil`) is `Cache-Control: private, no-store` — never the public cache headers or an ETag. Keep that in any new public render path.
- Never redirect to a value taken from the request (a `Referer`, a `next`/`return` parameter) as given: pass it through `sameSitePath(r, raw, fallback)` (`handlers_fork.go`), which keeps only a path on this site.
- JavaScript in admin templates never builds an inline event handler with data in it (`'<div onclick="f(\'' + value + ...`): create the element with DOM calls, put values in `dataset`, attach a listener. `TestServedTemplatesAndJS_NoInlineHandlersBuiltFromData` scans for it. A confirmation message goes in `data-confirm` on the `<form>` — nothing reads `data-message` or a `data-confirm` on a button (`TestAdminTemplates_NoDeadConfirmAttributes`, `TestAdminTemplates_DeleteFormsAskFirst`).

## Configuration

**Environment Variables (Production):**
- `MONGO_URI` - MongoDB connection string
- `DATABASE_NAME` - MongoDB database name (default: `lightcms`; also `database_name` in the JSON config, env wins). Read by the server and every `cmd/` tool through `config.ResolveDatabaseName`; logged at startup
- `SESSION_SECRET` - 32+ char secret
- `BASE_URL` - Public URL (e.g., https://example.com)
- `PORT` - Server port (default: 80)
- `ENV` - "production" or "development"
- `SECURE_COOKIES` - "true" for HTTPS (default in env mode; set "false" only for plain-HTTP local runs)

Values are trimmed of surrounding whitespace when read (`config.env`, `Config.trim`), so an env file with CRLF line endings works; `SESSION_SECRET` is the exception and is used exactly as given. Read a new setting through `env()`, not `os.Getenv`.

**JSON Config (Development):**
`config.dev.json`:
```json
{
  "port": "8082",
  "mongo_uri": "mongodb+srv://...",
  "env": "development",
  "session_secret": "dev-secret-change-in-prod",
  "base_url": "http://localhost:8082",
  "secure_cookies": false
}
```

## Default Templates

7 built-in system templates:
1. Blog Post
2. Press Release
3. Explanatory Page
4. Blank Page
5. Homepage
6. Concept Page
7. Standard Page

Template fields support types: text, textarea, richtext, date, image, select

## Bulk Operations & Programmatic SEO

LightCMS supports large-scale content operations (2,000+ pages) via optimized bulk APIs. These guidelines apply to programmatic SEO, mass content migration, site-wide link fixing, and similar large-scale tasks.

### Bulk Content Creation
- Use `bulk_create_content` (up to 100 items/call) instead of calling `create_content` in a loop
- Uses MongoDB `InsertMany` with unordered mode — one failure doesn't abort the batch
- Published items get parallel HTML generation (10 concurrent)
- Set `upsert: true` to update existing pages instead of failing on duplicates
- Example: creating 2,000 pages = 20 batch calls of 100

### Content Upsert (Idempotent Creates)
- `create_content` accepts `upsert: true` — if a page exists at the same path, it updates instead of failing
- Eliminates the most common failure mode in retry scenarios
- `bulk_create_content` also supports `upsert: true` for batch idempotency

### Bulk Search & Replace
- `search_replace_preview` and `search_replace_execute` accept a `pairs` array for multi-pair mode
- Each page is scanned once with all pairs applied in order — O(pages) instead of O(pairs × pages)
- Critical for operations like fixing hundreds of broken links in a single pass
- Returns `pages_scanned`, `pages_modified`, `total_replacements` counts
- Pairs are applied in array order — replacement from pair 1 may create text that pair 2 matches

### Conditional Republish
- Static HTML generation uses content hashing (SHA-256) — unchanged pages are skipped automatically
- `RegenerateAllContent` (triggered by theme/template changes) clears all hashes first to force full regen
- For search/replace with `auto_republish: true`, only modified pages get republished

### Content List Pagination
- `list_content` supports `limit` and `offset` parameters for paginated results
- Returns `{items, total, limit, offset, has_more}` envelope when `limit` is set
- Default (no limit) returns all items — backward compatible
- Max limit: 500 per request
- For sites with 2,000+ pages, use pagination to avoid large JSON responses

### Rate Limits
- API: 300 requests/minute per bearer token (sliding window)
- Burst: 20 requests/second per bearer token (prevents runaway scripts)
- Bulk endpoints: additional per-endpoint limits (regenerate, search/replace, bulk update)
- When rate limited, response includes `Retry-After` header

### Best Practices for Large-Scale Operations
1. **Use batch APIs** — `bulk_create_content` and `bulk_update_content` over individual calls
2. **Use multi-pair search/replace** — preview first, then execute with all pairs in one call
3. **Set `auto_republish: true`** on search/replace to avoid a separate publish step
4. **Include `version_comment`** on all bulk operations for readable version history
5. **Use `upsert: true`** for retry-safe content creation (idempotent)
6. **Paginate list_content** with `limit: 100` to avoid loading 2,000+ items at once
7. **Don't exceed 100 items per bulk call** — this is enforced server-side

## Deployment

Deployed to Fly.io (`metavert-cms` app, machine `d890122a371528`). Uses environment variables for configuration. Health check at `/health`.

> **⚠️ NEVER run plain `fly deploy` — it creates orphan machines and does not update the live one. The only deploy command is `./deploy.sh`.**

**Deploy procedure — always use the deploy script:**
```bash
./deploy.sh
```

The script builds the image via `fly deploy --detach`, extracts the image ref, destroys any stuck new machines, then updates the actual running machine directly via `fly machines update`. This is necessary because the running machine (`d890122a371528`) is a legacy non-Launch machine — `fly deploy` doesn't see it as a managed machine and creates new stuck orphan machines instead of updating it. The machine ID is hardcoded in `deploy.sh`.

## Testing Locally

1. Create `config.dev.json` with MongoDB Atlas connection
2. `go build -o bin/lightcms ./cmd/server`
3. `./bin/lightcms`
4. Access admin at http://localhost:8082/cm (default: admin@localhost / admin123)
