package serialx

import (
	"bytes"
	"context"
	"io"
	"sync"
	"time"

	"go.bug.st/serial"

	"localaihub/internal/config"
	"localaihub/internal/transport/contract"
	"localaihub/internal/wire"
)

type Port struct {
	p       serial.Port
	target  *config.Resolved
	ch      chan []byte
	mu      sync.Mutex
	writeMu sync.Mutex
	buf     bytes.Buffer
	closed  bool
}

func Factory(ctx context.Context, t *config.Resolved, opts contract.Options) (contract.Transport, error) {
	mode := &serial.Mode{BaudRate: t.BaudRate, DataBits: 8, Parity: serial.NoParity, StopBits: serial.OneStopBit}
	p, err := serial.Open(t.SerialPort, mode)
	if err != nil {
		return nil, wire.E("SERIAL_BUSY", err.Error())
	}
	c := &Port{p: p, target: t, ch: make(chan []byte, 64)}
	go c.loop()
	return c, nil
}

func List() ([]string, error) {
	return serial.GetPortsList()
}

func (c *Port) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return nil
	}
	c.closed = true
	return c.p.Close()
}

func (c *Port) Capabilities() contract.Capabilities {
	return contract.Capabilities{Interactive: true, Transact: true, Reconnect: true}
}

func (c *Port) ReadLoop() <-chan []byte { return c.ch }

func (c *Port) loop() {
	buf := make([]byte, 1024)
	for {
		n, err := c.p.Read(buf)
		if n > 0 {
			b := append([]byte(nil), buf[:n]...)
			c.mu.Lock()
			c.buf.Write(b)
			if c.buf.Len() > 1024*1024 {
				drop := c.buf.Len() - 512*1024
				c.buf.Next(drop)
			}
			c.mu.Unlock()
			select {
			case c.ch <- b:
			default:
			}
		}
		if err != nil {
			if err != io.EOF {
				c.mu.Lock()
				c.closed = true
				c.mu.Unlock()
			}
			close(c.ch)
			return
		}
	}
}

func (c *Port) Write(p []byte) (int, error) {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	c.mu.Lock()
	dead := c.closed
	c.mu.Unlock()
	if dead {
		return 0, wire.E("SERIAL_DISCONNECTED", "port closed")
	}
	return c.p.Write(p)
}

func (c *Port) Transact(ctx context.Context, payload []byte, wait contract.Matcher, window int) ([]byte, error) {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	if window <= 0 {
		window = 65536
	}
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil, wire.E("SERIAL_DISCONNECTED", "port closed")
	}
	start := c.buf.Len()
	c.mu.Unlock()
	if _, err := c.p.Write(payload); err != nil {
		return nil, wire.E("SERIAL_DISCONNECTED", err.Error())
	}
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return c.sliceFrom(start, window), wire.E("SERIAL_TIMEOUT", "wait timed out")
		case <-ticker.C:
			chunk := c.sliceFrom(start, window)
			if len(chunk) > window {
				return chunk, wire.E("SERIAL_TIMEOUT", "matcher window exceeded")
			}
			if match(chunk, wait) {
				return chunk, nil
			}
			c.mu.Lock()
			dead := c.closed
			c.mu.Unlock()
			if dead {
				return chunk, wire.E("SERIAL_DISCONNECTED", "port closed")
			}
		}
	}
}

func (c *Port) sliceFrom(start, window int) []byte {
	c.mu.Lock()
	defer c.mu.Unlock()
	b := c.buf.Bytes()
	if start > len(b) {
		start = len(b)
	}
	out := b[start:]
	if len(out) > window {
		out = out[:window+1]
	}
	return append([]byte(nil), out...)
}

func match(b []byte, m contract.Matcher) bool {
	if m.Regex != nil {
		return m.Regex.Match(b)
	}
	if m.Literal == "" {
		return true
	}
	return bytes.Contains(b, []byte(m.Literal))
}
