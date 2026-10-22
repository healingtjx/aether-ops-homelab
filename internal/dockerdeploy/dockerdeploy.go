package dockerdeploy

import (
	"aether_ops/internal/registry"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
	"gopkg.in/yaml.v3"
)

type LogFunc func(format string, args ...any)

type SSHConfig struct {
	Host       string
	Port       int
	User       string
	Password   string
	KeyPath    string
	PrivateKey string
	Passphrase string
}

type Options struct {
	ScriptID           string
	ScriptsDir         string
	ScriptParams       map[string]string
	ImageTag           string
	SSH                SSHConfig
	RemoteDir          string
	ComposeFile        string
	ServiceName        string
	ImageRef           string
	RegistryProvider   string
	RegistryHost       string
	RegistryUsername   string
	RegistryCredential string
}

type Result struct {
	Status  string
	Message string
}

type remoteStepsFile struct {
	BeforeUpload []remoteStep `json:"before_upload"`
	AfterUpload  []remoteStep `json:"after_upload"`
	Restart      []remoteStep `json:"restart"`
}

type remoteStep struct {
	Name     string   `json:"name"`
	Commands []string `json:"commands"`
}

const deployTimeout = 30 * time.Minute

func LoadSSHConfigFromEnv() (SSHConfig, error) {
	values := loadDotEnvValues()
	get := func(primary string, fallback string) string {
		if v := strings.TrimSpace(os.Getenv(primary)); v != "" {
			return v
		}
		if v := strings.TrimSpace(values[primary]); v != "" {
			return v
		}
		if fallback != "" {
			if v := strings.TrimSpace(os.Getenv(fallback)); v != "" {
				return v
			}
			if v := strings.TrimSpace(values[fallback]); v != "" {
				return v
			}
		}
		return ""
	}

	port := 22
	if raw := get("AETHER_DEPLOY_SSH_PORT", "REMOTE_PORT"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed <= 0 {
			return SSHConfig{}, fmt.Errorf("SSH 端口不合法: %s", raw)
		}
		port = parsed
	}

	cfg := SSHConfig{
		Host:       get("AETHER_DEPLOY_SSH_HOST", "REMOTE_HOST"),
		Port:       port,
		User:       get("AETHER_DEPLOY_SSH_USER", "REMOTE_USER"),
		Password:   get("AETHER_DEPLOY_SSH_PASSWORD", "REMOTE_PASSWORD"),
		KeyPath:    get("AETHER_DEPLOY_SSH_KEY_PATH", "REMOTE_KEY"),
		PrivateKey: get("AETHER_DEPLOY_SSH_PRIVATE_KEY", "REMOTE_PRIVATE_KEY"),
		Passphrase: get("AETHER_DEPLOY_SSH_PASSPHRASE", "REMOTE_KEY_PASSPHRASE"),
	}
	if cfg.Host == "" {
		return SSHConfig{}, fmt.Errorf("缺少 AETHER_DEPLOY_SSH_HOST 或 REMOTE_HOST")
	}
	if cfg.User == "" {
		return SSHConfig{}, fmt.Errorf("缺少 AETHER_DEPLOY_SSH_USER 或 REMOTE_USER")
	}
	if cfg.KeyPath == "" && cfg.PrivateKey == "" && cfg.Password == "" {
		return SSHConfig{}, fmt.Errorf("缺少 AETHER_DEPLOY_SSH_KEY_PATH、AETHER_DEPLOY_SSH_PRIVATE_KEY 或 AETHER_DEPLOY_SSH_PASSWORD")
	}
	return cfg, nil
}

