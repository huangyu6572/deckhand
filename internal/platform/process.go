package platform

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

func SameDirExecutable(current, name string) (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	exe, err = filepath.EvalSymlinks(exe)
	if err != nil {
		exe, _ = os.Executable()
	}
	dir := filepath.Dir(exe)
	candidate := filepath.Join(dir, name)
	if runtime.GOOS == "windows" && filepath.Ext(candidate) == "" {
		candidate += ".exe"
	}
	if _, err := os.Stat(candidate); err != nil {
		return "", err
	}
	return candidate, nil
}

func StartDetached(exe string, args ...string) error {
	cmd := exec.Command(exe, args...)
	cmd.Dir = filepath.Dir(exe)
	cmd.Stdin = nil
	cmd.Stdout = nil
	cmd.Stderr = nil
	detach(cmd)
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}
