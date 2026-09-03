package config

import (
	"context"
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveYAMLAndCOMAndUserHost(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "connections.yaml"), []byte(`
targets:
  dev-web:
    transport: ssh
    host: 192.168.1.20
    user: devops
    auth:
      type: ssh-agent
  board-01:
    transport: serial
    port: COM3
    baud_rate: 115200
`), 0o600)
	file, err := LoadConnections(dir)
	if err != nil {
		t.Fatal(err)
	}
	r := &Resolver{DataDir: dir, Settings: DefaultSettings(), File: file, AllowPublic: false}
	got, err := r.Resolve(context.Background(), "dev-web")
	if err != nil {
		t.Fatal(err)
	}
	if got.Host != "192.168.1.20" || got.User != "devops" {
		t.Fatalf("yaml: %+v", got)
	}
	com, err := r.Resolve(context.Background(), "COM3")
	if err != nil {
		t.Fatal(err)
	}
	if com.Transport != "serial" || com.SerialPort != "COM3" {
		t.Fatalf("com: %+v", com)
	}
	uh, err := r.Resolve(context.Background(), "user@192.168.0.10")
	if err != nil {
		t.Fatal(err)
	}
	if uh.User != "user" || uh.Host != "192.168.0.10" || !uh.Ephemeral {
		t.Fatalf("userhost: %+v", uh)
	}
	if !strings.HasPrefix(got.ConnKey, "devops|192.168.1.20|22|agent|") {
		t.Fatalf("connKey %q", got.ConnKey)
	}
	if strings.Contains(got.ConnKey, "|192.168.1.20|192.168.1.20") {
		t.Fatal("connKey should not use pinned_ip as a distinct field")
	}
	_, err = r.Resolve(context.Background(), "user@8.8.8.8")
	if err == nil {
		t.Fatal("expected SCOPE_NOT_INTRANET")
	}
}

func TestUnknownYAMLField(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "connections.yaml"), []byte("targets: {}\nunknown: 1\n"), 0o600)
	_, err := LoadConnections(dir)
	if err == nil {
		t.Fatal("expected CONFIG_INVALID")
	}
}

func TestConnKeyIncludesHostKeyFP(t *testing.T) {
	dir := t.TempDir()
	raw := make([]byte, 32)
	for i := range raw {
		raw[i] = byte(i)
	}
	b64 := base64.StdEncoding.EncodeToString(raw)
	os.MkdirAll(filepath.Join(dir, "ssh"), 0o700)
	os.WriteFile(filepath.Join(dir, "ssh", "known_hosts"), []byte("192.168.1.20 ssh-ed25519 "+b64+"\n"), 0o600)
	os.WriteFile(filepath.Join(dir, "connections.yaml"), []byte(`
targets:
  dev-web:
    transport: ssh
    host: 192.168.1.20
    user: devops
    auth:
      type: ssh-agent
`), 0o600)
	file, err := LoadConnections(dir)
	if err != nil {
		t.Fatal(err)
	}
	r := &Resolver{DataDir: dir, Settings: DefaultSettings(), File: file, AllowPublic: false}
	got, err := r.Resolve(context.Background(), "dev-web")
	if err != nil {
		t.Fatal(err)
	}
	fp := FingerprintSHA256(raw)
	if got.HostKeyFP != fp {
		t.Fatalf("fp %q want %q", got.HostKeyFP, fp)
	}
	if !strings.Contains(got.ConnKey, fp) {
		t.Fatalf("connKey missing fp: %q", got.ConnKey)
	}
}
