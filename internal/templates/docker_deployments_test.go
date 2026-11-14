package templates

import (
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"aether_ops/internal/models"
)

func TestDockerDeploymentsPageGroups(t *testing.T) {
	rdr, err := New(os.DirFS("../.."))
	if err != nil {
		t.Fatal(err)
	}
	type group struct {
		ProjectName string
		Mappings    []models.DockerDeployMapping
	}
	for _, tc := range []struct {
		name   string
		groups []group
		want   []string
	}{
		{name: "empty", want: []string{`colspan="8"`, "暂无发布映射"}},
		{
			name: "grouped",
			groups: []group{{ProjectName: "项目 A", Mappings: []models.DockerDeployMapping{
				{ID: 1, ComposeServiceName: "service-a"},
				{ID: 2, ComposeServiceName: "service-b"},
			}}},
			want: []string{"项目 A", "2 个发布映射", `aria-expanded="true"`, `id="docker-deployment-1" class="ci-config-module-row is-visible"`, `id="docker-deployment-2"`, "service-a", "service-b"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			rdr.Page(w, httptest.NewRequest("GET", "/docker-deployments", nil), "docker_deployments_page", map[string]any{"Groups": tc.groups})
			if w.Code != 200 {
				t.Fatalf("render failed: %s", w.Body.String())
			}
			for _, want := range tc.want {
				if !strings.Contains(w.Body.String(), want) {
					t.Errorf("render missing %q", want)
				}
			}
		})
	}
}

func TestDockerDeploymentResetButton(t *testing.T) {
	rdr, err := New(os.DirFS("../.."))
	if err != nil {
		t.Fatal(err)
	}
	for _, status := range []string{"running", "success", "failed", ""} {
		t.Run(status, func(t *testing.T) {
			w := httptest.NewRecorder()
			rdr.Partial(w, "docker_deployment_row", models.DockerDeployMapping{ID: 7, LastDeployStatus: &status})
			body := w.Body.String()
			if strings.Contains(body, `hx-post="/docker-deployments/7/reset"`) != (status == "running") {
				t.Fatalf("unexpected reset action for %q: %s", status, body)
			}
			if status == "running" && !strings.Contains(body, "不会终止远端命令") {
				t.Fatal("missing reset explanation")
			}
		})
	}
}
