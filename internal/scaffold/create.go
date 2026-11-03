// Package scaffold creates new projects only. Its migration allowance never
// applies to serving, restarting, or operating an existing application.
package scaffold

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	goruntime "runtime"
	"strings"

	"golang.org/x/term"
	"laraserve/internal/detect"
	"laraserve/internal/env"
	"laraserve/internal/filelock"
	"laraserve/internal/manifest"
	"laraserve/internal/mysql"
	"laraserve/internal/paths"
	"laraserve/internal/prepare"
	"laraserve/internal/projectconfig"
	"laraserve/internal/projects"
	"laraserve/internal/runtime"
	"laraserve/internal/ui"
	"laraserve/internal/versions"
)

type Request struct {
	Path           string `json:"path"`
	Version        string `json:"version,omitempty"`
	PHP            string `json:"php,omitempty"`
	Starter        string `json:"starter,omitempty"`
	Database       string `json:"database,omitempty"`
	Migrate        *bool  `json:"migrate,omitempty"`
	OfficialWizard bool   `json:"official_wizard,omitempty"`
	HostPath       bool   `json:"host_path,omitempty"`
	AllowInsecure  bool   `json:"allow_insecure_dependencies,omitempty"`
}

func (r Request) Validate() error {
	if !filepath.IsAbs(r.Path) || len(r.Path) > 4096 || strings.ContainsRune(r.Path, 0) {
		return fmt.Errorf("choose an absolute destination path")
	}
	switch r.Starter {
	case "", "none", "react", "vue", "svelte", "livewire":
	default:
		return fmt.Errorf("unknown starter kit")
	}
	switch r.Database {
	case "", "sqlite", "mysql", "external":
	default:
		return fmt.Errorf("choose sqlite, mysql or external")
	}
	if len(r.Version) > 96 || strings.ContainsAny(r.Version, "\r\n\x00") {
		return fmt.Errorf("invalid skeleton constraint")
	}
	entries, err := os.ReadDir(r.Path)
	if err == nil && len(entries) > 0 {
		return fmt.Errorf("destination is not empty; choose a new or empty directory")
	}
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("destination: %w", err)
	}
	if r.Starter != "" && r.Starter != "none" && r.Version != "" {
		return fmt.Errorf("skeleton version applies only to the plain Laravel skeleton")
	}
	return nil
}
func Create(ctx context.Context, home string, r Request) error {
	if err := r.Validate(); err != nil {
		return err
	}
	if r.OfficialWizard && !term.IsTerminal(int(os.Stdin.Fd())) {
		return fmt.Errorf("--wizard requires an interactive terminal; use --starter and --database for unattended creation")
	}
	release, err := filelock.Acquire(ctx, filepath.Join(home, "startup.lock"))
	if err != nil {
		return err
	}
	defer release()
	// Recheck the destination after waiting. Partial apps are never overwritten.
	if err = r.Validate(); err != nil {
		return err
	}
	root, err := Destination(r.Path)
	if err != nil {
		return err
	}
	if err = guardCreationState(home, root); err != nil {
		return err
	}
	selected := r.PHP
	var rt *runtime.Runtime
	if selected != "" {
		selected, err = versions.ProjectPHP(r.Path, runtime.PHPVersions(), selected, "")
		if err != nil {
			return err
		}
		rt, err = runtime.EnsurePHP(filepath.Join(home, "runtimes"), selected)
	} else {
		rt, err = runtime.Ensure(filepath.Join(home, "runtimes"))
	}
	if err != nil {
		return err
	}
	if err = runtime.EnsureComposer(rt); err != nil {
		return err
	}
	stateDir := filepath.Join(home, "projects", paths.ProjectID(root))
	baseEnv := env.Build(rt, stateDir, r.HostPath, nil)
	if r.OfficialWizard {
		if err = officialWizard(ctx, home, rt, root, r, baseEnv); err != nil {
			return err
		}
	} else {
		pkg := "laravel/laravel"
		if r.Starter != "" && r.Starter != "none" {
			pkg = "laravel/" + r.Starter + "-starter-kit"
		}
		ui.Info("Creating %s in %s", pkg, root)
		if err = prepare.ComposerCreatePackage(ctx, rt.PHP, rt.Composer, root, pkg, r.Version, baseEnv, r.AllowInsecure); err != nil {
			return fmt.Errorf("creation failed: %w; partial files were retained in %s", err, root)
		}
	}
	if _, err = detect.Inspect(root); err != nil {
		return err
	}
	root, err = projects.Canonical(root)
	if err != nil {
		return err
	}
	stateDir = filepath.Join(home, "projects", paths.ProjectID(root))
	if r.PHP != "" {
		if err = os.WriteFile(filepath.Join(root, ".php-version"), []byte(r.PHP+"\n"), 0644); err != nil {
			return err
		}
	}
	if !r.OfficialWizard {
		doc, e := projectconfig.ReadEnvironment(root)
		if e != nil {
			return e
		}
		driver := r.Database
		if driver == "" {
			driver = "sqlite"
		}
		if driver != "external" {
			changes := map[string]string{"DB_CONNECTION": driver}
			if driver == "mysql" {
				name := regexp.MustCompile(`[^a-z0-9]+`).ReplaceAllString(strings.ToLower(filepath.Base(root)), "_")
				name = strings.Trim(name, "_")
				if name == "" {
					name = "app"
				}
				if len(name) > 48 {
					name = name[:48]
				}
				changes["DB_DATABASE"] = name
			}
			if _, e = projectconfig.UpdateEnvironment(home, root, paths.ProjectID(root), doc.Revision, changes); e != nil {
				return e
			}
		}
	}
	footprint, err := manifest.Load(filepath.Join(home, "manifest.json"))
	if err != nil {
		return err
	}
	footprint.Project(paths.ProjectID(root), root)
	if err = footprint.Save(); err != nil {
		return err
	}
	migrate := r.Migrate == nil || *r.Migrate
	// An external DB is deliberately not migrated by a creation default.
	if migrate && r.Database != "external" {
		st, e := prepare.EnsureState(stateDir)
		if e != nil {
			return e
		}
		extra := st.AppEnv(root, "http://localhost")
		if r.Database == "mysql" {
			version, e := versions.ProjectMySQL(root, runtime.MySQLVersions(), "", "")
			if e != nil {
				return e
			}
			base, e := runtime.EnsureMySQLVersion(filepath.Join(home, "runtimes"), version)
			if e != nil {
				return e
			}
			db, e := mysql.Start(ctx, base, filepath.Join(home, "databases", paths.ProjectID(root), "mysql-"+version), baseEnv, filepath.Base(root))
			if e != nil {
				return e
			}
			defer db.Close()
			for k, v := range db.Env {
				extra[k] = v
			}
		}
		if e = prepare.ArtisanContext(ctx, rt.PHP, root, env.Build(rt, st.Dir, r.HostPath, extra), "migrate", "--force", "--no-interaction"); e != nil {
			return fmt.Errorf("new project created but migration failed: %w; files and data were retained", e)
		}
	}
	canonical, err := projects.Canonical(root)
	if err != nil {
		return err
	}
	p := &projects.Project{ID: paths.ProjectID(canonical), Name: filepath.Base(canonical), Source: canonical, Root: canonical, Args: []string{canonical}, HTTPS: true, PHP: selected}
	if err = projects.New(home).Import(p); err != nil {
		return err
	}
	ui.OK("Project created: %s", root)
	return nil
}

