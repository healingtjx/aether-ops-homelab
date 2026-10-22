package dockerdeploy

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestScriptCatalog(t *testing.T) {
	dir := t.TempDir()
	valid := `{"name":"Example","restart":[{"commands":["true"]}]}`
	os.WriteFile(filepath.Join(dir, "good.json"), []byte(valid), 0600)
	os.WriteFile(filepath.Join(dir, "bad.json"), []byte(`{`), 0600)
	outside := filepath.Join(t.TempDir(), "outside.json")
	os.WriteFile(outside, []byte(valid), 0600)
	if err := os.Symlink(outside, filepath.Join(dir, "link.json")); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"../outside.json", "/outside.json", "link.json", "bad.json", "missing.json"} {
		if _, err := LoadScript(dir, id); err == nil {
			t.Errorf("accepted %s", id)
		}
	}
	options, err := ListScripts(dir)
	if err != nil || len(options) != 3 {
		t.Fatalf("catalog: %v %v", options, err)
	}
	script, err := LoadScript(dir, "good.json")
	if err != nil || !script.login() {
		t.Fatalf("default login: %v", err)
	}
}
func TestScriptExpansionIsLiteral(t *testing.T) {
	value := "path with 'quotes'; $(printf injected)\nnext"
	command, err := expandScriptCommand("printf '%s' ${VALUE}", map[string]string{"VALUE": value})
	if err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command("sh", "-c", command).Output()
	if err != nil || string(out) != value {
		t.Fatalf("got %q: %v", out, err)
	}
	for _, c := range []string{`echo "${VALUE}"`, `echo '${VALUE}'`, `echo x${VALUE}`, `echo ${VALUE}x`, `echo ${MISSING}`, `echo ${VALUE:-x}`} {
		if _, err := expandScriptCommand(c, map[string]string{"VALUE": value}); err == nil {
			t.Errorf("accepted unsafe/unknown %q", c)
		}
	}
}
func TestBastionScriptContract(t *testing.T) {
	s, err := LoadScript("../../devops/deploy-scripts", "bastion-deploy.json")
	if err != nil {
		t.Fatal(err)
	}
	if s.login() {
		t.Fatal("bastion must skip Ops registry login")
	}
	opts := Options{ServiceName: "pc-html", ImageTag: "26.1022.1", ImageRef: "registry/pc:26.1022.1"}
	params := map[string]string{"DEPLOY_PROJECT": "tanliao", "DEPLOY_ENV": "prod"}
	steps, err := s.compile(params, opts)
	if err != nil {
		t.Fatal(err)
	}
	command := steps[0].Commands[0]
	if !strings.Contains(command, "--expected-image 'registry/pc:26.1022.1'") || !strings.Contains(command, "--tag '26.1022.1'") {
		t.Fatal(command)
	}
	params["IMAGE_TAG"] = "override"
	if _, err = s.compile(params, opts); err == nil {
		t.Fatal("allowed system override")
	}
	delete(params, "IMAGE_TAG")
	delete(params, "DEPLOY_ENV")
	if _, err = s.compile(params, opts); err == nil {
		t.Fatal("allowed missing environment")
	}
}
func TestSelectedScriptFailsBeforeSSH(t *testing.T) {
	result, err := Deploy(context.Background(), Options{ScriptID: "missing.json", ScriptsDir: t.TempDir(), ServiceName: "pc", ImageRef: "repo:v1"}, nil)
	if err != nil || result.Status != "failed" || !strings.Contains(result.Message, "读取发布脚本") {
		t.Fatalf("%+v %v", result, err)
	}
}
