//go:build !windows

package platform

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type Mutex struct {
	f    *os.File
	name string
}

func mutexPath(name string) (string, error) {
	dir, err := DataDir()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Join(dir, "runtime"), 0o700); err != nil {
		return "", err
	}
	safe := filepath.Base(name)
	return filepath.Join(dir, "runtime", safe+".mutex"), nil
}

func AcquireMutex(name string, timeout time.Duration) (*Mutex, error) {
	p, err := mutexPath(name)
	if err != nil {
		return nil, err
	}
	deadline := time.Now().Add(timeout)
	if timeout < 0 {
		deadline = time.Now().Add(24 * time.Hour)
	}
	for {
		f, err := os.OpenFile(p, os.O_CREATE|os.O_RDWR, 0o600)
		if err != nil {
			return nil, err
		}
		if err := flock(f); err == nil {
			return &Mutex{f: f, name: name}, nil
		}
		f.Close()
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("timeout waiting for mutex %s", name)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func TryCreateInstanceMutex(name string) (*Mutex, bool, error) {
	m, err := AcquireMutex(name, 50*time.Millisecond)
	if err != nil {
		return nil, true, nil
	}
	return m, false, nil
}

func (m *Mutex) Close() {
	if m == nil || m.f == nil {
		return
	}
	_ = m.f.Close()
	m.f = nil
}
