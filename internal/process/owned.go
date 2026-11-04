package process

import (
	"bytes"
	"fmt"
	"os/exec"
	"sync"
)

func RunOwned(cmd *exec.Cmd) error {
	if err := StartOwned(cmd); err != nil {
		return err
	}
	return WaitOwned(cmd)
}
func CombinedOutputOwned(cmd *exec.Cmd) ([]byte, error) {
	if cmd.Stdout != nil || cmd.Stderr != nil {
		return nil, fmt.Errorf("output already configured")
	}
	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output
	err := RunOwned(cmd)
	return output.Bytes(), err
}

var ownedPipes sync.Map

func WaitOwned(cmd *exec.Cmd) error {
	err := cmd.Wait()
	if pipe, ok := ownedPipes.LoadAndDelete(cmd); ok {
		_ = pipe.(interface{ Close() error }).Close()
	}
	return err
}
