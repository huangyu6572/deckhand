package config

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"localaihub/internal/wire"
)

type Resolved struct {
	Ref            string
	Name           string
	Transport      string
	Host           string
	User           string
	Port           int
	AuthRef        string
	KeyPath        string
	PinnedIP       string
	HostKeyFP      string
	Jump           *Resolved
	SerialPort     string
	BaudRate       int
	PromptPattern  string
	Reconnect      bool
	WorkspaceRoot  string
	Ephemeral      bool
	DefaultTimeout time.Duration
	Warnings       []string
	Identity       string
	ConnKey        string
}

var comRe = regexp.MustCompile(`(?i)^COM\d+$`)
var userHostRe = regexp.MustCompile(`^([^@\s]+)@(\[[^\]]+\]|[^:\s]+)(?::(\d+))?$`)

type Resolver struct {
	DataDir     string
	Settings    Settings
	File        File
	SSHConfig   string
	AllowPublic bool
}

func (r *Resolver) ListPersistent() []string {
	names := make([]string, 0, len(r.File.Targets))
	for n := range r.File.Targets {
		names = append(names, n)
	}
	return names
}

func (r *Resolver) Resolve(ctx context.Context, ref string) (*Resolved, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return nil, wire.E("INVALID_ARGUMENT", "target is required")
	}
	if t, ok := r.File.Targets[ref]; ok {
		out, err := r.fromYAML(ctx, ref, t)
		if err != nil {
			return nil, err
		}
		if _, err := os.Stat(r.SSHConfig); err == nil {
			if h, _ := ParseOpenSSH(r.SSHConfig, ref, 0); h != nil {
				out.Warnings = append(out.Warnings, "yaml target overrides OpenSSH Host of the same name")
			}
		}
		return out, r.scopeCheck(ctx, out)
	}
	if r.SSHConfig != "" {
		h, err := ParseOpenSSH(r.SSHConfig, ref, 0)
		if err != nil {
			return nil, err
		}
		if h != nil {
			out, err := r.fromOpenSSH(ctx, h)
			if err != nil {
				return nil, err
			}
			return out, r.scopeCheck(ctx, out)
		}
	}
	if m := userHostRe.FindStringSubmatch(ref); m != nil {
		port := 22
		if m[3] != "" {
			p, err := strconv.Atoi(m[3])
			if err != nil {
				return nil, wire.E("INVALID_ARGUMENT", "invalid port")
			}
			port = p
		}
		host := strings.Trim(m[2], "[]")
		out := &Resolved{
			Ref:            ref,
			Name:           ref,
			Transport:      "ssh",
			Host:           host,
			User:           m[1],
			Port:           port,
			AuthRef:        "agent",
			Ephemeral:      true,
			DefaultTimeout: 120 * time.Second,
		}
		if err := r.pinDNS(ctx, out); err != nil {
			return nil, err
		}
		out.Identity = identity(out)
		out.HostKeyFP = FirstKnownFingerprint(r.DataDir, out.Host, out.PinnedIP)
		out.ConnKey = connKey(out)
		return out, r.scopeCheck(ctx, out)
	}
	if comRe.MatchString(ref) {
		out := &Resolved{
			Ref:            ref,
			Name:           strings.ToUpper(ref),
			Transport:      "serial",
			SerialPort:     strings.ToUpper(ref),
			BaudRate:       115200,
			Reconnect:      true,
			Ephemeral:      true,
			DefaultTimeout: 5 * time.Second,
		}
		out.Identity = "serial|" + out.SerialPort
		out.ConnKey = out.Identity
		return out, nil
	}
	return nil, wire.Ef("TARGET_NOT_FOUND", "unknown target %q", ref)
}

func (r *Resolver) fromYAML(ctx context.Context, name string, t TargetYAML) (*Resolved, error) {
	out := &Resolved{Ref: name, Name: name, Transport: t.Transport, Ephemeral: false}
	if t.DefaultTimeout != "" {
		d, err := time.ParseDuration(t.DefaultTimeout)
		if err != nil {
			return nil, wire.Ef("CONFIG_INVALID", "default_timeout: %v", err)
		}
		out.DefaultTimeout = d
	}
	switch t.Transport {
	case "ssh":
		if out.DefaultTimeout == 0 {
			out.DefaultTimeout = 120 * time.Second
		}
		out.Host = t.Host
		out.User = t.User
		out.Port = 22
		if t.Port != nil {
			switch v := t.Port.(type) {
			case int:
				out.Port = v
			case int64:
				out.Port = int(v)
			case uint64:
				out.Port = int(v)
			case string:
				p, err := strconv.Atoi(v)
				if err != nil {
					return nil, wire.Ef("CONFIG_INVALID", "invalid ssh port %q", v)
				}
				out.Port = p
			default:
				return nil, wire.E("CONFIG_INVALID", "invalid ssh port")
			}
		}
		if t.Auth != nil {
			switch t.Auth.Type {
			case "ssh-agent", "agent":
				out.AuthRef = "agent"
			case "private-key":
				p := t.Auth.KeyPath
				p = expandHome(p)
				if !filepath.IsAbs(p) {
					p = filepath.Join(r.DataDir, p)
				}
				out.AuthRef = "key:" + p
				out.KeyPath = p
			case "password":
				out.AuthRef = "cred:" + name
			default:
				return nil, wire.Ef("CONFIG_INVALID", "unknown auth.type %q", t.Auth.Type)
			}
		} else {
			out.AuthRef = "agent"
		}
		out.WorkspaceRoot = t.WorkspaceRoot
		if err := r.pinDNS(ctx, out); err != nil {
			return nil, err
		}
	case "serial":
		if out.DefaultTimeout == 0 {
			out.DefaultTimeout = 5 * time.Second
		}
		switch v := t.Port.(type) {
		case string:
			out.SerialPort = v
		default:
			return nil, wire.E("CONFIG_INVALID", "serial port must be a string like COM3")
		}
		out.BaudRate = t.BaudRate
		if out.BaudRate == 0 {
			out.BaudRate = 115200
		}
		out.PromptPattern = t.PromptPattern
		out.Reconnect = true
		if t.Reconnect != nil {
			out.Reconnect = *t.Reconnect
		}
		out.Identity = "serial|" + out.SerialPort
		out.ConnKey = out.Identity
		return out, nil
	}
	out.Identity = identity(out)
	out.HostKeyFP = FirstKnownFingerprint(r.DataDir, out.Host, out.PinnedIP)
	out.ConnKey = connKey(out)
	return out, nil
}

