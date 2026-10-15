package handlers

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"aether_ops/internal/gitops"
	"aether_ops/internal/middleware"
	"aether_ops/internal/models"
	"aether_ops/internal/scriptexec"
	"aether_ops/internal/templates"
)

type CIConfigHandler struct {
	db       *pgxpool.Pool
	rdr      *templates.Renderer
	workDir  string
	runStore *deployRunStore
}

type ciConfigGroup struct {
	ProjectName       string
	ApplicationName   string
	GitURL            string
	ReleaseBranch     string
	ReleaseScriptName string
	Configs           []ciConfigRow
}

type ciConfigRow struct {
	models.CIConfig
	ArtifactBuildID *int64
}

func NewCIConfigHandler(db *pgxpool.Pool, rdr *templates.Renderer, workDir string) *CIConfigHandler {
	return &CIConfigHandler{db: db, rdr: rdr, workDir: workDir, runStore: newDeployRunStore()}
}

func (h *CIConfigHandler) Register(r chi.Router) {
	r.Get("/ci-configs", h.List)
	r.Post("/ci-configs", h.Create)
	r.Get("/ci-configs/{id}/edit", h.EditForm)
	r.Put("/ci-configs/{id}", h.Update)
	r.Get("/ci-configs/{id}/row", h.GetRow)
	r.Get("/ci-configs/{id}/run/confirm", h.RunConfirm)
	r.Post("/ci-configs/{id}/run/prepare", h.RunPrepare)
	r.Get("/ci-configs/{id}/run/stream", h.RunStream)
	r.Delete("/ci-configs/{id}", h.Delete)
}

type deployRun struct {
	ConfigID  int64
	Variables models.Variables
	ExpiresAt time.Time
}

type deployRunStore struct {
	mu   sync.Mutex
	runs map[string]deployRun
}

func newDeployRunStore() *deployRunStore {
	return &deployRunStore{runs: map[string]deployRun{}}
}

