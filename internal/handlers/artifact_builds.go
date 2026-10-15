package handlers

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"aether_ops/internal/gitops"
	"aether_ops/internal/middleware"
	"aether_ops/internal/models"
	"aether_ops/internal/registry"
	"aether_ops/internal/scriptexec"
	"aether_ops/internal/templates"
)

type ArtifactBuildHandler struct {
	db       *pgxpool.Pool
	rdr      *templates.Renderer
	workDir  string
	runStore *deployRunStore
}

func NewArtifactBuildHandler(db *pgxpool.Pool, rdr *templates.Renderer, workDir string) *ArtifactBuildHandler {
	return &ArtifactBuildHandler{db: db, rdr: rdr, workDir: workDir, runStore: newDeployRunStore()}
}

func (h *ArtifactBuildHandler) Register(r chi.Router) {
	r.Post("/artifact-builds", h.Create)
	r.Get("/artifact-builds/{id}/edit", h.EditForm)
	r.Put("/artifact-builds/{id}", h.Update)
	r.Get("/artifact-builds/{id}/row", h.GetRow)
	r.Get("/artifact-builds/{id}/run/confirm", h.RunConfirm)
	r.Post("/artifact-builds/{id}/run/prepare", h.RunPrepare)
	r.Get("/artifact-builds/{id}/run/stream", h.RunStream)
}

func (h *ArtifactBuildHandler) Create(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	applicationID, err := strconv.ParseInt(r.FormValue("application_id"), 10, 64)
	if err != nil || applicationID == 0 {
		writeModalError(w, "应用不能为空")
		return
	}
	releaseScriptID, err := strconv.ParseInt(r.FormValue("release_script_id"), 10, 64)
	if err != nil || releaseScriptID == 0 {
		writeModalError(w, "脚本不能为空")
		return
	}
	buildMode, err := models.NormalizeArtifactBuildMode(r.FormValue("build_mode"))
	if err != nil {
		writeModalError(w, err.Error())
		return
	}
	artifactRegistryID := int64(0)
	if buildMode == models.ArtifactBuildModeDocker {
		artifactRegistryID, err = strconv.ParseInt(r.FormValue("artifact_registry_id"), 10, 64)
		if err != nil || artifactRegistryID == 0 {
			writeModalError(w, "镜像仓库不能为空")
			return
		}
		if _, err := models.GetArtifactRegistry(ctx, h.db, artifactRegistryID); err != nil {
			writeModalError(w, "镜像仓库不存在")
			return
		}
	}

	_, err = models.CreateArtifactBuildConfigs(ctx, h.db, applicationID, releaseScriptID, buildMode, artifactRegistryID)
	if err != nil {
		writeModalError(w, err.Error())
		return
	}
	w.Header().Set("HX-Refresh", "true")
	w.WriteHeader(http.StatusOK)
}

func writeModalError(w http.ResponseWriter, message string) {
	w.WriteHeader(http.StatusUnprocessableEntity)
	w.Write([]byte(`<span class="error">` + message + `</span>`))
}

func (h *ArtifactBuildHandler) EditForm(w http.ResponseWriter, r *http.Request) {
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
	if config.ConfigType != "artifact" {
		http.Error(w, "not artifact build config", http.StatusNotFound)
		return
	}
	scripts, _ := models.ListReleaseScripts(ctx, h.db)
	registries, _ := models.ListArtifactRegistries(ctx, h.db)
	h.rdr.Partial(w, "artifact_build_edit_form", map[string]any{
		"Config":     config,
		"Scripts":    scripts,
		"Registries": registries,
	})
}

func (h *ArtifactBuildHandler) Update(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
	releaseScriptID, err := strconv.ParseInt(r.FormValue("artifact_release_script_id"), 10, 64)
	if err != nil || releaseScriptID == 0 {
		h.renderArtifactBuildEditError(w, ctx, id, "发布脚本不能为空")
		return
	}
	buildMode, err := models.NormalizeArtifactBuildMode(r.FormValue("build_mode"))
	if err != nil {
		h.renderArtifactBuildEditError(w, ctx, id, err.Error())
		return
	}
	artifactRegistryID := int64(0)
	if buildMode == models.ArtifactBuildModeDocker {
		artifactRegistryID, err = strconv.ParseInt(r.FormValue("artifact_registry_id"), 10, 64)
		if err != nil || artifactRegistryID == 0 {
			h.renderArtifactBuildEditError(w, ctx, id, "镜像仓库不能为空")
			return
		}
		if _, err := models.GetArtifactRegistry(ctx, h.db, artifactRegistryID); err != nil {
			h.renderArtifactBuildEditError(w, ctx, id, "镜像仓库不存在")
			return
		}
	}

	config, err := models.UpdateArtifactBuildSettings(ctx, h.db, id, releaseScriptID, buildMode, artifactRegistryID)
	if err != nil {
		h.renderArtifactBuildEditError(w, ctx, id, err.Error())
		return
	}
	w.Header().Set("HX-Refresh", "true")
	h.rdr.Partial(w, "artifact_build_row", config)
}

