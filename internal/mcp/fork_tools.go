package mcp

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type ListForksInput struct{}

type CreateForkInput struct {
	Name        string `json:"name" jsonschema:"Fork workspace name,required"`
	Description string `json:"description,omitempty" jsonschema:"Optional description of what this fork is for"`
}

type GetForkInput struct {
	ID string `json:"id" jsonschema:"Fork ID (returned by create_fork or list_forks),required"`
}

type ForkPageInput struct {
	ForkID    string `json:"fork_id" jsonschema:"Fork workspace ID,required"`
	ContentID string `json:"content_id,omitempty" jsonschema:"ID of the live content item to fork into this workspace"`
	Path      string `json:"path,omitempty" jsonschema:"URL path of the live page to fork (e.g. /about). Use instead of content_id when you know the path."`
}

type RemoveForkPageInput struct {
	ForkID string `json:"fork_id" jsonschema:"Fork workspace ID,required"`
	PageID string `json:"page_id" jsonschema:"ID of the fork page to remove (not the live content ID),required"`
}

type MergeForkInput struct {
	ForkID     string `json:"fork_id" jsonschema:"Fork workspace ID to merge into live,required"`
	Digest     string `json:"digest" jsonschema:"The digest get_fork_diff returned for the changes you reviewed. The merge is refused if the fork or a page it changes was edited since,required"`
	PublishNew bool   `json:"publish_new,omitempty" jsonschema:"Also publish the pages this merge creates. Default false: new pages keep the publish state they had in the fork (normally draft). Pages on hold always stay drafts"`
}

type PurgeForkCopiesInput struct {
	ForkID string `json:"fork_id" jsonschema:"ID of a merged or archived fork,required"`
	DryRun bool   `json:"dry_run,omitempty" jsonschema:"If true, return the count and the list of {id, full_path} without deleting anything"`
}

type ArchiveForkInput struct {
	ForkID string `json:"fork_id" jsonschema:"Fork workspace ID to archive,required"`
}

type DeleteForkInput struct {
	ForkID string `json:"fork_id" jsonschema:"Fork workspace ID to permanently delete,required"`
}

