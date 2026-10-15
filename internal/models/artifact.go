package models

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type ArtifactRegistry struct {
	ID              int64
	Name            string
	Provider        string
	RegistryURL     string
	Namespace       string
	RepositoryNames string
	AuthType        string
	Username        string
	Credential      string
	Enabled         bool
	LastSyncStatus  *string
	LastSyncMessage *string
	LastSyncedAt    *time.Time
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

type Artifact struct {
	ID           int64
	RegistryID   int64
	RegistryName string
	Provider     string
	RegistryURL  string
	Namespace    string
	Repository   string
	Tag          string
	Digest       *string
	MediaType    *string
	SizeBytes    *int64
	PushedAt     *time.Time
	RemoteStatus string
	LastSyncedAt *time.Time
	DeletedAt    *time.Time
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

type ArtifactOperationLog struct {
	ID            int64
	RegistryID    *int64
	ArtifactID    *int64
	OperationType string
	Status        string
	Message       *string
	StartedAt     time.Time
	FinishedAt    *time.Time
}

type ArtifactSyncItem struct {
	Repository string
	Tag        string
	Digest     string
	MediaType  string
	SizeBytes  int64
	PushedAt   *time.Time
}

func (a Artifact) ImageRef() string {
	return a.RegistryHost() + "/" + a.RepositoryTagRef()
}

func (a Artifact) RegistryHost() string {
	return normalizeRegistryHost(a.RegistryURL)
}

func (a Artifact) RepositoryTagRef() string {
	return strings.Trim(strings.TrimSpace(a.Repository), "/") + ":" + a.Tag
}

func ListArtifactRegistries(ctx context.Context, db *pgxpool.Pool) ([]ArtifactRegistry, error) {
	rows, err := queryRows(ctx, db, sqlListArtifactRegistries)
	if err != nil {
		return nil, fmt.Errorf("list artifact registries: %w", err)
	}
	defer rows.Close()
	return pgx.CollectRows(rows, pgx.RowToStructByName[ArtifactRegistry])
}

func GetArtifactRegistry(ctx context.Context, db *pgxpool.Pool, id int64) (ArtifactRegistry, error) {
	rows, err := queryRows(ctx, db, sqlGetArtifactRegistry, id)
	if err != nil {
		return ArtifactRegistry{}, fmt.Errorf("get artifact registry: %w", err)
	}
	defer rows.Close()
	return pgx.CollectOneRow(rows, pgx.RowToStructByName[ArtifactRegistry])
}

func CreateArtifactRegistry(ctx context.Context, db *pgxpool.Pool, name string, provider string, registryURL string, namespace string, repositoryNames string, authType string, username string, credential string, enabled bool) (ArtifactRegistry, error) {
	provider, authType = normalizeArtifactRegistry(provider, authType)
	var id int64
	err := queryRow(ctx, db, sqlCreateArtifactRegistry, strings.TrimSpace(name), provider, strings.TrimSpace(registryURL), strings.Trim(strings.TrimSpace(namespace), "/"), normalizeRepositoryNames(repositoryNames), authType, strings.TrimSpace(username), credential, enabled).Scan(&id)
	if err != nil {
		if strings.Contains(err.Error(), "23505") {
			return ArtifactRegistry{}, fmt.Errorf("仓库名称已存在")
		}
		return ArtifactRegistry{}, fmt.Errorf("create artifact registry: %w", err)
	}
	return GetArtifactRegistry(ctx, db, id)
}

func UpdateArtifactRegistry(ctx context.Context, db *pgxpool.Pool, id int64, name string, provider string, registryURL string, namespace string, repositoryNames string, authType string, username string, credential string, keepCredential bool, enabled bool) (ArtifactRegistry, error) {
	provider, authType = normalizeArtifactRegistry(provider, authType)
	_, err := execSQL(ctx, db, sqlUpdateArtifactRegistry, id, strings.TrimSpace(name), provider, strings.TrimSpace(registryURL), strings.Trim(strings.TrimSpace(namespace), "/"), normalizeRepositoryNames(repositoryNames), authType, strings.TrimSpace(username), credential, keepCredential, enabled)
	if err != nil {
		if strings.Contains(err.Error(), "23505") {
			return ArtifactRegistry{}, fmt.Errorf("仓库名称已存在")
		}
		return ArtifactRegistry{}, fmt.Errorf("update artifact registry: %w", err)
	}
	return GetArtifactRegistry(ctx, db, id)
}

func DeleteArtifactRegistry(ctx context.Context, db *pgxpool.Pool, id int64) error {
	_, err := execSQL(ctx, db, sqlDeleteArtifactRegistry, id)
	if err != nil {
		return fmt.Errorf("delete artifact registry: %w", err)
	}
	return nil
}

func UpdateArtifactRegistrySyncStatus(ctx context.Context, db *pgxpool.Pool, id int64, status string, message string) error {
	var err error
	if status == "success" {
		_, err = execSQL(ctx, db, sqlUpdateArtifactRegistrySyncStatusSuccess, id, status, message)
	} else {
		_, err = execSQL(ctx, db, sqlUpdateArtifactRegistrySyncStatus, id, status, message)
	}
	if err != nil {
		return fmt.Errorf("update artifact registry sync status: %w", err)
	}
	return nil
}

func ListArtifacts(ctx context.Context, db *pgxpool.Pool, registryID int64, status string) ([]Artifact, error) {
	sql, args := buildListArtifactsSQL(registryID, status)
	rows, err := queryRows(ctx, db, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("list artifacts: %w", err)
	}
	defer rows.Close()
	return pgx.CollectRows(rows, pgx.RowToStructByName[Artifact])
}

func GetArtifact(ctx context.Context, db *pgxpool.Pool, id int64) (Artifact, error) {
	rows, err := queryRows(ctx, db, sqlGetArtifact, id)
	if err != nil {
		return Artifact{}, fmt.Errorf("get artifact: %w", err)
	}
	defer rows.Close()
	return pgx.CollectOneRow(rows, pgx.RowToStructByName[Artifact])
}

func ListArtifactTagsByRepositories(ctx context.Context, db *pgxpool.Pool, registryID int64, repositories []string, tagPrefix string) ([]string, error) {
	normalized := make([]string, 0, len(repositories))
	seen := make(map[string]struct{}, len(repositories))
	for _, repository := range repositories {
		repository = strings.Trim(strings.TrimSpace(repository), "/")
		if repository == "" {
			continue
		}
		if _, ok := seen[repository]; ok {
			continue
		}
		seen[repository] = struct{}{}
		normalized = append(normalized, repository)
	}
	if len(normalized) == 0 {
		return nil, nil
	}

	rows, err := queryRows(ctx, db, sqlListArtifactTagsByRepositories, registryID, normalized, strings.TrimSpace(tagPrefix))
	if err != nil {
		return nil, fmt.Errorf("list artifact tags by repositories: %w", err)
	}
	defer rows.Close()

	tags := make([]string, 0)
	for rows.Next() {
		var tag string
		if err := rows.Scan(&tag); err != nil {
			return nil, fmt.Errorf("scan artifact tag: %w", err)
		}
		tags = append(tags, tag)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate artifact tags: %w", err)
	}
	return tags, nil
}

func SyncArtifacts(ctx context.Context, db *pgxpool.Pool, registryID int64, items []ArtifactSyncItem) error {
	tx, err := db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin sync artifacts: %w", err)
	}
	defer tx.Rollback(ctx)

	seen := make([]string, 0, len(items))
	for _, item := range items {
		repository := strings.Trim(strings.TrimSpace(item.Repository), "/")
		tag := strings.TrimSpace(item.Tag)
		if repository == "" || tag == "" {
			continue
		}
		seen = append(seen, repository+"\n"+tag)
		if _, err := execSQL(ctx, tx, sqlUpsertArtifact, registryID, repository, tag, nullableString(item.Digest), nullableString(item.MediaType), nullableInt64Value(item.SizeBytes), item.PushedAt); err != nil {
			return fmt.Errorf("upsert artifact %s:%s: %w", repository, tag, err)
		}
	}

	if len(seen) == 0 {
		if _, err := execSQL(ctx, tx, sqlDeleteAllSyncedArtifacts, registryID); err != nil {
			return fmt.Errorf("delete missing artifacts: %w", err)
		}
	} else if _, err := execSQL(ctx, tx, sqlDeleteMissingArtifacts, registryID, seen); err != nil {
		return fmt.Errorf("delete missing artifacts: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit sync artifacts: %w", err)
	}
	return nil
}

func UpsertArtifacts(ctx context.Context, db *pgxpool.Pool, registryID int64, items []ArtifactSyncItem) error {
	tx, err := db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin upsert artifacts: %w", err)
	}
	defer tx.Rollback(ctx)

	if err := upsertArtifactItems(ctx, tx, registryID, items); err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit upsert artifacts: %w", err)
	}
	return nil
}

