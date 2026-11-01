package cli

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"laraserve/internal/detect"
	"laraserve/internal/env"
	"laraserve/internal/paths"
	"laraserve/internal/prepare"
	"laraserve/internal/runtime"
	"laraserve/internal/ui"
	"laraserve/internal/versions"
)

func cmdNew(args []string) error {
	fs := flag.NewFlagSet("new", flag.ContinueOnError)
	version := fs.String("version", "", "Laravel skeleton version constraint (e.g. ^12.0)")
	phpVersion := fs.String("php-version", "", "PHP version for the new project (e.g. 8.3)")
	allowInsecure := fs.Bool("allow-insecure-dependencies", false, "allow Composer dependencies with known security advisories for legacy projects")
	hostPath := fs.Bool("host-path", false, "allow host tools on PATH")
	fs.Usage = func() {
		fmt.Fprintln(fs.Output(), "Usage: laraserve new <path> [--version CONSTRAINT] [--php-version V] [--host-path]")
		fs.PrintDefaults()
	}
	if err := parse(fs, args); err != nil {
		return err
	}
	if fs.NArg() != 1 || strings.TrimSpace(fs.Arg(0)) == "" {
		return fmt.Errorf("usage: laraserve new <path> [--version CONSTRAINT] [--php-version V] [--host-path]")
	}
	root, err := filepath.Abs(fs.Arg(0))
	if err != nil {
		return err
	}
	entries, err := os.ReadDir(root)
	if err == nil && len(entries) > 0 {
		return fmt.Errorf("destination %s is not empty; choose a new or empty directory", root)
	}
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("destination: %w", err)
	}
	h, err := paths.New()
	if err != nil {
		return err
	}
	var rt *runtime.Runtime
	if *phpVersion != "" {
		selected, selectErr := versions.ProjectPHP(root, runtime.PHPVersions(), *phpVersion, "")
		if selectErr != nil {
			return selectErr
		}
		rt, err = runtime.EnsurePHP(h.Runtimes(), selected)
	} else {
		rt, err = runtime.Ensure(h.Runtimes())
	}
	if err != nil {
		return err
	}
	if err := runtime.EnsureComposer(rt); err != nil {
		return err
	}
	stateDir := filepath.Join(h.Projects(), paths.ProjectID(root))
	baseEnv := env.Build(rt, stateDir, *hostPath, nil)
	if *allowInsecure {
		ui.Warn("Allowing Composer dependencies with known security advisories for this installation")
	}
	ui.Info("Creating Laravel app in %s (needs network or a Composer cache)", root)
	if err := prepare.ComposerCreateProject(rt.PHP, rt.Composer, root, strings.TrimSpace(*version), baseEnv, *allowInsecure); err != nil {
		return fmt.Errorf("composer create-project failed: %w\n  Partial files may remain in %s; inspect them before retrying", err, root)
	}
	if _, err := detect.Inspect(root); err != nil {
		return fmt.Errorf("created project could not be verified: %w", err)
	}
	if *phpVersion != "" {
		if err := os.WriteFile(filepath.Join(root, ".php-version"), []byte(*phpVersion+"\n"), 0644); err != nil {
			return err
		}
	}
	ui.Fields("Project created",
		ui.Field{Label: "Project", Value: root},
		ui.Field{Label: "Next", Value: fmt.Sprintf("laraserve serve %q", root)},
	)
	return nil
}
