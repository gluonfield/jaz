package worktrees

import "golang.org/x/sys/windows"

func freeBytes(path string) (uint64, error) {
	native, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return 0, err
	}
	var available uint64
	err = windows.GetDiskFreeSpaceEx(native, &available, nil, nil)
	return available, err
}