func (s *Server) registerForkTools() {
	// List forks
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:  "list_forks",
		Title: "List Forks",
		Description: `List all content fork workspaces. Forks let you stage changes to multiple pages as a batch before merging them live.

Each fork shows its status (active/merged/archived), page count, and who created it.
Use get_fork to see the specific pages in a fork.`,
		Annotations: &mcp.ToolAnnotations{
			Title:         "List Forks",
			ReadOnlyHint:  true,
			OpenWorldHint: boolPtr(false),
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, args ListForksInput) (*mcp.CallToolResult, any, error) {
		forks, err := s.client.ListForks(ctx)
		if err != nil {
			return errorResult(err), nil, nil
		}
		return jsonResult(forks), nil, nil
	})

	// Create fork
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:  "create_fork",
		Title: "Create Fork",
		Description: `Create a new named fork workspace for staging content changes.

A fork is an isolated workspace where you can edit copies of live pages without affecting the public site.
When ready, an admin can merge the fork to push all changes live at once.

Typical workflow:
1. create_fork — create the workspace
2. fork_page — copy pages you want to edit into the fork (returns a fork page ID)
3. update_content (with the fork page ID) — make your edits
4. merge_fork — merge all changes to the live site (admin only)

Returns the fork ID needed for subsequent fork operations.`,
		Annotations: &mcp.ToolAnnotations{
			Title:           "Create Fork",
			ReadOnlyHint:    false,
			DestructiveHint: boolPtr(false),
			IdempotentHint:  false,
			OpenWorldHint:   boolPtr(false),
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, args CreateForkInput) (*mcp.CallToolResult, any, error) {
		result, err := s.client.CreateFork(ctx, args.Name, args.Description)
		if err != nil {
			return errorResult(err), nil, nil
		}
		return jsonResult(result), nil, nil
	})

	// Get fork
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:  "get_fork",
		Title: "Get Fork",
		Description: `Get details of a fork workspace including its status and list of pages.

Returns: fork metadata + array of pages (id, title, full_path, updated_at).
Use the page id with get_content or update_content to read/edit fork pages.`,
		Annotations: &mcp.ToolAnnotations{
			Title:         "Get Fork",
			ReadOnlyHint:  true,
			OpenWorldHint: boolPtr(false),
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, args GetForkInput) (*mcp.CallToolResult, any, error) {
		fork, err := s.client.GetFork(ctx, args.ID)
		if err != nil {
			return errorResult(err), nil, nil
		}
		return jsonResult(fork), nil, nil
	})

	// Fork page
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:  "fork_page",
		Title: "Fork Page",
		Description: `Copy a live page into a fork workspace so you can edit it without affecting the public site.

Provide either:
- path: the URL path of the page (e.g. "/about", "/blog/my-post") — preferred when you know the URL
- content_id: the MongoDB ObjectID of the content item

Returns the fork page ID. Use this ID with update_content to make edits:
  1. fork_page → get fork_page_id
  2. update_content with id=fork_page_id to edit
  3. get_content with id=fork_page_id to verify
  4. merge_fork when all edits are ready (admin only)

If the page is already in this fork, returns the existing fork copy.`,
		Annotations: &mcp.ToolAnnotations{
			Title:           "Fork Page",
			ReadOnlyHint:    false,
			DestructiveHint: boolPtr(false),
			IdempotentHint:  true,
			OpenWorldHint:   boolPtr(false),
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, args ForkPageInput) (*mcp.CallToolResult, any, error) {
		if args.ForkID == "" {
			return errorResult(fmt.Errorf("fork_id is required")), nil, nil
		}
		if args.ContentID == "" && args.Path == "" {
			return errorResult(fmt.Errorf("either content_id or path is required")), nil, nil
		}
		result, err := s.client.ForkPage(ctx, args.ForkID, args.ContentID, args.Path)
		if err != nil {
			return errorResult(err), nil, nil
		}
		return jsonResult(result), nil, nil
	})

	// Remove fork page
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "remove_fork_page",
		Title:       "Remove Fork Page",
		Description: "Remove a page from a fork workspace (discards the fork copy, does not affect the live page).",
		Annotations: &mcp.ToolAnnotations{
			Title:           "Remove Fork Page",
			ReadOnlyHint:    false,
			DestructiveHint: boolPtr(true),
			IdempotentHint:  true,
			OpenWorldHint:   boolPtr(false),
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, args RemoveForkPageInput) (*mcp.CallToolResult, any, error) {
		if err := s.client.RemoveForkPage(ctx, args.ForkID, args.PageID); err != nil {
			return errorResult(err), nil, nil
		}
		return textResult(fmt.Sprintf("Page %s removed from fork %s", args.PageID, args.ForkID)), nil, nil
	})

	// Merge fork
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:  "merge_fork",
		Title: "Merge Fork",
		Description: `Merge all pages in a fork workspace into live content, exactly as reviewed.

Call get_fork_diff first, review (and show the user) the changes, and pass its digest. The merge is refused if the fork, or a live page it changes, was edited after that diff was taken: review again and use the new digest.

Who can merge: a person whose role can merge, at /cm/forks/{id}. An API key or connection (this tool) can merge only a fork it created itself, and only when an admin has allowed it to merge its own forks under API Keys or Connected Apps, and only while its owner's role can merge. Anything else is refused: a person merges it.

For each page in the fork:
- If a matching live page exists (same URL): updates it with the fork content (fork wins). If the live page was edited after the fork was created, records a conflict but still merges.
- If no live page exists at that URL: creates a new live page. It keeps the publish state it had in the fork (normally draft) unless publish_new is true, which publishes it. Pages on hold always stay drafts.

Each live page the merge creates or changes gets a version in its history, so it can be reverted; rollback_agent_session undoes a merge made in that session. After merging, the fork status changes to "merged", its page copies are deleted, and the created/updated counts are kept on the fork record. Published live pages are regenerated immediately.

Returns: updated and created counts, created_ids and updated_ids (live page IDs), not_published (new pages publish_new left as drafts, with the reason), and any conflicts detected.

ALWAYS confirm with the user before merging, as this pushes changes to the live site.`,
		Annotations: &mcp.ToolAnnotations{
			Title:           "Merge Fork",
			ReadOnlyHint:    false,
			DestructiveHint: boolPtr(true),
			IdempotentHint:  false,
			OpenWorldHint:   boolPtr(false),
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, args MergeForkInput) (*mcp.CallToolResult, any, error) {
		if args.Digest == "" {
			return errorResult(fmt.Errorf("digest is required: call get_fork_diff, review the changes, and pass its digest")), nil, nil
		}
		result, err := s.client.MergeFork(ctx, args.ForkID, args.PublishNew, args.Digest)
		if err != nil {
			return errorResult(err), nil, nil
		}
		return jsonResult(result), nil, nil
	})

	// Archive fork
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "archive_fork",
		Title:       "Archive Fork",
		Description: "Archive a fork without merging it. The fork and its pages are preserved but the fork becomes read-only. Requires admin role.",
		Annotations: &mcp.ToolAnnotations{
			Title:           "Archive Fork",
			ReadOnlyHint:    false,
			DestructiveHint: boolPtr(false),
			IdempotentHint:  true,
			OpenWorldHint:   boolPtr(false),
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, args ArchiveForkInput) (*mcp.CallToolResult, any, error) {
		if err := s.client.ArchiveFork(ctx, args.ForkID); err != nil {
			return errorResult(err), nil, nil
		}
		return textResult(fmt.Sprintf("Fork %s archived", args.ForkID)), nil, nil
	})

	// Delete fork
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "delete_fork",
		Title:       "Delete Fork",
		Description: "Permanently delete a fork and all its page copies. Deleting a merged fork removes only its history record (and any leftover copies) — the merged live pages are not affected. Requires admin role.",
		Annotations: &mcp.ToolAnnotations{
			Title:           "Delete Fork",
			ReadOnlyHint:    false,
			DestructiveHint: boolPtr(true),
			IdempotentHint:  false,
			OpenWorldHint:   boolPtr(false),
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, args DeleteForkInput) (*mcp.CallToolResult, any, error) {
		if err := s.client.DeleteFork(ctx, args.ForkID); err != nil {
			return errorResult(err), nil, nil
		}
		return textResult(fmt.Sprintf("Fork %s deleted", args.ForkID)), nil, nil
	})

	// Purge leftover copies of a merged/archived fork
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:  "purge_fork_copies",
		Title: "Purge Fork Copies",
		Description: `Delete the page copies still attached to a merged or archived fork. Requires admin role.

Merges clean up after themselves since v7.4; forks merged before that kept their copies. Live pages are never touched, and the fork record is kept. Active forks are refused.

Use dry_run: true first to see the count and the list of {id, full_path}. Returns the number deleted and writes an audit log entry.`,
		Annotations: &mcp.ToolAnnotations{
			Title:           "Purge Fork Copies",
			ReadOnlyHint:    false,
			DestructiveHint: boolPtr(true),
			IdempotentHint:  true,
			OpenWorldHint:   boolPtr(false),
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, args PurgeForkCopiesInput) (*mcp.CallToolResult, any, error) {
		if args.ForkID == "" {
			return errorResult(fmt.Errorf("fork_id is required")), nil, nil
		}
		result, err := s.client.PurgeForkCopies(ctx, args.ForkID, args.DryRun)
		if err != nil {
			return errorResult(err), nil, nil
		}
		return jsonResult(result), nil, nil
	})
}
