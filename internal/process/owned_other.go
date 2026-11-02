//go:build !linux && !windows

package process

import "os/exec"

func InitOwnership() error           { return nil }
func StartOwned(cmd *exec.Cmd) error { return cmd.Start() }
