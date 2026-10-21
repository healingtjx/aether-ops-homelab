package scriptexec

import (
	"bufio"
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type ExecResult struct {
	Status  string
	Message string
}

type LogFunc func(format string, args ...any)

const execTimeout = 10 * time.Minute

type ExecOptions struct {
	Env     []string
	Timeout time.Duration
	Maven   MavenOptions
	NPM     NPMOptions
}

type MavenOptions struct {
	HomePath         string
	UserSettingsFile string
	LocalRepository  string
}

type NPMOptions struct {
	HomePath       string
	UserConfigFile string
	CacheDirectory string
}

func ExecWithLog(ctx context.Context, scriptContent string, workDir string, log LogFunc) (ExecResult, error) {
	return ExecWithOptions(ctx, scriptContent, workDir, ExecOptions{}, log)
}

func ExecWithOptions(ctx context.Context, scriptContent string, workDir string, opts ExecOptions, log LogFunc) (ExecResult, error) {
	if _, err := exec.LookPath("bash"); err != nil {
		return ExecResult{Status: "failed", Message: "未找到 bash 命令"}, nil
	}

	if err := os.MkdirAll(workDir, 0755); err != nil {
		return ExecResult{Status: "failed", Message: "创建工作目录失败: " + err.Error()}, nil
	}

	tmpFile, err := os.CreateTemp(workDir, ".aether_run_*.sh")
	if err != nil {
		return ExecResult{Status: "failed", Message: "创建临时脚本文件失败: " + err.Error()}, nil
	}
	tmpPath := tmpFile.Name()
	defer os.Remove(tmpPath)

	if _, err := tmpFile.WriteString(scriptContent); err != nil {
		tmpFile.Close()
		return ExecResult{Status: "failed", Message: "写入脚本内容失败: " + err.Error()}, nil
	}
	tmpFile.Close()

	if err := os.Chmod(tmpPath, 0755); err != nil {
		return ExecResult{Status: "failed", Message: "设置脚本权限失败: " + err.Error()}, nil
	}

	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = execTimeout
	}
	execCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	env, cleanup, err := buildExecEnv(workDir, opts)
	if err != nil {
		return ExecResult{Status: "failed", Message: err.Error()}, nil
	}
	defer cleanup()

	cmd := exec.CommandContext(execCtx, "/bin/bash", tmpPath)
	cmd.Dir = workDir
	cmd.Env = env

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return ExecResult{Status: "failed", Message: "创建日志管道失败: " + err.Error()}, nil
	}
	// 合并输出到同一管道，日志回调仅由当前协程调用，避免并发写入 SSE 响应。
	cmd.Stderr = cmd.Stdout

	if err := cmd.Start(); err != nil {
		return ExecResult{Status: "failed", Message: "启动脚本失败: " + err.Error()}, nil
	}

	reader := bufio.NewReader(stdout)
	var readErr error
	for {
		line, err := reader.ReadString('\n')
		if len(line) > 0 && log != nil {
			log("%s", strings.TrimSuffix(strings.TrimSuffix(line, "\n"), "\r"))
		}
		if err != nil {
			if err != io.EOF {
				readErr = err
				cancel()
			}
			break
		}
	}

	err = cmd.Wait()
	if readErr != nil {
		return ExecResult{Status: "failed", Message: "读取脚本日志失败: " + readErr.Error()}, nil
	}
	if err != nil {
		if execCtx.Err() == context.DeadlineExceeded {
			return ExecResult{Status: "failed", Message: "脚本执行超时"}, nil
		}
		return ExecResult{Status: "failed", Message: "脚本执行失败: " + err.Error()}, nil
	}

	return ExecResult{Status: "success", Message: "部署成功"}, nil
}

func buildExecEnv(workDir string, opts ExecOptions) ([]string, func(), error) {
	env := append(os.Environ(), opts.Env...)
	cleanups := make([]func(), 0, 2)
	cleanup := func() {
		for i := len(cleanups) - 1; i >= 0; i-- {
			cleanups[i]()
		}
	}

	maven := normalizeMavenOptions(opts.Maven)
	if maven.HomePath != "" || maven.UserSettingsFile != "" || maven.LocalRepository != "" {
		var err error
		env, cleanups, err = appendMavenEnv(env, cleanups, workDir, maven)
		if err != nil {
			cleanup()
			return nil, nil, err
		}
	}

	npm := normalizeNPMOptions(opts.NPM)
	if npm.HomePath != "" || npm.UserConfigFile != "" || npm.CacheDirectory != "" {
		var err error
		env, cleanups, err = appendNPMEnv(env, cleanups, workDir, npm)
		if err != nil {
			cleanup()
			return nil, nil, err
		}
	}

	return env, cleanup, nil
}