func (h *ArtifactBuildHandler) renderArtifactBuildEditError(w http.ResponseWriter, ctx context.Context, id int64, message string) {
	config, _ := models.GetCIConfig(ctx, h.db, id)
	scripts, _ := models.ListReleaseScripts(ctx, h.db)
	registries, _ := models.ListArtifactRegistries(ctx, h.db)
	w.WriteHeader(http.StatusUnprocessableEntity)
	h.rdr.Partial(w, "artifact_build_edit_form", map[string]any{
		"Config":     config,
		"Scripts":    scripts,
		"Registries": registries,
		"Error":      message,
	})
}

func (h *ArtifactBuildHandler) GetRow(w http.ResponseWriter, r *http.Request) {
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
	if config.ConfigType != "artifact" {
		http.Error(w, "not artifact build config", http.StatusNotFound)
		return
	}
	h.rdr.Partial(w, "artifact_build_row", config)
}

func (h *ArtifactBuildHandler) RunConfirm(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
	config, err := models.RefreshArtifactBuildConfigFromApplication(ctx, h.db, id)
	if err != nil {
		http.Error(w, "打包配置不存在", http.StatusNotFound)
		return
	}
	if config.ConfigType != "artifact" {
		http.Error(w, "打包配置不存在", http.StatusNotFound)
		return
	}
	script, err := models.GetReleaseScript(ctx, h.db, artifactBuildScriptID(config))
	if err != nil {
		http.Error(w, "脚本不存在", http.StatusUnprocessableEntity)
		return
	}
	app, err := models.GetApplication(ctx, h.db, config.ApplicationID)
	if err != nil {
		http.Error(w, "应用不存在", http.StatusUnprocessableEntity)
		return
	}

	h.rdr.Partial(w, "artifact_build_run_confirm", map[string]any{
		"Config":          config,
		"Script":          script,
		"App":             app,
		"DefaultImageTag": h.defaultArtifactImageTag(ctx, config, time.Now()),
		"Variables":       h.artifactBuildVariables(config, script, app, nil).Entries(),
	})
}

func (h *ArtifactBuildHandler) artifactBuildVariables(config models.CIConfig, script models.ReleaseScript, app models.Application, runtimeVariables models.Variables) models.Variables {
	workDir := ""
	if h.workDir != "" {
		workDir = scriptexec.ResolveWorkDir(h.workDir, app.ProjectName, app.Name)
	}
	return mergeRunVariables(config, script, app, workDir, runtimeVariables)
}

func (h *ArtifactBuildHandler) defaultArtifactImageTag(ctx context.Context, config models.CIConfig, now time.Time) string {
	prefix := now.Format("06.0102")
	if config.ArtifactRegistryID == nil || *config.ArtifactRegistryID == 0 {
		return nextArtifactImageTag(prefix, nil)
	}

	repositories, err := models.ListDockerDeployRepositoriesForArtifactBuild(ctx, h.db, config.ID, *config.ArtifactRegistryID)
	if err != nil || len(repositories) == 0 {
		return nextArtifactImageTag(prefix, nil)
	}
	tags, err := models.ListArtifactTagsByRepositories(ctx, h.db, *config.ArtifactRegistryID, repositories, prefix)
	if err != nil {
		return nextArtifactImageTag(prefix, nil)
	}
	return nextArtifactImageTag(prefix, tags)
}

func nextArtifactImageTag(prefix string, tags []string) string {
	maxSuffix := -1
	for _, tag := range tags {
		tag = strings.TrimSpace(tag)
		if tag == prefix {
			if maxSuffix < 0 {
				maxSuffix = 0
			}
			continue
		}
		suffix, ok := strings.CutPrefix(tag, prefix+".")
		if !ok || suffix == "" {
			continue
		}
		n, err := strconv.Atoi(suffix)
		if err != nil || n < 0 {
			continue
		}
		if n > maxSuffix {
			maxSuffix = n
		}
	}
	return fmt.Sprintf("%s.%d", prefix, maxSuffix+1)
}

