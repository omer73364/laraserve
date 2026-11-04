package frontend

import (
	"context"
	"encoding/json"
	"fmt"
	"laraserve/internal/activity"
	"laraserve/internal/process"
	"laraserve/internal/projectconfig"
	"laraserve/internal/runtime"
	"laraserve/internal/statefile"
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
	stamp, e := dependencyFingerprint(root, node.Version)
	if e != nil {
		return nil, e
	}
	marker := filepath.Join(stateDir, "frontend-install.digest")
	old, _ := os.ReadFile(marker)
	previousNode, _ := os.ReadFile(filepath.Join(stateDir, "frontend-node.version"))
	moduleInfo, modules := os.Stat(filepath.Join(root, "node_modules"))
	if modules == nil && !moduleInfo.IsDir() {
		return nil, fmt.Errorf("node_modules must be a directory; move the conflicting file before starting the app")
	}
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
			rebuildOutput := &activity.Output{Destination: os.Stdout}
			rebuild.Stdout, rebuild.Stderr = rebuildOutput, rebuildOutput
			if e = process.RunOwned(rebuild); e != nil {
				return nil, rebuildOutput.Error(e)
			}
		}
	}

	// Installation may create/update a lockfile: fingerprint the resulting input.
	stamp, e = dependencyFingerprint(root, node.Version)
	if e != nil {
		return nil, e
	}
	if e = os.MkdirAll(stateDir, 0700); e != nil {
		return nil, e
	}
	if e = statefile.WriteBytes(filepath.Join(stateDir, "frontend-node.version"), []byte(node.Version)); e != nil {
		return nil, e
	}
	if e = statefile.WriteBytes(marker, []byte(stamp)); e != nil {
		return nil, e
	}
	return node, nil
}

// Configuration changes affect resolution just as lockfile and Node changes do.
// Hash file contents only; registry credentials never appear in state or logs.
func dependencyFingerprint(root, nodeVersion string) (string, error) {
	stamp := "frontend-inputs-v2:" + nodeVersion
	for _, name := range []string{"package.json", "package-lock.json", "npm-shrinkwrap.json", "yarn.lock", "pnpm-lock.yaml", ".npmrc", ".yarnrc", ".yarnrc.yml", "pnpm-workspace.yaml"} {
		data, err := projectconfig.ReadRegular(filepath.Join(root, name), 32<<20)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return "", fmt.Errorf("cannot read frontend input %s: %w", name, err)
		}
		stamp += name + ":" + projectconfig.Digest(data)
	}
	return projectconfig.Digest([]byte(stamp)), nil
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
