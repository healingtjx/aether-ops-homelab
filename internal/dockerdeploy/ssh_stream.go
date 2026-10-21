package dockerdeploy

import (
	"context"
	"fmt"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
)

type remoteOutput struct {
	ctx    context.Context
	chunks chan string
}

func (w *remoteOutput) Write(p []byte) (int, error) {
	select {
	case w.chunks <- string(p):
		return len(p), nil
	case <-w.ctx.Done():
		return 0, w.ctx.Err()
	}
}

type remoteLogLines struct {
	pending string
	log     LogFunc
}

func (l *remoteLogLines) add(chunk string) {
	l.pending += chunk
	for {
		i := strings.IndexAny(l.pending, "\r\n")
		if i < 0 {
			// Bound buffered output even for commands that never emit a newline.
			if len(l.pending) >= 16384 {
				l.flush()
			}
			return
		}
		line := strings.TrimSpace(l.pending[:i])
		l.pending = l.pending[i+1:]
		if line != "" {
			logf(l.log, "%s", line)
		}
	}
}

func (l *remoteLogLines) flush() {
	if line := strings.TrimSpace(l.pending); line != "" {
		logf(l.log, "%s", line)
	}
	l.pending = ""
}

// Deliver all callbacks on the caller goroutine: SSE and database log ordering
// must not race with completion or heartbeat messages.
func runSSHStreaming(ctx context.Context, client *ssh.Client, command string, log LogFunc) error {
	streamCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	w := &remoteOutput{ctx: streamCtx, chunks: make(chan string)}
	done := make(chan error, 1)
	go func() {
		session, err := client.NewSession()
		if err != nil {
			done <- err
			return
		}
		defer session.Close()
		session.Stdout = w
		session.Stderr = w
		done <- session.Run(command)
	}()
	lines := remoteLogLines{log: log}
	defer lines.flush()
	started := time.Now()
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case chunk := <-w.chunks:
			lines.add(chunk)
		case err := <-done:
			return err
		case <-ticker.C:
			lines.flush()
			logf(log, "远端命令仍在执行，已等待 %s（等待命令结束不代表容器未启动）", time.Since(started).Round(time.Second))
		case <-ctx.Done():
			cancel()
			// This deployment owns the SSH connection; close it to release any
			// blocked session creation or command wait.
			client.Close()
			return fmt.Errorf("等待远端命令结束超时或取消，容器实际状态需确认: %w", ctx.Err())
		}
	}
}
