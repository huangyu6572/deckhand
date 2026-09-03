//go:build windows

package platform

import (
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	credTypeGeneric      = 1
	credPersistLocalMach = 2
)

var (
	modadvapi32     = windows.NewLazySystemDLL("advapi32.dll")
	procCredWriteW  = modadvapi32.NewProc("CredWriteW")
	procCredReadW   = modadvapi32.NewProc("CredReadW")
	procCredFree    = modadvapi32.NewProc("CredFree")
	procCredDeleteW = modadvapi32.NewProc("CredDeleteW")
)

type winCred struct {
	Flags              uint32
	Type               uint32
	TargetName         *uint16
	Comment            *uint16
	LastWritten        windows.Filetime
	CredentialBlobSize uint32
	CredentialBlob     *byte
	Persist            uint32
	AttributeCount     uint32
	Attributes         uintptr
	TargetAlias        *uint16
	UserName           *uint16
}

func credTarget(name string) string { return ProductName + "/" + name }

func WriteCredential(target, secret string) error {
	tn, err := windows.UTF16PtrFromString(credTarget(target))
	if err != nil {
		return err
	}
	blob := []byte(secret)
	c := winCred{
		Type:               credTypeGeneric,
		TargetName:         tn,
		CredentialBlobSize: uint32(len(blob)),
		Persist:            credPersistLocalMach,
	}
	if len(blob) > 0 {
		c.CredentialBlob = &blob[0]
	}
	r, _, e := procCredWriteW.Call(uintptr(unsafe.Pointer(&c)), 0)
	if r == 0 {
		return e
	}
	return nil
}

func ReadCredential(target string) (string, error) {
	tn, err := windows.UTF16PtrFromString(credTarget(target))
	if err != nil {
		return "", err
	}
	var cred *winCred
	r, _, e := procCredReadW.Call(uintptr(unsafe.Pointer(tn)), credTypeGeneric, 0, uintptr(unsafe.Pointer(&cred)))
	if r == 0 {
		return "", e
	}
	defer procCredFree.Call(uintptr(unsafe.Pointer(cred)))
	if cred.CredentialBlobSize == 0 || cred.CredentialBlob == nil {
		return "", nil
	}
	b := unsafe.Slice(cred.CredentialBlob, cred.CredentialBlobSize)
	out := make([]byte, len(b))
	copy(out, b)
	return string(out), nil
}

func DeleteCredential(target string) error {
	tn, err := windows.UTF16PtrFromString(credTarget(target))
	if err != nil {
		return err
	}
	r, _, e := procCredDeleteW.Call(uintptr(unsafe.Pointer(tn)), credTypeGeneric, 0)
	if r == 0 {
		if errno, ok := e.(syscall.Errno); ok && errno == windows.ERROR_NOT_FOUND {
			return nil
		}
		return e
	}
	return nil
}
