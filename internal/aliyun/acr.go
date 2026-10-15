package aliyun

import (
	"context"
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"sort"
	"strings"
	"time"

	uuidpkg "github.com/google/uuid"
)

const (
	acrAPIVersion   = "2016-06-07"
	acrContentType  = "application/json"
	envAccessKeyID  = "ALIYUN_ACCESS_KEY_ID"
	envAccessSecret = "ALIYUN_ACCESS_KEY_SECRET"
)

var regionRegex = regexp.MustCompile(`\.([a-z]+-[a-z]+(?:-[0-9]+)?)\.`)

// ExtractRegion 从阿里云 ACR registry URL 中提取 region。
// crpi-xxx.ap-southeast-1.personal.cr.aliyuncs.com -> ap-southeast-1
// registry.cn-hangzhou.aliyuncs.com               -> cn-hangzhou
func ExtractRegion(registryURL string) string {
	host := strings.TrimSpace(registryURL)
	host = strings.TrimPrefix(host, "https://")
	host = strings.TrimPrefix(host, "http://")
	if i := strings.Index(host, "/"); i >= 0 {
		host = host[:i]
	}
	m := regionRegex.FindStringSubmatch(host)
	if len(m) >= 2 {
		return m[1]
	}
	return ""
}

// DeleteRepoTag 调用阿里云 ACR OpenAPI 删除指定 tag。
// repoPath 例如 "bs_code/bs-houseems"，会自动按第一个 "/" 拆为 namespace/repo。
func DeleteRepoTag(ctx context.Context, region, repoPath, tag string) error {
	accessKey := strings.TrimSpace(os.Getenv(envAccessKeyID))
	secret := strings.TrimSpace(os.Getenv(envAccessSecret))
	if accessKey == "" || secret == "" {
		return fmt.Errorf("缺少环境变量 %s / %s", envAccessKeyID, envAccessSecret)
	}
	if region == "" {
		return fmt.Errorf("region 不能为空")
	}
	ns, repo, ok := strings.Cut(strings.Trim(repoPath, "/"), "/")
	if !ok || ns == "" || repo == "" {
		return fmt.Errorf("repository %q 不符合 namespace/repo 格式", repoPath)
	}

	endpoint := fmt.Sprintf("https://cr.%s.aliyuncs.com", region)
	path := fmt.Sprintf("/repos/%s/%s/tags/%s",
		url.PathEscape(ns), url.PathEscape(repo), url.PathEscape(tag))

	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, endpoint+path, nil)
	if err != nil {
		return err
	}
	now := time.Now().UTC().Format(http.TimeFormat)
	nonce := uuidpkg.NewString()

	req.Header.Set("Accept", acrContentType)
	req.Header.Set("Content-Type", acrContentType)
	req.Header.Set("Date", now)
	req.Header.Set("Host", req.URL.Host)
	req.Header.Set("x-acs-signature-method", "HMAC-SHA1")
	req.Header.Set("x-acs-signature-version", "1.0")
	req.Header.Set("x-acs-signature-nonce", nonce)
	req.Header.Set("x-acs-version", acrAPIVersion)
	req.Header.Set("x-acs-region-id", region)

	signature := signROA(req, secret)
	req.Header.Set("Authorization", "acs "+accessKey+":"+signature)

	const maxRetries = 3
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			wait := time.Duration(attempt*3) * time.Second
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(wait):
			}
			// 重建请求（nonce/date 需刷新）
			req, err = http.NewRequestWithContext(ctx, http.MethodDelete, endpoint+path, nil)
			if err != nil {
				return err
			}
			now = time.Now().UTC().Format(http.TimeFormat)
			req.Header.Set("Accept", acrContentType)
			req.Header.Set("Content-Type", acrContentType)
			req.Header.Set("Date", now)
			req.Header.Set("Host", req.URL.Host)
			req.Header.Set("x-acs-signature-method", "HMAC-SHA1")
			req.Header.Set("x-acs-signature-version", "1.0")
			req.Header.Set("x-acs-signature-nonce", uuidpkg.NewString())
			req.Header.Set("x-acs-version", acrAPIVersion)
			req.Header.Set("x-acs-region-id", region)
			signature = signROA(req, secret)
			req.Header.Set("Authorization", "acs "+accessKey+":"+signature)
		}

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return fmt.Errorf("请求阿里云 ACR 失败: %w", err)
		}
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		resp.Body.Close()

		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			return nil
		}
		bodyStr := strings.TrimSpace(string(body))
		// 限流时重试
		if strings.Contains(bodyStr, "Throttling") && attempt < maxRetries {
			continue
		}
		return fmt.Errorf("阿里云 ACR 返回 %s: %s", resp.Status, bodyStr)
	}
	return fmt.Errorf("阿里云 ACR 限流重试 %d 次后仍失败", maxRetries)
}

// signROA 按阿里云 ROA 签名规则计算签名。
// StringToSign = HTTPMethod + "\n" + Accept + "\n" + Content-MD5 + "\n" + Content-Type + "\n" + Date + "\n" + CanonicalizedHeaders + CanonicalizedResource
func signROA(req *http.Request, secret string) string {
	var sb strings.Builder
	sb.WriteString(req.Method)
	sb.WriteString("\n")
	sb.WriteString(req.Header.Get("Accept"))
	sb.WriteString("\n")
	sb.WriteString(req.Header.Get("Content-MD5"))
	sb.WriteString("\n")
	sb.WriteString(req.Header.Get("Content-Type"))
	sb.WriteString("\n")
	sb.WriteString(req.Header.Get("Date"))
	sb.WriteString("\n")
	sb.WriteString(canonicalizedHeaders(req))
	sb.WriteString(canonicalizedResource(req))

	mac := hmac.New(sha1.New, []byte(secret))
	mac.Write([]byte(sb.String()))
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

func canonicalizedHeaders(req *http.Request) string {
	keys := make([]string, 0)
	for k := range req.Header {
		lk := strings.ToLower(k)
		if strings.HasPrefix(lk, "x-acs-") {
			keys = append(keys, lk)
		}
	}
	sort.Strings(keys)
	var sb strings.Builder
	for _, k := range keys {
		sb.WriteString(k)
		sb.WriteString(":")
		sb.WriteString(strings.TrimSpace(req.Header.Get(k)))
		sb.WriteString("\n")
	}
	return sb.String()
}

func canonicalizedResource(req *http.Request) string {
	path := req.URL.Path
	if req.URL.RawQuery == "" {
		return path
	}
	values, _ := url.ParseQuery(req.URL.RawQuery)
	keys := make([]string, 0, len(values))
	for k := range values {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var sb strings.Builder
	sb.WriteString(path)
	sb.WriteString("?")
	for i, k := range keys {
		if i > 0 {
			sb.WriteString("&")
		}
		v := values.Get(k)
		sb.WriteString(k)
		if v != "" {
			sb.WriteString("=")
			sb.WriteString(v)
		}
	}
	return sb.String()
}
