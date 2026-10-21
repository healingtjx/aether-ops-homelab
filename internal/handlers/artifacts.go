package handlers

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"aether_ops/internal/models"
	"aether_ops/internal/registry"
	"aether_ops/internal/templates"
)

type ArtifactHandler struct {
	db        *pgxpool.Pool
	rdr       *templates.Renderer
	syncLocks sync.Map
}

type artifactRepositoryGroup struct {
	RegistryID     int64
	RegistryName   string
	Provider       string
	RegistryURL    string
	Namespace      string
	Repository     string
	PresentCount   int
	MissingCount   int
	DeletedCount   int
	LastSyncedAt   *time.Time
	LatestPushedAt *time.Time
	Artifacts      []models.Artifact
}

func NewArtifactHandler(db *pgxpool.Pool, rdr *templates.Renderer) *ArtifactHandler {
	return &ArtifactHandler{db: db, rdr: rdr}
}

func (h *ArtifactHandler) Register(r chi.Router) {
	r.Get("/artifacts", h.List)
	r.Post("/artifact-registries", h.CreateRegistry)
	r.Get("/artifact-registries/{id}/edit", h.EditRegistryForm)
	r.Put("/artifact-registries/{id}", h.UpdateRegistry)
	r.Post("/artifact-registries/{id}/sync", h.SyncRegistry)
	r.Delete("/artifact-registries/{id}", h.DeleteRegistry)
	r.Delete("/artifacts/{id}/remote", h.DeleteRemoteArtifact)
}

func (h *ArtifactHandler) List(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	registryID, _ := strconv.ParseInt(r.URL.Query().Get("registry_id"), 10, 64)
	status := strings.TrimSpace(r.URL.Query().Get("status"))
	if status == "" {
		status = "present"
	}

	registries, err := models.ListArtifactRegistries(ctx, h.db)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	artifacts, err := models.ListArtifacts(ctx, h.db, registryID, status)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	h.rdr.Page(w, r, "artifacts_page", map[string]any{
		"Registries":         registries,
		"Groups":             groupArtifactsByRepository(artifacts),
		"SelectedRegistryID": registryID,
		"SelectedStatus":     status,
	})
}

func groupArtifactsByRepository(artifacts []models.Artifact) []artifactRepositoryGroup {
	groups := make([]artifactRepositoryGroup, 0)
	groupIndex := map[string]int{}

	for _, artifact := range artifacts {
		key := strconv.FormatInt(artifact.RegistryID, 10) + "\n" + artifact.Repository
		idx, ok := groupIndex[key]
		if !ok {
			idx = len(groups)
			groupIndex[key] = idx
			groups = append(groups, artifactRepositoryGroup{
				RegistryID:   artifact.RegistryID,
				RegistryName: artifact.RegistryName,
				Provider:     artifact.Provider,
				RegistryURL:  artifact.RegistryURL,
				Namespace:    artifact.Namespace,
				Repository:   artifact.Repository,
				Artifacts:    []models.Artifact{},
			})
		}
		switch artifact.RemoteStatus {
		case "present":
			groups[idx].PresentCount++
		case "deleted":
			groups[idx].DeletedCount++
		}
		if artifact.LastSyncedAt != nil && (groups[idx].LastSyncedAt == nil || artifact.LastSyncedAt.After(*groups[idx].LastSyncedAt)) {
			groups[idx].LastSyncedAt = artifact.LastSyncedAt
		}
		if artifact.PushedAt != nil && (groups[idx].LatestPushedAt == nil || artifact.PushedAt.After(*groups[idx].LatestPushedAt)) {
			groups[idx].LatestPushedAt = artifact.PushedAt
		}
		groups[idx].Artifacts = append(groups[idx].Artifacts, artifact)
	}

	for i := range groups {
		sort.SliceStable(groups[i].Artifacts, func(a, b int) bool {
			left := artifactSortTime(groups[i].Artifacts[a])
			right := artifactSortTime(groups[i].Artifacts[b])
			if left.Equal(right) {
				return groups[i].Artifacts[a].Tag > groups[i].Artifacts[b].Tag
			}
			return left.After(right)
		})
	}

	return groups
}

func artifactSortTime(artifact models.Artifact) time.Time {
	if artifact.PushedAt != nil {
		return *artifact.PushedAt
	}
	if artifact.LastSyncedAt != nil {
		return *artifact.LastSyncedAt
	}
	return artifact.CreatedAt
}

