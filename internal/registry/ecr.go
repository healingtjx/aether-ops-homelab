package registry

import (
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

var ecrHostPattern = regexp.MustCompile(`^[0-9]{12}\.dkr\.ecr\.([a-z0-9-]+)\.amazonaws\.com(?:\.cn)?$`)

func ecrLoginPassword(host string) (string, error) {
	match := ecrHostPattern.FindStringSubmatch(host)
	if match == nil {
		return "", fmt.Errorf("ECR 地址必须为私有仓库标准域名")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, "aws", "ecr", "get-login-password", "--region", match[1]).Output()
	if err != nil {
		return "", fmt.Errorf("获取 ECR 登录凭据失败，请检查服务器 AWS CLI 和身份配置: %w", err)
	}
	password := strings.TrimSpace(string(output))
	if password == "" {
		return "", fmt.Errorf("AWS CLI 返回的 ECR 登录凭据为空")
	}
	return password, nil
}
