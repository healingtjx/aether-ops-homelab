package scriptexec

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExecWithOptionsInjectsEnv(t *testing.T) {
	var lines []string
	result, err := ExecWithOptions(
		context.Background(),
		`#!/usr/bin/env bash
echo "$AETHER_TEST_VALUE"
`,
		t.TempDir(),
		ExecOptions{Env: []string{"AETHER_TEST_VALUE=ok"}},
		func(format string, args ...any) {
			lines = append(lines, strings.TrimSpace(fmt.Sprintf(format, args...)))
		},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Status != "success" {
		t.Fatalf("status = %s, message = %s", result.Status, result.Message)
	}
	if len(lines) != 1 || lines[0] != "ok" {
		t.Fatalf("lines = %#v, want [ok]", lines)
	}
}

func TestExecWithOptionsInjectsNPMConfig(t *testing.T) {
	dir := t.TempDir()
	npmHome := filepath.Join(dir, "node")
	if err := os.MkdirAll(filepath.Join(npmHome, "bin"), 0755); err != nil {
		t.Fatalf("mkdir npm home: %v", err)
	}
	npmBin := filepath.Join(npmHome, "bin", "npm")
	if err := os.WriteFile(npmBin, []byte("#!/bin/sh\necho \"$@\"\n"), 0755); err != nil {
		t.Fatalf("write npm stub: %v", err)
	}

	var lines []string
	result, err := ExecWithOptions(
		context.Background(),
		`#!/usr/bin/env bash
npm install
`,
		dir,
		ExecOptions{NPM: NPMOptions{
			HomePath:       npmHome,
			UserConfigFile: filepath.Join(dir, ".npmrc"),
			CacheDirectory: filepath.Join(dir, ".npm-cache"),
		}},
		func(format string, args ...any) {
			lines = append(lines, strings.TrimSpace(fmt.Sprintf(format, args...)))
		},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Status != "success" {
		t.Fatalf("status = %s, message = %s", result.Status, result.Message)
	}
	if len(lines) != 1 {
		t.Fatalf("lines = %#v, want one line", lines)
	}

	wantUserConfig := "--userconfig " + filepath.Join(dir, ".npmrc")
	wantCache := "--cache " + filepath.Join(dir, ".npm-cache")
	for _, want := range []string{wantUserConfig, wantCache, "install"} {
		if !strings.Contains(lines[0], want) {
			t.Fatalf("line = %q, want it to contain %q", lines[0], want)
		}
	}
}

func TestExecWithOptionsMergesOutputBeforeReturning(t *testing.T) {
	var lines []string
	result, err := ExecWithOptions(context.Background(), `for i in {1..200}; do
 echo "out-$i"
 echo "err-$i" >&2
done
printf tail >&2
`, t.TempDir(), ExecOptions{}, func(format string, args ...any) {
		lines = append(lines, fmt.Sprintf(format, args...))
	})
	if err != nil || result.Status != "success" {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	if len(lines) != 401 {
		t.Fatalf("got %d lines", len(lines))
	}
	for i := 1; i <= 200; i++ {
		if lines[(i-1)*2] != fmt.Sprintf("out-%d", i) || lines[(i-1)*2+1] != fmt.Sprintf("err-%d", i) {
			t.Fatalf("wrong output order at %d", i)
		}
	}
	if lines[400] != "tail" {
		t.Fatalf("lost final stderr: %q", lines[400])
	}
}

func TestExecWithOptionsPreservesLongLogLine(t *testing.T) {
	want := strings.Repeat("x", 128*1024)
	var lines []string
	result, err := ExecWithOptions(context.Background(), "printf '%s\\n' '"+want+"' >&2\nprintf done\n", t.TempDir(), ExecOptions{}, func(format string, args ...any) {
		lines = append(lines, fmt.Sprintf(format, args...))
	})
	if err != nil || result.Status != "success" {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	if len(lines) != 2 || lines[0] != want || lines[1] != "done" {
		t.Fatalf("long log truncated or trailing output lost (lines=%d)", len(lines))
	}
}
