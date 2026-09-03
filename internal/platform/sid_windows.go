//go:build windows

package platform

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

func CurrentSID() (string, error) {
	tok := windows.GetCurrentProcessToken()
	user, err := tok.GetTokenUser()
	if err != nil {
		return "", err
	}
	return user.User.Sid.String(), nil
}

func PipeName(sid string) string {
	return `\\.\pipe\` + ProductName + `-` + sid
}

func BootstrapMutexName(sid string) string {
	return `Local\` + ProductName + `-bootstrap-` + sid
}

func InstanceMutexName(sid string) string {
	return `Local\` + ProductName + `-hubd-` + sid
}

func HighIntegrity() (bool, error) {
	tok := windows.GetCurrentProcessToken()
	var needed uint32
	err := windows.GetTokenInformation(tok, windows.TokenIntegrityLevel, nil, 0, &needed)
	if err != nil && err != windows.ERROR_INSUFFICIENT_BUFFER {
		return false, err
	}
	buf := make([]byte, needed)
	if err := windows.GetTokenInformation(tok, windows.TokenIntegrityLevel, &buf[0], needed, &needed); err != nil {
		return false, err
	}
	type sidAndAttr struct {
		Sid        *windows.SID
		Attributes uint32
	}
	label := (*sidAndAttr)(unsafe.Pointer(&buf[0]))
	count := int(label.Sid.SubAuthorityCount())
	if count == 0 {
		return false, nil
	}
	rid := label.Sid.SubAuthority(uint32(count - 1))
	return rid >= 0x3000, nil
}
