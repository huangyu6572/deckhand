package wire

import (
	"crypto/rand"
	"encoding/hex"
)

func NewID(prefix string) string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	return prefix + hex.EncodeToString(b[:])
}

func NewRequestID() string { return NewID("req_") }
func NewJobID() string     { return NewID("job_") }
func NewSessID() string    { return NewID("sess_") }
func NewConnID() string    { return NewID("conn_") }
