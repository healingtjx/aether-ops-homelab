package dockerdeploy

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReplaceComposeServiceImage(t *testing.T) {
	input := []byte(`
services:
  bmser-auth:
    image: registry.old/bs_code/bs-blade-auth:old
    restart: always
  other:
    image: registry.old/other:old
`)

	updated, oldImage, err := replaceComposeServiceImage(input, "bmser-auth", "registry.new/bs_code/bs-blade-auth:new")
	if err != nil {
		t.Fatalf("replaceComposeServiceImage() error = %v", err)
	}
	if oldImage != "registry.old/bs_code/bs-blade-auth:old" {
		t.Fatalf("oldImage = %q", oldImage)
	}
	output := string(updated)
	if !strings.Contains(output, "image: registry.new/bs_code/bs-blade-auth:new") {
		t.Fatalf("updated compose missing new image:\n%s", output)
	}
	if !strings.Contains(output, "image: registry.old/other:old") {
		t.Fatalf("updated compose changed unrelated service:\n%s", output)
	}
}

func TestReplaceComposeServiceImageMissingService(t *testing.T) {
	_, _, err := replaceComposeServiceImage([]byte("services:\n  api:\n    image: api:old\n"), "missing", "api:new")
	if err == nil || !strings.Contains(err.Error(), "missing") {
		t.Fatalf("expected missing service error, got %v", err)
	}
}

func TestLoadSSHConfigFromEnvFile(t *testing.T) {
	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")
	if err := os.WriteFile(envPath, []byte(`
AETHER_DEPLOY_SSH_HOST=10.0.0.1
AETHER_DEPLOY_SSH_PORT=2222
AETHER_DEPLOY_SSH_USER=deploy
AETHER_DEPLOY_SSH_KEY_PATH='~/.ssh/aether_ops_deploy'
`), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AETHER_ENV_FILE", envPath)
	t.Setenv("AETHER_DEPLOY_SSH_HOST", "")
	t.Setenv("AETHER_DEPLOY_SSH_PORT", "")
	t.Setenv("AETHER_DEPLOY_SSH_USER", "")
	t.Setenv("AETHER_DEPLOY_SSH_PASSWORD", "")
	t.Setenv("AETHER_DEPLOY_SSH_KEY_PATH", "")

	cfg, err := LoadSSHConfigFromEnv()
	if err != nil {
		t.Fatalf("LoadSSHConfigFromEnv() error = %v", err)
	}
	if cfg.Host != "10.0.0.1" || cfg.Port != 2222 || cfg.User != "deploy" || cfg.KeyPath != "~/.ssh/aether_ops_deploy" {
		t.Fatalf("unexpected config: %#v", cfg)
	}
}

func TestLoadSSHConfigFromEnvRequiresAuth(t *testing.T) {
	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")
	if err := os.WriteFile(envPath, []byte(`
AETHER_DEPLOY_SSH_HOST=10.0.0.1
AETHER_DEPLOY_SSH_USER=deploy
`), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AETHER_ENV_FILE", envPath)
	t.Setenv("AETHER_DEPLOY_SSH_PASSWORD", "")
	t.Setenv("AETHER_DEPLOY_SSH_KEY_PATH", "")
	t.Setenv("AETHER_DEPLOY_SSH_PRIVATE_KEY", "")

	_, err := LoadSSHConfigFromEnv()
	if err == nil || !strings.Contains(err.Error(), "AETHER_DEPLOY_SSH_KEY_PATH") {
		t.Fatalf("expected missing auth error, got %v", err)
	}
}
