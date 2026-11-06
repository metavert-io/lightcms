# Changelog

All notable changes to LightCMS are documented here, organized by version.

---

## [7.4.6] - 2026-11-06

### Fixed
- **Images uploaded in the admin editor survive a deploy.** An image field or OG image uploaded through the page form was written to `static/uploads` inside the running image, which every deploy replaces, so the picture disappeared at the next deploy and the page showed a broken image, unless the machine that ran the deploy happened to have a copy of the file. These uploads are now written to `content/uploads`, on the persistent volume, and `/uploads/` is served from there; their URLs are unchanged. At startup the server copies any file the image still carries in `static/uploads` into `content/uploads` (never overwriting one), so files baked into earlier images keep working. Asset-library, API and MCP uploads were already on the volume.

### Internal
- **Docker base images are pinned by digest** in `Dockerfile`, `Dockerfile.mcp` and `Dockerfile.smithery`, with the tag each was resolved from in a comment above it. `Dockerfile.smithery` builds again: its Go 1.24 image refused this module's `go 1.25.0` requirement. It now uses the same Go 1.25.14 as the other two, and Alpine 3.23 instead of `alpine:latest`.

---

## [7.4.5] - 2026-10-16

### Changed
- **LightCMS moved to `github.com/metavert-io/lightcms`.** The Go module path is now `github.com/metavert-io/lightcms/v7`: use `go get github.com/metavert-io/lightcms/v7` and update imports. The old `github.com/jonradoff/lightcms` address still redirects for browsers and git, but Go refuses a module whose `go.mod` path does not match where it was fetched from, so the new path is required from this version on. Earlier versions stay available under the old path.
- **The MCP Docker image is published to `ghcr.io/metavert-io/lightcms-mcp`.** `ghcr.io/jonradoff/lightcms-mcp` stays available at its last build and gets no further updates.
- `server.json` points at the repository and release downloads under `metavert-io`.

No runtime changes.

---

## [7.4.4] - 2026-10-09

### Security
- **An OAuth authorization request must name its `redirect_uri`.** `/oauth/authorize` compared `redirect_uri` with the client's registered list only when the parameter was present; a request without one passed the client check. A missing `redirect_uri` is now refused with `invalid_client` ("redirect_uri is required").
- **The sample script on the Search tool page builds its results as elements.** The copy-and-paste example wrote titles, keywords and snippets into `innerHTML` (and a keyword into an inline `onclick`), so a site that pasted it would run markup from a page title. The example now creates the elements and sets their text. Sites that copied the earlier example should copy it again.

### Changed
- **A page deleted through the API or MCP is unpublished in the database, and restoring it publishes it again.** `DELETE /api/v1/content/{id}` (`delete_content`) used to leave `published: true` on the deleted row; 7.4.3 made every public query skip deleted rows, and the row itself now says the same. The delete records that the page was published, and `restore_content` (and an agent-session rollback) brings it back published as before — unless it was put on hold in the meantime, in which case it comes back as a draft. Pages deleted before 7.4.4 restore as they always did.

### Internal
- The remaining live-content queries that matched `fork_id: {$exists: false}` now match `fork_id: nil` (a missing or null field), the form the rest of the code uses.

---

## [7.4.3] - 2026-10-07

