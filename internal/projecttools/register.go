package projecttools

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"laraserve/internal/paths"
	"laraserve/internal/projectconfig"
	"laraserve/internal/projects"
)

// RegisterExisting records an app without executing PHP, installing dependencies,
// changing project files, starting services, or provisioning a database.
func RegisterExisting(home, source string) (*projects.Project, error) {
	if !filepath.IsAbs(source) {
		return nil, fmt.Errorf("choose an absolute project folder")
	}
	root, err := projects.Canonical(source)
	if err != nil {
		return nil, fmt.Errorf("cannot open project folder: %w", err)
	}
	info, err := os.Stat(root)
	if err != nil || !info.IsDir() {
		return nil, fmt.Errorf("choose a project folder, not a file")
	}
	store := projects.New(home)
	records, err := store.List()
	if err != nil {
		return nil, err
	}
	for _, existing := range records {
		canonical, err := projects.Canonical(existing.Source)
		if err == nil && canonical == root {
			return existing, nil
		}
	}
	if _, err = projectconfig.ReadRegular(filepath.Join(root, "artisan"), 1<<20); err != nil {
		return nil, fmt.Errorf("choose a Laravel app with a regular artisan file")
	}
	b, err := projectconfig.ReadRegular(filepath.Join(root, "composer.json"), 1<<20)
	if err != nil {
		return nil, fmt.Errorf("cannot read this app's composer.json")
	}
	var composer struct {
		Require map[string]string `json:"require"`
	}
	if json.Unmarshal(b, &composer) != nil || composer.Require["laravel/framework"] == "" {
		return nil, fmt.Errorf("composer.json must require laravel/framework")
	}
	config, err := projectconfig.Read(root)
	if err != nil {
		return nil, err
	}
	database := map[string]string{"managed": "Private MySQL", "external": "External", "sqlite": "SQLite"}[config.Config.Database]
	p := &projects.Project{ID: paths.ProjectID(root), Name: filepath.Base(root), Source: root, Root: root, Args: []string{root}, HTTPS: true, PHP: config.Config.PHP, Database: database}
	// Import never overwrites an existing record, including its saved launch flags.
	if err = store.Import(p); err != nil {
		return nil, err
	}
	return store.Load(p.ID)
}