func Deploy(ctx context.Context, opts Options, log LogFunc) (Result, error) {
	opts.RemoteDir = strings.TrimSpace(opts.RemoteDir)
	opts.ComposeFile = strings.TrimSpace(opts.ComposeFile)
	opts.ServiceName = strings.TrimSpace(opts.ServiceName)
	opts.ImageRef = strings.TrimSpace(opts.ImageRef)
	opts.RegistryHost = strings.TrimSpace(opts.RegistryHost)
	opts.RegistryUsername = strings.TrimSpace(opts.RegistryUsername)
	if opts.ComposeFile == "" {
		opts.ComposeFile = "docker-compose.yml"
	}
	if (opts.ScriptID == "" && opts.RemoteDir == "") || opts.ServiceName == "" || opts.ImageRef == "" {
		return Result{Status: "failed", Message: "远端目录、service 名和镜像不能为空"}, nil
	}

	var selected *Script
	var compiled []remoteStep
	if opts.ScriptID != "" {
		script, err := LoadScript(opts.ScriptsDir, opts.ScriptID)
		if err != nil {
			return Result{Status: "failed", Message: err.Error()}, nil
		}
		compiled, err = script.compile(opts.ScriptParams, opts)
		if err != nil {
			return Result{Status: "failed", Message: err.Error()}, nil
		}
		selected = &script
	}
	execCtx, cancel := context.WithTimeout(ctx, deployTimeout)
	defer cancel()

	client, err := dialSSH(execCtx, opts.SSH)
	if err != nil {
		return Result{Status: "failed", Message: err.Error()}, nil
	}
	defer client.Close()

	if selected == nil || selected.login() {
		if err := dockerLogin(execCtx, client, opts, log); err != nil {
			return Result{Status: "failed", Message: err.Error()}, nil
		}
	}
	if selected != nil {
		logf(log, "发布脚本: %s (%s)，SSH 目标: %s@%s:%d", selected.Name, opts.ScriptID, opts.SSH.User, opts.SSH.Host, opts.SSH.Port)
		for _, step := range compiled {
			if len(step.Commands) == 0 {
				continue
			}
			logf(log, "==> %s", step.Name)
			if err := runSSHStreaming(execCtx, client, strings.Join(step.Commands, " && "), log); err != nil {
				return Result{Status: "failed", Message: fmt.Sprintf("执行步骤 %s 失败: %v", step.Name, err)}, nil
			}
		}
		return Result{Status: "success", Message: "远端步骤执行成功"}, nil
	}

	steps, stepsPath, hasSteps, err := loadRemoteStepsFile()
	if err != nil {
		return Result{Status: "failed", Message: err.Error()}, nil
	}
	if hasSteps {
		logf(log, "读取远端步骤配置: %s", stepsPath)
		if err := runRemoteSteps(execCtx, client, steps, opts, log); err != nil {
			return Result{Status: "failed", Message: err.Error()}, nil
		}
		return Result{Status: "success", Message: "远端步骤执行成功"}, nil
	}

	logf(log, "读取远端 compose: %s:%s/%s", opts.SSH.Host, opts.RemoteDir, opts.ComposeFile)
	composeData, err := runSSH(execCtx, client, "cd "+shellQuote(opts.RemoteDir)+" && cat "+shellQuote(opts.ComposeFile), nil)
	if err != nil {
		return Result{Status: "failed", Message: "读取 compose 失败: " + err.Error()}, nil
	}

	updated, oldImage, err := replaceComposeServiceImage(composeData, opts.ServiceName, opts.ImageRef)
	if err != nil {
		return Result{Status: "failed", Message: err.Error()}, nil
	}
	logf(log, "替换 service %s 镜像", opts.ServiceName)
	logf(log, "旧镜像: %s", oldImage)
	logf(log, "新镜像: %s", opts.ImageRef)

	tmpFile := fmt.Sprintf("/tmp/aether_ops_compose_%d.yml", time.Now().UnixNano())
	if _, err := runSSH(execCtx, client, "umask 077 && cat > "+shellQuote(tmpFile), bytes.NewReader(updated)); err != nil {
		return Result{Status: "failed", Message: "上传 compose 临时文件失败: " + err.Error()}, nil
	}

	backup := opts.ComposeFile + ".aether.bak." + time.Now().Format("20060102150405")
	cmd := strings.Join([]string{
		"cd " + shellQuote(opts.RemoteDir),
		"cp " + shellQuote(opts.ComposeFile) + " " + shellQuote(backup),
		"mv " + shellQuote(tmpFile) + " " + shellQuote(opts.ComposeFile),
		"docker compose -f " + shellQuote(opts.ComposeFile) + " up -d " + shellQuote(opts.ServiceName),
	}, " && ")
	logf(log, "执行远端发布: docker compose up -d %s", opts.ServiceName)
	err = runSSHStreaming(execCtx, client, cmd, log)
	if err != nil {
		return Result{Status: "failed", Message: "执行 docker compose 失败: " + err.Error()}, nil
	}

	return Result{Status: "success", Message: "产物发布成功"}, nil
}

func loadRemoteStepsFile() (remoteStepsFile, string, bool, error) {
	path := strings.TrimSpace(os.Getenv("AETHER_DEPLOY_STEPS_FILE"))
	if path == "" {
		path = "upload_remote_steps.json"
	}
	if !filepath.IsAbs(path) {
		wd, err := os.Getwd()
		if err != nil {
			return remoteStepsFile{}, path, false, fmt.Errorf("获取工作目录失败: %w", err)
		}
		path = filepath.Join(wd, path)
	}

	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return remoteStepsFile{}, path, false, nil
	}
	if err != nil {
		return remoteStepsFile{}, path, false, fmt.Errorf("读取远端步骤配置失败: %w", err)
	}

	var steps remoteStepsFile
	if err := json.Unmarshal(data, &steps); err != nil {
		return remoteStepsFile{}, path, false, fmt.Errorf("解析远端步骤配置失败: %w", err)
	}
	return steps, path, true, nil
}

