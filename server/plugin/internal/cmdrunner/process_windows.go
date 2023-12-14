package cmdrunner

import (
	"syscall"
)

const (
	exit_STILL_ACTIVE = 259

	processDesiredAccess = syscall.STANDARD_RIGHTS_READ |
		syscall.PROCESS_QUERY_INFORMATION |
		syscall.SYNCHRONIZE
)

func _pidAlive(pid int) bool {
	h, err := syscall.OpenProcess(processDesiredAccess, false, uint32(pid))
	if err != nil {
		return false
	}
	defer syscall.CloseHandle(h)

	var ec uint32
	if e := syscall.GetExitCodeProcess(h, &ec); e != nil {
		return false
	}

	return ec == exit_STILL_ACTIVE
}
