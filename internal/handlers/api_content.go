package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/metavert-io/lightcms/v7/internal/auth"
	"github.com/metavert-io/lightcms/v7/internal/models"
	"github.com/metavert-io/lightcms/v7/internal/services"

	"github.com/gorilla/mux"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// API Content endpoints

type contentListItem struct {
	ID              string                 `json:"id"`
	Title           string                 `json:"title"`
	Slug            string                 `json:"slug"`
	FullPath        string                 `json:"full_path"`
	Category        string                 `json:"category"`
	Tags            []string               `json:"tags,omitempty"`
	Published       bool                   `json:"published"`
	Hold            bool                   `json:"hold,omitempty"`
	Deleted         bool                   `json:"deleted"`
	UpdatedAt       string                 `json:"updated_at"`
	MetaDescription string                 `json:"meta_description,omitempty"`
	TemplateID      string                 `json:"template_id,omitempty"`
	TemplateName    string                 `json:"template_name,omitempty"`
	ForkID          string                 `json:"fork_id,omitempty"` // set on fork copies (only listed with include_forks)
	Data            map[string]interface{} `json:"data,omitempty"`
}

// forkIDHex renders an optional fork ID for API responses ("" for live pages).
func forkIDHex(id *primitive.ObjectID) string {
	if id == nil {
		return ""
	}
	return id.Hex()
}

// contentErrorStatus maps a content-service error to an HTTP status: a
// publish refused because the page is a fork copy or on hold is a conflict
// with the page's state, not a server fault.
func contentErrorStatus(err error) int {
	if errors.Is(err, services.ErrContentHeld) || errors.Is(err, services.ErrPublishForkCopy) {
		return http.StatusConflict
	}
	return http.StatusInternalServerError
}

// formatContentList converts content models to API response items with optional field filtering.
func (a *APIHandler) formatContentList(contents []models.Content, includeData bool, includeFieldsParam string) []contentListItem {
	if !includeData && includeFieldsParam == "" {
		// No data requested — return minimal items
		result := make([]contentListItem, len(contents))
		for i, c := range contents {
			result[i] = contentListItem{
				ID: c.ID.Hex(), Title: c.Title, Slug: c.Slug,
				FullPath: c.FullPath, Category: c.Category, Tags: c.Tags,
				Published: c.Published, Hold: c.Hold, Deleted: c.Deleted,
				UpdatedAt:       c.UpdatedAt.Format("2006-01-02T15:04:05Z"),
				MetaDescription: c.MetaDescription,
				TemplateID:      c.TemplateID.Hex(), TemplateName: c.TemplateName,
				ForkID: forkIDHex(c.ForkID),
			}
		}
		return result
	}

	var includeFields map[string]bool
	if includeFieldsParam != "" {
		includeFields = make(map[string]bool)
		for _, f := range strings.Split(includeFieldsParam, ",") {
			f = strings.TrimSpace(f)
			if f != "" {
				includeFields[f] = true
			}
		}
	}

	result := make([]contentListItem, len(contents))
	for i, c := range contents {
		item := contentListItem{
			ID: c.ID.Hex(), Title: c.Title, Slug: c.Slug,
			FullPath: c.FullPath, Category: c.Category, Tags: c.Tags,
			Published: c.Published, Hold: c.Hold, Deleted: c.Deleted,
			UpdatedAt:       c.UpdatedAt.Format("2006-01-02T15:04:05Z"),
			MetaDescription: c.MetaDescription,
			TemplateID:      c.TemplateID.Hex(), TemplateName: c.TemplateName,
			ForkID: forkIDHex(c.ForkID),
		}
		if includeFields != nil {
			item.Data = make(map[string]interface{})
			for k, v := range c.Data {
				if includeFields[k] {
					item.Data[k] = v
				}
			}
		} else {
			item.Data = c.Data
		}
		result[i] = item
	}
	return result
}

