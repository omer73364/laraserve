package frontend

import (
	"context"
	"encoding/json"
	"fmt"
	"laraserve/internal/activity"
	"laraserve/internal/process"
	"laraserve/internal/projectconfig"
	"laraserve/internal/runtime"
	"laraserve/internal/versions"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func NeedsVite(root string) bool {
	b, e := projectconfig.ReadRegular(filepath.Join(root, "package.json"), 1<<20)
	if e != nil {
		return false
	}
	var p struct {
		Dependencies, DevDependencies map[string]string
		Scripts                       map[string]string
	}
	if json.Unmarshal(b, &p) != nil {
		return false
	}
	return p.Dependencies["vite"] != "" || p.DevDependencies["vite"] != ""
}

// EnsureDependencies is called only for a selected/detected frontend. It also
// installs after manifest/lock changes and when node_modules was removed.
func EnsureDependencies(ctx context.Context, cache, root, stateDir string) (*runtime.NodeRuntime, error) {
	node, e := Select(cache, root)
	if e != nil {
		return nil, e
	}
	manager, _, e := versions.PackageManager(root)
	if e != nil {
		return nil, e
	}
	stamp := ""
	for _, name := range []string{"package.json", "package-lock.json", "npm-shrinkwrap.json", "yarn.lock", "pnpm-lock.yaml"} {
		b, err := projectconfig.ReadRegular(filepath.Join(root, name), 32<<20)
		if err == nil {
			stamp += name + projectconfig.Digest(b)
		} else if !os.IsNotExist(err) {
			return nil, err
		}
	}
	stamp = projectconfig.Digest([]byte(stamp + node.Version))
	marker := filepath.Join(stateDir, "frontend-install.digest")
	old, _ := os.ReadFile(marker)
	previousNode, _ := os.ReadFile(filepath.Join(stateDir, "frontend-node.version"))
	_, modules := os.Stat(filepath.Join(root, "node_modules"))
	_, vite := os.Stat(filepath.Join(root, "node_modules", "vite", "dist", "node", "index.js"))
	if string(old) == stamp && modules == nil && (!NeedsVite(root) || vite == nil) {
		return node, nil
	}
	activity.Detail("Node " + node.Version + " · installing " + manager + " dependencies")
	var cmd *exec.Cmd
	if manager == "npm" {
		cmd = exec.CommandContext(ctx, node.Node, node.NPM, "install", "--no-fund")
	} else {
		// Corepack honors packageManager pins; it is never used to rewrite another manager's lockfile.
		tool, err := exec.LookPath(manager)
		if err != nil {
			return nil, fmt.Errorf("this project uses %s; install %s (or enable Corepack) so LaraServe can install its dependencies", manager, manager)
		}
		cmd = exec.CommandContext(ctx, tool, "install")
	}
	cmd.Dir = root
	cmd.Env = Environment(node, os.Environ())
	output := &activity.Output{Destination: os.Stdout}
	cmd.Stdout, cmd.Stderr = output, output
	if e = process.RunOwned(cmd); e != nil {
		return nil, dependencyInstallError(manager, output.Error(e))
	}
	// A freshly generated lock can reveal stricter build-tool engine constraints.
	compatible, e := Select(cache, root)
	if e != nil {
		return nil, e
	}
	if compatible.Version != node.Version || (modules == nil && string(previousNode) != compatible.Version) {
		node = compatible
		activity.Detail("Rebuilding dependencies with compatible Node " + node.Version)
		if manager == "npm" {
			rebuild := exec.CommandContext(ctx, node.Node, node.NPM, "rebuild")
			rebuild.Dir = root
			rebuild.Env = Environment(node, os.Environ())
			rebuild.Stdout, rebuild.Stderr = output, output
			if e = process.RunOwned(rebuild); e != nil {
				return nil, output.Error(e)
			}
		}
	}

	// Installation may create/update a lockfile: fingerprint the resulting input.
	stamp = ""
	for _, name := range []string{"package.json", "package-lock.json", "npm-shrinkwrap.json", "yarn.lock", "pnpm-lock.yaml"} {
		b, err := projectconfig.ReadRegular(filepath.Join(root, name), 32<<20)
		if err == nil {
			stamp += name + projectconfig.Digest(b)
		}
	}
	if e = os.MkdirAll(stateDir, 0700); e != nil {
		return nil, e
	}
	if e = os.WriteFile(filepath.Join(stateDir, "frontend-node.version"), []byte(node.Version), 0600); e != nil {
		return nil, e
	}
	if e = os.WriteFile(marker, []byte(projectconfig.Digest([]byte(stamp+node.Version))), 0600); e != nil {
		return nil, e
	}
	return node, nil
}

func dependencyInstallError(manager string, err error) error {
	text := err.Error()
	if manager == "npm" && (strings.Contains(text, "ERESOLVE") || strings.Contains(text, "Could not resolve dependency:")) {
		return fmt.Errorf("npm dependency conflict: packages require incompatible peer dependency versions. Check the package and required version below and align them in package.json. Changing Node alone will not fix this. For an older app known to work with legacy npm, you can opt in by adding legacy-peer-deps=true to the project's .npmrc; this ignores peer dependency checks and does not fix incompatibilities. Details: %w", err)
	}
	return fmt.Errorf("%s install failed: %w", manager, err)
}

func HasPackage(root string) bool {
	_, e := projectconfig.ReadRegular(filepath.Join(root, "package.json"), 1<<20)
	return e == nil
}

// Build prepares assets when the live Vite server is not selected.
func Build(ctx context.Context, node *runtime.NodeRuntime, root string) error {
	b, e := projectconfig.ReadRegular(filepath.Join(root, "package.json"), 1<<20)
	if e != nil {
		return e
	}
	var p struct {
		Scripts                       map[string]string
		DevDependencies, Dependencies map[string]string
	}
	if e = json.Unmarshal(b, &p); e != nil {
		return e
	}
	script := "build"
	if p.Scripts[script] == "" {
		if p.Scripts["production"] != "" && (p.DevDependencies["laravel-mix"] != "" || p.Dependencies["laravel-mix"] != "") {
			script = "production"
		} else {
			return nil
		}
	}
	manager, _, e := versions.PackageManager(root)
	if e != nil {
		return e
	}
	activity.Detail("Building frontend assets · " + manager + " run " + script)
	var cmd *exec.Cmd
	if manager == "npm" {
		cmd = exec.CommandContext(ctx, node.Node, node.NPM, "run", script)
	} else {
		tool, err := exec.LookPath(manager)
		if err != nil {
			return err
		}
		cmd = exec.CommandContext(ctx, tool, "run", script)
	}
	cmd.Dir = root
	cmd.Env = Environment(node, os.Environ())
	output := &activity.Output{Destination: os.Stdout}
	cmd.Stdout, cmd.Stderr = output, output
	if e = process.RunOwned(cmd); e != nil {
		return fmt.Errorf("frontend asset build failed: %w", output.Error(e))
	}
	return nil
}
