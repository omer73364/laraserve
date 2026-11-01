package cli

import (
	"flag"
	"fmt"
	"path/filepath"
	"strings"

	"laraserve/internal/detect"
	"laraserve/internal/paths"
	"laraserve/internal/runtime"
	"laraserve/internal/ui"
	"laraserve/internal/versions"
)

func cmdVersions(args []string) error {
	fs := flag.NewFlagSet("versions", flag.ContinueOnError)
	if err := parse(fs, args); err != nil {
		return err
	}
	if fs.NArg() > 1 {
		return fmt.Errorf("usage: laraserve versions [path]")
	}
	root := "."
	if fs.NArg() == 1 {
		root = fs.Arg(0)
	}
	root, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	c, err := load()
	if err != nil {
		return err
	}
	locked, err := versions.ReadLock(filepath.Join(c.home.Projects(), paths.ProjectID(root), "runtime-versions.json"))
	if err != nil {
		return err
	}
	php, err := versions.ProjectPHP(root, runtime.PHPVersions(), "", locked.PHP)
	if err != nil {
		return err
	}
	ui.Info("PHP: %s", php)
	conn := detect.DatabaseConnection(root)
	if conn == "mysql" || conn == "mariadb" {
		mysql, err := versions.ProjectMySQL(root, runtime.MySQLVersions(), "", locked.MySQL)
		if err != nil {
			return err
		}
		ui.Info("MySQL: %s", mysql)
	}
	ui.Info("Available PHP: %s", strings.Join(runtime.PHPVersions(), ", "))
	ui.Info("Available MySQL: %s", strings.Join(runtime.MySQLVersions(), ", "))
	return nil
}
