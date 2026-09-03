//go:build !windows

package sshx

import (
	"fmt"
	"net"
)

func platformDialAgent() (net.Conn, error) {
	return nil, fmt.Errorf("no ssh-agent")
}