func (r *Resolver) fromOpenSSH(ctx context.Context, h *SSHHost) (*Resolved, error) {
	out := &Resolved{
		Ref:            h.Name,
		Name:           h.Name,
		Transport:      "ssh",
		Host:           h.HostName,
		User:           h.User,
		Port:           h.Port,
		Ephemeral:      true,
		DefaultTimeout: 120 * time.Second,
		Warnings:       h.Warnings,
	}
	if out.User == "" {
		out.User = os.Getenv("USERNAME")
		if out.User == "" {
			out.User = os.Getenv("USER")
		}
	}
	if h.IdentityFile != "" {
		p := expandIdentity(h.IdentityFile, out.Host, strconv.Itoa(out.Port), out.User)
		out.AuthRef = "key:" + p
		out.KeyPath = p
	} else {
		out.AuthRef = "agent"
	}
	if h.ProxyJump != "" {
		jump, err := r.Resolve(ctx, h.ProxyJump)
		if err != nil {
			if !wire.Is(err, "TARGET_NOT_FOUND") {
				return nil, err
			}
			jump, err = r.Resolve(ctx, normalizeJump(h.ProxyJump))
			if err != nil {
				return nil, wire.Ef("OPENSSH_UNSUPPORTED", "ProxyJump: %v", err)
			}
		}
		if jump.Transport != "ssh" {
			return nil, wire.E("OPENSSH_UNSUPPORTED", "ProxyJump must be SSH")
		}
		out.Jump = jump
	}
	if err := r.pinDNS(ctx, out); err != nil {
		return nil, err
	}
	out.Identity = identity(out)
	out.HostKeyFP = FirstKnownFingerprint(r.DataDir, out.Host, out.PinnedIP)
	out.ConnKey = connKey(out)
	return out, nil
}

func normalizeJump(j string) string {
	if strings.Contains(j, "@") {
		return j
	}
	user := os.Getenv("USERNAME")
	if user == "" {
		user = os.Getenv("USER")
	}
	return user + "@" + j
}

func (r *Resolver) pinDNS(ctx context.Context, t *Resolved) error {
	if t.Transport != "ssh" {
		return nil
	}
	host := t.Host
	if ip := net.ParseIP(strings.Trim(host, "[]")); ip != nil {
		t.PinnedIP = ip.String()
		if r.Settings.Network.Scope == "intranet" && !r.AllowPublic && !IsIntranetIP(ip) {
			return publicErr(host)
		}
		return nil
	}
	resolver := net.DefaultResolver
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	addrs, err := resolver.LookupIPAddr(ctx, host)
	if err != nil {
		return wire.Ef("REMOTE_UNREACHABLE", "dns: %v", err)
	}
	if len(addrs) == 0 {
		return wire.E("REMOTE_UNREACHABLE", "dns returned no addresses")
	}
	var pick string
	for _, a := range addrs {
		ip := a.IP
		if r.Settings.Network.Scope == "intranet" && !r.AllowPublic && !IsIntranetIP(ip) {
			return publicErr(host)
		}
		if pick == "" {
			pick = ip.String()
		}
	}
	t.PinnedIP = pick
	return nil
}

func (r *Resolver) scopeCheck(ctx context.Context, t *Resolved) error {
	if t.Jump != nil {
		if err := r.scopeCheck(ctx, t.Jump); err != nil {
			return err
		}
	}
	if t.Transport != "ssh" {
		return nil
	}
	if r.Settings.Network.Scope == "all" || r.AllowPublic {
		return nil
	}
	ip := net.ParseIP(t.PinnedIP)
	if ip == nil || !IsIntranetIP(ip) {
		return publicErr(t.Host)
	}
	return nil
}

func publicErr(host string) error {
	return wire.Ef("SCOPE_NOT_INTRANET", "%s is not intranet; retry with --allow-public or set network.scope: all", host)
}

func identity(t *Resolved) string {
	return fmt.Sprintf("%s@%s:%d", t.User, t.Host, t.Port)
}

func connKey(t *Resolved) string {
	jump := ""
	if t.Jump != nil {
		jump = t.Jump.ConnKey
	}
	return strings.Join([]string{t.User, t.Host, strconv.Itoa(t.Port), t.AuthRef, t.HostKeyFP, jump}, "|")
}

func expandIdentity(p, host, port, user string) string {
	p = expandHome(p)
	p = strings.ReplaceAll(p, "%h", host)
	p = strings.ReplaceAll(p, "%p", port)
	p = strings.ReplaceAll(p, "%r", user)
	return p
}
