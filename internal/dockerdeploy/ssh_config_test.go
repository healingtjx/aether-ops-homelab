package dockerdeploy

import "testing"

func TestDeploymentSSHDoesNotInheritGlobalCredentials(t *testing.T) {
	t.Setenv("AETHER_DEPLOY_SSH_PASSWORD", "unrelated-secret")
	_, err := LoadSSHConfigForDeployment(map[string]string{"DEPLOY_SSH_HOST": "160.191.53.106", "DEPLOY_SSH_USER": "root"})
	if err == nil {
		t.Fatal("explicit target must require its own credentials")
	}
}

func TestDeploymentSSHConfiguration(t *testing.T) {
	for _, port := range []string{"0", "65536", "bad"} {
		_, err := LoadSSHConfigForDeployment(map[string]string{"DEPLOY_SSH_HOST": "target", "DEPLOY_SSH_USER": "root", "DEPLOY_SSH_PORT": port, "DEPLOY_SSH_KEY_PATH": "/keys/target"})
		if err == nil {
			t.Fatalf("accepted invalid port %s", port)
		}
	}
	cfg, err := LoadSSHConfigForDeployment(map[string]string{"DEPLOY_SSH_HOST": "target", "DEPLOY_SSH_USER": "root", "DEPLOY_SSH_KEY_PATH": "/keys/target"})
	if err != nil || cfg.Host != "target" || cfg.User != "root" || cfg.Port != 22 || cfg.KeyPath != "/keys/target" {
		t.Fatalf("unexpected configuration: %+v, %v", cfg, err)
	}
}
