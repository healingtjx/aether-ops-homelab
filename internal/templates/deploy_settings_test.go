package templates

import (
	"aether_ops/internal/dockerdeploy"
	"aether_ops/internal/models"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestDeployMappingRendersWithoutSecrets(t *testing.T) {
	r, err := New(os.DirFS("../.."))
	if err != nil {
		t.Fatal(err)
	}
	scripts, err := dockerdeploy.ListScripts("../../devops/deploy-scripts")
	if err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	r.Partial(w, "docker_deployment_edit_form", map[string]any{"Mapping": models.DockerDeployMapping{ID: 7, ScriptID: "bastion-deploy.json", SSHMode: "independent", DeploySSH: models.Variables{"DEPLOY_SSH_HOST": "bastion", "DEPLOY_SSH_PASSWORD": "must-not-render-secret", "DEPLOY_SSH_PASSPHRASE": "must-not-render-passphrase"}, ScriptParams: models.Variables{"DEPLOY_PROJECT": "tanliao", "DEPLOY_ENV": "prod"}}, "Scripts": scripts})
	body := w.Body.String()
	if w.Code != 200 || !strings.Contains(body, "通用跳板机发布") {
		t.Fatal(body)
	}
	if strings.Contains(body, "must-not-render") {
		t.Fatal("SSH secret leaked into HTML")
	}
}
