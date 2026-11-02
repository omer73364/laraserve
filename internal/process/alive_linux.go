package process

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"syscall"
)

func Alive(pid int) (bool, error) {
	if pid <= 0 {
		return false, fmt.Errorf("invalid process ID")
	}
	b, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat")
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	end := strings.LastIndex(string(b), ")")
	if end < 0 || end+2 >= len(b) {
		return false, fmt.Errorf("invalid process status")
	}
	if b[end+2] == 'Z' || b[end+2] == 'X' {
		return false, nil
	}
	err = syscall.Kill(pid, 0)
	if errors.Is(err, syscall.ESRCH) {
		return false, nil
	}
	if err == nil || errors.Is(err, syscall.EPERM) {
		return true, nil
	}
	return false, err
}
