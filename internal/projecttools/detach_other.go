//go:build !linux && !darwin && !windows

package projecttools

import "os/exec"

func DetachTask(cmd *exec.Cmd) {}
