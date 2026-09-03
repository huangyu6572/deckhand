package contract

import (
	"context"
	"io"
	"regexp"

	"localaihub/internal/config"
)

type Capabilities struct {
	Exec        bool
	Interactive bool
	Transact    bool
	Files       bool
	Resize      bool
	ExitCode    bool
	Reconnect   bool
}

type Transport interface {
	Close() error
	Capabilities() Capabilities
}

type ExecConn interface {
	Transport
	Exec(ctx context.Context, command string, stdout, stderr io.Writer) (exitCode int, err error)
}

type FileConn interface {
	Transport
	Upload(ctx context.Context, localPath, remotePath string) (bytes int64, sha256hex string, err error)
	Download(ctx context.Context, remotePath, localPath string) (bytes int64, sha256hex string, err error)
}

type PTY interface {
	io.ReadWriteCloser
	Resize(cols, rows int) error
}

type PTYConn interface {
	Transport
	OpenPTY(ctx context.Context, cols, rows int) (PTY, error)
}

type Matcher struct {
	Literal string
	Regex   *regexp.Regexp
}

type SerialConn interface {
	Transport
	Transact(ctx context.Context, payload []byte, wait Matcher, window int) (matched []byte, err error)
	ReadLoop() <-chan []byte
}

type Factory func(ctx context.Context, target *config.Resolved, opts Options) (Transport, error)

type Options struct {
	DataDir      string
	AllowPublic  bool
	OnHostKeyNew func(host, comment string)
	Password     func() (string, error)
	Warn         func(string)
}
