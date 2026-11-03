package cli

import (
	"context"
	"flag"
	"fmt"
	"laraserve/internal/process"
	"laraserve/internal/scaffold"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"laraserve/internal/paths"

	"laraserve/internal/runtime"
)

func cmdNew(args []string) error {
	fs := flag.NewFlagSet("new", flag.ContinueOnError)
	version := fs.String("version", "", "Laravel skeleton version constraint (e.g. ^12.0)")
	phpVersion := fs.String("php-version", "", "PHP version for the new project (e.g. 8.3)")
	allowInsecure := fs.Bool("allow-insecure-dependencies", false, "allow Composer dependencies with known security advisories for legacy projects")
	starter := fs.String("starter", "none", "official starter package: none, react, vue, svelte, livewire")
	database := fs.String("database", "sqlite", "sqlite, mysql or external")
	migrate := fs.Bool("migrate", true, "apply migrations during new app creation only")
	wizard := fs.Bool("wizard", false, "run the official Laravel installer interactively")
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
	home, err := paths.New()
	if err != nil {
		return err
	}
	if err = process.InitOwnership(); err != nil {
		return err
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	restore := runtime.UseDownloadContext(ctx)
	defer restore()
	request := scaffold.Request{Path: root, Version: strings.TrimSpace(*version), PHP: *phpVersion, Starter: *starter, Database: *database, Migrate: migrate, OfficialWizard: *wizard, HostPath: *hostPath, AllowInsecure: *allowInsecure}
	return scaffold.Create(ctx, home.Root, request)
}