// Reusing a folder name must not apply a new app's migrations to retained data.
func guardCreationState(home, root string) error {
	ids := map[string]bool{paths.ProjectID(root): true}
	old, err := manifest.Load(filepath.Join(home, "manifest.json"))
	if err != nil {
		return err
	}
	for id, p := range old.Projects {
		if filepath.Clean(p.Path) == root || goruntime.GOOS == "windows" && strings.EqualFold(filepath.Clean(p.Path), root) {
			ids[id] = true
		}
	}
	for id := range ids {
		if !projects.ValidID(id) {
			return fmt.Errorf("invalid retained project state")
		}
		for _, path := range []string{filepath.Join(home, "projects", id, "database.sqlite"), filepath.Join(home, "databases", id)} {
			if _, err := os.Lstat(path); err == nil {
				return fmt.Errorf("private database state already exists for this destination; choose a new path or inspect the retained state before creating another app")
			} else if !os.IsNotExist(err) {
				return err
			}
		}
	}
	return nil
}

// Destination computes the identity before the final app directory exists.
func Destination(path string) (string, error) {
	parent, err := filepath.EvalSymlinks(filepath.Dir(path))
	if err != nil {
		return "", fmt.Errorf("destination parent must exist: %w", err)
	}
	root := filepath.Join(parent, filepath.Base(path))
	if goruntime.GOOS == "windows" {
		root = strings.ToLower(root)
	}
	return root, nil
}
