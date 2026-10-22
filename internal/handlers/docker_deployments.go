package handlers

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"html"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"aether_ops/internal/dockerdeploy"
	"aether_ops/internal/middleware"
	"aether_ops/internal/models"
	"aether_ops/internal/templates"
)

type DockerDeploymentHandler struct {
	scriptsDir string
	db         *pgxpool.Pool
	rdr        *templates.Renderer
	runStore   *artifactDeployRunStore
}

type artifactDeployRun struct {
	MappingID  int64
	ArtifactID int64
	ExpiresAt  time.Time
}

type artifactDeployRunStore struct {
	mu   sync.Mutex
	runs map[string]artifactDeployRun
}

func NewDockerDeploymentHandler(db *pgxpool.Pool, rdr *templates.Renderer, scriptsDir string) *DockerDeploymentHandler {
	return &DockerDeploymentHandler{scriptsDir: scriptsDir, db: db, rdr: rdr, runStore: newArtifactDeployRunStore()}
}

func (h *DockerDeploymentHandler) Register(r chi.Router) {
	r.Get("/docker-deployments", h.List)
	r.Post("/docker-deployments", h.Create)
	r.Get("/docker-deployments/{id}/edit", h.EditForm)
	r.Put("/docker-deployments/{id}", h.Update)
	r.Get("/docker-deployments/{id}/row", h.GetRow)
	r.Get("/docker-deployments/{id}/runs/latest", h.LatestRunLog)
	r.Get("/docker-deployments/{id}/run/confirm", h.RunConfirm)
	r.Post("/docker-deployments/{id}/run/prepare", h.RunPrepare)
	r.Get("/docker-deployments/{id}/run/stream", h.RunStream)
	r.Delete("/docker-deployments/{id}", h.Delete)
}

func newArtifactDeployRunStore() *artifactDeployRunStore {
	return &artifactDeployRunStore{runs: map[string]artifactDeployRun{}}
}

func (s *artifactDeployRunStore) Put(run artifactDeployRun) (string, error) {
	tokenBytes := make([]byte, 16)
	if _, err := rand.Read(tokenBytes); err != nil {
		return "", err
	}
	token := hex.EncodeToString(tokenBytes)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cleanupLocked(time.Now())
	s.runs[token] = run
	return token, nil
}

func (s *artifactDeployRunStore) Get(token string, mappingID int64) (artifactDeployRun, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cleanupLocked(time.Now())
	run, ok := s.runs[token]
	if !ok || run.MappingID != mappingID {
		return artifactDeployRun{}, false
	}
	delete(s.runs, token)
	return run, true
}

func (s *artifactDeployRunStore) cleanupLocked(now time.Time) {
	for token, run := range s.runs {
		if now.After(run.ExpiresAt) {
			delete(s.runs, token)
		}
	}
}

func (h *DockerDeploymentHandler) List(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	mappings, err := models.ListDockerDeployMappings(ctx, h.db)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	configs, err := models.ListCIConfigs(ctx, h.db)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	registries, err := models.ListArtifactRegistries(ctx, h.db)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	repositories, err := models.ListArtifactRepositories(ctx, h.db)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	h.rdr.Page(w, r, "docker_deployments_page", map[string]any{
		"Mappings":     mappings,
		"CIConfigs":    configs,
		"Registries":   registries,
		"Repositories": repositories,
		"Scripts":      h.scriptOptions(),
	})
}

func (h *DockerDeploymentHandler) Create(w http.ResponseWriter, r *http.Request) {
	input, err := parseDockerDeployMappingForm(r)
	if err != nil {
		w.WriteHeader(http.StatusUnprocessableEntity)
		w.Write([]byte(`<span class="error">` + html.EscapeString(err.Error()) + `</span>`))
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	if err := h.validateRepositorySelection(ctx, input.registryID, input.repository); err != nil {
		w.WriteHeader(http.StatusUnprocessableEntity)
		w.Write([]byte(`<span class="error">` + html.EscapeString(err.Error()) + `</span>`))
		return
	}
	input.settings, err = h.parseDeploySettings(r, models.DockerDeployMapping{}, input)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}
	mapping, err := models.CreateDockerDeployMapping(ctx, h.db, input.ciConfigID, input.registryID, input.repository, input.serviceName, input.remoteDir, input.composeFile, input.description, input.enabled, input.settings)
	if err != nil {
		w.WriteHeader(http.StatusUnprocessableEntity)
		w.Write([]byte(`<span class="error">` + html.EscapeString(err.Error()) + `</span>`))
		return
	}
	w.Header().Set("HX-Refresh", "true")
	h.rdr.Partial(w, "docker_deployment_row", mapping)
}

