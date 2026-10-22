package handlers

import (
	"aether_ops/internal/models"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestDeploySettingsAuthentication(t *testing.T) {
	h := DockerDeploymentHandler{}
	old := models.DockerDeployMapping{SSHMode: "independent", DeploySSH: models.Variables{"DEPLOY_SSH_PASSWORD": "saved-secret"}}
	base := url.Values{"ssh_mode": {"independent"}, "ssh_host": {"bastion"}, "ssh_user": {"ubuntu"}, "ssh_port": {"22"}, "ssh_auth": {"password"}}
	parse := func(v url.Values) (models.DockerDeploySettings, error) {
		r := httptest.NewRequest("POST", "/", strings.NewReader(v.Encode()))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		return h.parseDeploySettings(r, old, dockerDeployMappingInput{})
	}
	s, err := parse(base)
	if err != nil || s.DeploySSH["DEPLOY_SSH_PASSWORD"] != "saved-secret" {
		t.Fatalf("preserve: %+v %v", s, err)
	}
	base.Set("ssh_auth", "key")
	base.Set("ssh_key_path", "/keys/deploy")
	s, err = parse(base)
	if err != nil || s.DeploySSH["DEPLOY_SSH_PASSWORD"] != "" || s.DeploySSH["DEPLOY_SSH_KEY_PATH"] != "/keys/deploy" {
		t.Fatalf("switch: %+v %v", s, err)
	}
	base.Set("ssh_host", "")
	if _, err = parse(base); err == nil {
		t.Fatal("incomplete independent SSH accepted")
	}
	base.Set("ssh_mode", "inherit")
	s, err = parse(base)
	if err != nil || len(s.DeploySSH) != 0 {
		t.Fatal("inherit retains credentials")
	}
}
func TestDeploySettingsScriptValidation(t *testing.T) {
	h := DockerDeploymentHandler{scriptsDir: "../../devops/deploy-scripts"}
	v := url.Values{"script_id": {"bastion-deploy.json"}, "script_param_key": {"DEPLOY_PROJECT", "DEPLOY_ENV"}, "script_param_value": {"tanliao", "prod"}}
	r := httptest.NewRequest("POST", "/", strings.NewReader(v.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	s, err := h.parseDeploySettings(r, models.DockerDeployMapping{}, dockerDeployMappingInput{serviceName: "pc-html"})
	if err != nil || s.ScriptParams["DEPLOY_PROJECT"] != "tanliao" {
		t.Fatalf("%+v %v", s, err)
	}
}
