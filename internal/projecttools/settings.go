// Package projecttools is the shared implementation behind CLI and dashboard
// settings, environment editing, project tasks and runtime management.
package projecttools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"laraserve/internal/filelock"
	"laraserve/internal/projectconfig"
	"laraserve/internal/projects"
	"laraserve/internal/runtime"
	"laraserve/internal/statefile"
	"laraserve/internal/versions"
)

func ConfigurePHP(rt *runtime.Runtime, stateDir string, c projectconfig.Config) error {
	directives, err := c.INI()
	if err != nil || directives == "" {
		return err
	}
	original, err := projectconfig.ReadRegular(filepath.Join(rt.INI, "php.ini"), 128<<10)
	if err != nil {
		return err
	}
	dir := filepath.Join(stateDir, "php")
	if err = os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	if err = statefile.WriteBytes(filepath.Join(dir, "php.ini"), append(original, []byte("\n; Project settings\n"+directives)...)); err != nil {
		return err
	}
	rt.INI = dir
	return nil
}
func SettingsRoot(p *projects.Project) (string, error) {
	stat, err := os.Stat(p.Source)
	if err != nil {
		return "", err
	}
	if !stat.IsDir() {
		return "", fmt.Errorf("bundles are read-only; edit the source project")
	}
	return p.Source, nil
}
func SaveSettings(ctx context.Context, home string, p *projects.Project, revision string, c projectconfig.Config) (projectconfig.Document, error) {
	if err := os.MkdirAll(taskDir(home), 0700); err != nil {
		return projectconfig.Document{}, err
	}
	taskRelease, taskOK, taskErr := filelock.Try(filepath.Join(taskDir(home), p.ID+".work.lock"))
	if taskErr != nil && !os.IsNotExist(taskErr) {
		return projectconfig.Document{}, taskErr
	}
	if taskErr == nil {
		if !taskOK {
			return projectconfig.Document{}, fmt.Errorf("wait for the project task before changing settings")
		}
		defer taskRelease()
	}
	release, err := filelock.Acquire(ctx, filepath.Join(home, "startup.lock"))
	if err != nil {
		return projectconfig.Document{}, err
	}
	defer release()
	if err := RequireNoTasks(home, p.ID); err != nil {
		return projectconfig.Document{}, err
	}
	root, err := SettingsRoot(p)
	if err != nil {
		return projectconfig.Document{}, err
	}
	if err = c.Validate(); err != nil {
		return projectconfig.Document{}, err
	}
	if c.PHP != "" {
		if _, err = versions.ProjectPHP(root, runtime.PHPVersions(), c.PHP, ""); err != nil {
			return projectconfig.Document{}, err
		}
	}
	// Validate the requested MySQL pin without allowing an old config pin to win.
	if c.MySQL != "" {
		if _, err = versions.ProjectMySQL(root, runtime.MySQLVersions(), c.MySQL, ""); err != nil {
			return projectconfig.Document{}, err
		}
	}
	result, err := projectconfig.Save(home, root, p.ID, revision, c)
	if err != nil {
		return result, err
	}
	// Remove stale remembered flags for fields now managed by the shareable file.
	latest, err := projects.New(home).Load(p.ID)
	if err != nil {
		return result, err
	}
	args := []string{latest.Source}
	controlled := map[string]bool{"domain": true, "http": true, "port": true, "php-version": true, "mysql-version": true, "mysql": true, "phpmyadmin": true}
	for i := 1; i < len(latest.Args); i++ {
		arg := latest.Args[i]
		key, _, _ := strings.Cut(strings.TrimLeft(arg, "-"), "=")
		if controlled[key] {
			if !strings.Contains(arg, "=") && i+1 < len(latest.Args) && !strings.HasPrefix(latest.Args[i+1], "-") {
				i++
			}
			continue
		}
		args = append(args, arg)
	}
	latest.Args = args
	latest.Domain = c.Domain
	if c.HTTPS != nil {
		latest.HTTPS = *c.HTTPS
	} else {
		latest.HTTPS = true
	}
	requested := 0
	if c.Port != nil {
		requested = *c.Port
		latest.Port = *c.Port
	} else {
		latest.Port = 0
	}
	latest.RequestedPort = &requested
	return result, projects.New(home).Save(latest)
}
func SaveEnvironment(ctx context.Context, home string, p *projects.Project, revision string, changes map[string]string) (projectconfig.Environment, error) {
	if err := os.MkdirAll(taskDir(home), 0700); err != nil {
		return projectconfig.Environment{}, err
	}
	taskRelease, taskOK, taskErr := filelock.Try(filepath.Join(taskDir(home), p.ID+".work.lock"))
	if taskErr != nil && !os.IsNotExist(taskErr) {
		return projectconfig.Environment{}, taskErr
	}
	if taskErr == nil {
		if !taskOK {
			return projectconfig.Environment{}, fmt.Errorf("wait for the project task before editing the environment")
		}
		defer taskRelease()
	}
	release, err := filelock.Acquire(ctx, filepath.Join(home, "startup.lock"))
	if err != nil {
		return projectconfig.Environment{}, err
	}
	defer release()
	if err := RequireNoTasks(home, p.ID); err != nil {
		return projectconfig.Environment{}, err
	}
	root, err := SettingsRoot(p)
	if err != nil {
		return projectconfig.Environment{}, err
	}
	return projectconfig.UpdateEnvironment(home, root, p.ID, revision, changes)
}

// ReadSettings includes remembered explicit options in the initial editor view.
// A save promotes these visible options into the shareable file before removing
// old machine-specific flags, so changing one field cannot toggle a service.
func ReadSettings(p *projects.Project) (projectconfig.Document, error) {
	root, err := SettingsRoot(p)
	if err != nil {
		return projectconfig.Document{}, err
	}
	doc, err := projectconfig.Read(root)
	if err != nil {
		return doc, err
	}
	c := &doc.Config
	flags := map[string]string{}
	for i := 1; i < len(p.Args); i++ {
		arg := p.Args[i]
		if !strings.HasPrefix(arg, "-") {
			continue
		}
		key, value, ok := strings.Cut(strings.TrimLeft(arg, "-"), "=")
		if !ok {
			value = "true"
			if i+1 < len(p.Args) && !strings.HasPrefix(p.Args[i+1], "-") {
				i++
				value = p.Args[i]
			}
		}
		flags[key] = value
	}
	if c.Domain == "" {
		if v, ok := flags["domain"]; ok {
			c.Domain = v
		}
	}
	if c.PHP == "" {
		c.PHP = flags["php-version"]
	}
	if c.MySQL == "" {
		c.MySQL = flags["mysql-version"]
	}
	if c.HTTPS == nil {
		if v, ok := flags["http"]; ok {
			http, e := strconv.ParseBool(v)
			if e == nil {
				https := !http
				c.HTTPS = &https
			}
		}
	}
	if c.Port == nil {
		if v, ok := flags["port"]; ok {
			port, e := strconv.Atoi(v)
			if e == nil {
				c.Port = &port
			}
		}
	}
	if c.PHPMyAdmin == nil {
		if v, ok := flags["phpmyadmin"]; ok {
			admin, e := strconv.ParseBool(v)
			if e == nil {
				c.PHPMyAdmin = &admin
			}
		}
	}
	if c.Database == "" {
		if v, ok := flags["mysql"]; ok {
			managed, e := strconv.ParseBool(v)
			if e == nil {
				c.Database = "external"
				if managed {
					c.Database = "managed"
				}
			}
		}
	}
	return doc, nil
}
