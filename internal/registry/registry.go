package registry

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"
	"sync"
	"time"

	"aether_ops/internal/aliyun"
)

const manifestAccept = "application/vnd.docker.distribution.manifest.v2+json, application/vnd.oci.image.manifest.v1+json, application/vnd.docker.distribution.manifest.list.v2+json, application/vnd.oci.image.index.v1+json"

type AuthType string

const (
	AuthBasic  AuthType = "basic"
	AuthBearer AuthType = "bearer"
)

type Config struct {
	RegistryURL string
	Namespace   string
	Provider    string
	AuthType    AuthType
	Username    string
	Credential  string
}

type Client struct {
	baseURL    string
	namespace  string
	provider   string
	authType   AuthType
	username   string
	credential string
	httpClient *http.Client
	tokenMu    sync.Mutex
	tokens     map[string]string
}

type Image struct {
	Repository string
	Tag        string
	Digest     string
	MediaType  string
	SizeBytes  int64
	PushedAt   *time.Time
}

type RepositoryError struct {
	Repository string
	Err        error
}

type ListResult struct {
	Images []Image
	Errors []RepositoryError
}

func NewClient(cfg Config) (*Client, error) {
	baseURL := strings.TrimSpace(cfg.RegistryURL)
	if baseURL == "" {
		return nil, fmt.Errorf("仓库地址不能为空")
	}
	if !strings.HasPrefix(baseURL, "http://") && !strings.HasPrefix(baseURL, "https://") {
		baseURL = "https://" + baseURL
	}
	u, err := url.Parse(baseURL)
	if err != nil {
		return nil, fmt.Errorf("仓库地址不合法: %w", err)
	}
	u.Path = strings.TrimRight(u.Path, "/")
	u.RawQuery = ""
	u.Fragment = ""

	// Empty ECR credentials use the execution host's AWS identity. Fetch a fresh
	// password for each client instead of persisting an expiring login token.
	if cfg.Provider == "aws_ecr" && strings.TrimSpace(cfg.Credential) == "" {
		password, err := ecrLoginPassword(u.Hostname())
		if err != nil {
			return nil, err
		}
		cfg.AuthType, cfg.Username, cfg.Credential = AuthBasic, "AWS", password
	}

	authType := cfg.AuthType
	if authType == "" {
		authType = AuthBasic
	}
	if authType != AuthBasic && authType != AuthBearer {
		return nil, fmt.Errorf("不支持的认证类型: %s", authType)
	}

	return &Client{
		baseURL:    strings.TrimRight(u.String(), "/"),
		namespace:  strings.Trim(strings.TrimSpace(cfg.Namespace), "/"),
		provider:   strings.TrimSpace(cfg.Provider),
		authType:   authType,
		username:   strings.TrimSpace(cfg.Username),
		credential: cfg.Credential,
		httpClient: &http.Client{Timeout: 30 * time.Second},
		tokens:     map[string]string{},
	}, nil
}

func (c *Client) ListImages(ctx context.Context) ([]Image, error) {
	repos, err := c.catalog(ctx)
	if err != nil {
		return nil, err
	}
	return c.listRepositoryImages(ctx, repos)
}

func (c *Client) ListRepositoryImages(ctx context.Context, repos []string) ([]Image, error) {
	result, err := c.ListRepositoryImagesResult(ctx, repos)
	if err != nil {
		return nil, err
	}
	return result.Images, nil
}

func (c *Client) ListRepositoryImagesResult(ctx context.Context, repos []string) (ListResult, error) {
	normalized := make([]string, 0, len(repos))
	for _, repo := range repos {
		repo = c.normalizeRepository(repo)
		if repo == "" {
			continue
		}
		normalized = append(normalized, repo)
	}
	return c.listRepositoryImagesBestEffort(ctx, normalized), nil
}

func (c *Client) GetImage(ctx context.Context, repository string, tag string) (Image, error) {
	repository = c.normalizeRepository(repository)
	tag = strings.TrimSpace(tag)
	if repository == "" || tag == "" {
		return Image{}, fmt.Errorf("镜像仓库和 tag 不能为空")
	}
	return c.manifest(ctx, repository, tag)
}

func (c *Client) normalizeRepository(repository string) string {
	repository = strings.Trim(strings.TrimSpace(repository), "/")
	if repository == "" {
		return ""
	}
	if c.namespace != "" && !strings.HasPrefix(repository, c.namespace+"/") {
		repository = c.namespace + "/" + repository
	}
	return repository
}

func (c *Client) listRepositoryImages(ctx context.Context, repos []string) ([]Image, error) {
	images := make([]Image, 0)
	for _, repo := range repos {
		tags, err := c.tags(ctx, repo)
		if err != nil {
			return nil, err
		}
		for _, tag := range tags {
			image, err := c.manifest(ctx, repo, tag)
			if err != nil {
				return nil, err
			}
			images = append(images, image)
		}
	}
	return images, nil
}

