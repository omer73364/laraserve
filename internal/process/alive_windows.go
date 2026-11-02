package process

import (
	"errors"
	"fmt"
	"golang.org/x/sys/windows"
)

func Alive(pid int) (bool, error) {
	if pid <= 0 {
		return false, fmt.Errorf("invalid process ID")
	}
	handle, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if errors.Is(err, windows.ERROR_INVALID_PARAMETER) {
		return false, nil
	}
	if errors.Is(err, windows.ERROR_ACCESS_DENIED) {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	defer windows.CloseHandle(handle)
	var code uint32
	if err = windows.GetExitCodeProcess(handle, &code); err != nil {
		return false, err
	}
	return code == 259, nil
}
