package sshx

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"time"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"

	"localaihub/internal/config"
	"localaihub/internal/transport/contract"
	"localaihub/internal/wire"
)

type Conn struct {
	client *ssh.Client
	target *config.Resolved
	opts   contract.Options
}

func Factory(ctx context.Context, t *config.Resolved, opts contract.Options) (contract.Transport, error) {
	return Dial(ctx, t, opts)
}

func Dial(ctx context.Context, t *config.Resolved, opts contract.Options) (*Conn, error) {
	if t.Jump != nil {
		jump, err := Dial(ctx, t.Jump, opts)
		if err != nil {
			return nil, err
		}
		addr := net.JoinHostPort(t.PinnedIP, fmt.Sprintf("%d", t.Port))
		nc, err := jump.client.Dial("tcp", addr)
		if err != nil {
			jump.Close()
			return nil, wire.Ef("REMOTE_UNREACHABLE", "jump dial: %v", err)
		}
		cfg, plan, err := clientConfig(t, opts)
		if err != nil {
			nc.Close()
			jump.Close()
			return nil, err
		}
		cc, chans, reqs, err := ssh.NewClientConn(nc, t.Host, cfg)
		if err != nil {
			nc.Close()
			jump.Close()
			return nil, mapSSHErr(err, t, plan)
		}
		c := ssh.NewClient(cc, chans, reqs)
		return &Conn{client: c, target: t, opts: opts}, nil
	}
	d := net.Dialer{Timeout: 20 * time.Second}
	addr := net.JoinHostPort(t.PinnedIP, fmt.Sprintf("%d", t.Port))
	nc, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, wire.Ef("REMOTE_UNREACHABLE", "%v", err)
	}
	cfg, plan, err := clientConfig(t, opts)
	if err != nil {
		nc.Close()
		return nil, err
	}
	cc, chans, reqs, err := ssh.NewClientConn(nc, t.Host, cfg)
	if err != nil {
		nc.Close()
		return nil, mapSSHErr(err, t, plan)
	}
	return &Conn{client: ssh.NewClient(cc, chans, reqs), target: t, opts: opts}, nil
}

func (c *Conn) Close() error {
	if c.client != nil {
		return c.client.Close()
	}
	return nil
}

func (c *Conn) Capabilities() contract.Capabilities {
	return contract.Capabilities{Exec: true, Interactive: true, Files: true, Resize: true, ExitCode: true, Reconnect: true}
}

func (c *Conn) Exec(ctx context.Context, command string, stdout, stderr io.Writer) (int, error) {
	s, err := c.client.NewSession()
	if err != nil {
		return -1, wire.Ef("REMOTE_UNREACHABLE", "%v", err)
	}
	defer s.Close()
	s.Stdout = stdout
	s.Stderr = stderr
	done := make(chan error, 1)
	go func() { done <- s.Run(command) }()
	select {
	case <-ctx.Done():
		_ = s.Close()
		if ctx.Err() == context.DeadlineExceeded {
			return -1, wire.E("JOB_TIMEOUT", "command timed out")
		}
		if ctx.Err() == context.Canceled {
			return -1, wire.E("JOB_CANCELLED", "cancelled")
		}
		return -1, wire.E("EXECUTION_UNKNOWN", ctx.Err().Error())
	case err := <-done:
		if err == nil {
			return 0, nil
		}
		if ee, ok := err.(*ssh.ExitError); ok {
			return ee.ExitStatus(), nil
		}
		return -1, wire.E("EXECUTION_UNKNOWN", err.Error())
	}
}

func (c *Conn) Upload(ctx context.Context, localPath, remotePath string) (int64, string, error) {
	cli, err := sftp.NewClient(c.client)
	if err != nil {
		return 0, "", wire.Ef("REMOTE_UNREACHABLE", "sftp: %v", err)
	}
	defer cli.Close()
	in, err := os.Open(localPath)
	if err != nil {
		return 0, "", wire.E("LOCAL_IO_ERROR", err.Error())
	}
	defer in.Close()
	dir := filepath.ToSlash(filepath.Dir(remotePath))
	_ = cli.MkdirAll(dir)
	tmp := dir + "/.hubtmp-" + wire.NewID("")
	out, err := cli.Create(tmp)
	if err != nil {
		return 0, "", wire.Ef("REMOTE_UNREACHABLE", "sftp create: %v", err)
	}
	h := sha256.New()
	n, err := io.Copy(io.MultiWriter(out, h), in)
	cerr := out.Close()
	if err != nil || cerr != nil {
		_ = cli.Remove(tmp)
		if err == nil {
			err = cerr
		}
		return n, "", wire.Ef("REMOTE_UNREACHABLE", "sftp write: %v", err)
	}
	sum := hex.EncodeToString(h.Sum(nil))
	_ = cli.Remove(remotePath)
	if err := cli.Rename(tmp, remotePath); err != nil {
		_ = cli.Remove(tmp)
		return n, sum, wire.Ef("REMOTE_UNREACHABLE", "sftp rename: %v", err)
	}
	return n, sum, nil
}