func runRemoteSteps(ctx context.Context, client *ssh.Client, steps remoteStepsFile, opts Options, log LogFunc) error {
	vars := map[string]string{
		"REMOTE_DIR":     opts.RemoteDir,
		"COMPOSE_FILE":   opts.ComposeFile,
		"SERVICE_NAME":   opts.ServiceName,
		"IMAGE_REF":      opts.ImageRef,
		"REGISTRY_HOST":  opts.RegistryHost,
		"REGISTRY_USER":  opts.RegistryUsername,
		"REGISTRY_TOKEN": opts.RegistryCredential,
		"REMOTE_BIN":     os.Getenv("REMOTE_BIN"),
		"REMOTE_TMP":     os.Getenv("REMOTE_TMP"),
		"REMOTE_BIN_DIR": filepath.Dir(os.Getenv("REMOTE_BIN")),
	}
	for _, step := range append(append(steps.BeforeUpload, steps.AfterUpload...), steps.Restart...) {
		if strings.TrimSpace(step.Name) != "" {
			logf(log, "==> %s", step.Name)
		}
		commands := make([]string, 0, len(step.Commands))
		for _, command := range step.Commands {
			command = strings.TrimSpace(expandRemoteCommand(command, vars))
			if command != "" {
				commands = append(commands, command)
			}
		}
		if len(commands) == 0 {
			continue
		}
		err := runSSHStreaming(ctx, client, strings.Join(commands, " && "), log)
		if err != nil {
			if strings.TrimSpace(step.Name) != "" {
				return fmt.Errorf("执行远端步骤 %s 失败: %w", step.Name, err)
			}
			return fmt.Errorf("执行远端步骤失败: %w", err)
		}
	}
	return nil
}

func expandRemoteCommand(command string, vars map[string]string) string {
	return os.Expand(command, func(key string) string {
		if value, ok := vars[key]; ok {
			return value
		}
		return os.Getenv(key)
	})
}

func dockerLogin(ctx context.Context, client *ssh.Client, opts Options, log LogFunc) error {
	if opts.RegistryProvider == "aws_ecr" {
		logf(log, "通过 AWS CLI 获取本次发布的 ECR 登录凭据")
		password, err := registry.ECRLoginPassword(ctx, opts.RegistryHost)
		if err != nil {
			return err
		}
		opts.RegistryUsername, opts.RegistryCredential = "AWS", password
	}
	if opts.RegistryHost == "" || opts.RegistryUsername == "" || opts.RegistryCredential == "" {
		return nil
	}
	logf(log, "登录镜像仓库: %s", opts.RegistryHost)
	cmd := "docker login " + shellQuote(opts.RegistryHost) + " -u " + shellQuote(opts.RegistryUsername) + " --password-stdin"
	output, err := runSSH(ctx, client, cmd, strings.NewReader(opts.RegistryCredential))
	if len(strings.TrimSpace(string(output))) > 0 {
		for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
			logf(log, "%s", line)
		}
	}
	if err != nil {
		return fmt.Errorf("登录镜像仓库失败: %w", err)
	}
	return nil
}

func replaceComposeServiceImage(data []byte, serviceName string, imageRef string) ([]byte, string, error) {
	var root yaml.Node
	if err := yaml.Unmarshal(data, &root); err != nil {
		return nil, "", fmt.Errorf("解析 docker-compose.yml 失败: %w", err)
	}
	if len(root.Content) == 0 || root.Content[0].Kind != yaml.MappingNode {
		return nil, "", fmt.Errorf("docker-compose.yml 格式不合法")
	}

	services := mappingValue(root.Content[0], "services")
	if services == nil || services.Kind != yaml.MappingNode {
		return nil, "", fmt.Errorf("docker-compose.yml 缺少 services")
	}
	service := mappingValue(services, serviceName)
	if service == nil || service.Kind != yaml.MappingNode {
		return nil, "", fmt.Errorf("docker-compose.yml 中不存在 service: %s", serviceName)
	}
	image := mappingValue(service, "image")
	if image == nil || image.Kind != yaml.ScalarNode {
		return nil, "", fmt.Errorf("service %s 缺少 image 字段", serviceName)
	}
	oldImage := image.Value
	image.Value = imageRef

	var buf bytes.Buffer
	encoder := yaml.NewEncoder(&buf)
	encoder.SetIndent(2)
	if err := encoder.Encode(&root); err != nil {
		return nil, "", fmt.Errorf("生成 docker-compose.yml 失败: %w", err)
	}
	if err := encoder.Close(); err != nil {
		return nil, "", fmt.Errorf("生成 docker-compose.yml 失败: %w", err)
	}
	return buf.Bytes(), oldImage, nil
}

