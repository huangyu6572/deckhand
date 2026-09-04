package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWrapBashDoesNotNeedLocalQuotes(t *testing.T) {
	got, err := wrapRemoteCommand("bash", "cd /tmp && pwd && echo $?")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "base64 -d | bash -s") {
		t.Fatalf("got %q", got)
	}
	if strings.Contains(got, "$?") || strings.Contains(got, "&&") {
		t.Fatalf("user script leaked into SSH argv: %q", got)
	}
}

func TestWrapPowershellEncoded(t *testing.T) {
	got, err := wrapRemoteCommand("powershell", `Write-Output $PWD`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(got, "powershell.exe -NoProfile -NonInteractive -EncodedCommand ") {
		t.Fatalf("got %q", got)
	}
	if strings.Contains(got, "$PWD") {
		t.Fatalf("script leaked: %q", got)
	}
}

func TestExtractBashC(t *testing.T) {
	s, warn := extractBashC([]string{"bash", "-c", "cd /opt && pwd"})
	if s != "cd /opt && pwd" {
		t.Fatalf("script %q", s)
	}
	if warn == "" {
		t.Fatal("expected rewrite warn")
	}
}

func TestLooksPowerShellMangled(t *testing.T) {
	if !looksPowerShellMangled("set") {
		t.Fatal("set")
	}
	if !looksPowerShellMangled("echo exit=True") {
		t.Fatal("True")
	}
	if looksPowerShellMangled("cd /tmp && pwd") {
		t.Fatal("false positive")
	}
}

func TestPrepareRunCommandScriptFile(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "x.sh")
	if err := os.WriteFile(p, []byte("#!/bin/bash\ncd /tmp && pwd\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	target, cmd, _, err := prepareRunCommand([]string{"dev-web"}, p, "")
	if err != nil {
		t.Fatal(err)
	}
	if target != "dev-web" {
		t.Fatal(target)
	}
	if !strings.Contains(cmd, "base64 -d | bash -s") {
		t.Fatalf("shebang should select bash wrap: %q", cmd)
	}
}

func TestPrepareRunCommandRejectsMangled(t *testing.T) {
	_, _, _, err := prepareRunCommand([]string{"t", "bash", "-c", "set"}, "", "")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestCmdShellRejected(t *testing.T) {
	_, err := wrapRemoteCommand("cmd", "dir")
	if err == nil {
		t.Fatal("expected error")
	}
}
