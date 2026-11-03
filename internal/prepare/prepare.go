// Package prepare creates per-project state and builds the Laravel
// environment variables. The project is not modified: the app key, SQLite
// database, storage and caches all live in ~/.laraserve/projects/<id>/.
package prepare

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"laraserve/internal/process"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"laraserve/internal/detect"
)

type State struct {
	Dir     string
	Key     string
	SQLite  string
	Storage string
	Fresh   bool // SQLite database was just created
}

// EnsureState creates directories, the app key and an empty SQLite file.
func EnsureState(dir string) (*State, error) {
	s := &State{
		Dir:     dir,
		SQLite:  filepath.Join(dir, "database.sqlite"),
		Storage: filepath.Join(dir, "storage"),
	}
	for _, d := range []string{
		"app/public", "framework/cache/data", "framework/sessions", "framework/views", "logs",
	} {
		if err := os.MkdirAll(filepath.Join(s.Storage, d), 0o755); err != nil {
			return nil, err
		}
	}
	if err := os.MkdirAll(filepath.Join(dir, "cache"), 0o755); err != nil {
		return nil, err
	}
	keyFile := filepath.Join(dir, "app.key")
	if b, err := os.ReadFile(keyFile); err == nil {
		s.Key = strings.TrimSpace(string(b))
	} else {
		raw := make([]byte, 32)
		if _, err := rand.Read(raw); err != nil {
			return nil, err
		}
		s.Key = "base64:" + base64.StdEncoding.EncodeToString(raw)
		if err := os.WriteFile(keyFile, []byte(s.Key), 0o600); err != nil {
			return nil, err
		}
	}
	if _, err := os.Stat(s.SQLite); os.IsNotExist(err) {
		if err := os.WriteFile(s.SQLite, nil, 0o644); err != nil {
			return nil, err
		}
		s.Fresh = true
	}
	return s, nil
}

// AppEnv returns the Laravel variables laraserve injects through the process
// environment (real environment variables win over .env in Laravel).
func (s *State) AppEnv(projectRoot, appURL string) map[string]string {
	dot := detect.DotEnv(filepath.Join(projectRoot, ".env"))
	env := map[string]string{
		"APP_URL":              appURL,
		"LARAVEL_STORAGE_PATH": s.Storage,
		"APP_SERVICES_CACHE":   filepath.Join(s.Dir, "cache", "services.php"),
		"APP_PACKAGES_CACHE":   filepath.Join(s.Dir, "cache", "packages.php"),
		"APP_CONFIG_CACHE":     filepath.Join(s.Dir, "cache", "config.php"),
		"APP_ROUTES_CACHE":     filepath.Join(s.Dir, "cache", "routes-v7.php"),
		"APP_EVENTS_CACHE":     filepath.Join(s.Dir, "cache", "events.php"),
	}
	if dot["APP_KEY"] == "" {
		env["APP_KEY"] = s.Key
	}
	conn := detect.DatabaseConnection(projectRoot)
	if conn == "" || conn == "sqlite" {
		env["DB_CONNECTION"] = "sqlite"
		env["DB_DATABASE"] = s.SQLite
	}
	return env
}

// Artisan runs `php artisan args...` with the cached PHP.
func Artisan(php, root string, env []string, args ...string) error {
	return artisanContext(context.Background(), php, root, env, args...)
}

func artisanContext(ctx context.Context, php, root string, env []string, args ...string) error {
	full := append([]string{"artisan"}, args...)
	cmd := process.PHPContext(ctx, php, full...)
	cmd.Dir, cmd.Env = root, env
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	return process.RunOwned(cmd)
}

// EnsureMigrations runs only when explicitly requested. Fresh databases and
// failures from an earlier run must never trigger schema changes on startup.
func (s *State) EnsureMigrations(php, root string, env []string, requested bool) error {
	return s.EnsureMigrationsContext(context.Background(), php, root, env, requested)
}

