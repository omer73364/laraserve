// Package projectcheck explains project preparation without executing project
// code, installing dependencies, reading secret values, or changing files.
package projectcheck

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"laraserve/internal/frontend"
	"laraserve/internal/paths"
	"laraserve/internal/projectconfig"
	"laraserve/internal/runtime"
	"laraserve/internal/versions"
)

type Check struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	Status string `json:"status"`
	Detail string `json:"detail"`
	Target string `json:"target,omitempty"`
}
type Report struct {
	Checked  string  `json:"checked_utc"`
	Errors   int     `json:"errors"`
	Warnings int     `json:"warnings"`
	Checks   []Check `json:"checks"`
}

func Inspect(root, cache string) Report {
	r := Report{Checked: time.Now().UTC().Format(time.RFC3339), Checks: []Check{}}
	add := func(id, title, status, detail, target string) {
		r.Checks = append(r.Checks, Check{id, title, status, detail, target})
		if status == "error" {
			r.Errors++
		}
		if status == "warning" {
			r.Warnings++
		}
	}
	if info, err := os.Stat(root); err != nil || !info.IsDir() {
		add("project", "Project folder", "error", "The project folder is missing or cannot be opened. Restore it or add its new location.", "")
		return r
	}
	if regular(root, "artisan") {
		add("artisan", "Laravel application", "ok", "Artisan is present.", "")
	} else {
		add("artisan", "Laravel application", "error", "The artisan file is missing. Choose the Laravel application root.", "")
	}
	doc, configErr := projectconfig.Read(root)
	if configErr != nil {
		add("configuration", "Project settings", "error", configErr.Error(), "configuration")
	} else {
		add("configuration", "Project settings", "ok", "Project settings are valid.", "configuration")
	}
	data, err := projectconfig.ReadRegular(filepath.Join(root, "composer.json"), 1<<20)
	var composer struct {
		Require map[string]string `json:"require"`
	}
	if err == nil {
		err = json.Unmarshal(data, &composer)
	}
	if err != nil || composer.Require["laravel/framework"] == "" {
		add("php", "PHP requirements", "error", "A valid composer.json requiring laravel/framework is needed.", "configuration/runtimes")
	} else if configErr == nil {
		version, err := versions.ProjectPHP(root, runtime.PHPVersions(), "", "")
		if err != nil {
			add("php", "PHP requirements", "error", err.Error(), "configuration/runtimes")
		} else {
			add("php", "PHP requirements", "ok", "Compatible PHP "+version+" is available in the runtime catalog.", "configuration/runtimes")
		}
	}
	if regular(root, filepath.Join("vendor", "autoload.php")) {
		add("vendor", "PHP dependencies", "ok", "Composer dependencies are present; startup verifies them.", "actions")
	} else {
		add("vendor", "PHP dependencies", "automatic", "Missing Composer dependencies will be installed when you start the app.", "actions")
	}
	if regular(root, ".env") {
		add("environment", "Environment file", "ok", ".env is present. Its contents were not read by this check.", "environment")
	} else if _, err := os.Lstat(filepath.Join(root, ".env")); !os.IsNotExist(err) {
		add("environment", "Environment file", "error", ".env must be an accessible regular file. Check its type and permissions.", "environment")
	} else if regular(root, ".env.example") {
		add("environment", "Environment file", "automatic", ".env.example will be copied to .env on startup without overwriting existing values.", "environment")
	} else {
		add("environment", "Environment file", "warning", "No .env or .env.example was found. Review the application's environment requirements.", "environment")
	}
	if _, err := os.Lstat(filepath.Join(root, "package.json")); os.IsNotExist(err) {
		add("frontend", "Frontend", "ok", "No package.json: no frontend dependencies are needed.", "")
		return r
	}
	if doc.Config.Services.AutoDetect != nil && !*doc.Config.Services.AutoDetect && !doc.Config.Services.Vite {
		add("frontend", "Frontend preparation", "warning", "Automatic frontend preparation is disabled in project settings. Dependencies and asset builds must be managed manually or enabled in Development services.", "configuration/services")
		return r
	}
	var pkg struct {
		Scripts map[string]string `json:"scripts"`
	}
	data, err = projectconfig.ReadRegular(filepath.Join(root, "package.json"), 1<<20)
	if err == nil {
		err = json.Unmarshal(data, &pkg)
	}
	if err != nil {
		add("frontend", "Frontend manifest", "error", "package.json is unreadable or invalid JSON. Fix it before installing frontend dependencies.", "actions")
		return r
	}
	manager, _, err := versions.PackageManager(root)
	if err != nil {
		add("manager", "Package manager", "error", err.Error(), "actions")
	} else if manager != "npm" {
		if _, err := exec.LookPath(manager); err != nil {
			add("manager", "Package manager", "error", fmt.Sprintf("This app uses %s. Install it or enable its Corepack shim before starting.", manager), "actions")
		} else {
			add("manager", "Package manager", "ok", "Detected "+manager+".", "actions")
		}
	} else {
		add("manager", "Package manager", "ok", "npm will use this project's managed Node runtime.", "actions")
	}
	if available, err := runtime.CachedNodeVersions(cache); err == nil && configErr == nil {
		pins, lockErr := versions.ReadLock(filepath.Join(cache, "node-project-"+paths.ProjectID(root)+".json"))
		selection, err := versions.ResolveProjectNode(root, available, "", pins.Node, runtime.NodeLTSVersions(cache)...)
		if lockErr != nil {
			add("node", "Node compatibility", "error", "The saved Node selection could not be read. "+lockErr.Error(), "configuration/runtimes")
		} else if err != nil {
			add("node", "Node compatibility", "warning", "The cached release catalog could not resolve Node. Startup refreshes release metadata when needed. "+err.Error(), "configuration/runtimes")
		} else {
			add("node", "Node compatibility", "ok", "Node "+selection.Version+" · "+selection.Explanation(), "configuration/runtimes")
		}
	} else {
		add("node", "Node compatibility", "automatic", "Node requirements will be resolved during startup using the publisher's release catalog. This offline check does not download it.", "configuration/runtimes")
	}
	if info, err := os.Stat(filepath.Join(root, "node_modules")); err == nil && info.IsDir() {
		add("modules", "Frontend dependencies", "ok", "node_modules is present; startup checks whether installation inputs changed.", "actions")
	} else if err != nil && os.IsNotExist(err) {
		add("modules", "Frontend dependencies", "automatic", "Frontend dependencies will be installed when you start the app.", "actions")
	} else {
		add("modules", "Frontend dependencies", "error", "node_modules is not an accessible directory. Move the conflicting file or fix its permissions.", "actions")
	}
	if doc.Config.Services.Vite && !frontend.NeedsVite(root) {
		add("vite", "Vite configuration", "warning", "Vite is enabled, but this app does not declare it. Disable Vite for a Laravel Mix app or install the intended frontend tooling.", "configuration/services")
	}
	return r
}

func regular(root, name string) bool {
	info, err := os.Lstat(filepath.Join(root, name))
	return err == nil && info.Mode().IsRegular()
}
