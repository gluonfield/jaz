package filelock

import "os"

func Lock(path string) (func(), error) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	if err := lock(file); err != nil {
		file.Close()
		return nil, err
	}
	return func() {
		_ = unlock(file)
		_ = file.Close()
	}, nil
}
