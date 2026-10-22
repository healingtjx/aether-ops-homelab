package handlers

import (
	"fmt"
	"net/http"
	"strings"

	"aether_ops/internal/dockerdeploy"
	"aether_ops/internal/models"
)

func (h *DockerDeploymentHandler) scriptOptions() []dockerdeploy.ScriptOption {
	options, err := dockerdeploy.ListScripts(h.scriptsDir)
	if err != nil {
		return []dockerdeploy.ScriptOption{{Name: "脚本目录读取失败", Error: err.Error()}}
	}
	return options
}

func (h *DockerDeploymentHandler) parseDeploySettings(r *http.Request, old models.DockerDeployMapping, input dockerDeployMappingInput) (models.DockerDeploySettings, error) {
	s := models.DockerDeploySettings{ScriptID: strings.TrimSpace(r.FormValue("script_id")), SSHMode: r.FormValue("ssh_mode"), DeploySSH: models.Variables{}, ScriptParams: models.Variables{}}
	if s.SSHMode == "" {
		s.SSHMode = "inherit"
	}
	if s.ScriptID != "" {
		script, err := dockerdeploy.LoadScript(h.scriptsDir, s.ScriptID)
		if err != nil {
			return s, err
		}
		s.ScriptParams, err = models.ParseVariables(r.Form["script_param_key"], r.Form["script_param_value"])
		if err != nil {
			return s, err
		}
		// Validate required fields now; artifact-derived values are supplied only at execution.
		_, err = script.Variables(s.ScriptParams, dockerdeploy.Options{RemoteDir: input.remoteDir, ComposeFile: input.composeFile, ServiceName: input.serviceName, ImageRef: "selected-artifact", ImageTag: "selected-tag", RegistryHost: "selected-registry", RegistryUsername: "registry-user"})
		if err != nil {
			return s, err
		}
	}
	if s.SSHMode == "inherit" {
		return s, nil
	}
	if s.SSHMode != "independent" {
		return s, fmt.Errorf("SSH 配置模式不合法")
	}
	s.DeploySSH["DEPLOY_SSH_HOST"] = strings.TrimSpace(r.FormValue("ssh_host"))
	s.DeploySSH["DEPLOY_SSH_PORT"] = strings.TrimSpace(r.FormValue("ssh_port"))
	s.DeploySSH["DEPLOY_SSH_USER"] = strings.TrimSpace(r.FormValue("ssh_user"))
	switch r.FormValue("ssh_auth") {
	case "password":
		password := r.FormValue("ssh_password")
		if password == "" && old.SSHMode == "independent" {
			password = old.DeploySSH["DEPLOY_SSH_PASSWORD"]
		}
		s.DeploySSH["DEPLOY_SSH_PASSWORD"] = password
	case "key":
		s.DeploySSH["DEPLOY_SSH_KEY_PATH"] = strings.TrimSpace(r.FormValue("ssh_key_path"))
		passphrase := r.FormValue("ssh_passphrase")
		if passphrase == "" && old.SSHMode == "independent" && old.DeploySSH["DEPLOY_SSH_KEY_PATH"] == s.DeploySSH["DEPLOY_SSH_KEY_PATH"] {
			passphrase = old.DeploySSH["DEPLOY_SSH_PASSPHRASE"]
		}
		s.DeploySSH["DEPLOY_SSH_PASSPHRASE"] = passphrase
	default:
		return s, fmt.Errorf("请选择 SSH 认证方式")
	}
	_, err := dockerdeploy.LoadSSHConfigForDeployment(s.DeploySSH)
	return s, err
}
