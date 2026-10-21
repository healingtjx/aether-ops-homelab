package dockerdeploy

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func TestRemoteLogLines(t *testing.T) {
	var got []string
	l := remoteLogLines{log: func(format string, args ...any) {
		got = append(got, fmt.Sprintf(format, args...))
	}}
	l.add("Pull")
	l.add("ing\rDownloaded\r\n\n启动")
	if want := []string{"Pulling", "Downloaded"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("live output = %v, want %v", got, want)
	}
	l.flush()
	if want := []string{"Pulling", "Downloaded", "启动"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("final output = %v, want %v", got, want)
	}
	l.add(strings.Repeat("x", 16384))
	if l.pending != "" || len(got) != 4 {
		t.Fatal("unterminated output was not flushed at buffer limit")
	}
}

func TestRemoteOutputCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	w := remoteOutput{ctx: ctx, chunks: make(chan string)}
	if n, err := w.Write([]byte("progress")); n != 0 || err != context.Canceled {
		t.Fatalf("Write = %d, %v", n, err)
	}
}
