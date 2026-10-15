package handlers

import (
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"aether_ops/internal/models"
	"aether_ops/internal/registry"
)

func TestMergeRunVariablesUsesModuleNameForProjectName(t *testing.T) {
	config := models.CIConfig{
		ReleaseBranch: "release",
		ModuleName:    "blade-auth",
		ModulePath:    "blade-auth",
	}
	script := models.ReleaseScript{
		DefaultVariables: models.Variables{"REMOTE_HOST": "script-host"},
	}
	app := models.Application{
		Name:        "bs_portal",
		ProjectName: "beisi",
	}
	workDir := filepath.Join("/tmp", "ci", "beisi", "bs_portal")

	got := mergeRunVariables(config, script, app, workDir, nil)
	want := models.Variables{
		"PROJECT_NAME":     "blade-auth",
		"APPLICATION_NAME": "bs_portal",
		"PROJECT_GROUP":    "beisi",
		"MODULE_NAME":      "blade-auth",
		"MODULE_PATH":      "blade-auth",
		"MODULE_WORK_DIR":  filepath.Join(workDir, "blade-auth"),
		"RELEASE_BRANCH":   "release",
		"WORK_DIR":         workDir,
		"REMOTE_HOST":      "script-host",
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
}

func TestGroupCIConfigsGroupsOnlyByGitURL(t *testing.T) {
	configs := []models.CIConfig{
		{
			ID:                1,
			ApplicationID:     10,
			ApplicationName:   "app",
			ProjectName:       "ops",
			ReleaseScriptID:   20,
			ReleaseScriptName: "deploy-a",
			ReleaseBranch:     "main",
			ModuleName:        "api",
		},
		{
			ID:                2,
			ApplicationID:     10,
			ApplicationName:   "app",
			ProjectName:       "ops",
			ReleaseScriptID:   21,
			ReleaseScriptName: "deploy-b",
			ReleaseBranch:     "release",
			ModuleName:        "worker",
		},
		{
			ID:              3,
			ApplicationID:   11,
			ApplicationName: "other",
			ProjectName:     "ops",
			ReleaseBranch:   "main",
			ModuleName:      "other",
		},
	}
	applications := []models.Application{
		{ID: 10, GitURL: "git@example.com:ops/app.git"},
		{ID: 11, GitURL: "git@example.com:ops/other.git"},
	}

	groups := groupCIConfigs(configs, applications)
	if len(groups) != 2 {
		t.Fatalf("len(groups) got %d, want 2", len(groups))
	}
	if len(groups[0].Configs) != 2 {
		t.Fatalf("first group config count got %d, want 2", len(groups[0].Configs))
	}
	if groups[0].GitURL != "git@example.com:ops/app.git" {
		t.Fatalf("first group GitURL got %q", groups[0].GitURL)
	}
}

func TestMergeRunVariablesKeepsRootWorkDirAndAllowsRuntimeOverride(t *testing.T) {
	config := models.CIConfig{
		ReleaseBranch: "main",
		ModuleName:    "api",
		ModulePath:    "services/api",
		Variables: models.Variables{
			"REMOTE_PORT": "22",
		},
	}
	script := models.ReleaseScript{
		DefaultVariables: models.Variables{
			"PROJECT_NAME": "script-project",
			"REMOTE_PORT":  "2222",
		},
	}
	app := models.Application{Name: "mono", ProjectName: "ops"}
	workDir := filepath.Join("/tmp", "ci", "ops", "mono")

	got := mergeRunVariables(config, script, app, workDir, models.Variables{
		"PROJECT_NAME": "runtime-project",
	})

	if got["WORK_DIR"] != workDir {
		t.Fatalf("WORK_DIR got %q, want %q", got["WORK_DIR"], workDir)
	}
	if got["MODULE_WORK_DIR"] != filepath.Join(workDir, "services/api") {
		t.Fatalf("MODULE_WORK_DIR got %q", got["MODULE_WORK_DIR"])
	}
	if got["PROJECT_NAME"] != "runtime-project" {
		t.Fatalf("PROJECT_NAME got %q", got["PROJECT_NAME"])
	}
	if got["REMOTE_PORT"] != "22" {
		t.Fatalf("REMOTE_PORT got %q", got["REMOTE_PORT"])
	}
}

func TestArtifactBuildVariablesMatchCIRunVariables(t *testing.T) {
	config := models.CIConfig{
		ReleaseBranch: "release",
		ModuleName:    "api",
		ModulePath:    "services/api",
		Variables:     models.Variables{"PORT": "8080"},
	}
	script := models.ReleaseScript{DefaultVariables: models.Variables{"PORT": "80", "TARGET": "default"}}
	app := models.Application{Name: "service", ProjectName: "ops"}
	handler := &ArtifactBuildHandler{workDir: "/tmp/ci"}
	runtime := models.Variables{"TARGET": "production"}

	got := handler.artifactBuildVariables(config, script, app, runtime)
	want := mergeRunVariables(config, script, app, filepath.Join("/tmp/ci", "ops", "service"), runtime)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
}

func TestArtifactSyncItemFromImage(t *testing.T) {
	pushedAt := time.Date(2026, 6, 13, 12, 0, 0, 0, time.UTC)
	got := artifactSyncItemFromImage(registry.Image{
		Repository: "bs_code/bs-blade-auth",
		Tag:        "26.0613",
		Digest:     "sha256:abc",
		MediaType:  "application/vnd.oci.image.manifest.v1+json",
		SizeBytes:  1024,
		PushedAt:   &pushedAt,
	})

	want := models.ArtifactSyncItem{
		Repository: "bs_code/bs-blade-auth",
		Tag:        "26.0613",
		Digest:     "sha256:abc",
		MediaType:  "application/vnd.oci.image.manifest.v1+json",
		SizeBytes:  1024,
		PushedAt:   &pushedAt,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
}

func TestNextArtifactImageTag(t *testing.T) {
	tests := []struct {
		name   string
		prefix string
		tags   []string
		want   string
	}{
		{
			name:   "starts from zero when no tags",
			prefix: "26.0616",
			want:   "26.0616.0",
		},
		{
			name:   "increments highest numeric suffix",
			prefix: "26.0616",
			tags:   []string{"26.0616.1", "26.0616.6", "26.0616.2"},
			want:   "26.0616.7",
		},
		{
			name:   "plain date counts as zero",
			prefix: "26.0616",
			tags:   []string{"26.0616"},
			want:   "26.0616.1",
		},
		{
			name:   "ignores other dates and non numeric suffixes",
			prefix: "26.0616",
			tags:   []string{"26.0615.9", "26.0616-beta", "26.0616.rc1", "26.0616.3"},
			want:   "26.0616.4",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := nextArtifactImageTag(tt.prefix, tt.tags); got != tt.want {
				t.Fatalf("nextArtifactImageTag() = %q, want %q", got, tt.want)
			}
		})
	}
}
