package dockerdeploy

import (
	"fmt"
	"strconv"
	"strings"
)

// LoadSSHConfigForDeployment uses explicit per-CI deployment credentials when
// present. Never inherit another server's credentials for an explicit target.
func LoadSSHConfigForDeployment(values map[string]string) (SSHConfig, error) {
	explicit := false
	for key := range values {
		if strings.HasPrefix(key, "DEPLOY_SSH_") {
			explicit = true
		}
	}
	if !explicit {
		return LoadSSHConfigFromEnv()
	}
	cfg := SSHConfig{
		Host:       strings.TrimSpace(values["DEPLOY_SSH_HOST"]),
		User:       strings.TrimSpace(values["DEPLOY_SSH_USER"]),
		Port:       22,
		Password:   values["DEPLOY_SSH_PASSWORD"],
		KeyPath:    strings.TrimSpace(values["DEPLOY_SSH_KEY_PATH"]),
		PrivateKey: values["DEPLOY_SSH_PRIVATE_KEY"],
		Passphrase: values["DEPLOY_SSH_PASSPHRASE"],
	}
	if raw := strings.TrimSpace(values["DEPLOY_SSH_PORT"]); raw != "" {
		port, err := strconv.Atoi(raw)
		if err != nil || port < 1 || port > 65535 {
			return SSHConfig{}, fmt.Errorf("DEPLOY_SSH_PORT 不合法")
		}
		cfg.Port = port
	}
	if cfg.Host == "" || cfg.User == "" {
		return SSHConfig{}, fmt.Errorf("请配置 DEPLOY_SSH_HOST 和 DEPLOY_SSH_USER")
	}
	if cfg.Password == "" && cfg.KeyPath == "" && cfg.PrivateKey == "" {
		return SSHConfig{}, fmt.Errorf("请配置该发布目标的 DEPLOY_SSH_KEY_PATH、DEPLOY_SSH_PRIVATE_KEY 或 DEPLOY_SSH_PASSWORD")
	}
	return cfg, nil
}
