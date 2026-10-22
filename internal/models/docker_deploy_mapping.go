package models

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type DockerDeploySettings struct {
	ScriptID     string
	ScriptParams Variables
	SSHMode      string
	DeploySSH    Variables
}

type DockerDeployMapping struct {
	ScriptID           string
	ScriptParams       Variables
	SSHMode            string
	DeploySSH          Variables
	ID                 int64
	CIConfigID         int64
	RegistryID         int64
	Repository         string
	ComposeServiceName string
	RemoteDir          string
	ComposeFile        string
	Description        *string
	Enabled            bool
	LastDeployStatus   *string
	LastDeployMessage  *string
	LastDeployedAt     *time.Time
	CreatedAt          time.Time
	UpdatedAt          time.Time
	ApplicationID      int64
	ApplicationName    string
	ProjectID          int64
	ProjectName        string
	ReleaseBranch      string
	ModuleName         string
	ModulePath         string
	RegistryName       string
	RegistryURL        string
}

type ArtifactRepositoryOption struct {
	RegistryID   int64
	RegistryName string
	Repository   string
	TagCount     int
}

func (m DockerDeployMapping) RegistryHost() string {
	return normalizeRegistryHost(m.RegistryURL)
}

func ListDockerDeployMappings(ctx context.Context, db *pgxpool.Pool) ([]DockerDeployMapping, error) {
	rows, err := queryRows(ctx, db, sqlListDockerDeployMappings)
	if err != nil {
		return nil, fmt.Errorf("list docker deploy mappings: %w", err)
	}
	defer rows.Close()
	return pgx.CollectRows(rows, pgx.RowToStructByName[DockerDeployMapping])
}

func GetDockerDeployMapping(ctx context.Context, db *pgxpool.Pool, id int64) (DockerDeployMapping, error) {
	rows, err := queryRows(ctx, db, sqlGetDockerDeployMapping, id)
	if err != nil {
		return DockerDeployMapping{}, fmt.Errorf("get docker deploy mapping: %w", err)
	}
	defer rows.Close()
	return pgx.CollectOneRow(rows, pgx.RowToStructByName[DockerDeployMapping])
}

func CreateDockerDeployMapping(ctx context.Context, db *pgxpool.Pool, ciConfigID int64, registryID int64, repository string, serviceName string, remoteDir string, composeFile string, description *string, enabled bool, settings DockerDeploySettings) (DockerDeployMapping, error) {
	repository, serviceName, remoteDir, composeFile = normalizeDockerDeployMapping(repository, serviceName, remoteDir, composeFile)
	var id int64
	err := queryRow(ctx, db, sqlCreateDockerDeployMapping, ciConfigID, registryID, repository, serviceName, remoteDir, composeFile, description, enabled, settings.ScriptID, settings.ScriptParams, settings.SSHMode, settings.DeploySSH).Scan(&id)
	if err != nil {
		if strings.Contains(err.Error(), "23505") {
			return DockerDeployMapping{}, fmt.Errorf("该发布映射已存在")
		}
		return DockerDeployMapping{}, fmt.Errorf("create docker deploy mapping: %w", err)
	}
	return GetDockerDeployMapping(ctx, db, id)
}

func UpdateDockerDeployMapping(ctx context.Context, db *pgxpool.Pool, id int64, ciConfigID int64, registryID int64, repository string, serviceName string, remoteDir string, composeFile string, description *string, enabled bool, settings DockerDeploySettings) (DockerDeployMapping, error) {
	repository, serviceName, remoteDir, composeFile = normalizeDockerDeployMapping(repository, serviceName, remoteDir, composeFile)
	_, err := execSQL(ctx, db, sqlUpdateDockerDeployMapping, id, ciConfigID, registryID, repository, serviceName, remoteDir, composeFile, description, enabled, settings.ScriptID, settings.ScriptParams, settings.SSHMode, settings.DeploySSH)
	if err != nil {
		if strings.Contains(err.Error(), "23505") {
			return DockerDeployMapping{}, fmt.Errorf("该发布映射已存在")
		}
		return DockerDeployMapping{}, fmt.Errorf("update docker deploy mapping: %w", err)
	}
	return GetDockerDeployMapping(ctx, db, id)
}

