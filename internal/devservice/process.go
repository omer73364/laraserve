// Package devservice runs optional development processes under the app owner.
package devservice

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"laraserve/internal/process"
)

type Command struct {
	Label, Root, Log, Address string
	Executable                string
	Args, Env                 []string
	PHP                       bool
}

func (c Command) Name() string { return c.Label }
func (c Command) Run(ctx context.Context, ready func()) error {
	if err := os.MkdirAll(filepath.Dir(c.Log), 0700); err != nil {
		return err
	}
	log, err := os.OpenFile(c.Log, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return err
	}
	defer log.Close()
	cmd := exec.CommandContext(ctx, c.Executable, c.Args...)
	if c.PHP {
		cmd = process.PHPContext(ctx, c.Executable, c.Args...)
	}
	cmd.Dir = c.Root
	cmd.Env = c.Env
	cmd.Stdout = log
	cmd.Stderr = log
	if err = process.StartOwned(cmd); err != nil {
		return err
	}
	done := make(chan error, 1)
	go func() { done <- process.WaitOwned(cmd) }()
	timer := time.NewTimer(30 * time.Second)
	defer timer.Stop()
	tick := time.NewTicker(100 * time.Millisecond)
	defer tick.Stop()
	// Workers have no listening socket; require them to survive startup before
	// reporting ready. Network services must actually accept a loopback connection.
	started := time.Now()
	isReady := false
	for {
		select {
		case err := <-done:
			if ctx.Err() != nil {
				return nil
			}
			if err == nil {
				err = fmt.Errorf("process exited")
			}
			return fmt.Errorf("%s: %w; see %s", c.Label, err, c.Log)
		case <-ctx.Done():
			<-done
			return nil
		case <-timer.C:
			if !isReady {
				return stopFailed(cmd, done, c.Label, c.Log)
			}
		case <-tick.C:
			if isReady {
				continue
			}
			if c.Address == "" {
				if time.Since(started) < 500*time.Millisecond {
					continue
				}
			} else {
				connection, err := net.DialTimeout("tcp", c.Address, 100*time.Millisecond)
				if err != nil {
					continue
				}
				connection.Close()
			}
			isReady = true
			ready()
		}
	}
}
func stopFailed(cmd *exec.Cmd, done <-chan error, label, log string) error {
	// CommandContext's cancellation closes the guard's parent pipe on Linux.
	if cmd.Cancel != nil {
		_ = cmd.Cancel()
	} else {
		_ = cmd.Process.Kill()
	}
	<-done
	return fmt.Errorf("%s did not become ready within 30 seconds; see %s", label, log)
}

// Reserve chooses a loopback-only address and keeps it occupied during runtime
// preparation. Release immediately before starting the corresponding process.
func Reserve() (net.Listener, error) { return net.Listen("tcp4", "127.0.0.1:0") }
