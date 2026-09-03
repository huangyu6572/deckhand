package app

import (
	"net"
	"testing"

	"localaihub/internal/wire"
)

func TestIPCTargetList(t *testing.T) {
	dir := t.TempDir()
	a, err := OpenDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { a.Close() })
	c1, c2 := net.Pipe()
	defer c2.Close()
	go a.handleConn(c1)
	fr := &wire.Frame{V: wire.ProtocolVersion, Kind: wire.KindRequest, ID: "req_ipc", Method: wire.TargetList, Params: wire.MustJSON(map[string]any{})}
	if err := wire.WriteFrame(c2, fr); err != nil {
		t.Fatal(err)
	}
	got, err := wire.ReadFrame(c2, 16<<20)
	if err != nil {
		t.Fatal(err)
	}
	if got.Kind != wire.KindResponse || got.OK == nil || !*got.OK {
		t.Fatalf("%+v", got)
	}
}

func TestIPCBadVersion(t *testing.T) {
	dir := t.TempDir()
	a, err := OpenDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { a.Close() })
	c1, c2 := net.Pipe()
	defer c2.Close()
	go a.handleConn(c1)
	fr := &wire.Frame{V: 2, Kind: wire.KindRequest, ID: "req_v", Method: wire.TargetList}
	if err := wire.WriteFrame(c2, fr); err != nil {
		t.Fatal(err)
	}
	got, err := wire.ReadFrame(c2, 16<<20)
	if err != nil {
		t.Fatal(err)
	}
	if got.OK != nil && *got.OK {
		t.Fatal("expected failure")
	}
}