func (h *ArtifactBuildHandler) RunPrepare(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
	config, err := models.GetCIConfig(ctx, h.db, id)
	if err != nil || config.ConfigType != "artifact" {
		http.Error(w, "打包配置不存在", http.StatusNotFound)
		return
	}
	runVariables := models.Variables{}
	if config.BuildMode == models.ArtifactBuildModeScript {
		runtimeVariables, err := models.ParseVariablesFromRequest(r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusUnprocessableEntity)
			return
		}
		script, err := models.GetReleaseScript(ctx, h.db, artifactBuildScriptID(config))
		if err != nil {
			http.Error(w, "脚本不存在", http.StatusUnprocessableEntity)
			return
		}
		app, err := models.GetApplication(ctx, h.db, config.ApplicationID)
		if err != nil {
			http.Error(w, "应用不存在", http.StatusUnprocessableEntity)
			return
		}
		runVariables = h.artifactBuildVariables(config, script, app, runtimeVariables)
	} else {
		imageTag := strings.TrimSpace(r.FormValue("image_tag"))
		if imageTag == "" {
			http.Error(w, "版本号不能为空", http.StatusUnprocessableEntity)
			return
		}
		runVariables["IMAGE_TAG"] = imageTag
		runVariables["ARTIFACT_TAG"] = imageTag
	}
	token, err := h.runStore.Put(deployRun{
		ConfigID:  id,
		Variables: runVariables,
		ExpiresAt: time.Now().Add(15 * time.Minute),
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Write([]byte(token))
}

func (h *ArtifactBuildHandler) RunStream(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	config, err := models.RefreshArtifactBuildConfigFromApplication(r.Context(), h.db, id)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if config.ConfigType != "artifact" {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if config.LastRunStatus != nil && *config.LastRunStatus == "running" {
		http.Error(w, "脚本正在执行中，请稍后", http.StatusConflict)
		return
	}
	scriptID := artifactBuildScriptID(config)
	script, err := models.GetReleaseScript(r.Context(), h.db, scriptID)
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
	fail := func(opLogID int64, msg string) {
		if opLogID > 0 {
			models.UpdateOperationLogStatus(context.Background(), h.db, opLogID, "failed", msg)
		}
		models.UpdateCIRunStatus(context.Background(), h.db, id, "failed", msg)
		sendSSE("log", msg)
		sendSSE("done", "failed")
	}

	var userID int64
	if u := middleware.CurrentUser(r.Context()); u != nil {
		userID = u.ID
	}
	opLog, _ := models.CreateOperationLogWithDetail(r.Context(), h.db, config.ApplicationID, config.ProjectID, "artifact_build", userID, models.OperationLogDetail{
		CIConfigID:        config.ID,
		ReleaseScriptID:   scriptID,
		ReleaseScriptName: script.Name,
		ReleaseBranch:     config.ReleaseBranch,
		ModuleName:        config.ModuleName,
		ModulePath:        config.ModulePath,
	})

	models.UpdateCIRunStatus(context.Background(), h.db, id, "running", "")
	if h.workDir == "" {
		fail(opLog.ID, "CI 工作目录未配置，请在 config.yaml 中设置 work_dir.ci_path")
		return
	}

	workDir := scriptexec.ResolveWorkDir(h.workDir, app.ProjectName, app.Name)
	execCtx, cancel := context.WithTimeout(context.Background(), 60*time.Minute)
	defer cancel()
	logFn := func(format string, args ...any) {
		sendSSE("log", fmt.Sprintf(format, args...))
	}

	tools, err := models.GetGlobalToolConfig(r.Context(), h.db)
	if err != nil {
		fail(opLog.ID, err.Error())
		return
	}

	sendSSE("log", fmt.Sprintf("开始拉取代码: %s", config.ReleaseBranch))
	pullResult, err := gitops.PullWithOptions(execCtx, app.GitURL, config.ReleaseBranch, workDir, gitops.PullOptions{GitPath: tools.GitPath}, logFn)
	if err != nil {
		pullResult.Status = "failed"
		pullResult.Message = err.Error()
	}
	if pullResult.Status != "success" {
		fail(opLog.ID, pullResult.Message)
		return
	}
	sendSSE("log", "代码拉取完成")

	token := strings.TrimSpace(r.URL.Query().Get("token"))
	if token == "" {
		fail(opLog.ID, "打包变量 token 缺失，请重新点击打包并确认运行参数")
		return
	}
	run, ok := h.runStore.Get(token, id)
	if !ok {
		fail(opLog.ID, "打包变量 token 无效或已过期，请重新点击打包并确认运行参数")
		return
	}
	runVariables := mergeRunVariables(config, script, app, workDir, run.Variables)
	result, err := scriptexec.ExecWithOptions(execCtx, script.Content, workDir, scriptexec.ExecOptions{
		Env:     runVariables.Env(),
		Timeout: 60 * time.Minute,
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
	if status != "success" {
		fail(opLog.ID, message)
		return
	}
	if message != "" {
		sendSSE("log", message)
	}
	if config.BuildMode == models.ArtifactBuildModeScript {
		if message == "" {
			message = "打包脚本执行完成"
		}
		models.UpdateOperationLogStatus(context.Background(), h.db, opLog.ID, "success", message)
		models.UpdateCIRunStatus(context.Background(), h.db, id, "success", message)
		sendSSE("done", "success")
		return
	}

	imageTag := strings.TrimSpace(run.Variables["IMAGE_TAG"])
	if imageTag == "" {
		fail(opLog.ID, "打包版本号缺失，无法同步本次镜像")
		return
	}

	sendSSE("log", "开始同步本次镜像")
	syncCtx, syncCancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer syncCancel()
	syncMsg, err := h.syncArtifactBuildImages(syncCtx, config, imageTag, logFn)
	if err != nil {
		fail(opLog.ID, err.Error())
		return
	}
	message = "打包完成；" + syncMsg
	models.UpdateOperationLogStatus(context.Background(), h.db, opLog.ID, "success", message)
	models.UpdateCIRunStatus(context.Background(), h.db, id, "success", message)
	sendSSE("log", syncMsg)
	sendSSE("done", "success")
}

func artifactBuildScriptID(config models.CIConfig) int64 {
	if config.ArtifactReleaseScriptID != nil && *config.ArtifactReleaseScriptID > 0 {
		return *config.ArtifactReleaseScriptID
	}
	return config.ReleaseScriptID
}

func (h *ArtifactBuildHandler) syncArtifactBuildImages(ctx context.Context, config models.CIConfig, tag string, logf func(string, ...any)) (string, error) {
	if config.ArtifactRegistryID == nil || *config.ArtifactRegistryID == 0 {
		return "", fmt.Errorf("打包配置缺少镜像仓库")
	}
	tag = strings.TrimSpace(tag)
	if tag == "" {
		return "", fmt.Errorf("镜像 tag 不能为空")
	}

	reg, err := models.GetArtifactRegistry(ctx, h.db, *config.ArtifactRegistryID)
	if err != nil {
		return "", fmt.Errorf("镜像仓库不存在")
	}
	if !reg.Enabled {
		return "", fmt.Errorf("仓库未启用")
	}
	models.UpdateArtifactRegistrySyncStatus(ctx, h.db, reg.ID, "running", "正在同步本次镜像")
	finishFailed := func(err error) (string, error) {
		models.UpdateArtifactRegistrySyncStatus(context.Background(), h.db, reg.ID, "failed", err.Error())
		return "", err
	}

	repositories, err := models.ListDockerDeployRepositoriesForArtifactBuild(ctx, h.db, config.ID, reg.ID)
	if err != nil {
		return finishFailed(err)
	}
	if len(repositories) == 0 {
		return finishFailed(fmt.Errorf("未找到当前模块的发布映射，无法同步本次镜像"))
	}

	client, err := newRegistryClient(reg)
	if err != nil {
		return finishFailed(err)
	}

	items := make([]models.ArtifactSyncItem, 0, len(repositories))
	refs := make([]string, 0, len(repositories))
	for _, repository := range repositories {
		logf("同步镜像: %s:%s", repository, tag)
		image, err := client.GetImage(ctx, repository, tag)
		if err != nil {
			return finishFailed(fmt.Errorf("同步镜像 %s:%s 失败: %w", repository, tag, err))
		}
		items = append(items, artifactSyncItemFromImage(image))
		refs = append(refs, image.Repository+":"+image.Tag)
	}

	if err := models.UpsertArtifacts(ctx, h.db, reg.ID, items); err != nil {
		return finishFailed(err)
	}
	msg := fmt.Sprintf("已同步本次镜像 %d 个：%s", len(items), strings.Join(refs, "，"))
	models.UpdateArtifactRegistrySyncStatus(ctx, h.db, reg.ID, "success", msg)
	return msg, nil
}

func artifactSyncItemFromImage(image registry.Image) models.ArtifactSyncItem {
	return models.ArtifactSyncItem{
		Repository: image.Repository,
		Tag:        image.Tag,
		Digest:     image.Digest,
		MediaType:  image.MediaType,
		SizeBytes:  image.SizeBytes,
		PushedAt:   image.PushedAt,
	}
}

func (h *ArtifactBuildHandler) syncArtifactBuildRegistries(ctx context.Context, config models.CIConfig) (string, error) {
	if config.ArtifactRegistryID != nil && *config.ArtifactRegistryID > 0 {
		return syncArtifactRegistryByID(ctx, h.db, *config.ArtifactRegistryID)
	}

	registries, err := models.ListArtifactRegistries(ctx, h.db)
	if err != nil {
		return "", err
	}
	messages := make([]string, 0)
	for _, reg := range registries {
		if !reg.Enabled {
			continue
		}
		msg, err := syncArtifactRegistryNow(ctx, h.db, reg)
		if err != nil {
			return "", err
		}
		messages = append(messages, reg.Name+"："+msg)
	}
	if len(messages) == 0 {
		return "", fmt.Errorf("没有启用的镜像仓库")
	}
	return "同步全部启用仓库完成；" + strings.Join(messages, "；"), nil
}
