package workspace

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestUnder(t *testing.T) {
	if !Under("/home/u/sub-2-api/a", "/home/u/sub-2-api") {
		t.Fatal("child")
	}
	if !Under("/home/u/sub-2-api", "/home/u/sub-2-api") {
		t.Fatal("self")
	}
	if Under("/home/u", "/home/u/sub-2-api") {
		t.Fatal("parent")
	}
	if Under("/home/u/other", "/home/u/sub-2-api") {
		t.Fatal("sibling")
	}
	if !Under("~/sub-2-api/x", "~/sub-2-api") {
		t.Fatal("tilde child")
	}
	if Under("/home/u/sub-2-api", "~/sub-2-api") {
		t.Fatal("mixed forms must not pretend to match")
	}
}

func TestCheckCommand(t *testing.T) {
	wd := "/home/huangyu/sub-2-api"
	if err := CheckCommand("git clone https://github.com/huangyu6572/sub2api.git .", wd); err != nil {
		t.Fatal(err)
	}
	if err := CheckCommand("ls -la && cat README.md", wd); err != nil {
		t.Fatal(err)
	}
	if err := CheckCommand("rm -rf * .[!.]*", wd); err != nil {
		t.Fatal(err)
	}
	if err := CheckCommand("ls /home/huangyu", wd); err != nil {
		t.Fatal("query other dirs must be allowed")
	}
	if err := CheckCommand("cat /etc/os-release", wd); err != nil {
		t.Fatal("read other dirs must be allowed")
	}
	if err := CheckCommand("find /tmp -maxdepth 1", wd); err != nil {
		t.Fatal(err)
	}
	if err := CheckCommand("rm /home/huangyu/foo", wd); err == nil {
		t.Fatal("rm outside must fail")
	}
	if err := CheckCommand("mkdir /tmp/x", wd); err == nil {
		t.Fatal("mkdir outside")
	}
	if err := CheckCommand("echo hi > /tmp/out", wd); err == nil {
		t.Fatal("redirect outside")
	}
	if err := CheckCommand("echo hi>/tmp/out", wd); err == nil {
		t.Fatal("redirect outside without space")
	}
	if err := CheckCommand("cd /tmp", wd); err == nil {
		t.Fatal("cd /tmp")
	}
	if err := CheckCommand("cd ..", wd); err == nil {
		t.Fatal("cd ..")
	}
	if err := CheckCommand("cat /usr/bin/git", wd); err != nil {
		t.Fatal("system bin allow")
	}
	if err := CheckCommand("ls /home/huangyu/sub-2-api/src", wd); err != nil {
		t.Fatal(err)
	}
	if err := CheckCommand("find /tmp -delete", wd); err == nil {
		t.Fatal("find -delete outside")
	}
	if err := CheckCommand("chmod 777 /tmp/x", wd); err == nil {
		t.Fatal("chmod outside")
	}
	if err := CheckCommand("mv /tmp/a ./b", wd); err == nil {
		t.Fatal("mv from outside")
	}
	if err := CheckCommand("touch /tmp/x", wd); err == nil {
		t.Fatal("touch outside")
	}
	if err := CheckCommand("cp /etc/os-release .", wd); err == nil {
		t.Fatal("cp from outside is not query")
	}
	if err := CheckCommand("awk '{print}' /etc/os-release", wd); err != nil {
		t.Fatal("awk read must be allowed")
	}
	if err := CheckCommand("tee /tmp/out", wd); err == nil {
		t.Fatal("tee outside")
	}
}

func TestConfine(t *testing.T) {
	wd, err := Confine("", "/opt/app")
	if err != nil || wd != "/opt/app" {
		t.Fatalf("%q %v", wd, err)
	}
	wd, err = Confine("/opt/app/svc", "/opt/app")
	if err != nil || wd != "/opt/app/svc" {
		t.Fatalf("%q %v", wd, err)
	}
	if _, err := Confine("/tmp", "/opt/app"); err == nil {
		t.Fatal("expected outside")
	}
}

func TestBindCdsFirst(t *testing.T) {
	got := Bind("pwd", "~/sub-2-api")
	if !strings.Contains(got, "base64 -d | bash -s") {
		t.Fatalf("got %q", got)
	}
	if strings.Contains(got, "cd -- '~/sub-2-api'") || strings.Contains(got, "mkdir -p -- '~/sub-2-api'") {
		t.Fatal("quoted tilde leaked into ssh argv")
	}
	const prefix = "printf '%s' "
	const suffix = " | base64 -d | bash -s"
	if !strings.HasPrefix(got, prefix) || !strings.HasSuffix(got, suffix) {
		t.Fatalf("wrap %q", got)
	}
	b64 := strings.TrimSuffix(strings.TrimPrefix(got, prefix), suffix)
	body, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		t.Fatal(err)
	}
	script := string(body)
	if !strings.Contains(script, `mkdir -p -- "$HOME/sub-2-api"`) {
		t.Fatalf("mkdir missing: %q", script)
	}
	if !strings.Contains(script, `cd -- "$HOME/sub-2-api"`) {
		t.Fatalf("cd missing: %q", script)
	}
}
