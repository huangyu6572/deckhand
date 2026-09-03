package wire

import (
	"bytes"
	"testing"
)

func TestFrameRoundTrip(t *testing.T) {
	var buf bytes.Buffer
	f := &Frame{V: 1, Kind: KindRequest, ID: "req_01", Method: JobRun, Params: MustJSON(map[string]any{"target": "t"})}
	if err := WriteFrame(&buf, f); err != nil {
		t.Fatal(err)
	}
	got, err := ReadFrame(&buf, 16<<20)
	if err != nil {
		t.Fatal(err)
	}
	if got.Method != JobRun || got.ID != "req_01" {
		t.Fatalf("%+v", got)
	}
}

func TestProtocolVersionMismatch(t *testing.T) {
	var buf bytes.Buffer
	ok := true
	f := &Frame{V: 2, Kind: KindResponse, ID: "req_01", OK: &ok}
	_ = WriteFrame(&buf, f)
	_, err := ReadFrame(&buf, 16<<20)
	if err == nil {
		t.Fatal("expected version error")
	}
}

func TestOversizeFrame(t *testing.T) {
	var buf bytes.Buffer
	f := &Frame{V: 1, Kind: KindRequest, ID: "req_01", Method: JobRun, Params: MustJSON(map[string]any{"x": "yyyyyyyy"})}
	if err := WriteFrame(&buf, f); err != nil {
		t.Fatal(err)
	}
	_, err := ReadFrame(&buf, 4)
	if err == nil {
		t.Fatal("expected oversize")
	}
}

func TestTruncateUTF8(t *testing.T) {
	s, trunc := TruncateUTF8("hello", 100)
	if trunc || s != "hello" {
		t.Fatal(s, trunc)
	}
	s, trunc = TruncateUTF8("你好世界", 4)
	if !trunc || !utf8Valid(s) {
		t.Fatalf("%q %v", s, trunc)
	}
}

func utf8Valid(s string) bool {
	for i := 0; i < len(s); {
		if s[i] < 0x80 {
			i++
			continue
		}
		return true
	}
	return true
}
