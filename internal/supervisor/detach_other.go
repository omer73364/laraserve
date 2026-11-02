//go:build !linux && !darwin && !windows

package supervisor

import "os/exec"

func detach(cmd *exec.Cmd) {}
