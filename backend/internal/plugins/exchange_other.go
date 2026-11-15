//go:build !darwin && !linux

package plugins

import "errors"

func exchangeDirectories(staged, live string) error {
	return errors.New("atomic plugin replacement is unsupported on this platform; the installed plugin was preserved")
}
