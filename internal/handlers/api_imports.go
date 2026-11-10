package handlers

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/metavert-io/lightcms/v7/internal/auth"
	"github.com/metavert-io/lightcms/v7/internal/models"
	"github.com/metavert-io/lightcms/v7/internal/services"
	"github.com/metavert-io/lightcms/v7/internal/services/importer"

	"github.com/gorilla/mux"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// SetImportService attaches an ImportService to the APIHandler.
func (a *APIHandler) SetImportService(is *services.ImportService) {
	a.importService = is
}

// ---------------------------------------------------------------------------
// GET /api/v1/imports/sources
// ---------------------------------------------------------------------------

func (a *APIHandler) APIListImportSources(w http.ResponseWriter, r *http.Request) {
	if !a.requirePermission(w, r, auth.PermContentEdit) {
		return
	}
	if a.importService == nil {
		a.jsonError(w, http.StatusServiceUnavailable, "import service unavailable")
		return
	}
	sources, err := a.importService.ListSources(r.Context())
	if err != nil {
		a.jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if sources == nil {
		sources = []models.ImportSource{}
	}
	a.jsonResponse(w, http.StatusOK, sources)
}

// ---------------------------------------------------------------------------
// POST /api/v1/imports/sources
// ---------------------------------------------------------------------------

func (a *APIHandler) APICreateImportSource(w http.ResponseWriter, r *http.Request) {
	if !a.requirePermission(w, r, auth.PermContentEdit) {
		return
	}
	if a.importService == nil {
		a.jsonError(w, http.StatusServiceUnavailable, "import service unavailable")
		return
	}

	var body struct {
		Name         string `json:"name"`
		URL          string `json:"url"`
		TemplateName string `json:"template_name"`
		FolderPath   string `json:"folder_path"`
		AutoPublish  bool   `json:"auto_publish"`
		Schedule     string `json:"schedule"`
		Active       *bool  `json:"active"`
	}
	if err := a.decodeJSON(r, &body); err != nil {
		a.jsonError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if body.Name == "" {
		a.jsonError(w, http.StatusBadRequest, "name is required")
		return
	}
	if body.URL == "" {
		a.jsonError(w, http.StatusBadRequest, "url is required")
		return
	}
	if body.AutoPublish && !a.requirePermission(w, r, auth.PermContentPublish) {
		return
	}

	schedule := body.Schedule
	if schedule == "" {
		schedule = "daily"
	}
	switch schedule {
	case "hourly", "daily", "weekly":
		// valid
	default:
		a.jsonError(w, http.StatusBadRequest, "schedule must be one of: hourly, daily, weekly")
		return
	}
	if body.FolderPath != "" && !isValidImportFolder(body.FolderPath) {
		a.jsonError(w, http.StatusBadRequest, "invalid folder_path")
		return
	}
	active := true
	if body.Active != nil {
		active = *body.Active
	}

	src := &models.ImportSource{
		Name:         body.Name,
		URL:          body.URL,
		TemplateName: body.TemplateName,
		FolderPath:   body.FolderPath,
		AutoPublish:  body.AutoPublish,
		Schedule:     schedule,
		Active:       active,
	}

	// Resolve template ID if template_name provided
	if body.TemplateName != "" {
		if tid := a.resolveTemplateIDByName(r, body.TemplateName); !tid.IsZero() {
			src.TemplateID = tid
		}
	}

	// Set created_by from user context
	if user := a.getAPIUser(r); user != nil {
		if oid, err := primitive.ObjectIDFromHex(user.ID); err == nil {
			src.CreatedBy = oid
		}
	}

	if err := a.importService.CreateSource(r.Context(), src); err != nil {
		a.jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	a.auditLog(r, "import_source.create", "import_source", src.ID.Hex(), map[string]interface{}{"name": src.Name, "url": src.URL})
	a.jsonResponse(w, http.StatusCreated, src)
}

// ---------------------------------------------------------------------------
// PUT /api/v1/imports/sources/{id}
// ---------------------------------------------------------------------------

func (a *APIHandler) APIUpdateImportSource(w http.ResponseWriter, r *http.Request) {
	if !a.requirePermission(w, r, auth.PermContentEdit) {
		return
	}
	if a.importService == nil {
		a.jsonError(w, http.StatusServiceUnavailable, "import service unavailable")
		return
	}

	id, err := primitive.ObjectIDFromHex(mux.Vars(r)["id"])
	if err != nil {
		a.jsonError(w, http.StatusBadRequest, "invalid source id")
		return
	}

	var body struct {
		Name         *string `json:"name"`
		URL          *string `json:"url"`
		TemplateName *string `json:"template_name"`
		FolderPath   *string `json:"folder_path"`
		AutoPublish  *bool   `json:"auto_publish"`
		Schedule     *string `json:"schedule"`
		Active       *bool   `json:"active"`
	}
	if err := a.decodeJSON(r, &body); err != nil {
		a.jsonError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	updates := bson.M{}
	if body.Name != nil {
		updates["name"] = *body.Name
	}
	if body.URL != nil {
		updates["url"] = *body.URL
	}
	if body.TemplateName != nil {
		updates["template_name"] = *body.TemplateName
		if *body.TemplateName != "" {
			if tid := a.resolveTemplateIDByName(r, *body.TemplateName); !tid.IsZero() {
				updates["template_id"] = tid
			}
		}
	}
	if body.FolderPath != nil {
		if *body.FolderPath != "" && !isValidImportFolder(*body.FolderPath) {
			a.jsonError(w, http.StatusBadRequest, "invalid folder_path")
			return
		}
		updates["folder_path"] = *body.FolderPath
	}
	if body.AutoPublish != nil {
		if *body.AutoPublish && !a.requirePermission(w, r, auth.PermContentPublish) {
			return
		}
		updates["auto_publish"] = *body.AutoPublish
	}
	if body.Schedule != nil {
		switch *body.Schedule {
		case "hourly", "daily", "weekly":
			// valid
		default:
			a.jsonError(w, http.StatusBadRequest, "schedule must be one of: hourly, daily, weekly")
			return
		}
		updates["schedule"] = *body.Schedule
	}
	if body.Active != nil {
		updates["active"] = *body.Active
	}

	if err := a.importService.UpdateSource(r.Context(), id, updates); err != nil {
		a.jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}

	a.auditLog(r, "import_source.update", "import_source", id.Hex(), nil)

	src, err := a.importService.GetSource(r.Context(), id)
	if err != nil {
		a.jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	a.jsonResponse(w, http.StatusOK, src)
}

// ---------------------------------------------------------------------------
// DELETE /api/v1/imports/sources/{id}
// ---------------------------------------------------------------------------

func (a *APIHandler) APIDeleteImportSource(w http.ResponseWriter, r *http.Request) {
	if !a.requirePermission(w, r, auth.PermContentEdit) {
		return
	}
	if a.importService == nil {
		a.jsonError(w, http.StatusServiceUnavailable, "import service unavailable")
		return
	}

	id, err := primitive.ObjectIDFromHex(mux.Vars(r)["id"])
	if err != nil {
		a.jsonError(w, http.StatusBadRequest, "invalid source id")
		return
	}

	if err := a.importService.DeleteSource(r.Context(), id); err != nil {
		a.jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	a.auditLog(r, "import_source.delete", "import_source", id.Hex(), nil)
	a.jsonResponse(w, http.StatusOK, map[string]interface{}{"success": true})
}

// ---------------------------------------------------------------------------
// POST /api/v1/imports/sources/{id}/trigger
// ---------------------------------------------------------------------------

func (a *APIHandler) APITriggerImportSource(w http.ResponseWriter, r *http.Request) {
	if !a.requirePermission(w, r, auth.PermContentEdit) {
		return
	}
	if a.importService == nil {
		a.jsonError(w, http.StatusServiceUnavailable, "import service unavailable")
		return
	}

	id, err := primitive.ObjectIDFromHex(mux.Vars(r)["id"])
	if err != nil {
		a.jsonError(w, http.StatusBadRequest, "invalid source id")
		return
	}

	src, err := a.importService.GetSource(r.Context(), id)
	if err != nil {
		a.jsonError(w, http.StatusNotFound, "import source not found")
		return
	}
	if src.AutoPublish && !a.requirePermission(w, r, auth.PermContentPublish) {
		return
	}

	triggeredBy := "api"
	if user := a.getAPIUser(r); user != nil {
		triggeredBy = user.Email
	}

	job, err := a.importService.RunRSSImport(r.Context(), id, triggeredBy)
	if err != nil {
		a.jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	a.auditLog(r, "import_source.trigger", "import_source", id.Hex(), map[string]interface{}{"job_id": job.ID.Hex()})
	a.jsonResponse(w, http.StatusOK, map[string]interface{}{
		"job_id":  job.ID.Hex(),
		"message": "Import started",
	})
}

// ---------------------------------------------------------------------------
// POST /api/v1/imports/markdown
// ---------------------------------------------------------------------------

func (a *APIHandler) APIImportMarkdown(w http.ResponseWriter, r *http.Request) {
	if !a.requirePermission(w, r, auth.PermContentEdit) {
		return
	}
	if a.importService == nil {
		a.jsonError(w, http.StatusServiceUnavailable, "import service unavailable")
		return
	}

	var body struct {
		Pages []struct {
			Filename string `json:"filename"`
			Content  string `json:"content"`
		} `json:"pages"`
		DefaultTemplate string `json:"default_template"`
		DefaultFolder   string `json:"default_folder"`
		AutoPublish     bool   `json:"auto_publish"`
	}
	if err := a.decodeJSON(r, &body); err != nil {
		a.jsonError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if len(body.Pages) == 0 {
		a.jsonError(w, http.StatusBadRequest, "pages is required and must be non-empty")
		return
	}
	if body.AutoPublish && !a.requirePermission(w, r, auth.PermContentPublish) {
		return
	}

	defaultFolder := body.DefaultFolder
	if defaultFolder == "" {
		defaultFolder = "/imports"
	}
	if !isValidImportFolder(defaultFolder) {
		a.jsonError(w, http.StatusBadRequest, "invalid default_folder")
		return
	}

	pages := make([]importer.MarkdownPage, 0, len(body.Pages))
	for _, p := range body.Pages {
		filename := p.Filename
		if filename == "" {
			filename = "page.md"
		}
		pages = append(pages, importer.ParseMarkdownFile(filename, p.Content))
	}

	triggeredBy := "api"
	if user := a.getAPIUser(r); user != nil {
		triggeredBy = user.Email
	}

	job, err := a.importService.RunMarkdownImport(r.Context(), pages, body.DefaultTemplate, defaultFolder, body.AutoPublish, triggeredBy)
	if err != nil {
		a.jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	a.auditLog(r, "import.markdown", "import", job.ID.Hex(), map[string]interface{}{"pages": len(pages)})
	a.jsonResponse(w, http.StatusOK, map[string]interface{}{
		"job_id":  job.ID.Hex(),
		"message": "Markdown import started",
	})
}

// ---------------------------------------------------------------------------
// POST /api/v1/imports/csv
// ---------------------------------------------------------------------------

func (a *APIHandler) APIImportCSV(w http.ResponseWriter, r *http.Request) {
	if !a.requirePermission(w, r, auth.PermContentEdit) {
		return
	}
	if a.importService == nil {
		a.jsonError(w, http.StatusServiceUnavailable, "import service unavailable")
		return
	}

	var body struct {
		CSVData      string `json:"csv_data"`
		TitleColumn  string `json:"title_column"`
		TemplateName string `json:"template_name"`
		FolderPath   string `json:"folder_path"`
		AutoPublish  bool   `json:"auto_publish"`
		SlugColumn   string `json:"slug_column"`
	}
	if err := a.decodeJSON(r, &body); err != nil {
		a.jsonError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if body.CSVData == "" {
		a.jsonError(w, http.StatusBadRequest, "csv_data is required")
		return
	}
	if body.TitleColumn == "" {
		a.jsonError(w, http.StatusBadRequest, "title_column is required")
		return
	}
	if body.AutoPublish && !a.requirePermission(w, r, auth.PermContentPublish) {
		return
	}

	_, records, err := importer.ParseCSV(strings.NewReader(body.CSVData))
	if err != nil {
		a.jsonError(w, http.StatusBadRequest, "failed to parse CSV: "+err.Error())
		return
	}
	if len(records) == 0 {
		a.jsonError(w, http.StatusBadRequest, "CSV contains no data rows")
		return
	}

	folderPath := body.FolderPath
	if folderPath == "" {
		folderPath = "/imports"
	}
	if !isValidImportFolder(folderPath) {
		a.jsonError(w, http.StatusBadRequest, "invalid folder_path")
		return
	}

	// Resolve template
	var templateID primitive.ObjectID
	if body.TemplateName != "" {
		if tid := a.resolveTemplateIDByName(r, body.TemplateName); !tid.IsZero() {
			templateID = tid
		}
	}

	// Identity column map: all columns map to themselves
	columnMap := make(map[string]string)
	if len(records) > 0 {
		for col := range records[0].Fields {
			columnMap[col] = col
		}
	}

	triggeredBy := "api"
	if user := a.getAPIUser(r); user != nil {
		triggeredBy = user.Email
	}

	job, err := a.importService.RunCSVImport(r.Context(), records, columnMap, body.TitleColumn, body.SlugColumn, templateID, folderPath, body.AutoPublish, triggeredBy)
	if err != nil {
		a.jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	a.auditLog(r, "import.csv", "import", job.ID.Hex(), map[string]interface{}{"rows": len(records)})
	a.jsonResponse(w, http.StatusOK, map[string]interface{}{
		"job_id":  job.ID.Hex(),
		"message": "CSV import started",
	})
}

// ---------------------------------------------------------------------------
// GET /api/v1/imports/jobs
// ---------------------------------------------------------------------------

func (a *APIHandler) APIListImportJobs(w http.ResponseWriter, r *http.Request) {
	if !a.requirePermission(w, r, auth.PermContentEdit) {
		return
	}
	if a.importService == nil {
		a.jsonError(w, http.StatusServiceUnavailable, "import service unavailable")
		return
	}

	limit := 20
	if l := r.URL.Query().Get("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 {
			if n > 100 {
				n = 100
			}
			limit = n
		}
	}

	jobs, err := a.importService.ListJobs(r.Context(), limit)
	if err != nil {
		a.jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if jobs == nil {
		jobs = []models.ImportJob{}
	}
	a.jsonResponse(w, http.StatusOK, jobs)
}

// ---------------------------------------------------------------------------
// GET /api/v1/imports/jobs/{id}
// ---------------------------------------------------------------------------

func (a *APIHandler) APIGetImportJob(w http.ResponseWriter, r *http.Request) {
	if !a.requirePermission(w, r, auth.PermContentEdit) {
		return
	}
	if a.importService == nil {
		a.jsonError(w, http.StatusServiceUnavailable, "import service unavailable")
		return
	}

	id, err := primitive.ObjectIDFromHex(mux.Vars(r)["id"])
	if err != nil {
		a.jsonError(w, http.StatusBadRequest, "invalid job id")
		return
	}

	ctx := r.Context()
	job, err := a.importService.GetJob(ctx, id)
	if err != nil {
		a.jsonError(w, http.StatusNotFound, "job not found")
		return
	}

	includeLogs := true
	if v := r.URL.Query().Get("logs"); v == "false" {
		includeLogs = false
	}

	result := map[string]interface{}{"job": job}
	if includeLogs {
		logs, _ := a.importService.GetJobLogs(ctx, id, 0)
		if logs == nil {
			logs = []models.ImportLog{}
		}
		result["logs"] = logs
	}
	a.jsonResponse(w, http.StatusOK, result)
}

// ---------------------------------------------------------------------------
// POST /api/v1/imports/jobs/{id}/cancel
// ---------------------------------------------------------------------------

func (a *APIHandler) APICancelImportJob(w http.ResponseWriter, r *http.Request) {
	if !a.requirePermission(w, r, auth.PermContentEdit) {
		return
	}
	if a.importService == nil {
		a.jsonError(w, http.StatusServiceUnavailable, "import service unavailable")
		return
	}

	id, err := primitive.ObjectIDFromHex(mux.Vars(r)["id"])
	if err != nil {
		a.jsonError(w, http.StatusBadRequest, "invalid job id")
		return
	}

	ctx := r.Context()
	job, err := a.importService.GetJob(ctx, id)
	if err != nil {
		a.jsonError(w, http.StatusNotFound, "job not found")
		return
	}

	if job.Status != models.ImportStatusRunning {
		a.jsonError(w, http.StatusBadRequest, "job is not running")
		return
	}

	if err := a.importService.CancelJob(ctx, id); err != nil {
		a.jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}

	a.jsonResponse(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"message": "Job cancelled",
	})
}

// ---------------------------------------------------------------------------
// Internal helpers
// ---------------------------------------------------------------------------

// isValidImportFolder validates a folder_path for use in import operations.
// Rejects path traversal sequences, null bytes, backslashes, and double slashes.
// Must start with "/" and contain only safe characters.
func isValidImportFolder(path string) bool {
	if path == "" || path[0] != '/' {
		return false
	}
	for _, c := range []string{"..", "\x00", "\\", "//"} {
		if strings.Contains(path, c) {
			return false
		}
	}
	return true
}

// resolveTemplateIDByName looks up a template by name and returns its ObjectID.
// Returns zero ObjectID if not found.
func (a *APIHandler) resolveTemplateIDByName(r *http.Request, name string) primitive.ObjectID {
	templates, err := a.templateService.ListTemplates(r.Context())
	if err != nil {
		return primitive.NilObjectID
	}
	for _, t := range templates {
		if strings.EqualFold(t.Name, name) {
			return t.ID
		}
	}
	return primitive.NilObjectID
}
