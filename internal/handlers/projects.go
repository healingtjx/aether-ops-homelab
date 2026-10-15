package handlers

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"aether_ops/internal/models"
	"aether_ops/internal/templates"
)

type ProjectHandler struct {
	db  *pgxpool.Pool
	rdr *templates.Renderer
}

func NewProjectHandler(db *pgxpool.Pool, rdr *templates.Renderer) *ProjectHandler {
	return &ProjectHandler{db: db, rdr: rdr}
}

func (h *ProjectHandler) Register(r chi.Router) {
	r.Get("/projects", h.List)
	r.Post("/projects", h.Create)
	r.Get("/projects/{id}/edit", h.EditForm)
	r.Put("/projects/{id}", h.Update)
	r.Get("/projects/{id}/row", h.GetRow)
	r.Delete("/projects/{id}", h.Delete)
}

func (h *ProjectHandler) List(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	projects, err := models.ListProjects(ctx, h.db)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	h.rdr.Page(w, r, "projects_page", map[string]any{"Projects": projects})
}

func (h *ProjectHandler) Create(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	name := strings.TrimSpace(r.FormValue("name"))
	if name == "" {
		w.WriteHeader(http.StatusUnprocessableEntity)
		w.Write([]byte(`<span class="error">名称不能为空</span>`))
		return
	}

	exists, err := models.ProjectNameExists(ctx, h.db, name, 0)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if exists {
		w.WriteHeader(http.StatusUnprocessableEntity)
		w.Write([]byte(`<span class="error">项目名称已存在</span>`))
		return
	}

	var desc *string
	if d := strings.TrimSpace(r.FormValue("description")); d != "" {
		desc = &d
	}

	project, err := models.CreateProject(ctx, h.db, name, desc)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	h.rdr.Partial(w, "project_row", project)
}

func (h *ProjectHandler) EditForm(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	project, err := models.GetProject(ctx, h.db, id)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	h.rdr.Partial(w, "project_edit_form", map[string]any{"Project": project})
}

func (h *ProjectHandler) Update(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	name := strings.TrimSpace(r.FormValue("name"))
	if name == "" {
		w.WriteHeader(http.StatusUnprocessableEntity)
		h.rdr.Partial(w, "project_edit_form", map[string]any{
			"Project": models.Project{ID: id, Name: name},
			"Error":   "名称不能为空",
		})
		return
	}

	exists, err := models.ProjectNameExists(ctx, h.db, name, id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if exists {
		w.WriteHeader(http.StatusUnprocessableEntity)
		h.rdr.Partial(w, "project_edit_form", map[string]any{
			"Project": models.Project{ID: id, Name: name},
			"Error":   "项目名称已存在",
		})
		return
	}

	var desc *string
	if d := strings.TrimSpace(r.FormValue("description")); d != "" {
		desc = &d
	}

	project, err := models.UpdateProject(ctx, h.db, id, name, desc)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	h.rdr.Partial(w, "project_row", project)
}

func (h *ProjectHandler) GetRow(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	project, err := models.GetProject(ctx, h.db, id)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	h.rdr.Partial(w, "project_row", project)
}

func (h *ProjectHandler) Delete(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	if err := models.DeleteProject(ctx, h.db, id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}