func (h *DockerDeploymentHandler) EditForm(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	mapping, err := models.GetDockerDeployMapping(ctx, h.db, id)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	configs, _ := models.ListCIConfigs(ctx, h.db)
	registries, _ := models.ListArtifactRegistries(ctx, h.db)
	repositories, _ := models.ListArtifactRepositories(ctx, h.db)
	h.rdr.Partial(w, "docker_deployment_edit_form", map[string]any{
		"Mapping":      mapping,
		"CIConfigs":    configs,
		"Registries":   registries,
		"Repositories": repositories,
		"Scripts":      h.scriptOptions(),
	})
}

func (h *DockerDeploymentHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
	input, err := parseDockerDeployMappingForm(r)
	if err != nil {
		h.renderDockerDeployEditError(w, r, id, err.Error())
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	if err := h.validateRepositorySelection(ctx, input.registryID, input.repository); err != nil {
		h.renderDockerDeployEditError(w, r, id, err.Error())
		return
	}
	old, err := models.GetDockerDeployMapping(ctx, h.db, id)
	if err != nil {
		http.Error(w, "发布映射不存在", http.StatusNotFound)
		return
	}
	input.settings, err = h.parseDeploySettings(r, old, input)
	if err != nil {
		h.renderDockerDeployEditError(w, r, id, err.Error())
		return
	}
	mapping, err := models.UpdateDockerDeployMapping(ctx, h.db, id, input.ciConfigID, input.registryID, input.repository, input.serviceName, input.remoteDir, input.composeFile, input.description, input.enabled, input.settings)
	if err != nil {
		h.renderDockerDeployEditError(w, r, id, err.Error())
		return
	}
	w.Header().Set("HX-Refresh", "true")
	h.rdr.Partial(w, "docker_deployment_row", mapping)
}

func (h *DockerDeploymentHandler) renderDockerDeployEditError(w http.ResponseWriter, r *http.Request, id int64, message string) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	mapping, _ := models.GetDockerDeployMapping(ctx, h.db, id)
	configs, _ := models.ListCIConfigs(ctx, h.db)
	registries, _ := models.ListArtifactRegistries(ctx, h.db)
	repositories, _ := models.ListArtifactRepositories(ctx, h.db)
	w.WriteHeader(http.StatusUnprocessableEntity)
	h.rdr.Partial(w, "docker_deployment_edit_form", map[string]any{
		"Mapping":      mapping,
		"CIConfigs":    configs,
		"Registries":   registries,
		"Repositories": repositories,
		"Scripts":      h.scriptOptions(),
		"Error":        message,
	})
}

func (h *DockerDeploymentHandler) GetRow(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	mapping, err := models.GetDockerDeployMapping(ctx, h.db, id)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	h.rdr.Partial(w, "docker_deployment_row", mapping)
}

func (h *DockerDeploymentHandler) RunConfirm(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	mapping, artifacts, err := h.prepareArtifactDeploy(ctx, id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}
	h.rdr.Partial(w, "docker_deployment_run_confirm", map[string]any{
		"Mapping":   mapping,
		"Artifacts": artifacts,
	})
}

func (h *DockerDeploymentHandler) RunPrepare(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
	artifactID, err := strconv.ParseInt(r.FormValue("artifact_id"), 10, 64)
	if err != nil || artifactID == 0 {
		http.Error(w, "产物不能为空", http.StatusUnprocessableEntity)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	mapping, err := models.GetDockerDeployMapping(ctx, h.db, id)
	if err != nil {
		http.Error(w, "发布映射不存在", http.StatusNotFound)
		return
	}
	artifact, err := models.GetArtifact(ctx, h.db, artifactID)
	if err != nil {
		http.Error(w, "产物不存在", http.StatusNotFound)
		return
	}
	if err := validateDeployArtifact(mapping, artifact); err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}

	token, err := h.runStore.Put(artifactDeployRun{MappingID: id, ArtifactID: artifactID, ExpiresAt: time.Now().Add(15 * time.Minute)})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Write([]byte(token))
}

