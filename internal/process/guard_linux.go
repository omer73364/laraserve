package process

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

func runGuard(args []string) int {
	descriptor, err := strconv.Atoi(args[0])
	if err != nil || descriptor < 3 {
		fmt.Fprintln(os.Stderr, "invalid service ownership descriptor")
		return 2
	}
	parent := os.NewFile(uintptr(descriptor), "service-owner")
	if parent == nil {
		return 2
	}
	defer parent.Close()
	child := exec.Command(args[1], args[2:]...)
	child.Stdin, child.Stdout, child.Stderr = os.Stdin, os.Stdout, os.Stderr
	child.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	signals := make(chan os.Signal, 2)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(signals)
	if err = child.Start(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	ownerGone := make(chan struct{})
	go func() { var b [1]byte; _, _ = parent.Read(b[:]); close(ownerGone) }()
	// Observe without reaping: retaining the group leader prevents PID/group ID
	// reuse until every signal to this owned process group has been sent.
	observed := make(chan error, 1)
	go func() {
		for {
			var info unix.Siginfo
			err := unix.Waitid(unix.P_PID, child.Process.Pid, &info, unix.WEXITED|unix.WNOWAIT, nil)
			if errors.Is(err, unix.EINTR) {
				continue
			}
			observed <- err
			return
		}
	}()
	stopped := false
	select {
	case err = <-observed:
	case <-ownerGone:
		stopped = true
	case <-signals:
		stopped = true
	}
	if stopped {
		_ = syscall.Kill(-child.Process.Pid, syscall.SIGTERM)
		select {
		case err = <-observed:
		case <-time.After(20 * time.Second):
			_ = syscall.Kill(-child.Process.Pid, syscall.SIGKILL)
			err = <-observed
		}
	}
	if err == nil {
		// Also remove descendants that outlived a completed command. The leader is
		// still unreaped, so this numeric group cannot refer to an unrelated group.
		_ = syscall.Kill(-child.Process.Pid, syscall.SIGKILL)
	}
	waitErr := child.Wait()
	if stopped {
		return 0
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "service wait:", err)
		return 1
	}
	if waitErr != nil {
		if exit, ok := waitErr.(*exec.ExitError); ok {
			return exit.ExitCode()
		}
		fmt.Fprintln(os.Stderr, waitErr)
		return 1
	}
	return 0
}
