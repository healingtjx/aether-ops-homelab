package registry

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestECRDeleteTag(t *testing.T) {
	for _, tt := range []struct {
		name, response, exit, wantError string
	}{
		{"success", `{"imageIds":[{"imageTag":"old"}],"failures":[]}`, "0", ""},
		{"missing", `{"failures":[{"imageId":{"imageTag":"old"},"failureCode":"ImageNotFound"}]}`, "0", ""},
		{"failure", `{"failures":[{"imageId":{"imageTag":"old"},"failureCode":"InvalidImageTag"}]}`, "0", "InvalidImageTag"},
		{"mixed", `{"imageIds":[{"imageTag":"old"}],"failures":[{"failureCode":"InvalidParameter"}]}`, "0", "InvalidParameter"},
		{"permission", `{}`, "1", "ecr:BatchDeleteImage"},
		{"service error", `{}`, "254", "AccessDeniedException"},
		{"invalid json", `invalid`, "0", "解析"},
		{"empty", `{}`, "0", "未确认"},
		{"wrong tag", `{"imageIds":[{"imageTag":"keep"}]}`, "0", "未确认"},
		{"wrong missing tag", `{"failures":[{"imageId":{"imageTag":"keep"},"failureCode":"ImageNotFound"}]}`, "0", "ImageNotFound"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			script := `#!/bin/sh
printf '%s\n' "$@" > "$ECR_TEST_ARGS"
printf '%s' "$ECR_TEST_RESPONSE"
if [ "$ECR_TEST_EXIT" = "254" ]; then
  printf '%s\n' 'An error occurred (AccessDeniedException) when calling the BatchDeleteImage operation: not authorized' >&2
fi
exit "$ECR_TEST_EXIT"
`
			if err := os.WriteFile(filepath.Join(dir, "aws"), []byte(script), 0700); err != nil {
				t.Fatal(err)
			}
			argsFile := filepath.Join(dir, "args")
			t.Setenv("PATH", dir)
			t.Setenv("ECR_TEST_ARGS", argsFile)
			t.Setenv("ECR_TEST_RESPONSE", tt.response)
			t.Setenv("ECR_TEST_EXIT", tt.exit)
			c := &Client{baseURL: "https://123456789012.dkr.ecr.cn-north-1.amazonaws.com.cn", provider: "aws_ecr", namespace: "team"}
			for _, input := range [][2]string{{"app", "sha256:must-not-delete"}, {"team/app", ""}} {
				err := c.DeleteManifest(context.Background(), input[0], input[1], "old")
				if tt.wantError == "" && err != nil || tt.wantError != "" && (err == nil || !strings.Contains(err.Error(), tt.wantError)) {
					t.Fatalf("unexpected error: %v", err)
				}
				args, err := os.ReadFile(argsFile)
				if err != nil {
					t.Fatal(err)
				}
				want := "ecr\nbatch-delete-image\n--region\ncn-north-1\n--registry-id\n123456789012\n--repository-name\nteam/app\n--image-ids\n[{\"imageTag\":\"old\"}]\n--output\njson\n"
				if string(args) != want {
					t.Fatalf("unexpected arguments: %s", args)
				}
			}
		})
	}
}

func TestECRDeleteValidationAndCancellation(t *testing.T) {
	c := &Client{baseURL: "https://123456789012.dkr.ecr.ap-northeast-1.amazonaws.com", provider: "aws_ecr"}
	for _, input := range [][2]string{{"", "old"}, {"app", ""}} {
		if err := c.DeleteManifest(context.Background(), input[0], "digest", input[1]); err == nil {
			t.Fatal("accepted empty repository or tag")
		}
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "aws"), []byte("#!/bin/sh\nexit 0\n"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	expired, cancelDeadline := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancelDeadline()
	for _, ctx := range []context.Context{canceled, expired} {
		if err := c.DeleteManifest(ctx, "app", "digest", "old"); !errors.Is(err, ctx.Err()) {
			t.Fatalf("expected context error, got %v", err)
		}
	}
	c.baseURL = "https://example.com"
	if err := c.DeleteManifest(context.Background(), "app", "digest", "old"); err == nil {
		t.Fatal("accepted invalid host")
	}
}

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
