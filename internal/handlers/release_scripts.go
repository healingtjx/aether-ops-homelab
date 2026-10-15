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

type ReleaseScriptHandler struct {
	db  *pgxpool.Pool
	rdr *templates.Renderer
}

func NewReleaseScriptHandler(db *pgxpool.Pool, rdr *templates.Renderer) *ReleaseScriptHandler {
	return &ReleaseScriptHandler{db: db, rdr: rdr}
}

func (h *ReleaseScriptHandler) Register(r chi.Router) {
	r.Get("/release-scripts", h.List)
	r.Post("/release-scripts", h.Create)
	r.Get("/release-scripts/{id}/edit", h.EditForm)
	r.Put("/release-scripts/{id}", h.Update)
	r.Get("/release-scripts/{id}/row", h.GetRow)
	r.Delete("/release-scripts/{id}", h.Delete)
}

func (h *ReleaseScriptHandler) List(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	scripts, err := models.ListReleaseScripts(ctx, h.db)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	h.rdr.Page(w, r, "release_scripts_page", map[string]any{"Scripts": scripts})
}

func (h *ReleaseScriptHandler) Create(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	name := strings.TrimSpace(r.FormValue("name"))
	if name == "" {
		w.WriteHeader(http.StatusUnprocessableEntity)
		w.Write([]byte(`<span class="error">名称不能为空</span>`))
		return
	}

	content := r.FormValue("content")
	if strings.TrimSpace(content) == "" {
		w.WriteHeader(http.StatusUnprocessableEntity)
		w.Write([]byte(`<span class="error">脚本内容不能为空</span>`))
		return
	}

	var desc *string
	if d := strings.TrimSpace(r.FormValue("description")); d != "" {
		desc = &d
	}

	defaultVariables, err := models.ParseVariablesFromRequest(r)
	if err != nil {
		w.WriteHeader(http.StatusUnprocessableEntity)
		w.Write([]byte(`<span class="error">` + err.Error() + `</span>`))
		return
	}

	script, err := models.CreateReleaseScript(ctx, h.db, name, desc, content, defaultVariables)
	if err != nil {
		w.WriteHeader(http.StatusUnprocessableEntity)
		w.Write([]byte(`<span class="error">` + err.Error() + `</span>`))
		return
	}
	h.rdr.Partial(w, "release_script_row", script)
}

func (h *ReleaseScriptHandler) EditForm(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	script, err := models.GetReleaseScript(ctx, h.db, id)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	h.rdr.Partial(w, "release_script_edit_form", map[string]any{"Script": script})
}

func (h *ReleaseScriptHandler) Update(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	name := strings.TrimSpace(r.FormValue("name"))
	if name == "" {
		script, _ := models.GetReleaseScript(ctx, h.db, id)
		w.WriteHeader(http.StatusUnprocessableEntity)
		h.rdr.Partial(w, "release_script_edit_form", map[string]any{
			"Script": models.ReleaseScript{ID: id, Name: name, Content: script.Content},
			"Error":  "名称不能为空",
		})
		return
	}

	content := r.FormValue("content")
	if strings.TrimSpace(content) == "" {
		script, _ := models.GetReleaseScript(ctx, h.db, id)
		w.WriteHeader(http.StatusUnprocessableEntity)
		h.rdr.Partial(w, "release_script_edit_form", map[string]any{
			"Script": models.ReleaseScript{ID: id, Name: name, Content: script.Content},
			"Error":  "脚本内容不能为空",
		})
		return
	}

	var desc *string
	if d := strings.TrimSpace(r.FormValue("description")); d != "" {
		desc = &d
	}

	defaultVariables, err := models.ParseVariablesFromRequest(r)
	if err != nil {
		w.WriteHeader(http.StatusUnprocessableEntity)
		h.rdr.Partial(w, "release_script_edit_form", map[string]any{
			"Script": models.ReleaseScript{ID: id, Name: name, Description: desc, Content: content, DefaultVariables: defaultVariables},
			"Error":  err.Error(),
		})
		return
	}

	script, err := models.UpdateReleaseScript(ctx, h.db, id, name, desc, content, defaultVariables)
	if err != nil {
		w.WriteHeader(http.StatusUnprocessableEntity)
		h.rdr.Partial(w, "release_script_edit_form", map[string]any{
			"Script": models.ReleaseScript{ID: id, Name: name, Description: desc, Content: content, DefaultVariables: defaultVariables},
			"Error":  err.Error(),
		})
		return
	}
	h.rdr.Partial(w, "release_script_row", script)
}

func (h *ReleaseScriptHandler) GetRow(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	script, err := models.GetReleaseScript(ctx, h.db, id)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	h.rdr.Partial(w, "release_script_row", script)
}

func (h *ReleaseScriptHandler) Delete(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	if err := models.DeleteReleaseScript(ctx, h.db, id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}
