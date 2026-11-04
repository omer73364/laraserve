package scaffold

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/term"
	"laraserve/internal/filelock"
	"laraserve/internal/prepare"
	"laraserve/internal/runtime"
)

// The official interactive installer runs only in a real terminal. The web
// creation form selects the official starter packages without impersonating a TTY.
func officialWizard(ctx context.Context, home string, rt *runtime.Runtime, root string, r Request, environment []string) error {
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		return fmt.Errorf("--wizard requires an interactive terminal; use --starter and --database for unattended creation")
	}
	if r.Database == "mysql" || r.Database == "external" {
		return fmt.Errorf("use --starter with --database=mysql or external for LaraServe database provisioning; the interactive installer currently supports SQLite creation")
	}
	if r.Version != "" {
		return fmt.Errorf("the official installer selects its own current Laravel version; omit --version")
	}
	if r.Migrate != nil && !*r.Migrate {
		return fmt.Errorf("the official wizard may migrate new projects; use the built-in --starter flow with --migrate=false to suppress this")
	}
	version := rt.PHPVersion
	if version == "" {
		version = "default"
	}
	dir := filepath.Join(home, "tools", "laravel-installer-"+version)
	if err := os.MkdirAll(filepath.Dir(dir), 0700); err != nil {
		return err
	}
	release, err := filelock.Acquire(ctx, dir+".lock")
	if err != nil {
		return err
	}
	defer release()
	launcher := filepath.Join(dir, "bin", "laravel")
	if _, err = os.Stat(filepath.Join(dir, ".ready")); err != nil {
		staging, err := os.MkdirTemp(filepath.Dir(dir), ".installer-")
		if err != nil {
			return err
		}
		defer os.RemoveAll(staging)
		destination := filepath.Join(staging, "installer")
		if err = prepare.ComposerTask(ctx, rt.PHP, filepath.Dir(dir), environment, []string{rt.Composer, "create-project", "--no-interaction", "--no-scripts", "--no-dev", "--no-plugins", "--prefer-dist", "--", "laravel/installer", destination, "^5.0"}); err != nil {
			return err
		}
		if _, err = os.Stat(filepath.Join(destination, "bin", "laravel")); err != nil {
			return err
		}
		if err = os.WriteFile(filepath.Join(destination, ".ready"), []byte("ready\n"), 0600); err != nil {
			return err
		}
		if _, err = os.Stat(dir); err == nil {
			return fmt.Errorf("incomplete installer cache exists; inspect %s", dir)
		}
		if err = os.Rename(destination, dir); err != nil {
			return err
		}
	}
	args := []string{launcher, "new", root, "--no-node", "--no-boost"}
	if r.Starter != "" && r.Starter != "none" {
		args = append(args, "--"+r.Starter)
	}
	if r.Database != "" && r.Database != "external" {
		args = append(args, "--database="+r.Database)
	}
	return prepare.InteractivePHPTools(ctx, rt.PHP, rt.Composer, filepath.Dir(root), environment, args)
}
