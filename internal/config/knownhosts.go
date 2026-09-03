package config

import (
	"crypto/sha256"
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
)

func KnownHostsPath(dataDir string) string {
	return filepath.Join(dataDir, "ssh", "known_hosts")
}

// FirstKnownFingerprint returns the SHA256 fingerprint of the first known_hosts
// key for host or ip, or "" if none. Used in ConnKey so a rotated host key
// cannot reuse a pooled ssh.Client.
func FirstKnownFingerprint(dataDir, host, ip string) string {
	b, err := os.ReadFile(KnownHostsPath(dataDir))
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(b), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}
		if !hostNamesMatch(fields[0], host, ip) {
			continue
		}
		raw, err := base64.StdEncoding.DecodeString(fields[2])
		if err != nil {
			continue
		}
		return FingerprintSHA256(raw)
	}
	return ""
}

func FingerprintSHA256(keyMarshal []byte) string {
	fp := sha256.Sum256(keyMarshal)
	return "SHA256:" + base64.RawStdEncoding.EncodeToString(fp[:])
}

func LookupKnownHost(path, host, ip, wantKeyB64 string) (known, changed bool, err error) {
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, false, nil
		}
		return false, false, err
	}
	for _, line := range strings.Split(string(b), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}
		if !hostNamesMatch(fields[0], host, ip) {
			continue
		}
		if fields[2] == wantKeyB64 {
			return true, false, nil
		}
		return true, true, nil
	}
	return false, false, nil
}

func hostNamesMatch(field, host, ip string) bool {
	for _, n := range strings.Split(field, ",") {
		if n == host || (ip != "" && n == ip) {
			return true
		}
	}
	return false
}
