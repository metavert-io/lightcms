# LightCMS MCP Developer Guide

This document covers how to use the LightCMS MCP interface effectively, with a focus on agentic and programmatic workflows.

---

## 1. Overview

LightCMS ships with a dual-transport MCP server:

- **Stdio** — for local tools like Claude Desktop and Claude Code running on the same machine
- **Streamable HTTP** — for remote or sandboxed clients (Claude's Cowork, API agents, any MCP-compatible app)

Both transports expose the same 92 tools and 3 prompt resources. Authentication is enforced on all operations — API keys for direct access, OAuth 2.1 for remote clients.

This document is primarily aimed at developers building agentic workflows on top of LightCMS: bulk content pipelines, automated ingestion, site migrations, and AI-generated content publishing.

---

## 2. Connection

### Stdio (Claude Desktop / Claude Code)

Build the MCP binary and register it with your client:

```bash
# Build
go build -o bin/lightcms-mcp ./cmd/mcp

# Register with Claude Code (use the wrapper script, not the binary directly)
claude mcp add --transport stdio lightcms-mcp -- /path/to/lightcms/bin/lightcms-mcp-wrapper.sh
```

Or run the one-step setup script:

```bash
export LIGHTCMS_API_KEY=lc_your_key_here
./setup-mcp.sh
```

Restart Claude Code and run `/mcp` to verify the connection.

**Environment variables (stdio mode):**

| Variable | Description |
|----------|-------------|
| `LIGHTCMS_URL` | Server URL (default: `http://localhost:8082`) |
| `LIGHTCMS_API_KEY` | API key (required) |

### HTTP Streamable (Claude Code / API Agents)

The HTTP MCP endpoint is available at `/mcp` on your LightCMS instance. It supports both API key and OAuth 2.1 authentication.

```bash
# Register with Claude Code via HTTP transport
claude mcp add --transport http lightcms https://yoursite.example.com/mcp \
  --header "Authorization: Bearer lc_your_key_here"
```

For remote clients (Cowork, etc.) that can't carry embedded credentials, OAuth 2.1 discovery is available at `/.well-known/oauth-authorization-server`. The client handles authorization automatically — just provide your LightCMS URL. You sign in, and choose the connection's access: **Sandbox** (default: it can change only forks it creates) or **Full** (anything your role can do).

On the hosted endpoint, `upload_asset` takes `data_base64`; `file_path` reads a local file only when the MCP server runs on your own machine (the stdio binary).

---

## 3. ID Format

All `id` parameters in LightCMS MCP tools are **MongoDB ObjectIDs** — 24-character lowercase hex strings (e.g., `64c3b5e4fa7d0f2234567890`). IDs are returned by creation calls (`create_import_source`, `import_markdown`, etc.) and listing calls (`list_import_jobs`, `list_content`, etc.). Passing a string that is not a valid 24-hex-char ObjectID will return a `400 Bad Request`.

---

## 4. Available Tools

92 tools total across 12 categories, plus 3 prompt resources.

| Category | Count | Examples |
|----------|-------|---------|
| **Content** | 22 | create, read, update, delete, publish, unpublish, restore, versioning, bulk update, bulk field operation, export, backlinks |
| **Templates** | 5 | list, get, create, update, delete |
| **Snippets** | 5 | list, get, create, update, delete |
| **Assets** | 6 | upload, upload from URL, get, delete, list files, list folders |
| **Search** | 7 | full-text search, end-user search, search-replace (global + scoped, preview + execute), reindex embeddings |
| **Settings** | 23 | theme CRUD + versioning + pinning, site config, redirects, folders, collections, regenerate all |
| **Forks** | 8 | list, create, get, fork page, remove page, merge, archive, delete |
| **Import** | 10 | list/create/update/delete/trigger import sources, import markdown, import CSV, list/get/cancel import jobs |
| **Webhooks** | 6 | list, create, get, update, delete webhooks; regenerate secret |
| **Content Locking** | 4 | get lock, acquire lock, release lock, force-unlock |
| **Scheduled Publishing** | 3 | schedule publish, list scheduled, cancel scheduled |
| **Audit & Link Check** | 3 | get audit log, check links, list broken links |
| **Prompt Resources** | 3 | `lightcms://site/structure`, `lightcms://content/recent`, `lightcms://theme/config` |

---

## 3.5 Semantic Search

LightCMS uses Voyage AI embeddings for semantic search alongside traditional full-text search. This is especially useful for agentic workflows where you need to find content by meaning, not just keywords.

**Find content by topic:**
```
search_content(query="articles about database performance", semantic=true, limit=10)
→ returns semantically relevant results even if they don't contain those exact words
```

**Hybrid search (semantic + full-text, recommended):**
```
end_user_search(query="how do I deploy to production", limit=5)
→ combines semantic similarity with BM25 full-text for best coverage
```

**Use cases for agents:**
- Before bulk-importing content, search semantically to check for near-duplicates
- Find all pages covering a topic to update them consistently
- Identify content gaps by searching for topics that return few results
- Power the built-in chat widget — the same search pipeline drives `POST /api/chat/query`

**Reindex after bulk imports:**
```
reindex_embeddings()
→ regenerates Voyage AI embeddings for all content; call after large import_markdown batches
```

---

## 4. Agentic Workflow: Bulk Content Creation via Import

`import_markdown` is specifically designed for agentic bulk content creation. Instead of making dozens of individual `create_content` and `publish_content` calls, an agent generates all its pages as Markdown with YAML frontmatter, submits them in a single call, and polls one job for completion.

### Step 1: Design your content schema

Ask the agent to list templates and pick one:

```
list_templates
→ "Blog Post" template with fields: title, body, tags, author
```

### Step 2: Generate content as Markdown with frontmatter

The agent generates multiple Markdown pages, each with a YAML frontmatter block:

```markdown
---
title: Introduction to Async Go
slug: async-go-intro
folder: /blog
template: Blog Post
tags: go, concurrency
author: Claude
published: false
---

# Introduction to Async Go

Go's goroutines make concurrency a first-class citizen of the language.
Unlike threads, goroutines are lightweight — you can spawn thousands without
significant overhead. Here's how to think about async patterns in Go...
```

```markdown
---
title: Goroutine Patterns
slug: goroutine-patterns
folder: /blog
template: Blog Post
tags: go, concurrency, patterns
author: Claude
published: false
---

# Goroutine Patterns

The most common goroutine patterns you'll use in production Go code...
```

### Step 3: Import in bulk

```
import_markdown(
  pages=[
    { content: "---\ntitle: Introduction to Async Go\n...", filename: "async-go-intro.md" },
    { content: "---\ntitle: Goroutine Patterns\n...", filename: "goroutine-patterns.md" },
    ... (10 more)
  ],
  default_folder="/blog",
  default_template="Blog Post",
  auto_publish=false
)
→ { job_id: "64a1f3c2e8b5d90012345678", message: "Import job started" }
```

### Step 4: Monitor the job

```
get_import_job(id="64a1f3c2e8b5d90012345678", include_logs=true)
→ {
    job: { status: "done", total_pages: 12, created: 12, updated: 0, failed: 0 },
    logs: [
      { level: "info", message: "Imported /blog/async-go-intro", path: "/blog/async-go-intro" },
      { level: "info", message: "Imported /blog/goroutine-patterns", path: "/blog/goroutine-patterns" },
      ...
    ]
  }
```

### Step 5: Review and publish

```
list_content(folder="/blog", published=false)
→ 12 new draft pages

publish_multiple(ids=["id1", "id2", ..., "id12"])
→ all 12 pages are now live
```

### Why import_markdown beats individual create_content calls

| Approach | Calls for 12 pages | Behavior |
|----------|-------------------|---------|
| `create_content` × 12 + `publish_content` × 12 | 24 sequential calls | Synchronous, slow, verbose |
| `import_markdown` once + `get_import_job` once | 2 calls | Async, parallel goroutines, single job |

The import pipeline runs pages in parallel server-side. A 100-page batch typically completes in 2–5 seconds. All created pages are linked to one job for easy audit and rollback.

---

## 5. Agentic Workflow: CSV Data Ingestion

`import_csv` lets an agent ingest any structured data — product catalogs, staff directories, press release archives, event listings — without first building a template mapping layer.

Each CSV row becomes one content page. The `title_column` names which column becomes the page title; all other columns are stored as content fields automatically (if the target template has matching field names).

```
import_csv(
  csv_data="name,title,bio,photo_url\nJane Smith,CEO,\"Jane has led...\",/images/jane.jpg\nAlex Jones,CTO,\"Alex built...\",/images/alex.jpg",
  title_column="name",
  folder_path="/team",
  template_name="Team Member",
  auto_publish=true
)
→ { job_id: "64b2a4d3f9c6e01123456789", message: "Import job started" }
```

Then poll the job:

```
get_import_job(id="64b2a4d3f9c6e01123456789")
→ { status: "done", created: 2, updated: 0, failed: 0 }
```

**Tips:**
- Include a `slug` column to control URL paths explicitly; otherwise slugs are auto-generated from the title column.
- Re-importing the same CSV with updated values updates existing pages rather than creating duplicates (deduplication is by `full_path`).
- For large catalogs, chunk into 100-row CSV segments and poll between chunks.

---

## 6. Agentic Workflow: RSS Syndication Setup

`create_import_source` configures a recurring RSS/Atom feed that LightCMS polls on a schedule and imports automatically.

### Create a feed source

```
create_import_source(
  name="TechCrunch AI",
  url="https://techcrunch.com/category/artificial-intelligence/feed/",
  folder_path="/news/ai",
  template_name="Press Release",
  schedule="daily",
  auto_publish=false
)
→ { id: "64c3b5e4fa7d0f2234567890", name: "TechCrunch AI", schedule: "daily", active: true }
```

### Trigger immediately to test

```
trigger_import_source(id="64c3b5e4fa7d0f2234567890")
→ { job_id: "64c3b5e4fa7d0f2234567891" }

get_import_job(id="64c3b5e4fa7d0f2234567891", include_logs=true)
→ { status: "done", created: 8, updated: 0, failed: 0, logs: [...] }
```

### Review and adjust

```
list_content(folder="/news/ai", published=false)
→ 8 new draft articles

# Happy with results — enable auto_publish
update_import_source(
  id="64c3b5e4fa7d0f2234567890",
  auto_publish=true
)
```

### Manage sources

```
list_import_sources()
→ all configured RSS/Atom sources with last run status and next scheduled time

delete_import_source(id="64c3b5e4fa7d0f2234567890")
→ source removed; scheduled polling stops
```

**Schedules:** `hourly`, `daily`, `weekly`. The scheduler uses the configured `schedule` field; `trigger_import_source` overrides for immediate one-shot execution regardless of schedule.

---

## 6.5 Webhooks

Webhooks let external systems react to LightCMS content events in real time. Each webhook has its own HMAC-SHA256 secret for signature verification.

**Create a webhook:**
```
create_webhook(
  name="Publish Notifier",
  url="https://myapp.example.com/hooks/cms",
  events=["content.publish", "content.unpublish", "content.delete"],
  active=true
)
→ { id: "...", secret: "lc_wh_..." }  ← save this secret; shown only once
```

**Verify signatures in your receiver:**
```python
import hmac, hashlib
def verify(secret, body, signature):
    expected = "sha256=" + hmac.new(secret.encode(), body, hashlib.sha256).hexdigest()
    return hmac.compare_digest(expected, signature)
```

**Check delivery history:**
```
list_webhook_deliveries(id="...", limit=20)
→ recent delivery attempts with status codes and response bodies
```

**Regenerate a compromised secret:**
```
regenerate_webhook_secret(id="...")
→ { secret: "lc_wh_new..." }  ← update your receiver with this new secret
```

---

## 6.6 Content Locking & Scheduled Publishing

**Check if a page is being edited:**
```
get_content_lock(content_id="...")
→ { locked: true, user_email: "editor@example.com", expires_at: "..." }
  or { locked: false }
```

**Schedule a page to publish automatically:**
```
schedule_content_publish(
  content_id="...",
  publish_at="2026-04-01T09:00:00Z"
)
→ { success: true, publish_at: "2026-04-01T09:00:00Z" }
```

**List all scheduled content:**
```
list_scheduled_content()
→ content items with publish_at set, not yet published
```

**Cancel a scheduled publish:**
```
cancel_scheduled_publish(content_id="...")
```

---

## 7. Agentic Workflow: Site Structure Inspection

Before making bulk changes, give your agent full context about the site using MCP prompt resources. These resources are read-only and return structured data about the live site.

```
Read: lightcms://site/structure
→ folder tree, template list, content counts per folder

Read: lightcms://content/recent
→ 20 most recently updated content items with titles, paths, and publish status

Read: lightcms://theme/config
→ current theme colors, fonts, header/footer HTML
```

These resources are read automatically when Claude agents are given the LightCMS context — they appear as pre-loaded context before any tools are called. This means the agent already knows your site's folder structure and templates before it starts creating content, without needing an explicit `list_folders` + `list_templates` round trip.

**Typical agent startup sequence:**

1. Resources are pre-loaded: site structure, recent content, theme
2. Agent identifies the right folder and template for the task
3. Agent proceeds directly to content operations

---

## 8. Frontmatter Reference

All supported YAML frontmatter keys for `import_markdown`:

| Key | Type | Description |
|-----|------|-------------|
| `title` | string | Page title. Required if the filename doesn't provide one. |
| `slug` | string | URL path segment (e.g., `my-page`). Auto-generated from title if omitted. |
| `folder` | string | Target folder path (e.g., `/blog`). Falls back to `default_folder` if omitted. |
| `template` | string | Template name to use. Falls back to `default_template` if omitted. |
| `published` | bool | Set to `true` to auto-publish on import (overrides the `auto_publish` parameter). |
| `publish_at` | datetime | ISO 8601 date/time for scheduled publishing (e.g., `2026-04-01T09:00:00Z`). |
| `tags` | string | Comma-separated tag list (e.g., `go, concurrency, patterns`). |
| `description` | string | Meta description for SEO. Stored as `meta_description`. |
| Any other key | string | Stored as a content field if the target template has a matching field name. |

**Example with all keys:**

```markdown
---
title: Advanced Goroutine Patterns
slug: advanced-goroutine-patterns
folder: /blog/go
template: Blog Post
published: false
publish_at: 2026-04-15T09:00:00Z
tags: go, concurrency, advanced
description: Deep dive into production-grade goroutine patterns for Go engineers.
author: Claude
excerpt: From worker pools to fan-out/fan-in, these patterns solve real problems.
---

Body content here...
```

In this example, `author` and `excerpt` are stored as content fields because the "Blog Post" template defines those fields. Unknown keys that have no matching template field are silently ignored.

---

## 9. Performance Notes

- `import_markdown` with 100 pages processes in approximately 2–5 seconds. Pages are imported in parallel goroutines server-side.
- `import_csv` with 100 rows has similar throughput. Parsing is synchronous but page creation is parallelized.
- Deduplication is by `full_path` — re-importing the same slug updates the existing page rather than creating a duplicate. This makes import operations safely idempotent.
- For batches of 1,000+ pages, break into 100-page chunks and poll `get_import_job` between each chunk to avoid holding very large payloads in memory and to get per-chunk error visibility.
- RSS sources run on their configured schedule (hourly/daily/weekly). `trigger_import_source` overrides the schedule for immediate one-shot execution. Previously-imported feed items are skipped (deduplicated by feed item GUID / URL).
- Import job logs auto-expire after 90 days (MongoDB TTL index). Job records themselves are retained longer for auditing.
- `list_import_jobs` returns the 20 most recent jobs by default (configurable up to 100). For high-volume RSS setups, use the `limit` parameter to retrieve longer history.

---

## 10. Security Notes

- All MCP tools require a valid API key (`Authorization: Bearer lc_...`) or a valid OAuth 2.1 access token. Either acts as a person (the key's owner, or whoever approved the connection), with that person's current role.
- A key or connection set to **Sandbox** access can change only forks it created; the server refuses everything else, whichever tool asks. `start_agent_sandbox` records the open sandbox on the key or connection, so it holds across sessions and restarts.
- `merge_fork` needs the `digest` from `get_fork_diff`, and refuses if anything changed since. Through a key or connection it merges only forks that key or connection created, and only when an admin has allowed it to merge its own forks.
- Import tools (`import_markdown`, `import_csv`, `create_import_source`, `trigger_import_source`) require at minimum **editor** role. `delete_import_source` requires editor or admin.
- `cancel_import_job` requires editor or admin role.
- RSS feed URLs are fetched server-side by the LightCMS backend. Ensure your LightCMS server has outbound HTTP access to the feed host if you are running behind a firewall or NAT.
- Imported content is subject to the same sanitization pipeline as manually created content. The `markdown_script_policy` site setting controls whether raw HTML and `<script>` tags are permitted in imported markdown fields.
- Frontmatter values are treated as untrusted input — they are parsed and validated before being stored, not interpolated directly into templates.
- Import job logs are visible to any authenticated user with at least viewer role. Avoid including sensitive data in page titles or slugs if logs are shared with lower-trust users.
