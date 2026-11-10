package mcp

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// sandboxFork returns the caller's open sandbox, which the server keeps on the
// API key or OAuth connection: the same in every MCP session, and kept across
// restarts. When it cannot be read, writes are not redirected; the server
// still refuses anything a sandboxed caller may not do.
func (s *Server) sandboxFork(ctx context.Context) (id, name string, active bool) {
	state, err := s.client.GetSandbox(ctx)
	if err != nil || state == nil || !state.Active {
		return "", "", false
	}
	return state.ForkID, state.Name, true
}

// sandboxTargetForContent resolves which content ID an update should hit
// while a sandbox is active: fork copies pass through, live pages get a
// copy-on-write fork copy, and pages from other forks are rejected.
func (s *Server) sandboxTargetForContent(ctx context.Context, contentID string) (string, error) {
	forkID, _, active := s.sandboxFork(ctx)
	if !active {
		return contentID, nil
	}
	content, err := s.client.GetContent(ctx, contentID, false)
	if err != nil {
		return "", fmt.Errorf("resolve sandbox target: %w", err)
	}
	if content.ForkID == forkID {
		return contentID, nil // already the sandbox copy
	}
	if content.ForkID != "" {
		return "", fmt.Errorf("content %s belongs to another fork (%s); the active sandbox is %s", contentID, content.ForkID, forkID)
	}
	// Live page: copy-on-write into the sandbox (idempotent).
	page, err := s.client.ForkPage(ctx, forkID, contentID, "")
	if err != nil {
		return "", fmt.Errorf("sandbox copy-on-write failed: %w", err)
	}
	return page.ID, nil
}

// sandboxTargetForPath is the path-based variant of sandboxTargetForContent.
// It returns the ID of the sandbox copy for the page at path ("" when no
// sandbox is active, meaning: use the normal by-path flow).
func (s *Server) sandboxTargetForPath(ctx context.Context, path string) (string, error) {
	forkID, _, active := s.sandboxFork(ctx)
	if !active {
		return "", nil
	}
	page, err := s.client.ForkPage(ctx, forkID, "", path)
	if err != nil {
		return "", fmt.Errorf("sandbox copy-on-write failed for %s: %w", path, err)
	}
	return page.ID, nil
}

type StartAgentSandboxInput struct {
	Name        string `json:"name,omitempty" jsonschema:"Short name for the sandbox fork (e.g. 'refresh-pricing-pages'). Auto-generated when omitted."`
	Description string `json:"description,omitempty" jsonschema:"What this agent session intends to change — shown to the human reviewer."`
}

type EndAgentSandboxInput struct {
	Action string `json:"action" jsonschema:"'submit' keeps the fork and hands it to a human for review/merge; 'discard' deletes the fork and every change in it."`
}

