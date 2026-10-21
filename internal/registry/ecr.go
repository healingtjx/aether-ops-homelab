package registry

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

var ecrHostPattern = regexp.MustCompile(`^[0-9]{12}\.dkr\.ecr\.([a-z0-9-]+)\.amazonaws\.com(?:\.cn)?$`)

// Delete by tag: deleting a digest would also remove tags retained by cleanup.
func (c *Client) deleteECRTag(ctx context.Context, repository, tag string) error {
	u, err := url.Parse(c.baseURL)
	if err != nil {
		return fmt.Errorf("ECR 地址不合法: %w", err)
	}
	match := ecrHostPattern.FindStringSubmatch(u.Hostname())
	if match == nil {
		return fmt.Errorf("ECR 地址必须为私有仓库标准域名")
	}
	repository = c.normalizeRepository(repository)
	tag = strings.TrimSpace(tag)
	if repository == "" || tag == "" {
		return fmt.Errorf("ECR 镜像仓库和 tag 不能为空")
	}
	ids, err := json.Marshal([]map[string]string{{"imageTag": tag}})
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, "aws", "ecr", "batch-delete-image",
		"--region", match[1], "--registry-id", strings.SplitN(u.Hostname(), ".", 2)[0],
		"--repository-name", repository, "--image-ids", string(ids), "--output", "json").Output()
	if err != nil {
		if ctx.Err() != nil {
			return fmt.Errorf("ECR 删除已取消或超时: %w", ctx.Err())
		}
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			if detail := strings.TrimSpace(string(exitErr.Stderr)); detail != "" {
				return fmt.Errorf("ECR 删除 %s:%s 失败: %w; AWS CLI: %s", repository, tag, err, detail)
			}
		}
		return fmt.Errorf("ECR 删除失败，请检查服务器 AWS CLI 身份及 ecr:BatchDeleteImage 权限: %w", err)
	}
	var result struct {
		ImageIDs []struct {
			Tag string `json:"imageTag"`
		} `json:"imageIds"`
		Failures []struct {
			ImageID struct {
				Tag string `json:"imageTag"`
			} `json:"imageId"`
			Code string `json:"failureCode"`
		} `json:"failures"`
	}
	if err := json.Unmarshal(output, &result); err != nil {
		return fmt.Errorf("解析 ECR 删除结果失败: %w", err)
	}
	confirmed := false
	for _, failure := range result.Failures {
		if failure.Code != "ImageNotFound" || failure.ImageID.Tag != tag {
			return fmt.Errorf("ECR 删除 %s:%s 失败: %s", repository, tag, failure.Code)
		}
		confirmed = true
	}
	for _, id := range result.ImageIDs {
		if id.Tag == tag {
			confirmed = true
		}
	}
	if !confirmed {
		return fmt.Errorf("ECR 未确认目标 tag %s:%s 已删除", repository, tag)
	}
	return nil
}

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