func (h *DockerDeploymentHandler) RunStream(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
	token := strings.TrimSpace(r.URL.Query().Get("token"))
	run, ok := h.runStore.Get(token, id)
	if !ok {
		http.Error(w, "发布 token 无效或已过期", http.StatusUnprocessableEntity)
		return
	}

	mapping, err := models.GetDockerDeployMapping(r.Context(), h.db, id)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if !mapping.Enabled {
		http.Error(w, "发布映射未启用", http.StatusUnprocessableEntity)
		return
	}
	if mapping.LastDeployStatus != nil && *mapping.LastDeployStatus == "running" {
		http.Error(w, "该映射正在发布中，请稍后", http.StatusConflict)
		return
	}
	artifact, err := models.GetArtifact(r.Context(), h.db, run.ArtifactID)
	if err != nil {
		http.Error(w, "artifact not found", http.StatusNotFound)
		return
	}
	if err := validateDeployArtifact(mapping, artifact); err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}
	registry, err := models.GetArtifactRegistry(r.Context(), h.db, mapping.RegistryID)
	if err != nil {
		http.Error(w, "registry not found", http.StatusNotFound)
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
	imageRef := artifact.ImageRef()
	opLog, _ := models.CreateOperationLogWithDetail(r.Context(), h.db, mapping.ApplicationID, mapping.ProjectID, "artifact_deploy", userID, models.OperationLogDetail{
		CIConfigID:         mapping.CIConfigID,
		ReleaseBranch:      mapping.ReleaseBranch,
		ModuleName:         mapping.ModuleName,
		ModulePath:         mapping.ModulePath,
		ArtifactID:         artifact.ID,
		ArtifactImage:      imageRef,
		ComposeServiceName: mapping.ComposeServiceName,
		DeployRemoteDir:    mapping.RemoteDir,
	})
	deployRun, err := models.CreateDockerDeployRun(r.Context(), h.db, id, opLog.ID, artifact.ID, imageRef)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	lineNo := 0
	sendLog := func(message string) {
		sendSSE("log", message)
		lineNo++
		_, _ = models.CreateDockerDeployRunLog(context.Background(), h.db, deployRun.ID, lineNo, message)
	}
	finishRun := func(status, message string) {
		_, _ = models.UpdateDockerDeployRunStatus(context.Background(), h.db, deployRun.ID, status, message)
	}

	models.UpdateDockerDeployStatus(context.Background(), h.db, id, "running", "")
	ciConfig, err := models.GetCIConfig(r.Context(), h.db, mapping.CIConfigID)
	var sshConfig dockerdeploy.SSHConfig
	if err == nil {
		values := ciConfig.Variables
		if mapping.SSHMode == "independent" {
			values = models.MergeVariables(models.Variables{"DEPLOY_SSH_HOST": ""}, mapping.DeploySSH)
		}
		sshConfig, err = dockerdeploy.LoadSSHConfigForDeployment(values)
	}
	if err != nil {
		msg := err.Error()
		models.UpdateOperationLogStatus(context.Background(), h.db, opLog.ID, "failed", msg)
		models.UpdateDockerDeployStatus(context.Background(), h.db, id, "failed", msg)
		sendLog(msg)
		finishRun("failed", msg)
		sendSSE("done", "failed")
		return
	}

	deployCtx, cancel := context.WithTimeout(context.Background(), 31*time.Minute)
	defer cancel()
	logFn := func(format string, args ...any) {
		sendLog(fmt.Sprintf(format, args...))
	}
	sendLog(fmt.Sprintf("开始发布产物: %s", imageRef))
	result, err := dockerdeploy.Deploy(deployCtx, dockerdeploy.Options{
		ScriptID:           mapping.ScriptID,
		ScriptsDir:         h.scriptsDir,
		ScriptParams:       mapping.ScriptParams,
		ImageTag:           artifact.Tag,
		SSH:                sshConfig,
		RemoteDir:          mapping.RemoteDir,
		ComposeFile:        mapping.ComposeFile,
		ServiceName:        mapping.ComposeServiceName,
		ImageRef:           imageRef,
		RegistryProvider:   registry.Provider,
		RegistryHost:       mapping.RegistryHost(),
		RegistryUsername:   registry.Username,
		RegistryCredential: registry.Credential,
	}, logFn)

	status := result.Status
	message := result.Message
	if err != nil {
		status = "failed"
		message = err.Error()
	}
	models.UpdateOperationLogStatus(context.Background(), h.db, opLog.ID, status, message)
	models.UpdateDockerDeployStatus(context.Background(), h.db, id, status, message)
	if message != "" {
		sendLog(message)
	}
	finishRun(status, message)
	sendSSE("done", status)
}

