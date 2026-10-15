package handlers

import (
	"context"
	"fmt"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"aether_ops/internal/gitops"
	"aether_ops/internal/middleware"
	"aether_ops/internal/models"
	"aether_ops/internal/templates"
)

type ApplicationHandler struct {
	db      *pgxpool.Pool
	rdr     *templates.Renderer
	workDir string
}

func NewApplicationHandler(db *pgxpool.Pool, rdr *templates.Renderer, workDir string) *ApplicationHandler {
	return &ApplicationHandler{db: db, rdr: rdr, workDir: workDir}
}

func (h *ApplicationHandler) Register(r chi.Router) {
	r.Get("/applications", h.List)
	r.Post("/applications", h.Create)
	r.Get("/applications/{id}/edit", h.EditForm)
	r.Put("/applications/{id}", h.Update)
	r.Get("/applications/{id}/row", h.GetRow)
	r.Get("/applications/{id}/pull/stream", h.PullStream)
	r.Delete("/applications/{id}", h.Delete)
}

func (h *ApplicationHandler) List(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	apps, err := models.ListApplications(ctx, h.db)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	projects, err := models.ListProjects(ctx, h.db)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	h.rdr.Page(w, r, "applications_page", map[string]any{
		"Applications": apps,
		"Projects":     projects,
	})
}

func (h *ApplicationHandler) Create(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	projectID, err := strconv.ParseInt(r.FormValue("project_id"), 10, 64)
	if err != nil || projectID == 0 {
		w.WriteHeader(http.StatusUnprocessableEntity)
		w.Write([]byte(`<span class="error">项目不能为空</span>`))
		return
	}

	name := strings.TrimSpace(r.FormValue("name"))
	if name == "" {
		w.WriteHeader(http.StatusUnprocessableEntity)
		w.Write([]byte(`<span class="error">名称不能为空</span>`))
		return
	}

	gitURL := strings.TrimSpace(r.FormValue("git_url"))
	if gitURL == "" {
		w.WriteHeader(http.StatusUnprocessableEntity)
		w.Write([]byte(`<span class="error">Git 地址不能为空</span>`))
		return
	}

	defaultBranch := strings.TrimSpace(r.FormValue("default_branch"))
	if defaultBranch == "" {
		defaultBranch = "main"
	}

	var desc *string
	if d := strings.TrimSpace(r.FormValue("description")); d != "" {
		desc = &d
	}

	app, err := models.CreateApplication(ctx, h.db, projectID, name, desc, gitURL, defaultBranch)
	if err != nil {
		w.WriteHeader(http.StatusUnprocessableEntity)
		w.Write([]byte(`<span class="error">` + err.Error() + `</span>`))
		return
	}
	h.rdr.Partial(w, "application_row", app)
}

func (h *ApplicationHandler) EditForm(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	app, err := models.GetApplication(ctx, h.db, id)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	projects, _ := models.ListProjects(ctx, h.db)

	h.rdr.Partial(w, "application_edit_form", map[string]any{
		"Application": app,
		"Projects":    projects,
	})
}

func (h *ApplicationHandler) Update(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	projectID, err := strconv.ParseInt(r.FormValue("project_id"), 10, 64)
	if err != nil || projectID == 0 {
		w.WriteHeader(http.StatusUnprocessableEntity)
		w.Write([]byte(`<span class="error">项目不能为空</span>`))
		return
	}

	name := strings.TrimSpace(r.FormValue("name"))
	if name == "" {
		w.WriteHeader(http.StatusUnprocessableEntity)
		w.Write([]byte(`<span class="error">名称不能为空</span>`))
		return
	}

	gitURL := strings.TrimSpace(r.FormValue("git_url"))
	if gitURL == "" {
		w.WriteHeader(http.StatusUnprocessableEntity)
		w.Write([]byte(`<span class="error">Git 地址不能为空</span>`))
		return
	}

	defaultBranch := strings.TrimSpace(r.FormValue("default_branch"))
	if defaultBranch == "" {
		defaultBranch = "main"
	}

	var desc *string
	if d := strings.TrimSpace(r.FormValue("description")); d != "" {
		desc = &d
	}

	app, err := models.UpdateApplication(ctx, h.db, id, projectID, name, desc, gitURL, defaultBranch)
	if err != nil {
		projects, _ := models.ListProjects(ctx, h.db)
		w.WriteHeader(http.StatusUnprocessableEntity)
		h.rdr.Partial(w, "application_edit_form", map[string]any{
			"Application": models.Application{ID: id, ProjectID: projectID, Name: name, GitURL: gitURL, DefaultBranch: defaultBranch},
			"Projects":    projects,
			"Error":       err.Error(),
		})
		return
	}
	h.rdr.Partial(w, "application_row", app)
}

func (h *ApplicationHandler) GetRow(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	app, err := models.GetApplication(ctx, h.db, id)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	h.rdr.Partial(w, "application_row", app)
}

func (h *ApplicationHandler) PullStream(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	app, err := models.GetApplication(r.Context(), h.db, id)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	sendSSE := func(event, data string) {
		fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, data)
		flusher.Flush()
	}

	var userID int64
	if u := middleware.CurrentUser(r.Context()); u != nil {
		userID = u.ID
	}

	opLog, _ := models.CreateOperationLogWithDetail(r.Context(), h.db, id, app.ProjectID, "pull", userID, models.OperationLogDetail{
		ReleaseBranch: app.DefaultBranch,
	})

	if h.workDir == "" {
		msg := "应用工作目录未配置，请在 config.yaml 中设置 work_dir.application_path"
		models.UpdateOperationLogStatus(context.Background(), h.db, opLog.ID, "failed", msg)
		models.UpdatePullStatus(context.Background(), h.db, id, "failed", msg)
		sendSSE("log", msg)
		sendSSE("done", "failed")
		return
	}

	targetDir := filepath.Join(h.workDir, app.ProjectName, app.Name)

	pullCtx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	logFn := func(format string, args ...any) {
		line := fmt.Sprintf(format, args...)
		sendSSE("log", line)
	}

	tools, err := models.GetGlobalToolConfig(r.Context(), h.db)
	if err != nil {
		models.UpdateOperationLogStatus(context.Background(), h.db, opLog.ID, "failed", err.Error())
		models.UpdatePullStatus(context.Background(), h.db, id, "failed", err.Error())
		sendSSE("log", err.Error())
		sendSSE("done", "failed")
		return
	}

	result, err := gitops.PullWithOptions(pullCtx, app.GitURL, app.DefaultBranch, targetDir, gitops.PullOptions{GitPath: tools.GitPath}, logFn)

	status := result.Status
	message := result.Message
	if err != nil {
		status = "failed"
		message = err.Error()
	}

	models.UpdateOperationLogStatus(context.Background(), h.db, opLog.ID, status, message)
	models.UpdatePullStatus(context.Background(), h.db, id, status, message)
	sendSSE("done", status)
}

func (h *ApplicationHandler) Delete(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
	if err := models.DeleteApplication(ctx, h.db, id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}
