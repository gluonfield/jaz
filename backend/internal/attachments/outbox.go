package attachments

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"unicode"
)

func ValidFilename(name string) bool {
	return strings.TrimSpace(name) != "" && name != "." && filepath.IsLocal(name) && !strings.ContainsAny(name, `/\`) && strings.IndexFunc(name, unicode.IsControl) < 0
}

func OpenOutboxFile(cwd, name string) (*os.File, error) {
	if !ValidFilename(name) {
		return nil, errors.New("register_file requires a file name in .jaz-outbox")
	}
	working, err := os.OpenRoot(cwd)
	if err != nil {
		return nil, err
	}
	defer working.Close()
	const outbox = ".jaz-outbox"
	info, err := working.Lstat(outbox)
	if err != nil || !info.IsDir() {
		return nil, errors.New(".jaz-outbox must be a regular directory")
	}
	root, err := working.OpenRoot(outbox)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	openedOutbox, err := root.Stat(".")
	if err != nil || !os.SameFile(info, openedOutbox) {
		return nil, errors.New(".jaz-outbox changed while it was being opened")
	}
	info, err = root.Lstat(name)
	if err != nil || !info.Mode().IsRegular() {
		return nil, errors.New("outbox entry must be a regular file")
	}
	source, err := root.OpenFile(name, outboxReadFlags, 0)
	if err != nil {
		return nil, err
	}
	opened, err := source.Stat()
	if err != nil || !os.SameFile(info, opened) {
		source.Close()
		return nil, errors.New("file changed while it was being registered")
	}
	return source, nil
}
