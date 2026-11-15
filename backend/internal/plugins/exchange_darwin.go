package plugins

import "golang.org/x/sys/unix"

func exchangeDirectories(staged, live string) error {
	return unix.RenamexNp(staged, live, unix.RENAME_SWAP)
}