func upsertArtifactItems(ctx context.Context, exec sqlRunner, registryID int64, items []ArtifactSyncItem) error {
	for _, item := range items {
		repository := strings.Trim(strings.TrimSpace(item.Repository), "/")
		tag := strings.TrimSpace(item.Tag)
		if repository == "" || tag == "" {
			continue
		}
		if _, err := execSQL(ctx, exec, sqlUpsertArtifact, registryID, repository, tag, nullableString(item.Digest), nullableString(item.MediaType), nullableInt64Value(item.SizeBytes), item.PushedAt); err != nil {
			return fmt.Errorf("upsert artifact %s:%s: %w", repository, tag, err)
		}
	}
	return nil
}

func MarkArtifactDeleted(ctx context.Context, db *pgxpool.Pool, id int64) error {
	_, err := execSQL(ctx, db, sqlMarkArtifactDeleted, id)
	if err != nil {
		return fmt.Errorf("mark artifact deleted: %w", err)
	}
	return nil
}

func CreateArtifactOperationLog(ctx context.Context, db *pgxpool.Pool, registryID int64, artifactID int64, operationType string) (ArtifactOperationLog, error) {
	var id int64
	err := queryRow(ctx, db, sqlCreateArtifactOperationLog, nullableInt64(registryID), nullableInt64(artifactID), operationType).Scan(&id)
	if err != nil {
		return ArtifactOperationLog{}, fmt.Errorf("create artifact operation log: %w", err)
	}
	return GetArtifactOperationLog(ctx, db, id)
}

