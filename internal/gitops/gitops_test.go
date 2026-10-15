package gitops

import (
	"reflect"
	"testing"
)

func TestGitArgsAddsSafeDirectoryBeforeCommand(t *testing.T) {
	got := gitArgs("/data/repos/ci/beisi/bs-code", []string{"remote", "get-url", "origin"})
	want := []string{"-c", "safe.directory=/data/repos/ci/beisi/bs-code", "remote", "get-url", "origin"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("gitArgs() = %#v, want %#v", got, want)
	}
}

func TestGitArgsKeepsCloneCommandWhenSafeDirectoryIsEmpty(t *testing.T) {
	got := gitArgs("", []string{"clone", "-b", "main", "git@example.com:ops/app.git", "/tmp/app"})
	want := []string{"clone", "-b", "main", "git@example.com:ops/app.git", "/tmp/app"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("gitArgs() = %#v, want %#v", got, want)
	}
}
