//go:build linux || darwin

package projecttools

import (
	"os/exec"
	"syscall"
)

func DetachTask(cmd *exec.Cmd) { cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true} }
