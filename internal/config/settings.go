package config

import (
	"bytes"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"

	"localaihub/internal/platform"
	"localaihub/internal/wire"
)

type Settings struct {
	Network    NetworkSettings    `yaml:"network"`
	SSH        SSHSettings        `yaml:"ssh"`
	Daemon     DaemonSettings     `yaml:"daemon"`
	Limits     Limits             `yaml:"limits"`
	Connection ConnectionSettings `yaml:"connection"`
	Log        LogSettings        `yaml:"log"`
}

type NetworkSettings struct {
	Scope string `yaml:"scope"`
}

type SSHSettings struct {
	HostKey string `yaml:"host_key"`
}

type DaemonSettings struct {
	IdleExit Duration `yaml:"idle_exit"`
}

type Limits struct {
	MaxResponseBytes     int `yaml:"max_response_bytes"`
	MaxEventBytes        int `yaml:"max_event_bytes"`
	MaxOperationLogBytes int `yaml:"max_operation_log_bytes"`
	MaxTotalLogBytes     int `yaml:"max_total_log_bytes"`
	MaxConcurrentJobs    int `yaml:"max_concurrent_jobs"`
	MatcherMaxBytes      int `yaml:"matcher_max_bytes"`
	PipeMaxFrameBytes    int `yaml:"pipe_max_frame_bytes"`
}

type ConnectionSettings struct {
	IdleTimeout Duration `yaml:"idle_timeout"`
}

type LogSettings struct {
	RetainDays int `yaml:"retain_days"`
}

type Duration time.Duration

func (d *Duration) UnmarshalYAML(n *yaml.Node) error {
	if n.Kind != yaml.ScalarNode {
		return wire.E("CONFIG_INVALID", "duration must be a string or number")
	}
	if n.Value == "0" || n.Value == "" {
		*d = 0
		return nil
	}
	if v, err := time.ParseDuration(n.Value); err == nil {
		*d = Duration(v)
		return nil
	}
	return wire.Ef("CONFIG_INVALID", "invalid duration %q", n.Value)
}

func (d Duration) MarshalYAML() (any, error) {
	if d == 0 {
		return 0, nil
	}
	return time.Duration(d).String(), nil
}

func (d Duration) Duration() time.Duration { return time.Duration(d) }

func DefaultSettings() Settings {
	return Settings{
		Network: NetworkSettings{Scope: "intranet"},
		SSH:     SSHSettings{HostKey: "accept-new-intranet"},
		Daemon:  DaemonSettings{IdleExit: 0},
		Limits: Limits{
			MaxResponseBytes:     8388608,
			MaxEventBytes:        262144,
			MaxOperationLogBytes: 104857600,
			MaxTotalLogBytes:     1073741824,
			MaxConcurrentJobs:    32,
			MatcherMaxBytes:      65536,
			PipeMaxFrameBytes:    16777216,
		},
		Connection: ConnectionSettings{IdleTimeout: Duration(30 * time.Minute)},
		Log:        LogSettings{RetainDays: 30},
	}
}

func LoadSettings(dataDir string) (Settings, error) {
	path := filepath.Join(dataDir, "settings.yaml")
	s := DefaultSettings()
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			out, merr := yaml.Marshal(s)
			if merr != nil {
				return s, merr
			}
			if werr := platform.AtomicWriteFile(path, out, 0o600); werr != nil {
				return s, werr
			}
			return s, nil
		}
		return s, err
	}
	dec := yaml.NewDecoder(bytes.NewReader(b))
	dec.KnownFields(true)
	if err := dec.Decode(&s); err != nil {
		return s, wire.Ef("CONFIG_INVALID", "settings.yaml: %v", err)
	}
	if s.Network.Scope != "intranet" && s.Network.Scope != "all" {
		return s, wire.E("CONFIG_INVALID", "network.scope must be intranet or all")
	}
	if s.Limits.PipeMaxFrameBytes <= 0 {
		s.Limits.PipeMaxFrameBytes = 16777216
	}
	if s.Limits.MaxConcurrentJobs <= 0 {
		s.Limits.MaxConcurrentJobs = 32
	}
	return s, nil
}