func (s *Server) registerSandboxTools() {
	// Start sandbox
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:  "start_agent_sandbox",
		Title: "Start Agent Sandbox",
		Description: `Start a sandboxed editing session backed by a content fork ("pull request for content").

The server records the sandbox on this API key or connection. Until it ends, every request from this key or connection is limited to the fork, in any session, and across restarts:
- update_content / update_content_by_path copy pages into the fork and edit the copies (copy-on-write). Live content is not modified.
- create_content creates new pages inside the fork.
- Anything else that would change the live site is refused by the server.
- The site is unaffected until the fork is merged at /cm/forks.

A key or connection set to Sandbox access is always limited to forks it created; it needs a sandbox (or create_fork) before it can change anything. On one with Full access the sandbox is an opt-in safety net. Recommended for any multi-page or risky editing task. End with end_agent_sandbox.`,
		Annotations: &mcp.ToolAnnotations{
			Title:           "Start Agent Sandbox",
			ReadOnlyHint:    false,
			DestructiveHint: boolPtr(false),
			IdempotentHint:  false,
			OpenWorldHint:   boolPtr(false),
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, args StartAgentSandboxInput) (*mcp.CallToolResult, any, error) {
		state, err := s.client.StartSandbox(ctx, args.Name, args.Description)
		if err != nil {
			return errorResult(err), nil, nil
		}
		if state.ForkID == "" {
			return textResult("sandbox started but no ID returned by the server"), nil, nil
		}
		return jsonResult(map[string]interface{}{
			"success":    true,
			"fork_id":    state.ForkID,
			"name":       state.Name,
			"review_url": state.ReviewURL,
			"message":    "Sandbox active. All content writes now go into this fork; live content is protected. Call end_agent_sandbox when done.",
		}), nil, nil
	})

	// Sandbox status
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "get_agent_sandbox",
		Title:       "Get Agent Sandbox Status",
		Description: "Show whether an agent sandbox is open for this API key or connection, and if so its fork ID, name, and the pages changed so far (with per-field diffs against live). Also says whether this key or connection is set to Sandbox access.",
		Annotations: &mcp.ToolAnnotations{
			Title:        "Get Agent Sandbox Status",
			ReadOnlyHint: true,
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct{}) (*mcp.CallToolResult, any, error) {
		state, err := s.client.GetSandbox(ctx)
		if err != nil {
			return errorResult(err), nil, nil
		}
		if !state.Active {
			if state.Restricted {
				return textResult("No agent sandbox is open. This connection is set to Sandbox access: it can change only forks it created, so start a sandbox with start_agent_sandbox before changing content."), nil, nil
			}
			return textResult("No agent sandbox is active. Content tools operate on live content."), nil, nil
		}
		diff, err := s.client.GetForkDiff(ctx, state.ForkID)
		if err != nil {
			return errorResult(err), nil, nil
		}
		return jsonResult(map[string]interface{}{
			"active":     true,
			"restricted": state.Restricted,
			"fork_id":    state.ForkID,
			"name":       state.Name,
			"changes":    diff.Pages,
		}), nil, nil
	})

	// End sandbox
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "end_agent_sandbox",
		Title:       "End Agent Sandbox",
		Description: `End the sandbox. action='submit' leaves the fork for review and merge (at /cm/forks/{id}); action='discard' permanently deletes the fork and all its changes. A key or connection set to Sandbox access stays limited to forks it created afterwards. Merging is a separate step (merge_fork) that a sandboxed connection can't take.`,
		Annotations: &mcp.ToolAnnotations{
			Title:           "End Agent Sandbox",
			ReadOnlyHint:    false,
			DestructiveHint: boolPtr(true),
			IdempotentHint:  false,
			OpenWorldHint:   boolPtr(false),
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, args EndAgentSandboxInput) (*mcp.CallToolResult, any, error) {
		if args.Action != "submit" && args.Action != "discard" {
			return textResult("action must be 'submit' or 'discard'"), nil, nil
		}
		forkID, name, active := s.sandboxFork(ctx)
		if !active {
			return textResult("No agent sandbox is active."), nil, nil
		}
		if args.Action == "discard" {
			result, err := s.client.EndSandbox(ctx, "discard")
			if err != nil {
				return errorResult(err), nil, nil
			}
			msg := fmt.Sprintf("Sandbox %q discarded — fork and all its changes deleted. Live content was never modified.", name)
			if restricted, _ := result["restricted"].(bool); restricted {
				msg += " This connection is set to Sandbox access and can still change only forks it creates."
			}
			return textResult(msg), nil, nil
		}

		diff, err := s.client.GetForkDiff(ctx, forkID)
		if err != nil {
			return errorResult(err), nil, nil
		}
		result, err := s.client.EndSandbox(ctx, "submit")
		if err != nil {
			return errorResult(err), nil, nil
		}
		return jsonResult(map[string]interface{}{
			"success":    true,
			"fork_id":    forkID,
			"name":       name,
			"changes":    diff.Pages,
			"review_url": "/cm/forks/" + forkID,
			"restricted": result["restricted"],
			"message":    "Sandbox submitted. A person can review the changes and merge the fork from the admin UI.",
		}), nil, nil
	})

	// Fork diff (also useful outside sandbox sessions, e.g. for reviewers)
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "get_fork_diff",
		Title:       "Get Fork Diff",
		Description: "Per-field differences between each page in a fork and its live counterpart — the review surface for merging a fork. Status 'added' means the page exists only in the fork.",
		Annotations: &mcp.ToolAnnotations{
			Title:        "Get Fork Diff",
			ReadOnlyHint: true,
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, args GetForkInput) (*mcp.CallToolResult, any, error) {
		diff, err := s.client.GetForkDiff(ctx, args.ID)
		if err != nil {
			return errorResult(err), nil, nil
		}
		return jsonResult(diff), nil, nil
	})
}

type AgentSessionInput struct {
	SessionID string `json:"session_id,omitempty" jsonschema:"Agent session ID. Defaults to this MCP session's own ID."`
}