func (a *APIHandler) APIListContent(w http.ResponseWriter, r *http.Request) {
	if !a.requirePermission(w, r, auth.PermContentView) {
		return
	}
	includeDeleted, _ := strconv.ParseBool(r.URL.Query().Get("include_deleted"))
	// Fork copies are working copies, not site pages: listed only on request.
	includeForks, _ := strconv.ParseBool(r.URL.Query().Get("include_forks"))
	category := r.URL.Query().Get("category")
	includeData, _ := strconv.ParseBool(r.URL.Query().Get("include_data"))
	includeFieldsParam := r.URL.Query().Get("include_fields") // comma-separated field names

	var folderID *primitive.ObjectID
	if fid := r.URL.Query().Get("folder_id"); fid != "" {
		id, err := primitive.ObjectIDFromHex(fid)
		if err != nil {
			a.jsonError(w, http.StatusBadRequest, "invalid folder_id")
			return
		}
		folderID = &id
	}

	// Pagination: if limit is set, use paginated listing
	limitParam := r.URL.Query().Get("limit")
	offsetParam := r.URL.Query().Get("offset")

	if limitParam != "" {
		limit, err := strconv.Atoi(limitParam)
		if err != nil || limit < 1 {
			a.jsonError(w, http.StatusBadRequest, "limit must be a positive integer")
			return
		}
		if limit > 500 {
			limit = 500
		}
		offset := 0
		if offsetParam != "" {
			offset, _ = strconv.Atoi(offsetParam)
			if offset < 0 {
				offset = 0
			}
		}

		contents, pr, err := a.contentService.ListContentPaginated(r.Context(), services.PaginationOpts{
			Limit:          limit,
			Offset:         offset,
			IncludeDeleted: includeDeleted,
			IncludeForks:   includeForks,
			Category:       category,
			FolderID:       folderID,
		})
		if err != nil {
			a.jsonError(w, http.StatusInternalServerError, err.Error())
			return
		}

		items := a.formatContentList(contents, includeData, includeFieldsParam)
		a.jsonResponse(w, http.StatusOK, map[string]interface{}{
			"items":    items,
			"total":    pr.Total,
			"limit":    pr.Limit,
			"offset":   pr.Offset,
			"has_more": pr.HasMore,
		})
		return
	}

	contents, err := a.contentService.ListContent(r.Context(), includeDeleted, category, folderID, includeForks)
	if err != nil {
		a.jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// If no data requested, return as-is (existing behavior — raw Content models)
	if !includeData && includeFieldsParam == "" {
		a.jsonResponse(w, http.StatusOK, contents)
		return
	}

	a.jsonResponse(w, http.StatusOK, a.formatContentList(contents, includeData, includeFieldsParam))
}

func (a *APIHandler) APIGetContent(w http.ResponseWriter, r *http.Request) {
	if !a.requirePermission(w, r, auth.PermContentView) {
		return
	}
	id, err := primitive.ObjectIDFromHex(mux.Vars(r)["id"])
	if err != nil {
		a.jsonError(w, http.StatusBadRequest, "invalid content ID")
		return
	}

	content, err := a.contentService.GetContent(r.Context(), id)
	if err != nil {
		a.jsonError(w, http.StatusNotFound, "content not found")
		return
	}

	if r.URL.Query().Get("include_rendered") == "true" {
		rendered, warnings, _ := a.renderContentWithWarnings(r, content)
		type ContentWithRendered struct {
			*models.Content
			RenderedHTML string   `json:"rendered_html"`
			Warnings     []string `json:"warnings,omitempty"`
		}
		a.jsonResponse(w, http.StatusOK, ContentWithRendered{Content: content, RenderedHTML: rendered, Warnings: warnings})
		return
	}

	a.jsonResponse(w, http.StatusOK, content)
}

// contentByPath resolves the ?path= of a by-path request. Paths resolve to
// live pages only; a fork copy is reachable by path only when the caller
// names its fork with ?fork_id=, so a path can never silently land on a copy.
// It writes the error response itself and returns nil when resolution fails.
func (a *APIHandler) contentByPath(w http.ResponseWriter, r *http.Request, path string) *models.Content {
	if fid := r.URL.Query().Get("fork_id"); fid != "" {
		forkID, err := primitive.ObjectIDFromHex(fid)
		if err != nil {
			a.jsonError(w, http.StatusBadRequest, "invalid fork_id")
			return nil
		}
		if a.forkService == nil {
			a.jsonError(w, http.StatusNotFound, "content not found at path: "+path)
			return nil
		}
		content, err := a.forkService.GetForkPageByPath(r.Context(), forkID, path)
		if err != nil {
			a.jsonError(w, http.StatusNotFound, "content not found in fork at path: "+path)
			return nil
		}
		return content
	}
	content, err := a.contentService.GetContentByPath(r.Context(), path)
	if err != nil {
		a.jsonError(w, http.StatusNotFound, "content not found at path: "+path)
		return nil
	}
	return content
}

func (a *APIHandler) APIGetContentByPath(w http.ResponseWriter, r *http.Request) {
	if !a.requirePermission(w, r, auth.PermContentView) {
		return
	}
	path := r.URL.Query().Get("path")
	if path == "" {
		a.jsonError(w, http.StatusBadRequest, "path parameter is required")
		return
	}

	content := a.contentByPath(w, r, path)
	if content == nil {
		return
	}

	a.jsonResponse(w, http.StatusOK, content)
}

func (a *APIHandler) APIGetBacklinks(w http.ResponseWriter, r *http.Request) {
	if !a.requirePermission(w, r, auth.PermContentView) {
		return
	}
	path := r.URL.Query().Get("path")
	if path == "" {
		a.jsonError(w, http.StatusBadRequest, "path parameter is required")
		return
	}

	backlinks, err := a.contentService.GetBacklinks(r.Context(), path)
	if err != nil {
		a.jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}

	a.jsonResponse(w, http.StatusOK, backlinks)
}

// applyPublishedField applies a "published" value from a partial update. A
// request that changes whether a live page is published needs content.publish,
// like the publish and unpublish endpoints. A fork copy is never published, so
// the value is ignored there. It reports false after writing a refusal.
func (a *APIHandler) applyPublishedField(w http.ResponseWriter, r *http.Request, content *models.Content, raw map[string]json.RawMessage) bool {
	v, ok := raw["published"]
	if !ok || content.ForkID != nil {
		return true
	}
	var published bool
	if err := json.Unmarshal(v, &published); err != nil {
		a.jsonError(w, http.StatusBadRequest, "published must be true or false")
		return false
	}
	if published != content.Published && !a.requirePermission(w, r, auth.PermContentPublish) {
		return false
	}
	content.Published = published
	return true
}

func (a *APIHandler) APICreateContent(w http.ResponseWriter, r *http.Request) {
	if !a.requirePermission(w, r, auth.PermContentCreate) {
		return
	}

	var req struct {
		TemplateID      string                 `json:"template_id"`
		Title           string                 `json:"title"`
		Slug            string                 `json:"slug"`
		FolderPath      string                 `json:"folder_path"`
		Category        string                 `json:"category"`
		Tags            []string               `json:"tags"`
		MetaDescription string                 `json:"meta_description"`
		OGImage         string                 `json:"og_image"`
		NoIndex         bool                   `json:"noindex"`
		Hold            bool                   `json:"hold"`
		AuthorName      string                 `json:"author_name"`
		AuthorURL       string                 `json:"author_url"`
		Data            map[string]interface{} `json:"data"`
		Published       bool                   `json:"published"`
		UseHeader       bool                   `json:"use_header"`
		UseFooter       bool                   `json:"use_footer"`
		UseTheme        bool                   `json:"use_theme"`
		RawMode         bool                   `json:"raw_mode"`
		VersionComment  string                 `json:"version_comment"`
		Upsert          bool                   `json:"upsert"`
		ForkID          string                 `json:"fork_id"`
	}
	if err := a.decodeJSON(r, &req); err != nil {
		a.jsonError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.TemplateID == "" || req.Title == "" || req.Slug == "" {
		a.jsonError(w, http.StatusBadRequest, "template_id, title, and slug are required")
		return
	}

	// Creating a published page is publishing it. (A page created in a fork
	// goes live only through a merge.)
	if req.Published && req.ForkID == "" && !a.requirePermission(w, r, auth.PermContentPublish) {
		return
	}

	templateID, err := primitive.ObjectIDFromHex(req.TemplateID)
	if err != nil {
		a.jsonError(w, http.StatusBadRequest, "invalid template_id")
		return
	}

	// Get template name
	tmpl, err := a.templateService.GetTemplate(r.Context(), templateID)
	if err != nil {
		a.jsonError(w, http.StatusBadRequest, "template not found")
		return
	}

	// Resolve folder
	var folderID *primitive.ObjectID
	folderPath := req.FolderPath
	if folderPath != "" {
		folders, _ := a.settingsService.ListFolders(r.Context())
		for _, f := range folders {
			if f.Path == folderPath {
				folderID = &f.ID
				break
			}
		}
	}

	// A sandboxed caller creates pages only inside its own forks.
	if a.getAPIUser(r).Sandboxed() && req.ForkID == "" {
		a.jsonError(w, http.StatusForbidden, sandboxRefusal)
		return
	}

	// Optional fork target: the new page is created inside the fork
	// workspace instead of live content, and reaches live only on merge.
	var forkID *primitive.ObjectID
	if req.ForkID != "" {
		if req.Upsert {
			a.jsonError(w, http.StatusBadRequest, "fork_id cannot be combined with upsert")
			return
		}
		fid, err := primitive.ObjectIDFromHex(req.ForkID)
		if err != nil {
			a.jsonError(w, http.StatusBadRequest, "invalid fork_id")
			return
		}
		fork, err := a.forkService.GetByID(r.Context(), fid)
		if err != nil {
			a.jsonError(w, http.StatusBadRequest, "fork not found")
			return
		}
		if fork.Status != "active" {
			a.jsonError(w, http.StatusBadRequest, "fork is not active (status: "+fork.Status+")")
			return
		}
		if !a.callerMayWriteFork(w, r, fork) {
			return
		}
		forkID = &fid
	}

	content := &models.Content{
		TemplateID:      templateID,
		TemplateName:    tmpl.Name,
		Title:           req.Title,
		Slug:            req.Slug,
		FolderID:        folderID,
		FolderPath:      folderPath,
		Category:        req.Category,
		Tags:            req.Tags,
		MetaDescription: req.MetaDescription,
		OGImage:         req.OGImage,
		NoIndex:         req.NoIndex,
		Hold:            req.Hold,
		AuthorName:      req.AuthorName,
		AuthorURL:       req.AuthorURL,
		Data:            req.Data,
		Published:       req.Published && forkID == nil, // a fork copy is never published
		UseHeader:       req.UseHeader,
		UseFooter:       req.UseFooter,
		UseTheme:        req.UseTheme,
		RawMode:         req.RawMode,
		ForkID:          forkID,
	}

	comment := req.VersionComment

	if req.Upsert {
		created, err := a.contentService.UpsertContent(r.Context(), content, comment)
		if err != nil {
			a.jsonError(w, contentErrorStatus(err), err.Error())
			return
		}
		action := "updated"
		if created {
			action = "created"
		}
		a.auditLog(r, "content."+action, "content", content.ID.Hex(), map[string]interface{}{"title": content.Title, "path": content.FullPath, "upsert": true})
		status := http.StatusOK
		if created {
			status = http.StatusCreated
		}
		a.jsonResponse(w, status, map[string]interface{}{
			"id":        content.ID.Hex(),
			"full_path": content.FullPath,
			"action":    action,
			"content":   content,
		})
		return
	}

	args := []string{}
	if comment != "" {
		args = append(args, comment)
	}
	if err := a.contentService.CreateContent(r.Context(), content, args...); err != nil {
		a.jsonError(w, contentErrorStatus(err), err.Error())
		return
	}

	a.auditLog(r, "content.create", "content", content.ID.Hex(), map[string]interface{}{"title": content.Title, "path": content.FullPath})
	a.jsonResponse(w, http.StatusCreated, content)
}

func (a *APIHandler) APIUpdateContent(w http.ResponseWriter, r *http.Request) {
	if !a.requirePermission(w, r, auth.PermContentEdit) {
		return
	}

	id, err := primitive.ObjectIDFromHex(mux.Vars(r)["id"])
	if err != nil {
		a.jsonError(w, http.StatusBadRequest, "invalid content ID")
		return
	}

	// Get existing content
	content, err := a.contentService.GetContent(r.Context(), id)
	if err != nil {
		a.jsonError(w, http.StatusNotFound, "content not found")
		return
	}

	// A sandboxed caller edits copies in its own forks, never live pages.
	if !a.callerMayWriteContent(w, r, content) {
		return
	}

	// Decode partial update from request body
	var raw map[string]json.RawMessage
	if err := a.decodeJSON(r, &raw); err != nil {
		a.jsonError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// Apply fields that are present in the request
	if v, ok := raw["title"]; ok {
		json.Unmarshal(v, &content.Title)
	}
	if v, ok := raw["slug"]; ok {
		json.Unmarshal(v, &content.Slug)
	}
	if v, ok := raw["template_id"]; ok {
		var tidStr string
		json.Unmarshal(v, &tidStr)
		if tid, err := primitive.ObjectIDFromHex(tidStr); err == nil {
			content.TemplateID = tid
			if tmpl, err := a.templateService.GetTemplate(r.Context(), tid); err == nil {
				content.TemplateName = tmpl.Name
			}
		}
	}
	if v, ok := raw["folder_path"]; ok {
		json.Unmarshal(v, &content.FolderPath)
		// Resolve folder ID
		content.FolderID = nil
		if content.FolderPath != "" {
			folders, _ := a.settingsService.ListFolders(r.Context())
			for _, f := range folders {
				if f.Path == content.FolderPath {
					content.FolderID = &f.ID
					break
				}
			}
		}
	}
	if v, ok := raw["category"]; ok {
		json.Unmarshal(v, &content.Category)
	}
	if v, ok := raw["tags"]; ok {
		json.Unmarshal(v, &content.Tags)
	}
	if v, ok := raw["meta_description"]; ok {
		json.Unmarshal(v, &content.MetaDescription)
	}
	if v, ok := raw["og_image"]; ok {
		json.Unmarshal(v, &content.OGImage)
	}
	if v, ok := raw["noindex"]; ok {
		json.Unmarshal(v, &content.NoIndex)
	}
	if v, ok := raw["hold"]; ok {
		json.Unmarshal(v, &content.Hold)
	}
	if v, ok := raw["author_name"]; ok {
		json.Unmarshal(v, &content.AuthorName)
	}
	if v, ok := raw["author_url"]; ok {
		json.Unmarshal(v, &content.AuthorURL)
	}
	if v, ok := raw["data"]; ok {
		json.Unmarshal(v, &content.Data)
	}
	if v, ok := raw["clear_fields"]; ok {
		var clearFields []string
		if err := json.Unmarshal(v, &clearFields); err == nil {
			if content.Data == nil {
				content.Data = make(map[string]interface{})
			}
			for _, field := range clearFields {
				content.Data[field] = ""
			}
		}
	}
	if !a.applyPublishedField(w, r, content, raw) {
		return
	}
	if v, ok := raw["use_header"]; ok {
		json.Unmarshal(v, &content.UseHeader)
	}
	if v, ok := raw["use_footer"]; ok {
		json.Unmarshal(v, &content.UseFooter)
	}
	if v, ok := raw["use_theme"]; ok {
		json.Unmarshal(v, &content.UseTheme)
	}
	if v, ok := raw["raw_mode"]; ok {
		json.Unmarshal(v, &content.RawMode)
	}

	var versionComment string
	if v, ok := raw["version_comment"]; ok {
		json.Unmarshal(v, &versionComment)
	}

	args := []string{}
	if versionComment != "" {
		args = append(args, versionComment)
	}
	if err := a.contentService.UpdateContent(r.Context(), content, args...); err != nil {
		a.jsonError(w, contentErrorStatus(err), err.Error())
		return
	}

	a.auditLog(r, "content.update", "content", content.ID.Hex(), map[string]interface{}{"title": content.Title})
	a.jsonResponse(w, http.StatusOK, content)
}

func (a *APIHandler) APIDeleteContent(w http.ResponseWriter, r *http.Request) {
	if !a.requirePermission(w, r, auth.PermContentDelete) {
		return
	}

	id, err := primitive.ObjectIDFromHex(mux.Vars(r)["id"])
	if err != nil {
		a.jsonError(w, http.StatusBadRequest, "invalid content ID")
		return
	}

	if err := a.contentService.DeleteContent(r.Context(), id); err != nil {
		a.jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}

	a.auditLog(r, "content.delete", "content", id.Hex(), nil)
	a.jsonResponse(w, http.StatusOK, map[string]interface{}{"success": true})
}

func (a *APIHandler) APIRestoreContent(w http.ResponseWriter, r *http.Request) {
	if !a.requirePermission(w, r, auth.PermContentEdit) {
		return
	}

	id, err := primitive.ObjectIDFromHex(mux.Vars(r)["id"])
	if err != nil {
		a.jsonError(w, http.StatusBadRequest, "invalid content ID")
		return
	}

	// Restoring a page that was live before it was deleted publishes it again.
	deleted, err := a.contentService.GetContent(r.Context(), id)
	if err != nil {
		a.jsonError(w, http.StatusNotFound, "content not found")
		return
	}
	if deleted.PublishedBeforeDelete && deleted.ForkID == nil && !deleted.Hold && !a.requirePermission(w, r, auth.PermContentPublish) {
		return
	}

	if err := a.contentService.RestoreContent(r.Context(), id); err != nil {
		a.jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}

	a.auditLog(r, "content.restore", "content", id.Hex(), nil)
	a.jsonResponse(w, http.StatusOK, map[string]interface{}{"success": true})
}

func (a *APIHandler) APIPublishContent(w http.ResponseWriter, r *http.Request) {
	if !a.requirePermission(w, r, auth.PermContentPublish) {
		return
	}

	id, err := primitive.ObjectIDFromHex(mux.Vars(r)["id"])
	if err != nil {
		a.jsonError(w, http.StatusBadRequest, "invalid content ID")
		return
	}

	if err := a.contentService.PublishContent(r.Context(), id); err != nil {
		a.jsonError(w, contentErrorStatus(err), err.Error())
		return
	}

	a.auditLog(r, "content.publish", "content", id.Hex(), nil)
	a.jsonResponse(w, http.StatusOK, map[string]interface{}{"success": true})
}

func (a *APIHandler) APIUnpublishContent(w http.ResponseWriter, r *http.Request) {
	if !a.requirePermission(w, r, auth.PermContentPublish) {
		return
	}

	id, err := primitive.ObjectIDFromHex(mux.Vars(r)["id"])
	if err != nil {
		a.jsonError(w, http.StatusBadRequest, "invalid content ID")
		return
	}

	if err := a.contentService.UnpublishContent(r.Context(), id); err != nil {
		a.jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}

	a.auditLog(r, "content.unpublish", "content", id.Hex(), nil)
	a.jsonResponse(w, http.StatusOK, map[string]interface{}{"success": true})
}

func (a *APIHandler) APIListContentVersions(w http.ResponseWriter, r *http.Request) {
	if !a.requirePermission(w, r, auth.PermContentView) {
		return
	}
	id, err := primitive.ObjectIDFromHex(mux.Vars(r)["id"])
	if err != nil {
		a.jsonError(w, http.StatusBadRequest, "invalid content ID")
		return
	}

	versions, err := a.contentService.GetVersions(r.Context(), id)
	if err != nil {
		a.jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}

	a.jsonResponse(w, http.StatusOK, versions)
}

func (a *APIHandler) APIGetContentVersion(w http.ResponseWriter, r *http.Request) {
	if !a.requirePermission(w, r, auth.PermContentView) {
		return
	}
	id, err := primitive.ObjectIDFromHex(mux.Vars(r)["id"])
	if err != nil {
		a.jsonError(w, http.StatusBadRequest, "invalid content ID")
		return
	}

	version, err := strconv.Atoi(mux.Vars(r)["version"])
	if err != nil {
		a.jsonError(w, http.StatusBadRequest, "invalid version number")
		return
	}

	v, err := a.contentService.GetVersion(r.Context(), id, version)
	if err != nil {
		a.jsonError(w, http.StatusNotFound, "version not found")
		return
	}

	a.jsonResponse(w, http.StatusOK, v)
}

func (a *APIHandler) APIRevertContentVersion(w http.ResponseWriter, r *http.Request) {
	if !a.requirePermission(w, r, auth.PermContentEdit) {
		return
	}

	id, err := primitive.ObjectIDFromHex(mux.Vars(r)["id"])
	if err != nil {
		a.jsonError(w, http.StatusBadRequest, "invalid content ID")
		return
	}

	version, err := strconv.Atoi(mux.Vars(r)["version"])
	if err != nil {
		a.jsonError(w, http.StatusBadRequest, "invalid version number")
		return
	}

	var req struct {
		VersionComment string `json:"version_comment"`
	}
	a.decodeJSON(r, &req)

	comment := req.VersionComment
	if comment == "" {
		comment = fmt.Sprintf("Reverted to version %d", version)
	}

	// A revert restores the version's published state too: changing what
	// is live needs content.publish, as publishing does.
	current, err := a.contentService.GetContent(r.Context(), id)
	if err != nil {
		a.jsonError(w, http.StatusNotFound, "content not found")
		return
	}
	target, err := a.contentService.GetVersion(r.Context(), id, version)
	if err != nil {
		a.jsonError(w, http.StatusNotFound, "version not found")
		return
	}
	if target.Published != current.Published && current.ForkID == nil && !a.requirePermission(w, r, auth.PermContentPublish) {
		return
	}

	if err := a.contentService.RevertToVersion(r.Context(), id, version, comment); err != nil {
		a.jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}

	a.auditLog(r, "content.revert", "content", id.Hex(), map[string]interface{}{"version": version})
	a.jsonResponse(w, http.StatusOK, map[string]interface{}{"success": true})
}

// APISearchContent handles content search
func (a *APIHandler) APISearchContent(w http.ResponseWriter, r *http.Request) {
	if !a.requirePermission(w, r, auth.PermContentView) {
		return
	}
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	if query == "" {
		a.jsonError(w, http.StatusBadRequest, "q parameter is required")
		return
	}
	if len(query) > 200 {
		query = query[:200]
	}

	searchType := r.URL.Query().Get("type")
	if searchType == "" {
		searchType = "fulltext"
	}
	includeDeleted, _ := strconv.ParseBool(r.URL.Query().Get("include_deleted"))
	includeForks, _ := strconv.ParseBool(r.URL.Query().Get("include_forks"))

	// Use service to list all content, then filter in-memory (matching MCP behavior)
	contents, err := a.contentService.ListContent(r.Context(), includeDeleted, "", nil, includeForks)
	if err != nil {
		a.jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}

	type SearchMatch struct {
		ID           string   `json:"id"`
		Title        string   `json:"title"`
		FullPath     string   `json:"full_path"`
		TemplateName string   `json:"template_name"`
		Published    bool     `json:"published"`
		ForkID       string   `json:"fork_id,omitempty"` // set on fork copies (include_forks only)
		MatchedIn    []string `json:"matched_in"`
	}

	queryLower := strings.ToLower(query)
	var matches []SearchMatch

	for _, c := range contents {
		var matchedIn []string

		if strings.Contains(strings.ToLower(c.Title), queryLower) {
			matchedIn = append(matchedIn, "title")
		}

		if searchType == "fulltext" {
			for fieldName, v := range c.Data {
				if strVal, ok := v.(string); ok {
					if strings.Contains(strings.ToLower(strVal), queryLower) {
						matchedIn = append(matchedIn, fieldName)
					}
				}
			}
		}

		if len(matchedIn) > 0 {
			matches = append(matches, SearchMatch{
				ID:           c.ID.Hex(),
				Title:        c.Title,
				FullPath:     c.FullPath,
				TemplateName: c.TemplateName,
				Published:    c.Published,
				ForkID:       forkIDHex(c.ForkID),
				MatchedIn:    matchedIn,
			})
		}
	}

	a.jsonResponse(w, http.StatusOK, map[string]interface{}{
		"query":       query,
		"search_type": searchType,
		"total":       len(matches),
		"matches":     matches,
	})
}

// ─── Search/replace helper ─────────────────────────────────────────────────────

// searchReplaceHelper holds compiled search pattern for reuse.
type searchReplaceHelper struct {
	search  string
	replace string
	isRegex bool
	re      *regexp.Regexp
}

const (
	maxSearchReplaceTextLen = 100_000 // 100K max for search or replace text
	maxRegexPatternLen      = 1_000   // regex patterns are expensive to compile and match
	maxSearchReplacePairs   = 100     // match the bulk-create limit
)

func newSearchReplaceHelper(search, replace string, isRegex bool) (*searchReplaceHelper, error) {
	if len(search) > maxSearchReplaceTextLen {
		return nil, fmt.Errorf("search text exceeds maximum length of %d characters", maxSearchReplaceTextLen)
	}
	if len(replace) > maxSearchReplaceTextLen {
		return nil, fmt.Errorf("replace text exceeds maximum length of %d characters", maxSearchReplaceTextLen)
	}
	h := &searchReplaceHelper{search: search, replace: replace, isRegex: isRegex}
	if isRegex {
		if len(search) > maxRegexPatternLen {
			return nil, fmt.Errorf("regex pattern exceeds maximum length of %d characters", maxRegexPatternLen)
		}
		re, err := regexp.Compile(search)
		if err != nil {
			return nil, fmt.Errorf("invalid regex: %w", err)
		}
		h.re = re
	}
	return h, nil
}

func (h *searchReplaceHelper) count(s string) int {
	if h.isRegex {
		return len(h.re.FindAllString(s, -1))
	}
	return strings.Count(s, h.search)
}

func (h *searchReplaceHelper) replaceIn(s string) string {
	if h.isRegex {
		return h.re.ReplaceAllString(s, h.replace)
	}
	return strings.ReplaceAll(s, h.search, h.replace)
}

func (h *searchReplaceHelper) contains(s string) bool {
	if h.isRegex {
		return h.re.MatchString(s)
	}
	return strings.Contains(s, h.search)
}

// srPair is used for multi-pair search/replace requests.
type srPair struct {
	Search  string `json:"search"`
	Replace string `json:"replace"`
	Regex   bool   `json:"regex"`
}

// parseSRHelpers builds search/replace helpers from either single-pair or multi-pair request.
func parseSRHelpers(search, replace string, regex bool, pairs []srPair) ([]srPair, []*searchReplaceHelper, error) {
	if len(pairs) > maxSearchReplacePairs {
		return nil, nil, fmt.Errorf("maximum %d pairs per request", maxSearchReplacePairs)
	}
	if len(pairs) > 0 {
		helpers := make([]*searchReplaceHelper, len(pairs))
		for i, p := range pairs {
			h, err := newSearchReplaceHelper(p.Search, p.Replace, p.Regex)
			if err != nil {
				return nil, nil, fmt.Errorf("pair %d: %w", i, err)
			}
			helpers[i] = h
		}
		return pairs, helpers, nil
	}
	if search == "" {
		return nil, nil, fmt.Errorf("search text is required")
	}
	h, err := newSearchReplaceHelper(search, replace, regex)
	if err != nil {
		return nil, nil, err
	}
	return []srPair{{search, replace, regex}}, []*searchReplaceHelper{h}, nil
}

// APISearchReplacePreview previews search and replace
func (a *APIHandler) APISearchReplacePreview(w http.ResponseWriter, r *http.Request) {
	if !a.requirePermission(w, r, auth.PermSearchReplace) {
		return
	}

	var req struct {
		Search  string   `json:"search"`
		Replace string   `json:"replace"`
		Regex   bool     `json:"regex"`
		Pairs   []srPair `json:"pairs"`
	}
	if err := a.decodeJSON(r, &req); err != nil {
		a.jsonError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	pairs, helpers, err := parseSRHelpers(req.Search, req.Replace, req.Regex, req.Pairs)
	if err != nil {
		a.jsonError(w, http.StatusBadRequest, err.Error())
		return
	}

	cursor, err := a.contentService.StreamContent(r.Context(), false)
	if err != nil {
		a.jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer cursor.Close(r.Context())

	type MatchDetail struct {
		ID           string         `json:"id"`
		Title        string         `json:"title"`
		FullPath     string         `json:"full_path"`
		Published    bool           `json:"published"`
		MatchCount   int            `json:"match_count"`
		FieldMatches map[string]int `json:"field_matches"`
	}

	var matches []MatchDetail
	totalMatchCount := 0
	// Per-pair match counts
	pairCounts := make([]int, len(helpers))

	for cursor.Next(r.Context()) {
		var content models.Content
		if cursor.Decode(&content) != nil {
			continue
		}
		matchCount := 0
		fieldMatches := make(map[string]int)
		for hi, srh := range helpers {
			for fieldName, value := range content.Data {
				if strVal, ok := value.(string); ok {
					if count := srh.count(strVal); count > 0 {
						matchCount += count
						fieldMatches[fieldName] += count
						pairCounts[hi] += count
					}
				}
			}
			if srh.contains(content.Title) {
				n := srh.count(content.Title)
				matchCount += n
				fieldMatches["title"] += n
				pairCounts[hi] += n
			}
		}
		if matchCount > 0 {
			matches = append(matches, MatchDetail{
				ID:           content.ID.Hex(),
				Title:        content.Title,
				FullPath:     content.FullPath,
				Published:    content.Published,
				MatchCount:   matchCount,
				FieldMatches: fieldMatches,
			})
			totalMatchCount += matchCount
		}
	}

	resp := map[string]interface{}{
		"total_matches":  totalMatchCount,
		"affected_pages": len(matches),
		"matches":        matches,
	}
	// Include per-pair summary for multi-pair mode
	if len(pairs) > 1 {
		pairSummary := make([]map[string]interface{}, len(pairs))
		for i, p := range pairs {
			pairSummary[i] = map[string]interface{}{
				"search":  p.Search,
				"replace": p.Replace,
				"matches": pairCounts[i],
			}
		}
		resp["pairs_summary"] = pairSummary
	} else {
		resp["search"] = pairs[0].Search
		resp["replace"] = pairs[0].Replace
	}

	a.jsonResponse(w, http.StatusOK, resp)
}

// APISearchReplaceExecute executes search and replace
func (a *APIHandler) APISearchReplaceExecute(w http.ResponseWriter, r *http.Request) {
	if !a.requirePermission(w, r, auth.PermSearchReplace) {
		return
	}

	if !acquireBulkOp() {
		a.jsonError(w, http.StatusTooManyRequests, "a bulk operation is already in progress, please retry shortly")
		return
	}
	defer releaseBulkOp()

	var req struct {
		Search         string   `json:"search"`
		Replace        string   `json:"replace"`
		Regex          bool     `json:"regex"`
		Pairs          []srPair `json:"pairs"`
		VersionComment string   `json:"version_comment"`
		AutoRepublish  bool     `json:"auto_republish"`
	}
	if err := a.decodeJSON(r, &req); err != nil {
		a.jsonError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	pairs, helpers, err := parseSRHelpers(req.Search, req.Replace, req.Regex, req.Pairs)
	if err != nil {
		a.jsonError(w, http.StatusBadRequest, err.Error())
		return
	}

	versionComment := req.VersionComment
	if versionComment == "" {
		versionComment = fmt.Sprintf("Bulk search and replace: '%s' → '%s'", req.Search, req.Replace)
	}

	// Enforce a maximum duration for the entire operation.
	ctx, cancel := context.WithTimeout(r.Context(), bulkOpTimeout)
	defer cancel()

	// Stream documents one-by-one — avoids loading the full collection into memory.
	cursor, err := a.contentService.StreamContent(ctx, false)
	if err != nil {
		a.jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer cursor.Close(ctx)

	type UpdatedPage struct {
		ID            string   `json:"id"`
		Title         string   `json:"title"`
		FullPath      string   `json:"full_path"`
		MatchCount    int      `json:"match_count"`
		FieldsUpdated []string `json:"fields_updated"`
	}

	// srApplyAndUpdate applies all search/replace helpers to one content item.
	// Uses per-page locking to prevent concurrent updates to the same page.
	_ = pairs // used for response metadata
	srApplyAndUpdate := func(content models.Content) *UpdatedPage {
		if ctx.Err() != nil {
			return nil
		}
		unlock := lockContent(content.ID.Hex())
		defer unlock()
		needsUpdate := false
		matchCount := 0
		fieldsUpdatedSet := make(map[string]bool)
		wasPublished := content.Published
		newData := make(map[string]interface{})
		for k, v := range content.Data {
			newData[k] = v
		}
		// Apply all helpers in order
		for _, srh := range helpers {
			for fieldName, value := range newData {
				if strVal, ok := value.(string); ok && srh.contains(strVal) {
					matchCount += srh.count(strVal)
					newData[fieldName] = srh.replaceIn(strVal)
					needsUpdate = true
					fieldsUpdatedSet[fieldName] = true
				}
			}
		}
		newTitle := content.Title
		for _, srh := range helpers {
			if srh.contains(newTitle) {
				matchCount += srh.count(newTitle)
				newTitle = srh.replaceIn(newTitle)
				needsUpdate = true
				fieldsUpdatedSet["title"] = true
			}
		}
		var fieldsUpdated []string
		for f := range fieldsUpdatedSet {
			fieldsUpdated = append(fieldsUpdated, f)
		}
		if !needsUpdate {
			return nil
		}
		content.Title = newTitle
		content.Data = newData
		if err := a.contentService.UpdateContent(ctx, &content, versionComment); err != nil {
			return nil
		}
		if req.AutoRepublish && wasPublished {
			a.contentService.PublishContent(ctx, content.ID)
		}
		return &UpdatedPage{
			ID: content.ID.Hex(), Title: newTitle,
			FullPath: content.FullPath, MatchCount: matchCount,
			FieldsUpdated: fieldsUpdated,
		}
	}

	// Process concurrently with a bounded worker pool fed by the streaming cursor.
	// Channel buffer = workers*4 provides backpressure without unbounded buffering.
	type srResult struct{ page *UpdatedPage }
	jobs := make(chan models.Content, bulkConcurrency*4)
	resultCh := make(chan srResult, bulkConcurrency*4)
	var wg sync.WaitGroup
	for range bulkConcurrency {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for c := range jobs {
				resultCh <- srResult{srApplyAndUpdate(c)}
			}
		}()
	}
	pagesScanned := 0
	go func() {
		for cursor.Next(ctx) {
			if ctx.Err() != nil {
				break
			}
			var c models.Content
			if cursor.Decode(&c) == nil {
				pagesScanned++
				select {
				case jobs <- c:
				case <-ctx.Done():
				}
			}
		}
		close(jobs)
	}()
	wg.Wait()
	close(resultCh)

	var updatedPages []UpdatedPage
	totalReplacements := 0
	for res := range resultCh {
		if res.page != nil {
			updatedPages = append(updatedPages, *res.page)
			totalReplacements += res.page.MatchCount
		}
	}

	a.auditLog(r, "content.search_replace", "content", "", map[string]interface{}{
		"pairs_count":   len(pairs),
		"pages_updated": len(updatedPages), "total_replacements": totalReplacements,
	})
	resp := map[string]interface{}{
		"success":            true,
		"pages_scanned":      pagesScanned,
		"pages_modified":     len(updatedPages),
		"total_replacements": totalReplacements,
		"pages_updated":      len(updatedPages),
		"updated_pages":      updatedPages,
	}
	if ctx.Err() != nil {
		resp["warning"] = "operation timed out; results are partial"
	}
	if len(pairs) == 1 {
		resp["search"] = pairs[0].Search
		resp["replace"] = pairs[0].Replace
	}
	a.jsonResponse(w, http.StatusOK, resp)
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

// renderContentWithWarnings renders content body HTML and returns template
// validation warnings (missing required fields, unresolved placeholders).
func (a *APIHandler) renderContentWithWarnings(r *http.Request, content *models.Content) (string, []string, error) {
	tmpl, err := a.templateService.GetTemplate(r.Context(), content.TemplateID)
	if err != nil {
		return "", nil, err
	}

	var warnings []string

	// Check required fields
	for _, field := range tmpl.Fields {
		if field.Required {
			v, ok := content.Data[field.Name]
			if !ok || fmt.Sprintf("%v", v) == "" {
				warnings = append(warnings, fmt.Sprintf("required field '%s' (%s) is empty", field.Name, field.Label))
			}
		}
	}

	// Render template body
	data := make(map[string]interface{})
	for k, v := range content.Data {
		if str, ok := v.(string); ok {
			data[k] = template.HTML(str)
		} else {
			data[k] = v
		}
	}
	data["title"] = content.Title
	data["slug"] = content.Slug

	t, err := template.New("content").Parse(tmpl.HTMLLayout)
	if err != nil {
		warnings = append(warnings, fmt.Sprintf("template parse error: %v", err))
		return "", warnings, nil
	}

	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		warnings = append(warnings, fmt.Sprintf("template execute error: %v", err))
	}

	// Check for unresolved Go template placeholders in output ({{.xxx}} not filled)
	unresolvedRe := regexp.MustCompile(`\{\{\.(\w+)\}\}`)
	for _, match := range unresolvedRe.FindAllStringSubmatch(tmpl.HTMLLayout, -1) {
		field := match[1]
		if field != "title" && field != "slug" {
			if _, exists := content.Data[field]; !exists {
				warnings = append(warnings, fmt.Sprintf("placeholder '{{.%s}}' has no matching field in content data", field))
			}
		}
	}

	// Warn about likely unclosed HTML tags
	openTagRe := regexp.MustCompile(`<(div|section|article|aside|main|header|footer|nav|ul|ol|table|tbody|thead|tr|form|select)\b`)
	closeTagRe := regexp.MustCompile(`</(div|section|article|aside|main|header|footer|nav|ul|ol|table|tbody|thead|tr|form|select)>`)
	body := buf.String()
	opens := len(openTagRe.FindAllString(body, -1))
	closes := len(closeTagRe.FindAllString(body, -1))
	if opens > closes {
		warnings = append(warnings, fmt.Sprintf("possible unclosed HTML tags: %d open vs %d close block elements", opens, closes))
	}

	return body, warnings, nil
}

// ─── Batch operations ─────────────────────────────────────────────────────────

// APIBatchPublishContent publishes multiple content items in one call.
// Body: {"ids": ["id1","id2",...]} or {"publish_all_drafts": true}
//
// Fork copies and held pages are never published here: they are returned in
// "skipped" with a reason instead of "failed". publish_all_drafts does not
// consider fork copies at all (they are not drafts of the site).
func (a *APIHandler) APIBatchPublishContent(w http.ResponseWriter, r *http.Request) {
	if !a.requirePermission(w, r, auth.PermContentPublish) {
		return
	}

	var req struct {
		IDs              []string `json:"ids"`
		PublishAllDrafts bool     `json:"publish_all_drafts"`
	}
	if err := a.decodeJSON(r, &req); err != nil {
		a.jsonError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	const (
		reasonHeld = "on hold"
		reasonFork = "fork copy; merge the fork instead"
	)
	var published []string
	skipped := []map[string]string{}
	var failed []map[string]string
	skip := func(id primitive.ObjectID, reason string) {
		skipped = append(skipped, map[string]string{"id": id.Hex(), "reason": reason})
	}

	var ids []primitive.ObjectID
	if req.PublishAllDrafts {
		// ListContent leaves fork copies out by default.
		contents, err := a.contentService.ListContent(r.Context(), false, "", nil)
		if err != nil {
			a.jsonError(w, http.StatusInternalServerError, err.Error())
			return
		}
		for _, c := range contents {
			switch {
			case c.Published:
			case c.Hold:
				skip(c.ID, reasonHeld)
			default:
				ids = append(ids, c.ID)
			}
		}
	} else {
		var requested []primitive.ObjectID
		for _, sid := range req.IDs {
			id, err := primitive.ObjectIDFromHex(sid)
			if err != nil {
				a.jsonError(w, http.StatusBadRequest, fmt.Sprintf("invalid id: %s", sid))
				return
			}
			requested = append(requested, id)
		}
		// One query to classify the requested items; unknown IDs fall through
		// to PublishContent and are reported as failed, as before.
		existing, err := a.contentService.GetContentByIDs(r.Context(), requested)
		if err != nil {
			a.jsonError(w, http.StatusInternalServerError, err.Error())
			return
		}
		for _, id := range requested {
			c := existing[id]
			switch {
			case c != nil && c.ForkID != nil:
				skip(id, reasonFork)
			case c != nil && c.Hold:
				skip(id, reasonHeld)
			default:
				ids = append(ids, id)
			}
		}
	}

	for _, id := range ids {
		err := a.contentService.PublishContent(r.Context(), id)
		switch {
		case err == nil:
			published = append(published, id.Hex())
		// Held or forked between the check above and now.
		case errors.Is(err, services.ErrContentHeld):
			skip(id, reasonHeld)
		case errors.Is(err, services.ErrPublishForkCopy):
			skip(id, reasonFork)
		default:
			failed = append(failed, map[string]string{"id": id.Hex(), "error": sanitizeAPIError(err)})
		}
	}

	a.auditLog(r, "content.batch_publish", "content", "", map[string]interface{}{"count": len(published), "skipped": len(skipped)})
	a.jsonResponse(w, http.StatusOK, map[string]interface{}{
		"published": published,
		"skipped":   skipped,
		"failed":    failed,
	})
}

// ─── Preview ──────────────────────────────────────────────────────────────────

// APIPreviewContent renders a content item's HTML without saving or publishing.
// Returns the rendered body HTML plus any validation warnings.
func (a *APIHandler) APIPreviewContent(w http.ResponseWriter, r *http.Request) {
	if !a.requirePermission(w, r, auth.PermContentView) {
		return
	}
	id, err := primitive.ObjectIDFromHex(mux.Vars(r)["id"])
	if err != nil {
		a.jsonError(w, http.StatusBadRequest, "invalid content ID")
		return
	}

	content, err := a.contentService.GetContent(r.Context(), id)
	if err != nil {
		a.jsonError(w, http.StatusNotFound, "content not found")
		return
	}

	// Allow optional field overrides in the request body (simulate edits without saving)
	var overrides map[string]json.RawMessage
	a.decodeJSON(r, &overrides) // ignore error — overrides are optional
	if v, ok := overrides["title"]; ok {
		json.Unmarshal(v, &content.Title)
	}
	if v, ok := overrides["data"]; ok {
		json.Unmarshal(v, &content.Data)
	}

	rendered, warnings, err := a.renderContentWithWarnings(r, content)
	if err != nil {
		a.jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}

	a.jsonResponse(w, http.StatusOK, map[string]interface{}{
		"content_id":    id.Hex(),
		"rendered_html": rendered,
		"warnings":      warnings,
	})
}

// ─── By-path update ───────────────────────────────────────────────────────────

// APIUpdateContentByPath updates content identified by URL path rather than ID.
// PUT /api/v1/content/by-path?path=/some/path
// The path resolves to the live page; add &fork_id= to update that fork's copy.
func (a *APIHandler) APIUpdateContentByPath(w http.ResponseWriter, r *http.Request) {
	if !a.requirePermission(w, r, auth.PermContentEdit) {
		return
	}

	path := r.URL.Query().Get("path")
	if path == "" {
		a.jsonError(w, http.StatusBadRequest, "path parameter is required")
		return
	}

	content := a.contentByPath(w, r, path)
	if content == nil {
		return
	}

	// A sandboxed caller edits copies in its own forks, never live pages.
	if !a.callerMayWriteContent(w, r, content) {
		return
	}

	var raw map[string]json.RawMessage
	if err := a.decodeJSON(r, &raw); err != nil {
		a.jsonError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if v, ok := raw["title"]; ok {
		json.Unmarshal(v, &content.Title)
	}
	if v, ok := raw["data"]; ok {
		json.Unmarshal(v, &content.Data)
	}
	if v, ok := raw["category"]; ok {
		json.Unmarshal(v, &content.Category)
	}
	if v, ok := raw["tags"]; ok {
		json.Unmarshal(v, &content.Tags)
	}
	if v, ok := raw["meta_description"]; ok {
		json.Unmarshal(v, &content.MetaDescription)
	}
	if v, ok := raw["og_image"]; ok {
		json.Unmarshal(v, &content.OGImage)
	}
	if v, ok := raw["noindex"]; ok {
		json.Unmarshal(v, &content.NoIndex)
	}
	if v, ok := raw["hold"]; ok {
		json.Unmarshal(v, &content.Hold)
	}
	if v, ok := raw["author_name"]; ok {
		json.Unmarshal(v, &content.AuthorName)
	}
	if v, ok := raw["author_url"]; ok {
		json.Unmarshal(v, &content.AuthorURL)
	}
	if !a.applyPublishedField(w, r, content, raw) {
		return
	}
	if v, ok := raw["use_header"]; ok {
		json.Unmarshal(v, &content.UseHeader)
	}
	if v, ok := raw["use_footer"]; ok {
		json.Unmarshal(v, &content.UseFooter)
	}
	if v, ok := raw["use_theme"]; ok {
		json.Unmarshal(v, &content.UseTheme)
	}
	if v, ok := raw["raw_mode"]; ok {
		json.Unmarshal(v, &content.RawMode)
	}

	var versionComment string
	if v, ok := raw["version_comment"]; ok {
		json.Unmarshal(v, &versionComment)
	}

	args := []string{}
	if versionComment != "" {
		args = append(args, versionComment)
	}
	if err := a.contentService.UpdateContent(r.Context(), content, args...); err != nil {
		a.jsonError(w, contentErrorStatus(err), err.Error())
		return
	}

	a.auditLog(r, "content.update", "content", content.ID.Hex(), map[string]interface{}{"title": content.Title, "path": content.FullPath})
	a.jsonResponse(w, http.StatusOK, content)
}

// ─── Scoped search & replace ──────────────────────────────────────────────────

// scopeFilter defines optional filters for scoped search-and-replace.
type scopeFilter struct {
	ContentIDs   []string `json:"content_ids"`
	FolderPath   string   `json:"folder_path"`
	TemplateName string   `json:"template_name"`
	Category     string   `json:"category"`
}

func (f *scopeFilter) matches(c models.Content) bool {
	if len(f.ContentIDs) > 0 {
		for _, sid := range f.ContentIDs {
			if c.ID.Hex() == sid {
				return true
			}
		}
		return false
	}
	if f.FolderPath != "" && !strings.HasPrefix(c.FullPath, f.FolderPath) {
		return false
	}
	if f.TemplateName != "" && !strings.EqualFold(c.TemplateName, f.TemplateName) {
		return false
	}
	if f.Category != "" && !strings.EqualFold(c.Category, f.Category) {
		return false
	}
	return true
}

// APIScopedSearchReplacePreview previews a scoped search-and-replace.
func (a *APIHandler) APIScopedSearchReplacePreview(w http.ResponseWriter, r *http.Request) {
	if !a.requirePermission(w, r, auth.PermSearchReplace) {
		return
	}

	var req struct {
		Search  string      `json:"search"`
		Replace string      `json:"replace"`
		Regex   bool        `json:"regex"`
		Scope   scopeFilter `json:"scope"`
	}
	if err := a.decodeJSON(r, &req); err != nil {
		a.jsonError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Search == "" {
		a.jsonError(w, http.StatusBadRequest, "search text is required")
		return
	}

	srh, err := newSearchReplaceHelper(req.Search, req.Replace, req.Regex)
	if err != nil {
		a.jsonError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Search-and-replace works on live pages only; fork copies named in
	// content_ids are reported as skipped.
	skipped, err := a.skippedForkCopies(r.Context(), req.Scope)
	if err != nil {
		a.jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Push scope filters to MongoDB, then stream the cursor document-by-document.
	cursor, err := a.contentService.StreamContentScoped(r.Context(), scopeToContentScope(req.Scope))
	if err != nil {
		a.jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer cursor.Close(r.Context())

	type MatchDetail struct {
		ID           string         `json:"id"`
		Title        string         `json:"title"`
		FullPath     string         `json:"full_path"`
		Published    bool           `json:"published"`
		MatchCount   int            `json:"match_count"`
		FieldMatches map[string]int `json:"field_matches"`
	}

	var matches []MatchDetail
	totalMatchCount := 0

	for cursor.Next(r.Context()) {
		var content models.Content
		if cursor.Decode(&content) != nil {
			continue
		}
		matchCount := 0
		fieldMatches := make(map[string]int)
		for fieldName, value := range content.Data {
			if strVal, ok := value.(string); ok {
				if count := srh.count(strVal); count > 0 {
					matchCount += count
					fieldMatches[fieldName] = count
				}
			}
		}
		if srh.contains(content.Title) {
			n := srh.count(content.Title)
			matchCount += n
			fieldMatches["title"] = n
		}
		if matchCount > 0 {
			matches = append(matches, MatchDetail{
				ID: content.ID.Hex(), Title: content.Title,
				FullPath: content.FullPath, Published: content.Published,
				MatchCount: matchCount, FieldMatches: fieldMatches,
			})
			totalMatchCount += matchCount
		}
	}

	a.jsonResponse(w, http.StatusOK, map[string]interface{}{
		"search":         req.Search,
		"replace":        req.Replace,
		"scope":          req.Scope,
		"total_matches":  totalMatchCount,
		"affected_pages": len(matches),
		"matches":        matches,
		"skipped":        skipped,
	})
}

// APIScopedSearchReplaceExecute executes a scoped search-and-replace.
func (a *APIHandler) APIScopedSearchReplaceExecute(w http.ResponseWriter, r *http.Request) {
	if !a.requirePermission(w, r, auth.PermSearchReplace) {
		return
	}

	if !acquireBulkOp() {
		a.jsonError(w, http.StatusTooManyRequests, "a bulk operation is already in progress, please retry shortly")
		return
	}
	defer releaseBulkOp()

	var req struct {
		Search         string      `json:"search"`
		Replace        string      `json:"replace"`
		Regex          bool        `json:"regex"`
		VersionComment string      `json:"version_comment"`
		AutoRepublish  bool        `json:"auto_republish"`
		Scope          scopeFilter `json:"scope"`
	}
	if err := a.decodeJSON(r, &req); err != nil {
		a.jsonError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Search == "" {
		a.jsonError(w, http.StatusBadRequest, "search text is required")
		return
	}

	srh, err := newSearchReplaceHelper(req.Search, req.Replace, req.Regex)
	if err != nil {
		a.jsonError(w, http.StatusBadRequest, err.Error())
		return
	}

	versionComment := req.VersionComment
	if versionComment == "" {
		versionComment = fmt.Sprintf("Scoped search and replace: '%s' → '%s'", req.Search, req.Replace)
	}

	ctx, cancel := context.WithTimeout(r.Context(), bulkOpTimeout)
	defer cancel()

	// Search-and-replace works on live pages only; fork copies named in
	// content_ids are reported as skipped.
	skipped, err := a.skippedForkCopies(ctx, req.Scope)
	if err != nil {
		a.jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Push scope filters to MongoDB and stream results to avoid loading the full
	// scoped set into memory before processing starts.
	cursor2, err := a.contentService.StreamContentScoped(ctx, scopeToContentScope(req.Scope))
	if err != nil {
		a.jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer cursor2.Close(ctx)

	type UpdatedPage struct {
		ID            string   `json:"id"`
		Title         string   `json:"title"`
		FullPath      string   `json:"full_path"`
		MatchCount    int      `json:"match_count"`
		FieldsUpdated []string `json:"fields_updated"`
	}

	scopedApplyAndUpdate := func(content models.Content) *UpdatedPage {
		if ctx.Err() != nil {
			return nil
		}
		needsUpdate := false
		matchCount := 0
		var fieldsUpdated []string
		wasPublished := content.Published
		newData := make(map[string]interface{})
		for k, v := range content.Data {
			newData[k] = v
		}
		for fieldName, value := range content.Data {
			if strVal, ok := value.(string); ok && srh.contains(strVal) {
				matchCount += srh.count(strVal)
				newData[fieldName] = srh.replaceIn(strVal)
				needsUpdate = true
				fieldsUpdated = append(fieldsUpdated, fieldName)
			}
		}
		newTitle := content.Title
		if srh.contains(content.Title) {
			matchCount += srh.count(content.Title)
			newTitle = srh.replaceIn(content.Title)
			needsUpdate = true
			fieldsUpdated = append(fieldsUpdated, "title")
		}
		if !needsUpdate {
			return nil
		}
		content.Title = newTitle
		content.Data = newData
		if err := a.contentService.UpdateContent(ctx, &content, versionComment); err != nil {
			return nil
		}
		if req.AutoRepublish && wasPublished {
			a.contentService.PublishContent(ctx, content.ID)
		}
		return &UpdatedPage{
			ID: content.ID.Hex(), Title: newTitle,
			FullPath: content.FullPath, MatchCount: matchCount,
			FieldsUpdated: fieldsUpdated,
		}
	}

	type scopedSRResult struct{ page *UpdatedPage }
	jobs2 := make(chan models.Content, bulkConcurrency*4)
	resultCh2 := make(chan scopedSRResult, bulkConcurrency*4)
	var wg2 sync.WaitGroup
	for range bulkConcurrency {
		wg2.Add(1)
		go func() {
			defer wg2.Done()
			for c := range jobs2 {
				resultCh2 <- scopedSRResult{scopedApplyAndUpdate(c)}
			}
		}()
	}
	scopedScanned := 0
	go func() {
		for cursor2.Next(ctx) {
			if ctx.Err() != nil {
				break
			}
			var c models.Content
			if cursor2.Decode(&c) == nil {
				scopedScanned++
				select {
				case jobs2 <- c:
				case <-ctx.Done():
				}
			}
		}
		close(jobs2)
	}()
	wg2.Wait()
	close(resultCh2)

	var updatedPages []UpdatedPage
	totalReplacements := 0
	for res := range resultCh2 {
		if res.page != nil {
			updatedPages = append(updatedPages, *res.page)
			totalReplacements += res.page.MatchCount
		}
	}

	a.auditLog(r, "content.search_replace", "content", "", map[string]interface{}{
		"search": req.Search, "replace": req.Replace,
		"pages_updated": len(updatedPages), "total_replacements": totalReplacements,
	})
	resp := map[string]interface{}{
		"success":            true,
		"search":             req.Search,
		"replace":            req.Replace,
		"pages_scanned":      scopedScanned,
		"pages_modified":     len(updatedPages),
		"total_replacements": totalReplacements,
		"pages_updated":      len(updatedPages),
		"updated_pages":      updatedPages,
		"skipped":            skipped,
	}
	if ctx.Err() != nil {
		resp["warning"] = "operation timed out; results are partial"
	}
	a.jsonResponse(w, http.StatusOK, resp)
}

// sanitizeAPIError converts internal errors to safe external messages,
// logging the full detail server-side. Prevents leaking MongoDB internals,
// field names, and collection structure to API consumers.
func sanitizeAPIError(err error) string {
	msg := err.Error()
	switch {
	case strings.Contains(msg, "not found"):
		return "not found"
	case strings.Contains(msg, "duplicate key"):
		return "duplicate content at this path"
	case strings.Contains(msg, "template"):
		return "template error"
	default:
		log.Printf("[api] internal error (sanitized): %v", err)
		return "operation failed"
	}
}

// ─── Bulk operations ──────────────────────────────────────────────────────────

const bulkConcurrency = 10

// bulkOpSem limits the number of concurrent heavy bulk operations (search/replace,
// bulk update) to prevent resource exhaustion that can hang the HTTP server.
// A buffered channel of size 2 means at most 2 heavy operations run simultaneously.
var bulkOpSem = make(chan struct{}, 2)

// acquireBulkOp tries to acquire a slot for a heavy bulk operation. Returns false
// if the semaphore is full (caller should return 429 Too Many Requests).
func acquireBulkOp() bool {
	select {
	case bulkOpSem <- struct{}{}:
		return true
	default:
		return false
	}
}

func releaseBulkOp() {
	<-bulkOpSem
}

// bulkOpTimeout is the maximum duration for a single search/replace or bulk
// update operation. Prevents runaway operations from tying up the server.
const bulkOpTimeout = 3 * time.Minute

// contentLocks provides per-page mutual exclusion for concurrent bulk operations.
// This prevents race conditions when multiple workers try to update the same page
// simultaneously (e.g., during search/replace or bulk update).
var contentLocks = struct {
	sync.Mutex
	m map[string]*sync.Mutex
}{m: make(map[string]*sync.Mutex)}

// lockContent acquires a per-page mutex. The returned function unlocks it.
func lockContent(id string) func() {
	contentLocks.Lock()
	mu, ok := contentLocks.m[id]
	if !ok {
		mu = &sync.Mutex{}
		contentLocks.m[id] = mu
	}
	contentLocks.Unlock()
	mu.Lock()
	return mu.Unlock
}

// APIBulkCreateContent creates multiple content items in a single call.
func (a *APIHandler) APIBulkCreateContent(w http.ResponseWriter, r *http.Request) {
	if !a.requirePermission(w, r, auth.PermContentCreate) {
		return
	}

	var req struct {
		Items []struct {
			TemplateID      string                 `json:"template_id"`
			Title           string                 `json:"title"`
			Slug            string                 `json:"slug"`
			FolderPath      string                 `json:"folder_path"`
			Category        string                 `json:"category"`
			Tags            []string               `json:"tags"`
			MetaDescription string                 `json:"meta_description"`
			OGImage         string                 `json:"og_image"`
			NoIndex         bool                   `json:"noindex"`
			AuthorName      string                 `json:"author_name"`
			AuthorURL       string                 `json:"author_url"`
			Data            map[string]interface{} `json:"data"`
			Published       bool                   `json:"published"`
			UseHeader       bool                   `json:"use_header"`
			UseFooter       bool                   `json:"use_footer"`
			UseTheme        bool                   `json:"use_theme"`
			RawMode         bool                   `json:"raw_mode"`
		} `json:"items"`
		VersionComment string `json:"version_comment"`
		Upsert         bool   `json:"upsert"`
	}
	if err := a.decodeJSON(r, &req); err != nil {
		a.jsonError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if len(req.Items) == 0 {
		a.jsonError(w, http.StatusBadRequest, "items array is required")
		return
	}
	if len(req.Items) > 100 {
		a.jsonError(w, http.StatusBadRequest, "maximum 100 items per batch")
		return
	}
	for _, item := range req.Items {
		if item.Published {
			if !a.requirePermission(w, r, auth.PermContentPublish) {
				return
			}
			break
		}
	}

	// Resolve templates and folders once
	templateCache := make(map[string]*models.Template)
	folders, _ := a.settingsService.ListFolders(r.Context())
	folderMap := make(map[string]*primitive.ObjectID)
	for _, f := range folders {
		id := f.ID
		folderMap[f.Path] = &id
	}

	items := make([]*models.Content, len(req.Items))
	for i, item := range req.Items {
		if item.TemplateID == "" || item.Title == "" || item.Slug == "" {
			a.jsonError(w, http.StatusBadRequest, fmt.Sprintf("item %d: template_id, title, and slug are required", i))
			return
		}

		tmpl, ok := templateCache[item.TemplateID]
		if !ok {
			tid, err := primitive.ObjectIDFromHex(item.TemplateID)
			if err != nil {
				a.jsonError(w, http.StatusBadRequest, fmt.Sprintf("item %d: invalid template_id", i))
				return
			}
			tmpl, err = a.templateService.GetTemplate(r.Context(), tid)
			if err != nil {
				a.jsonError(w, http.StatusBadRequest, fmt.Sprintf("item %d: template not found", i))
				return
			}
			templateCache[item.TemplateID] = tmpl
		}

		c := &models.Content{
			TemplateID:      tmpl.ID,
			TemplateName:    tmpl.Name,
			Title:           item.Title,
			Slug:            item.Slug,
			FolderPath:      item.FolderPath,
			FolderID:        folderMap[item.FolderPath],
			Category:        item.Category,
			Tags:            item.Tags,
			MetaDescription: item.MetaDescription,
			OGImage:         item.OGImage,
			NoIndex:         item.NoIndex,
			AuthorName:      item.AuthorName,
			AuthorURL:       item.AuthorURL,
			Data:            item.Data,
			Published:       item.Published,
			UseHeader:       item.UseHeader,
			UseFooter:       item.UseFooter,
			UseTheme:        item.UseTheme,
			RawMode:         item.RawMode,
		}
		items[i] = c
	}

	// Defense-in-depth: sanitize content data based on script policy and caller role.
	// Primary enforcement is at render/generation time, but this prevents storage of
	// script-injection vectors when the policy restricts them.
	siteConfig, _ := a.settingsService.GetSiteConfig(r.Context())
	scriptPolicy := "all"
	if siteConfig != nil && siteConfig.MarkdownScriptPolicy != "" {
		scriptPolicy = siteConfig.MarkdownScriptPolicy
	}
	apiUser := a.getAPIUser(r)
	isAdmin := apiUser != nil && apiUser.Role == "admin"
	if scriptPolicy == "none" || (scriptPolicy == "admin_only" && !isAdmin) {
		for _, c := range items {
			c.Data = services.SanitizeContentData(c.Data)
		}
	}

	// If upsert mode, use individual upserts (slower but handles duplicates)
	if req.Upsert {
		type upsertResult struct {
			Index    int    `json:"index"`
			ID       string `json:"id"`
			FullPath string `json:"full_path"`
			Action   string `json:"action"`
			Success  bool   `json:"success"`
			Error    string `json:"error,omitempty"`
		}
		results := make([]upsertResult, len(items))
		succeeded := 0
		for i, c := range items {
			created, err := a.contentService.UpsertContent(r.Context(), c, req.VersionComment)
			if err != nil {
				results[i] = upsertResult{Index: i, Success: false, Error: sanitizeAPIError(err)}
			} else {
				action := "updated"
				if created {
					action = "created"
				}
				results[i] = upsertResult{Index: i, ID: c.ID.Hex(), FullPath: c.FullPath, Action: action, Success: true}
				succeeded++
			}
		}
		a.jsonResponse(w, http.StatusOK, map[string]interface{}{
			"total":     len(items),
			"succeeded": succeeded,
			"failed":    len(items) - succeeded,
			"results":   results,
		})
		return
	}

	results := a.contentService.BulkCreateContent(r.Context(), items, req.VersionComment)

	succeeded := 0
	for _, r := range results {
		if r.Success {
			succeeded++
		}
	}

	// Collect sample of created IDs for audit trail (up to 10)
	var sampleIDs []string
	for _, res := range results {
		if res.Success && len(sampleIDs) < 10 {
			sampleIDs = append(sampleIDs, res.ID)
		}
	}
	a.auditLog(r, "content.bulk_create", "content", "", map[string]interface{}{
		"total": len(items), "succeeded": succeeded, "sample_ids": sampleIDs,
	})
	a.jsonResponse(w, http.StatusOK, map[string]interface{}{
		"total":     len(items),
		"succeeded": succeeded,
		"failed":    len(items) - succeeded,
		"results":   results,
	})
}

// APIBulkUpdateContent updates multiple content items in a single call.
// Performance: pre-fetches all content in one $in query, then processes
// updates concurrently with a pool of bulkConcurrency workers.
func (a *APIHandler) APIBulkUpdateContent(w http.ResponseWriter, r *http.Request) {
	if !a.requirePermission(w, r, auth.PermContentEdit) {
		return
	}

	type updateSpec struct {
		ID              string                 `json:"id"`
		Title           string                 `json:"title,omitempty"`
		Tags            []string               `json:"tags,omitempty"`
		Data            map[string]interface{} `json:"data,omitempty"`
		ClearFields     []string               `json:"clear_fields,omitempty"`
		MetaDescription string                 `json:"meta_description,omitempty"`
	}
	var req struct {
		Updates        []updateSpec `json:"updates"`
		VersionComment string       `json:"version_comment"`
		DryRun         bool         `json:"dry_run"`
	}
	if err := a.decodeJSON(r, &req); err != nil {
		a.jsonError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if len(req.Updates) == 0 {
		a.jsonError(w, http.StatusBadRequest, "updates array is required and must not be empty")
		return
	}
	if len(req.Updates) > 100 {
		a.jsonError(w, http.StatusBadRequest, "maximum 100 updates per call")
		return
	}

	versionComment := req.VersionComment
	if versionComment == "" {
		versionComment = "Bulk update"
	}

	type UpdateResult struct {
		ID      string `json:"id"`
		Success bool   `json:"success"`
		Error   string `json:"error,omitempty"`
	}

	// Parse all IDs up front so we can batch-fetch content in one query.
	type indexedUpdate struct {
		idx int
		id  primitive.ObjectID
		upd updateSpec
	}
	valid := make([]indexedUpdate, 0, len(req.Updates))
	results := make([]UpdateResult, len(req.Updates))
	for i, upd := range req.Updates {
		id, err := primitive.ObjectIDFromHex(upd.ID)
		if err != nil {
			results[i] = UpdateResult{ID: upd.ID, Success: false, Error: "invalid ID"}
			continue
		}
		valid = append(valid, indexedUpdate{i, id, upd})
	}

	if len(valid) == 0 {
		goto respond
	}

	// Defense-in-depth: sanitize data fields based on script policy.
	{
		siteConfig, _ := a.settingsService.GetSiteConfig(r.Context())
		scriptPolicy := "all"
		if siteConfig != nil && siteConfig.MarkdownScriptPolicy != "" {
			scriptPolicy = siteConfig.MarkdownScriptPolicy
		}
		apiUser := a.getAPIUser(r)
		isAdmin := apiUser != nil && apiUser.Role == "admin"
		if scriptPolicy == "none" || (scriptPolicy == "admin_only" && !isAdmin) {
			for i := range valid {
				if valid[i].upd.Data != nil {
					valid[i].upd.Data = services.SanitizeContentData(valid[i].upd.Data)
				}
			}
		}
	}

	if req.DryRun {
		// Batch-fetch all requested IDs in one query for dry-run existence check.
		ids := make([]primitive.ObjectID, len(valid))
		for i, v := range valid {
			ids[i] = v.id
		}
		contentMap, err := a.contentService.GetContentByIDs(r.Context(), ids)
		if err != nil {
			a.jsonError(w, http.StatusInternalServerError, err.Error())
			return
		}
		for _, v := range valid {
			if _, ok := contentMap[v.id]; ok {
				results[v.idx] = UpdateResult{ID: v.upd.ID, Success: true}
			} else {
				results[v.idx] = UpdateResult{ID: v.upd.ID, Success: false, Error: "not found"}
			}
		}
		goto respond
	}

	{
		// Batch-fetch all content in one $in query.
		ids := make([]primitive.ObjectID, len(valid))
		for i, v := range valid {
			ids[i] = v.id
		}
		contentMap, err := a.contentService.GetContentByIDs(r.Context(), ids)
		if err != nil {
			a.jsonError(w, http.StatusInternalServerError, err.Error())
			return
		}

		// Process updates concurrently with a bounded worker pool.
		type work struct {
			idx     int
			id      primitive.ObjectID
			upd     updateSpec
			content *models.Content
		}
		jobs := make(chan work, len(valid))
		var wg sync.WaitGroup
		var mu sync.Mutex

		workers := bulkConcurrency
		if len(valid) < workers {
			workers = len(valid)
		}
		for range workers {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for job := range jobs {
					unlock := lockContent(job.id.Hex())
					c := job.content
					if job.upd.Title != "" {
						c.Title = job.upd.Title
					}
					if job.upd.Tags != nil {
						c.Tags = job.upd.Tags
					}
					if job.upd.MetaDescription != "" {
						c.MetaDescription = job.upd.MetaDescription
					}
					if job.upd.Data != nil {
						if c.Data == nil {
							c.Data = make(map[string]interface{})
						}
						for k, v := range job.upd.Data {
							c.Data[k] = v
						}
					}
					for _, field := range job.upd.ClearFields {
						if c.Data == nil {
							c.Data = make(map[string]interface{})
						}
						c.Data[field] = ""
					}
					var res UpdateResult
					if err := a.contentService.UpdateContent(r.Context(), c, versionComment); err != nil {
						res = UpdateResult{ID: job.upd.ID, Success: false, Error: sanitizeAPIError(err)}
					} else {
						res = UpdateResult{ID: job.upd.ID, Success: true}
					}
					unlock()
					mu.Lock()
					results[job.idx] = res
					mu.Unlock()
				}
			}()
		}

		for _, v := range valid {
			c, ok := contentMap[v.id]
			if !ok {
				mu.Lock()
				results[v.idx] = UpdateResult{ID: v.upd.ID, Success: false, Error: "not found"}
				mu.Unlock()
				continue
			}
			jobs <- work{v.idx, v.id, v.upd, c}
		}
		close(jobs)
		wg.Wait()
	}

respond:
	succeeded, failed := 0, 0
	for _, r := range results {
		if r.Success {
			succeeded++
		} else if r.ID != "" {
			failed++
		}
	}
	a.auditLog(r, "content.bulk_update", "content", "", map[string]interface{}{
		"total": len(req.Updates), "succeeded": succeeded, "failed": failed, "dry_run": req.DryRun,
	})
	a.jsonResponse(w, http.StatusOK, map[string]interface{}{
		"dry_run":   req.DryRun,
		"total":     len(req.Updates),
		"succeeded": succeeded,
		"failed":    failed,
		"results":   results,
	})
}

// reasonForkCopyScope is the skip reason for fork copies named in a scope.
const reasonForkCopyScope = "fork copy; this operation works on live pages only"

// skippedForkCopies reports which explicitly requested content_ids are fork
// copies. Scoped operations run on live pages only, so those IDs match
// nothing; they are returned as {id, reason} so callers are told instead of
// getting a silently smaller result. Always returns a non-nil slice.
func (a *APIHandler) skippedForkCopies(ctx context.Context, f scopeFilter) ([]map[string]string, error) {
	skipped := []map[string]string{}
	ids, err := a.contentService.ForkCopyIDs(ctx, scopeToContentScope(f).ContentIDs)
	if err != nil {
		return nil, err
	}
	for _, id := range ids {
		skipped = append(skipped, map[string]string{"id": id.Hex(), "reason": reasonForkCopyScope})
	}
	return skipped, nil
}

// scopeToContentScope converts a scopeFilter (handler-layer) to services.ContentScope.
// ContentIDs strings are parsed to ObjectIDs; unparseable ones are silently dropped.
func scopeToContentScope(f scopeFilter) services.ContentScope {
	var ids []primitive.ObjectID
	for _, s := range f.ContentIDs {
		if id, err := primitive.ObjectIDFromHex(s); err == nil {
			ids = append(ids, id)
		}
	}
	return services.ContentScope{
		TemplateName: f.TemplateName,
		Category:     f.Category,
		FolderPath:   f.FolderPath,
		ContentIDs:   ids,
	}
}

// APIBulkFieldOperation applies a field operation to all matching content.
// Performance: pushes scope filters to MongoDB via ListContentScoped, then
// processes updates concurrently with a pool of bulkConcurrency workers.
func (a *APIHandler) APIBulkFieldOperation(w http.ResponseWriter, r *http.Request) {
	if !a.requirePermission(w, r, auth.PermContentEdit) {
		return
	}

	var req struct {
		Operation      string      `json:"operation"` // "clear", "set", "prepend", "append", "wrap"
		Field          string      `json:"field"`
		Value          string      `json:"value,omitempty"`
		Before         string      `json:"before,omitempty"` // for "wrap"
		After          string      `json:"after,omitempty"`  // for "wrap"
		VersionComment string      `json:"version_comment"`
		DryRun         bool        `json:"dry_run"`
		Scope          scopeFilter `json:"scope"`
	}
	if err := a.decodeJSON(r, &req); err != nil {
		a.jsonError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	validOps := map[string]bool{"clear": true, "set": true, "prepend": true, "append": true, "wrap": true}
	if !validOps[req.Operation] {
		a.jsonError(w, http.StatusBadRequest, "operation must be one of: clear, set, prepend, append, wrap")
		return
	}
	if req.Field == "" {
		a.jsonError(w, http.StatusBadRequest, "field is required")
		return
	}
	// Block system/internal field names — only template data fields are allowed.
	blockedFields := map[string]bool{
		"_id": true, "template_id": true, "template_name": true, "published": true,
		"deleted": true, "slug": true, "full_path": true, "folder_path": true,
		"folder_id": true, "created_at": true, "updated_at": true, "published_at": true,
		"fork_id": true, "category": true, "tags": true, "meta_description": true,
		"og_image": true, "content_hash": true, "use_header": true, "use_footer": true,
		"use_theme": true, "raw_mode": true, "locked_by": true, "locked_at": true,
		"noindex": true, "author_name": true, "author_url": true, "content_modified_at": true,
		"hold": true,
	}
	if blockedFields[req.Field] {
		a.jsonError(w, http.StatusBadRequest, "cannot modify system field via bulk field operation")
		return
	}

	// Push scope filters to MongoDB — avoids full-collection load.
	contents, err := a.contentService.ListContentScoped(r.Context(), scopeToContentScope(req.Scope))
	if err != nil {
		a.jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	// Fork copies named in content_ids are never touched; report them.
	skipped, err := a.skippedForkCopies(r.Context(), req.Scope)
	if err != nil {
		a.jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}

	versionComment := req.VersionComment
	if versionComment == "" {
		versionComment = fmt.Sprintf("Bulk field %s: %s", req.Operation, req.Field)
	}

	type ItemResult struct {
		ID       string `json:"id"`
		Title    string `json:"title"`
		FullPath string `json:"full_path"`
		Success  bool   `json:"success"`
		HasValue bool   `json:"has_value,omitempty"` // dry_run only: whether the field currently has a non-empty value
		Error    string `json:"error,omitempty"`
	}

	results := make([]ItemResult, len(contents))

	if req.DryRun {
		for i, c := range contents {
			existing := ""
			if v, ok := c.Data[req.Field]; ok {
				existing, _ = v.(string)
			}
			results[i] = ItemResult{
				ID: c.ID.Hex(), Title: c.Title,
				FullPath: c.FullPath, Success: true, HasValue: existing != "",
			}
		}
		goto respond
	}

	{
		type work struct {
			idx     int
			content models.Content
		}
		jobs := make(chan work, len(contents))
		var wg sync.WaitGroup
		var mu sync.Mutex

		workers := bulkConcurrency
		if len(contents) < workers {
			workers = len(contents)
		}
		for range workers {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for job := range jobs {
					unlock := lockContent(job.content.ID.Hex())
					c := job.content
					if c.Data == nil {
						c.Data = make(map[string]interface{})
					}
					existing := ""
					if v, ok := c.Data[req.Field]; ok {
						existing, _ = v.(string)
					}
					var newVal string
					switch req.Operation {
					case "clear":
						newVal = ""
					case "set":
						newVal = req.Value
					case "prepend":
						newVal = req.Value + existing
					case "append":
						newVal = existing + req.Value
					case "wrap":
						newVal = req.Before + existing + req.After
					}
					c.Data[req.Field] = newVal
					var res ItemResult
					if err := a.contentService.UpdateContent(r.Context(), &c, versionComment); err != nil {
						res = ItemResult{ID: c.ID.Hex(), Title: c.Title, FullPath: c.FullPath, Success: false, Error: sanitizeAPIError(err)}
					} else {
						res = ItemResult{ID: c.ID.Hex(), Title: c.Title, FullPath: c.FullPath, Success: true}
					}
					unlock()
					mu.Lock()
					results[job.idx] = res
					mu.Unlock()
				}
			}()
		}
		for i, c := range contents {
			jobs <- work{i, c}
		}
		close(jobs)
		wg.Wait()
	}

respond:
	succeeded, failed := 0, 0
	for _, res := range results {
		if res.Success {
			succeeded++
		} else {
			failed++
		}
	}
	a.auditLog(r, "content.bulk_field_op", "content", "", map[string]interface{}{
		"operation": req.Operation, "field": req.Field, "dry_run": req.DryRun,
		"total": len(results), "succeeded": succeeded, "failed": failed, "skipped": len(skipped),
	})
	a.jsonResponse(w, http.StatusOK, map[string]interface{}{
		"dry_run":   req.DryRun,
		"operation": req.Operation,
		"field":     req.Field,
		"total":     len(results),
		"succeeded": succeeded,
		"failed":    failed,
		"skipped":   skipped,
		"results":   results,
	})
}

// APIExportContent exports content items with their field data.
func (a *APIHandler) APIExportContent(w http.ResponseWriter, r *http.Request) {
	if !a.requirePermission(w, r, auth.PermContentEdit) {
		return
	}
	var req struct {
		TemplateName string   `json:"template_name,omitempty"`
		Category     string   `json:"category,omitempty"`
		FolderPath   string   `json:"folder_path,omitempty"`
		ContentIDs   []string `json:"content_ids,omitempty"`
		Fields       []string `json:"fields,omitempty"` // only these fields; empty = all
	}
	if r.Method == "POST" {
		if err := a.decodeJSON(r, &req); err != nil {
			a.jsonError(w, http.StatusBadRequest, "invalid request body")
			return
		}
	}

	// Push all scope filters to MongoDB via ListContentScoped to avoid loading the
	// full collection into memory when only a subset is needed.
	exportScope := scopeFilter{
		ContentIDs:   req.ContentIDs,
		FolderPath:   req.FolderPath,
		TemplateName: req.TemplateName,
		Category:     req.Category,
	}
	contents, err := a.contentService.ListContentScoped(r.Context(), scopeToContentScope(exportScope))
	if err != nil {
		a.jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	// Fork copies named in content_ids are not exported; report them.
	skipped, err := a.skippedForkCopies(r.Context(), exportScope)
	if err != nil {
		a.jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}

	fieldFilter := make(map[string]bool)
	for _, f := range req.Fields {
		fieldFilter[f] = true
	}

	type ExportItem struct {
		ID           string                 `json:"id"`
		Title        string                 `json:"title"`
		Slug         string                 `json:"slug"`
		FullPath     string                 `json:"full_path"`
		TemplateName string                 `json:"template_name"`
		Category     string                 `json:"category"`
		Tags         []string               `json:"tags,omitempty"`
		Published    bool                   `json:"published"`
		UpdatedAt    string                 `json:"updated_at"`
		Data         map[string]interface{} `json:"data"`
	}

	var items []ExportItem
	for _, c := range contents {
		data := c.Data
		if len(fieldFilter) > 0 {
			data = make(map[string]interface{})
			for k, v := range c.Data {
				if fieldFilter[k] {
					data[k] = v
				}
			}
		}
		items = append(items, ExportItem{
			ID:           c.ID.Hex(),
			Title:        c.Title,
			Slug:         c.Slug,
			FullPath:     c.FullPath,
			TemplateName: c.TemplateName,
			Category:     c.Category,
			Tags:         c.Tags,
			Published:    c.Published,
			UpdatedAt:    c.UpdatedAt.Format("2006-01-02T15:04:05Z"),
			Data:         data,
		})
	}

	if items == nil {
		items = []ExportItem{}
	}
	a.jsonResponse(w, http.StatusOK, map[string]interface{}{
		"total":   len(items),
		"items":   items,
		"skipped": skipped,
	})
}
