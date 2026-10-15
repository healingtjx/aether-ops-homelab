package models

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type GlobalToolConfig struct {
	ID                    int16
	GitPath               string
	MavenHomePath         string
	MavenUserSettingsFile string
	MavenLocalRepository  string
	NPMHomePath           string
	NPMUserConfigFile     string
	NPMCacheDirectory     string
	CreatedAt             time.Time
	UpdatedAt             time.Time
}

func DefaultGlobalToolConfig() GlobalToolConfig {
	return GlobalToolConfig{
		ID:      1,
		GitPath: "git",
	}
}

func GetGlobalToolConfig(ctx context.Context, db *pgxpool.Pool) (GlobalToolConfig, error) {
	if err := EnsureGlobalToolConfigSchema(ctx, db); err != nil {
		return GlobalToolConfig{}, err
	}

	rows, err := queryRows(ctx, db, sqlGetGlobalToolConfig)
	if err != nil {
		return GlobalToolConfig{}, fmt.Errorf("get global tool config: %w", err)
	}
	defer rows.Close()

	cfg, err := pgx.CollectOneRow(rows, pgx.RowToStructByName[GlobalToolConfig])
	if err != nil {
		if err == pgx.ErrNoRows {
			return DefaultGlobalToolConfig(), nil
		}
		return GlobalToolConfig{}, fmt.Errorf("get global tool config: %w", err)
	}
	return cfg.withDefaults(), nil
}

func UpdateGlobalToolConfig(ctx context.Context, db *pgxpool.Pool, gitPath string, mavenHomePath string, mavenUserSettingsFile string, mavenLocalRepository string, npmHomePath string, npmUserConfigFile string, npmCacheDirectory string) (GlobalToolConfig, error) {
	if err := EnsureGlobalToolConfigSchema(ctx, db); err != nil {
		return GlobalToolConfig{}, err
	}

	gitPath = strings.TrimSpace(gitPath)
	mavenHomePath = strings.TrimSpace(mavenHomePath)
	mavenUserSettingsFile = strings.TrimSpace(mavenUserSettingsFile)
	mavenLocalRepository = strings.TrimSpace(mavenLocalRepository)
	npmHomePath = strings.TrimSpace(npmHomePath)
	npmUserConfigFile = strings.TrimSpace(npmUserConfigFile)
	npmCacheDirectory = strings.TrimSpace(npmCacheDirectory)
	if gitPath == "" {
		gitPath = "git"
	}

	_, err := execSQL(ctx, db, sqlUpsertGlobalToolConfig, gitPath, mavenHomePath, mavenUserSettingsFile, mavenLocalRepository, npmHomePath, npmUserConfigFile, npmCacheDirectory)
	if err != nil {
		return GlobalToolConfig{}, fmt.Errorf("update global tool config: %w", err)
	}
	return GetGlobalToolConfig(ctx, db)
}

func EnsureGlobalToolConfigSchema(ctx context.Context, db *pgxpool.Pool) error {
	_, err := execSQL(ctx, db, sqlEnsureGlobalToolConfigTable)
	if err != nil {
		return fmt.Errorf("ensure global tool config schema: %w", err)
	}

	if err := ensureColumn(ctx, db, "maven_home_path", "VARCHAR(1024) NOT NULL DEFAULT ''"); err != nil {
		return err
	}
	if err := ensureColumn(ctx, db, "maven_user_settings_file", "VARCHAR(1024) NOT NULL DEFAULT ''"); err != nil {
		return err
	}
	if err := ensureColumn(ctx, db, "maven_local_repository", "VARCHAR(1024) NOT NULL DEFAULT ''"); err != nil {
		return err
	}
	if err := ensureColumn(ctx, db, "npm_home_path", "VARCHAR(1024) NOT NULL DEFAULT ''"); err != nil {
		return err
	}
	if err := ensureColumn(ctx, db, "npm_user_config_file", "VARCHAR(1024) NOT NULL DEFAULT ''"); err != nil {
		return err
	}
	if err := ensureColumn(ctx, db, "npm_cache_directory", "VARCHAR(1024) NOT NULL DEFAULT ''"); err != nil {
		return err
	}

	_, err = execSQL(ctx, db, sqlEnsureGlobalToolConfigRow)
	if err != nil {
		return fmt.Errorf("ensure global tool config schema: %w", err)
	}
	return nil
}

func ensureColumn(ctx context.Context, db *pgxpool.Pool, name string, definition string) error {
	var exists bool
	err := queryRow(ctx, db, sqlGlobalToolConfigColumnExists, name).Scan(&exists)
	if err != nil {
		return fmt.Errorf("check global tool config column %s: %w", name, err)
	}
	if exists {
		return nil
	}

	_, err = execSQL(ctx, db, sqlAddGlobalToolConfigColumn(name, definition))
	if err != nil {
		return fmt.Errorf("add global tool config column %s: %w", name, err)
	}
	return nil
}

func (c GlobalToolConfig) withDefaults() GlobalToolConfig {
	if strings.TrimSpace(c.GitPath) == "" {
		c.GitPath = "git"
	}
	return c
}