func (c *Client) listRepositoryImagesBestEffort(ctx context.Context, repos []string) ListResult {
	result := ListResult{Images: make([]Image, 0), Errors: make([]RepositoryError, 0)}
	for _, repo := range repos {
		tags, err := c.tags(ctx, repo)
		if err != nil {
			result.Errors = append(result.Errors, RepositoryError{Repository: repo, Err: err})
			continue
		}
		for _, tag := range tags {
			image, err := c.manifest(ctx, repo, tag)
			if err != nil {
				result.Errors = append(result.Errors, RepositoryError{Repository: repo + ":" + tag, Err: err})
				continue
			}
			result.Images = append(result.Images, image)
		}
	}
	return result
}

func (c *Client) DeleteManifest(ctx context.Context, repository string, digest string, tag string) error {
	if c.provider == "aliyun_acr" {
		region := aliyun.ExtractRegion(c.baseURL)
		if region == "" {
			return fmt.Errorf("无法从 registry URL 中提取 region: %s", c.baseURL)
		}
		repoPath := repository
		if c.namespace != "" {
			repoPath = c.namespace + "/" + strings.TrimPrefix(repository, c.namespace+"/")
		}
		return aliyun.DeleteRepoTag(ctx, region, repoPath, tag)
	}

	repository = strings.Trim(strings.TrimSpace(repository), "/")
	digest = strings.TrimSpace(digest)
	if repository == "" || digest == "" {
		return fmt.Errorf("镜像仓库和 digest 不能为空")
	}

	resp, err := c.do(ctx, http.MethodDelete, "/v2/"+path.Join(repository, "manifests", digest), "")
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted && resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		return responseError(resp)
	}
	return nil
}

func (c *Client) catalog(ctx context.Context) ([]string, error) {
	resp, err := c.do(ctx, http.MethodGet, "/v2/_catalog?n=10000", "")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, responseError(resp)
	}

	var body struct {
		Repositories []string `json:"repositories"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("解析仓库列表失败: %w", err)
	}
	return body.Repositories, nil
}

func (c *Client) tags(ctx context.Context, repository string) ([]string, error) {
	resp, err := c.do(ctx, http.MethodGet, "/v2/"+path.Join(repository, "tags/list"), "")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, responseError(resp)
	}

	var body struct {
		Tags []string `json:"tags"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("解析 tag 列表失败: %w", err)
	}
	return body.Tags, nil
}

func (c *Client) manifest(ctx context.Context, repository string, tag string) (Image, error) {
	data, digest, mediaTypeHeader, err := c.manifestData(ctx, repository, tag)
	if err != nil {
		return Image{}, err
	}

	size, pushedAt, mediaType, err := c.parseManifest(ctx, repository, data, mediaTypeHeader)
	if err != nil {
		return Image{}, err
	}

	return Image{
		Repository: repository,
		Tag:        tag,
		Digest:     digest,
		MediaType:  mediaType,
		SizeBytes:  size,
		PushedAt:   pushedAt,
	}, nil
}

func (c *Client) manifestData(ctx context.Context, repository string, ref string) ([]byte, string, string, error) {
	resp, err := c.do(ctx, http.MethodGet, "/v2/"+path.Join(repository, "manifests", ref), manifestAccept)
	if err != nil {
		return nil, "", "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, "", "", responseError(resp)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", "", fmt.Errorf("读取 manifest 失败: %w", err)
	}
	return data, resp.Header.Get("Docker-Content-Digest"), resp.Header.Get("Content-Type"), nil
}

func (c *Client) parseManifest(ctx context.Context, repository string, data []byte, mediaTypeHeader string) (int64, *time.Time, string, error) {
	var body struct {
		MediaType string `json:"mediaType"`
		Config    struct {
			Size int64 `json:"size"`
		} `json:"config"`
		Layers []struct {
			Size int64 `json:"size"`
		} `json:"layers"`
		Manifests []struct {
			MediaType   string            `json:"mediaType"`
			Digest      string            `json:"digest"`
			Size        int64             `json:"size"`
			Annotations map[string]string `json:"annotations"`
			Platform    struct {
				Architecture string `json:"architecture"`
				OS           string `json:"os"`
			} `json:"platform"`
		} `json:"manifests"`
		History []struct {
			V1Compatibility string `json:"v1Compatibility"`
		} `json:"history"`
	}
	if err := json.NewDecoder(bytes.NewReader(data)).Decode(&body); err != nil {
		return 0, nil, "", fmt.Errorf("解析 manifest 失败: %w", err)
	}

	size := body.Config.Size
	for _, layer := range body.Layers {
		size += layer.Size
	}
	if len(body.Manifests) > 0 {
		resolvedSize := int64(0)
		for _, manifest := range body.Manifests {
			if isAttestationManifest(manifest.MediaType, manifest.Annotations, manifest.Platform.OS, manifest.Platform.Architecture) {
				continue
			}
			if strings.TrimSpace(manifest.Digest) == "" {
				continue
			}
			childData, _, childMediaType, err := c.manifestData(ctx, repository, manifest.Digest)
			if err != nil {
				continue
			}
			childSize, _, _, err := c.parseManifest(ctx, repository, childData, childMediaType)
			if err != nil {
				continue
			}
			resolvedSize += childSize
		}
		if resolvedSize > 0 {
			size = resolvedSize
		} else {
			for _, manifest := range body.Manifests {
				size += manifest.Size
			}
		}
	}

	var pushedAt *time.Time
	for _, item := range body.History {
		var history struct {
			Created string `json:"created"`
		}
		if err := json.Unmarshal([]byte(item.V1Compatibility), &history); err != nil || history.Created == "" {
			continue
		}
		if t, err := time.Parse(time.RFC3339Nano, history.Created); err == nil {
			pushedAt = &t
			break
		}
	}

	mediaType := body.MediaType
	if mediaType == "" {
		mediaType = mediaTypeHeader
	}

	return size, pushedAt, mediaType, nil
}