// registerGovernanceTools adds the agent-session ledger tools.
func (s *Server) registerGovernanceTools() {
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "get_agent_session_changes",
		Title:       "Get Agent Session Changes",
		Description: "The audit ledger for an agent session: every content item it changed, with actions and timestamps. Defaults to the current MCP session. Requires audit permissions.",
		Annotations: &mcp.ToolAnnotations{
			Title:        "Get Agent Session Changes",
			ReadOnlyHint: true,
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, args AgentSessionInput) (*mcp.CallToolResult, any, error) {
		sessionID := args.SessionID
		if sessionID == "" {
			sessionID = s.client.AgentSession()
		}
		if sessionID == "" {
			return textResult("no session_id given and this MCP session has no agent-session ID"), nil, nil
		}
		changes, err := s.client.GetAgentSessionChanges(ctx, sessionID)
		if err != nil {
			return errorResult(err), nil, nil
		}
		return jsonResult(changes), nil, nil
	})

	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "rollback_agent_session",
		Title:       "Rollback Agent Session",
		Description: `Undo every content change made by an agent session as a unit: content the session created is soft-deleted, deleted content is restored, and updated content reverts to its latest pre-session version. Defaults to the current MCP session ("undo everything I did"). Non-content changes (templates, settings) are listed for manual review but not auto-reverted.`,
		Annotations: &mcp.ToolAnnotations{
			Title:           "Rollback Agent Session",
			ReadOnlyHint:    false,
			DestructiveHint: boolPtr(true),
			IdempotentHint:  true,
			OpenWorldHint:   boolPtr(false),
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, args AgentSessionInput) (*mcp.CallToolResult, any, error) {
		sessionID := args.SessionID
		if sessionID == "" {
			sessionID = s.client.AgentSession()
		}
		if sessionID == "" {
			return textResult("no session_id given and this MCP session has no agent-session ID"), nil, nil
		}
		result, err := s.client.RollbackAgentSession(ctx, sessionID)
		if err != nil {
			return errorResult(err), nil, nil
		}
		return jsonResult(result), nil, nil
	})
}

type BackfillPublishedDatesInput struct {
	DryRun bool `json:"dry_run,omitempty" jsonschema:"Only count the pages that would change"`
}

type RepairForkDamageInput struct {
	DryRun bool `json:"dry_run,omitempty" jsonschema:"Only list what would be repaired; change nothing"`
}

type MaintenanceScanInput struct {
	LinkCheck bool `json:"link_check,omitempty" jsonschema:"Also start an async broken-link check job"`
}

// registerMaintenanceTools adds the self-maintenance tools.
func (s *Server) registerMaintenanceTools() {
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "get_maintenance_report",
		Title:       "Get Maintenance Report",
		Description: "The latest site-health scan: stale pages (no update in ~6 months), published pages missing meta descriptions, and lingering drafts. Use it as a work queue — fix issues via the sandbox workflow.",
		Annotations: &mcp.ToolAnnotations{Title: "Get Maintenance Report", ReadOnlyHint: true},
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct{}) (*mcp.CallToolResult, any, error) {
		report, err := s.client.GetMaintenanceReport(ctx)
		if err != nil {
			return errorResult(err), nil, nil
		}
		return jsonResult(report), nil, nil
	})

	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "backfill_published_dates",
		Title:       "Backfill Published Dates",
		Description: "Give published pages that have no published date (created before v7.3.2) a published date equal to their creation date, for structured data and feeds. Metadata only: pages don't re-render. Use dry_run first to see the count.",
		Annotations: &mcp.ToolAnnotations{
			Title:        "Backfill Published Dates",
			ReadOnlyHint: false, DestructiveHint: boolPtr(false), IdempotentHint: true, OpenWorldHint: boolPtr(false),
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, args BackfillPublishedDatesInput) (*mcp.CallToolResult, any, error) {
		res, err := s.client.BackfillPublishedDates(ctx, args.DryRun)
		if err != nil {
			return errorResult(err), nil, nil
		}
		return jsonResult(res), nil, nil
	})

	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "repair_fork_damage",
		Title:       "Repair Fork Damage",
		Description: "Repair damage the admin editor could do when saving a fork copy before v7.4.1: (1) fork copies left flagged published get the flag cleared; (2) live published pages whose generated HTML file is missing are regenerated. Page content is not changed: no versions, no IndexNow pings. A flagged copy with has_live_page=false is a page created inside its fork, where the flag meant publish-on-merge — after the repair, merge that fork with publish_new to publish it. Run with dry_run first and show the user the lists. Requires admin role.",
		Annotations: &mcp.ToolAnnotations{
			Title:        "Repair Fork Damage",
			ReadOnlyHint: false, DestructiveHint: boolPtr(false), IdempotentHint: true, OpenWorldHint: boolPtr(false),
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, args RepairForkDamageInput) (*mcp.CallToolResult, any, error) {
		res, err := s.client.RepairForkDamage(ctx, args.DryRun)
		if err != nil {
			return errorResult(err), nil, nil
		}
		return jsonResult(res), nil, nil
	})

	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "run_maintenance_scan",
		Title:       "Run Maintenance Scan",
		Description: "Run a site-health scan now and return the fresh report. Scans also run automatically once a day.",
		Annotations: &mcp.ToolAnnotations{
			Title:        "Run Maintenance Scan",
			ReadOnlyHint: false, DestructiveHint: boolPtr(false), IdempotentHint: true, OpenWorldHint: boolPtr(false),
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, args MaintenanceScanInput) (*mcp.CallToolResult, any, error) {
		report, err := s.client.RunMaintenanceScan(ctx, args.LinkCheck)
		if err != nil {
			return errorResult(err), nil, nil
		}
		return jsonResult(report), nil, nil
	})
}

