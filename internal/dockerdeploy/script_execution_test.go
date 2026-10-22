package dockerdeploy

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
)

func TestScriptExecutionSkipsLoginAndStopsOnFailure(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(fmt.Sprint(fail), func(t *testing.T) {
			_, key, err := ed25519.GenerateKey(rand.Reader)
			if err != nil {
				t.Fatal(err)
			}
			signer, err := ssh.NewSignerFromKey(key)
			if err != nil {
				t.Fatal(err)
			}
			cfg := &ssh.ServerConfig{PasswordCallback: func(_ ssh.ConnMetadata, _ []byte) (*ssh.Permissions, error) { return nil, nil }}
			cfg.AddHostKey(signer)
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			defer listener.Close()
			commands := make(chan []string, 1)
			go func() {
				c, err := listener.Accept()
				if err != nil {
					commands <- nil
					return
				}
				defer c.Close()
				conn, channels, requests, err := ssh.NewServerConn(c, cfg)
				if err != nil {
					commands <- nil
					return
				}
				defer conn.Close()
				go ssh.DiscardRequests(requests)
				var got []string
				for next := range channels {
					ch, reqs, err := next.Accept()
					if err != nil {
						break
					}
					for req := range reqs {
						if req.Type != "exec" {
							req.Reply(false, nil)
							continue
						}
						var payload struct{ Command string }
						ssh.Unmarshal(req.Payload, &payload)
						got = append(got, payload.Command)
						req.Reply(true, nil)
						fmt.Fprintln(ch, "step output")
						status := uint32(0)
						if fail {
							status = 1
						}
						ch.SendRequest("exit-status", false, ssh.Marshal(struct{ Status uint32 }{status}))
						ch.Close()
						break
					}
				}
				commands <- got
			}()
			dir := t.TempDir()
			os.WriteFile(filepath.Join(dir, "test.json"), []byte(`{"registry_login":false,"restart":[{"name":"first","commands":["echo ${IMAGE_TAG}"]},{"name":"second","commands":["echo done"]}]}`), 0600)
			addr := listener.Addr().(*net.TCPAddr)
			opts := Options{ScriptsDir: dir, ScriptID: "test.json", ServiceName: "pc", ImageRef: "repo:v1", ImageTag: "v1", RegistryProvider: "aws_ecr", RegistryHost: "not-an-ecr-host", SSH: SSHConfig{Host: "127.0.0.1", Port: addr.Port, User: "deploy", Password: "secret"}}
			var logs []string
			result, err := Deploy(context.Background(), opts, func(f string, a ...any) { logs = append(logs, fmt.Sprintf(f, a...)) })
			want := "success"
			n := 2
			if fail {
				want = "failed"
				n = 1
			}
			if err != nil || result.Status != want {
				t.Fatalf("%+v %v", result, err)
			}
			select {
			case got := <-commands:
				if len(got) != n || got[0] != "echo 'v1'" {
					t.Fatalf("commands: %v", got)
				}
			case <-time.After(time.Second * 3):
				t.Fatal("SSH server did not finish")
			}
			if !strings.Contains(strings.Join(logs, "\n"), "step output") {
				t.Fatal("missing streamed output")
			}
		})
	}
}
