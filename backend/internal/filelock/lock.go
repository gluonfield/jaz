package filelock

import "os"

func Lock(path string) (func(), error) {
	return acquire(path, true)
}

func TryLock(path string) (func(), error) {
	return acquire(path, false)
}

func acquire(path string, wait bool) (func(), error) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	if err := lock(file, wait); err != nil {
		file.Close()
		return nil, err
	}
	return func() {
		_ = unlock(file)
		_ = file.Close()
	}, nil
}