type IndexNowSetInput struct {
	Enabled bool `json:"enabled" jsonschema:"true to submit changed pages to IndexNow automatically, false to stop"`
}

type IndexNowSubmitInput struct {
	All   bool     `json:"all,omitempty" jsonschema:"Submit every published page (limited to once per hour)"`
	Paths []string `json:"paths,omitempty" jsonschema:"Specific site paths to submit, e.g. [\"/blog/post\"]"`
}

// registerIndexNowTools adds the IndexNow search-engine notification tools.
func (s *Server) registerIndexNowTools() {
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "get_indexnow_status",
		Title:       "Get IndexNow Status",
		Description: "IndexNow notifies Bing, Yandex and other engines when pages change. Returns whether it is active (and why not if inactive), the key file URL, whether the key file verified, total URLs submitted, and recent submissions with HTTP results.",
		Annotations: &mcp.ToolAnnotations{Title: "Get IndexNow Status", ReadOnlyHint: true},
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct{}) (*mcp.CallToolResult, any, error) {
		st, err := s.client.GetIndexNowStatus(ctx)
		if err != nil {
			return errorResult(err), nil, nil
		}
		return jsonResult(st), nil, nil
	})

	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "set_indexnow_enabled",
		Title:       "Enable/Disable IndexNow",
		Description: "Turn automatic IndexNow submission on or off for this site (on by default).",
		Annotations: &mcp.ToolAnnotations{
			Title:        "Enable/Disable IndexNow",
			ReadOnlyHint: false, DestructiveHint: boolPtr(false), IdempotentHint: true, OpenWorldHint: boolPtr(false),
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, args IndexNowSetInput) (*mcp.CallToolResult, any, error) {
		st, err := s.client.SetIndexNowEnabled(ctx, args.Enabled)
		if err != nil {
			return errorResult(err), nil, nil
		}
		return jsonResult(st), nil, nil
	})

	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "submit_indexnow",
		Title:       "Submit URLs to IndexNow",
		Description: "Submit pages to IndexNow now. Changes are already submitted automatically on publish/update/delete; use this to resubmit specific paths, or all=true to submit every published page (once per hour max).",
		Annotations: &mcp.ToolAnnotations{
			Title:        "Submit URLs to IndexNow",
			ReadOnlyHint: false, DestructiveHint: boolPtr(false), IdempotentHint: false, OpenWorldHint: boolPtr(true),
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, args IndexNowSubmitInput) (*mcp.CallToolResult, any, error) {
		if !args.All && len(args.Paths) == 0 {
			return errorResult(fmt.Errorf("provide all=true or a list of paths")), nil, nil
		}
		res, err := s.client.SubmitIndexNow(ctx, args.All, args.Paths)
		if err != nil {
			return errorResult(err), nil, nil
		}
		return jsonResult(res), nil, nil
	})
}

