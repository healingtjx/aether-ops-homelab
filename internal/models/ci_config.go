package models

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type CIConfig struct {
	ID                        int64
	ConfigType                string
	BuildMode                 string
	ApplicationID             int64
	ApplicationName           string
	ProjectID                 int64
	ProjectName               string
	ReleaseScriptID           int64
	ReleaseScriptName         string
	ArtifactRegistryID        *int64
	ArtifactRegistryName      *string
	ArtifactReleaseScriptID   *int64
	ArtifactReleaseScriptName *string
	ReleaseBranch             string
	ModuleName                string
	ModulePath                string
	Variables                 Variables
	LastRunStatus             *string
	LastRunMessage            *string
	LastRunAt                 *time.Time
	CreatedAt                 time.Time
	UpdatedAt                 time.Time
}

func ListCIConfigs(ctx context.Context, db *pgxpool.Pool) ([]CIConfig, error) {
	return ListCIConfigsByType(ctx, db, "ci")
}

func ListArtifactBuildConfigs(ctx context.Context, db *pgxpool.Pool) ([]CIConfig, error) {
	configs, err := ListCIConfigsByType(ctx, db, "artifact")
	if err != nil {
		return nil, fmt.Errorf("list artifact build configs: %w", err)
	}
	return configs, nil
}

func ListCIConfigsByType(ctx context.Context, db *pgxpool.Pool, configType string) ([]CIConfig, error) {
	rows, err := queryRows(ctx, db, sqlListCIConfigsByType, configType)
	if err != nil {
		return nil, fmt.Errorf("list ci configs: %w", err)
	}
	defer rows.Close()
	return pgx.CollectRows(rows, pgx.RowToStructByName[CIConfig])
}

func GetCIConfig(ctx context.Context, db *pgxpool.Pool, id int64) (CIConfig, error) {
	rows, err := queryRows(ctx, db, sqlGetCIConfig, id)
	if err != nil {
		return CIConfig{}, fmt.Errorf("get ci config: %w", err)
	}
	defer rows.Close()
	return pgx.CollectOneRow(rows, pgx.RowToStructByName[CIConfig])
}

func CreateCIConfig(ctx context.Context, db *pgxpool.Pool, applicationID int64, releaseScriptID int64, releaseBranch string, moduleName string, modulePath string, variables Variables) (CIConfig, error) {
	releaseBranch = normalizeBranch(releaseBranch)
	moduleName, modulePath = normalizeModule(moduleName, modulePath)

	tx, err := db.Begin(ctx)
	if err != nil {
		return CIConfig{}, fmt.Errorf("begin create ci config: %w", err)
	}
	defer tx.Rollback(ctx)

	var id int64
	err = queryRow(ctx, tx, sqlCreateCIConfig, "ci", ArtifactBuildModeDocker, applicationID, releaseScriptID, nil, nil, releaseBranch, moduleName, modulePath, variables).Scan(&id)
	if err != nil {
		if strings.Contains(err.Error(), "23505") {
			return CIConfig{}, fmt.Errorf("该应用已绑定此发布脚本和模块")
		}
		return CIConfig{}, fmt.Errorf("create ci config: %w", err)
	}
	if _, err := execSQL(ctx, tx, sqlUpdateApplicationBranch, applicationID, releaseBranch); err != nil {
		return CIConfig{}, fmt.Errorf("update application branch: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return CIConfig{}, fmt.Errorf("commit create ci config: %w", err)
	}
	return GetCIConfig(ctx, db, id)
}

func CreateArtifactBuildConfigs(ctx context.Context, db *pgxpool.Pool, applicationID int64, releaseScriptID int64, buildMode string, artifactRegistryID int64) ([]CIConfig, error) {
	buildMode, err := NormalizeArtifactBuildMode(buildMode)
	if err != nil {
		return nil, err
	}
	if buildMode == ArtifactBuildModeDocker && artifactRegistryID == 0 {
		return nil, fmt.Errorf("Docker 镜像模式必须选择镜像仓库")
	}
	if buildMode == ArtifactBuildModeScript {
		artifactRegistryID = 0
	}
	if _, err := GetApplication(ctx, db, applicationID); err != nil {
		return nil, fmt.Errorf("应用不存在")
	}
	sourceConfigs, err := ListCIConfigsByType(ctx, db, "ci")
	if err != nil {
		return nil, err
	}

	sourceByApplication := make([]CIConfig, 0)
	for _, config := range sourceConfigs {
		if config.ApplicationID == applicationID {
			sourceByApplication = append(sourceByApplication, config)
		}
	}
	if len(sourceByApplication) == 0 {
		return nil, fmt.Errorf("请先在 CI 配置中为该应用添加模块，再创建打包配置")
	}

	tx, err := db.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin create artifact build config: %w", err)
	}
	defer tx.Rollback(ctx)

	ids := make([]int64, 0, len(sourceByApplication))
	for _, source := range sourceByApplication {
		var id int64
		err = queryRow(ctx, tx, sqlCreateCIConfig, "artifact", buildMode, applicationID, releaseScriptID, nullableInt64(artifactRegistryID), releaseScriptID, source.ReleaseBranch, source.ModuleName, source.ModulePath, source.Variables).Scan(&id)
		if err != nil {
			if strings.Contains(err.Error(), "23505") {
				return nil, fmt.Errorf("该应用模块 %s 已绑定此打包脚本", source.ModuleName)
			}
			return nil, fmt.Errorf("create artifact build config: %w", err)
		}
		ids = append(ids, id)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit create artifact build config: %w", err)
	}

	configs := make([]CIConfig, 0, len(ids))
	for _, id := range ids {
		config, err := GetCIConfig(ctx, db, id)
		if err != nil {
			return nil, err
		}
		configs = append(configs, config)
	}
	return configs, nil
}