func DeleteDockerDeployMapping(ctx context.Context, db *pgxpool.Pool, id int64) error {
	_, err := execSQL(ctx, db, sqlDeleteDockerDeployMapping, id)
	if err != nil {
		return fmt.Errorf("delete docker deploy mapping: %w", err)
	}
	return nil
}

func UpdateDockerDeployStatus(ctx context.Context, db *pgxpool.Pool, id int64, status string, message string) (DockerDeployMapping, error) {
	var err error
	if status == "success" {
		_, err = execSQL(ctx, db, sqlUpdateDockerDeployStatusSuccess, id, status, message)
	} else {
		_, err = execSQL(ctx, db, sqlUpdateDockerDeployStatus, id, status, message)
	}
	if err != nil {
		return DockerDeployMapping{}, fmt.Errorf("update docker deploy status: %w", err)
	}
	return GetDockerDeployMapping(ctx, db, id)
}

func ListArtifactsByRepository(ctx context.Context, db *pgxpool.Pool, registryID int64, repository string) ([]Artifact, error) {
	rows, err := queryRows(ctx, db, sqlListArtifactsByRepository, registryID, strings.Trim(strings.TrimSpace(repository), "/"))
	if err != nil {
		return nil, fmt.Errorf("list artifacts by repository: %w", err)
	}
	defer rows.Close()
	return pgx.CollectRows(rows, pgx.RowToStructByName[Artifact])
}

func ListArtifactRepositories(ctx context.Context, db *pgxpool.Pool) ([]ArtifactRepositoryOption, error) {
	rows, err := queryRows(ctx, db, sqlListArtifactRepositories)
	if err != nil {
		return nil, fmt.Errorf("list artifact repositories: %w", err)
	}
	defer rows.Close()
	return pgx.CollectRows(rows, pgx.RowToStructByName[ArtifactRepositoryOption])
}

func ArtifactRepositoryExists(ctx context.Context, db *pgxpool.Pool, registryID int64, repository string) (bool, error) {
	var exists bool
	err := queryRow(ctx, db, sqlArtifactRepositoryExists, registryID, strings.Trim(strings.TrimSpace(repository), "/")).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("check artifact repository: %w", err)
	}
	return exists, nil
}

func ListDockerDeployRepositoriesForArtifactBuild(ctx context.Context, db *pgxpool.Pool, artifactConfigID int64, registryID int64) ([]string, error) {
	rows, err := queryRows(ctx, db, sqlListDockerDeployRepositoriesForArtifactBuild, artifactConfigID, registryID)
	if err != nil {
		return nil, fmt.Errorf("list docker deploy repositories for artifact build: %w", err)
	}
	defer rows.Close()

	repositories := make([]string, 0)
	for rows.Next() {
		var repository string
		if err := rows.Scan(&repository); err != nil {
			return nil, fmt.Errorf("scan docker deploy repository: %w", err)
		}
		repository = strings.Trim(strings.TrimSpace(repository), "/")
		if repository != "" {
			repositories = append(repositories, repository)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate docker deploy repositories: %w", err)
	}
	return repositories, nil
}

func normalizeDockerDeployMapping(repository string, serviceName string, remoteDir string, composeFile string) (string, string, string, string) {
	repository = strings.Trim(strings.TrimSpace(repository), "/")
	serviceName = strings.TrimSpace(serviceName)
	remoteDir = strings.TrimRight(strings.TrimSpace(remoteDir), "/")
	composeFile = strings.TrimSpace(composeFile)
	if composeFile == "" {
		composeFile = "docker-compose.yml"
	}
	return repository, serviceName, remoteDir, composeFile
}

func normalizeRegistryHost(registryURL string) string {
	return strings.TrimRight(strings.TrimPrefix(strings.TrimPrefix(strings.TrimSpace(registryURL), "https://"), "http://"), "/")
}

// ScriptParamsJSON never includes SSH credentials.
func (m DockerDeployMapping) ScriptParamsJSON() string {
	b, _ := json.Marshal(m.ScriptParams)
	return string(b)
}
