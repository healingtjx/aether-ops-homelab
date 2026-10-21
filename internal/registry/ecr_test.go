package registry

import (
	"os"
	"path/filepath"
	"testing"
)

func TestECRAutomaticCredentials(t *testing.T) {
	dir := t.TempDir()
	script := `#!/bin/sh
[ "$*" = "ecr get-login-password --region ap-northeast-1" ] || exit 2
printf 'test-password\n'
`
	if err := os.WriteFile(filepath.Join(dir, "aws"), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	c, err := NewClient(Config{RegistryURL: "https://319966842764.dkr.ecr.ap-northeast-1.amazonaws.com", Provider: "aws_ecr"})
	if err != nil {
		t.Fatal(err)
	}
	if c.username != "AWS" || c.credential != "test-password" || c.authType != AuthBasic {
		t.Fatal("incorrect ECR authentication")
	}
}
func TestECRRejectsInvalidHost(t *testing.T) {
	for _, host := range []string{"example.com", "public.ecr.aws", "319966842764.dkr.ecr.ap-northeast-1.amazonaws.com.evil.test"} {
		if _, err := ecrLoginPassword(host); err == nil {
			t.Errorf("accepted %s", host)
		}
	}
}
func TestECRExplicitCredentialsDoNotInvokeAWS(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	_, err := NewClient(Config{RegistryURL: "https://319966842764.dkr.ecr.ap-northeast-1.amazonaws.com", Provider: "aws_ecr", Username: "AWS", Credential: "explicit"})
	if err != nil {
		t.Fatal(err)
	}
}
