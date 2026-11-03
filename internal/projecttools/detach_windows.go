package projecttools

import (
	"golang.org/x/sys/windows"
	"os/exec"
	"syscall"
)

func DetachTask(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.CREATE_NEW_PROCESS_GROUP | windows.DETACHED_PROCESS}
}
