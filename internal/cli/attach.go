package cli

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"os"
	"sync"
	"time"

	"golang.org/x/term"

	"localaihub/internal/wire"
)

func invoke(ctx context.Context, method string, params map[string]any, g *globals) (wire.Result, error) {
	if timeoutMS(g) < 0 {
		return nil, wire.E("INVALID_ARGUMENT", "invalid --timeout")
	}
	conn, err := ensureDaemon(ctx)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	id := wire.NewRequestID()
	fr := &wire.Frame{V: wire.ProtocolVersion, Kind: wire.KindRequest, ID: id, Method: method, Params: wire.MustJSON(params)}
	if err := wire.WriteFrame(conn, fr); err != nil {
		return nil, wire.E("IPC_PROTOCOL_ERROR", err.Error())
	}
	max := 16 * 1024 * 1024
	for {
		in, err := wire.ReadFrame(conn, max)
		if err != nil {
			if err == io.EOF {
				return nil, wire.E("IPC_PROTOCOL_ERROR", "no response")
			}
			if e, ok := err.(*wire.Error); ok {
				return nil, e
			}
			return nil, wire.E("IPC_PROTOCOL_ERROR", err.Error())
		}
		if in.Kind != wire.KindResponse {
			continue
		}
		var payload wire.Result
		_ = json.Unmarshal(in.Payload, &payload)
		if st, _ := payload["stream"].(bool); st {
			continue
		}
		return payload, nil
	}
}

func attachSession(ctx context.Context, sessionID string, g *globals, interactive bool) error {
	if interactive && !g.JSONL && !isTTY() {
		return wire.E("INVALID_ARGUMENT", "session attach requires a TTY")
	}
	conn, err := ensureDaemon(ctx)
	if err != nil {
		printErr(err, g)
		os.Exit(3)
	}
	defer conn.Close()
	id := wire.NewRequestID()
	fr := &wire.Frame{V: wire.ProtocolVersion, Kind: wire.KindRequest, ID: id, Method: wire.SessionAttach, Params: wire.MustJSON(map[string]any{"session_id": sessionID})}
	if err := wire.WriteFrame(conn, fr); err != nil {
		return wire.E("IPC_PROTOCOL_ERROR", err.Error())
	}
	var wmu sync.Mutex
	send := func(method string, params map[string]any) {
		wmu.Lock()
		defer wmu.Unlock()
		_ = wire.WriteFrame(conn, &wire.Frame{
			V: wire.ProtocolVersion, Kind: wire.KindRequest, ID: wire.NewRequestID(),
			Method: method, Params: wire.MustJSON(params),
		})
	}
	stop := make(chan struct{})
	var stopOnce sync.Once
	halt := func() { stopOnce.Do(func() { close(stop) }) }
	defer halt()
	if interactive || (isTTY() && !g.JSONL) {
		fd := int(os.Stdin.Fd())
		if term.IsTerminal(fd) {
			old, err := term.MakeRaw(fd)
			if err == nil {
				defer term.Restore(fd, old)
			}
			go stdinLoop(os.Stdin, sessionID, send, conn, stop)
			go resizeLoop(fd, sessionID, send, stop)
		}
	}
	max := 16 * 1024 * 1024
	for {
		in, err := wire.ReadFrame(conn, max)
		if err != nil {
			halt()
			if err == io.EOF {
				return nil
			}
			return nil
		}
		switch in.Kind {
		case wire.KindEvent:
			if g.JSONL {
				_, _ = os.Stdout.Write(append(in.Event, '\n'))
				continue
			}
			var ev wire.Event
			_ = json.Unmarshal(in.Event, &ev)
			chunk := ev.Data
			if ev.DataBase64 != "" {
				if b, err := base64.StdEncoding.DecodeString(ev.DataBase64); err == nil {
					chunk = string(b)
				}
			}
			if ev.Type == "stderr" {
				fmtWrite(os.Stderr, chunk)
			} else if ev.Type == "stdout" || ev.Type == "data" {
				fmtWrite(os.Stdout, chunk)
			}
		case wire.KindResponse:
			var payload wire.Result
			_ = json.Unmarshal(in.Payload, &payload)
			if st, _ := payload["stream"].(bool); st {
				continue
			}
			halt()
			return finish(payload, g)
		}
	}
}

func fmtWrite(w io.Writer, s string) {
	_, _ = io.WriteString(w, s)
}

func stdinLoop(r io.Reader, sessionID string, send func(string, map[string]any), conn io.Closer, stop <-chan struct{}) {
	buf := make([]byte, 1)
	afterNL := true
	pendingTilde := false
	for {
		select {
		case <-stop:
			return
		default:
		}
		n, err := r.Read(buf)
		if n == 1 {
			c := buf[0]
			if pendingTilde {
				pendingTilde = false
				if c == '.' {
					send(wire.SessionDetach, map[string]any{"session_id": sessionID})
					_ = conn.Close()
					return
				}
				payload := []byte{'~', c}
				send(wire.SessionWrite, map[string]any{"session_id": sessionID, "data": string(payload)})
				afterNL = c == '\n' || c == '\r'
				continue
			}
			if afterNL && c == '~' {
				pendingTilde = true
				afterNL = false
				continue
			}
			afterNL = c == '\n' || c == '\r'
			send(wire.SessionWrite, map[string]any{"session_id": sessionID, "data": string(buf[:1])})
		}
		if err != nil {
			send(wire.SessionDetach, map[string]any{"session_id": sessionID})
			_ = conn.Close()
			return
		}
	}
}

func resizeLoop(fd int, sessionID string, send func(string, map[string]any), stop <-chan struct{}) {
	cols, rows, err := term.GetSize(fd)
	if err == nil && cols > 0 && rows > 0 {
		send(wire.SessionResize, map[string]any{"session_id": sessionID, "cols": cols, "rows": rows})
	}
	t := time.NewTicker(500 * time.Millisecond)
	defer t.Stop()
	lastC, lastR := cols, rows
	for {
		select {
		case <-stop:
			return
		case <-t.C:
			c, r, err := term.GetSize(fd)
			if err != nil || c <= 0 || r <= 0 {
				continue
			}
			if c != lastC || r != lastR {
				lastC, lastR = c, r
				send(wire.SessionResize, map[string]any{"session_id": sessionID, "cols": c, "rows": r})
			}
		}
	}
}
