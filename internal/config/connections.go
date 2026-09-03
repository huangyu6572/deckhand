package config

import (
	"bytes"
	"os"
	"path/filepath"
	"regexp"

	"gopkg.in/yaml.v3"

	"localaihub/internal/platform"
	"localaihub/internal/wire"
)

var targetNameRe = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_-]{0,62}$`)

type File struct {
	Targets map[string]TargetYAML `yaml:"targets"`
}

type TargetYAML struct {
	Transport      string    `yaml:"transport"`
	Host           string    `yaml:"host,omitempty"`
	User           string    `yaml:"user,omitempty"`
	Port           any       `yaml:"port,omitempty"`
	Auth           *AuthYAML `yaml:"auth,omitempty"`
	WorkspaceRoot  string    `yaml:"workspace_root,omitempty"`
	BaudRate       int       `yaml:"baud_rate,omitempty"`
	PromptPattern  string    `yaml:"prompt_pattern,omitempty"`
	Reconnect      *bool     `yaml:"reconnect,omitempty"`
	DefaultTimeout string    `yaml:"default_timeout,omitempty"`
}

type AuthYAML struct {
	Type    string `yaml:"type"`
	KeyPath string `yaml:"key_path,omitempty"`
}

func LoadConnections(dataDir string) (File, error) {
	path := filepath.Join(dataDir, "connections.yaml")
	f := File{Targets: map[string]TargetYAML{}}
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			out := []byte("targets: {}\n")
			if werr := platform.AtomicWriteFile(path, out, 0o600); werr != nil {
				return f, werr
			}
			return f, nil
		}
		return f, err
	}
	dec := yaml.NewDecoder(bytes.NewReader(b))
	dec.KnownFields(true)
	if err := dec.Decode(&f); err != nil {
		return f, wire.Ef("CONFIG_INVALID", "connections.yaml: %v", err)
	}
	if f.Targets == nil {
		f.Targets = map[string]TargetYAML{}
	}
	for name, t := range f.Targets {
		if !targetNameRe.MatchString(name) {
			return f, wire.Ef("CONFIG_INVALID", "invalid target name %q", name)
		}
		if t.Transport != "ssh" && t.Transport != "serial" {
			return f, wire.Ef("CONFIG_INVALID", "target %s: transport must be ssh or serial", name)
		}
	}
	return f, nil
}

func ConnectionsPath(dataDir string) string {
	return filepath.Join(dataDir, "connections.yaml")
}
