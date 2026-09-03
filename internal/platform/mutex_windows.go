//go:build windows

package platform

import (
	"fmt"
	"time"

	"golang.org/x/sys/windows"
)

type Mutex struct {
	handle windows.Handle
	name   string
}

func AcquireMutex(name string, timeout time.Duration) (*Mutex, error) {
	ptr, err := windows.UTF16PtrFromString(name)
	if err != nil {
		return nil, err
	}
	h, err := windows.CreateMutex(nil, false, ptr)
	if err != nil && err != windows.ERROR_ALREADY_EXISTS {
		return nil, err
	}
	wait := uint32(windows.INFINITE)
	if timeout >= 0 {
		wait = uint32(timeout / time.Millisecond)
	}
	ev, err := windows.WaitForSingleObject(h, wait)
	if err != nil {
		windows.CloseHandle(h)
		return nil, err
	}
	if ev == uint32(windows.WAIT_TIMEOUT) {
		windows.CloseHandle(h)
		return nil, fmt.Errorf("timeout waiting for mutex %s", name)
	}
	return &Mutex{handle: h, name: name}, nil
}

func TryCreateInstanceMutex(name string) (*Mutex, bool, error) {
	ptr, err := windows.UTF16PtrFromString(name)
	if err != nil {
		return nil, false, err
	}
	h, err := windows.CreateMutex(nil, true, ptr)
	if err == windows.ERROR_ALREADY_EXISTS {
		if h != 0 {
			windows.CloseHandle(h)
		}
		return nil, true, nil
	}
	if err != nil {
		return nil, false, err
	}
	return &Mutex{handle: h, name: name}, false, nil
}

func (m *Mutex) Close() {
	if m == nil || m.handle == 0 {
		return
	}
	windows.ReleaseMutex(m.handle)
	windows.CloseHandle(m.handle)
	m.handle = 0
}
