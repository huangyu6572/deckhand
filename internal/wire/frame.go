package wire

import (
	"encoding/binary"
	"encoding/json"
	"io"
	"unicode/utf8"
)

type Frame struct {
	V       int             `json:"v"`
	Kind    string          `json:"kind"`
	ID      string          `json:"id"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	OK      *bool           `json:"ok,omitempty"`
	Payload json.RawMessage `json:"payload,omitempty"`
	Event   json.RawMessage `json:"event,omitempty"`
}

func WriteFrame(w io.Writer, f *Frame) error {
	if f.V == 0 {
		f.V = ProtocolVersion
	}
	body, err := json.Marshal(f)
	if err != nil {
		return err
	}
	var hdr [4]byte
	binary.LittleEndian.PutUint32(hdr[:], uint32(len(body)))
	if _, err := w.Write(hdr[:]); err != nil {
		return err
	}
	_, err = w.Write(body)
	return err
}

func ReadFrame(r io.Reader, max int) (*Frame, error) {
	var hdr [4]byte
	if _, err := io.ReadFull(r, hdr[:]); err != nil {
		return nil, err
	}
	n := int(binary.LittleEndian.Uint32(hdr[:]))
	if n == 0 || (max > 0 && n > max) {
		return nil, E("IPC_PROTOCOL_ERROR", "invalid frame length")
	}
	body := make([]byte, n)
	if _, err := io.ReadFull(r, body); err != nil {
		return nil, err
	}
	var f Frame
	if err := json.Unmarshal(body, &f); err != nil {
		return nil, E("IPC_PROTOCOL_ERROR", "invalid json")
	}
	if f.Kind != KindRequest && f.Kind != KindResponse && f.Kind != KindEvent {
		return nil, E("IPC_PROTOCOL_ERROR", "unknown kind")
	}
	if f.V != ProtocolVersion {
		return nil, E("IPC_PROTOCOL_ERROR", "protocol version mismatch; upgrade hub.exe and hubd.exe together")
	}
	return &f, nil
}

type Result map[string]any

func Base(ok bool, requestID, status string) Result {
	return Result{
		"ok":         ok,
		"request_id": requestID,
		"status":     status,
	}
}

func Fail(requestID string, err error) Result {
	e, ok := err.(*Error)
	if !ok {
		e = E("STORAGE_ERROR", err.Error())
	}
	status := e.Status
	if status == "" {
		status = "failed"
	}
	r := Base(false, requestID, status)
	r["error_code"] = e.Code
	r["message"] = e.Message
	r["retryable"] = e.Retryable
	return r
}

func MustJSON(v any) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return b
}

func TruncateUTF8(s string, max int) (string, bool) {
	if max <= 0 || len(s) <= max {
		return s, false
	}
	b := []byte(s)
	if len(b) > max {
		b = b[:max]
	}
	for len(b) > 0 && !utf8.Valid(b) {
		b = b[:len(b)-1]
	}
	return string(b), true
}

type Event struct {
	Type        string `json:"type"`
	OperationID string `json:"operation_id"`
	Cursor      int64  `json:"cursor,omitempty"`
	Timestamp   string `json:"timestamp"`
	Data        string `json:"data,omitempty"`
	DataBase64  string `json:"data_base64,omitempty"`
	State       string `json:"state,omitempty"`
	ErrorCode   string `json:"error_code,omitempty"`
	Message     string `json:"message,omitempty"`
	Stream      string `json:"stream,omitempty"`
}
