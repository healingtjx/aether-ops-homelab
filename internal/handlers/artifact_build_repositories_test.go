package handlers

import (
	"aether_ops/internal/models"
	"context"
	"testing"
)

func TestArtifactBuildRepositoryWithoutDeployment(t *testing.T) {
	h := &ArtifactBuildHandler{}
	repos, err := h.artifactBuildRepositories(context.Background(), models.CIConfig{Variables: models.Variables{"IMAGE_REPOSITORY": " tio-mg-pc-html "}}, 2)
	if err != nil || len(repos) != 1 || repos[0] != "tio-mg-pc-html" {
		t.Fatalf("repositories=%v error=%v", repos, err)
	}
}
