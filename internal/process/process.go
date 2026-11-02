// Package process handles differences between native Windows PHP and FrankenPHP.
package process

import (
	"context"
	"os"
	"os/exec"
	"runtime"
)

func PHPArgs(args ...string) []string {
	if runtime.GOOS == "windows" {
		return args
	}
	return append([]string{"php-cli"}, args...)
}
func PHP(php string, args ...string) *exec.Cmd { return exec.Command(php, PHPArgs(args...)...) }
func PHPContext(ctx context.Context, php string, args ...string) *exec.Cmd {
	return exec.CommandContext(ctx, php, PHPArgs(args...)...)
}
func Stop(cmd *exec.Cmd) error {
	if runtime.GOOS == "windows" {
		return cmd.Process.Kill()
	}
	return cmd.Process.Signal(os.Interrupt)
}