// EnsureMigrationsContext cancels an explicitly requested migration if its
// managed database stops.
func (s *State) EnsureMigrationsContext(ctx context.Context, php, root string, env []string, requested bool) error {
	if !requested {
		return nil
	}
	if err := artisanContext(ctx, php, root, env, "migrate", "--force", "--no-interaction"); err != nil {
		return fmt.Errorf("database migrations failed: %w; fix the migration error and rerun with --migrate", err)
	}
	return nil
}

// ComposerInstall runs `composer install` (needs network unless cached).
func ComposerInstall(php, composerPhar, root string, env []string, allowInsecure ...bool) error {
	return ComposerInstallContext(context.Background(), php, composerPhar, root, env, allowInsecure...)
}
func ComposerInstallContext(ctx context.Context, php, composerPhar, root string, env []string, allowInsecure ...bool) error {
	env = composerPolicyEnv(env, allowInsecure)
	cmd := process.PHPContext(ctx, php, composerPhar, "install",
		"--no-interaction", "--prefer-dist", "--no-progress", "--no-scripts", "--no-plugins")
	cmd.Dir = root
	return runComposer(cmd, php, env)
}

// runComposer supplies a PHP CLI launcher for Composer's @php scripts.
// PHP_BINARY otherwise points at FrankenPHP, which interprets artisan as a
// server subcommand. Keep the launcher private and alive until scripts finish.
func runComposer(cmd *exec.Cmd, php string, env []string) error {
	if runtime.GOOS == "windows" {
		cmd.Env = append([]string{}, env...)
		cmd.Env = append(cmd.Env, "PHP_BINARY="+php)
		cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
		return process.RunOwned(cmd)
	}
	php, err := filepath.Abs(php)
	if err != nil {
		return err
	}
	dir, err := os.MkdirTemp("", "laraserve-php-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	launcher := filepath.Join(dir, "php")
	quote := func(s string) string { return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'" }
	// Composer puts PHP ini overrides before the script. FrankenPHP does not
	// support -d, so drop those overrides while preserving script arguments.
	script := "#!/bin/sh\nwhile [ \"$#\" -gt 0 ]; do\n" +
		"  case \"$1\" in\n" +
		"    -d) [ \"$#\" -ge 2 ] || exit 2; shift 2 ;;\n" +
		"    -d*) shift ;;\n" +
		"    *) break ;;\n" +
		"  esac\ndone\nexec " + quote(php) + " php-cli \"$@\"\n"
	if err := os.WriteFile(launcher, []byte(script), 0o700); err != nil {
		return err
	}
	cmd.Env = make([]string, 0, len(env)+2)
	path := ""
	for _, entry := range env {
		if strings.HasPrefix(entry, "PHP_BINARY=") {
			continue
		}
		if strings.HasPrefix(entry, "PATH=") {
			path = strings.TrimPrefix(entry, "PATH=")
			continue
		}
		cmd.Env = append(cmd.Env, entry)
	}
	cmd.Env = append(cmd.Env, "PHP_BINARY="+launcher, "PATH="+dir+string(os.PathListSeparator)+path)
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	return process.RunOwned(cmd)
}

// ComposerCreateProject scaffolds an app using the cached PHP and Composer.
func ComposerCreateProject(php, composerPhar, root, version string, env []string, allowInsecure ...bool) error {
	env = composerPolicyEnv(env, allowInsecure)
	args := []string{composerPhar, "create-project",
		"--no-interaction", "--prefer-dist", "--no-progress", "--no-scripts", "--", "laravel/laravel", root}
	if version != "" {
		args = append(args, version)
	}
	cmd := process.PHP(php, args...)
	if err := runComposer(cmd, php, env); err != nil {
		return err
	}
	// Laravel's post-create scripts may migrate automatically. Skip those hooks,
	// but retain the standard environment file setup without changing the schema.
	target := filepath.Join(root, ".env")
	if _, err := os.Stat(target); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}
	example, err := os.ReadFile(filepath.Join(root, ".env.example"))
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	f, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	_, err = f.Write(example)
	closeErr := f.Close()
	if err != nil {
		return err
	}
	return closeErr
}

// The opt-in affects known advisory blocking only, not malware policies or PHP
// platform checks, and lives only in this Composer process's environment.
func composerPolicyEnv(env []string, allow []bool) []string {
	if len(allow) == 0 || !allow[0] {
		return env
	}
	out := make([]string, 0, len(env)+1)
	for _, entry := range env {
		if !strings.HasPrefix(entry, "COMPOSER_POLICY_ADVISORIES_BLOCK=") {
			out = append(out, entry)
		}
	}
	return append(out, "COMPOSER_POLICY_ADVISORIES_BLOCK=0")
}

// ComposerTask runs an explicit action using the same PHP launcher as install.
func ComposerTask(ctx context.Context, php, root string, env []string, args []string) error {
	cmd := process.PHPContext(ctx, php, args...)
	cmd.Dir = root
	return runComposer(cmd, php, env)
}

// ArtisanContext runs a deliberate command with a cancellable owned lifetime.
func ArtisanContext(ctx context.Context, php, root string, environment []string, args ...string) error {
	return artisanContext(ctx, php, root, environment, args...)
}
func ComposerCreatePackage(ctx context.Context, php, composer, root, pkg, version string, environment []string, allow bool) error {
	args := []string{composer, "create-project", "--no-interaction", "--prefer-dist", "--no-progress", "--no-scripts", "--", pkg, root}
	if version != "" {
		args = append(args, version)
	}
	cmd := process.PHPContext(ctx, php, args...)
	if err := runComposer(cmd, php, composerPolicyEnv(environment, []bool{allow})); err != nil {
		return err
	}
	target := filepath.Join(root, ".env")
	if _, err := os.Stat(target); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}
	example, err := os.ReadFile(filepath.Join(root, ".env.example"))
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	return os.WriteFile(target, example, 0600)
}
func InteractivePHPTools(ctx context.Context, php, composer, root string, environment []string, args []string) error {
	dir, err := os.MkdirTemp("", "laraserve-installer-tools-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	quote := func(s string) string { return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'" }
	phpBinary := php
	if runtime.GOOS != "windows" {
		phpBinary = filepath.Join(dir, "php")
		script := "#!/bin/sh\nwhile [ \"$#\" -gt 0 ]; do\n case \"$1\" in\n -d) shift 2 ;;\n -d*) shift ;;\n *) break ;;\n esac\ndone\nexec " + quote(php) + " php-cli \"$@\"\n"
		if err = os.WriteFile(phpBinary, []byte(script), 0700); err != nil {
			return err
		}
		if err = os.WriteFile(filepath.Join(dir, "composer"), []byte("#!/bin/sh\nexec "+quote(phpBinary)+" "+quote(composer)+" \"$@\"\n"), 0700); err != nil {
			return err
		}
	} else {
		escape := func(s string) string { return strings.ReplaceAll(s, "%", "%%") }
		script := "@echo off\r\n\"" + escape(php) + "\" \"" + escape(composer) + "\" %*\r\n"
		if err = os.WriteFile(filepath.Join(dir, "composer.bat"), []byte(script), 0600); err != nil {
			return err
		}
	}
	var out []string
	path := ""
	for _, v := range environment {
		if strings.HasPrefix(v, "PATH=") {
			path = strings.TrimPrefix(v, "PATH=")
			continue
		}
		if strings.HasPrefix(v, "PHP_BINARY=") || strings.HasPrefix(v, "COMPOSER_NO_INTERACTION=") {
			continue
		}
		out = append(out, v)
	}
	out = append(out, "PATH="+dir+string(os.PathListSeparator)+path, "PHP_BINARY="+phpBinary)
	cmd := process.PHPContext(ctx, php, args...)
	cmd.Dir, cmd.Env = root, out
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	// Keep the interactive child in the foreground terminal process group.
	// Noninteractive tasks continue to use owned pipe guards.
	return cmd.Run()
}
