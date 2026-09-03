package config

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"localaihub/internal/wire"
)

type SSHHost struct {
	Name           string
	HostName       string
	User           string
	Port           int
	IdentityFile   string
	IdentitiesOnly bool
	ProxyJump      string
	Warnings       []string
}

var ignoredSSHKeys = map[string]bool{
	"serveraliveinterval":      true,
	"serveralivecountmax":      true,
	"loglevel":                 true,
	"forwardagent":             true,
	"compression":              true,
	"userknownhostsfile":       true,
	"stricthostkeychecking":    true,
	"hashknownhosts":           true,
	"gssapiauthentication":     true,
	"preferredauthentications": true,
	"addkeystoagent":           true,
}

func ParseOpenSSH(path, host string, includeDepth int) (*SSHHost, error) {
	if includeDepth > 1 {
		return nil, wire.E("OPENSSH_UNSUPPORTED", "nested Include deeper than one level")
	}
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	var current []string
	inMatch := false
	foundExact := false
	h := &SSHHost{Name: host, Port: 22}
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v := splitSSH(line)
		lk := strings.ToLower(k)
		if lk == "include" {
			inc := expandHome(v)
			if !filepath.IsAbs(inc) {
				inc = filepath.Join(filepath.Dir(path), inc)
			}
			if includeDepth >= 1 {
				return nil, wire.E("OPENSSH_UNSUPPORTED", "Include nested more than one level")
			}
			if _, err := os.Stat(inc); err != nil {
				return nil, wire.Ef("OPENSSH_UNSUPPORTED", "Include file missing: %s", inc)
			}
			sub, err := ParseOpenSSH(inc, host, includeDepth+1)
			if err != nil {
				return nil, err
			}
			if sub != nil && foundExact {
				mergeSSH(h, sub)
			} else if sub != nil && !foundExact {
				*h = *sub
				foundExact = true
			}
			continue
		}
		if lk == "match" {
			inMatch = true
			if strings.EqualFold(strings.Fields(v)[0], "host") || strings.Contains(strings.ToLower(v), host) {
				return nil, wire.E("OPENSSH_UNSUPPORTED", "Match blocks are not supported")
			}
			continue
		}
		if lk == "host" {
			inMatch = false
			current = strings.Fields(v)
			continue
		}
		if inMatch {
			continue
		}
		if !hostApplies(current, host) {
			continue
		}
		if !containsExact(current, host) {
			continue
		}
		foundExact = true
		if err := applySSHKey(h, lk, v); err != nil {
			return nil, err
		}
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	if !foundExact {
		return nil, nil
	}
	if h.HostName == "" {
		h.HostName = host
	}
	return h, nil
}

func splitSSH(line string) (string, string) {
	i := strings.IndexAny(line, " \t=")
	if i < 0 {
		return line, ""
	}
	k := line[:i]
	rest := strings.TrimLeft(line[i:], " \t=")
	rest = strings.Trim(rest, `"`)
	return k, rest
}

func hostApplies(patterns []string, host string) bool {
	return containsExact(patterns, host)
}

func containsExact(patterns []string, host string) bool {
	for _, p := range patterns {
		if p == host {
			return true
		}
	}
	return false
}

func applySSHKey(h *SSHHost, k, v string) error {
	switch k {
	case "hostname":
		h.HostName = v
	case "user":
		h.User = v
	case "port":
		p, err := strconv.Atoi(v)
		if err != nil {
			return wire.Ef("OPENSSH_UNSUPPORTED", "invalid Port %s", v)
		}
		h.Port = p
	case "identityfile":
		if h.IdentityFile == "" {
			h.IdentityFile = v
		}
	case "identitiesonly":
		h.IdentitiesOnly = strings.EqualFold(v, "yes") || v == "1"
	case "proxyjump":
		if strings.Contains(v, ",") {
			return wire.E("OPENSSH_UNSUPPORTED", "multi-hop ProxyJump is not supported")
		}
		h.ProxyJump = v
	case "proxycommand":
		return wire.E("OPENSSH_UNSUPPORTED", "ProxyCommand is not supported")
	case "match":
		return wire.E("OPENSSH_UNSUPPORTED", "Match is not supported")
	default:
		if ignoredSSHKeys[k] {
			h.Warnings = append(h.Warnings, fmt.Sprintf("ignoring ssh_config %s", k))
			return nil
		}
	}
	return nil
}

func mergeSSH(dst, src *SSHHost) {
	if src.HostName != "" {
		dst.HostName = src.HostName
	}
	if src.User != "" {
		dst.User = src.User
	}
	if src.Port != 0 {
		dst.Port = src.Port
	}
	if src.IdentityFile != "" && dst.IdentityFile == "" {
		dst.IdentityFile = src.IdentityFile
	}
	dst.Warnings = append(dst.Warnings, src.Warnings...)
}

func expandHome(p string) string {
	if strings.HasPrefix(p, "~/") || strings.HasPrefix(p, `~\`) {
		home, err := os.UserHomeDir()
		if err != nil {
			return p
		}
		return filepath.Join(home, p[2:])
	}
	return p
}