func appendMavenEnv(env []string, cleanups []func(), workDir string, maven MavenOptions) ([]string, []func(), error) {
	mavenBin := ""
	mavenPathPrefix := ""
	if maven.HomePath != "" {
		mavenBin = filepath.Join(maven.HomePath, "bin", "mvn")
		if _, err := os.Stat(mavenBin); err != nil {
			return nil, nil, err
		}
		mavenPathPrefix = filepath.Join(maven.HomePath, "bin")
		env = append(env, "MAVEN_HOME="+maven.HomePath)
	} else {
		path, err := exec.LookPath("mvn")
		if err != nil {
			return nil, nil, err
		}
		mavenBin = path
	}

	wrapperDir, err := os.MkdirTemp(workDir, ".aether_tools_*")
	if err != nil {
		return nil, nil, err
	}
	cleanups = append(cleanups, func() {
		os.RemoveAll(wrapperDir)
	})

	wrapperPath := filepath.Join(wrapperDir, "mvn")
	wrapper := `#!/bin/sh
if [ -n "$AETHER_MAVEN_USER_SETTINGS_FILE" ]; then
  set -- --settings "$AETHER_MAVEN_USER_SETTINGS_FILE" "$@"
fi
if [ -n "$AETHER_MAVEN_LOCAL_REPOSITORY" ]; then
  set -- "-Dmaven.repo.local=$AETHER_MAVEN_LOCAL_REPOSITORY" "$@"
fi
exec "$AETHER_MAVEN_BIN" "$@"
`
	if err := os.WriteFile(wrapperPath, []byte(wrapper), 0755); err != nil {
		return nil, nil, err
	}

	pathValue := wrapperDir
	if mavenPathPrefix != "" {
		pathValue += string(os.PathListSeparator) + mavenPathPrefix
	}
	if currentPath := envValue(env, "PATH"); currentPath != "" {
		pathValue += string(os.PathListSeparator) + currentPath
	}

	env = append(env,
		"PATH="+pathValue,
		"AETHER_MAVEN_BIN="+mavenBin,
		"AETHER_MAVEN_USER_SETTINGS_FILE="+maven.UserSettingsFile,
		"AETHER_MAVEN_LOCAL_REPOSITORY="+maven.LocalRepository,
	)
	return env, cleanups, nil
}

func appendNPMEnv(env []string, cleanups []func(), workDir string, npm NPMOptions) ([]string, []func(), error) {
	npmBin := ""
	npmPathPrefix := ""
	if npm.HomePath != "" {
		npmBin = filepath.Join(npm.HomePath, "bin", "npm")
		if _, err := os.Stat(npmBin); err != nil {
			return nil, nil, err
		}
		npmPathPrefix = filepath.Join(npm.HomePath, "bin")
	} else {
		path, err := exec.LookPath("npm")
		if err != nil {
			return nil, nil, err
		}
		npmBin = path
	}

	wrapperDir, err := os.MkdirTemp(workDir, ".aether_tools_*")
	if err != nil {
		return nil, nil, err
	}
	cleanups = append(cleanups, func() {
		os.RemoveAll(wrapperDir)
	})

	wrapperPath := filepath.Join(wrapperDir, "npm")
	wrapper := `#!/bin/sh
if [ -n "$AETHER_NPM_USER_CONFIG_FILE" ]; then
  set -- --userconfig "$AETHER_NPM_USER_CONFIG_FILE" "$@"
fi
if [ -n "$AETHER_NPM_CACHE_DIRECTORY" ]; then
  set -- --cache "$AETHER_NPM_CACHE_DIRECTORY" "$@"
fi
exec "$AETHER_NPM_BIN" "$@"
`
	if err := os.WriteFile(wrapperPath, []byte(wrapper), 0755); err != nil {
		return nil, nil, err
	}

	pathValue := wrapperDir
	if npmPathPrefix != "" {
		pathValue += string(os.PathListSeparator) + npmPathPrefix
	}
	if currentPath := envValue(env, "PATH"); currentPath != "" {
		pathValue += string(os.PathListSeparator) + currentPath
	}

	env = append(env,
		"PATH="+pathValue,
		"AETHER_NPM_BIN="+npmBin,
		"AETHER_NPM_USER_CONFIG_FILE="+npm.UserConfigFile,
		"AETHER_NPM_CACHE_DIRECTORY="+npm.CacheDirectory,
	)
	return env, cleanups, nil
}

func normalizeMavenOptions(opts MavenOptions) MavenOptions {
	opts.HomePath = strings.TrimSpace(opts.HomePath)
	opts.UserSettingsFile = strings.TrimSpace(opts.UserSettingsFile)
	opts.LocalRepository = strings.TrimSpace(opts.LocalRepository)
	return opts
}

func normalizeNPMOptions(opts NPMOptions) NPMOptions {
	opts.HomePath = strings.TrimSpace(opts.HomePath)
	opts.UserConfigFile = strings.TrimSpace(opts.UserConfigFile)
	opts.CacheDirectory = strings.TrimSpace(opts.CacheDirectory)
	return opts
}

func envValue(env []string, name string) string {
	prefix := name + "="
	for i := len(env) - 1; i >= 0; i-- {
		if strings.HasPrefix(env[i], prefix) {
			return strings.TrimPrefix(env[i], prefix)
		}
	}
	return ""
}

func ResolveWorkDir(baseWorkDir string, projectName string, appName string) string {
	return filepath.Join(baseWorkDir, projectName, appName)
}