func (h *ArtifactHandler) CreateRegistry(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	input, err := parseArtifactRegistryForm(r, true)
	if err != nil {
		w.WriteHeader(http.StatusUnprocessableEntity)
		w.Write([]byte(`<span class="error">` + err.Error() + `</span>`))
		return
	}

	reg, err := models.CreateArtifactRegistry(ctx, h.db, input.name, input.provider, input.registryURL, input.namespace, input.repositoryNames, input.authType, input.username, input.credential, input.enabled)
	if err != nil {
		w.WriteHeader(http.StatusUnprocessableEntity)
		w.Write([]byte(`<span class="error">` + err.Error() + `</span>`))
		return
	}
	w.Header().Set("HX-Refresh", "true")
	h.rdr.Partial(w, "artifact_registry_row", reg)
}

func (h *ArtifactHandler) EditRegistryForm(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	reg, err := models.GetArtifactRegistry(ctx, h.db, id)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	h.rdr.Partial(w, "artifact_registry_edit_form", map[string]any{"Registry": reg})
}

func (h *ArtifactHandler) UpdateRegistry(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	input, err := parseArtifactRegistryForm(r, false)
	if err != nil {
		reg, _ := models.GetArtifactRegistry(ctx, h.db, id)
		w.WriteHeader(http.StatusUnprocessableEntity)
		h.rdr.Partial(w, "artifact_registry_edit_form", map[string]any{"Registry": reg, "Error": err.Error()})
		return
	}

	reg, err := models.UpdateArtifactRegistry(ctx, h.db, id, input.name, input.provider, input.registryURL, input.namespace, input.repositoryNames, input.authType, input.username, input.credential, input.keepCredential, input.enabled)
	if err != nil {
		w.WriteHeader(http.StatusUnprocessableEntity)
		h.rdr.Partial(w, "artifact_registry_edit_form", map[string]any{"Registry": reg, "Error": err.Error()})
		return
	}
	w.Header().Set("HX-Refresh", "true")
	h.rdr.Partial(w, "artifact_registry_row", reg)
}

func (h *ArtifactHandler) SyncRegistry(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	lockValue, _ := h.syncLocks.LoadOrStore(id, &sync.Mutex{})
	lock := lockValue.(*sync.Mutex)
	if !lock.TryLock() {
		http.Error(w, "该仓库正在同步中，请稍后", http.StatusConflict)
		return
	}
	defer lock.Unlock()

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Minute)
	defer cancel()

	reg, err := models.GetArtifactRegistry(ctx, h.db, id)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if !reg.Enabled {
		http.Error(w, "仓库未启用", http.StatusUnprocessableEntity)
		return
	}

	msg, err := syncArtifactRegistryNow(ctx, h.db, reg)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	w.Write([]byte(msg))
}

func syncArtifactRegistryByID(ctx context.Context, db *pgxpool.Pool, id int64) (string, error) {
	reg, err := models.GetArtifactRegistry(ctx, db, id)
	if err != nil {
		return "", fmt.Errorf("镜像仓库不存在")
	}
	if !reg.Enabled {
		return "", fmt.Errorf("仓库未启用")
	}
	return syncArtifactRegistryNow(ctx, db, reg)
}

func syncArtifactRegistryNow(ctx context.Context, db *pgxpool.Pool, reg models.ArtifactRegistry) (string, error) {
	opLog, _ := models.CreateArtifactOperationLog(ctx, db, reg.ID, 0, "sync")
	models.UpdateArtifactRegistrySyncStatus(ctx, db, reg.ID, "running", "正在同步")
	finish := func(status string, message string) {
		if opLog.ID > 0 {
			models.UpdateArtifactOperationLogStatus(ctx, db, opLog.ID, status, message)
		}
		models.UpdateArtifactRegistrySyncStatus(ctx, db, reg.ID, status, message)
	}

	client, err := newRegistryClient(reg)
	if err != nil {
		finish("failed", err.Error())
		return "", err
	}

	var images []registry.Image
	repositoryErrors := []registry.RepositoryError{}
	if len(reg.RepositoryList()) > 0 {
		result, resultErr := client.ListRepositoryImagesResult(ctx, reg.RepositoryList())
		err = resultErr
		images = result.Images
		repositoryErrors = result.Errors
	} else {
		images, err = client.ListImages(ctx)
	}
	if err != nil {
		finish("failed", err.Error())
		return "", err
	}

	items := make([]models.ArtifactSyncItem, 0, len(images))
	for _, image := range images {
		items = append(items, models.ArtifactSyncItem{
			Repository: image.Repository,
			Tag:        image.Tag,
			Digest:     image.Digest,
			MediaType:  image.MediaType,
			SizeBytes:  image.SizeBytes,
			PushedAt:   image.PushedAt,
		})
	}
	if err := models.SyncArtifacts(ctx, db, reg.ID, items); err != nil {
		finish("failed", err.Error())
		return "", err
	}

	msg := fmt.Sprintf("同步完成，共 %d 个镜像 tag", len(items))
	if len(repositoryErrors) > 0 {
		msg = fmt.Sprintf("%s，%d 个仓库同步失败：%s", msg, len(repositoryErrors), summarizeRepositoryErrors(repositoryErrors))
	}
	finish("success", msg)
	return msg, nil
}