func (s *deployRunStore) Put(run deployRun) (string, error) {
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

func (s *deployRunStore) Get(token string, configID int64) (deployRun, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cleanupLocked(time.Now())

	run, ok := s.runs[token]
	if !ok || run.ConfigID != configID {
		return deployRun{}, false
	}
	return run, true
}

func (s *deployRunStore) cleanupLocked(now time.Time) {
	for token, run := range s.runs {
		if now.After(run.ExpiresAt) {
			delete(s.runs, token)
		}
	}
}

func (h *CIConfigHandler) List(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	configs, err := models.ListCIConfigs(ctx, h.db)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	artifactConfigs, err := models.ListArtifactBuildConfigs(ctx, h.db)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	applications, err := models.ListApplications(ctx, h.db)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	scripts, err := models.ListReleaseScripts(ctx, h.db)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	registries, err := models.ListArtifactRegistries(ctx, h.db)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	h.rdr.Page(w, r, "ci_configs_page", map[string]any{
		"Groups":       groupCIConfigsWithArtifacts(configs, applications, artifactConfigs),
		"Applications": applications,
		"Scripts":      scripts,
		"Registries":   registries,
	})
}

func groupCIConfigs(configs []models.CIConfig, applications []models.Application) []ciConfigGroup {
	return groupCIConfigsWithArtifacts(configs, applications, nil)
}

func groupCIConfigsWithArtifacts(configs []models.CIConfig, applications []models.Application, artifactConfigs []models.CIConfig) []ciConfigGroup {
	groups := make([]ciConfigGroup, 0)
	groupIndex := map[string]int{}
	gitURLByApplicationID := map[int64]string{}
	for _, app := range applications {
		gitURLByApplicationID[app.ID] = strings.TrimSpace(app.GitURL)
	}
	artifactByModule := map[string]int64{}
	for _, config := range artifactConfigs {
		artifactByModule[ciConfigModuleKey(config.ApplicationID, config.ModuleName)] = config.ID
	}

	for _, config := range configs {
		gitURL := gitURLByApplicationID[config.ApplicationID]
		key := gitURL
		if key == "" {
			key = strconv.FormatInt(config.ApplicationID, 10)
		}
		idx, ok := groupIndex[key]
		if !ok {
			idx = len(groups)
			groupIndex[key] = idx
			groups = append(groups, ciConfigGroup{
				ProjectName:       config.ProjectName,
				ApplicationName:   config.ApplicationName,
				GitURL:            gitURL,
				ReleaseBranch:     config.ReleaseBranch,
				ReleaseScriptName: config.ReleaseScriptName,
				Configs:           []ciConfigRow{},
			})
		}
		row := ciConfigRow{CIConfig: config}
		if artifactID, ok := artifactByModule[ciConfigModuleKey(config.ApplicationID, config.ModuleName)]; ok {
			row.ArtifactBuildID = &artifactID
		}
		groups[idx].Configs = append(groups[idx].Configs, row)
	}

	return groups
}

func ciConfigModuleKey(applicationID int64, moduleName string) string {
	return strconv.FormatInt(applicationID, 10) + "\x00" + strings.TrimSpace(moduleName)
}

func (h *CIConfigHandler) Create(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	applicationID, err := strconv.ParseInt(r.FormValue("application_id"), 10, 64)
	if err != nil || applicationID == 0 {
		w.WriteHeader(http.StatusUnprocessableEntity)
		w.Write([]byte(`<span class="error">应用不能为空</span>`))
		return
	}

	releaseScriptID, err := strconv.ParseInt(r.FormValue("release_script_id"), 10, 64)
	if err != nil || releaseScriptID == 0 {
		w.WriteHeader(http.StatusUnprocessableEntity)
		w.Write([]byte(`<span class="error">发布脚本不能为空</span>`))
		return
	}

	releaseBranch := strings.TrimSpace(r.FormValue("release_branch"))
	if releaseBranch == "" {
		w.WriteHeader(http.StatusUnprocessableEntity)
		w.Write([]byte(`<span class="error">测试分支不能为空</span>`))
		return
	}

	moduleName, modulePath, err := parseModuleFields(r)
	if err != nil {
		w.WriteHeader(http.StatusUnprocessableEntity)
		w.Write([]byte(`<span class="error">` + err.Error() + `</span>`))
		return
	}

	variables, err := models.ParseVariablesFromRequest(r)
	if err != nil {
		w.WriteHeader(http.StatusUnprocessableEntity)
		w.Write([]byte(`<span class="error">` + err.Error() + `</span>`))
		return
	}

	config, err := models.CreateCIConfig(ctx, h.db, applicationID, releaseScriptID, releaseBranch, moduleName, modulePath, variables)
	if err != nil {
		w.WriteHeader(http.StatusUnprocessableEntity)
		w.Write([]byte(`<span class="error">` + err.Error() + `</span>`))
		return
	}
	w.Header().Set("HX-Refresh", "true")
	h.rdr.Partial(w, "ci_config_row", h.buildCIConfigRow(ctx, config))
}

func (h *CIConfigHandler) EditForm(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	config, err := models.GetCIConfig(ctx, h.db, id)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	scripts, _ := models.ListReleaseScripts(ctx, h.db)
	registries, _ := models.ListArtifactRegistries(ctx, h.db)
	artifactConfig, _ := h.findArtifactBuildConfig(ctx, config)

	h.rdr.Partial(w, "ci_config_edit_form", map[string]any{
		"Config":         config,
		"Scripts":        scripts,
		"Registries":     registries,
		"ArtifactConfig": artifactConfig,
	})
}

func (h *CIConfigHandler) Update(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	releaseScriptID, err := strconv.ParseInt(r.FormValue("release_script_id"), 10, 64)
	if err != nil || releaseScriptID == 0 {
		h.renderCIConfigEditError(w, ctx, id, "发布脚本不能为空")
		return
	}

	releaseBranch := strings.TrimSpace(r.FormValue("release_branch"))
	if releaseBranch == "" {
		h.renderCIConfigEditError(w, ctx, id, "测试分支不能为空")
		return
	}

	moduleName, modulePath, err := parseModuleFields(r)
	if err != nil {
		h.renderCIConfigEditError(w, ctx, id, err.Error())
		return
	}

	variables, err := models.ParseVariablesFromRequest(r)
	if err != nil {
		h.renderCIConfigEditError(w, ctx, id, err.Error())
		return
	}

	currentConfig, err := models.GetCIConfig(ctx, h.db, id)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	artifactConfig, _ := h.findArtifactBuildConfig(ctx, currentConfig)
	artifactReleaseScriptID, err := parseOptionalInt64(r.FormValue("artifact_release_script_id"))
	if err != nil {
		h.renderCIConfigEditError(w, ctx, id, "打包脚本不正确")
		return
	}
	artifactBuildMode := ""
	artifactRegistryID := int64(0)
	if artifactConfig != nil && artifactReleaseScriptID > 0 {
		artifactBuildMode, err = models.NormalizeArtifactBuildMode(r.FormValue("build_mode"))
		if err != nil {
			h.renderCIConfigEditError(w, ctx, id, err.Error())
			return
		}
		if artifactBuildMode == models.ArtifactBuildModeDocker {
			artifactRegistryID, err = parseOptionalInt64(r.FormValue("artifact_registry_id"))
			if err != nil || artifactRegistryID == 0 {
				h.renderCIConfigEditError(w, ctx, id, "镜像仓库不能为空")
				return
			}
			if _, err := models.GetArtifactRegistry(ctx, h.db, artifactRegistryID); err != nil {
				h.renderCIConfigEditError(w, ctx, id, "镜像仓库不存在")
				return
			}
		}
	}

	config, err := models.UpdateCIConfig(ctx, h.db, id, releaseScriptID, releaseBranch, moduleName, modulePath, variables)
	if err != nil {
		h.renderCIConfigEditError(w, ctx, id, err.Error())
		return
	}
	if artifactConfig != nil && artifactReleaseScriptID > 0 {
		if _, err := models.UpdateArtifactBuildSettings(ctx, h.db, artifactConfig.ID, artifactReleaseScriptID, artifactBuildMode, artifactRegistryID); err != nil {
			h.renderCIConfigEditError(w, ctx, id, err.Error())
			return
		}
	}
	w.Header().Set("HX-Refresh", "true")
	h.rdr.Partial(w, "ci_config_row", h.buildCIConfigRow(ctx, config))
}

func (h *CIConfigHandler) renderCIConfigEditError(w http.ResponseWriter, ctx context.Context, id int64, message string) {
	config, _ := models.GetCIConfig(ctx, h.db, id)
	scripts, _ := models.ListReleaseScripts(ctx, h.db)
	registries, _ := models.ListArtifactRegistries(ctx, h.db)
	artifactConfig, _ := h.findArtifactBuildConfig(ctx, config)
	w.WriteHeader(http.StatusUnprocessableEntity)
	h.rdr.Partial(w, "ci_config_edit_form", map[string]any{
		"Config":         config,
		"Scripts":        scripts,
		"Registries":     registries,
		"ArtifactConfig": artifactConfig,
		"Error":          message,
	})
}

func parseOptionalInt64(value string) (int64, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, nil
	}
	return strconv.ParseInt(value, 10, 64)
}