func UpdateArtifactOperationLogStatus(ctx context.Context, db *pgxpool.Pool, id int64, status string, message string) error {
	_, err := execSQL(ctx, db, sqlUpdateArtifactOperationLogStatus, id, status, message)
	if err != nil {
		return fmt.Errorf("update artifact operation log: %w", err)
	}
	return nil
}

func GetArtifactOperationLog(ctx context.Context, db *pgxpool.Pool, id int64) (ArtifactOperationLog, error) {
	rows, err := queryRows(ctx, db, sqlGetArtifactOperationLog, id)
	if err != nil {
		return ArtifactOperationLog{}, fmt.Errorf("get artifact operation log: %w", err)
	}
	defer rows.Close()
	return pgx.CollectOneRow(rows, pgx.RowToStructByName[ArtifactOperationLog])
}

func normalizeArtifactRegistry(provider string, authType string) (string, string) {
	provider = strings.TrimSpace(provider)
	switch provider {
	case "aws_ecr", "aliyun_acr":
	default:
		provider = "generic"
	}
	authType = strings.TrimSpace(authType)
	if authType != "bearer" {
		authType = "basic"
	}
	return provider, authType
}

func normalizeRepositoryNames(value string) string {
	lines := strings.FieldsFunc(value, func(r rune) bool {
		return r == '\n' || r == '\r' || r == ',' || r == ';'
	})
	repos := make([]string, 0, len(lines))
	seen := map[string]struct{}{}
	for _, line := range lines {
		repo := strings.Trim(strings.TrimSpace(line), "/")
		if repo == "" {
			continue
		}
		if _, ok := seen[repo]; ok {
			continue
		}
		seen[repo] = struct{}{}
		repos = append(repos, repo)
	}
	return strings.Join(repos, "\n")
}

func (r ArtifactRegistry) RepositoryList() []string {
	return splitRepositoryNames(r.RepositoryNames)
}

func splitRepositoryNames(value string) []string {
	lines := strings.FieldsFunc(value, func(r rune) bool {
		return r == '\n' || r == '\r' || r == ',' || r == ';'
	})
	repos := make([]string, 0, len(lines))
	for _, line := range lines {
		repo := strings.Trim(strings.TrimSpace(line), "/")
		if repo != "" {
			repos = append(repos, repo)
		}
	}
	return repos
}

func nullableInt64Value(v int64) *int64 {
	if v == 0 {
		return nil
	}
	return &v
}