func mappingValue(node *yaml.Node, key string) *yaml.Node {
	for i := 0; i+1 < len(node.Content); i += 2 {
		if node.Content[i].Value == key {
			return node.Content[i+1]
		}
	}
	return nil
}

func dialSSH(ctx context.Context, cfg SSHConfig) (*ssh.Client, error) {
	auth, err := buildSSHAuthMethods(cfg)
	if err != nil {
		return nil, err
	}
	clientConfig := &ssh.ClientConfig{
		User:            cfg.User,
		Auth:            auth,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         15 * time.Second,
	}
	addr := fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)
	type dialResult struct {
		client *ssh.Client
		err    error
	}
	ch := make(chan dialResult, 1)
	go func() {
		client, err := ssh.Dial("tcp", addr, clientConfig)
		ch <- dialResult{client: client, err: err}
	}()
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case result := <-ch:
		if result.err != nil {
			return nil, fmt.Errorf("SSH 连接失败: %w", result.err)
		}
		return result.client, nil
	}
}

func buildSSHAuthMethods(cfg SSHConfig) ([]ssh.AuthMethod, error) {
	auth := make([]ssh.AuthMethod, 0, 2)
	privateKey := strings.TrimSpace(cfg.PrivateKey)
	if privateKey != "" {
		signer, err := parsePrivateKey([]byte(strings.ReplaceAll(privateKey, `\n`, "\n")), cfg.Passphrase)
		if err != nil {
			return nil, fmt.Errorf("解析 SSH 私钥失败: %w", err)
		}
		auth = append(auth, ssh.PublicKeys(signer))
	}

	keyPath := strings.TrimSpace(cfg.KeyPath)
	if keyPath != "" {
		keyData, err := os.ReadFile(expandHomePath(keyPath))
		if err != nil {
			return nil, fmt.Errorf("读取 SSH 私钥失败: %w", err)
		}
		signer, err := parsePrivateKey(keyData, cfg.Passphrase)
		if err != nil {
			return nil, fmt.Errorf("解析 SSH 私钥失败: %w", err)
		}
		auth = append(auth, ssh.PublicKeys(signer))
	}

	if cfg.Password != "" {
		auth = append(auth, ssh.Password(cfg.Password))
	}
	if len(auth) == 0 {
		return nil, fmt.Errorf("缺少 SSH 认证方式")
	}
	return auth, nil
}

func parsePrivateKey(keyData []byte, passphrase string) (ssh.Signer, error) {
	if strings.TrimSpace(passphrase) != "" {
		return ssh.ParsePrivateKeyWithPassphrase(keyData, []byte(passphrase))
	}
	return ssh.ParsePrivateKey(keyData)
}

func expandHomePath(path string) string {
	if path == "~" {
		if home, err := os.UserHomeDir(); err == nil {
			return home
		}
	}
	if strings.HasPrefix(path, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, strings.TrimPrefix(path, "~/"))
		}
	}
	return path
}

func runSSH(ctx context.Context, client *ssh.Client, command string, stdin io.Reader) ([]byte, error) {
	type runResult struct {
		output []byte
		err    error
	}
	ch := make(chan runResult, 1)
	go func() {
		session, err := client.NewSession()
		if err != nil {
			ch <- runResult{err: err}
			return
		}
		defer session.Close()
		if stdin != nil {
			session.Stdin = stdin
		}
		output, err := session.CombinedOutput(command)
		if err != nil {
			msg := strings.TrimSpace(string(output))
			if msg != "" {
				err = fmt.Errorf("%w: %s", err, msg)
			}
		}
		ch <- runResult{output: output, err: err}
	}()
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case result := <-ch:
		return result.output, result.err
	}
}

func loadDotEnvValues() map[string]string {
	path := strings.TrimSpace(os.Getenv("AETHER_ENV_FILE"))
	if path == "" {
		path = ".env"
	}
	if !filepath.IsAbs(path) {
		if wd, err := os.Getwd(); err == nil {
			path = filepath.Join(wd, path)
		}
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return map[string]string{}
	}
	values := map[string]string{}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "export ") {
			line = strings.TrimSpace(strings.TrimPrefix(line, "export "))
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		value = strings.Trim(value, `"'`)
		if key != "" {
			values[key] = value
		}
	}
	return values
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}

func logf(log LogFunc, format string, args ...any) {
	if log != nil {
		log(format, args...)
	}
}