### Security
- **A page deleted through the API or MCP is no longer served.** `DELETE /api/v1/content/{id}` (and `delete_content`) soft-deletes a page and leaves its published flag and path in place. The public page handler looked pages up by path without excluding deleted ones, so a deleted published page kept answering `200` at its URL, rendered from the database, until it was unpublished or purged. Public page lookups (exact path, legacy slug, case-insensitive redirect) and the 404 page are now live-only: never a soft-deleted page, never a fork copy. Pages deleted in the admin editor were not affected (it unpublishes and renames the path).
- **Public collection pages and `lc:query` index pages list live pages only.** A collection page (`/<collection-slug>`) listed every page in its category with the published flag set, including soft-deleted pages and fork copies left flagged published by versions before 7.4.1. `lc:query` directives and the `[[wikilink]]` index excluded deleted pages but not fork copies. All three now require a live, published, non-deleted page.
- **Webhook delivery goes through the SSRF guard.** Webhook URLs are user-configured (editor and above) and were requested with a plain HTTP client, so a webhook could be pointed at the server's own network: loopback, private ranges, or a cloud metadata address. Delivery now uses the guard the broken-link scan and asset-from-URL already use: the host is resolved at connect time, the request is refused if any address it resolves to is loopback, link-local, private or reserved, and the connection is made to the address that was checked (so DNS rebinding does not get past it). Webhooks to public URLs are unchanged. **A refused delivery is recorded in the webhook's delivery log** as failed, with "blocked: the webhook URL resolves to a private, loopback or link-local address", and is not retried. The guard now lives in `internal/netguard`; the link checker service's client uses it too.
- **Fork preview pages are never cached.** A page viewed in fork preview was sent with the public cache headers (`Cache-Control: public, max-age=300, s-maxage=3600` and an ETag), so the browser kept showing the fork's copy, preview bar included, after Exit Preview, and a CDN caching HTML in front of the site could have stored unpublished preview content and served it to visitors. A response rendered for a preview is now `Cache-Control: private, no-store` with no ETag. Pages served without the preview cookie are unchanged.
- **RSS import sources are fetched through the SSRF guard.** A feed URL is user-configured and its response is imported into pages; it was fetched with a plain client, so a source could be pointed at the server's own network. A feed URL that resolves to a loopback, link-local or private address now fails with a blocked-address error.
- **"Exit Preview" no longer redirects to wherever the `Referer` header says.** `/cm/forks/exit-preview` sent the browser to the `Referer` as given, so a link to it from another site bounced the visitor back out to that site (an open redirect on the CMS's domain). It now returns to the referring page only when that page is on this site, as a path, and otherwise goes to the forks list.
- **The approver picker on the Approvals page is built without inline handlers.** Search results were written as HTML with the user's email concatenated into an `onclick` attribute and into the markup (the same pattern as the @mention dropdown fixed in 7.4.2), so an email containing a quote or markup would run as script for the admin creating a workflow. Results and chips are now DOM nodes with the email as text and in a data attribute, behind one listener. The new comment's Delete button and the chat-widget preview's hover colour were built the same way and are fixed the same way.
- **Admin-UI changes are recorded in the audit log.** Only sign-in, user management, search-and-replace and a few tool pages were logged from the admin UI, although the API logs the same changes. Creating, updating and deleting templates, pages, collections, folders, redirects, snippets, assets, API keys, webhooks, import sources and forks, restoring, reverting, regenerating and re-templating a page, saving or reverting the theme, saving site configuration and the search and chat settings now write an entry with the action name the API uses (`template.update`, `content.delete`, `theme.update`, `config.update`, ...), the signed-in user and the resource id. The action is declared on the route in `handlers.AdminRoutes` and written by the route wrapper after the handler succeeds, so a handler cannot forget it. A refused request, a validation error and a request that changed nothing write no entry.

### Fixed
- **Fork copies written through the API have no effect outside the fork.** Saving a fork copy with a changed title or path rewrote `[[wikilinks]]` in **live** pages to the copy's new title or path, and creating or saving a copy fired the `content.create` / `content.update` webhooks (bulk create too). Service `CreateContent`, `BulkCreateContent`, `UpdateContent` (and so `RevertToVersion` and upserts) now stop before every side effect for a copy: no webhook, no wikilink rewrite, no keyword rebuild, no index-page regeneration, no static file, embedding, IndexNow ping or Cloudflare purge.
- **Webhooks fired from a web request are delivered.** `FireEvent` looked up and called the webhooks on the request's context, which is cancelled as soon as the response is sent, so a webhook for a change made through the API, MCP or admin UI was usually dropped without a delivery-log entry. Delivery now runs on its own 60-second deadline. **If you have active webhooks, expect them to start receiving events they were missing.**
- **A fork copy no longer blocks, or is blocked by, a live page's path in the editor.** The duplicate-path checks when saving or restoring a page, and the slug check the editor runs while typing, matched fork copies: a live page could not be renamed or restored onto a path that existed only as a copy in some fork ("A page already exists at ..."), and a copy was compared against live pages. Live pages are now compared with live pages, and a fork copy with the other copies in its own fork.
- **API key and snippet Delete buttons ask before deleting.** Their confirmation text was in an attribute nothing read, so one click deleted. Both use the styled confirmation now.
- **`DELETE /api/v1/api-keys/{id}` no longer answers `{"success": true}` when nothing was deleted.** Deleting a key that belongs to someone else (or does not exist) without `apikey.manage_all` now returns `403`, as the admin UI does; an admin deleting a key that does not exist gets `404`. No `apikey.delete` audit entry is written for either.
- **Renaming a folder can no longer remove the homepage's generated file.** Moving a folder's pages removed each page's old file by path; a legacy page with an empty path mapped to the homepage's `index.html`, and a fork copy filed in the folder mapped to its live page's file. Both are now skipped.
- **Configuration values are trimmed.** `MONGO_URI`, `BASE_URL`, `PORT`, `ENV`, `SECURE_COOKIES`, `DATABASE_NAME`, the API keys, `EMAIL_FROM`, `LIGHTCMS_ADMIN_EMAIL`, `LIGHTCMS_URL` and `LIGHTCMS_API_KEY` (and the same fields in the JSON config file) lose surrounding whitespace. A value from an env file with Windows line endings ended in a carriage return: the MongoDB connection string did not connect, and `SECURE_COOKIES=false` was read as true. `SESSION_SECRET` is used exactly as given.
- **Renaming a folder no longer moves pages in a sibling folder whose name starts the same way.** Renaming `/blog` matched every folder path beginning with `/blog`, so pages in `/blogging` were rewritten too. Only the folder itself and folders below it are moved.
- **Disable Account and Reset Password ask before acting** on the user edit page.
- **Webhook and import changes made through the API are audit-logged** (`webhook.create|update|delete|regenerate_secret`, `import_source.create|update|delete|trigger`, `import.markdown`, `import.csv`), under the same action names as the admin UI.
- The content list's "Search failed" message and the duplicate-path redirect escape the text they include.
- Backlinks and the wikilink link check count live pages only.

### Changed
- **Removed three admin routes that nothing called:** `POST /cm/upload` (the image upload endpoint of the rich-text editor used in 1.x; it wrote to `static/uploads` outside the asset library and its review step), and `POST /cm/content/{id}/lock/refresh` and `/lock/force` (the admin editor has never taken content locks, so there was nothing to refresh or break from it). Files already under `/uploads/` are still served. Content locking is unchanged in the API and MCP (`/api/v1/content/{id}/lock`, `/lock/force`; `acquire_content_lock`, `force_unlock_content`).
- A webhook whose URL points at a private, loopback or link-local address stops delivering (see Security). Point it at a public address, or put a public relay in front of an internal receiver.
- Tests fail the build if a `/cm` route that changes state has neither an audit action nor an entry in an allowlist with its reason, if a delete form does not ask first, if a template carries a confirmation attribute nothing reads, or if JavaScript in a served template builds an inline event handler with data concatenated into it.

---

## [7.4.2] - 2026-10-07

### Security
- **Every admin route checks a permission.** 33 state-changing routes under `/cm` only checked that someone was signed in, so any role could call them: a viewer could create, edit and delete templates, snippets, collections, folders and redirects, save or revert the theme, save site configuration, create and edit pages, restore, regenerate, re-template and revert pages, upload and delete assets, and revoke **any** user's API key. Every `/cm` route is now declared in one table (`handlers.AdminRoutes`) with the permission it needs, the same one its `/api/v1` equivalent requires, and the check runs in front of the handler. A refused form post or page gets a styled "Not permitted" page (403); a refused fetch gets a JSON 403. Nothing is written.
  - **What each role can no longer do in the admin UI** (all of this was already refused by the API):
    - **Editors** can no longer create, edit or delete **templates, snippets, collections, folders and redirects**, save or revert the **theme**, save **site configuration**, or rebuild the search index. These need `template.*` / `settings.edit`, which only admins hold. Editors keep everything on content, assets, forks, imports, webhooks, approvals and comments.
    - **Contributors** can no longer edit a **published** page, a fork copy or a deleted page (saving a published page as a contributor silently unpublished it), and cannot delete, restore, regenerate, re-template or revert pages. They still create pages, save and resubmit their drafts, upload assets, comment, manage their own API keys and see the approvals page.
    - **Viewers** can no longer change anything, and no longer see the API keys and approvals pages (they have no key or approval permission).
  - Revoking an API key in the admin now follows the API's rule: your own keys, or any key with `apikey.manage_all` (admin).
  - The configuration page no longer puts the **Cloudflare API token** into the page for roles that cannot save it. Before, any signed-in user could read it from the form.
  - Buttons a role cannot use (New Template, New Snippet, New Collection, New Folder, New Redirect, Upload Asset, New Content, Save Theme, Save Configuration and the search and chat settings) are no longer shown to it.
- **Session-authenticated API requests need the CSRF token.** `/api/v1` (and `/mcp`) accept the admin session cookie so admin pages can call them. Those requests had no CSRF defence beyond the cookie's `SameSite=Strict`, which does not cover another origin on the same site (a different port or subdomain). A request authenticated by the session cookie that uses POST, PUT, PATCH or DELETE must now send the token the admin pages are rendered with as `X-CSRF-Token`, and passes the same Origin/Referer check as the `/cm` forms; otherwise it gets a JSON 403. **API keys and OAuth tokens are not affected**, and neither are GET requests. The admin UI's own calls (comments, approvals, approval workflows) send the token. The CSRF cookie is now site-wide and named `lightcms_csrf`; an admin page left open across the upgrade needs one reload before its forms submit.
- **The broken-link scan requires `content.edit`** (it was open to any signed-in role) **and no longer contacts internal addresses.** The scan fetches every external link found in published content from the server; a link, or a redirect, to a loopback, private or link-local address is now reported as broken without being requested.
- **`Strict-Transport-Security` follows server configuration** (secure cookies and an `https://` base URL) instead of the `X-Forwarded-Proto` request header, which any client can send.

### Fixed
- **Unpublishing a page in a folder no longer removes another page's file.** The admin editor removed `content/generated/<slug>.html` instead of the page's full path, so unpublishing `/docs/about` deleted the generated file of a root page `/about` and left its own behind. Deleting a legacy page that has no path could remove the homepage's file the same way. Both now go through the service's removal by full path.
- Admin Search and Replace is recorded in the audit log (`content.search_replace`, with pages and replacement counts), as the API's always was.
- Deleting or unpublishing a **fork copy** through the API no longer fires the `content.delete` / `content.unpublish` webhook or a Cloudflare purge for the live page at that path.
- Import lookups by path and by source URL, and the edit links on the analytics pages, resolve live pages only. They could land on a fork copy sharing the path.
- "Delete Page" no longer says "This cannot be undone": a deleted page is unpublished and kept under deleted content, where Restore brings it back.
- **@mentions in comments work.** The dropdown never opened (the user list was read in the wrong shape) and each entry's click handler was malformed. Entries are now built without inline handlers, so a name containing quotes or markup is just text.
- The delete and revert confirmation dialogs show their message as text, like the other dialogs.
- A message that opens over the search dialog on the content list is now layered correctly: it takes the keyboard, and Escape or Enter act on it rather than on the dialog underneath.
- The buttons in a fork's page header stay on one line when the fork has a long name.
- `resetpw` and `migrate-hourly-bots` read `MONGO_URI` from the environment, as their error message said they did (the config files are still read when it is not set).
- The test suites no longer rewrite tracked files (`static/sitemap.xml`, `static/css/theme-vars.css` under the test packages); a run that modifies a tracked file now fails.

### Added
- **Repair for damage from before 7.4.1.** The admin editor could leave a fork copy marked published, and could remove a live page's generated HTML. `POST /api/v1/maintenance/repair-fork-damage` (`?dry_run=true` to only list) and the `repair_fork_damage` MCP tool list the fork copies flagged published and the live published pages whose generated file is missing, then clear the flags and regenerate the files. Admin only; real runs are recorded in the audit log. Page content is not changed: no versions are saved and search engines are not pinged. For a page that exists only inside a fork the flag meant "publish on merge"; after a repair, merge that fork with "Publish new pages" to publish it. 130 MCP tools total. Only fork copies that have a live page at the same path have the flag cleared; a page that exists only inside a fork is reported with `has_live_page: false` and keeps its flag, which there means publish on merge.
- Tests fail the build if a `/cm` route that changes state is registered without a permission (outside a four-entry allowlist of sign-in and own-password routes), if a role reaches a route it should not (a role matrix over every route), or if a served template sends a state-changing request without the CSRF token.

---

## [7.4.1] - 2026-10-07

### Fixed — fork copies, follow-ups to 7.4.0
- **Search-and-replace no longer rewrites fork copies.** Global and scoped search-and-replace (preview and execute; API, MCP and the admin Search and Replace screen) now work on live pages only. They used to match and rewrite the working copies inside forks as well. The admin screen now runs through the same service path as the API: each rewritten page gets a version recording who ran it and what was replaced (the comment used to be a fixed "Updated via bulk link replacement"), and published pages stay published and are regenerated.
- **A path never resolves to a fork copy.** `GET` and `PUT /api/v1/content/by-path` (`get_content` by path, `update_content_by_path`), the copilot's get-by-path and upsert-by-path used to fall through to a fork copy when no live page existed at the path, so an update could land on a copy. They now resolve live pages only and return 404 for a path that exists only inside a fork. To reach a fork's copy by path, name the fork: `?path=/x&fork_id=<fork id>`. Agent sandbox sessions are unchanged: `update_content_by_path` inside a sandbox still writes to the sandbox copy, including pages created in the sandbox. Fork preview is unchanged.
- **Scoped operations report fork copies instead of silently skipping them.** `bulk_field_operation`, `export_content` and scoped search-and-replace return a `skipped` array of `{id, reason}` for fork copies named in `content_ids`.
- `reindex_embeddings` and the embedding statistics no longer count fork copies.
- **Working on a fork copy in the admin editor no longer touches the live page.** A fork copy shares its path with the live page, and the editor treated it as that page:
  - Saving a copy with **Published** ticked marked the copy published and overwrote the live page's generated HTML with the fork draft, which was then served to visitors. Published is now disabled on a fork copy (it goes live by merging the fork) and ignored if sent.
  - Saving a copy as a draft, or deleting it, removed the live page's generated HTML. The live page kept serving, rendered from the database on each request. Deleting also removed the redirects pointing at the live page.
  - Changing a copy's slug removed the live page's generated HTML, rewrote links to it in other pages and could create a redirect away from it; changing its title rewrote wikilinks across the site. None of that happens for a copy now.
  - **Delete Page** on a fork copy is now **Remove from Fork**: it removes the copy from its fork, exactly as Remove on the fork page does, and returns to the fork.
  - The same guard covers Regenerate, Change Template and Revert Version on a copy, a copy deleted through the API or `delete_content`, and the database change watcher, which removed the live page's generated HTML whenever a copy was updated by any route (editor, API, MCP, agent sandbox).
- Admin: merging or archiving a fork and removing a page from a fork now ask through the styled confirm dialog instead of a browser `confirm()`, and the empty-query warning in search-and-replace is a styled message instead of a browser `alert()`.

### Fixed — admin UI
- **The rich-text editor works again.** The admin Content-Security-Policy (`script-src 'self'`, added in v1.1) blocked the Quill editor, which was loaded from a CDN, so rich-text fields showed only the raw "Edit HTML" box. Quill 2.0.3 is now vendored in `static/admin/quill/` (byte-identical to upstream, with licence, source URLs and SHA-256 hashes in the README beside it) and served from the site itself. The policy is unchanged.
- **Admin forms work over plain HTTP in local development.** With `secure_cookies: false`, every admin POST on `http://localhost` failed with "Invalid or missing CSRF token" because the CSRF library assumes HTTPS when it checks the request origin. Requests are now treated as plain HTTP when secure cookies are off and the connection has no TLS. Production (`secure_cookies: true`) keeps the strict HTTPS check. The log line for a rejected request now includes the reason.
- **No more native browser dialogs in the admin.** Deleting a comment, reverting a theme version, deleting a webhook, regenerating a webhook secret, deleting an RSS source, approving a request and deleting an approval workflow now ask through the styled confirm dialog. Comment, approval and workflow errors, the "Comment required" and "Name is required" checks, and "URL copied" in the asset library are styled messages. Server error text in these messages is shown as text, never as HTML.
- **Webhook edit page: "Regenerate Secret" and "Save Changes" do what they say.** The regenerate form was nested inside the edit form, which browsers do not allow: "Regenerate Secret" saved the webhook instead of regenerating, and the event checkboxes, the Active box and "Save Changes" were cut out of the form.
- **"Delete Page" in the content editor deletes the page.** Its form was nested inside the edit form, which browsers do not allow, so the button submitted Update and the page was saved instead of deleted. The delete form now sits outside the edit form, with the button still beside Update and the same styled confirmation.
- **A comment you just posted has its Delete button straight away** (admins). The role was double-quoted on its way into the page script, so the button appeared only after a reload.
- **Search-and-replace messages show server text as text.** The "Replace Failed" and "Replace Complete" dialogs put the server's error message, the exception text and the page count into the dialog as HTML; they are now escaped like the other dialogs.

### Security
- **Admin Search and Replace is limited to administrators.** The two endpoints behind the content list's Search and Replace only checked that someone was signed in, so a viewer, contributor or editor could preview and run a site-wide replace. They now require the `search_replace.execute` permission the API endpoints require (admin only) and answer 403 otherwise; the screen shows the refusal as a styled message.
- **Search and Replace and the broken-link "Fix" are behind CSRF protection.** Both wrote content from routes under `/api/`, outside the CSRF-protected `/cm` routes, relying on the session cookie's `SameSite=Strict` alone. They moved to `/cm/replace/preview`, `/cm/replace/execute` and `/cm/tools/broken-links/fix`, and the admin pages send the CSRF token. The old `/api/content/replace-preview`, `/api/content/replace-execute` and `/api/tools/fix-link` routes are gone. The broken-link fix now also requires `content.edit`.
- **Deleting a page in the admin requires `content.delete`** (editor or admin). The handler only checked that someone was signed in, so a viewer or contributor could delete any page. Removing a copy from a fork requires `fork.create`, as it does on the fork page.
- A test reads the server's route table and fails the build if an admin route that changes state is registered outside the CSRF-protected `/cm` routes.

### Added
- **`DATABASE_NAME`** environment variable (and `database_name` in the JSON config) selects the MongoDB database for the server and the `cmd/` tools (`resetpw`, `addchat`, `addchat-spa`, `migrate-hourly-bots`). Default `lightcms`, so existing installs are unaffected. The server logs the database in use at startup.

### Changed
- README tool counts corrected (129 tools, with the full category breakdown).
- Tests fail the build if an admin template loads a script or stylesheet the admin CSP does not allow, if any served template or JS file calls `alert()`, `confirm()` or `prompt()`, or if the vendored Quill files differ from their recorded hashes.
- Tests also fail the build if a form is nested inside another form in any served template, if a template pre-quotes a value with `printf "%q"`, or if a `showAlert`/`showConfirm` message is neither a fixed string nor wrapped in `dialogText()`.

---

## [7.4.0] - 2026-10-07

### Added — draft safety
- **Hold flag.** Any page can be put on hold (`hold: true` on the create and update API, the `create_content`, `update_content` and `update_content_by_path` MCP tools, or the Hold checkbox in the editor). A held draft cannot be published by any path until the flag is cleared: single publish and updates that set `published` return 409, bulk publish and scheduled publishing skip it, an approval leaves it a draft, and a fork merge creates it as a draft. Holding a page that is already published does not unpublish it. The content list shows a "Held" badge. No migration: existing pages are not on hold.
- **Merges can publish what they create.** `POST /api/v1/forks/{id}/merge` accepts `{"publish_new": true}`, the `merge_fork` MCP tool accepts `publish_new`, and the admin merge button has a "Publish new pages" checkbox. Pages the merge creates are then published through the normal publish path. Held pages stay drafts and are listed in `not_published`. The default is unchanged: new pages keep the publish state they had in the fork.
- Merge results now include `created_ids` and `updated_ids` (the live pages).
- **Purge for already-merged forks:** `POST /api/v1/forks/{id}/purge-copies` (with `?dry_run=true` to list `{id, full_path}` without deleting) and the `purge_fork_copies` MCP tool delete the page copies still attached to a merged or archived fork. Admin only, refused for active forks, and recorded in the audit log. 129 MCP tools total.
- `include_forks` on `GET /api/v1/content`, `GET /api/v1/search` and the `list_content` and `search_content` MCP tools.

### Changed
- **Bulk publish no longer touches fork copies.** `publish_all_drafts` used to publish every unpublished document, including the working copies inside forks. It now ignores fork copies, and publishing one directly returns "cannot publish a fork copy; merge the fork instead" (409). Batch publish returns held pages and explicitly listed fork copies in a new `skipped` array of `{id, reason}`.
- **Fork copies are hidden from listings and search** unless `include_forks` is set. They used to appear in `list_content`, draft counts and `search_content` as duplicate drafts of live pages. Fork-specific endpoints (`get_fork`, fork diff, fork pages, `get_content` by ID) are unaffected. Site search, suggestions and search keywords also leave them out.
- **A merge deletes the fork's page copies.** The fork record is kept with status "merged" and the number of pages created and updated; the fork page and `get_fork` / `get_fork_diff` show those counts once the copies are gone.
- A merged fork can now be deleted. This removes the history record and any leftover copies; merged live pages are not affected.
- Scheduled publishing ignores fork copies. A copy of a scheduled page used to inherit its `publish_at`.

---

## [7.3.4] - 2026-10-05

### Fixed
- Deleting or unpublishing a legacy page with an empty `full_path` no longer deletes the homepage's static file. The empty path was being treated as `/`.

---

## [7.3.3] - 2026-10-05

### Fixed
- sitemap.xml now regenerates when it is more than 10 minutes old. Previously only admin-UI edits refreshed it, so pages created, changed or hidden through the API or MCP could be missing from it or listed after removal.

---

## [7.3.2] - 2026-10-05

### Fixed
- Pages created already-published (through the API, MCP, bulk create or import) now get a `published_at` date. Before this, only the explicit publish action set it, so such pages had no `datePublished` in structured data and sorted last in feeds. Unpublished drafts are unaffected.

### Added
- One-time backfill for pages affected by the bug above: `POST /api/v1/maintenance/backfill-published-dates` (with `?dry_run=true` to preview), or the `backfill_published_dates` MCP tool. It sets each affected page's `published_at` to its `created_at`. It changes metadata only (no re-render, no new versions) and records an audit-log entry. 128 MCP tools total.

---

## [7.3.1] - 2026-10-02

### Fixed
- Markdown copies: whitespace between inline siblings (e.g. two adjacent links) is kept, so links no longer run together.

---

## [7.3.0] - 2026-10-02

### Added — SEO & AI (generative engine optimization)
Every feature works on any LightCMS install. Existing sites behave as before until they change a setting, except that Markdown copies (below) are on by default.

- **AI traffic analytics** (Analytics → 🤖 AI traffic, `/cm/analytics/ai`):
  - Known crawlers are identified by operator and purpose: AI training (GPTBot, ClaudeBot, CCBot, …), AI search (OAI-SearchBot, Claude-SearchBot, PerplexityBot, …), user-initiated fetches (ChatGPT-User, Claude-User, Perplexity-User, …) and classic search engines.
  - Hits are counted per page, including `/llms.txt`, `/llms-full.txt`, `/robots.txt`, `/sitemap.xml`, the Markdown copies and the feeds.
  - Visits referred by AI assistants (ChatGPT, Perplexity, Claude, Gemini, Copilot, …) are recognized from the referrer or from the `utm_source` tag assistants add when they strip the referrer (e.g. `?utm_source=chatgpt.com`).
  - The report shows a daily chart by purpose, the pages AI reads most, and where AI referrals land.
  - Also available as `GET /api/v1/analytics/ai` and the `get_ai_traffic` MCP tool, and as an "AI visibility" summary in CMS Agent digests.
  - Crawlers such as Perplexity-User whose user agent doesn't say "bot" are now counted as bots.
- **AI crawler policy** (Tools → 🧭 SEO & AI, `/cm/tools/seo`):
  - Allow or block AI training, AI search and user-initiated fetches separately, with per-crawler overrides; the result is rendered into `/robots.txt`.
  - Optional Content-Signal line (`search` / `ai-input` / `ai-train`), plus free-form extra robots.txt lines.
  - Classic search engines are only affected by explicit overrides.
- **Markdown copies of pages** at `/<page>.md` (homepage: `/index.md`):
  - The page body converted to clean Markdown, with title, description, source URL and dates. The converter uses only `golang.org/x/net/html`, already a dependency.
  - Advertised with `<link rel="alternate" type="text/markdown">` on every page and linked from llms.txt, per the llms.txt convention.
  - Copies are served with `X-Robots-Tag: noindex` and a canonical `Link` header, so they never compete with the HTML page in search.
  - A real page whose path ends in `.md` always takes precedence. Can be turned off.
- **Richer structured data:**
  - Default author (Person or Organization, with URL and `sameAs` profiles) and per-page author override. User account names are never published automatically.
  - Publisher Organization with logo and `sameAs` links.
  - BreadcrumbList for pages in folders.
  - FAQPage, built only from explicit FAQ markup: `<details><summary>…?</summary>` pairs, or an "FAQ" / "Frequently Asked Questions" section whose question headings end in "?".
  - WebSite + Organization on the homepage.
  - Types a page already declares in its own JSON-LD are not duplicated. Raw-HTML pages now get structured data too.
- **Accurate modified dates:** a new `content_modified_at` is stamped only when a page's rendered HTML actually changes. Site-wide re-renders (theme, template, snippet changes) don't count. It drives JSON-LD `dateModified`, sitemap `lastmod`, feeds and Markdown copies; older pages fall back to `updated_at`.
- **RSS and Atom feeds** at `/feed.xml` and `/atom.xml`, plus `/<collection>/feed.xml` for each collection:
  - Full content with absolute links.
  - Default selection is Blog Post and Press Release templates plus the "blog" category; configurable, or every page.
  - Discoverable from every page's `<head>`, robots.txt and llms.txt.
- **Per-page "Hide from search engines & AI":** adds `noindex` (meta tag and `X-Robots-Tag`) and leaves the page out of the sitemap, llms.txt, feeds, Markdown copies and IndexNow. Available in the editor, REST API (`noindex`, `author_name`, `author_url`) and MCP content tools.
- **SEO & AI settings API and MCP:** `GET/PUT /api/v1/seo` (partial updates); `get_seo_settings`, `update_seo_settings`, `get_ai_traffic` (127 MCP tools total).

### Fixed
- Unpublishing, moving or deleting a page in the admin editor now notifies IndexNow. The editor writes to the database directly and previously skipped the transition notifications that API and MCP edits send.
- Page canonical links are absolute URLs.
- Fork diffs show changes to the new noindex and author fields.

---

## [7.2.5] - 2026-10-01

### Fixed
- IndexNow: a brand-new key is verified asynchronously by the search engine, and until then submissions return 403 `SiteVerificationNotCompleted`. These responses are now treated as "not yet" rather than fatal. Queued changes retry every 10 minutes for up to ~3 hours, and the one-time catch-up retries every 10 minutes instead of hourly. Before this fix, edits made during the first minutes after IndexNow activated were dropped.

---

## [7.2.4] - 2026-10-01

### Fixed
- Requests for `/<anything>.txt` that aren't the IndexNow key file now 404 (or serve a real page at that path) instead of returning the homepage with a 200 (regression in 7.2.3).
- IndexNow status (`GET /api/v1/indexnow`, `get_indexnow_status`) reports the key; it was shadowed by an empty field.

---

## [7.2.3] - 2026-10-01

### Added — IndexNow
- LightCMS now notifies IndexNow search engines (Bing, Yandex, Seznam, Naver, Yep, and others, through api.indexnow.org) whenever a public URL changes. That covers publish, unpublish, edits that change the rendered page, renames (old and new URL), deletes and restores. Works on every install with no setup: each site gets its own random key, served at `/{key}.txt` and stored in the settings collection as `indexnow_config`.
- Safe by default for self-hosters: inactive in development mode or when `BASE_URL` is not a public domain (localhost, IPs, .local/.test/.internal, example.com). Before submitting anything, the server fetches its own key file through `BASE_URL`, so a staging copy or misconfigured instance never submits for a domain it doesn't serve.
- Polite submission: changes are debounced into batches (up to 10,000 URLs per request). Each URL is resubmitted at most once per 10 minutes. 429/5xx responses retry with backoff; 400/403/422 are recorded and not retried. Template, theme, snippet and "regenerate all" re-renders are not submitted, because the content didn't change.
- One-time catch-up: the first time IndexNow is active on a site, every published page is submitted once. The claim is atomic, so multiple machines sharing a database submit once, and a failed attempt is retried hourly.
- Admin screen at Tools → IndexNow (/cm/tools/indexnow): on/off toggle, key file link and verification status, totals, recent submissions with HTTP results, "Submit all published pages now" (at most once an hour), and key rotation.
- REST: `GET/PUT /api/v1/indexnow`, `POST /api/v1/indexnow/submit` (`{"all": true}` or `{"paths": [...]}`). MCP: `get_indexnow_status`, `set_indexnow_enabled`, `submit_indexnow` (124 tools total).

### Fixed
- sitemap.xml no longer lists soft-deleted pages or fork copies, and its query uses a projection instead of loading full content documents.

---

## [7.2.2] - 2026-07-07

### Fixed
- Analytics: the trend-% KPI card resets when switching visitor tabs, instead of keeping the previous tab's value when the new tab has too little data for a regression.

### Changed
- Analytics edit-link resolution now runs one projected query over all tab datasets instead of three full-document queries (avoids loading embedding vectors per the projection invariant).
- Deduplicated the top-pages aggregation pipeline in the analytics service.

---

## [7.2.1] - 2026-07-04

### Fixed
- CMS Agent config page shows visible feedback after actions: success banner when a test digest is sent (with recipient and time) or configuration is saved, and a failure banner with the recorded error when a send fails.

---

## [7.2.0] - 2026-07-04

### Added — the CMS Agent
- New "CMS Agent" tool (/cm/tools/agent, admin only): the site's built-in agent, starting as configurable email digests of the analyses LightCMS already runs internally. Configure recipient, frequency (daily/weekdays/weekly), send hour (UTC), and sections: site health, traffic summary, awaiting-review queue (forks/approvals/scheduled), agent activity (from provenance), broken-link checks, and optional AI commentary (Claude writes an executive summary; requires ANTHROPIC_API_KEY).
- Outbound email via Resend: set RESEND_API_KEY and EMAIL_FROM (env) or resend_api_key/email_from (config.dev.json). Zero new dependencies — a single HTTPS call. "Send test digest now" button and last-sent/last-error status on the config screen.
- Digest sends are logged to the agent_digests collection; the scheduler checks every 10 minutes and dedupes to one digest per due day.

---

## [7.1.2] - 2026-07-04

### Added
- Slugs preserve their authored casing (e.g. /CLAUDE.md) while resolving case-insensitively: wrong-case requests 301 to the canonical URL, by-path lookups and upserts match case-insensitively, and case-variant duplicate paths are rejected at create/rename time.

---

## [7.1.1] - 2026-07-04

### Changed
- Copilot drawer: fullscreen toggle (⛶), and chat history moved to a vertical left sidebar (ChatGPT/Claude style) with search across titles and message content. Sidebar shows automatically in fullscreen; ☰ toggles it in drawer mode.

---

## [7.1.0] - 2026-07-04

### Changed
- Copilot is now a slide-in drawer available on every admin page (floating 🤖 button, bottom right; ESC or ✕ to close) instead of a dedicated section — ask for help without losing your place. /cm/copilot redirects to the dashboard with the drawer open.

### Added
- Copilot can read the daily maintenance report (get_maintenance_report): stale pages, missing meta descriptions, lingering drafts — "what needs attention on my site?" now has an answer.

---

## [7.0.9] - 2026-07-04

### Added
- Analytics pages (dashboard, per-page, per-referrer) offer 60-day and 90-day presets plus a custom date-range picker (inclusive end date). All three pages share one range parser.

---

## [7.0.8] - 2026-07-04

### Added
- Copilot keeps a history of chat sessions (per browser): a "Previous chats" selector restores earlier conversations with full context, and "New chat" starts fresh. Up to 30 sessions retained.

### Fixed
- /llms.txt and /llms-full.txt now query with field projections instead of loading full content documents (which include per-page embeddings) — on large sites these endpoints took 40+ seconds and pressured server memory.

---

## [7.0.7] - 2026-07-04

### Added
- Copilot chat renders markdown tables as styled HTML tables (and bullet lists as real lists) instead of raw pipe characters.

---

## [7.0.6] - 2026-07-04

### Added
- Copilot can answer analytics questions: new get_analytics tool exposes most popular pages, top referrers, and a traffic summary (DAU/MAU/uptime), defaulting to human traffic.
- Copilot chat shows an animated typing indicator with progressive status messages during long operations instead of static text.

### Fixed
- Analytics queries from the copilot include the current hour (the hour-bucket upper bound is exclusive).

---

## [7.0.5] - 2026-07-04

### Fixed
- Agent-session rollback could "revert" a page to the session's own version when the version write landed before the session's audit entry (timing race, caught by CI). Rollback now selects targets by version provenance — a session's own versions are never rollback targets — with timestamp comparison only as a fallback for pre-provenance versions.

---

## [7.0.4] - 2026-07-04

### Fixed
- Copilot chat requests failed CSRF validation ("Invalid or missing CSRF token"): the page template pre-quoted the token with printf %q inside a script block, but html/template already quotes values in JS context, so the browser sent a token wrapped in literal quote characters. Regression test renders through the real CSRF middleware.

---

## [7.0.3] - 2026-07-04

### Fixed
- WebSite JSON-LD now also reaches raw-HTML homepages (Blank Page templates with the theme disabled), injected into the authored <head>. The 7.0.1 change only covered themed homepages.

---

## [7.0.2] - 2026-07-04

### Added
- Admin sidebar shows the running software version under the logo (read from build.json, which is now kept in sync with releases).

---

## [7.0.1] - 2026-07-04

### Added
- Homepage now emits schema.org WebSite JSON-LD (name, tagline, URL) with a SearchAction advertising the public search endpoint — previously the homepage was the one page without structured data.

---

## [7.0.0] - 2026-07-04

### Added — the agentic release
- **llms.txt & llms-full.txt**: auto-generated Markdown site index and full plain-text content for AI crawlers (llmstxt.org proposal), plus schema.org JSON-LD (BlogPosting/NewsArticle/WebPage) on all published pages.
- **Fork-sandboxed agent sessions ("PRs for content")**: MCP tools `start_agent_sandbox` / `get_agent_sandbox` / `end_agent_sandbox`. While active, all agent content writes copy-on-write into a fork; publishing, deleting, and bulk operations are blocked; humans review the per-field diff (`GET /api/v1/forks/{id}/diff`, `get_fork_diff`) and merge.
- **Agent governance**: API keys accept a `scopes` permission allowlist and a `sandbox_only` flag (server-enforced fork-only writes). Every MCP session gets an agent-session ID; `GET /api/v1/agent-sessions/{id}/changes` lists everything a session changed and `POST .../rollback` undoes it as a unit.
- **Change provenance**: content versions record actor (human|agent), via (ui|api|copilot), and agent session. API edits now carry version attribution.
- **Admin copilot** (`/cm/copilot`): natural-language content editing via an Anthropic tool-use loop with per-tool RBAC and audit logging.
- **Self-hosted embeddings**: `LIGHTCMS_EMBEDDINGS_PROVIDER=ollama` generates semantic-search embeddings locally (default model nomic-embed-text) — no hosted API required.
- **Public read-only MCP endpoint** (`/mcp-public`): visitors' agents can search_site / get_page / list_pages / get_site_info with no authentication; drafts and forks are structurally excluded.
- **Maintenance scans**: daily site-health reports (stale pages, missing meta descriptions, drafts, optional broken-link job) via API and MCP tools.

### Fixed
- Editing an unpublished fork copy no longer deletes the live page's static file; fork content can no longer overwrite live static files or pollute the embedding index.

---

## [6.1.1] - 2026-03-31

### Security
- **Search/replace input validation**: Pairs array capped at 100 per request. Search and replace text capped at 100K characters. Regex patterns capped at 1,000 characters to prevent ReDoS.
- **Legacy API keys deprecated**: API keys without an associated user now return 403 instead of bypassing all permission checks. Keys must be re-created with a user owner.
- **Script policy enforced at write time**: Bulk create and bulk update operations now sanitize content data fields (bluemonday) when the script policy is `admin_only` or `none`, as defense-in-depth alongside the existing render-time enforcement.
- **Bulk field operation field validation**: System fields (`_id`, `published`, `template_id`, `full_path`, etc.) are blocked from modification via the bulk field operation endpoint.
- **Per-page locking in concurrent operations**: Search/replace, bulk update, and bulk field operations now use per-page mutual exclusion to prevent race conditions when multiple workers target the same content item.
- **Error message sanitization**: Bulk operation error responses no longer expose MongoDB internals, field names, or collection structure. Detailed errors are logged server-side only.
- **Enhanced audit logging**: Bulk create operations now include a sample of created content IDs (up to 10) in the audit trail.

---

## [6.1.0] - 2026-03-31

### Added
- **Bulk Create Content**: New `bulk_create_content` MCP tool and `POST /api/v1/content/bulk-create` endpoint. Creates up to 100 content items in a single call using MongoDB `InsertMany` (unordered — partial failures don't abort the batch). Published items get parallel HTML generation (10 concurrent goroutines). Returns per-item `{id, full_path, success, error}` results.
- **Content Upsert**: `create_content` and `bulk_create_content` now accept `upsert: true`. If a page already exists at the same path, it updates instead of returning a duplicate key error. Returns `{action: "created"}` or `{action: "updated"}`. Eliminates the most common failure mode in retry scenarios.
- **Multi-Pair Search & Replace**: `search_replace_preview` and `search_replace_execute` (and scoped variants) now accept a `pairs` array of `{search, replace, regex}` objects. All pairs are applied in a single pass per page — O(pages) instead of O(pairs x pages). Critical for operations like fixing hundreds of broken links simultaneously.
- **Content List Pagination**: `list_content` now supports `limit` (1-500) and `offset` query parameters. When `limit` is set, returns `{items, total, limit, offset, has_more}` envelope. Default behavior (no limit) unchanged for backward compatibility.
- **Burst Rate Limiting**: New per-token 20 requests/second burst limiter on all `/api/v1/` endpoints. Returns HTTP 429 with `Retry-After: 1` header. Complements the existing 300 req/min sliding window. Prevents runaway scripts from overwhelming the server.
- **Site Analytics Dashboard**: New analytics section at `/cm/analytics` with hourly visitor chart (HTML bar chart with hover tooltips), uptime monitoring strip, top pages by views, top referrers (with Non-bot/Bots/All tabs), browser/device donut chart. Per-page analytics at `/cm/analytics/page` with referrer breakdown. Referrer drill-down report at `/cm/analytics/referrer` showing top pages from each source.
- **Analytics Tab on Content Editor**: New "Analytics" tab in the content edit page bottom panel showing 7-day and 30-day view counts and top referrers for that specific page.
- **Page View Tracking**: Every public page view records page path, referrer source (direct/internal/external domain), and browser category. Data buffered in memory and flushed to MongoDB every 30 seconds for minimal performance impact.

### Changed
- **Search/Replace now returns counts**: Both preview and execute responses include `pages_scanned`, `pages_modified` (execute), `total_replacements`, and per-pair summaries in multi-pair mode.
- **Conditional Republish**: Static HTML generation now uses SHA-256 content hashing. Pages whose rendered output hasn't changed are skipped during regeneration. `RegenerateAllContent` clears all hashes first to force full regeneration when theme/template/snippet changes affect output globally.
- **Deploy script hardened**: Orphan machine cleanup now catches machines in any state (created, starting, stopped), not just created.

### Fixed
- **Analytics data not recording**: The `site_stats` collection couldn't be created due to MongoDB Atlas's 500-collection limit. Hourly stats now stored in the existing `user_activity` collection using a sentinel document pattern.
- **Analytics chart empty**: JavaScript time format mismatch (Go's ISO omits zero milliseconds, JS `toISOString()` always includes them) caused the hour-keyed lookup to never match. Fixed by using epoch-millisecond keys.
- **Analytics write buffer failing silently**: The per-page referrer key used `\x00` (null byte) as a separator, which BSON rejects in field names. Changed to `||` separator. This bug caused ALL buffered writes (page views, referrers, user agents) to silently fail since they shared a single `$inc` update.
- **Uptime strip current hour**: The most recent hour in the uptime strip now always shows green (the server is by definition online if you're viewing the page).

---

## [6.0.2] - 2026-03-24

### Added
- **`GET /api/v1/users` endpoint**: Returns a minimal user list (id, email, display_name, role) for UI features like @mention autocomplete and workflow approver selection. Requires `content.view` permission.
- **Workflow creation UI**: Admin approvals page now includes an inline "New Workflow" form. Search users by email to add approvers, choose trigger type (all_contributor / folder_path / template_id / tag), set mode (sequential / concurrent), and create the workflow without leaving the page.
- **API attribution in audit log**: Audit log entries created via API key now have `via_api: true` stored in the database. The audit log display shows "(api)" next to the username for API-originated actions. Session-based (browser) actions are unaffected.

### Fixed
- **Admin UI calls to `/api/v1/` now authenticated via session**: The API auth middleware now accepts session cookies as a fallback when no `Authorization: Bearer` header is present. This fixes all admin UI interactions that call `/api/v1/` endpoints directly from browser JavaScript (posting comments, approving/rejecting requests, deleting workflows) — they were returning "Missing authorization header" for session-authenticated users.
- **Contributor role missing from user dropdowns**: The Create User and Edit User forms now include the Contributor role option.
- **Approvals moved to Content nav section**: The "Approvals" link in the admin sidebar is now under the Content section (alongside Content, Templates, Forks, Imports) instead of the Inbox section.
- **Broken `Bearer ` token in mention user search**: The @mention user autocomplete was sending an empty Bearer token, causing a 401. It now relies on session cookie fallback.

---

## [6.0.1] - 2026-03-24

### Security
- **CSRF key hardening**: CSRF protection key is now derived via SHA-256 hash of the session secret instead of zero-padding, eliminating low-entropy padding.
- **APIGetSiteConfig auth**: GET `/api/v1/config` now requires `settings.view` permission. The Cloudflare API token is redacted (`***`) in the response — it is write-only via the admin UI.
- **OAuth system key user context**: The system API key used for OAuth-authenticated MCP sessions is now associated with an admin user. OAuth sessions previously bypassed all permission checks via a nil-user path; they now properly resolve to admin-level RBAC.
- **Self-approval blocked**: Users can no longer approve their own content submissions.
- **Sequential workflow order enforced**: Only the approver assigned to the current step in a sequential workflow may advance it.
- **Workflow approver validation**: Approver user IDs are validated against real users when creating or updating approval workflows.
- **Bulk export gated**: `APIExportContent` now requires `content.edit` permission (was unauthenticated).
- **20+ read-only API endpoints now require login**: All `/api/v1/` read endpoints (content list/get/versions/search/preview/backlinks, template list/get, asset list/get/folders, snippet list/get, redirect list/get, folder list/get, collection list/get, theme get/versions, config get, comment list) now require at minimum viewer-level credentials. Public end-user endpoints (`/api/search`, `/api/chat`, `/api/contact`) are unchanged.
- **Comment rate limiting**: `POST /api/v1/content/{id}/comments` is now rate-limited to 20 requests/minute per API token.
- **Comment length limit**: Comment text is capped at 10,000 characters.
- **Comment XSS fix**: `user_display_name` and `user_email` are now HTML-escaped before injection into dynamically created comment DOM nodes. Mention dropdown also escaped.
- **`APIForceUnlockContent` RBAC**: Replaced hardcoded `role == "admin"` string check with `requirePermission(PermUserManage)`.
- **Sidebar approval badge efficiency**: `renderAdmin` now calls `CountPending()` (single count query) instead of `ListPending()` (full result set) for the badge count on every admin page render.

---

## [6.0.0] - 2026-03-24

### Added
- **Content Approvals**: Configurable approval workflows for content publishing. Contributors can save drafts and submit for approval; editors and admins approve or reject from the new `/cm/approvals` dashboard. Rejection comments are auto-posted to the content's discussion thread.
- **Contributor role**: New RBAC role between Viewer and Editor. Contributors can create content and upload assets (held in a pending queue), post discussion comments, and submit content for approval — but cannot publish directly or manage templates, snippets, theme, or settings.
- **Approval workflows**: Configurable trigger-based workflows (by contributor, folder path, template ID, or tag). Supports sequential mode (approvers in order) and concurrent mode (any N-of-M). Default behavior (no workflow) allows any editor or admin to approve.
- **Content Discussion tab**: Inline threaded comment section at the bottom of every content edit page. Supports @mention autocomplete. Admins can delete any comment. Live badge count on the tab.
- **Tabbed bottom section on content edit page**: Discussion, Version History, and Forks are now organized into tabs instead of stacked plain sections.
- **Approvals Dashboard** (`/cm/approvals`): Shows "My Queue" (requests where the current user is an approver) and "Other Pending". Approve/reject directly from the page with a required rejection comment. Approval badge count shown in the sidebar nav.
- **Dashboard: Requiring Approvals section**: When pending approval requests exist, a summary appears on the admin dashboard.
- **Dashboard: Recent Comments section**: Five most recent discussion comments with links to the relevant content pages.
- **Webhook events**: Three new event types — `comment.created`, `content.pending_approval`, `asset.pending_review`. Subscribable from the webhook form and documented in the reference table.
- **MCP tools** (14 new tools): `list_comments`, `create_comment`, `delete_comment`, `list_approval_workflows`, `get_approval_workflow`, `create_approval_workflow`, `update_approval_workflow`, `delete_approval_workflow`, `list_approval_requests`, `get_approval_request`, `submit_for_approval`, `approve_request`, `reject_request`, `cancel_approval_request`.
- **Messages mark-all-read**: "Mark All as Read" button on the contact messages list page.

### Changed
- MCP server version updated to 6.0.0.

---

## [5.0.0] - 2026-03-24

### Added
- **Import Pipeline**: Three new import types with a unified job dashboard at `/cm/imports`:
  - **RSS/Atom Import**: Configure recurring feed sources with schedule (hourly/daily/weekly), template mapping, folder targeting, and auto-publish. Full run history and per-job logs.
  - **Markdown Import**: Upload single `.md` files or `.zip` archives of Markdown. YAML frontmatter controls title, slug, folder, template, tags, and scheduling. Supports Notion exports, Obsidian vaults, Hugo/Jekyll migrations, and AI-generated content.
  - **CSV Import**: Upload CSV files and map columns to content fields. Specify the title column; all other columns become fields automatically.
- **Real-time import status**: SSE-powered live log stream at `/cm/imports/{jobID}`. Watch imports happen line-by-line or review full history after the fact.
- **MCP import tools** (10 new tools): `list_import_sources`, `create_import_source`, `update_import_source`, `delete_import_source`, `trigger_import_source`, `import_markdown`, `import_csv`, `list_import_jobs`, `get_import_job`, `cancel_import_job`.
- **Agentic bulk content creation**: `import_markdown` is specifically designed for AI agents to generate and import large batches of content in a single call. See `MCP.md` for workflow examples.
- **MCP.md**: New developer guide for MCP/agentic workflows with bulk import examples.
- **Deduplication**: Imports match by `full_path` — re-importing the same slug updates rather than duplicating.
- **Import job TTL**: Log entries auto-expire after 90 days.

---

## [4.5.0] - 2026-03-24

### Added
- **Webhooks**: HMAC-SHA256 signed webhook events for content lifecycle (publish, unpublish, delete, create, update). Configurable per-webhook with retry logic and delivery history. Admin UI at `/cm/webhooks`.
- **Scheduled Publishing**: Set a future `publish_at` timestamp on any content item. Background scheduler publishes automatically.
- **Content Locking**: Advisory lock when editing content (30-minute expiry). Warning banner if another user is already editing. Force unlock for admins.
- **Incremental Static Regeneration**: Template changes now regenerate content in batches of 20 instead of all-at-once, preventing server overload on large sites.
- **Edge Caching Headers**: ETag, Cache-Control, Last-Modified, and Vary headers on all public pages. Conditional request support (304 Not Modified).
- **Cloudflare Integration**: Configure Zone ID and API Token to automatically purge Cloudflare cache when content is published or unpublished.
- **Structured JSON Logging**: All server logs now emit structured JSON with timestamp, level, message, and context fields.
- **Rate Limit Dashboard**: New tab on the audit log page showing locked IPs, attempt counts, and a clear button.
- **MCP Prompt Resources**: Three new MCP resources — `lightcms://site/structure`, `lightcms://content/recent`, `lightcms://theme/config` — exposing live site data for AI context.

---

## v4.2.0 — AI Chat Widget, Security Hardening, Fork MCP Tools & Performance Overhaul

### AI Chat Widget

An embeddable AI-powered chat widget that lets site visitors ask questions in natural language and receive answers synthesized from the site's own content.

- **Two-phase query pipeline**: Every question first runs a hybrid semantic + full-text search (reusing the same `SearchService` used by the MCP `end_user_search` tool) to retrieve the most relevant content excerpts. Those excerpts are then passed to Claude Haiku for streaming synthesis into a conversational answer.
- **Server-sent events (SSE) streaming**: The `/api/chat/query` endpoint responds as a live SSE stream, forwarding Haiku's token-by-token output to the browser in real time. Falls back to plain JSON for clients that don't support SSE.
- **Graceful AI-optional mode**: When no `ANTHROPIC_API_KEY` is configured, the widget still works — it returns ranked search results with excerpts, skipping the synthesis step. Sites without an API key get a useful search-in-chat experience.
- **Fully configurable from the admin UI**: A new "Chat Widget" admin page exposes all settings — enabled/disabled toggle, widget title, welcome message, placeholder text, primary color, position (bottom-left / bottom-right), max results, and editable system/user prompt templates with `{siteName}`, `{excerpts}`, and `{question}` placeholders.
- **Embeddable JS widget** (`/static/js/chat-widget.js`): A self-contained floating chat bubble that fetches its config from `/api/chat/config` and posts queries to `/api/chat/query`. Add to any page with a single `<script>` tag.
- **Dedicated rate limiting**: Chat endpoints use separate per-IP and global rate limits (default: 5/min per IP, 30/min global), independent from the search and API rate limiters. Anthropic API calls are metered separately from ordinary search traffic.
- **Source attribution**: Every response includes a `sources` SSE event listing the content pages whose excerpts were used, with titles and paths.

### Security

- **CORS lockdown**: Chat widget endpoints (`/api/chat/*`) now restrict `Access-Control-Allow-Origin` to the site's configured `BASE_URL` instead of `*`. Falls back to `*` only when `BASE_URL` is unset.
- **Prompt injection defense**: User query text in the chat widget is wrapped in `<user_question>...</user_question>` XML delimiters before being interpolated into the Anthropic prompt. The `</` sequence is escaped to prevent tag injection.
- **Prompt template validation**: Saved chat widget system/user prompts are validated to only allow known placeholders (`{siteName}`, `{question}`, `{excerpts}`). Unknown placeholders are rejected at save time.
- **Configurable upload size limit**: `MaxUploadBytes` added to `SiteConfig`, settable via the admin Configuration page and the `update_site_config` API. Defaults to 1 MiB (2× the largest asset stored at time of release). Enforced via `http.MaxBytesReader` on both the file upload endpoint and the asset upload endpoint.
- **API body size limit**: All `/api/v1/` endpoints now enforce a 10 MiB request body cap via `APIBodySizeLimit` middleware, preventing memory exhaustion from oversized payloads.
- **Fly.io IP spoofing fix**: `TrustedProxyConfig` adds a `TrustFlyProxy` mode that reads `Fly-Client-IP` (set exclusively by Fly.io's edge proxy) instead of `X-Forwarded-For`, which can be set by anyone. `DefaultCloudConfig()` now uses this mode, preventing rate-limit and audit-log bypass.
- **Session secret entropy**: Production deployments now hard-fail on startup if `SESSION_SECRET` is shorter than 32 characters (previously 16).
- **Per-endpoint rate limiters**: New limiters protect expensive endpoints beyond the global 300 req/min cap — regenerate (2/min), search-replace execute (10/min), asset-from-url (10/min), bulk-update (5/min), export (5/min), reindex-embeddings (1/min).
- **Rate limiter map pruning**: A background goroutine prunes stale token entries from all rate limiter maps every 5 minutes, preventing unbounded memory growth over long-running deployments.

### Fork MCP Tools (8 new tools)

Full MCP and REST API coverage for the fork workspaces system introduced in v4.0.0:

- **`list_forks`** — List all forks with status and page count.
- **`create_fork`** — Create a named fork workspace.
- **`get_fork`** — Retrieve fork details including merge conflicts from last merge.
- **`fork_page`** — Add a page to a fork (accepts content ID or path); returns fork page ID for use with `update_content`.
- **`remove_fork_page`** — Remove a page from a fork (reverts to live content on preview).
- **`merge_fork`** — Merge all fork changes into live content (admin only).
- **`archive_fork`** — Archive a fork without merging.
- **`delete_fork`** — Permanently delete a fork and all its page copies.

### Performance

- **Admin template caching**: Admin HTML templates are compiled once at startup (via `sync.Once`) and cached. Previously each admin page request re-parsed the full template from source — eliminated entirely.
- **Content list pagination**: The admin Content page now loads 100 items at a time with Previous/Next controls and a total count. Previously it loaded the entire `content` collection into memory, which OOM'd the server on large sites.
- **Content indexes**: Added `{updated_at: -1}` and compound `{fork_id, deleted, updated_at}` indexes to fix `Sort exceeded memory limit` errors on the MongoDB Atlas free tier when the collection grew past ~32 MiB.
- **Search/replace streaming**: All four search/replace handlers (global preview, global execute, scoped preview, scoped execute) now stream documents one at a time via cursor iteration instead of loading the entire collection into a Go slice. Memory is bounded regardless of collection size. Coverage is complete — every document is still checked, nothing is truncated.
- **New `StreamContent` / `StreamContentScoped`** service methods return raw `*mongo.Cursor` for callers that need bounded-memory processing.
- **`QueryContentForDirective` cap**: `lc:query` template directives now apply a 10,000-item `SetLimit` cap, preventing a single index page from OOM-ing the server on a large site.
- **Wikilink index cache**: `buildWikilinkIndex` caches results for 60 seconds (TTL), invalidated immediately on any title, path, or publish-status change. Previously a full `content` collection scan ran for every page publish — on bulk operations this was O(n) scans for n pages.
- **`UpdateWikilinksOnRename` streaming**: Changed from `FindAll` (full load into slice) to streaming cursor iteration, bounding memory when many pages reference a renamed item.
- **Export scope push-down**: `APIExportContent` now uses `ListContentScoped` to push all filters (template, category, folder, content IDs) to MongoDB, instead of loading all content then filtering in Go.

### Database (earlier v4.2.0 work)

- **New indexes**: Added `settings.type`, `login_attempts.ip`, `theme_versions.version`, and `content.plain_text` (text index) to eliminate collection scans on hot query paths.
- **`ListAssets` projection**: Excludes the binary `data` field from asset listing queries, dramatically reducing wire transfer for asset list operations.
- **Atomic login rate limiting**: `RecordFailedLogin` replaced a read-then-write pattern with `FindOneAndUpdate` + `$inc`, eliminating a race condition under concurrent login attempts.

### Search (earlier v4.2.0 work)

- **Parallel hybrid search**: `SearchHybrid` now runs `SearchFullText` and `SearchSemantic` concurrently via goroutines, halving latency when both sources are available.
- **`sort.Slice` everywhere**: All ranking insertion sorts in `SearchFullText`, `Suggest`, and `RebuildKeywords` replaced with `sort.Slice` / `sort.SliceStable` for O(n log n) behaviour on larger result sets.
- **Pre-normalized search config**: `getSearchConfig` lowercases and trims all path/template lists once on cache load, removing redundant per-query `strings.ToLower`/`strings.TrimSpace` calls.

### Content Regeneration (earlier v4.2.0 work)

- **Parallel `RegenerateAllContent`**: Sequential per-page loop replaced with a semaphore-bounded goroutine pool (6 workers), enabling concurrent static page generation.
- **Single wikilink index per bulk regen**: `buildWikilinkIndex` is called once before the worker pool starts and shared across all workers.
- **Targeted `UpdateWikilinksOnRename`**: `$regex` pre-filter limits the scan to documents that likely reference the old title/path.

---

## v4.1.0 — Bug Fixes

### Bug Fixes

- **Redirect deletion now takes effect immediately** — Redirects were served with `Cache-Control: max-age=3600`, causing browsers to cache redirect responses for up to an hour. Deleting a redirect had no effect until the browser cache expired. Changed to `Cache-Control: no-store` so browsers never cache redirects and always re-check with the server.

---

## v4.0.0 — Content Forks

This release adds fork workspaces for safe page experimentation, plus bulk operation resilience improvements and content search refinements.

### Content Forks

- **Fork workspaces** — Editors and admins can create isolated staging workspaces ("forks") where sets of page changes can be authored, previewed, and reviewed before going live. Only touched pages live in a fork (sparse model — unmodified pages fall through to live).
- **Fork preview** — Activate fork preview via a floating purple bar injected into the live site. A `lc_fork_preview` cookie routes page requests through the fork, showing exactly how the site will look after merge.
- **Merge with conflict detection** — Admins merge forks into live content. If a live page was edited after the fork was created, the conflict is recorded in the merge result (fork wins). Newly created fork pages are inserted as live content on merge.
- **Permission gating** — Creating and editing forks requires at least `editor` role; merging requires `admin`.
- **"Fork to workspace" button** — Available on any content edit page; opens the forks list with a one-click "Add to this fork" action.
- **Fork exclusion from content list** — Fork copies are invisible in the main admin content list and search results.

### Performance & Resilience

- **Coalescing background workers** — `RegenerateIndexPages` and `RebuildKeywords` now use debounce channels: a bulk update of 25 items triggers one regeneration pass instead of 25 concurrent goroutines. Eliminates the goroutine explosion that caused server timeouts during large bulk operations.

### Content Search

- **Slug search mode** — New search option alongside Title Only and Full Text. Searches slug and full_path fields, sorted alphabetically by slug.
- **Homepage always first** — The site index page (`/`) is always pinned to position 0 in the content list and all search modes. For slug search, the homepage is explicitly fetched and prepended if not already in results.
- **"/" query fix** — Searching `/` in Title or Full Text mode now correctly includes the homepage (stored internally with empty slug/path).

### Dashboard

- Removed Collections KPI stat card.

### Bug Fixes

- **Fork page unique index** — Replaced the single-field `full_path` unique index with a compound `{full_path, fork_id}` unique index, allowing fork copies to share paths with their live counterparts without constraint violations.

---

## v3.3.0 — Bulk Operation Performance

This release significantly improves the performance of all bulk and batch operations, with particular gains at scale (hundreds of content items).

### Performance improvements

- **Scope filters pushed to MongoDB** — `bulk_field_operation`, `scoped_search_replace_execute`, and `export_content` now push `template_name`, `category`, `folder_path`, and `content_ids` filters down to MongoDB instead of loading all content and filtering in Go. For a 600-item site running a scoped operation on 100 pages, this eliminates ~500 unnecessary document transfers.
- **Goroutine worker pool (10 concurrent)** — All four bulk/search-replace execute handlers now process items concurrently with a pool of 10 workers instead of sequentially. On a 100-item batch this yields up to 8–10× faster wall time.
- **Batch content fetch in `bulk_update_content`** — Per-item `GetContent` calls replaced with a single `$in` query that fetches all requested IDs at once. 100-item bulk update: 100 DB round-trips → 1 for the initial fetch.
- **`content_versions` index on `content_id`** — Previously missing; every `UpdateContent` call triggered a full collection scan on `content_versions` for the version count. This index makes versioning O(log n) per item.
- **`template_name` index on `content`** — Enables the new MongoDB-level template scope filter to use an index scan instead of a full collection scan.
- **`InsertMany` helper** — Added `db.InsertMany` for future bulk version insert use.

### New service methods

- `ContentService.ListContentScoped(ctx, ContentScope)` — MongoDB-native scoped list with support for template_name, category, folder_path, and content_ids filters.
- `ContentService.GetContentByIDs(ctx, []ObjectID)` — Batch fetch multiple content items in one `$in` query, returning a map keyed by ID.

---

## v3.2.0 — Healthz Endpoint & DAU/MAU Analytics

This release adds a structured health check endpoint following the vibectl VibeCtl Health Check Protocol, plus user activity tracking for DAU/MAU metrics surfaced in both the health endpoint and the admin dashboard.

### `/healthz` Endpoint

- New `GET /healthz` endpoint — unauthenticated, returns JSON following the [vibectl VibeCtl Health Check Protocol](https://github.com/jonradoff/vibectl):
  - `status`: `healthy` / `degraded` / `unhealthy` derived from dependency checks
  - `name`: `"LightCMS"` — software identifier
  - `version`: current build version (e.g. `"3.2.0"`)
  - `uptime`: seconds since process start
  - `dependencies`: live MongoDB ping check
  - `kpis`: DAU, MAU, and content pages created today
- Returns HTTP 503 when status is `unhealthy`
- The existing `/health` endpoint (plain-text `OK` for Fly.io TCP probes) is unchanged

### Analytics & KPIs

- New `AnalyticsService` backed by `user_activity` MongoDB collection
- Activity recorded automatically on admin login and API key authentication (fire-and-forget goroutine, no request latency impact)
- Storage: upsert on `{user_id, date}` — at most one document per user per calendar day, O(1) writes
- **DAU** (Daily Active Users): distinct users active today (UTC)
- **MAU** (Monthly Active Users): distinct users active in the last 30 calendar days (aggregation pipeline)
- **Content created today**: content items with `created_at ≥ midnight UTC` (no additional tracking needed — uses existing field)

### Admin Dashboard

- Three new stat cards added to the dashboard: Daily Active Users, Monthly Active Users, Pages Created Today

---

## v3.1.0 — Bulk Operations, Wiki-Like Markup & Security Hardening

This release adds first-class support for bulk content operations (eliminating N×1 API call patterns), a full wiki-like markup system for content authoring, and comprehensive security fixes.

### Bulk Content Operations

- **`bulk_update_content`**: Update up to 100 content items in a single MCP/API call. Each item uses merge semantics — only supplied fields change. Supports `clear_fields`, `dry_run` validation, and returns per-item success/error details.
- **`bulk_field_operation`**: Apply a single operation (`clear`, `set`, `prepend`, `append`, `wrap`) to a field across all matching pages in one call. Scoped by template, folder, category, or content IDs. Supports `dry_run`.
- **`export_content`**: Export content items with full field data as a structured JSON array. Designed for the export → transform → `bulk_update_content` pipeline. Scope filters supported.
- **`list_content` with field data**: New `include_data: true` and `include_fields: ["field1"]` parameters return full template field values in list results, eliminating per-item `get_content` calls for bulk read workflows.
- **`clear_fields` on `update_content`**: Explicitly set fields to empty string — removes ambiguity about merge semantics.
- **`dry_run` on `update_content` and `bulk_update_content`**: Validate payloads without committing changes.
- **Concurrency guidance**: Tool descriptions document that up to 20 concurrent `update_content` calls are safe; larger batches should use `bulk_update_content`.

### Regex Search & Replace

- All four search/replace tools (`search_replace_preview`, `search_replace_execute`, `scoped_search_replace_preview`, `scoped_search_replace_execute`) now support `"regex": true`.
- When enabled, `search` is treated as a Go RE2 regular expression. Use `$1`, `$2` for capture group references in `replace`.
- Input validation: patterns capped at 500 characters, max 20 capture groups, 10× expansion guard on replacement.

### Wiki-Like Markup System

- **Wikilinks**: `[[Page Title]]`, `[[Page Title|display text]]`, `[[/path]]`, `[[/path|display text]]` syntax in any content field. Resolves to `<a>` at page generation time; broken links render as `<span class="broken-link">`. Links auto-update when a page's title or path changes.
- **Snippet includes**: `[[include:snippet-name]]` embeds a named snippet inline in any content field. Recursion depth limit (3) and cycle detection prevent infinite expansion.
- **Table of contents**: Add `{{.lc_toc}}` anywhere in a template's HTML layout to auto-generate a `<nav class="lc-toc">` from the page's headings.
- **Heading IDs**: All `<h1>`–`<h6>` tags automatically get `id=` attributes derived from their text content, enabling deep-linking.
- **Markdown field type**: Template fields can be set to type `markdown`. Field values are converted through goldmark (GitHub Flavored Markdown) at page generation time. Supports tables, strikethrough, task lists, autolinks.
- **Inline `#tag` detection**: Mentioning `#tagname` in any content field automatically adds that tag to the page's tag list, which feeds into `lc:query` index pages.
- **`get_backlinks` MCP tool**: Returns all published pages that link to a given path via wikilinks or `<a>` tags. Useful for impact assessment before renaming/deleting pages.
- **Version history user attribution**: The "By" column in version history now shows the email of the editor who made each change.
- **Admin search by slug/path**: The admin content search now matches on slug and full URL path in addition to title and content.

### Configurable Script Policy

- New site config setting `markdown_script_policy` controls who may use raw `<script>` tags and unsafe HTML in content fields:
  - `"all"` (default) — all users may use scripts; backward-compatible with existing content
  - `"admin_only"` — admin-authored content passes through unchanged; editor-authored content is sanitized via bluemonday (strips `<script>`, `<iframe>`, event handlers, `javascript:` URIs; allows all other HTML)
  - `"none"` — all content sanitized regardless of author role
- Configurable via the admin Settings page or via the `update_site_config` MCP tool.

### Security Fixes

- **Export authorization**: `export_content` now requires authentication (previously unauthenticated)
- **Regex input validation**: Pattern length cap (500 chars), capture group limit (20), and expansion guard prevent regex-based DoS
- **Snippet recursion guard**: Depth limit of 3 and cycle detection prevent infinite expansion chains
- **Version history permissions**: `get_content_versions` and `get_content_version` now enforce `PermContentView`
- **Folder path scope bug**: Scoped search/replace with `folder_path: "/blog"` no longer accidentally matches `/blog-old`, `/blog-archive`, etc.
- **Bulk field op limit**: `bulk_field_operation` returns 400 if the operation would affect more than 500 pages (prevents unbounded database writes per request)
- **Wikilink href safety**: Resolved links that don't start with `/` are rendered as broken-link spans rather than potentially-unsafe hrefs
- **TOC HTML escaping**: Heading IDs in TOC anchor `href` attributes are now properly HTML-escaped
- **Rate limiter cleanup**: Background goroutine prunes stale entries every 10 minutes to prevent unbounded memory growth

---

## v3.0.0 — Dynamic Index Pages: Tags, Snippets & `lc:query`

This release adds a first-class system for building dynamic, automatically-updated index pages — the most significant content modelling capability added since v1.0.

### Content Tagging

- **Tags field on all content items**: any content item can now carry zero or more freeform string labels
- Tags are set in the admin editor (below the main fields) or via `{"tags": [...]}` in the REST API
- Tags are exact-match strings; capitalization and spacing are preserved
- Full REST API support: `GET /api/v1/content?tag=TAGNAME` filters by tag; `PUT /api/v1/content/{id}` accepts a `tags` array
- Tags are indexed in MongoDB for efficient filtering

### Snippets

- **New `snippets` collection**: named HTML template fragments with Go template variable support
- **Admin UI** at `/cm/snippets`: create, edit, and delete snippets with a live editor
- **REST API**: `GET/POST /api/v1/snippets`, `GET/PUT/DELETE /api/v1/snippets/{id}`
- **Available variables in snippets**: `{{.Title}}`, `{{.FullPath}}`, `{{.Slug}}`, `{{.MetaDescription}}`, `{{.PublishedAt}}`
- Snippets are rendered through Go's `html/template`, so struct fields are HTML-escaped by default — safe from XSS

### `lc:query` Directives

- **Embed live content queries in template layouts** using HTML comment syntax:
  ```html
  <!-- lc:query filter="tag:TAGNAME" sort="title:asc" snippet="snippet-name" -->
  ```
- Directives are processed at page generation time before Go's template engine runs, then replaced with the rendered output of all matching published content items
- Supports `filter="tag:X"`, `filter="category:X"`, `filter="template:X"`, `filter="folder:X"` (and shorthand `tag="X"` etc.)
- Sort options: `title:asc`, `title:desc`, `created_at:asc`, `created_at:desc`
- Multiple directives per template — each is an independent query
- Falls back to a plain `<a href>` link if no snippet is specified

### Automatic Cascade Regeneration

- When a tagged content item is published or updated, all index pages whose templates contain `lc:query` directives are automatically regenerated — no manual rebuild needed
- Regeneration also triggers when a template layout or snippet is updated
- `POST /api/v1/regenerate` triggers a full-site regeneration (unchanged behaviour, now also covers `lc:query` pages)

### MCP Tools (5 new, 54 → 59 total)

- `list_snippets` — list all snippets
- `get_snippet` — retrieve a snippet by ID or name
- `create_snippet` — create a new named HTML snippet
- `update_snippet` — update a snippet's name or HTML
- `delete_snippet` — delete a snippet

### Security Fix

- **XSS in `lc:query` default fallback**: when `lc:query` had no `snippet` attribute, `item.Title` and `item.FullPath` were concatenated directly into HTML without escaping. Fixed to use `template.HTMLEscapeString()` on both values. The snippet path (which is the recommended usage) was already safe via `html/template` auto-escaping.

### Documentation

- README expanded with full documentation of Tags, Snippets, `lc:query`, and the complete walkthrough for building a tagging-powered index page
- New MCP example (Example 10): building a dynamic glossary index end-to-end
- Snippets endpoint added to REST API reference table
- Security analysis section in changelog (this entry)

---

## v2.6.0 — MCP Tool Improvements

### Bug Fixes
- **`get_theme` returned empty strings** — `ThemeSettings` struct was missing `json` struct tags; all theme fields now serialize correctly via the REST API and MCP
- **`search_replace_execute` response missing search/replace fields** — both `search_replace_execute` and `scoped_search_replace_execute` now echo back the `search` and `replace` strings used, alongside `total_replacements`, `pages_updated`, and `updated_pages`

### New MCP Feature: Rendered HTML in `get_content`
- `get_content` now accepts an `include_rendered` boolean parameter
- When `true`, the response includes a `rendered_html` field containing the fully rendered page HTML (template fields interpolated + theme header/footer applied)
- Works for both published and draft content — lets agents inspect exactly what visitors see without any publishing step
- Complements `preview_content`, which is better for previewing unsaved field overrides

---

## v2.5.0 — Security Hardening

### SSRF Prevention (Critical)
- **`upload_asset_from_url`** now uses a custom HTTP dialer that resolves the target hostname at connect time and blocks all private/reserved IP ranges before making any network request, preventing Server-Side Request Forgery (SSRF) and DNS-rebinding attacks
- Blocked ranges: loopback (127.0.0.0/8, ::1/128), RFC1918 private (10/8, 172.16/12, 192.168/16), link-local/AWS metadata (169.254.0.0/16), CGNAT (100.64.0.0/10), IPv6 ULA (fc00::/7) and link-local (fe80::/10)
- Error messages from this endpoint no longer echo internal network details back to callers

### ReDoS Prevention (Critical)
- **`SearchFullText`** now runs under a 5-second context deadline, preventing unanchored regex scans from pinning CPU on large corpora or pathological queries

### Permission Checks (High)
- **`POST /api/v1/search-replace/preview`**: now requires `PermSearchReplace` (admin-only); previously accessible to any authenticated API key
- **`POST /api/v1/search-replace/scoped/preview`**: same fix — matched the execute endpoint's permission requirement
- **`POST /api/v1/reindex-embeddings`**: now requires `PermSettingsEdit` (admin-only); previously accessible to any authenticated API key, enabling potential DoS via expensive embedding operations

### API Rate Limiting (High)
- New per-bearer-token sliding-window rate limiter applied to the entire `/api/v1/` subrouter: 300 requests per token per minute. Returns 429 with `Retry-After: 60` on violation.

### Asset serve_path Whitelist (Medium)
- `upload_asset` and `upload_asset_from_url` now enforce that `serve_path` begins with `/assets/`, `/images/`, `/docs/`, `/media/`, or `/files/`

### Trusted Proxy IP Extraction (Medium)
- Public search rate limiter now uses `middleware.GetClientIP` with the configured trusted proxy settings, preventing IP spoofing via forged `X-Forwarded-For` headers to bypass per-IP limits

### Database Indexes (Medium)
- Added compound index `{published: 1, deleted: 1}` on the `content` collection, covering the most common list query pattern and improving performance under load

### Audit Log TTL Index Idempotency (Low)
- Audit log TTL index creation now tolerates "already exists" errors, ensuring the 365-day TTL is always present even on databases created before this index was added

### Session Secret Validation (Low)
- Server now checks `SESSION_SECRET` length at startup: warns if < 32 characters, fails hard if < 16 characters in production mode

---

## v2.1.0 — Agentic API Improvements

### Theme Reliability
- **Theme CSS on startup**: `static/css/theme-vars.css` is now regenerated from the database every time the server starts, preventing blank styles after a deploy or container restart

### New Content Endpoints
- **`PUT /api/v1/content/by-path?path=/slug`**: Update content by URL path instead of MongoDB ID — useful when you know the page URL but not its ID
- **`POST /api/v1/content/batch-publish`**: Publish multiple content items in one call; pass an ID list or `publish_all_drafts: true`
- **`GET/POST /api/v1/content/{id}/preview`**: Render a content item's HTML without publishing; accepts optional title/data overrides to preview unsaved edits; returns `rendered_html` and `warnings` (missing required fields, unclosed tags, unresolved placeholders)
- **`GET /api/v1/content/{id}?include_rendered=true`**: Include rendered body HTML and validation warnings alongside regular content data

### New Asset Endpoints
- **`POST /api/v1/assets/from-url`**: Fetch a remote URL and store it as a LightCMS asset (HTTP/HTTPS only, 50 MB cap, MIME validation)

### Theme Version Pinning
- **`POST /api/v1/theme/versions/{version}/pin`** and **`/unpin`**: Lock (or unlock) a theme version to protect it from being auto-pruned
- `ThemeVersion` model gains `locked` field (stored in `theme_versions` collection)

### Scoped Search & Replace
- **`POST /api/v1/search-replace/scoped/preview`** and **`/execute`**: Run search-and-replace filtered by `content_ids`, `folder_path`, `template_name`, and/or `category` — safer than a full site-wide replacement

### New MCP Tools (13 added, 41 → 54 total)
- `update_content_by_path` — update by URL path; merges `data` fields like `update_content`
- `publish_multiple` — batch publish by ID list or all drafts at once
- `preview_content` — render HTML without saving; supports field overrides
- `scoped_search_replace_preview` / `scoped_search_replace_execute` — folder/template/category-scoped S&R
- `upload_asset_from_url` — fetch remote file and store as asset
- `pin_theme_version` / `unpin_theme_version` — protect important theme milestones
- Improved descriptions on `get_content`, `create_content`, `update_content`, `update_theme`, `search_replace_preview`, `search_replace_execute` — all now include workflow guidance and examples

---

## v2.0.1 — Configurable Search Ranking

### Configurable Search Ranking
- All search ranking parameters are now stored in the database (`settings` collection, `type=search_config`) and editable from the admin panel at **Tools → End User Search → Search Ranking**
- Previously hardcoded values (nav boost 0.15, title boost 0.20, concept template boost 0.05, `/videos/` demotion −0.05) are now the defaults that ship with every new install
- Configurable fields: title match boost, nav page boost, boosted template name substrings (one per line), template boost score, demoted path prefixes (one per line), demotion penalty score
- Values are clamped to −1.0…1.0 to prevent accidental misconfiguration
- Changes take effect immediately — in-memory cache is invalidated on save
- Save confirmation banner shown after successful update

### Search API Documentation
- Expanded typeahead suggest API documentation in the admin Integration Guide, including full parameter table, response schema, and a combined search+suggest JavaScript example
- Updated README with full Search API and Suggest API reference sections including JavaScript example

---

## v2.0.0 — Multi-User RBAC & Smart Search

### Multi-User Access Control
- **Role-Based Access Control (RBAC)**: Three roles — admin, editor, viewer — with granular permissions enforced on every admin UI page and REST API endpoint
- **User Management**: Admin-only panel at `/cm/users` for creating users, assigning roles, disabling accounts, and resetting passwords
- **Email-based login**: Authentication migrated from a single shared password to per-user email + password credentials
- **Force password change**: Temporary passwords prompt a mandatory change on first login
- **Automatic migration**: On first startup with an empty users collection, the existing admin password hash is carried over into a new admin user account

### Audit Logging
- **Persistent audit trail**: All mutations logged with acting user, action, resource, and timestamp
- **365-day retention**: Audit logs auto-expire via MongoDB TTL index
- **Filterable UI** at `/cm/audit`: filter by action type, resource, and date range
- **Async logging**: `LogAsync` fire-and-forget pattern to avoid blocking request handlers

### User-Scoped API Keys
- API keys now belong to a specific user and inherit that user's permissions
- Admins can view and manage all keys; non-admins can only manage their own
- Keys created before v2.0 remain functional as system-level keys with full access

### Smart Search Ranking
- **Structural boost**: Nav-linked pages (parsed from header HTML, cached 5 min) surface above other results
- **Template-based ranking**: Concept pages rank above generic body-only content
- **Video deprioritisation**: Pages under `/videos/` rank below all other content types
- **Typeahead suggestions**: Same structural ranking applied to prefix-match suggestions
- Ranking priority: title+nav > title-only > nav-linked > concept pages > body-only > video transcripts

### Bug Fixes
- Fixed `/cm/audit` page crash caused by `subtract`/`add` template functions receiving mismatched integer types
- Made arithmetic template functions (`subtract`, `add`) type-flexible via `interface{}` dispatch

---

## v1.4.0 — End-User Search API

- **Full-text search** (`/api/search?q=...`): regex-based exact matching across all published content
- **Semantic vector search**: Voyage AI embeddings stored in MongoDB Atlas; `$vectorSearch` pipeline for similarity queries
- **Hybrid mode**: Reciprocal rank fusion (RRF, k=60) merges full-text and semantic results into a single ranked list
- **Title boosting**: Results where the query appears in the page title float above body-only matches
- **Graceful degradation**: Works without a Voyage API key (full-text only); automatically enables semantic search when configured
- **Rate limiting**: Per-IP (10 req/min) and global (100 req/min) limits with DDoS protection
- **Embedding pipeline**: Background batch generation with progress tracking in the admin panel
- **Typeahead suggestions**: `/api/search/suggest` endpoint for prefix-matching page titles and extracted keywords
- **WARP proxy**: Voyage API calls routed through Cloudflare WARP on Fly.io to avoid IP-based rate limiting
- Upgraded embedding model from `voyage-3-lite` to `voyage-4-lite`
- Fixed SVG assets not displaying when uploaded with `/assets` path prefix
- Expanded upload allowlist to include CSS, JS, JSON, and other text-based web assets

---

## v1.2.0 — OAuth 2.1 & HTTP MCP Transport

- **OAuth 2.1 authorization server**: Full authorization code flow with PKCE (S256), dynamic client registration (RFC 7591), token rotation, and revocation (RFC 7009)
- **Remote MCP clients**: HTTP streamable MCP endpoint at `/mcp` — connects Claude's Cowork, Claude Desktop, and any MCP-compatible app without embedding credentials
- **Discovery endpoints**: `/.well-known/oauth-authorization-server` (RFC 8414) and `/.well-known/oauth-protected-resource` (RFC 9728) for automatic client setup
- **Dynamic MCP server card**: `/.well-known/mcp/server-card.json` with full tool schemas, served live from the running server
- **Smithery registry support**: `smithery.yaml` and packaging config for registry publication
- **Test suite**: 82% → 86% coverage with CI via GitHub Actions and Codecov integration
- Loading state feedback on OAuth authorize buttons

---

## v1.1.0 — REST API, CLI Tool & API Keys

- **REST API** at `/api/v1/`: full JSON API for all content management operations — content, templates, assets, theme, config, redirects, folders, collections
- **API key authentication**: `lc_`-prefixed keys stored as SHA-256 hashes; created and managed in the admin panel
- **CLI tool** (`cmd/cli`): command-line interface wrapping the REST API for scripting and CI/CD workflows
- **MCP refactor**: MCP tools now use the REST API client (`internal/apiclient`) rather than direct DB access
- Partial update support on all PUT endpoints (send only changed fields)

---

## v1.0.0 — Initial Release

- **Content management**: Create, edit, publish, and delete content using customizable templates
- **Template system**: Define reusable page structures with typed fields (text, textarea, richtext, date, image, select); HTML layout with `{{.field_name}}` placeholders
- **Static page generation**: Published content rendered to `content/generated/` for fast, zero-runtime serving
- **Content versioning**: Automatic version snapshots on every update; revert to any prior version
- **Soft delete**: Deleted content recoverable from the admin panel
- **Content collections**: Auto-generated paginated listing pages filtered by category
- **Folders & URL organization**: Hierarchical path structure for content
- **MCP server** (stdio): 43 tools for managing the entire site through AI agents (Claude Code, Claude Desktop)
- **Theme customization**: Colors, fonts, border radius, custom CSS; header/footer HTML injection
- **Theme versioning**: Full history of theme changes with one-click revert
- **Asset management**: Upload, organize, and serve images, documents, and other files
- **URL redirects**: 301/302 rules managed from the admin panel
- **Rich text editor**: TinyMCE integration for visual editing
- **Search & replace**: Site-wide text replacement with preview before execution
- **Admin branding**: Custom logo and site name in the admin panel
- **Security**: CSRF protection, bcrypt passwords (cost 12), session cookies (SameSite=Strict), file upload validation, login rate limiting
- **Fly.io deployment**: `fly.toml` and `Dockerfile` for one-command production deploy
- Deployed at https://metavert-cms.fly.dev/
