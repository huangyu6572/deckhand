//go:build windows

package platform

import (
	"context"
	"fmt"
	"net"
	"time"

	"github.com/Microsoft/go-winio"
)

func ListenPipe(sid string) (net.Listener, error) {
	sddl := fmt.Sprintf("D:P(A;;GA;;;%s)(A;;GA;;;SY)", sid)
	return winio.ListenPipe(PipeName(sid), &winio.PipeConfig{
		SecurityDescriptor: sddl,
		InputBufferSize:    1024 * 1024,
		OutputBufferSize:   1024 * 1024,
	})
}

func DialPipe(ctx context.Context, sid string) (net.Conn, error) {
	d := 2 * time.Second
	if deadline, ok := ctx.Deadline(); ok {
		d = time.Until(deadline)
		if d < 0 {
			d = 0
		}
	}
	return winio.DialPipe(PipeName(sid), &d)
}