func summarizeRepositoryErrors(errors []registry.RepositoryError) string {
	parts := make([]string, 0, len(errors))
	for i, item := range errors {
		if i >= 5 {
			parts = append(parts, "...")
			break
		}
		parts = append(parts, item.Repository)
	}
	return strings.Join(parts, ", ")
}

func (h *ArtifactHandler) DeleteRegistry(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	if err := models.DeleteArtifactRegistry(ctx, h.db, id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("HX-Refresh", "true")
	w.WriteHeader(http.StatusOK)
}

func (h *ArtifactHandler) DeleteRemoteArtifact(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Minute)
	defer cancel()

	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	artifact, err := models.GetArtifact(ctx, h.db, id)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if artifact.RemoteStatus == "deleted" {
		http.Error(w, "产物已删除", http.StatusUnprocessableEntity)
		return
	}
	digest := ""
	if artifact.Digest != nil {
		digest = *artifact.Digest
	}

	reg, err := models.GetArtifactRegistry(ctx, h.db, artifact.RegistryID)
	if err != nil {
		http.Error(w, "registry not found", http.StatusNotFound)
		return
	}

	opLog, _ := models.CreateArtifactOperationLog(ctx, h.db, artifact.RegistryID, artifact.ID, "delete")
	client, err := newRegistryClient(reg)
	if err != nil {
		models.UpdateArtifactOperationLogStatus(ctx, h.db, opLog.ID, "failed", err.Error())
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}
	if err := client.DeleteManifest(ctx, artifact.Repository, digest, artifact.Tag); err != nil {
		models.UpdateArtifactOperationLogStatus(ctx, h.db, opLog.ID, "failed", err.Error())
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	if err := models.MarkArtifactDeleted(ctx, h.db, artifact.ID); err != nil {
		models.UpdateArtifactOperationLogStatus(ctx, h.db, opLog.ID, "failed", err.Error())
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	models.UpdateArtifactOperationLogStatus(ctx, h.db, opLog.ID, "success", "远端产物已删除")
	artifact, _ = models.GetArtifact(ctx, h.db, id)
	h.rdr.Partial(w, "artifact_row", artifact)
}

func (h *ArtifactHandler) finishArtifactOperation(ctx context.Context, logID int64, registryID int64, status string, message string) {
	if logID > 0 {
		models.UpdateArtifactOperationLogStatus(ctx, h.db, logID, status, message)
	}
	models.UpdateArtifactRegistrySyncStatus(ctx, h.db, registryID, status, message)
}

type artifactRegistryInput struct {
	name            string
	provider        string
	registryURL     string
	namespace       string
	repositoryNames string
	authType        string
	username        string
	credential      string
	keepCredential  bool
	enabled         bool
}

func parseArtifactRegistryForm(r *http.Request, requireCredential bool) (artifactRegistryInput, error) {
	if err := r.ParseForm(); err != nil {
		return artifactRegistryInput{}, err
	}
	input := artifactRegistryInput{
		name:            strings.TrimSpace(r.FormValue("name")),
		provider:        strings.TrimSpace(r.FormValue("provider")),
		registryURL:     strings.TrimSpace(r.FormValue("registry_url")),
		namespace:       strings.Trim(strings.TrimSpace(r.FormValue("namespace")), "/"),
		repositoryNames: r.FormValue("repository_names"),
		authType:        strings.TrimSpace(r.FormValue("auth_type")),
		username:        strings.TrimSpace(r.FormValue("username")),
		credential:      r.FormValue("credential"),
		enabled:         r.FormValue("enabled") == "on",
	}
	if input.name == "" {
		return input, fmt.Errorf("名称不能为空")
	}
	if input.registryURL == "" {
		return input, fmt.Errorf("仓库地址不能为空")
	}
	if input.authType == "" {
		input.authType = "basic"
	}
	input.keepCredential = !requireCredential && strings.TrimSpace(input.credential) == ""
	if requireCredential && input.provider != "aws_ecr" && strings.TrimSpace(input.credential) == "" {
		return input, fmt.Errorf("凭据不能为空")
	}
	if input.authType == "basic" && input.username == "" {
		return input, fmt.Errorf("Basic 认证用户名不能为空")
	}
	return input, nil
}

func newRegistryClient(reg models.ArtifactRegistry) (*registry.Client, error) {
	return registry.NewClient(registry.Config{
		RegistryURL: reg.RegistryURL,
		Namespace:   reg.Namespace,
		Provider:    reg.Provider,
		AuthType:    registry.AuthType(reg.AuthType),
		Username:    reg.Username,
		Credential:  reg.Credential,
	})
}
