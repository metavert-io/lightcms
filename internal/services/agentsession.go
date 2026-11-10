package services

import (
	"context"
	"fmt"
	"sort"
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// AgentSessionService provides the session ledger: everything an AI-agent
// session changed (grouped from audit logs by the X-Agent-Session header)
// and a best-effort rollback of those changes as a unit.
type AgentSessionService struct {
	audit   *AuditService
	content *ContentService
}

func NewAgentSessionService(audit *AuditService, content *ContentService) *AgentSessionService {
	return &AgentSessionService{audit: audit, content: content}
}

// AgentSessionChange summarizes what happened to one content item during a session.
type AgentSessionChange struct {
	ContentID string    `json:"content_id"`
	Path      string    `json:"path,omitempty"`
	Title     string    `json:"title,omitempty"`
	Actions   []string  `json:"actions"`
	FirstAt   time.Time `json:"first_at"`
	LastAt    time.Time `json:"last_at"`
}

// AgentSessionSummary is the ledger for one agent session.
type AgentSessionSummary struct {
	SessionID    string               `json:"session_id"`
	Entries      int                  `json:"entries"`
	ContentItems []AgentSessionChange `json:"content_items"`
	OtherActions []string             `json:"other_actions,omitempty"` // non-content mutations (templates, settings, …)
}

// Changes returns the ledger for a session.
func (s *AgentSessionService) Changes(ctx context.Context, sessionID string) (*AgentSessionSummary, error) {
	if sessionID == "" {
		return nil, fmt.Errorf("session ID is required")
	}
	logs, _, err := s.audit.List(ctx, AuditFilter{AgentSession: sessionID, Limit: 5000})
	if err != nil {
		return nil, err
	}
	// List returns newest-first; process oldest-first.
	sort.Slice(logs, func(i, j int) bool { return logs[i].CreatedAt.Before(logs[j].CreatedAt) })

	summary := &AgentSessionSummary{SessionID: sessionID, Entries: len(logs)}
	byContent := map[string]*AgentSessionChange{}
	var order []string

	touch := func(id, action string, at time.Time) *AgentSessionChange {
		c, ok := byContent[id]
		if !ok {
			c = &AgentSessionChange{ContentID: id, FirstAt: at}
			byContent[id] = c
			order = append(order, id)
		}
		c.Actions = append(c.Actions, action)
		c.LastAt = at
		return c
	}

	for _, l := range logs {
		if l.Action == "fork.merge" {
			// A merge changes live pages through the fork: each page it
			// created or updated is part of the session's changes.
			for _, id := range stringList(l.Details["created_ids"]) {
				touch(id, "fork.merge.create", l.CreatedAt)
			}
			for _, id := range stringList(l.Details["updated_ids"]) {
				touch(id, "fork.merge.update", l.CreatedAt)
			}
		}
		if l.Resource != "content" || l.ResourceID == "" {
			if l.Resource != "" && l.Action != "" {
				summary.OtherActions = append(summary.OtherActions, l.Action+" "+l.Resource+"/"+l.ResourceID)
			}
			continue
		}
		c := touch(l.ResourceID, l.Action, l.CreatedAt)
		if p, ok := l.Details["path"].(string); ok && p != "" {
			c.Path = p
		}
		if t, ok := l.Details["title"].(string); ok && t != "" {
			c.Title = t
		}
	}
	for _, id := range order {
		summary.ContentItems = append(summary.ContentItems, *byContent[id])
	}
	return summary, nil
}

// stringList reads a list of strings from an audit entry's details, as
// stored ([]string) or as read back from the database ([]interface{}).
func stringList(v interface{}) []string {
	switch list := v.(type) {
	case []string:
		return list
	case []interface{}:
		out := make([]string, 0, len(list))
		for _, item := range list {
			if s, ok := item.(string); ok {
				out = append(out, s)
			}
		}
		return out
	case primitive.A:
		return stringList([]interface{}(list))
	}
	return nil
}

// RollbackResult reports what a session rollback did per content item.
type RollbackResult struct {
	SessionID string               `json:"session_id"`
	Reverted  []string             `json:"reverted,omitempty"` // content updated → reverted to pre-session version
	Deleted   []string             `json:"deleted,omitempty"`  // content created → soft-deleted
	Restored  []string             `json:"restored,omitempty"` // content deleted → restored
	Skipped   []RollbackSkipReason `json:"skipped,omitempty"`
}

type RollbackSkipReason struct {
	ContentID string `json:"content_id"`
	Reason    string `json:"reason"`
}

// Rollback undoes a session's content changes: items the session created
// are soft-deleted, deleted items are restored, and updated items revert
// to their latest version predating the session's first touch. Pages a
// merge by the session created or updated count as created or updated by it.
// Non-content changes (templates, settings, …) are reported but not
// auto-reverted.
func (s *AgentSessionService) Rollback(ctx context.Context, sessionID string) (*RollbackResult, error) {
	summary, err := s.Changes(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	if summary.Entries == 0 {
		return nil, fmt.Errorf("no audit entries found for session %s", sessionID)
	}

	result := &RollbackResult{SessionID: sessionID}
	comment := "Rollback of agent session " + sessionID

	for _, item := range summary.ContentItems {
		id, err := primitive.ObjectIDFromHex(item.ContentID)
		if err != nil {
			result.Skipped = append(result.Skipped, RollbackSkipReason{item.ContentID, "invalid content ID"})
			continue
		}

		if _, err := s.content.GetContent(ctx, id); err != nil {
			// Typically a page copy the session made in a fork that was then
			// merged: merging removes the copies, and the merge's own entry
			// covers the live pages.
			result.Skipped = append(result.Skipped, RollbackSkipReason{item.ContentID, "no longer exists (fork page copies are removed when their fork is merged)"})
			continue
		}

		created, deleted := false, false
		for _, a := range item.Actions {
			switch a {
			case "content.create", "content.created", "fork.merge.create":
				created = true
			case "content.delete":
				deleted = true
			}
		}

		switch {
		case created:
			// The session created it — remove it (soft delete keeps recovery possible).
			if err := s.content.DeleteContent(ctx, id); err != nil {
				result.Skipped = append(result.Skipped, RollbackSkipReason{item.ContentID, "delete failed: " + err.Error()})
			} else {
				result.Deleted = append(result.Deleted, item.ContentID)
			}
		case deleted:
			if err := s.content.RestoreContent(ctx, id); err != nil {
				result.Skipped = append(result.Skipped, RollbackSkipReason{item.ContentID, "restore failed: " + err.Error()})
			} else {
				result.Restored = append(result.Restored, item.ContentID)
			}
		default:
			// Updated: revert to the newest version NOT authored by this
			// session. Provenance (version.AgentSession) is authoritative —
			// timestamps race, because a session's version write can land
			// milliseconds before its own audit entry. Versions without
			// provenance fall back to the timestamp comparison.
			versions, err := s.content.GetVersions(ctx, id)
			if err != nil || len(versions) == 0 {
				result.Skipped = append(result.Skipped, RollbackSkipReason{item.ContentID, "no version history"})
				continue
			}
			target := -1
			for _, v := range versions { // newest first
				if v.AgentSession == sessionID {
					continue // the session's own change — never a rollback target
				}
				if v.AgentSession == "" && !v.CreatedAt.Before(item.FirstAt) {
					continue // no provenance: only trust versions that predate the session
				}
				target = v.Version
				break
			}
			if target < 0 {
				result.Skipped = append(result.Skipped, RollbackSkipReason{item.ContentID, "no version predates the session"})
				continue
			}
			if err := s.content.RevertToVersion(ctx, id, target, comment); err != nil {
				result.Skipped = append(result.Skipped, RollbackSkipReason{item.ContentID, "revert failed: " + err.Error()})
			} else {
				result.Reverted = append(result.Reverted, item.ContentID)
			}
		}
	}

	return result, nil
}