// UpdateSEOSettingsInput is a partial update: omitted fields are unchanged.
type UpdateSEOSettingsInput struct {
	TrainingPolicy   *string           `json:"training_policy,omitempty" jsonschema:"AI training crawlers (GPTBot, ClaudeBot, Google-Extended…): allow or disallow"`
	AISearchPolicy   *string           `json:"ai_search_policy,omitempty" jsonschema:"AI search crawlers (OAI-SearchBot, PerplexityBot…): allow or disallow"`
	UserFetchPolicy  *string           `json:"user_fetch_policy,omitempty" jsonschema:"User-initiated AI fetchers (ChatGPT-User, Claude-User…): allow or disallow"`
	CrawlerOverrides map[string]string `json:"crawler_overrides,omitempty" jsonschema:"Per-crawler overrides by robots token, e.g. {\"GPTBot\":\"disallow\"}; replaces the whole map"`
	ContentSignals   *bool             `json:"content_signals,omitempty" jsonschema:"Emit a Content-Signal line in robots.txt"`
	RobotsExtra      *string           `json:"robots_extra,omitempty" jsonschema:"Extra robots.txt lines appended verbatim"`
	MarkdownDisabled *bool             `json:"markdown_disabled,omitempty" jsonschema:"true turns off Markdown copies at /<page>.md"`
	AuthorType       *string           `json:"author_type,omitempty" jsonschema:"Default author type: Person or Organization"`
	AuthorName       *string           `json:"author_name,omitempty" jsonschema:"Default author name for structured data and feeds"`
	AuthorURL        *string           `json:"author_url,omitempty" jsonschema:"Default author profile URL"`
	AuthorSameAs     []string          `json:"author_same_as,omitempty" jsonschema:"Author profile URLs (schema.org sameAs)"`
	PublisherSameAs  []string          `json:"publisher_same_as,omitempty" jsonschema:"Site/organization profile URLs (schema.org sameAs)"`
	FeedDisabled     *bool             `json:"feed_disabled,omitempty" jsonschema:"true turns off /feed.xml and /atom.xml"`
	FeedAllPages     *bool             `json:"feed_all_pages,omitempty" jsonschema:"Include every page in the site feed"`
	FeedTemplates    []string          `json:"feed_templates,omitempty" jsonschema:"Template names included in the site feed"`
	FeedCategories   []string          `json:"feed_categories,omitempty" jsonschema:"Categories included in the site feed"`
	FeedLimit        *int              `json:"feed_limit,omitempty" jsonschema:"Items per feed (1-500)"`
}

type AITrafficInput struct {
	Days int `json:"days,omitempty" jsonschema:"Days to cover (default 30, max 90)"`
}

// registerSEOTools adds the SEO & AI settings and AI traffic tools.
func (s *Server) registerSEOTools() {
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "get_seo_settings",
		Title:       "Get SEO & AI Settings",
		Description: "The site's search & AI settings: AI crawler policy (training / AI search / user fetch, per-crawler overrides) with the resulting robots.txt, Markdown copies, default author/publisher for structured data, and feed settings.",
		Annotations: &mcp.ToolAnnotations{Title: "Get SEO & AI Settings", ReadOnlyHint: true},
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct{}) (*mcp.CallToolResult, any, error) {
		res, err := s.client.GetSEOSettings(ctx)
		if err != nil {
			return errorResult(err), nil, nil
		}
		return jsonResult(res), nil, nil
	})

	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "update_seo_settings",
		Title:       "Update SEO & AI Settings",
		Description: "Change search & AI settings. Only the fields you pass change. Blocking AI training crawlers does not affect Google/Bing search.",
		Annotations: &mcp.ToolAnnotations{
			Title:        "Update SEO & AI Settings",
			ReadOnlyHint: false, DestructiveHint: boolPtr(false), IdempotentHint: true, OpenWorldHint: boolPtr(false),
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, args UpdateSEOSettingsInput) (*mcp.CallToolResult, any, error) {
		b, _ := json.Marshal(args)
		var updates map[string]interface{}
		json.Unmarshal(b, &updates) //nolint:errcheck
		if len(updates) == 0 {
			return errorResult(fmt.Errorf("no settings provided")), nil, nil
		}
		res, err := s.client.UpdateSEOSettings(ctx, updates)
		if err != nil {
			return errorResult(err), nil, nil
		}
		return jsonResult(res), nil, nil
	})

	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "get_ai_traffic",
		Title:       "Get AI Traffic",
		Description: "AI visibility report: requests from AI crawlers by purpose (training, AI search, user-initiated fetch) and by crawler, the pages AI reads most, visits referred by AI assistants (ChatGPT, Perplexity, Claude, Gemini…), and where they land.",
		Annotations: &mcp.ToolAnnotations{Title: "Get AI Traffic", ReadOnlyHint: true},
	}, func(ctx context.Context, req *mcp.CallToolRequest, args AITrafficInput) (*mcp.CallToolResult, any, error) {
		res, err := s.client.GetAITraffic(ctx, args.Days)
		if err != nil {
			return errorResult(err), nil, nil
		}
		return jsonResult(res), nil, nil
	})
}
