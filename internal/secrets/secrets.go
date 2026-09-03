package secrets

import (
	"os"

	"localaihub/internal/platform"
	"localaihub/internal/wire"
)

func Set(target, secret string) error {
	if target == "" {
		return wire.E("INVALID_ARGUMENT", "target is required")
	}
	if secret == "" {
		return wire.E("INVALID_ARGUMENT", "empty secret")
	}
	return platform.WriteCredential(target, secret)
}

func Get(target string) (string, error) {
	s, err := platform.ReadCredential(target)
	if err != nil {
		if os.IsNotExist(err) {
			return "", wire.E("AUTH_FAILED", "secret not set")
		}
		return "", wire.E("AUTH_FAILED", "secret not set")
	}
	return s, nil
}

func Delete(target string) error {
	err := platform.DeleteCredential(target)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
