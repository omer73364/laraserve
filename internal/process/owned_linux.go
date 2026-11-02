package process

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"time"
)

const guardCommand = "__laraserve-owned-exec"

// The guard's pipe closes on owner death, including SIGKILL. Unlike PDEATHSIG
// on the server itself, this also works when exec grants file capabilities.
func StartOwned(cmd *exec.Cmd) error {
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	read, write, err := os.Pipe()
	if err != nil {
		return err
	}
	originalPath := cmd.Path
	originalArgs := append([]string{}, cmd.Args[1:]...)
	descriptor := 3 + len(cmd.ExtraFiles)
	cmd.Path = executable
	cmd.Args = append([]string{executable, guardCommand, strconv.Itoa(descriptor), originalPath}, originalArgs...)
	cmd.ExtraFiles = append(cmd.ExtraFiles, read)
	if cmd.Cancel != nil {
		cmd.Cancel = func() error { return write.Close() }
		if cmd.WaitDelay < 25*time.Second {
			cmd.WaitDelay = 25 * time.Second
		}
	}
	if err = cmd.Start(); err != nil {
		read.Close()
		write.Close()
		return err
	}
	read.Close()
	ownedPipes.Store(cmd, write)
	return nil
}
func InitOwnership() error { return nil }
func init() {
	if len(os.Args) > 1 && os.Args[1] == guardCommand {
		if len(os.Args) < 4 {
			fmt.Fprintln(os.Stderr, "incomplete service guard invocation")
			os.Exit(2)
		}
		os.Exit(runGuard(os.Args[2:]))
	}
}