func isAttestationManifest(mediaType string, annotations map[string]string, os string, architecture string) bool {
	if annotations["vnd.docker.reference.type"] == "attestation-manifest" {
		return true
	}
	if strings.Contains(mediaType, "attestation") {
		return true
	}
	return os == "unknown" && architecture == "unknown"
}

func (c *Client) do(ctx context.Context, method string, requestPath string, accept string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+requestPath, nil)
	if err != nil {
		return nil, err
	}
	if accept != "" {
		req.Header.Set("Accept", accept)
	}
	c.setAuth(req)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("请求镜像仓库失败: %w", err)
	}
	if resp.StatusCode != http.StatusUnauthorized {
		return resp, nil
	}

	challenge := resp.Header.Get("WWW-Authenticate")
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()

	token, err := c.tokenForChallenge(ctx, challenge)
	if err != nil {
		return nil, err
	}

	req, err = http.NewRequestWithContext(ctx, method, c.baseURL+requestPath, nil)
	if err != nil {
		return nil, err
	}
	if accept != "" {
		req.Header.Set("Accept", accept)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err = c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("请求镜像仓库失败: %w", err)
	}
	return resp, nil
}

func (c *Client) setAuth(req *http.Request) {
	if c.credential == "" {
		return
	}
	if c.authType == AuthBearer {
		req.Header.Set("Authorization", "Bearer "+c.credential)
		return
	}
	req.SetBasicAuth(c.username, c.credential)
}

func responseError(resp *http.Response) error {
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
	msg := strings.TrimSpace(string(data))
	if msg == "" {
		msg = resp.Status
	}
	return fmt.Errorf("镜像仓库返回 %s: %s", resp.Status, msg)
}

func (c *Client) tokenForChallenge(ctx context.Context, header string) (string, error) {
	challenge, err := parseBearerChallenge(header)
	if err != nil {
		return "", err
	}

	key := challenge.cacheKey()
	c.tokenMu.Lock()
	if token := c.tokens[key]; token != "" {
		c.tokenMu.Unlock()
		return token, nil
	}
	c.tokenMu.Unlock()

	token, err := c.fetchToken(ctx, challenge)
	if err != nil {
		return "", err
	}

	c.tokenMu.Lock()
	c.tokens[key] = token
	c.tokenMu.Unlock()
	return token, nil
}

type bearerChallenge struct {
	Realm   string
	Service string
	Scope   string
}

func (c bearerChallenge) cacheKey() string {
	return c.Realm + "\n" + c.Service + "\n" + c.Scope
}

func parseBearerChallenge(header string) (bearerChallenge, error) {
	header = strings.TrimSpace(header)
	if !strings.HasPrefix(strings.ToLower(header), "bearer ") {
		return bearerChallenge{}, fmt.Errorf("镜像仓库需要认证，但未返回 Bearer token challenge")
	}

	parts := strings.Split(header[len("Bearer "):], ",")
	values := map[string]string{}
	for _, part := range parts {
		key, value, ok := strings.Cut(strings.TrimSpace(part), "=")
		if !ok {
			continue
		}
		value = strings.TrimSpace(value)
		value = strings.Trim(value, `"`)
		values[strings.ToLower(strings.TrimSpace(key))] = value
	}

	challenge := bearerChallenge{
		Realm:   values["realm"],
		Service: values["service"],
		Scope:   values["scope"],
	}
	if challenge.Realm == "" {
		return bearerChallenge{}, fmt.Errorf("镜像仓库认证 challenge 缺少 realm")
	}
	return challenge, nil
}

func (c *Client) fetchToken(ctx context.Context, challenge bearerChallenge) (string, error) {
	u, err := url.Parse(challenge.Realm)
	if err != nil {
		return "", fmt.Errorf("认证地址不合法: %w", err)
	}
	q := u.Query()
	if challenge.Service != "" {
		q.Set("service", challenge.Service)
	}
	if challenge.Scope != "" {
		q.Set("scope", challenge.Scope)
	}
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return "", err
	}
	if c.username != "" || c.credential != "" {
		req.SetBasicAuth(c.username, c.credential)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("请求镜像仓库 token 失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", responseError(resp)
	}

	var body struct {
		Token       string `json:"token"`
		AccessToken string `json:"access_token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return "", fmt.Errorf("解析镜像仓库 token 失败: %w", err)
	}
	token := body.Token
	if token == "" {
		token = body.AccessToken
	}
	if token == "" {
		return "", fmt.Errorf("镜像仓库 token 响应为空")
	}
	return token, nil
}
