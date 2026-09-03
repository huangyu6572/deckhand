//go:build !windows

package platform

import (
	"os"
	"path/filepath"
)

func secretFile(target string) (string, error) {
	dir, err := DataDir()
	if err != nil {
		return "", err
	}
	d := filepath.Join(dir, "secrets")
	if err := os.MkdirAll(d, 0o700); err != nil {
		return "", err
	}
	return filepath.Join(d, target+".secret"), nil
}

func WriteCredential(target, secret string) error {
	p, err := secretFile(target)
	if err != nil {
		return err
	}
	return AtomicWriteFile(p, []byte(secret), 0o600)
}

func ReadCredential(target string) (string, error) {
	p, err := secretFile(target)
	if err != nil {
		return "", err
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func DeleteCredential(target string) error {
	p, err := secretFile(target)
	if err != nil {
		return err
	}
	return os.Remove(p)
}