func UpdateArtifactBuildSettings(ctx context.Context, db *pgxpool.Pool, id int64, releaseScriptID int64, buildMode string, artifactRegistryID int64) (CIConfig, error) {
	buildMode, err := NormalizeArtifactBuildMode(buildMode)
	if err != nil {
		return CIConfig{}, err
	}
	if buildMode == ArtifactBuildModeDocker && artifactRegistryID == 0 {
		return CIConfig{}, fmt.Errorf("Docker 镜像模式必须选择镜像仓库")
	}
	if buildMode == ArtifactBuildModeScript {
		artifactRegistryID = 0
	}
	var updatedID int64
	if err := queryRow(ctx, db, sqlUpdateArtifactBuildSettings, id, releaseScriptID, buildMode, nullableInt64(artifactRegistryID)).Scan(&updatedID); err != nil {
		return CIConfig{}, fmt.Errorf("update artifact build settings: %w", err)
	}
	return GetCIConfig(ctx, db, updatedID)
}

const (
	ArtifactBuildModeDocker = "docker"
	ArtifactBuildModeScript = "script"
)

func NormalizeArtifactBuildMode(mode string) (string, error) {
	mode = strings.TrimSpace(mode)
	if mode == "" {
		mode = ArtifactBuildModeDocker
	}
	if mode != ArtifactBuildModeDocker && mode != ArtifactBuildModeScript {
		return "", fmt.Errorf("打包方式不正确")
	}
	return mode, nil
}

func UpdateCIConfig(ctx context.Context, db *pgxpool.Pool, id int64, releaseScriptID int64, releaseBranch string, moduleName string, modulePath string, variables Variables) (CIConfig, error) {
	releaseBranch = normalizeBranch(releaseBranch)
	moduleName, modulePath = normalizeModule(moduleName, modulePath)

	tx, err := db.Begin(ctx)
	if err != nil {
		return CIConfig{}, fmt.Errorf("begin update ci config: %w", err)
	}
	defer tx.Rollback(ctx)

	var applicationID int64
	err = queryRow(ctx, tx, sqlUpdateCIConfig, id, releaseScriptID, releaseBranch, moduleName, modulePath, variables).Scan(&applicationID)
	if err != nil {
		if strings.Contains(err.Error(), "23505") {
			return CIConfig{}, fmt.Errorf("该应用已绑定此发布脚本和模块")
		}
		return CIConfig{}, fmt.Errorf("update ci config: %w", err)
	}
	if _, err := execSQL(ctx, tx, sqlUpdateApplicationBranch, applicationID, releaseBranch); err != nil {
		return CIConfig{}, fmt.Errorf("update application branch: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return CIConfig{}, fmt.Errorf("commit update ci config: %w", err)
	}
	return GetCIConfig(ctx, db, id)
}

func RefreshArtifactBuildConfigFromApplication(ctx context.Context, db *pgxpool.Pool, id int64) (CIConfig, error) {
	var refreshedID int64
	if err := queryRow(ctx, db, sqlRefreshArtifactBuildConfigFromApplication, id).Scan(&refreshedID); err != nil {
		return CIConfig{}, fmt.Errorf("refresh artifact build config: %w", err)
	}
	return GetCIConfig(ctx, db, refreshedID)
}

func normalizeModule(name string, path string) (string, string) {
	name = strings.TrimSpace(name)
	path = strings.TrimSpace(path)
	if path == "" {
		path = name
	}
	return name, path
}

func normalizeBranch(branch string) string {
	branch = strings.TrimSpace(branch)
	if branch == "" {
		return "main"
	}
	return branch
}

func DeleteCIConfig(ctx context.Context, db *pgxpool.Pool, id int64) error {
	_, err := execSQL(ctx, db, sqlDeleteCIConfig, id)
	if err != nil {
		return fmt.Errorf("delete ci config: %w", err)
	}
	return nil
}

func UpdateCIRunStatus(ctx context.Context, db *pgxpool.Pool, id int64, status string, message string) (CIConfig, error) {
	if status == "success" {
		_, err := execSQL(ctx, db, sqlUpdateCIRunStatusSuccess, id, status, message)
		if err != nil {
			return CIConfig{}, fmt.Errorf("update ci run status: %w", err)
		}
	} else {
		_, err := execSQL(ctx, db, sqlUpdateCIRunStatus, id, status, message)
		if err != nil {
			return CIConfig{}, fmt.Errorf("update ci run status: %w", err)
		}
	}
	return GetCIConfig(ctx, db, id)
}
