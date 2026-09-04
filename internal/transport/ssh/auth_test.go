package sshx

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/crypto/ssh"

	"localaihub/internal/config"
	"localaihub/internal/wire"
)

func TestFormatAuthFailedIncludesKeysAndHint(t *testing.T) {
	msg := formatAuthFailed(
		&config.Resolved{User: "huangyu", Host: "124.222.36.172", Name: "cloud-172"},
		&authPlan{keyFPs: []string{"SHA256:abc (id_ed25519)"}},
		"ssh: unable to authenticate, attempted methods [none publickey]",
	)
	for _, want := range []string{
		"huangyu@124.222.36.172",
		"SHA256:abc (id_ed25519)",
		"unable to authenticate",
		"hub secret set cloud-172",
	} {
		if !strings.Contains(msg, want) {
			t.Fatalf("missing %q in %q", want, msg)
		}
	}
}

func TestMapSSHErrPreservesAuthDetail(t *testing.T) {
	err := mapSSHErr(
		errors.New("ssh: handshake failed: ssh: unable to authenticate, attempted methods [none publickey], no supported methods remain"),
		&config.Resolved{User: "u", Host: "h", Name: "t"},
		&authPlan{keyFPs: []string{"SHA256:x (key)"}},
	)
	if !wire.Is(err, "AUTH_FAILED") {
		t.Fatalf("code: %v", err)
	}
	if !strings.Contains(err.Error(), "SHA256:x (key)") {
		t.Fatalf("lost fingerprint: %v", err)
	}
	if err.Error() == "AUTH_FAILED: authentication failed" {
		t.Fatal("generic message swallowed ssh detail")
	}
}

func TestLoadSignerAndBuildAuthFromFile(t *testing.T) {
	dir := t.TempDir()
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	block, err := ssh.MarshalPrivateKey(priv, "test")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "id_ed25519")
	if err := os.WriteFile(path, pem.EncodeToMemory(block), 0o600); err != nil {
		t.Fatal(err)
	}
	s, err := loadSigner(path, "t")
	if err != nil {
		t.Fatal(err)
	}
	if s.PublicKey().Type() != ssh.KeyAlgoED25519 {
		t.Fatalf("type %s", s.PublicKey().Type())
	}
	plan, err := buildAuth(&config.Resolved{
		Name:    "t",
		User:    "u",
		Host:    "h",
		AuthRef: "key:" + path,
		KeyPath: path,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.methods) == 0 {
		t.Fatal("no auth methods")
	}
	if len(plan.keyFPs) == 0 {
		t.Fatal("no fingerprints")
	}
	if !strings.Contains(plan.keyFPs[0], path) && !strings.Contains(plan.keyFPs[0], "SHA256:") {
		t.Fatalf("fingerprint label %q", plan.keyFPs[0])
	}
}

func TestSamePath(t *testing.T) {
	if !samePath(`C:\Users\a\.ssh\id_ed25519`, `C:\Users\a\.ssh\id_ed25519`) {
		t.Fatal("expected equal")
	}
	if samePath(`C:\Users\a\.ssh\id_ed25519`, `C:\Users\a\.ssh\id_rsa`) {
		t.Fatal("expected different")
	}
}