func (h *DockerDeploymentHandler) LatestRunLog(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	mapping, err := models.GetDockerDeployMapping(ctx, h.db, id)
	if err != nil {
		http.Error(w, "发布映射不存在", http.StatusNotFound)
		return
	}
	run, err := models.GetLatestDockerDeployRun(ctx, h.db, id)
	if err != nil {
		if err != pgx.ErrNoRows {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		h.rdr.Partial(w, "docker_deployment_log_modal", map[string]any{
			"Mapping": mapping,
			"Error":   "暂无发布日志",
		})
		return
	}
	logs, err := models.ListDockerDeployRunLogs(ctx, h.db, run.ID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	h.rdr.Partial(w, "docker_deployment_log_modal", map[string]any{
		"Mapping": mapping,
		"Run":     run,
		"Logs":    logs,
	})
}

func (h *DockerDeploymentHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	if err := models.DeleteDockerDeployMapping(ctx, h.db, id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("HX-Refresh", "true")
	w.WriteHeader(http.StatusOK)
}

func (h *DockerDeploymentHandler) prepareArtifactDeploy(ctx context.Context, id int64) (models.DockerDeployMapping, []models.Artifact, error) {
	mapping, err := models.GetDockerDeployMapping(ctx, h.db, id)
	if err != nil {
		return models.DockerDeployMapping{}, nil, fmt.Errorf("发布映射不存在")
	}
	if !mapping.Enabled {
		return models.DockerDeployMapping{}, nil, fmt.Errorf("发布映射未启用")
	}
	artifacts, err := models.ListArtifactsByRepository(ctx, h.db, mapping.RegistryID, mapping.Repository)
	if err != nil {
		return models.DockerDeployMapping{}, nil, err
	}
	return mapping, artifacts, nil
}

type dockerDeployMappingInput struct {
	ciConfigID  int64
	registryID  int64
	repository  string
	serviceName string
	remoteDir   string
	composeFile string
	description *string
	enabled     bool
	settings    models.DockerDeploySettings
}

func parseDockerDeployMappingForm(r *http.Request) (dockerDeployMappingInput, error) {
	ciConfigID, err := strconv.ParseInt(r.FormValue("ci_config_id"), 10, 64)
	if err != nil || ciConfigID == 0 {
		return dockerDeployMappingInput{}, fmt.Errorf("CI 配置不能为空")
	}
	registryID, err := strconv.ParseInt(r.FormValue("registry_id"), 10, 64)
	if err != nil || registryID == 0 {
		return dockerDeployMappingInput{}, fmt.Errorf("镜像仓库不能为空")
	}
	repository := strings.Trim(strings.TrimSpace(r.FormValue("repository")), "/")
	if repository == "" {
		return dockerDeployMappingInput{}, fmt.Errorf("镜像 repository 不能为空")
	}
	serviceName := strings.TrimSpace(r.FormValue("compose_service_name"))
	if serviceName == "" {
		return dockerDeployMappingInput{}, fmt.Errorf("Compose service 名不能为空")
	}
	remoteDir := strings.TrimRight(strings.TrimSpace(r.FormValue("remote_dir")), "/")
	if remoteDir == "" && r.FormValue("script_id") == "" {
		return dockerDeployMappingInput{}, fmt.Errorf("远端目录不能为空")
	}
	composeFile := strings.TrimSpace(r.FormValue("compose_file"))
	if composeFile == "" {
		composeFile = "docker-compose.yml"
	}
	var desc *string
	if d := strings.TrimSpace(r.FormValue("description")); d != "" {
		desc = &d
	}
	return dockerDeployMappingInput{
		ciConfigID:  ciConfigID,
		registryID:  registryID,
		repository:  repository,
		serviceName: serviceName,
		remoteDir:   remoteDir,
		composeFile: composeFile,
		description: desc,
		enabled:     r.FormValue("enabled") == "on",
	}, nil
}

func validateDeployArtifact(mapping models.DockerDeployMapping, artifact models.Artifact) error {
	if artifact.RegistryID != mapping.RegistryID || artifact.Repository != mapping.Repository {
		return fmt.Errorf("产物与发布映射的镜像仓库不匹配")
	}
	if artifact.RemoteStatus != "present" {
		return fmt.Errorf("产物状态不是存在，不能发布")
	}
	return nil
}

func (h *DockerDeploymentHandler) validateRepositorySelection(ctx context.Context, registryID int64, repository string) error {
	exists, err := models.ArtifactRepositoryExists(ctx, h.db, registryID, repository)
	if err != nil {
		return err
	}
	if !exists {
		return fmt.Errorf("请选择已同步且存在的镜像 repository")
	}
	return nil
}
