package cli

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"laraserve/internal/filelock"
	"laraserve/internal/paths"
	"laraserve/internal/process"
	"laraserve/internal/runtime"
	"laraserve/internal/versions"
)

// Explicit Node/npm commands use the project's selected runtime. No dependencies
// or lifecycle scripts are installed merely because an application starts.
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
	if err := os.MkdirAll(cache, 0700); err != nil {
		return nil, err
	}
	available, err := runtime.NodeVersions(cache)
	if err != nil {
		return nil, err
	}
	lockFile := filepath.Join(cache, "node-project-"+paths.ProjectID(root)+".json")
	lock, err := versions.ReadLock(lockFile)
	if err != nil {
		return nil, err
	}
	selected, err := versions.ProjectNode(root, available, "", lock.Node)
	if err != nil {
		return nil, err
	}
	node, err := runtime.EnsureNode(cache, selected)
	if err != nil {
		return nil, err
	}
	if err = versions.WriteLock(lockFile, versions.Pins{Node: selected}); err != nil {
		return nil, err
	}
	return node, nil
}
func nodeEnvironment(node *runtime.NodeRuntime, base []string) []string {
	result := make([]string, 0, len(base)+1)
	path := filepath.Dir(node.Node)
	for _, entry := range base {
		key, value, ok := strings.Cut(entry, "=")
		if ok && strings.EqualFold(key, "PATH") {
			path += string(os.PathListSeparator) + value
			continue
		}
		result = append(result, entry)
	}
	return append(result, "PATH="+path)
}
