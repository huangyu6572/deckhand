//go:build !windows

package platform

import (
	"fmt"
	"os"
	"path/filepath"
)

func CurrentSID() (string, error) {
	return fmt.Sprintf("uid-%d", os.Getuid()), nil
}

func PipeName(sid string) string {
	dir, err := DataDir()
	if err != nil {
		return filepath.Join(os.TempDir(), ProductName+"-"+sid+".sock")
	}
	return filepath.Join(dir, "runtime", "hubd.sock")
}

func BootstrapMutexName(sid string) string { return ProductName + "-bootstrap-" + sid }
func InstanceMutexName(sid string) string  { return ProductName + "-hubd-" + sid }

func HighIntegrity() (bool, error) { return false, nil }