func parseModuleFields(r *http.Request) (string, string, error) {
	moduleName := strings.TrimSpace(r.FormValue("module_name"))
	if moduleName == "" {
		return "", "", fmt.Errorf("模块名称不能为空")
	}
	modulePath := strings.TrimSpace(r.FormValue("module_path"))
	if modulePath == "" {
		modulePath = moduleName
	}
	return moduleName, modulePath, nil
}

func (h *CIConfigHandler) RunConfirm(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	config, script, app, variables, err := h.prepareRunVariables(ctx, id, nil)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}

	h.rdr.Partial(w, "ci_config_run_confirm", map[string]any{
		"Config":    config,
		"Script":    script,
		"App":       app,
		"Variables": variables.Entries(),
	})
}

func (h *CIConfigHandler) RunPrepare(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	runtimeVariables, err := models.ParseVariablesFromRequest(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}

	_, _, _, variables, err := h.prepareRunVariables(ctx, id, runtimeVariables)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}

	token, err := h.runStore.Put(deployRun{
		ConfigID:  id,
		Variables: variables,
		ExpiresAt: time.Now().Add(15 * time.Minute),
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Write([]byte(token))
}

func (h *CIConfigHandler) GetRow(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	config, err := models.GetCIConfig(ctx, h.db, id)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	h.rdr.Partial(w, "ci_config_row", h.buildCIConfigRow(ctx, config))
}

func (h *CIConfigHandler) buildCIConfigRow(ctx context.Context, config models.CIConfig) ciConfigRow {
	row := ciConfigRow{CIConfig: config}
	artifactConfig, err := h.findArtifactBuildConfig(ctx, config)
	if err != nil || artifactConfig == nil {
		return row
	}
	id := artifactConfig.ID
	row.ArtifactBuildID = &id
	return row
}

func (h *CIConfigHandler) findArtifactBuildConfig(ctx context.Context, config models.CIConfig) (*models.CIConfig, error) {
	artifactConfigs, err := models.ListArtifactBuildConfigs(ctx, h.db)
	if err != nil {
		return nil, err
	}
	key := ciConfigModuleKey(config.ApplicationID, config.ModuleName)
	for _, artifactConfig := range artifactConfigs {
		if ciConfigModuleKey(artifactConfig.ApplicationID, artifactConfig.ModuleName) == key {
			config := artifactConfig
			return &config, nil
		}
	}
	return nil, nil
}

func (h *CIConfigHandler) RunStream(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	config, err := models.GetCIConfig(r.Context(), h.db, id)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	if config.LastRunStatus != nil && *config.LastRunStatus == "running" {
		http.Error(w, "脚本正在执行中，请稍后", http.StatusConflict)
		return
	}

	script, err := models.GetReleaseScript(r.Context(), h.db, config.ReleaseScriptID)
	if err != nil {
		http.Error(w, "release script not found", http.StatusNotFound)
		return
	}

	app, err := models.GetApplication(r.Context(), h.db, config.ApplicationID)
	if err != nil {
		http.Error(w, "application not found", http.StatusNotFound)
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

	opLog, _ := models.CreateOperationLogWithDetail(r.Context(), h.db, config.ApplicationID, config.ProjectID, "deploy", userID, models.OperationLogDetail{
		CIConfigID:        config.ID,
		ReleaseScriptID:   config.ReleaseScriptID,
		ReleaseScriptName: script.Name,
		ReleaseBranch:     config.ReleaseBranch,
		ModuleName:        config.ModuleName,
		ModulePath:        config.ModulePath,
	})

	models.UpdateCIRunStatus(context.Background(), h.db, id, "running", "")
	if updatedApp, err := models.UpdateApplicationBranch(context.Background(), h.db, config.ApplicationID, config.ReleaseBranch); err != nil {
		msg := err.Error()
		models.UpdateOperationLogStatus(context.Background(), h.db, opLog.ID, "failed", msg)
		models.UpdateCIRunStatus(context.Background(), h.db, id, "failed", msg)
		sendSSE("log", msg)
		sendSSE("done", "failed")
		return
	} else {
		app = updatedApp
	}

	if h.workDir == "" {
		msg := "CI 工作目录未配置，请在 config.yaml 中设置 work_dir.ci_path"
		models.UpdateOperationLogStatus(context.Background(), h.db, opLog.ID, "failed", msg)
		models.UpdateCIRunStatus(context.Background(), h.db, id, "failed", msg)
		sendSSE("log", msg)
		sendSSE("done", "failed")
		return
	}

	workDir := scriptexec.ResolveWorkDir(h.workDir, app.ProjectName, app.Name)
	runVariables := models.Variables(nil)
	if token := strings.TrimSpace(r.URL.Query().Get("token")); token != "" {
		run, ok := h.runStore.Get(token, id)
		if !ok {
			msg := "发布变量 token 无效或已过期"
			models.UpdateOperationLogStatus(context.Background(), h.db, opLog.ID, "failed", msg)
			models.UpdateCIRunStatus(context.Background(), h.db, id, "failed", msg)
			sendSSE("log", msg)
			sendSSE("done", "failed")
			return
		}
		runVariables = run.Variables
	} else {
		runVariables = mergeRunVariables(config, script, app, workDir, nil)
	}

	execCtx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	logFn := func(format string, args ...any) {
		line := fmt.Sprintf(format, args...)
		sendSSE("log", line)
	}

	tools, err := models.GetGlobalToolConfig(r.Context(), h.db)
	if err != nil {
		msg := err.Error()
		models.UpdateOperationLogStatus(context.Background(), h.db, opLog.ID, "failed", msg)
		models.UpdateCIRunStatus(context.Background(), h.db, id, "failed", msg)
		sendSSE("log", msg)
		sendSSE("done", "failed")
		return
	}

	sendSSE("log", fmt.Sprintf("开始拉取代码: %s", config.ReleaseBranch))
	pullResult, err := gitops.PullWithOptions(execCtx, app.GitURL, config.ReleaseBranch, workDir, gitops.PullOptions{GitPath: tools.GitPath}, logFn)
	if err != nil {
		pullResult.Status = "failed"
		pullResult.Message = err.Error()
	}
	if pullResult.Status != "success" {
		models.UpdateOperationLogStatus(context.Background(), h.db, opLog.ID, pullResult.Status, pullResult.Message)
		models.UpdateCIRunStatus(context.Background(), h.db, id, pullResult.Status, pullResult.Message)
		sendSSE("log", pullResult.Message)
		sendSSE("done", pullResult.Status)
		return
	}
	sendSSE("log", "代码拉取完成")

	result, err := scriptexec.ExecWithOptions(execCtx, script.Content, workDir, scriptexec.ExecOptions{
		Env: runVariables.Env(),
		Maven: scriptexec.MavenOptions{
			HomePath:         tools.MavenHomePath,
			UserSettingsFile: tools.MavenUserSettingsFile,
			LocalRepository:  tools.MavenLocalRepository,
		},
		NPM: scriptexec.NPMOptions{
			HomePath:       tools.NPMHomePath,
			UserConfigFile: tools.NPMUserConfigFile,
			CacheDirectory: tools.NPMCacheDirectory,
		},
	}, logFn)

	status := result.Status
	message := result.Message
	if err != nil {
		status = "failed"
		message = err.Error()
	}

	models.UpdateOperationLogStatus(context.Background(), h.db, opLog.ID, status, message)
	models.UpdateCIRunStatus(context.Background(), h.db, id, status, message)
	if message != "" {
		sendSSE("log", message)
	}
	sendSSE("done", status)
}

func (h *CIConfigHandler) prepareRunVariables(ctx context.Context, id int64, runtimeVariables models.Variables) (models.CIConfig, models.ReleaseScript, models.Application, models.Variables, error) {
	config, err := models.GetCIConfig(ctx, h.db, id)
	if err != nil {
		return models.CIConfig{}, models.ReleaseScript{}, models.Application{}, nil, fmt.Errorf("CI 配置不存在")
	}

	script, err := models.GetReleaseScript(ctx, h.db, config.ReleaseScriptID)
	if err != nil {
		return models.CIConfig{}, models.ReleaseScript{}, models.Application{}, nil, fmt.Errorf("发布脚本不存在")
	}

	app, err := models.GetApplication(ctx, h.db, config.ApplicationID)
	if err != nil {
		return models.CIConfig{}, models.ReleaseScript{}, models.Application{}, nil, fmt.Errorf("应用不存在")
	}

	workDir := ""
	if h.workDir != "" {
		workDir = scriptexec.ResolveWorkDir(h.workDir, app.ProjectName, app.Name)
	}

	return config, script, app, mergeRunVariables(config, script, app, workDir, runtimeVariables), nil
}

func mergeRunVariables(config models.CIConfig, script models.ReleaseScript, app models.Application, workDir string, runtimeVariables models.Variables) models.Variables {
	builtIns := models.Variables{
		"PROJECT_NAME":     app.Name,
		"APPLICATION_NAME": app.Name,
		"PROJECT_GROUP":    app.ProjectName,
		"MODULE_NAME":      config.ModuleName,
		"MODULE_PATH":      config.ModulePath,
		"RELEASE_BRANCH":   config.ReleaseBranch,
		"WORK_DIR":         workDir,
	}
	if config.ModuleName != "" {
		builtIns["PROJECT_NAME"] = config.ModuleName
	}
	if workDir != "" && config.ModulePath != "" {
		builtIns["MODULE_WORK_DIR"] = filepath.Join(workDir, config.ModulePath)
	}
	return models.MergeVariables(builtIns, script.DefaultVariables, config.Variables, runtimeVariables)
}

func (h *CIConfigHandler) Delete(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	if err := models.DeleteCIConfig(ctx, h.db, id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("HX-Refresh", "true")
	w.WriteHeader(http.StatusOK)
}
