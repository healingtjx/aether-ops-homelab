package handlers

import (
	"reflect"
	"testing"

	"aether_ops/internal/models"
)

func TestGroupDockerDeployments(t *testing.T) {
	if got := groupDockerDeployments(nil); len(got) != 0 {
		t.Fatalf("empty mappings produced groups: %+v", got)
	}
	// Applications in the same project belong together, while distinct project
	// IDs must remain separate even when their display names match.
	mappings := []models.DockerDeployMapping{
		{ID: 1, ProjectID: 20, ProjectName: "项目", ApplicationID: 1},
		{ID: 2, ProjectID: 10, ProjectName: "项目", ApplicationID: 1},
		{ID: 3, ProjectID: 20, ProjectName: "项目", ApplicationID: 2},
	}
	want := []dockerDeploymentGroup{
		{ProjectID: 20, ProjectName: "项目", Mappings: []models.DockerDeployMapping{mappings[0], mappings[2]}},
		{ProjectID: 10, ProjectName: "项目", Mappings: []models.DockerDeployMapping{mappings[1]}},
	}
	if got := groupDockerDeployments(mappings); !reflect.DeepEqual(got, want) {
		t.Fatalf("groups = %+v, want %+v", got, want)
	}
}
