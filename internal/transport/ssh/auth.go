package sshx

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/crypto/ssh"

	"localaihub/internal/config"
	"localaihub/internal/secrets"
	"localaihub/internal/wire"
)

type authPlan struct {
	methods       []ssh.AuthMethod
	keyFPs        []string
	triedPassword bool
	triedAgent    bool
}

func buildAuth(t *config.Resolved) (*authPlan, error) {
	plan := &authPlan{}
	var signers []ssh.Signer
	var loadErrs []string

	addSigner := func(s ssh.Signer, label string) {
		if s == nil {
			return
		}
		fp := ssh.FingerprintSHA256(s.PublicKey())
		for _, have := range plan.keyFPs {
			if have == fp {
				return
			}
		}
		signers = append(signers, s)
		plan.keyFPs = append(plan.keyFPs, fmt.Sprintf("%s (%s)", fp, label))
	}

	addFile := func(path, label string, required bool) error {
		s, err := loadSigner(path, t.Name)
		if err != nil {
			if required {
				return err
			}
			if !os.IsNotExist(err) {
				loadErrs = append(loadErrs, label+": "+err.Error())
			}
			return nil
		}
		addSigner(s, label)
		return nil
	}

	switch {
	case strings.HasPrefix(t.AuthRef, "key:"):
		label := t.KeyPath
		if label == "" {
			label = "key_path"
		}
		if err := addFile(t.KeyPath, label, true); err != nil {
			loadErrs = append(loadErrs, err.Error())
		}
		for _, p := range defaultIdentityFiles() {
			if samePath(p, t.KeyPath) {
				continue
			}
			_ = addFile(p, p, false)
		}
		plan.addAgent(addSigner)
		plan.addPassword(t.Name)
	case strings.HasPrefix(t.AuthRef, "cred:"):
		name := strings.TrimPrefix(t.AuthRef, "cred:")
		if name == "" {
			name = t.Name
		}
		pw, err := secrets.Get(name)
		if err != nil || pw == "" {
			return nil, wire.E("AUTH_FAILED", "password not set; run hub secret set "+name)
		}
		plan.addPasswordMethods(pw)
		plan.addAgent(addSigner)
		for _, p := range defaultIdentityFiles() {
			_ = addFile(p, p, false)
		}
	default:
		plan.addAgent(addSigner)
		for _, p := range defaultIdentityFiles() {
			_ = addFile(p, p, false)
		}
		plan.addPassword(t.Name)
	}

	if len(signers) > 0 {
		plan.methods = append([]ssh.AuthMethod{ssh.PublicKeys(signers...)}, plan.methods...)
	}
	if len(plan.methods) == 0 {
		msg := "no authentication methods available"
		if !plan.triedAgent {
			msg += "; ssh-agent not available"
		}
		if len(loadErrs) > 0 {
			msg += "; " + strings.Join(loadErrs, "; ")
		}
		msg += "; run hub secret set " + t.Name + " if the host accepts a password"
		return nil, wire.E("AUTH_FAILED", msg)
	}
	return plan, nil
}

func (plan *authPlan) addAgent(add func(ssh.Signer, string)) {
	ag, err := sshAgent()
	if err != nil {
		return
	}
	plan.triedAgent = true
	ss, err := ag.Signers()
	if err != nil {
		return
	}
	for i, s := range ss {
		add(s, fmt.Sprintf("ssh-agent#%d", i))
	}
}

func (plan *authPlan) addPassword(target string) {
	if target == "" {
		return
	}
	pw, err := secrets.Get(target)
	if err != nil || pw == "" {
		return
	}
	plan.addPasswordMethods(pw)
}

func (plan *authPlan) addPasswordMethods(pw string) {
	if plan.triedPassword {
		return
	}
	plan.triedPassword = true
	plan.methods = append(plan.methods,
		ssh.KeyboardInteractive(func(user, instruction string, questions []string, echos []bool) ([]string, error) {
			ans := make([]string, len(questions))
			for i := range questions {
				ans[i] = pw
			}
			return ans, nil
		}),
		ssh.Password(pw),
	)
}

func loadSigner(path, secretName string) (ssh.Signer, error) {
	if path == "" {
		return nil, wire.E("AUTH_FAILED", "key_path is empty")
	}
	key, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	signer, err := ssh.ParsePrivateKey(key)
	if err != nil {
		if _, ok := err.(*ssh.PassphraseMissingError); ok {
			pw, perr := secrets.Get(secretName)
			if perr != nil || pw == "" {
				return nil, wire.E("AUTH_FAILED", "key passphrase required; run hub secret set "+secretName)
			}
			signer, err = ssh.ParsePrivateKeyWithPassphrase(key, []byte(pw))
		}
		if err != nil {
			return nil, wire.E("AUTH_FAILED", "invalid private key "+path)
		}
	}
	if as, ok := signer.(ssh.AlgorithmSigner); ok && signer.PublicKey().Type() == ssh.KeyAlgoRSA {
		if wrapped, werr := ssh.NewSignerWithAlgorithms(as, []string{ssh.KeyAlgoRSASHA256, ssh.KeyAlgoRSASHA512}); werr == nil {
			return wrapped, nil
		}
	}
	return signer, nil
}

func defaultIdentityFiles() []string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return nil
	}
	dir := filepath.Join(home, ".ssh")
	names := []string{"id_ed25519", "id_ed25519_sk", "id_ecdsa", "id_ecdsa_sk", "id_rsa"}
	out := make([]string, 0, len(names))
	for _, n := range names {
		p := filepath.Join(dir, n)
		if _, err := os.Stat(p); err == nil {
			out = append(out, p)
		}
	}
	return out
}

func samePath(a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	ca, err1 := filepath.Abs(a)
	cb, err2 := filepath.Abs(b)
	if err1 != nil || err2 != nil {
		return strings.EqualFold(filepath.Clean(a), filepath.Clean(b))
	}
	return strings.EqualFold(ca, cb)
}

func mapSSHErr(err error, t *config.Resolved, plan *authPlan) error {
	if err == nil {
		return nil
	}
	if e, ok := err.(*wire.Error); ok {
		return e
	}
	msg := err.Error()
	low := strings.ToLower(msg)
	if strings.Contains(low, "unable to authenticate") || strings.Contains(low, "no supported methods") {
		return wire.E("AUTH_FAILED", formatAuthFailed(t, plan, msg))
	}
	if strings.Contains(low, "host key") {
		return wire.E("HOST_KEY_CHANGED", msg)
	}
	return wire.Ef("REMOTE_UNREACHABLE", "%v", err)
}

func formatAuthFailed(t *config.Resolved, plan *authPlan, sshMsg string) string {
	who := t.User + "@" + t.Host
	parts := []string{"authentication failed for " + who}
	if plan != nil && len(plan.keyFPs) > 0 {
		parts = append(parts, "offered keys: "+strings.Join(plan.keyFPs, ", "))
	} else {
		parts = append(parts, "offered keys: none")
	}
	if sshMsg != "" {
		parts = append(parts, sshMsg)
	}
	if plan == nil || !plan.triedPassword {
		name := t.Name
		if name == "" {
			name = "<target>"
		}
		parts = append(parts, "password not tried; if the host accepts a password, run: hub secret set "+name)
	}
	return strings.Join(parts, "; ")
}
