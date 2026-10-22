package dockerdeploy

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Script is a trusted, operator-maintained remote command template.
// Placeholders are complete shell words, e.g. cd ${REMOTE_DIR}, without quotes.
type Script struct {
	Name          string            `json:"name"`
	RegistryLogin *bool             `json:"registry_login,omitempty"`
	Defaults      map[string]string `json:"defaults,omitempty"`
	RequiredVars  []string          `json:"required_vars,omitempty"`
	BeforeUpload  []remoteStep      `json:"before_upload"`
	AfterUpload   []remoteStep      `json:"after_upload"`
	Restart       []remoteStep      `json:"restart"`
}
type ScriptOption struct {
	ID         string
	Name       string
	Definition Script
	Error      string
}

var parameterName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
var placeholder = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)\}`)
var systemParameters = map[string]bool{"IMAGE_REF": true, "IMAGE_TAG": true, "SERVICE_NAME": true, "REMOTE_DIR": true, "COMPOSE_FILE": true, "REGISTRY_HOST": true, "REGISTRY_USER": true}

func LoadScript(dir, id string) (Script, error) {
	var s Script
	if id == "" || filepath.Base(id) != id || strings.ContainsAny(id, `/\\`) || filepath.Ext(id) != ".json" {
		return s, fmt.Errorf("发布脚本文件名不合法")
	}
	absolute, err := filepath.Abs(dir)
	if err != nil {
		return s, err
	}
	root, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return s, fmt.Errorf("读取脚本目录: %w", err)
	}
	path, err := filepath.EvalSymlinks(filepath.Join(root, id))
	if err != nil {
		return s, fmt.Errorf("读取发布脚本 %s: %w", id, err)
	}
	rel, err := filepath.Rel(root, path)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return s, fmt.Errorf("发布脚本不能位于配置目录之外")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return s, err
	}
	if err = json.Unmarshal(data, &s); err != nil {
		return s, fmt.Errorf("解析发布脚本 %s: %w", id, err)
	}
	if s.Name == "" {
		s.Name = id
	}
	for k := range s.Defaults {
		if !parameterName.MatchString(k) || systemParameters[k] {
			return s, fmt.Errorf("脚本默认参数 %s 不合法或为系统保留参数", k)
		}
	}
	for _, k := range s.RequiredVars {
		if !parameterName.MatchString(k) {
			return s, fmt.Errorf("必填参数名不合法: %s", k)
		}
	}
	count := 0
	for _, step := range s.steps() {
		for _, c := range step.Commands {
			if strings.TrimSpace(c) != "" {
				count++
			}
		}
	}
	if count == 0 {
		return s, fmt.Errorf("发布脚本没有可执行命令")
	}
	return s, nil
}
func ListScripts(dir string) ([]ScriptOption, error) {
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	result := []ScriptOption{}
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		s, err := LoadScript(dir, e.Name())
		o := ScriptOption{ID: e.Name(), Name: s.Name, Definition: s}
		if err != nil {
			o.Name = e.Name()
			o.Error = err.Error()
		}
		result = append(result, o)
	}
	return result, nil
}
func (s Script) steps() []remoteStep {
	result := append([]remoteStep{}, s.BeforeUpload...)
	result = append(result, s.AfterUpload...)
	return append(result, s.Restart...)
}
func (s Script) login() bool { return s.RegistryLogin == nil || *s.RegistryLogin }
func (s Script) Variables(params map[string]string, opts Options) (map[string]string, error) {
	vars := map[string]string{}
	for k, v := range s.Defaults {
		vars[k] = v
	}
	for k, v := range params {
		if !parameterName.MatchString(k) || systemParameters[k] {
			return nil, fmt.Errorf("参数 %s 不合法或为系统保留参数", k)
		}
		vars[k] = v
	}
	for k, v := range map[string]string{"IMAGE_REF": opts.ImageRef, "IMAGE_TAG": opts.ImageTag, "SERVICE_NAME": opts.ServiceName, "REMOTE_DIR": opts.RemoteDir, "COMPOSE_FILE": opts.ComposeFile, "REGISTRY_HOST": opts.RegistryHost, "REGISTRY_USER": opts.RegistryUsername} {
		vars[k] = v
	}
	for _, k := range s.RequiredVars {
		if strings.TrimSpace(vars[k]) == "" {
			return nil, fmt.Errorf("缺少发布脚本参数: %s", k)
		}
	}
	return vars, nil
}
func expandScriptCommand(command string, vars map[string]string) (string, error) {
	// A placeholder must occupy a complete, unquoted shell word. Reject unsafe
	// contexts instead of attempting shell parsing or double interpolation.
	quote := byte(0)
	escaped := false
	for i := 0; i < len(command); i++ {
		ch := command[i]
		if ch == '$' && i+1 < len(command) && command[i+1] == '{' {
			end := strings.IndexByte(command[i:], '}')
			if end < 0 {
				return "", fmt.Errorf("未闭合的脚本参数")
			}
			end += i
			if escaped || quote != 0 || (i > 0 && !strings.ContainsRune(" \t\n", rune(command[i-1]))) || (end+1 < len(command) && !strings.ContainsRune(" \t\n;|&)", rune(command[end+1]))) {
				return "", fmt.Errorf("脚本参数必须是独立且未加引号的 shell 参数: %s", command[i:end+1])
			}
			if !parameterName.MatchString(command[i+2 : end]) {
				return "", fmt.Errorf("脚本参数格式不合法")
			}
			i = end
			continue
		}
		if escaped {
			escaped = false
			continue
		}
		if ch == '\\' && quote != '\'' {
			escaped = true
			continue
		}
		if quote != 0 {
			if ch == quote {
				quote = 0
			}
		} else if ch == '\'' || ch == '"' {
			quote = ch
		}
	}
	var err error
	result := placeholder.ReplaceAllStringFunc(command, func(p string) string {
		k := p[2 : len(p)-1]
		v, ok := vars[k]
		if !ok {
			err = fmt.Errorf("未知发布脚本参数: %s", k)
		}
		if strings.ContainsRune(v, 0) {
			err = fmt.Errorf("发布参数不能包含 NUL")
		}
		return shellQuote(v)
	})
	return result, err
}

func (s Script) compile(params map[string]string, opts Options) ([]remoteStep, error) {
	vars, err := s.Variables(params, opts)
	if err != nil {
		return nil, err
	}
	result := []remoteStep{}
	for _, step := range s.steps() {
		next := remoteStep{Name: step.Name}
		for _, command := range step.Commands {
			if strings.TrimSpace(command) == "" {
				continue
			}
			c, err := expandScriptCommand(command, vars)
			if err != nil {
				return nil, err
			}
			next.Commands = append(next.Commands, c)
		}
		result = append(result, next)
	}
	return result, nil
}

func (o ScriptOption) JSON() string { b, _ := json.Marshal(o.Definition); return string(b) }
