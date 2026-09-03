//go:build !windows

package platform

import (
	"context"
	"net"
	"os"
	"path/filepath"
)

func ListenPipe(sid string) (net.Listener, error) {
	name := PipeName(sid)
	_ = os.Remove(name)
	if err := os.MkdirAll(filepath.Dir(name), 0o700); err != nil {
		return nil, err
	}
	return net.Listen("unix", name)
}

func DialPipe(ctx context.Context, sid string) (net.Conn, error) {
	var d net.Dialer
	return d.DialContext(ctx, "unix", PipeName(sid))
}