func (c *Conn) Download(ctx context.Context, remotePath, localPath string) (int64, string, error) {
	cli, err := sftp.NewClient(c.client)
	if err != nil {
		return 0, "", wire.Ef("REMOTE_UNREACHABLE", "sftp: %v", err)
	}
	defer cli.Close()
	in, err := cli.Open(remotePath)
	if err != nil {
		return 0, "", wire.Ef("REMOTE_UNREACHABLE", "sftp open: %v", err)
	}
	defer in.Close()
	dir := filepath.Dir(localPath)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return 0, "", wire.E("LOCAL_IO_ERROR", err.Error())
	}
	tmp := localPath + ".hubtmp"
	out, err := os.Create(tmp)
	if err != nil {
		return 0, "", wire.E("LOCAL_IO_ERROR", err.Error())
	}
	h := sha256.New()
	n, err := io.Copy(io.MultiWriter(out, h), in)
	cerr := out.Close()
	if err != nil || cerr != nil {
		os.Remove(tmp)
		if err == nil {
			err = cerr
		}
		return n, "", wire.E("LOCAL_IO_ERROR", err.Error())
	}
	sum := hex.EncodeToString(h.Sum(nil))
	if err := os.Rename(tmp, localPath); err != nil {
		os.Remove(tmp)
		return n, sum, wire.E("LOCAL_IO_ERROR", err.Error())
	}
	return n, sum, nil
}

type ptySession struct {
	sess   *ssh.Session
	stdin  io.WriteCloser
	stdout io.Reader
}

func (p *ptySession) Read(b []byte) (int, error)  { return p.stdout.Read(b) }
func (p *ptySession) Write(b []byte) (int, error) { return p.stdin.Write(b) }
func (p *ptySession) Close() error {
	_ = p.stdin.Close()
	return p.sess.Close()
}
func (p *ptySession) Resize(cols, rows int) error {
	return p.sess.WindowChange(rows, cols)
}

func (c *Conn) OpenPTY(ctx context.Context, cols, rows int) (contract.PTY, error) {
	s, err := c.client.NewSession()
	if err != nil {
		return nil, wire.Ef("REMOTE_UNREACHABLE", "%v", err)
	}
	modes := ssh.TerminalModes{ssh.ECHO: 1, ssh.TTY_OP_ISPEED: 14400, ssh.TTY_OP_OSPEED: 14400}
	if cols <= 0 {
		cols = 80
	}
	if rows <= 0 {
		rows = 24
	}
	if err := s.RequestPty("xterm", rows, cols, modes); err != nil {
		s.Close()
		return nil, wire.Ef("REMOTE_UNREACHABLE", "pty: %v", err)
	}
	stdin, err := s.StdinPipe()
	if err != nil {
		s.Close()
		return nil, err
	}
	stdout, err := s.StdoutPipe()
	if err != nil {
		s.Close()
		return nil, err
	}
	if err := s.Shell(); err != nil {
		s.Close()
		return nil, wire.Ef("REMOTE_UNREACHABLE", "shell: %v", err)
	}
	return &ptySession{sess: s, stdin: stdin, stdout: stdout}, nil
}

func clientConfig(t *config.Resolved, opts contract.Options) (*ssh.ClientConfig, *authPlan, error) {
	plan, err := buildAuth(t)
	if err != nil {
		return nil, nil, err
	}
	return &ssh.ClientConfig{
		User:            t.User,
		Auth:            plan.methods,
		HostKeyCallback: hostKeyCB(t, opts),
		Timeout:         20 * time.Second,
	}, plan, nil
}

func hostKeyCB(t *config.Resolved, opts contract.Options) ssh.HostKeyCallback {
	return func(hostname string, remote net.Addr, key ssh.PublicKey) error {
		path := config.KnownHostsPath(opts.DataDir)
		_ = os.MkdirAll(filepath.Dir(path), 0o700)
		want := base64.StdEncoding.EncodeToString(key.Marshal())
		known, changed, err := config.LookupKnownHost(path, t.Host, t.PinnedIP, want)
		if err != nil {
			return err
		}
		if changed {
			return wire.E("HOST_KEY_CHANGED", "host key does not match known_hosts")
		}
		if known {
			return nil
		}
		line := fmt.Sprintf("%s %s %s\n", t.Host, key.Type(), want)
		f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
		if err != nil {
			return err
		}
		_, err = f.WriteString(line)
		_ = f.Close()
		if err != nil {
			return err
		}
		if opts.OnHostKeyNew != nil {
			opts.OnHostKeyNew(t.Host, config.FingerprintSHA256(key.Marshal()))
		}
		return nil
	}
}

func sshAgent() (agent.Agent, error) {
	if s := os.Getenv("SSH_AUTH_SOCK"); s != "" {
		c, err := net.Dial("unix", s)
		if err != nil {
			return nil, err
		}
		return agent.NewClient(c), nil
	}
	c, err := platformDialAgent()
	if err != nil {
		return nil, err
	}
	return agent.NewClient(c), nil
}
