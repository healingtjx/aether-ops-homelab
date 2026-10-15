package handlers

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"aether_ops/internal/models"
	"aether_ops/internal/templates"
)

type GlobalConfigHandler struct {
	db  *pgxpool.Pool
	rdr *templates.Renderer
}

func NewGlobalConfigHandler(db *pgxpool.Pool, rdr *templates.Renderer) *GlobalConfigHandler {
	return &GlobalConfigHandler{db: db, rdr: rdr}
}

func (h *GlobalConfigHandler) Register(r chi.Router) {
	r.Get("/global-config", h.Show)
	r.Put("/global-config/tools", h.UpdateTools)
}

func (h *GlobalConfigHandler) Show(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	cfg, err := models.GetGlobalToolConfig(ctx, h.db)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	h.rdr.Page(w, r, "global_config_page", map[string]any{"Config": cfg})
}

func (h *GlobalConfigHandler) UpdateTools(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	gitPath := strings.TrimSpace(r.FormValue("git_path"))
	mavenHomePath := strings.TrimSpace(r.FormValue("maven_home_path"))
	mavenUserSettingsFile := strings.TrimSpace(r.FormValue("maven_user_settings_file"))
	mavenLocalRepository := strings.TrimSpace(r.FormValue("maven_local_repository"))
	npmHomePath := strings.TrimSpace(r.FormValue("npm_home_path"))
	npmUserConfigFile := strings.TrimSpace(r.FormValue("npm_user_config_file"))
	npmCacheDirectory := strings.TrimSpace(r.FormValue("npm_cache_directory"))

	cfg, err := models.UpdateGlobalToolConfig(ctx, h.db, gitPath, mavenHomePath, mavenUserSettingsFile, mavenLocalRepository, npmHomePath, npmUserConfigFile, npmCacheDirectory)
	if err != nil {
		w.WriteHeader(http.StatusUnprocessableEntity)
		h.rdr.Partial(w, "global_config_tools_form", map[string]any{
			"Config": models.GlobalToolConfig{
				GitPath:               gitPath,
				MavenHomePath:         mavenHomePath,
				MavenUserSettingsFile: mavenUserSettingsFile,
				MavenLocalRepository:  mavenLocalRepository,
				NPMHomePath:           npmHomePath,
				NPMUserConfigFile:     npmUserConfigFile,
				NPMCacheDirectory:     npmCacheDirectory,
			},
			"Error": err.Error(),
		})
		return
	}
	h.rdr.Partial(w, "global_config_tools_form", map[string]any{
		"Config":  cfg,
		"Success": "已保存",
	})
}
