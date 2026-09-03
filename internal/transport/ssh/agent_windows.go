//go:build windows

package sshx

import (
	"net"
	"time"

	"github.com/Microsoft/go-winio"
)

func platformDialAgent() (net.Conn, error) {
	d := time.Second
	return winio.DialPipe(`\\.\pipe\openssh-ssh-agent`, &d)
}
