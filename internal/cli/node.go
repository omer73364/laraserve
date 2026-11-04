package cli

import (
	"context"
	"fmt"
	"laraserve/internal/frontend"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"

	"laraserve/internal/filelock"
	"laraserve/internal/paths"
	"laraserve/internal/process"
	"laraserve/internal/runtime"
	"laraserve/internal/versions"
)

// Explicit Node/npm commands share the project-selected runtime with automatic
// frontend preparation and Vite.
func cmdNode(args []string, npm bool) error {
	if err := process.InitOwnership(); err != nil {
		return err
	}
	target := "."
	if len(args) > 0 && args[0] != "--" {
		target = args[0]
		args = args[1:]
	}
	if len(args) > 0 && args[0] == "--" {
		args = args[1:]
	}
	if len(args) == 0 {
		args = []string{"--version"}
	}
	root, err := filepath.Abs(target)
	if err != nil {
		return err
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return err
	}
	home, err := paths.New()
	if err != nil {
		return err
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	release, err := filelock.Acquire(ctx, filepath.Join(home.Root, "startup.lock"))
	if err != nil {
		return err
	}
	restore := runtime.UseDownloadContext(ctx)
	node, err := projectNode(home.Runtimes(), root)
	restore()
	release()
	if err != nil {
		return err
	}
	if npm {
		manager, _, err := versions.PackageManager(root)
		if err != nil {
			return err
		}
		if manager != "npm" {
			return fmt.Errorf("project uses %s; refusing to change its lockfile with npm", manager)
		}
		args = append([]string{node.NPM}, args...)
	}
	cmd := exec.CommandContext(ctx, node.Node, args...)
	cmd.Dir = root
	cmd.Env = nodeEnvironment(node, os.Environ())
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return process.RunOwned(cmd)
}
func projectNode(cache, root string) (*runtime.NodeRuntime, error) {
	return frontend.Select(cache, root)
}
func nodeEnvironment(node *runtime.NodeRuntime, base []string) []string {
	return frontend.Environment(node, base)
}
