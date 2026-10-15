package gitops

import (
	"bufio"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type PullResult struct {
	Status  string
	Message string
}

type LogFunc func(format string, args ...any)

const gitTimeout = 5 * time.Minute

type PullOptions struct {
	GitPath string
}

func Pull(ctx context.Context, gitURL, branch, targetDir string) (PullResult, error) {
	return PullWithLog(ctx, gitURL, branch, targetDir, nil)
}

func PullWithLog(ctx context.Context, gitURL, branch, targetDir string, log LogFunc) (PullResult, error) {
	return PullWithOptions(ctx, gitURL, branch, targetDir, PullOptions{}, log)
}

func PullWithOptions(ctx context.Context, gitURL, branch, targetDir string, opts PullOptions, log LogFunc) (PullResult, error) {
	gitPath := strings.TrimSpace(opts.GitPath)
	if gitPath == "" {
		gitPath = "git"
	}
	gitCmd, err := exec.LookPath(gitPath)
	if err != nil {
		return PullResult{Status: "failed", Message: "未找到 git 命令"}, nil
	}

	if err := os.MkdirAll(filepath.Dir(targetDir), 0755); err != nil {
		return PullResult{Status: "failed", Message: "创建目录失败: " + err.Error()}, nil
	}

	return pullWithEnv(ctx, gitCmd, gitURL, branch, targetDir, nil, log)
}

func pullWithEnv(ctx context.Context, gitCmd, gitURL, branch, targetDir string, env []string, log LogFunc) (PullResult, error) {
	safeDir := safeDirectory(targetDir)
	if dirExists(filepath.Join(targetDir, ".git")) {
		currentURL, err := runGit(ctx, gitCmd, targetDir, env, safeDir, "remote", "get-url", "origin")
		if err != nil {
			if _, err := runGitLog(ctx, gitCmd, targetDir, env, log, safeDir, "remote", "add", "origin", gitURL); err != nil {
				return PullResult{Status: "failed", Message: "添加 remote origin 失败"}, nil
			}
		} else if strings.TrimSpace(currentURL) != gitURL {
			if _, err := runGitLog(ctx, gitCmd, targetDir, env, log, safeDir, "remote", "set-url", "origin", gitURL); err != nil {
				return PullResult{Status: "failed", Message: "更新 remote URL 失败"}, nil
			}
		}

		runGit(ctx, gitCmd, targetDir, env, safeDir, "remote", "set-branches", "origin", "*")

		if out, err := runGitLog(ctx, gitCmd, targetDir, env, log, safeDir, "fetch", "origin"); err != nil {
			return PullResult{Status: "failed", Message: "git fetch 失败: " + firstLine(out)}, nil
		}

		runGitLog(ctx, gitCmd, targetDir, env, log, safeDir, "checkout", "--", ".")
		runGitLog(ctx, gitCmd, targetDir, env, log, safeDir, "clean", "-fd")

		if out, err := runGitLog(ctx, gitCmd, targetDir, env, log, safeDir, "checkout", branch); err != nil {
			if out2, err2 := runGitLog(ctx, gitCmd, targetDir, env, log, safeDir, "checkout", "-b", branch, "origin/"+branch); err2 != nil {
				return PullResult{Status: "failed", Message: "git checkout 失败: " + firstLine(out) + "; " + firstLine(out2)}, nil
			}
		}

		if out, err := runGitLog(ctx, gitCmd, targetDir, env, log, safeDir, "reset", "--hard", "origin/"+branch); err != nil {
			return PullResult{Status: "failed", Message: "git reset 失败: " + firstLine(out)}, nil
		}
		runGitLog(ctx, gitCmd, targetDir, env, log, safeDir, "clean", "-fd")
	} else {
		if out, err := runGitLog(ctx, gitCmd, "", env, log, "", "clone", "-b", branch, gitURL, targetDir); err != nil {
			return PullResult{Status: "failed", Message: "git clone 失败: " + firstLine(out)}, nil
		}
	}
	return PullResult{Status: "success", Message: "拉取成功"}, nil
}

func runGit(ctx context.Context, gitCmd, dir string, env []string, safeDir string, args ...string) (string, error) {
	gitCtx, cancel := context.WithTimeout(ctx, gitTimeout)
	defer cancel()

	cmd := exec.CommandContext(gitCtx, gitCmd, gitArgs(safeDir, args)...)
	if dir != "" {
		cmd.Dir = dir
	}
	cmd.Env = append(os.Environ(), env...)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func runGitLog(ctx context.Context, gitCmd, dir string, env []string, log LogFunc, safeDir string, args ...string) (string, error) {
	gitCtx, cancel := context.WithTimeout(ctx, gitTimeout)
	defer cancel()

	cmd := exec.CommandContext(gitCtx, gitCmd, gitArgs(safeDir, args)...)
	if dir != "" {
		cmd.Dir = dir
	}
	cmd.Env = append(os.Environ(), env...)

	stdout, _ := cmd.StdoutPipe()
	stderr, _ := cmd.StderrPipe()

	if err := cmd.Start(); err != nil {
		return "", err
	}

	var buf strings.Builder
	scannerOut := bufio.NewScanner(stdout)
	scannerErr := bufio.NewScanner(stderr)

	go func() {
		for scannerErr.Scan() {
			line := scannerErr.Text()
			buf.WriteString(line)
			buf.WriteByte('\n')
			if log != nil {
				log("%s", line)
			}
		}
	}()

	for scannerOut.Scan() {
		line := scannerOut.Text()
		buf.WriteString(line)
		buf.WriteByte('\n')
		if log != nil {
			log("%s", line)
		}
	}

	err := cmd.Wait()
	return buf.String(), err
}

func safeDirectory(path string) string {
	abs, err := filepath.Abs(path)
	if err != nil {
		return path
	}
	return abs
}

func gitArgs(safeDir string, args []string) []string {
	if strings.TrimSpace(safeDir) == "" {
		return args
	}
	withConfig := make([]string, 0, len(args)+2)
	withConfig = append(withConfig, "-c", "safe.directory="+safeDir)
	withConfig = append(withConfig, args...)
	return withConfig
}

func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func firstLine(s string) string {
	before, _, found := strings.Cut(s, "\n")
	if found {
		return strings.TrimSpace(before)
	}
	return strings.TrimSpace(s)
}
